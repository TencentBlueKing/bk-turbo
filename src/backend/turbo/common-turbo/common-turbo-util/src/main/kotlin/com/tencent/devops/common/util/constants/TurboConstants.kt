package com.tencent.devops.common.util.constants

/**
 * 系统默认管理员id
 */
const val SYSTEM_ADMIN = "Turbo"

/**
 * 无权限错误提示信息
 */
const val NO_ADMIN_MEMBER_MESSAGE = "无权限，请先加入项目！"

/**
 * 统计数据需排除的plan id(DevCloud专用统计排除，服务计费不需要过滤掉)
 * 微信和王者的机器资源是他们自己管理，DevCloud无需计费，咱们内部计费需要纳入统计
 */
const val BASE_EXCLUDED_PLAN_ID_LIST_FOR_DEV_CLOUD = "EXCLUDED_PLAN_ID_LIST"

/**
 * 统计数据需排除的project id
 */
const val BASE_EXCLUDED_PROJECT_ID_LIST = "EXCLUDED_PROJECT_ID_LIST"

/**
 * 内部测试的方案，服务计费时需要过滤掉
 */
const val BASE_EXCLUDED_COMMON_PLAN_ID = "EXCLUDED_COMMON_PLAN_ID_LIST"
