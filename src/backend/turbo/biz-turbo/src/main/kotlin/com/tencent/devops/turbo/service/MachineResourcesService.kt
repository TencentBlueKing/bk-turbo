package com.tencent.devops.turbo.service

import com.tencent.devops.common.util.constants.BASE_EXCLUDED_PLAN_ID_LIST
import com.tencent.devops.turbo.dao.mongotemplate.TbsDaySummaryDao
import com.tencent.devops.turbo.dao.repository.BaseDataRepository
import com.tencent.devops.turbo.vo.apiwg.MachineResourcesStatVO
import org.slf4j.LoggerFactory
import org.springframework.beans.factory.annotation.Autowired
import org.springframework.stereotype.Service
import java.time.LocalDate

@Service
class MachineResourcesService @Autowired constructor(
    private val tbsDaySummaryDao: TbsDaySummaryDao,
    private val baseDataRepository: BaseDataRepository
) {

    companion object {
        private val logger = LoggerFactory.getLogger(this::class.java)
    }

    /**
     * 不传日期时默认统计上个月所有天数的数据
     */
    fun querySummary(startDate: String?, endDate: String?): List<MachineResourcesStatVO> {
        // 获取需要过滤掉的方案id集合
        val baseDataEntity = baseDataRepository.findFirstByParamCode(BASE_EXCLUDED_PLAN_ID_LIST)
        val filterPlanIds = baseDataEntity.takeIf { it != null }?.paramValue?.split(",")?.toSet() ?: emptySet()

        val today = LocalDate.now()

        val summaryEntityList = tbsDaySummaryDao.findByDay(
            startDate = startDate ?: today.minusMonths(1).withDayOfMonth(1).toString(),
            endDate = endDate ?: today.withDayOfMonth(1).minusDays(1).toString(),
            filterPlanIdNin = filterPlanIds
        )
        logger.info("summaryEntityList size: ${summaryEntityList.size}")

        return summaryEntityList.map {
            with(it) {
                MachineResourcesStatVO(
                    projectId = projectId!!,
                    projectName = projectName,
                    planId = planId!!,
                    planName = planName,
                    planCreator = planCreator,
                    engineCode = engineCode,
                    totalTimeWithCpu = totalTimeWithCpu,
                    productId = productId,
                    bgName = bgName,
                    bgId = bgId,
                    deptName = deptName,
                    deptId = deptId,
                    centerName = centerName,
                    centerId = centerId
                )
            }
        }
    }
}
