package com.tencent.devops.turbo.controller

import com.tencent.devops.api.pojo.Response
import com.tencent.devops.turbo.api.IServiceTurboPlanController
import com.tencent.devops.turbo.enums.ProjectEventType
import com.tencent.devops.turbo.pojo.ProjectCallbackEvent
import com.tencent.devops.turbo.service.TurboPlanService
import org.slf4j.LoggerFactory
import org.springframework.beans.factory.annotation.Autowired
import org.springframework.web.bind.annotation.RestController

@RestController
class ServiceTurboPlanController @Autowired constructor(
    private val turboPlanService: TurboPlanService
) : IServiceTurboPlanController {

    companion object {
        private val logger = LoggerFactory.getLogger(this::class.java)
    }

    override fun findTurboPlanIdByProjectIdAndPipelineInfo(
        projectId: String,
        pipelineId: String,
        pipelineElementId: String
    ): Response<String?> {
        return Response.success(
            turboPlanService.findMigratedTurboPlanByPipelineInfo(
                projectId,
                pipelineId,
                pipelineElementId
            )?.taskId
        )
    }

    override fun updatePlanStatusByProjectStatus(projectCallbackEvent: ProjectCallbackEvent): Response<Boolean> {
        try {
            // 1. 参数校验
            requireNotNull(projectCallbackEvent.event) { "事件类型不能为空" }
            requireNotNull(projectCallbackEvent.data) { "事件数据不能为空" }

            logger.info("开始处理项目状态更新事件: $projectCallbackEvent")

            // 2. 事件处理
            return when (projectCallbackEvent.event) {
                ProjectEventType.PROJECT_ENABLE, ProjectEventType.PROJECT_DISABLE -> {
                    with(projectCallbackEvent.data) {
                        require(userId.isNotBlank()) { "用户ID不能为空" }
                        require(projectId.isNotBlank()) { "项目ID不能为空" }

                        turboPlanService.updatePlanStatusByBkProjectStatus(
                            userId = userId,
                            projectId = projectId,
                            enabled = enabled
                        )

                        logger.info("项目状态更新完成: userId=$userId, projectId=$projectId, enabled=$enabled")
                        Response.success(true)
                    }
                }
                else -> {
                    logger.warn("不支持的事件类型: ${projectCallbackEvent.event}")
                    Response.success(false)
                }
            }
        } catch (e: IllegalArgumentException) {
            logger.error("参数校验失败: ${e.message}", e)
            return Response.success(false)
        } catch (e: Exception) {
            logger.error("处理项目状态更新异常", e)
            return Response.success(false)
        }
    }
}
