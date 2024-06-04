package com.tencent.devops.turbo.controller

import com.tencent.devops.api.pojo.Response
import com.tencent.devops.common.api.exception.TurboException
import com.tencent.devops.common.api.exception.code.IS_NOT_ADMIN_MEMBER
import com.tencent.devops.common.api.pojo.Page
import com.tencent.devops.common.util.constants.NO_ADMIN_MEMBER_MESSAGE
import com.tencent.devops.turbo.api.IServiceResourceStatController
import com.tencent.devops.turbo.service.ProjectResourcesService
import com.tencent.devops.turbo.service.TurboAuthService
import com.tencent.devops.turbo.vo.ProjectResourceUsageVO
import com.tencent.devops.turbo.vo.ResourceCostSummary
import org.springframework.beans.factory.annotation.Autowired
import org.springframework.web.bind.annotation.RestController

@RestController
class ServiceResourceStatController @Autowired constructor(
    private val projectResourcesService: ProjectResourcesService,
    private val turboAuthService: TurboAuthService
) : IServiceResourceStatController {

    override fun getSummary(
        startDate: String?,
        endDate: String?,
        pageNum: Int?,
        pageSize: Int?
    ): Response<Page<ProjectResourceUsageVO>> {
        return Response.success(projectResourcesService.querySummary(startDate, endDate, pageNum, pageSize))
    }

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
}
