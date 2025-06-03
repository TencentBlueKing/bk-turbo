package com.tencent.devops.common.util.enums

/**
 * 编译加速RBAC权限粒度(资源类型)
 */
enum class ResourceType(
    val id: String
) {
    TURBO_PLAN("turbo_plan"),
    PROJECT("project")
}
