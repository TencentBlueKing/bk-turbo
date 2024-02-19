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
 * Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated
 * documentation files (the "Software"), to deal in the Software without restriction, including without limitation
 * the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to
 * permit persons to whom the Software is furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all copies or substantial portions of the
 * Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT
 * LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN
 * NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY,
 * WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
 * SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 */

package com.tencent.devops.common.client.ms

import com.tencent.devops.common.api.exception.ClientException
import org.slf4j.LoggerFactory
import org.springframework.cloud.client.ServiceInstance
import org.springframework.cloud.client.discovery.DiscoveryClient
import org.springframework.cloud.consul.discovery.ConsulServiceInstance

open class ConsulServiceTarget<T> constructor(
        override val serviceName: String,
        override val type: Class<T>,
        private val discoveryClient: DiscoveryClient,
        private val tag: String?,
        override val commonUrlPrefix : String = "/api",
) : FeignTarget<T>(
    serviceName,
    type,
    commonUrlPrefix
) {

    companion object{
        private val logger = LoggerFactory.getLogger(ConsulServiceTarget::class.java)
    }

    override fun choose(serviceName: String): ServiceInstance {
        val serviceInstanceList: List<ServiceInstance> = usedInstance.getIfPresent(serviceName) ?: emptyList()

        if (serviceInstanceList.isEmpty()) {
            val currentInstanceList = discoveryClient.getInstances(serviceName)
            if (currentInstanceList.isEmpty()) {
                logger.error("Unable to find any valid [$serviceName] service provider")
                throw ClientException(
                    "Unable to find any valid [$serviceName] service provider"
                )
            }
            currentInstanceList.forEach { serviceInstance ->
                if (serviceInstance is ConsulServiceInstance && serviceInstance.tags.contains(tag)) {
                    serviceInstanceList.toMutableList().add(serviceInstance)
                }
            }

            if (serviceInstanceList.isEmpty()) {
                logger.error("Unable to find any valid [$serviceName] service provider")
                throw ClientException(
                    "Unable to find any valid [$serviceName] service provider"
                )
            } else {
                usedInstance.put(serviceName, serviceInstanceList)
                serviceInstanceList.toMutableList().shuffle()
            }
        }

        return serviceInstanceList[0]
    }

    override fun url(): String {
        return choose(serviceName).url()
    }
}
