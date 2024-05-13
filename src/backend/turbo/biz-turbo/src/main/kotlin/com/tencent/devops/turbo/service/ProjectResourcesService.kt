package com.tencent.devops.turbo.service

import com.tencent.devops.common.api.pojo.Page
import com.tencent.devops.common.util.MathUtil
import com.tencent.devops.common.util.constants.BASE_EXCLUDED_PLAN_ID_LIST
import com.tencent.devops.common.util.constants.BASE_EXCLUDED_PROJECT_ID_LIST
import com.tencent.devops.turbo.dao.mongotemplate.TbsDaySummaryDao
import com.tencent.devops.turbo.dao.repository.BaseDataRepository
import com.tencent.devops.turbo.vo.ProjectResourceUsageVO
import org.slf4j.LoggerFactory
import org.springframework.beans.factory.annotation.Autowired
import org.springframework.stereotype.Service
import java.time.LocalDate

@Service
class ProjectResourcesService @Autowired constructor(
    private val tbsDaySummaryDao: TbsDaySummaryDao,
    private val baseDataRepository: BaseDataRepository
) {

    companion object {
        private val logger = LoggerFactory.getLogger(this::class.java)
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
}
