package com.tencent.devops.common.web.interceptor

import com.tencent.devops.common.web.utils.CookieUtil
import org.springframework.context.i18n.LocaleContextHolder
import org.springframework.stereotype.Component
import org.springframework.web.servlet.HandlerInterceptor
import java.util.Locale
import javax.servlet.http.HttpServletRequest
import javax.servlet.http.HttpServletResponse

@Component
class LocaleInterceptor : HandlerInterceptor {

    companion object {
        private const val COOKIE_LOCALE_KEY = "blueking_language"
        private const val HEADER_LOCALE_KEY = "Accept-Language"
    }

    override fun preHandle(request: HttpServletRequest, response: HttpServletResponse, handler: Any): Boolean {
        val cookieLocale = CookieUtil.getCookieValue(request, COOKIE_LOCALE_KEY)
        val acceptLanguage = request.getHeader(HEADER_LOCALE_KEY)
        val parameterLocale = request.getParameter("locale")

        val locale = parseLocale(cookieLocale, acceptLanguage, parameterLocale)
        LocaleContextHolder.setLocale(locale)

        return true
    }

    private fun parseLocale(cookieLocale: String?, acceptLanguage: String?, paramLocale: String?): Locale {
        // 优先cookie
        if (!cookieLocale.isNullOrBlank()) {
            return Locale.forLanguageTag(cookieLocale)
        }
        // 请求头accept-language
        if (!acceptLanguage.isNullOrBlank()) {
            val languageTag = acceptLanguage.split(",")[0].trim()
            return Locale.forLanguageTag(languageTag)
        }

        if (!paramLocale.isNullOrBlank()) {
            return Locale.forLanguageTag(paramLocale)
        }

        return Locale.getDefault()
    }
}
