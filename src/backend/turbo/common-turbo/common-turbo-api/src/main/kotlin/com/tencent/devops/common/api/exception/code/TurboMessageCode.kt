package com.tencent.devops.common.api.exception.code

/**
 * 如果需要自定义错误码，必须同步添加到i18n资源文件
 * 若想不到异常分类，且需要自定义error message，请使用通用异常(不建议)
 */
const val TURBO_GENERAL_SYSTEM_FAIL = "2121001" // 通用异常
const val TURBO_THIRDPARTY_SYSTEM_FAIL = "2121002" // 调用第三方系统失败
const val TURBO_NO_DATA_FOUND = "2121003" // 没有查询到对应数据
const val TURBO_PARAM_INVALID = "2121004" // 参数异常
const val IS_NOT_ADMIN_MEMBER = "2121005" // 非管理员，操作失败
const val SUB_CLASS_CHECK_ERROR = "2121006" // 子类检查错误
const val PERMISSION_DENIED = "2121007" // {0}无权限
