package com.tencent.devops.turbo.config

import org.springframework.boot.context.properties.ConfigurationProperties
import org.springframework.stereotype.Component

@Component
@ConfigurationProperties(prefix = "domains")
data class DomainsProperties(
    /**
     * 后台脚本下载域名
     */
    var devgw: String = "null",

    /**
     * wiki域名
     */
    var iwiki: String = "null",
)
