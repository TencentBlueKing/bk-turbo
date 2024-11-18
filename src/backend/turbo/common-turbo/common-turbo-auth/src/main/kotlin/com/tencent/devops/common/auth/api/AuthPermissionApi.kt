package com.tencent.devops.common.auth.api

interface AuthPermissionApi {

    /**
     * 校验用户是否有项目维度的资源操作权限
     */
    fun validateUserProjectPermission(
        projectId: String,
        userId: String,
        action: String
    ): Boolean

    /**
     * 校验用户是否有指定实例资源的操作权限
     */
    fun validateUserResourcePermission(
        projectId: String,
        resourceCode: String,
        action: String,
        userId: String
    ): Boolean

    /**
     * 校验是否项目管理员
     */
    fun authProjectManager(projectId: String, userId: String): Boolean

    /**
     * 校验是否项目管理员
     */
    fun authProjectUser(projectId: String, userId: String): Boolean
}
