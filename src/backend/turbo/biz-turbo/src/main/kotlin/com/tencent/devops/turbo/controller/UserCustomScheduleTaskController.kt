package com.tencent.devops.turbo.controller

import com.tencent.devops.api.pojo.Response
import com.tencent.devops.common.api.exception.UnauthorizedErrorException
import com.tencent.devops.turbo.api.IUserCustomScheduleTaskController
import com.tencent.devops.turbo.pojo.CustomScheduleJobModel
import com.tencent.devops.turbo.service.CustomScheduleJobService
import com.tencent.devops.turbo.service.TurboAuthService
import org.springframework.beans.factory.annotation.Autowired
import org.springframework.web.bind.annotation.RestController


@RestController
class UserCustomScheduleTaskController @Autowired constructor(
    private val turboAuthService: TurboAuthService,
    private val customScheduleJobService: CustomScheduleJobService
) : IUserCustomScheduleTaskController {

    override fun addScheduleJob(
        user: String,
        projectId: String,
        customScheduleJobModel: CustomScheduleJobModel
    ): Response<Boolean> {
        if (!turboAuthService.getAuthResult(projectId, user)) {
            throw UnauthorizedErrorException()
        }
        return Response.success(customScheduleJobService.customScheduledJobAdd(customScheduleJobModel))
    }

    override fun deleteScheduleJob(user: String, projectId: String, jobName: String): Response<Boolean> {
        if (!turboAuthService.getAuthResult(projectId, user)) {
            throw UnauthorizedErrorException()
        }
        return Response.success(customScheduleJobService.customScheduledJobDel(jobName))
    }

    override fun triggerCustomScheduleJob(user: String, projectId: String, jobName: String): Response<String> {
        if (!turboAuthService.getAuthResult(projectId, user)) {
            throw UnauthorizedErrorException()
        }
        return Response.success(customScheduleJobService.trigger(jobName))
    }
}
