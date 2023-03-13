package com.tencent.devops.turbo.service

import com.tencent.devops.auth.api.service.ServiceManagerResource
import com.tencent.devops.auth.api.service.ServiceProjectAuthResource
import com.tencent.devops.common.client.Client
import org.slf4j.LoggerFactory
import org.springframework.beans.factory.annotation.Autowired
import org.springframework.beans.factory.annotation.Value
import org.springframework.stereotype.Service

@Suppress("SpringJavaInjectionPointsAutowiringInspection")
@Service
class TurboAuthService @Autowired constructor(
    private val client : Client
){

    companion object {
        private val logger = LoggerFactory.getLogger(TurboAuthService::class.java)
    }

    @Value("\${auth.token}")
    private val token : String? = null

    /**
     * 获取鉴权结果
     */
    fun getAuthResult(projectId: String, userId: String): Boolean {
        return validateProjectMember(projectId, userId) || validatePlatformMember(projectId, userId)
    }

    /**
     * 校验是否是项目成员
     */
    private fun validateProjectMember(projectId: String, userId: String) : Boolean {
        logger.info("project id: $projectId, user id: $userId, token : $token")
        val projectValidateResult = try {
            client.get(ServiceProjectAuthResource::class.java).isProjectUser(
                token = token!!,
                userId = userId,
                projectCode = projectId
            ).data?:false
        } catch (e : Exception) {
            e.printStackTrace()
            logger.info("validate project member fail! error message : ${e.message}")
            false
        }
        logger.info("project validate result: $projectValidateResult")
        return projectValidateResult
    }

    /**
     * 校验是否是平台管理员
     */
    fun validatePlatformMember(projectId : String, userId: String) : Boolean {
        val adminValidateResult =  try {
            client.get(ServiceManagerResource::class.java).validateManagerPermission(
                userId = userId,
                token = token!!,
                projectCode = projectId,
                resourceCode = "TURBO",
                action = "VIEW"
            ).data ?: false
        } catch (e : Exception) {
            logger.info("validate admin member fail! error message : ${e.message}")
            false
        }
        logger.info("admin validate result: $adminValidateResult")
        return adminValidateResult
    }

}
