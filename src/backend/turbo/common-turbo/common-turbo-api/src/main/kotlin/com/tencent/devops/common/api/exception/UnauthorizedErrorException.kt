package com.tencent.devops.common.api.exception

import com.tencent.devops.common.api.exception.code.IS_NOT_ADMIN_MEMBER

class UnauthorizedErrorException(
    val errorCode: String = IS_NOT_ADMIN_MEMBER,
    errorMessage: String = ""
) : RuntimeException(errorMessage)
