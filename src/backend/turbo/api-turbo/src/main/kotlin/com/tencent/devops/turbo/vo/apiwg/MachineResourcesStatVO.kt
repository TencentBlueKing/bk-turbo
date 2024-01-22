package com.tencent.devops.turbo.vo.apiwg

import io.swagger.annotations.ApiModel
import io.swagger.annotations.ApiModelProperty

@ApiModel("机器资源使用统计")
data class MachineResourcesStatVO(

    @ApiModelProperty("项目id")
    val projectId: String,

    @ApiModelProperty("项目名称")
    val projectName: String?,
    
    @ApiModelProperty("方案id")
    val planId: String,

    @ApiModelProperty("方案名称")
    val planName: String?,

    @ApiModelProperty("方案创建者")
    var planCreator: String?,

    @ApiModelProperty("加速模式")
    var engineCode: String?,

    @ApiModelProperty("加速时长*cpu核数(单位秒*核)")
    var totalTimeWithCpu: Double?,

    @ApiModelProperty("项目所属运营产品id")
    var productId: Int?,

    @ApiModelProperty("BG名称")
    var bgName: String?,

    @ApiModelProperty("BG id")
    var bgId: Int?,

    @ApiModelProperty("部门名称")
    var deptName: String?,

    @ApiModelProperty("部门id")
    var deptId: Int?,

    @ApiModelProperty("中心名称")
    var centerName: String?,

    @ApiModelProperty("中心id")
    var centerId: Int?,
)
