package com.tencent.devops.turbo.pojo

data class ProjectEnableStatusBroadCastEvent(
    val userId: String,
    val projectId: String,
    val enabled: Boolean
)
