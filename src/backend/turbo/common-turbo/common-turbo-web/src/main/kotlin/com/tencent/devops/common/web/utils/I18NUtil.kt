package com.tencent.devops.common.web.utils

import com.tencent.devops.common.service.utils.SpringContextUtil
import org.springframework.context.MessageSource
import org.springframework.context.i18n.LocaleContextHolder
import java.util.Locale

object I18NUtil {

    fun getMessage(code: String): String {
        return getMessage(code, null)
    }

    fun getMessage(code: String, args: Array<Any>?): String {
        return getMessage(code, args, LocaleContextHolder.getLocale())
    }

    fun getMessage(code: String, args: Array<Any>?, locale: Locale): String {
        val  messageSource = SpringContextUtil.getBean(MessageSource::class.java)
        return messageSource.getMessage(code, args, locale)
    }
}
