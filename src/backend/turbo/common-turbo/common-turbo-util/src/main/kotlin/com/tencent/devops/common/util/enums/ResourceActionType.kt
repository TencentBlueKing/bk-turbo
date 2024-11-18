package com.tencent.devops.common.util.enums

enum class ResourceActionType(
    val actionId: String,
    val alias: String
) {
    CREATE("turbo_plan_create", "新增加速方案"),
    LIST("turbo_plan_list", "加速方案列表"),
    VIEW("turbo_plan_view", "查看加速方案"),
    EDIT("turbo_plan_edit", "编辑加速方案"),
    DELETE("turbo_plan_delete", "删除加速方案"),
    OVERVIEW("turbo_plan_overview", "查看编译加速总览"),
    LIST_TASK("turbo_plan_list-task", "编译加速执行历史列表"),
    VIEW_TASK("turbo_plan_view-task", "查看编译加速执行详情")
}
