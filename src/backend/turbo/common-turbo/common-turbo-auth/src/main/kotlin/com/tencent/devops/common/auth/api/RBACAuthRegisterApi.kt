package com.tencent.devops.common.auth.api

import com.tencent.devops.auth.api.service.ServicePermissionAuthResource
import com.tencent.devops.common.api.exception.RemoteServiceException
import com.tencent.devops.common.api.exception.TurboException
import com.tencent.devops.common.api.exception.code.PERMISSION_DENIED
import com.tencent.devops.common.api.pojo.Result
import com.tencent.devops.common.client.Client
import org.slf4j.LoggerFactory
import org.springframework.beans.factory.annotation.Autowired

class RBACAuthRegisterApi @Autowired constructor(
    private val client: Client,
    private val rbacAuthProperties: RBACAuthProperties
) : AuthRegisterApi {

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
     * 注册加速方案
     */
    override fun registerTurboPlan(
        user: String,
        turboPlanId: String,
        turboPlanName: String,
        projectId: String
    ): Boolean {
        val result = registerResource(
            projectId = projectId,
            resourceCode = turboPlanId,
            resourceName = turboPlanName,
            creator = user
        )
        if (result.isNotOk() || result.data == null || result.data == false) {
            logger.error(
                "register resource failed! taskId: $turboPlanId, return code:${result.status}," +
                        " err message: ${result.message}"
            )
            throw TurboException(PERMISSION_DENIED, user)
        }
        return true
    }

    /**
     * 删除加速方案
     */
    override fun deleteTurboPlan(
        turboPlanId: String,
        projectId: String
    ): Boolean {
        val result = deleteResource(
            projectId = projectId,
            resourceCode = turboPlanId
        )
        if (result.isNotOk() || result.data == null || result.data == false) {
            logger.error(
                "delete resource failed! taskId: $turboPlanId, return code:${result.status}," +
                        " err message: ${result.message}"
            )
            throw RemoteServiceException(errorMessage = "delete resource failed!")
        }
        return true
    }

    /**
     * 调用api注册资源
     */
    private fun registerResource(
        projectId: String,
        resourceCode: String,
        resourceName: String,
        creator: String
    ): Result<Boolean> {
        return client.getDevopsService(ServicePermissionAuthResource::class.java, projectId).resourceCreateRelation(
            userId = creator,
            token = getBackendAccessToken(),
            projectCode = projectId,
            resourceType = rbacAuthProperties.rbacResourceType!!,
            resourceCode = resourceCode,
            resourceName = resourceName
        )
    }

    /**
     * 调用api删除资源
     */
    private fun deleteResource(
        projectId: String,
        resourceCode: String
    ): Result<Boolean> {
        return client.getDevopsService(ServicePermissionAuthResource::class.java, projectId).resourceDeleteRelation(
            token = getBackendAccessToken(),
            projectCode = projectId,
            resourceType = rbacAuthProperties.rbacResourceType!!,
            resourceCode = resourceCode
        )
    }
}
