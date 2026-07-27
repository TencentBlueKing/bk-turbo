package com.tencent.devops.turbo.model

import org.springframework.data.annotation.Id
import org.springframework.data.mongodb.core.index.Indexed
import org.springframework.data.mongodb.core.mapping.Document
import org.springframework.data.mongodb.core.mapping.Field
import java.time.LocalDateTime

/**
 * 每日私有资源统计数据实体类
 */
@Document(collection = "t_tbs_private_day_summary_entity")
data class TTbsPrivateDaySummaryEntity(
    @Id
    var id: String? = null,
    /**
     * 统计数据归属日期
     */
    @Field
    @Indexed(background = true)
    var day: String?,

    /**
     * 蓝盾项目英文id
     */
    @Field("project_id")
    var projectId: String? = null,

    /**
     * 项目名称
     */
    @Field("project_name")
    var projectName: String? = null,

    /**
     * 方案id
     */
    @Field("plan_id")
    var planId: String?,

    /**
     * 方案名称
     */
    @Field("plan_name")
    var planName: String? = null,

    /**
     * 方案创建人
     */
    @Field("plan_creator")
    var planCreator: String? = null,

    /**
     * UE加速活跃 Agent
     * 项目+加速方案类型（即同一项目同一用户使用多个UE加速方案时不重复统计）
     */
    @Field
    var user: String? = null,

    /**
     * 加速模式
     */
    @Field("engine_code")
    var engineCode: String?,

    /**
     * 加速时长(单位：秒)
     */
    @Field("total_time")
    var totalTime: Double?,

    /**
     * 加速时长*cpu核数(单位秒*核)
     */
    @Field("total_time_with_cpu")
    var totalTimeWithCpu: Double?,

    /**
     * 加速次数
     */
    @Field("total_record_number")
    var totalRecordNumber: Double?,

    /**
     * 项目所属运营产品
     */
    @Field("product_id")
    var productId: Int? = null,

    /**
     * BG信息
     */
    @Field("bg_name")
    var bgName: String? = null,
    @Field("bg_id")
    var bgId: Int? = null,

    /**
     * 业务线
     */
    @Field("business_line_name")
    var businessLineName: String? = null,
    @Field("business_line_id")
    var businessLineId: Int? = null,

    /**
     * 部门信息
     */
    @Field("dept_name")
    var deptName: String? = null,
    @Field("dept_id")
    var deptId: Int? = null,

    /**
     * 中心信息
     */
    @Field("center_name")
    var centerName: String? = null,
    @Field("center_id")
    var centerId: Int? = null,

    /**
     * 项目所属组织架构
     */
    @Field("org_path")
    var orgPath: String? = null,

    /**
     * 本entity创建时间
     */
    @Field("created_date")
    var createdDate: LocalDateTime?
) {
    companion object {
        /**
         * 从公共资源实体转换为私有资源实体
         */
        fun from(entity: TTbsDaySummaryEntity): TTbsPrivateDaySummaryEntity {
            return TTbsPrivateDaySummaryEntity(
                id = null,
                day = entity.day,
                projectId = entity.projectId,
                projectName = entity.projectName,
                planId = entity.planId,
                planName = entity.planName,
                planCreator = entity.planCreator,
                user = entity.user,
                engineCode = entity.engineCode,
                totalTime = entity.totalTime,
                totalTimeWithCpu = entity.totalTimeWithCpu,
                totalRecordNumber = entity.totalRecordNumber,
                productId = entity.productId,
                bgName = entity.bgName,
                bgId = entity.bgId,
                businessLineName = entity.businessLineName,
                businessLineId = entity.businessLineId,
                deptName = entity.deptName,
                deptId = entity.deptId,
                centerName = entity.centerName,
                centerId = entity.centerId,
                orgPath = entity.orgPath,
                createdDate = entity.createdDate
            )
        }
    }
}
