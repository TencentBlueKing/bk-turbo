package com.tencent.devops.common.web.utils

import com.tencent.devops.common.service.utils.SpringContextUtil
import org.springframework.context.MessageSource
import org.springframework.context.NoSuchMessageException
import org.springframework.context.i18n.LocaleContextHolder
import java.util.Locale

object I18NUtil {

    const val ERROR: String = "i18n error"

    fun getMessage(code: String): String {
        return getMessage(code, null)
    }

    fun getMessage(code: String, args: Array<Any>?): String {
        return getMessage(code, args, LocaleContextHolder.getLocale())
    }

    fun getMessage(code: String, args: Array<Any>?, locale: Locale): String {
        val  messageSource = SpringContextUtil.getBean(MessageSource::class.java)
        return try {
            messageSource.getMessage(code, args, locale)
        } catch (_: NoSuchMessageException) {
            ERROR
        }
    }
}
