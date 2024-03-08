package com.tencent.devops.turbo.api

import com.tencent.devops.api.pojo.Response
import com.tencent.devops.common.api.annotation.ServiceInterface
import com.tencent.devops.common.api.pojo.Page
import com.tencent.devops.turbo.vo.ProjectResourceUsageVO
import io.swagger.annotations.Api
import io.swagger.annotations.ApiOperation
import io.swagger.annotations.ApiParam
import org.springframework.cloud.openfeign.FeignClient
import org.springframework.http.MediaType
import org.springframework.web.bind.annotation.GetMapping
import org.springframework.web.bind.annotation.RequestMapping
import org.springframework.web.bind.annotation.RequestParam

@Api(tags = ["SERVER_RESOURCES"], description = "资源统计查询接口")
@RequestMapping("/service/resources")
@FeignClient(name = "turbo", contextId = "IServiceResourceStatController")
@ServiceInterface("turbo-new")
interface IServiceResourceStatController {

    @ApiOperation("获取项目使用服务器资源统计")
    @GetMapping("/getSummary", produces = [MediaType.APPLICATION_JSON_VALUE])
    fun getSummary(
        @ApiParam("起始统计日期")
        @RequestParam("startDate")
        startDate: String?,
        @ApiParam("截止统计日期")
        @RequestParam("endDate")
        endDate: String?,
        @ApiParam(value = "页数")
        @RequestParam(value = "pageNum")
        pageNum: Int?,
        @ApiParam(value = "每页多少条")
        @RequestParam("pageSize")
        pageSize: Int?,
    ): Response<Page<ProjectResourceUsageVO>>
}
