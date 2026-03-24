package com.tencent.devops.turbo.pojo

import io.swagger.annotations.ApiModel
import io.swagger.annotations.ApiModelProperty
import javax.validation.constraints.NotBlank

@ApiModel("运行编译加速插件时入参")
data class TurboPlanInstanceModel(
    @ApiModelProperty("项目id")
    @get:NotBlank(message = "{bizError.projectIdNotBlank}")
    val projectId: String?,
    @ApiModelProperty("编译加速方案id")
    @get:NotBlank(message = "{bizError.turboPlanIdNotBlank}")
    val turboPlanId: String?,
    @ApiModelProperty("流水线id")
    @get:NotBlank(message = "{bizError.pipelineIdNotBlank}")
    val pipelineId: String?,
    @ApiModelProperty("流水线元素id")
    @get:NotBlank(message = "{bizError.pipelineElementIdNotBlank}")
    val pipelineElementId: String?,
    @ApiModelProperty("流水线名称")
    val pipelineName: String? = null,
    @ApiModelProperty("流水线构建id")
    @get:NotBlank(message = "{bizError.buildIdNotBlank}")
    val buildId: String?
)
