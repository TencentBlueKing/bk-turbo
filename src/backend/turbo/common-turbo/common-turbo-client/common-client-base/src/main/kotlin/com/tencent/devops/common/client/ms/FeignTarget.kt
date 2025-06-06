/*
 * Tencent is pleased to support the open source community by making BK-CODECC 蓝鲸代码检查平台 available.
 *
 * Copyright (C) 2019 THL A29 Limited, a Tencent company.  All rights reserved.
 *
 * BK-CODECC 蓝鲸代码检查平台 is licensed under the MIT license.
 *
 * A copy of the MIT License is included in this file.
 *
 *
 * Terms of the MIT License:
 * ---------------------------------------------------
 * Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation
 * files (the "Software"), to deal in the Software without restriction, including without limitation the rights to use, copy,
 * modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the
 * Software is furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT
 * LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN
 * NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY,
 * WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
 * SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 */

package com.tencent.devops.common.client.ms

import com.google.common.cache.Cache
import com.google.common.cache.CacheBuilder
import feign.Request
import feign.RequestTemplate
import feign.Target
import org.springframework.cloud.client.ServiceInstance
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit

abstract class FeignTarget<T>(
    protected open val serviceName: String,
    protected open val type: Class<T>,
    protected open val commonUrlPrefix: String,
    // key: serviceName, value: List<ServiceInstance>
    protected val usedInstance: Cache<String, List<ServiceInstance>> = CacheBuilder.newBuilder()
        .maximumSize(1000)
        .expireAfterWrite(3, TimeUnit.SECONDS)
        .build<String, List<ServiceInstance>>()
) : Target<T> {

    override fun apply(input: RequestTemplate?): Request {
        if (input!!.url().indexOf("http") != 0) {
            input.target(url())
        }
        return input.request()
    }

    override fun type() = type

    override fun name() = serviceName
    protected fun ServiceInstance.url(): String {
        val finalHost = if (host.isNotBlank() && host.contains(":") && !host.startsWith("[")) {
            "[$host]"
        } else {
            host
        }
        return "${if (isSecure) "https" else "http"}://$finalHost:$port$commonUrlPrefix"
    }
    protected abstract fun choose(serviceName: String): ServiceInstance

    override fun url(): String {
        return choose(serviceName).url()
    }
}
