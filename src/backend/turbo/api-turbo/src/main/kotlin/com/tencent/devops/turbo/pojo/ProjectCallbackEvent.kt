package com.tencent.devops.turbo.pojo

import com.tencent.devops.turbo.enums.ProjectEventType

data class ProjectCallbackEvent(
    val event: ProjectEventType,
    val data: TurboPlanUpdateModel
)
