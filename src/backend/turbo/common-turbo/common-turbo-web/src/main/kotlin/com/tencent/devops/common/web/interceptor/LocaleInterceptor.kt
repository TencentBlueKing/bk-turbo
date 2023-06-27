package com.tencent.devops.common.web.interceptor

import org.springframework.context.i18n.LocaleContextHolder
import org.springframework.stereotype.Component
import org.springframework.web.servlet.HandlerInterceptor
import java.util.Locale
import javax.servlet.http.HttpServletRequest
import javax.servlet.http.HttpServletResponse

@Component
class LocaleInterceptor : HandlerInterceptor {

    override fun preHandle(request: HttpServletRequest, response: HttpServletResponse, handler: Any): Boolean {
        val acceptLanguage = request.getHeader("Accept-Language")
        val parameterLocale = request.getParameter("locale")
        val locale = parseLocale(acceptLanguage, parameterLocale)
        LocaleContextHolder.setLocale(locale)

        return true
    }

    private fun parseLocale(acceptLanguage: String?, paramLocale: String?): Locale {
        if (!acceptLanguage.isNullOrBlank()) {
            val languageTag = acceptLanguage.split(",")[0].trim()
            return Locale.forLanguageTag(languageTag)
        }

        if (!paramLocale.isNullOrBlank()) {
            return genLocale(paramLocale)
        }

        return Locale.getDefault()
    }

    private fun genLocale(requestParameter: String): Locale {
        val splitArr = if (requestParameter.contains("-")) {
            requestParameter.split("-")
        } else {
            requestParameter.split("_")
        }
        return Locale(splitArr[0], splitArr[1])
    }
}
