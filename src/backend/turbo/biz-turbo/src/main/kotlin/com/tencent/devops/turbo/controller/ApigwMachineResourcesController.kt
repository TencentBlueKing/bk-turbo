package com.tencent.devops.turbo.controller

import com.tencent.devops.api.pojo.Response
import com.tencent.devops.common.api.pojo.Page
import com.tencent.devops.turbo.api.IApigwMachineResourcesController
import com.tencent.devops.turbo.service.MachineResourcesService
import com.tencent.devops.turbo.vo.apiwg.MachineResourcesStatVO
import org.springframework.beans.factory.annotation.Autowired
import org.springframework.web.bind.annotation.RestController

@RestController
class ApigwMachineResourcesController @Autowired constructor(
    private val machineResourcesService: MachineResourcesService
) : IApigwMachineResourcesController{

    override fun getSummary(
        appCode: String,
        startDate: String?,
        endDate: String?,
        pageNum: Int?,
        pageSize: Int?
    ): Response<Page<MachineResourcesStatVO>> {
        return Response.success(machineResourcesService.querySummary(startDate, endDate, pageNum, pageSize))
    }
}
