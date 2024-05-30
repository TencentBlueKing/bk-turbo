package com.tencent.devops.turbo.sdk

import com.fasterxml.jackson.core.type.TypeReference
import com.tencent.devops.api.pojo.Response
import com.tencent.devops.common.util.JsonUtil
import com.tencent.devops.common.api.util.OkhttpUtil
import com.tencent.devops.turbo.config.TodCostProperties
import com.tencent.devops.turbo.vo.ProjectResourceCostVO
import com.tencent.devops.turbo.vo.ResourceCostSummary
import com.tencent.devops.web.util.SpringContextHolder
import org.slf4j.LoggerFactory

object TodCostApi {

    private val logger = LoggerFactory.getLogger(TodCostApi::class.java)

    private const val UPLOAD_URL = "/api/v1/datasource/report-source-data/"

    /**
     * 上报数据
     *
     * @param isOverwrite 是否清空再写入
     */
    private fun postData(month: String, isOverwrite: Boolean, dataList: List<ProjectResourceCostVO>): Boolean {
        val properties = SpringContextHolder.getBean<TodCostProperties>()
        val body = ResourceCostSummary(
            dataSourceName = properties.dataSourceName,
            month = month,
            isOverwrite = isOverwrite,
            bills = dataList
        )
        val responseStr = OkhttpUtil.doHttpPost(
            url = properties.host + UPLOAD_URL,
            jsonBody = JsonUtil.toJson(body),
            headers = mapOf(
                "Content-Type" to "application/json",
                "Platform-Key" to properties.platformKey
            )
        )
        val action = if (isOverwrite) "clean" else "upload"
        logger.info("$action data for month: $month, size: ${dataList.size}, result: $responseStr")
        val resMap = JsonUtil.to(responseStr, object : TypeReference<Response<Map<String, String>>>() {})
        return resMap.code == 200
    }

    /**
     * 清空指定月份的数据
     */
    fun cleanDataByMonth(month: String): Boolean {
        return postData(month, true, emptyList())
    }

    /**
     * 上报数据（追加）
     */
    fun uploadByPage(month: String, dataList: List<ProjectResourceCostVO>): Boolean {
        return postData(month, false, dataList)
    }
}
