package com.tencent.devops.common.auth.api.external

import com.tencent.devops.auth.api.service.ServicePermissionAuthResource
import com.tencent.devops.auth.api.service.ServiceProjectAuthResource
import com.tencent.devops.common.api.exception.TurboException
import com.tencent.devops.common.api.exception.code.TURBO_THIRDPARTY_SYSTEM_FAIL
import com.tencent.devops.common.auth.api.AuthPermissionApi
import com.tencent.devops.common.auth.api.RBACAuthProperties
import com.tencent.devops.common.client.Client
import com.tencent.devops.common.util.enums.ResourceType
import org.slf4j.LoggerFactory
import org.springframework.beans.factory.annotation.Autowired

class RBACAuthPermissionApi @Autowired constructor(
    private val client: Client,
    private val rbacAuthProperties: RBACAuthProperties
) : AuthPermissionApi {

    companion object {
        private val logger = LoggerFactory.getLogger(this::class.java)
    }

    /**
     * 查询非用户态Access Token
     */
    private fun getBackendAccessToken(): String {
        return rbacAuthProperties.token ?: ""
    }

    /**
     * 校验是否项目管理员
     */
    override fun authProjectManager(projectId: String, userId: String): Boolean {
        return checkProjectManager(projectId, userId)
    }


    /**
     * 校验用户是否有项目维度的资源操作权限
     */
    override fun validateUserProjectPermission(
        projectId: String,
        userId: String,
        action: String
    ): Boolean {
        logger.info("validateUserProject: $projectId, userId: $userId, action: $action")
        return validateUserResourcePermission(
            userId,
            getBackendAccessToken(),
            action,
            projectId,
            ResourceType.PROJECT.name
        )
    }

    /**
     * 校验用户是否有具体资源实例的操作权限
     * @param resourceCode 资源实例id
     */
    override fun validateUserResourcePermission(
        projectId: String,
        resourceCode: String,
        action: String,
        userId: String
    ): Boolean {
        logger.info("validateUserResource: $projectId, userId: $userId, action: $action, resource: $resourceCode")
        return validateUserPermission(
            projectId = projectId,
            resourceCode = resourceCode,
            action = action,
            userId = userId
        )
    }

    override fun authProjectUser(projectId: String, userId: String): Boolean {
        val result = client.getDevopsService(ServiceProjectAuthResource::class.java, projectId)
            .isProjectUser(
                userId = userId,
                token = getBackendAccessToken(),
                projectCode = projectId
            )
        if (result.isNotOk()) {
            throw TurboException(TURBO_THIRDPARTY_SYSTEM_FAIL, "getDevopsService failed: ${result.message}")
        }

        return result.data ?: false
    }


    /**
     * 校验用户是否有具体资源的操作权限
     * @param resourceCode 此处resourceCode实际为resourceType
     */
    private fun validateUserResourcePermission(
        userId: String,
        token: String,
        action: String,
        projectCode: String,
        resourceCode: String
    ): Boolean {
        val result = client.getDevopsService(ServicePermissionAuthResource::class.java, projectCode)
                .validateUserResourcePermission(userId, token, null, action, projectCode, resourceCode)
        if (result.isNotOk()) {
            throw TurboException(TURBO_THIRDPARTY_SYSTEM_FAIL, "getDevopsService failed: ${result.message}")
        }
        return result.data ?: false
    }

    /**
     * 校验用户是否是项目管理员
     */
    private fun checkProjectManager(
        projectCode: String,
        userId: String
    ): Boolean {
        val response = client.getDevopsService(ServiceProjectAuthResource::class.java, projectCode).checkProjectManager(
            token = getBackendAccessToken(),
            userId = userId,
            projectCode = projectCode
        )
        if (response.isNotOk()) {
            throw TurboException(TURBO_THIRDPARTY_SYSTEM_FAIL, "getDevopsService failed!")
        }
        return response.data ?: false
    }

    /**
     * 校验用户是否有具体资源实例的操作权限
     */
    private fun validateUserPermission(
        projectId: String,
        resourceCode: String,
        action: String,
        userId: String
    ): Boolean {
        val result = client.getDevopsService(ServicePermissionAuthResource::class.java)
            .validateUserResourcePermissionByRelation(
            userId = userId,
            token = getBackendAccessToken(),
            action = action,
            projectCode = projectId,
            resourceCode = resourceCode,
            resourceType = rbacAuthProperties.rbacResourceType!!
        )

        if (result.isNotOk()) {
            throw TurboException(TURBO_THIRDPARTY_SYSTEM_FAIL, "getDevopsService failed: ${result.message}")
        }

        return result.data ?: false
    }
}
