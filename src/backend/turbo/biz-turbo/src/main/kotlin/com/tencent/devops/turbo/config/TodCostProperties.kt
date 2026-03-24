package com.tencent.devops.turbo.config

import org.springframework.boot.context.properties.ConfigurationProperties
import org.springframework.stereotype.Component

@Component
@ConfigurationProperties(prefix = "todcost")
data class TodCostProperties(
    var host: String? = null,
    var platformKey: String = "",
    var dataSourceName: String = "",
    var serviceType: String = ""
)
