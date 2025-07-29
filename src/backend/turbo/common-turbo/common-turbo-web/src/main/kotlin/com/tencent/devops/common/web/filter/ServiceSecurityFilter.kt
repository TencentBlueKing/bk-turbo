package com.tencent.devops.common.web.filter

import com.tencent.devops.common.api.exception.UnauthorizedErrorException
import com.tencent.devops.common.security.jwt.JwtManager
import com.tencent.devops.common.util.constants.AUTH_HEADER_DEVOPS_JWT_TOKEN
import org.springframework.web.filter.OncePerRequestFilter
import java.net.InetAddress
import javax.servlet.FilterChain
import javax.servlet.http.HttpServletRequest
import javax.servlet.http.HttpServletResponse


class ServiceSecurityFilter(
    private val jwtManager: JwtManager
) : OncePerRequestFilter() {

    override fun doFilterInternal(
        request: HttpServletRequest,
        response: HttpServletResponse,
        filterChain: FilterChain
    ) {
        val uri = request.requestURI
        val clientIp = request.remoteAddr
        val jwt = request.getHeader(AUTH_HEADER_DEVOPS_JWT_TOKEN)
        val flag = shouldFilter(uri, clientIp)
        var error: UnauthorizedErrorException? = null
        if (flag && jwtManager.isSendEnable()) {
            error = check(jwt, clientIp, uri)
        }
        if (error != null && jwtManager.isAuthEnable()) {
            throw error
        }
    }

    private fun shouldFilter(uri: String, clientIp: String): Boolean {
        val localhost = kotlin.runCatching {
            InetAddress.getByName(clientIp).isLoopbackAddress
        }.getOrNull() ?: false
        // 不拦截本机请求
        if (localhost) {
            return false
        }

        // 不拦截的接口
        excludeVerifyPath.forEach {
            if (uri.startsWith(it)) {
                return false
            }
        }

        // 拦截api接口
        if (uri.startsWith("/api/")) {
            return true
        }

        // 默认不拦截
        return false
    }

    private fun check(
        jwt: String?,
        clientIp: String?,
        uri: String?
    ): UnauthorizedErrorException? {
        if (jwt.isNullOrBlank()) {
            logger.warn("Invalid request, jwt is empty!Client ip:$clientIp,uri:$uri")
            return UnauthorizedErrorException(
                errorCode = "401",
                errorMessage = "Unauthorized:turbo api jwt is empty!"
            )
        }
        val checkResult: Boolean = jwtManager.verifyJwt(jwt)
        if (!checkResult) {
            logger.warn("Invalid request, jwt is invalid or expired!Client ip:$clientIp,uri:$uri")
            return UnauthorizedErrorException(
                errorCode = "401",
                errorMessage = "Unauthorized:turbo api jwt is invalid or expired!"
            )
        }
        return null
    }

    companion object {
        private val excludeVerifyPath = listOf(
            "/api/swagger.json"
        )
    }
}
