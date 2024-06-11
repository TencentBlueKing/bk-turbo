package com.tencent.devops.turbo.vo

import com.fasterxml.jackson.annotation.JsonProperty
import io.swagger.annotations.ApiModelProperty

data class ResourceCostSummary(

    @ApiModelProperty("数据源名称")
    @JsonProperty("data_source_name")
    val dataSourceName: String,

    @ApiModelProperty("所属月份")
    val month: String,

    @ApiModelProperty("是否清空数据再写入")
    @JsonProperty("is_overwrite")
    val isOverwrite: Boolean,

    @ApiModelProperty("清单")
    val bills: List<ProjectResourceCostVO>
)
