package com.tencent.devops.turbo.vo

import io.swagger.annotations.ApiModel
import io.swagger.annotations.ApiModelProperty

@ApiModel("机器资源使用统计")
data class ProjectResourceUsageVO(

    @ApiModelProperty("项目id")
    val projectId: String,

    @ApiModelProperty("项目名称")
    val projectName: String?,

    @ApiModelProperty("加速模式")
    var engineCode: String?,

    @ApiModelProperty("加速时长*cpu核数(单位分钟*核)")
    var totalTimeWithCpu: Double?,

    @ApiModelProperty("项目所属运营产品id")
    var productId: Int?,

    @ApiModelProperty("BG名称")
    var bgName: String?,

    @ApiModelProperty("BG id")
    var bgId: Int?,

    @ApiModelProperty("业务线名称")
    var businessLineName: String?,

    @ApiModelProperty("业务线id")
    var businessLineId: Int?,

    @ApiModelProperty("部门名称")
    var deptName: String?,

    @ApiModelProperty("部门id")
    var deptId: Int?,

    @ApiModelProperty("中心名称")
    var centerName: String?,

    @ApiModelProperty("中心id")
    var centerId: Int?,
)
