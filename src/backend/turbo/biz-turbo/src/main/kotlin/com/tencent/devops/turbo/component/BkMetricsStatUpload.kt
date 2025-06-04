package com.tencent.devops.turbo.component

import com.tencent.devops.common.client.Client
import com.tencent.devops.common.util.DateTimeUtils
import com.tencent.devops.common.util.MathUtil
import com.tencent.devops.metrics.api.ServiceMetricsDataReportResource
import com.tencent.devops.metrics.pojo.dto.TurboDataReportDTO
import com.tencent.devops.turbo.dao.mongotemplate.TurboSummaryDao
import com.tencent.devops.turbo.pojo.TurboDaySummaryOverviewModel
import org.slf4j.LoggerFactory
import org.springframework.stereotype.Component
import java.time.LocalDate

/**
 * 按天统计和上报项目维度metrics数据
 */
@Component
class BkMetricsStatUpload(
    private val turboSummaryDao: TurboSummaryDao,
    private val client: Client
){
    companion object{
        private val logger = LoggerFactory.getLogger(this::class.java)
        private const val DEFAULT_PAGE_SIZE = 2000
    }

    fun entry(statisticsDate: String?, pageSizeParam: Int?): Boolean {
        val pageSize = pageSizeParam ?: DEFAULT_PAGE_SIZE

        // 生成统计时间戳
        val statisticsLocalDate = statisticsDate.takeUnless { it.isNullOrBlank() }
            ?.let { DateTimeUtils.dateStr2LocalDate(it) }
            ?: LocalDate.now().minusDays(1)

        val statisticsDateStr = DateTimeUtils.localDate2DateStr(statisticsLocalDate)

        // 分页从0开始统计，表示第一页
        var pageNum = 0
        do {
            val projectDaySummaryPage = turboSummaryDao.findProjectBySummaryDatePage(
                summaryDate = statisticsLocalDate,
                pageNum = pageNum,
                pageSize = pageSize
            )
            if (projectDaySummaryPage.isEmpty()) {
                break
            }

            projectDaySummaryPage.forEach {
                processAndUpload(statisticsDate = statisticsDateStr, overviewModel = it)
            }

            pageNum++
        } while (projectDaySummaryPage.size == pageSize)
        return true
    }

    /**
     * 计算节省时间及推送数据
     * 2025-4-23：取消队列推送，改为接口调用
     */
    private fun processAndUpload(statisticsDate: String, overviewModel: TurboDaySummaryOverviewModel) {
        val projectId = overviewModel.projectId ?: run {
            logger.error("Project ID is null in overview model: $overviewModel")
            return
        }

        val estimateTime = overviewModel.estimateTime ?: 0.0
        val executeTime = overviewModel.executeTime ?: 0.0
        // 单位：秒
        val saveTime = MathUtil.roundToTwoDigits(((estimateTime - executeTime) * 3600)).toDouble()

        val turboDataReportDTO = TurboDataReportDTO(
            statisticsTime = statisticsDate,
            projectId = projectId,
            turboSaveTime = saveTime
        )

        try {
            client.get(ServiceMetricsDataReportResource::class.java)
                .metricsTurboDataReport(turboDataReportDTO)
            logger.info("Successfully uploaded metrics for project $turboDataReportDTO")
        } catch (e: Exception) {
            logger.error("Failed to upload metrics for project $projectId", e)
        }
    }
}
