package com.tencent.devops.turbo.controller

import com.tencent.devops.api.pojo.Response
import com.tencent.devops.common.api.annotation.RequiresAuth
import com.tencent.devops.common.api.exception.TurboException
import com.tencent.devops.common.api.exception.code.IS_NOT_ADMIN_MEMBER
import com.tencent.devops.common.api.pojo.Page
import com.tencent.devops.common.util.constants.NO_ADMIN_MEMBER_MESSAGE
import com.tencent.devops.common.util.enums.ResourceActionType
import com.tencent.devops.common.util.enums.ResourceType
import com.tencent.devops.turbo.api.IUserTurboPlanController
import com.tencent.devops.turbo.pojo.TurboPlanModel
import com.tencent.devops.turbo.service.TurboAuthService
import com.tencent.devops.turbo.service.TurboPlanService
import com.tencent.devops.turbo.vo.TurboMigratedPlanVO
import com.tencent.devops.turbo.vo.TurboPlanDetailVO
import com.tencent.devops.turbo.vo.TurboPlanPageVO
import com.tencent.devops.turbo.vo.TurboPlanStatusBatchUpdateReqVO
import org.springframework.beans.factory.annotation.Autowired
import org.springframework.web.bind.annotation.RestController

@RestController
class UserTurboPlanController @Autowired constructor(
    private val turboPlanService: TurboPlanService,
    private val turboAuthService: TurboAuthService
) : IUserTurboPlanController {

    @RequiresAuth(resourceType = ResourceType.PROJECT, permission = ResourceActionType.CREATE)
    override fun addNewTurboPlan(turboPlanModel: TurboPlanModel, projectId: String, user: String): Response<String?> {
        return Response.success(turboPlanService.addNewTurboPlan(turboPlanModel, user))
    }

    @RequiresAuth(resourceType = ResourceType.PROJECT, permission = ResourceActionType.LIST)
    override fun getTurboPlanStatRowData(
        projectId: String,
        pageNum: Int?,
        pageSize: Int?,
        user: String
    ): Response<TurboPlanPageVO> {
        return Response.success(turboPlanService.getTurboPlanStatRowData(projectId, pageNum, pageSize))
    }

    @RequiresAuth
    override fun getTurboPlanDetailByPlanId(
        planId: String,
        projectId: String,
        user: String
    ): Response<TurboPlanDetailVO> {
        return Response.success(turboPlanService.getTurboPlanDetailByPlanId(planId))
    }

    @RequiresAuth(permission = ResourceActionType.EDIT)
    override fun putTurboPlanDetailNameAndOpenStatus(
        turboPlanModel: TurboPlanModel,
        planId: String,
        user: String,
        projectId: String
    ): Response<Boolean> {
        return Response.success(turboPlanService.putTurboPlanDetailNameAndOpenStatus(turboPlanModel, planId, user))
    }

    @RequiresAuth(permission = ResourceActionType.EDIT)
    override fun putTurboPlanConfigParam(
        turboPlanModel: TurboPlanModel,
        planId: String,
        user: String,
        projectId: String
    ): Response<Boolean> {
        return Response.success(turboPlanService.putTurboPlanConfigParam(turboPlanModel, planId, user))
    }

    @RequiresAuth(permission = ResourceActionType.EDIT)
    override fun putTurboPlanTopStatus(planId: String, topStatus: String, user: String): Response<Boolean> {
        return Response.success(turboPlanService.putTurboPlanTopStatus(planId, topStatus, user))
    }

    @RequiresAuth(resourceType = ResourceType.PROJECT, permission = ResourceActionType.LIST)
    override fun getAvailableTurboPlanList(
        projectId: String,
        pageNum: Int?,
        pageSize: Int?
    ): Response<Page<TurboPlanDetailVO>> {
        return Response.success(turboPlanService.getAvailableProjectIdList(projectId, pageNum, pageSize))
    }

    override fun findTurboPlanIdByProjectIdAndPipelineInfo(
        projectId: String,
        pipelineId: String,
        pipelineElementId: String
    ): Response<TurboMigratedPlanVO?> {
        return Response.success(
            turboPlanService.findMigratedTurboPlanByPipelineInfo(
                projectId,
                pipelineId,
                pipelineElementId
            )
        )
    }

    override fun manualRefreshStatus(
        reqVO: TurboPlanStatusBatchUpdateReqVO,
        user: String,
        projectId: String
    ): Response<String> {
        // 判断是否是管理员
        if (!turboAuthService.getAuthResult(projectId, user)) {
            throw TurboException(errorCode = IS_NOT_ADMIN_MEMBER, errorMessage = NO_ADMIN_MEMBER_MESSAGE)
        }
        return Response.success(turboPlanService.manualRefreshStatus(reqVO))
    }
}
