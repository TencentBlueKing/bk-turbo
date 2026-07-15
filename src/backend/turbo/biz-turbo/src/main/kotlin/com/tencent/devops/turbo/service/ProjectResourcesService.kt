package com.tencent.devops.turbo.service

import com.google.common.collect.Lists
import com.tencent.devops.common.api.exception.TurboException
import com.tencent.devops.common.api.exception.code.TURBO_PARAM_INVALID
import com.tencent.devops.common.api.pojo.Page
import com.tencent.devops.common.util.JsonUtil
import com.tencent.devops.common.util.MathUtil
import com.tencent.devops.common.util.constants.BASE_EXCLUDED_COMMON_PLAN_ID
import com.tencent.devops.common.util.constants.BASE_EXCLUDED_PROJECT_ID_LIST
import com.tencent.devops.turbo.config.TodCostProperties
import com.tencent.devops.turbo.dao.mongotemplate.TbsDaySummaryDao
import com.tencent.devops.turbo.dao.mongotemplate.TbsPrivateDaySummaryDao
import com.tencent.devops.turbo.dao.repository.BaseDataRepository
import com.tencent.devops.turbo.sdk.TodCostApi
import com.tencent.devops.turbo.vo.ProjectResourceCostVO
import com.tencent.devops.turbo.vo.ProjectResourceUsageVO
import com.tencent.devops.turbo.vo.ResourceCostSummary
import com.tencent.devops.web.util.SpringContextHolder
import kotlinx.coroutines.GlobalScope
import kotlinx.coroutines.launch
import org.slf4j.LoggerFactory
import org.springframework.beans.factory.annotation.Autowired
import org.springframework.stereotype.Service
import java.time.LocalDate

@Service
class ProjectResourcesService @Autowired constructor(
    private val tbsDaySummaryDao: TbsDaySummaryDao,
    private val tbsPrivateDaySummaryDao: TbsPrivateDaySummaryDao,
    private val baseDataRepository: BaseDataRepository
) {

    companion object {
        private val logger = LoggerFactory.getLogger(this::class.java)
        private const val UPLOAD_PAGE_SIZE = 1000
        private const val KIND_C_PLUS = "TURBO_C++"
        private const val KIND_UE_CLIENT = "TURBO_UE_CLIENT"
    }

    /**
     * 不传日期时默认统计上个月所有天数的数据
     */
    fun querySummary(
        startDate: String?,
        endDate: String?,
        pageNum: Int?,
        pageSize: Int?
    ): Page<ProjectResourceUsageVO> {
        val page = pageNum?.takeIf { it > 0 }?.let { it - 1 } ?: 0
        val pageSizeNum = pageSize?.coerceAtMost(10000) ?: 100

        // 成本分摊时只过滤内部测试方案，不再过滤 DevCloud 专用方案
        // 因为同一方案可能既用私有资源又用公共资源，需要按 总资源 - 私有资源 计算实际公共资源
        val filterPlanIds = getFilterIds(BASE_EXCLUDED_COMMON_PLAN_ID)
        val filterProjectIds = getFilterIds(BASE_EXCLUDED_PROJECT_ID_LIST)

        val today = LocalDate.now()
        val start = startDate ?: today.minusMonths(1).withDayOfMonth(1).toString()
        val end = endDate ?: today.withDayOfMonth(1).minusDays(1).toString()

        // 查询总资源数据（分页，按项目+引擎分组汇总）
        val summaryEntityList = tbsDaySummaryDao.findByDay(
            startDate = start,
            endDate = end,
            filterPlanIdNin = filterPlanIds,
            filterProjectIdNin = filterProjectIds,
            pageNum = page,
            pageSize = pageSizeNum
        )
        logger.info("summaryEntityList size: ${summaryEntityList.size}")

        // 查询私有资源数据（不分页，按项目+引擎分组汇总），用于从总资源中扣除
        val privateEntityList = tbsPrivateDaySummaryDao.findByDay(
            startDate = start,
            endDate = end,
            filterPlanIdNin = filterPlanIds,
            filterProjectIdNin = filterProjectIds
        )
        logger.info("privateSummaryEntityList size: ${privateEntityList.size}")

        // 构建私有资源 Map: key = "projectId|engineCode" -> totalTimeWithCpu
        val privateMap = privateEntityList.filter { !it.projectId.isNullOrBlank() }.associate {
            "${it.projectId}|${it.engineCode}" to (it.totalTimeWithCpu ?: 0.0)
        }

        // 从总资源中减去私有资源，得到实际使用的公共资源
        val resultList = summaryEntityList.filter { !it.projectId.isNullOrBlank() }.mapNotNull {
            val key = "${it.projectId}|${it.engineCode}"
            val totalTimeWithCpu = it.totalTimeWithCpu ?: 0.0
            val privateTimeWithCpu = privateMap[key] ?: 0.0
            val publicTimeWithCpu = totalTimeWithCpu - privateTimeWithCpu
            // 公共资源为0或负数的项目不需要分摊成本
            if (publicTimeWithCpu <= 0) return@mapNotNull null

            with(it) {
                ProjectResourceUsageVO(
                    projectId = projectId!!,
                    projectName = projectName,
                    engineCode = engineCode,
                    // 秒转分钟
                    totalTimeWithCpu = MathUtil.secondsToMinutes(publicTimeWithCpu).toDouble(),
                    productId = productId,
                    bgName = bgName,
                    bgId = bgId,
                    businessLineName = businessLineName,
                    businessLineId = businessLineId,
                    deptName = deptName,
                    deptId = deptId,
                    centerName = centerName,
                    centerId = centerId
                )
            }
        }
        return Page(page + 1, pageSizeNum, 0, resultList)
    }

    private fun checkDatesInMonth(month: String, start: LocalDate, end: LocalDate) {
        val monthDate = LocalDate.parse("${month.substring(0, 4)}-${month.substring(4, 6)}-01")
        // 归属月份需要在日期范围内，且开始日期不能大于结束日期，且结束日期不能大于当前日期
        if (monthDate.isBefore(start) || monthDate.isAfter(end) || start.isAfter(end)) {
            throw TurboException(
                TURBO_PARAM_INVALID,
                "The start and end dates are not within the month range of $month or start date is after end date."
            )
        }
    }

    private fun getFilterIds(paramCode: String): Set<String> {
        return baseDataRepository.findFirstByParamCode(paramCode)?.paramValue?.split(",")?.toSet() ?: emptySet()
    }

    /**
     * 触发自动统计和上报数据
     */
    fun triggerStatAndUpload(month: String, startDate: String?, endDate: String?): Boolean {
        logger.info("triggerStatAndUpload param: $month  $startDate, $endDate")

        val today = LocalDate.now()
        val start = startDate?.let { LocalDate.parse(it) } ?: today.minusMonths(1).withDayOfMonth(15)
        val end = endDate?.let { LocalDate.parse(it) } ?: today.withDayOfMonth(14)
        this.checkDatesInMonth(month, start, end)

        val filterPlanIds = getFilterIds(BASE_EXCLUDED_COMMON_PLAN_ID)
        val filterProjectIds = getFilterIds(BASE_EXCLUDED_PROJECT_ID_LIST)
        val properties = SpringContextHolder.getBean<TodCostProperties>()

        Lists.newArrayList(KIND_C_PLUS, KIND_UE_CLIENT).forEach {
            GlobalScope.launch {
                try {
                    uploadDataByType(
                        start.toString(),
                        end.toString(),
                        month,
                        filterPlanIds,
                        filterProjectIds,
                        it,
                        properties.serviceType
                    )
                } catch (e: Throwable) {
                    logger.error(e.message, e)
                }
            }
        }
        return true
    }

    /**
     * 统计和上报数据
     */
    private fun uploadDataByType(
        start: String,
        end: String,
        month: String,
        filterPlanIds: Set<String>,
        filterProjectIds: Set<String>,
        kind: String,
        serviceType: String,
    ) {
        logger.info("start processing $kind data")
        var pageNum = 0
        do {
            val entityList = when (kind) {
                KIND_C_PLUS -> tbsDaySummaryDao.findByDayForCPlus(
                    startDate = start,
                    endDate = end,
                    filterPlanIdNin = filterPlanIds,
                    filterProjectIdNin = filterProjectIds,
                    pageNum = pageNum,
                    pageSize = UPLOAD_PAGE_SIZE
                )
                KIND_UE_CLIENT -> tbsDaySummaryDao.findByDayForUE(
                    startDate = start,
                    endDate = end,
                    filterPlanIdNin = filterPlanIds,
                    filterProjectIdNin = filterProjectIds,
                    pageNum = pageNum,
                    pageSize = UPLOAD_PAGE_SIZE
                )
                else -> emptyList()
            }
            logger.info("$kind entity list size: ${entityList.size}")

            val data = entityList.filter { it.projectId?.isNotBlank() == true }.map {
                with(it) {
                    val usageNum = if (kind == KIND_C_PLUS) {
                        MathUtil.secondsToMinutes(totalTimeWithCpu!!)
                    } else {
                        totalTimeWithCpu.toString()
                    }
                    ProjectResourceCostVO(
                        costDate = month,
                        projectId = projectId!!,
                        name = projectName,
                        kind = kind,
                        serviceType = serviceType,
                        usage = usageNum,
                        bgName = bgName,
                        flag = 1
                    )
                }
            }
            if (data.isNotEmpty()) {
                logger.info("upload $kind by page: ${pageNum + 1}, size: ${data.size}")
                val uploadSuccess = TodCostApi.postData(month = month, data)
                if (!uploadSuccess) {
                    logger.error("upload $kind failed!!")
                    break
                }
            }
            pageNum++
        } while (entityList.size == UPLOAD_PAGE_SIZE)
        logger.info("processing $kind data completed")
    }

    /**
     * 手动上报指定的数据
     */
    fun manualUploadCostData(summary: ResourceCostSummary): Boolean {
        logger.info("manualUploadCostData: ${JsonUtil.toJson(summary)}")
        GlobalScope.launch {
            try {
                TodCostApi.upload(body = summary)
            } catch (e: Throwable) {
                logger.error(e.message, e)
            }
        }
        return true
    }
}
