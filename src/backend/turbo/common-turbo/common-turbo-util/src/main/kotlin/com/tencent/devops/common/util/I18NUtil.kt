package com.tencent.devops.common.util

import org.springframework.beans.factory.annotation.Autowired
import org.springframework.context.MessageSource
import org.springframework.context.i18n.LocaleContextHolder
import org.springframework.stereotype.Component
import java.util.Locale

@Component
object I18NUtil {

    @JvmStatic
    lateinit var messageSource: MessageSource
    @Autowired set


    @JvmStatic
    fun getMessage(code: String): String {
        return getMessage(code, null)
    }

    @JvmStatic
    fun getMessage(code: String, args: Array<Any>?): String {
        return getMessage(code, args, LocaleContextHolder.getLocale())
    }

    @JvmStatic
    fun getMessage(code: String, args: Array<Any>?, locale: Locale): String {
        return messageSource.getMessage(code, args, locale)
    }
}
