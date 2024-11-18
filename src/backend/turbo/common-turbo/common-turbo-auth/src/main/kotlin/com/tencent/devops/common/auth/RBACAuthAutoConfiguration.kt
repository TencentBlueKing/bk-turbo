package com.tencent.devops.common.auth

import com.tencent.devops.common.auth.api.RBACAuthProperties
import com.tencent.devops.common.auth.api.external.RBACAuthPermissionApi
import com.tencent.devops.common.auth.api.RBACAuthRegisterApi
import org.springframework.context.annotation.Bean
import org.springframework.context.annotation.Configuration
import com.tencent.devops.common.client.Client

@Configuration
class RBACAuthAutoConfiguration {

    @Bean
    fun rbacAuthProperties() = RBACAuthProperties()

    @Bean
    fun rbacAuthPermissionApi(
        rbacAuthProperties: RBACAuthProperties,
        client: Client
    ) = RBACAuthPermissionApi(client, rbacAuthProperties)

    @Bean
    fun rbacAuthRegisterApi(
        rbacAuthProperties: RBACAuthProperties,
        client: Client
    ) = RBACAuthRegisterApi(client, rbacAuthProperties)
}
