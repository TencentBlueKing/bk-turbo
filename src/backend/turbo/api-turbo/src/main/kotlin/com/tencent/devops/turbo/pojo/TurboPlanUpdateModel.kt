package com.tencent.devops.turbo.pojo

import io.swagger.annotations.ApiModel
import io.swagger.annotations.ApiModelProperty

@ApiModel("加速方案状态更新")
data class TurboPlanUpdateModel(
    @ApiModelProperty("操作用户")
    val userId: String,

    @ApiModelProperty("蓝盾项目ID")
    val projectId: String,

    @ApiModelProperty("蓝盾项目名称")
    val projectName: String,

    @ApiModelProperty("开启状态：true表示启用项目，false表示停用项目")
    val enabled: Boolean
)
