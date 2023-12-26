package com.tencent.devops.turbo.dto

import com.fasterxml.jackson.annotation.JsonProperty

data class TBSDaySummaryDto(
    val day: String,

    val user: String?,

    @JsonProperty("project_id")
    val projectId: String,

    /**
     * 加速时长(单位：秒)
     */
    @JsonProperty("total_time")
    val totalTime: Double,

    /**
     * 加速时长*cpu核数(单位秒*核)
     */
    @JsonProperty("total_time_with_cpu")
    val totalTimeWithCpu: Double,

    /**
     * 加速次数
     */
    @JsonProperty("total_record_number")
    val totalRecordNumber: Double,
)
