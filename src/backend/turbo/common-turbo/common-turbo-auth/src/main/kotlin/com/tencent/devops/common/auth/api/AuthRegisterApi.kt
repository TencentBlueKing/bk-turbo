package com.tencent.devops.common.auth.api

interface AuthRegisterApi {

    /**
     * 注册加速方案
     */
    fun registerTurboPlan(
        user: String,
        turboPlanId: String,
        turboPlanName: String,
        projectId: String
    ): Boolean

    /**
     * 删除加速方案
     */
    fun deleteTurboPlan(
        turboPlanId: String,
        projectId: String
    ): Boolean
}
