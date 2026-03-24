package com.tencent.devops.common.security


import com.tencent.devops.common.security.jwt.JwtManager
import com.tencent.devops.common.security.utils.EnvironmentUtil
import org.springframework.beans.factory.annotation.Value
import org.springframework.boot.autoconfigure.AutoConfigureOrder
import org.springframework.context.annotation.Bean
import org.springframework.context.annotation.Configuration
import org.springframework.context.annotation.DependsOn
import org.springframework.context.annotation.PropertySource
import org.springframework.core.Ordered
import org.springframework.core.env.Environment
import org.springframework.scheduling.annotation.EnableScheduling

@EnableScheduling
@Configuration
@AutoConfigureOrder(Ordered.LOWEST_PRECEDENCE)
class ServiceAutoConfiguration {

    @Value("\${bkci.security.public-key:#{null}}")
    private val publicKey: String? = null

    @Value("\${bkci.security.private-key:#{null}}")
    private val privateKey: String? = null

    @Value("\${bkci.security.enable:#{false}}")
    private val enable: Boolean = false

    @Bean
    fun environmentUtil() = EnvironmentUtil()

    @Bean
    @DependsOn("environmentUtil")
    fun jwtManager() = JwtManager(privateKey, publicKey, enable)
}
