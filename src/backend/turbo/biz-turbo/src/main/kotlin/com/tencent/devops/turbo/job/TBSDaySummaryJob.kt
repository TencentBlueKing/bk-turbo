package com.tencent.devops.turbo.job

import com.tencent.devops.common.util.DateTimeUtils
import com.tencent.devops.common.util.JsonUtil
import com.tencent.devops.turbo.dao.repository.TurboEngineConfigRepository
import com.tencent.devops.turbo.dao.repository.TurboPlanRepository
import com.tencent.devops.turbo.dto.TBSDaySummaryDto
import com.tencent.devops.turbo.model.TTbsDaySummaryEntity
import com.tencent.devops.turbo.sdk.TBSSdkApi
import org.quartz.Job
import org.quartz.JobExecutionContext
import org.slf4j.LoggerFactory
import org.springframework.beans.factory.annotation.Autowired
import java.time.LocalDate
import java.time.LocalDateTime

@Suppress("SpringJavaAutowiredMembersInspection")
class TBSDaySummaryJob @Autowired constructor(
    private val turboEngineConfigRepository: TurboEngineConfigRepository,
    private val turboPlanRepository: TurboPlanRepository
) : Job {

    companion object {
        private val logger = LoggerFactory.getLogger(this::class.java)
        private const val PAGE_SIZE = 3000
    }

    /**
     * 执行入口
     */
    override fun execute(context: JobExecutionContext) {
        logger.info("TBS day summary job start executing: ${JsonUtil.toJson(context.jobDetail)}")

        val jobParam = context.jobDetail.jobDataMap
        val statisticsDateStr = if (jobParam.containsKey("statisticsDate")) {
            jobParam["statisticsDate"] as String
        } else {
            // 统计昨天
            val statLocalDate = LocalDate.now().minusDays(1)
            DateTimeUtils.localDate2DateStr(statLocalDate)
        }

        val engineConfigEntities = turboEngineConfigRepository.findAll()
        engineConfigEntities.forEach { engineConfig ->
            val daySummaryDtoList = TBSSdkApi.queryTbsDaySummary(
                engineCode = engineConfig.engineCode,
                queryParam = mapOf(
                    "day" to statisticsDateStr
                )
            )

            logger.info("daySummaryDtoList size: ${daySummaryDtoList.size}")
            if (daySummaryDtoList.isEmpty()) {
                logger.warn("queryTbsDaySummary result is empty! engineCode: ${engineConfig.engineCode}")
                return@forEach
            }

            val summaryEntityList = this.dto2SummaryEntityList(daySummaryList = daySummaryDtoList)
            val planIdEntityMap = summaryEntityList.associateBy { it.planId }

            val planIdsList = planIdEntityMap.keys.chunked(PAGE_SIZE)
            for (planIds in planIdsList) {
                val turboPlanList = turboPlanRepository.findByIdIn(planIds)

            }
        }
    }

    /**
     * 把TBS的接口数据整理成entity
     */
    private fun dto2SummaryEntityList(daySummaryList: List<TBSDaySummaryDto>): List<TTbsDaySummaryEntity> {
        val summaryEntities = mutableListOf<TTbsDaySummaryEntity>()

        daySummaryList.forEach { summary ->
            // distcc与其它不一样，它的projectId就是planId
            val planIdAndEngineCode = summary.projectId

            val planId: String
            val engineCode: String

            // "60d54b87a26123319d011bob_cc"
            if (planIdAndEngineCode.contains("_")) {
                val stringArr = planIdAndEngineCode.split("_")
                planId = stringArr[0]
                engineCode = if (stringArr[1] == "cc") "disttask-cc" else if (stringArr[1] == "ue4") "disttask-ue4"
                    else stringArr[1]
            } else {
                planId = planIdAndEngineCode
                engineCode = "distcc"
            }

            val entity = TTbsDaySummaryEntity(
                day = summary.day,
                engineCode = engineCode,
                planId = planId,
                user = if (engineCode == "disttask-ue4") summary.user else null,
                totalTime = summary.totalTime,
                totalTimeWithCpu = summary.totalTimeWithCpu,
                totalRecordNumber = summary.totalRecordNumber,
                createdDate = LocalDateTime.now()
            )
            summaryEntities.add(entity)
        }
        return summaryEntities
    }

}