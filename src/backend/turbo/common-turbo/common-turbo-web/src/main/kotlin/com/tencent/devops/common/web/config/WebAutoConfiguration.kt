package com.tencent.devops.common.web.config

import com.fasterxml.jackson.databind.module.SimpleModule
import com.fasterxml.jackson.datatype.jsr310.deser.LocalDateTimeDeserializer
import com.fasterxml.jackson.datatype.jsr310.ser.LocalDateTimeSerializer
import com.tencent.devops.common.security.jwt.JwtManager
import com.tencent.devops.common.web.filter.ServiceSecurityFilter
import com.tencent.devops.common.web.interceptor.LocaleInterceptor
import com.tencent.devops.common.web.jasypt.DefaultEncryptor
import org.springframework.beans.factory.annotation.Value
import org.springframework.boot.web.servlet.FilterRegistrationBean
import org.springframework.context.MessageSource
import org.springframework.context.annotation.Bean
import org.springframework.context.annotation.Configuration
import org.springframework.context.annotation.Primary
import org.springframework.context.support.ResourceBundleMessageSource
import org.springframework.http.converter.HttpMessageConverter
import org.springframework.http.converter.json.MappingJackson2HttpMessageConverter
import org.springframework.validation.Validator
import org.springframework.validation.beanvalidation.LocalValidatorFactoryBean
import org.springframework.web.servlet.config.annotation.InterceptorRegistry
import org.springframework.web.servlet.config.annotation.WebMvcConfigurer
import java.nio.charset.StandardCharsets
import java.time.LocalDateTime
import java.time.format.DateTimeFormatter
import javax.servlet.Filter


@Suppress("MaxLineLength")
@Configuration
class WebAutoConfiguration : WebMvcConfigurer {

    @Bean("jasyptStringEncryptor")
    @Primary
    fun stringEncryptor(@Value("\${enc.key:rAFOey00bcuMNMrt}") key: String) = DefaultEncryptor(key)

    @Bean("messageSource")
    fun messageSource(): MessageSource {
        val messageSource = ResourceBundleMessageSource()
        messageSource.setBasename("i18n/message")
        messageSource.setDefaultEncoding(StandardCharsets.UTF_8.name())
        // 缓存6小时
        messageSource.setCacheSeconds(3600 * 6)
        return messageSource
    }

    override fun configureMessageConverters(converters: MutableList<HttpMessageConverter<*>>) {
        converters.forEach {
            if (it is MappingJackson2HttpMessageConverter) {
                val simpleModule = SimpleModule()
                simpleModule.addSerializer(LocalDateTime::class.java, LocalDateTimeSerializer(DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm:ss")))
                simpleModule.addDeserializer(LocalDateTime::class.java, LocalDateTimeDeserializer(DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm:ss")))
                it.objectMapper.registerModule(simpleModule)
            }
        }
        super.configureMessageConverters(converters)
    }

    override fun addInterceptors(registry: InterceptorRegistry) {
        registry.addInterceptor(LocaleInterceptor())
    }

    /**
     * 配置Spring Boot Validation框架和Spring Boot自身使用同样的国际化配置
     */
    @Bean
    override fun getValidator(): Validator {
        val bean = LocalValidatorFactoryBean()
        // 仅兼容Spring Boot spring.messages和原hibernate-validator的国际化文件
        // 不支持resource/ValidationMessages.properties系列
        bean.setValidationMessageSource(this.messageSource())
        return bean
    }

    @Bean
    fun serviceSecurityFilter(jwtManager: JwtManager): Filter {
        return ServiceSecurityFilter(jwtManager)
    }

    @Bean
    fun serviceSecurityFilterRegistration(jwtManager: JwtManager): FilterRegistrationBean<Filter> {
        val registration: FilterRegistrationBean<Filter> = FilterRegistrationBean<Filter>()
        registration.filter = serviceSecurityFilter(jwtManager)
        registration.addUrlPatterns("/api/*")
        registration.order = 1
        return registration
    }
}
