package com.tencent.devops.turbo.api

import com.tencent.devops.api.pojo.Response
import com.tencent.devops.common.api.annotation.ServiceInterface
import com.tencent.devops.common.api.pojo.Page
import com.tencent.devops.common.util.constants.AUTH_HEADER_DEVOPS_PROJECT_ID
import com.tencent.devops.common.util.constants.AUTH_HEADER_DEVOPS_USER_ID
import com.tencent.devops.turbo.vo.ProjectResourceUsageVO
import com.tencent.devops.turbo.vo.ResourceCostSummary
import io.swagger.annotations.Api
import io.swagger.annotations.ApiOperation
import io.swagger.annotations.ApiParam
import org.springframework.cloud.openfeign.FeignClient
import org.springframework.http.MediaType
import org.springframework.web.bind.annotation.DeleteMapping
import org.springframework.web.bind.annotation.GetMapping
import org.springframework.web.bind.annotation.PathVariable
import org.springframework.web.bind.annotation.PostMapping
import org.springframework.web.bind.annotation.PutMapping
import org.springframework.web.bind.annotation.RequestBody
import org.springframework.web.bind.annotation.RequestHeader
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

    @ApiOperation("触发项目资源统计上报任务")
    @PutMapping("/triggerAutoUpload", produces = [MediaType.APPLICATION_JSON_VALUE])
    fun triggerAutoUpload(
        @ApiParam(value = "用户信息", required = true)
        @RequestHeader(AUTH_HEADER_DEVOPS_USER_ID)
        userId: String,
        @ApiParam(value = "项目id", required = true)
        @RequestHeader(AUTH_HEADER_DEVOPS_PROJECT_ID)
        projectId: String,
        @ApiParam("所属周期月份")
        @RequestBody(required = true)
        summary: ResourceCostSummary,
        @ApiParam("起始统计日期")
        @RequestParam("startDate")
        startDate: String?,
        @ApiParam("截止统计日期")
        @RequestParam("endDate")
        endDate: String?
    ): Response<Boolean>

    @ApiOperation("手动上报项目资源统计数据")
    @PostMapping("/manualUpload", produces = [MediaType.APPLICATION_JSON_VALUE])
    fun triggerManualUpload(
        @ApiParam(value = "用户信息", required = true)
        @RequestHeader(AUTH_HEADER_DEVOPS_USER_ID)
        userId: String,
        @ApiParam(value = "项目id", required = true)
        @RequestHeader(AUTH_HEADER_DEVOPS_PROJECT_ID)
        projectId: String,
        @ApiParam("待上报的数据")
        @RequestBody(required = true)
        summary: ResourceCostSummary
    ): Response<Boolean>
}
