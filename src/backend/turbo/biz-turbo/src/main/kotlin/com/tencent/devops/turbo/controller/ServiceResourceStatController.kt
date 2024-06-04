package com.tencent.devops.turbo.controller

import com.tencent.devops.api.pojo.Response
import com.tencent.devops.common.api.pojo.Page
import com.tencent.devops.turbo.api.IServiceResourceStatController
import com.tencent.devops.turbo.service.ProjectResourcesService
import com.tencent.devops.turbo.vo.ProjectResourceUsageVO
import org.springframework.beans.factory.annotation.Autowired
import org.springframework.web.bind.annotation.RestController

@RestController
class ServiceResourceStatController @Autowired constructor(
    private val projectResourcesService: ProjectResourcesService
) : IServiceResourceStatController {

    override fun getSummary(
        startDate: String?,
        endDate: String?,
        pageNum: Int?,
        pageSize: Int?
    ): Response<Page<ProjectResourceUsageVO>> {
        return Response.success(projectResourcesService.querySummary(startDate, endDate, pageNum, pageSize))
    }
}
