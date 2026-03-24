package com.tencent.devops.turbo.vo

import com.fasterxml.jackson.annotation.JsonProperty
import io.swagger.annotations.ApiModel
import io.swagger.annotations.ApiModelProperty

@ApiModel("项目资源成本统计")
data class ProjectResourceCostVO(

    @ApiModelProperty("统计周期：yyyyMM")
    @JsonProperty("cost_date")
    val costDate: String,

    @ApiModelProperty("项目ID")
    @JsonProperty("project_id")
    val projectId: String,

    @ApiModelProperty("项目名称")
    val name: String?,

    @ApiModelProperty("加速模式:")
    var kind: String?,

    @ApiModelProperty("加速模式:")
    @JsonProperty("service_type")
    var serviceType: String?,

    @ApiModelProperty("C++加速时长*cpu核数(单位分钟*核) 、 UE用户数")
    var usage: String?,

    @ApiModelProperty("BG名称")
    @JsonProperty("bg_name")
    var bgName: String?,

    @ApiModelProperty("标识")
    var flag: Int = 1
)
