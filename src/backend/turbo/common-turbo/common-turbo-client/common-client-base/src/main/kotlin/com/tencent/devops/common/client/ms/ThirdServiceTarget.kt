package com.tencent.devops.common.client.ms

import feign.Request
import feign.RequestTemplate
import feign.Target

open class ThirdServiceTarget<T> constructor(
    private val serviceName: String,
    private val type: Class<T>,
    private val rootPath: String
) : Target<T> {

    override fun apply(input: RequestTemplate?): Request {
        if (input!!.url().indexOf("http") != 0) {
            input.insert(0, url())
        }
        return input.request()
    }

    override fun url(): String {
        return rootPath
    }

    override fun type() = type

    override fun name() = serviceName
}