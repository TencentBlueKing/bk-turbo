package com.tencent.devops.common.api.annotation

import com.tencent.devops.common.util.enums.ResourceActionType
import com.tencent.devops.common.util.enums.ResourceType

@MustBeDocumented
@Target(AnnotationTarget.FUNCTION, AnnotationTarget.CLASS)
@Retention(AnnotationRetention.RUNTIME)
annotation class RequiresAuth(
    /**
     * 资源类型 选填，默认加速方案
     */
    val resourceType: ResourceType = ResourceType.TURBO_PLAN,

    /**
     * 对资源的操作权限id  选填，默认查看方案权限
     */
    val permission: ResourceActionType = ResourceActionType.VIEW
)
