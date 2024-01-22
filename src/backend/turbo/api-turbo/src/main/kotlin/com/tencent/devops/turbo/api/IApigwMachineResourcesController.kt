package com.tencent.devops.turbo.api

import com.tencent.devops.api.pojo.Response
import com.tencent.devops.turbo.vo.apiwg.MachineResourcesStatVO
import io.swagger.annotations.Api
import io.swagger.annotations.ApiOperation
import io.swagger.annotations.ApiParam
import org.springframework.http.MediaType
import org.springframework.web.bind.annotation.GetMapping
import org.springframework.web.bind.annotation.RequestMapping
import org.springframework.web.bind.annotation.RequestParam

@Api(tags = ["OPENAPI_SERVER_RESOURCES"], description = "服务器资源查询接口")
@RequestMapping("/machine/resources", consumes = [MediaType.APPLICATION_JSON_VALUE],
    produces = [MediaType.APPLICATION_JSON_VALUE])
interface IApigwMachineResourcesController {

    @ApiOperation("获取使用服务器资源统计")
    @GetMapping("/getSummary")
    fun getSummary(
        @ApiParam("应用code")
        @RequestParam("appCode")
        appCode: String,
        @ApiParam("日期类型")
        @RequestParam("startDate")
        startDate: String?,
        @ApiParam("日期类型")
        @RequestParam("endDate")
        endDate: String?
    ): Response<List<MachineResourcesStatVO>>
}
