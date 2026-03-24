package com.tencent.devops.turbo.controller

import com.tencent.devops.api.pojo.Response
import com.tencent.devops.common.api.exception.TurboException
import com.tencent.devops.common.api.exception.code.IS_NOT_ADMIN_MEMBER
import com.tencent.devops.common.util.constants.NO_ADMIN_MEMBER_MESSAGE
import com.tencent.devops.turbo.api.IUserResourceStatController
import com.tencent.devops.turbo.component.BkMetricsStatUpload
import com.tencent.devops.turbo.service.ProjectResourcesService
import com.tencent.devops.turbo.service.TurboAuthService
import com.tencent.devops.turbo.vo.ResourceCostSummary
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import org.springframework.beans.factory.annotation.Autowired
import org.springframework.web.bind.annotation.RestController

@RestController
class UserResourceStatController @Autowired constructor(
    private val projectResourcesService: ProjectResourcesService,
    private val bkMetricsStatUpload: BkMetricsStatUpload,
    private val turboAuthService: TurboAuthService
) : IUserResourceStatController {

    override fun triggerAutoUpload(
        userId: String,
        projectId: String,
        month: String,
        startDate: String?,
        endDate: String?
    ): Response<Boolean> {
        // 判断是否是管理员
        if (!turboAuthService.getAuthResult(projectId, userId)) {
            throw TurboException(errorCode = IS_NOT_ADMIN_MEMBER, errorMessage = NO_ADMIN_MEMBER_MESSAGE)
        }
        return Response.success(projectResourcesService.triggerStatAndUpload(month, startDate, endDate))
    }

    override fun triggerManualUpload(
        userId: String,
        projectId: String,
        summary: ResourceCostSummary
    ): Response<Boolean> {
        // 判断是否是管理员
        if (!turboAuthService.getAuthResult(projectId, userId)) {
            throw TurboException(errorCode = IS_NOT_ADMIN_MEMBER, errorMessage = NO_ADMIN_MEMBER_MESSAGE)
        }
        return Response.success(projectResourcesService.manualUploadCostData(summary))
    }

    override fun triggerUploadMetrics(
        userId: String,
        projectId: String,
        statisticsDate: String?,
        pageSizeParam: Int?
    ): Response<Boolean> {
        // 判断是否是管理员
        if (!turboAuthService.getAuthResult(projectId, userId)) {
            throw TurboException(errorCode = IS_NOT_ADMIN_MEMBER, errorMessage = NO_ADMIN_MEMBER_MESSAGE)
        }

        runBlocking {
            launch {
                try {
                    bkMetricsStatUpload.entry(statisticsDate, pageSizeParam)
                } catch (e: Exception) {
                    throw TurboException(errorMessage = "执行统计上报任务失败：${e.message}")
                }
            }
        }
        return Response.success(true)
    }
}
