package com.tencent.devops.turbo.pojo

import com.tencent.devops.turbo.validate.TurboPlanGroup
import io.swagger.annotations.ApiModel
import io.swagger.annotations.ApiModelProperty
import javax.validation.constraints.NotBlank
import javax.validation.constraints.NotEmpty
import javax.validation.constraints.NotNull

@ApiModel("加速方案请求数据信息")
data class TurboPlanModel(
    @ApiModelProperty("蓝盾项目id")
    @get:NotBlank(
        message = "{bizError.projectIdNotBlank}",
        groups = [
            TurboPlanGroup.Create::class,
            TurboPlanGroup.UpdateDetail::class
        ]
    )
    val projectId: String?,
    @ApiModelProperty("加速方案名称")
    @get:NotBlank(
        message = "{bizError.planNameNotBlank}",
        groups = [
            TurboPlanGroup.Create::class,
            TurboPlanGroup.UpdateDetail::class,
            TurboPlanGroup.UpdateAll::class
        ]
    )
    val planName: String?,
    @ApiModelProperty("蓝盾模板代码")
    @get:NotBlank(
        message = "{bizError.engineCodeNotSelect}",
        groups = [
            TurboPlanGroup.Create::class,
            TurboPlanGroup.UpdateWhiteList::class,
            TurboPlanGroup.UpdateAll::class
        ]
    )
    val engineCode: String?,
    @ApiModelProperty("方案说明")
    val desc: String?,
    @ApiModelProperty("配置参数值")
    @get:NotNull(
        message = "{bizError.configParamNotNull}",
        groups = [
            TurboPlanGroup.UpdateParam::class,
            TurboPlanGroup.UpdateAll::class
        ]
    )
    @get:NotEmpty(
        message = "{bizError.configParamNotEmpty}",
        groups = [
            TurboPlanGroup.UpdateParam::class,
            TurboPlanGroup.UpdateAll::class
        ]
    )
    val configParam: Map<String, Any>?,
    @ApiModelProperty("白名单")
    @get:NotBlank(
        message = "{bizError.whiteListNotBlank}",
        groups = [
            TurboPlanGroup.UpdateWhiteList::class
        ]
    )
    val whiteList: String?,
    @ApiModelProperty("开启状态")
    @get:NotNull(
        message = "{bizError.openStatusNotNull}",
        groups = [
            TurboPlanGroup.UpdateDetail::class,
            TurboPlanGroup.UpdateAll::class
        ]
    )
    val openStatus: Boolean?
)
