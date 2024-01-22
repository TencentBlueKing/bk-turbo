package com.tencent.devops.turbo.aspect

import com.tencent.devops.common.api.exception.TurboException
import com.tencent.devops.common.api.exception.code.PERMISSION_DENIED
import org.aspectj.lang.JoinPoint
import org.aspectj.lang.annotation.Aspect
import org.aspectj.lang.annotation.Before
import org.aspectj.lang.reflect.MethodSignature
import org.slf4j.LoggerFactory
import org.springframework.stereotype.Component

@Aspect
@Component
class ApiAspect {

    companion object {
        private val logger = LoggerFactory.getLogger(this::class.java)
    }

    @Before("execution(* com.tencent.devops.turbo.controller.Apigw*.*(..))")
    fun beforeMethod(joinPoint: JoinPoint) {
        val methodName = joinPoint.signature.name
        logger.info("Pre enhancement the method: $methodName")

        val parameterNames = (joinPoint.signature as MethodSignature).parameterNames
        val parameterValues = joinPoint.args

        var appCode: String? = null
        for (index in parameterValues.indices) {
            when (parameterNames[index]) {
                "appCode" -> appCode = parameterValues[index].toString()
                else -> Unit
            }
        }

        logger.info("appCode[$appCode]")
        if (appCode.isNullOrBlank()) {
            throw TurboException(PERMISSION_DENIED, "user permission denied")
        }
    }
}
