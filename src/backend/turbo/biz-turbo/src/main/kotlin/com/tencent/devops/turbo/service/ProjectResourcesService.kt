package com.tencent.devops.turbo.service

import com.tencent.devops.common.api.exception.TurboException
import com.tencent.devops.common.api.exception.code.TURBO_PARAM_INVALID
import com.tencent.devops.common.api.pojo.Page
import com.tencent.devops.common.util.MathUtil
import com.tencent.devops.common.util.constants.BASE_EXCLUDED_PLAN_ID_LIST
import com.tencent.devops.common.util.constants.BASE_EXCLUDED_PROJECT_ID_LIST
import com.tencent.devops.turbo.config.TodCostProperties
import com.tencent.devops.turbo.dao.mongotemplate.TbsDaySummaryDao
import com.tencent.devops.turbo.dao.repository.BaseDataRepository
import com.tencent.devops.turbo.sdk.TodCostApi
import com.tencent.devops.turbo.vo.ProjectResourceCostVO
import com.tencent.devops.turbo.vo.ProjectResourceUsageVO
import com.tencent.devops.turbo.vo.ResourceCostSummary
import com.tencent.devops.web.util.SpringContextHolder
import org.slf4j.LoggerFactory
import org.springframework.beans.factory.annotation.Autowired
import org.springframework.stereotype.Service
import java.time.LocalDate
import java.time.YearMonth

@Service
class ProjectResourcesService @Autowired constructor(
    private val tbsDaySummaryDao: TbsDaySummaryDao,
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

        // 获取需要过滤掉的方案id集合
        val baseDataEntity = baseDataRepository.findFirstByParamCode(BASE_EXCLUDED_PLAN_ID_LIST)
        val filterPlanIds = baseDataEntity?.paramValue?.split(",")?.toSet() ?: emptySet()

        // 获取需要过滤掉的项目id集合
        val projectExcludedEntity = baseDataRepository.findFirstByParamCode(BASE_EXCLUDED_PROJECT_ID_LIST)
        val filterProjectIds = projectExcludedEntity?.paramValue?.split(",")?.toSet() ?: emptySet()

        val today = LocalDate.now()

        val summaryEntityList = tbsDaySummaryDao.findByDay(
            startDate = startDate ?: today.minusMonths(1).withDayOfMonth(1).toString(),
            endDate = endDate ?: today.withDayOfMonth(1).minusDays(1).toString(),
            filterPlanIdNin = filterPlanIds,
            filterProjectIdNin = filterProjectIds,
            pageNum = page,
            pageSize = pageSizeNum
        )
        logger.info("summaryEntityList size: ${summaryEntityList.size}")

        val resultList = summaryEntityList.filter { !(it.projectId.isNullOrBlank()) }.map {
            with(it) {
                ProjectResourceUsageVO(
                    projectId = projectId!!,
                    projectName = projectName,
                    engineCode = engineCode,
                    // 秒转分钟
                    totalTimeWithCpu = MathUtil.secondsToMinutes(totalTimeWithCpu!!).toDouble(),
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
        val year = month.substring(0, 4).toInt()
        val monthValue = month.substring(4).toInt()
        val monthParam = YearMonth.of(year, monthValue).month
        // 归属月份需要在日期范围内，且开始日期不能大于结束日期，且结束日期不能大于当前日期
        if (monthParam < start.month || monthParam > end.month || start > end || end > LocalDate.now()) {
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

        val filterPlanIds = getFilterIds(BASE_EXCLUDED_PLAN_ID_LIST)
        val filterProjectIds = getFilterIds(BASE_EXCLUDED_PROJECT_ID_LIST)
        val properties = SpringContextHolder.getBean<TodCostProperties>()

        this.uploadDataByType(
            start = start.toString(),
            end = end.toString(),
            month = month,
            filterPlanIds = filterPlanIds,
            filterProjectIds = filterProjectIds,
            kind = KIND_C_PLUS,
            serviceType = properties.serviceType
        )

        this.uploadDataByType(
            start = start.toString(),
            end = end.toString(),
            month = month,
            filterPlanIds = filterPlanIds,
            filterProjectIds = filterProjectIds,
            kind = KIND_UE_CLIENT,
            serviceType = properties.serviceType
        )
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
                TodCostApi.uploadByPage(month = month, data)
            }
            pageNum++
        } while (entityList.size == UPLOAD_PAGE_SIZE)
        logger.info("processing $kind data completed")
    }

    /**
     * 手动上报指定的数据
     */
    fun manualUploadCostData(summary: ResourceCostSummary): Boolean {
        return TodCostApi.uploadByPage(summary.month, summary.bills)
    }
}
