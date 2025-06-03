package com.tencent.devops.common.web.interceptor

import com.tencent.devops.common.api.annotation.RequiresAuth
import com.tencent.devops.common.api.exception.TurboException
import com.tencent.devops.common.api.exception.code.TURBO_PARAM_INVALID
import com.tencent.devops.common.auth.api.AuthPermissionApi
import com.tencent.devops.common.util.constants.AUTH_HEADER_DEVOPS_PLAN_ID
import com.tencent.devops.common.util.constants.AUTH_HEADER_DEVOPS_PROJECT_ID
import com.tencent.devops.common.util.constants.AUTH_HEADER_DEVOPS_USER_ID
import com.tencent.devops.common.util.enums.ResourceType
import com.tencent.devops.web.util.SpringContextHolder
import org.slf4j.Logger
import org.slf4j.LoggerFactory
import org.springframework.web.method.HandlerMethod
import org.springframework.web.servlet.HandlerInterceptor
import javax.servlet.http.HttpServletRequest
import javax.servlet.http.HttpServletResponse

/**
 * 鉴权拦截器
 */
class AuthInterceptor : HandlerInterceptor{

    companion object {
        val logger: Logger = LoggerFactory.getLogger(this::class.java)
    }

    override fun preHandle(request: HttpServletRequest, response: HttpServletResponse, handler: Any): Boolean {
        logger.info("handle uri: ${request.requestURI}")
        val method = (handler as HandlerMethod).method

        // 如果是没加`@RequiresAuth`注解的请求则放行
        val requiresAuth = method.getAnnotation(RequiresAuth::class.java) ?: return true
        val authPermissionApi = SpringContextHolder.getBean(AuthPermissionApi::class.java)

        val resourceType: ResourceType = requiresAuth.resourceType
        val actionId: String = requiresAuth.permission.actionId

        val userId = request.getHeader(AUTH_HEADER_DEVOPS_USER_ID)
        val projectId = request.getHeader(AUTH_HEADER_DEVOPS_PROJECT_ID)
        logger.info("auth resource: ${resourceType.id}, user: $userId, action: $actionId")

        val result = when (resourceType) {
            ResourceType.PROJECT -> {
                authPermissionApi.validateUserProjectPermission(projectId, userId, actionId)
            }
            ResourceType.TURBO_PLAN -> {
                val turboPlanId =
                    getPlanIdFromRequest(request) ?: return authPermissionApi.authProjectUser(projectId, userId)
                authPermissionApi.validateUserResourcePermission(projectId, turboPlanId, actionId, userId)
            }
            else -> {
                throw TurboException(TURBO_PARAM_INVALID, "Unrecognized resource type: ${resourceType.id}")
            }
        }

        if (!result) {
            response.status = HttpServletResponse.SC_FORBIDDEN
        }

        return false
    }

    /**
     *
     */
    private fun getPlanIdFromRequest(request: HttpServletRequest): String? {
        return request.getAttribute("planId") as? String ?: request.getHeader(AUTH_HEADER_DEVOPS_PLAN_ID)
    }
}
