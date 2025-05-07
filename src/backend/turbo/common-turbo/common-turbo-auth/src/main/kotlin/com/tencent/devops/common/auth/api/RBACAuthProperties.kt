package com.tencent.devops.common.auth.api

import org.springframework.beans.factory.annotation.Value
import org.springframework.stereotype.Component

@Component
class RBACAuthProperties {
    /**
     * RBAC权限系统资源类型
     */
    @Value("\${auth.rbac.resourceType:turbo_plan}")
    val rbacResourceType: String? = null

    /**
     * 接口access token
     */
    @Value("\${auth.rbac.token:#{null}}")
    val token: String? = null
}
