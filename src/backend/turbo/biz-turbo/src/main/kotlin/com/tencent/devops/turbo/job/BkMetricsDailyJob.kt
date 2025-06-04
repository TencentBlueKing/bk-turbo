package com.tencent.devops.turbo.job

import com.tencent.devops.common.util.JsonUtil
import com.tencent.devops.turbo.component.BkMetricsStatUpload
import org.quartz.Job
import org.quartz.JobExecutionContext
import org.slf4j.LoggerFactory
import org.springframework.beans.factory.annotation.Autowired

@Suppress("SpringJavaAutowiredMembersInspection")
class BkMetricsDailyJob @Autowired constructor(
    private val bkMetricsStatUpload: BkMetricsStatUpload
) : Job {

    companion object {
        private val logger = LoggerFactory.getLogger(BkMetricsDailyJob::class.java)
        private const val PAGE_SIZE_KEY = "pageSize"
        private const val STATISTICS_DATE_KEY = "statisticsDate"
    }

    override fun execute(context: JobExecutionContext) {
        logger.info("BkMetricsDailyJob context: ${JsonUtil.toJson(context.jobDetail)}")

        runCatching {
            val jobParam = context.jobDetail.jobDataMap
            val pageSize = jobParam[PAGE_SIZE_KEY] as? Int
            val statisticsDate = jobParam[STATISTICS_DATE_KEY] as? String

            logger.info("pageSize: $pageSize, statisticsDate: $statisticsDate")
            bkMetricsStatUpload.entry(statisticsDate, pageSize)
        }.onFailure { e ->
            logger.error("BkMetricsDailyJob execute failed: ${e.message}", e)
        }

        logger.info("BkMetricsDailyJob execute finish")
    }
}
