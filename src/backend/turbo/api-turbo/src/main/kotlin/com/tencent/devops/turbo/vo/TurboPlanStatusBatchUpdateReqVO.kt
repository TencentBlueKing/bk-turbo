package com.tencent.devops.turbo.vo

data class TurboPlanStatusBatchUpdateReqVO(
    val status: Boolean,
    val projectIdList: List<String>
)
