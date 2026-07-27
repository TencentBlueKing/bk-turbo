package com.tencent.devops.turbo.job

import com.tencent.devops.common.client.Client
import com.tencent.devops.common.util.DateTimeUtils
import com.tencent.devops.common.util.JsonUtil
import com.tencent.devops.common.util.constants.STAT_PRIVATE_ENGINE_CODE
import com.tencent.devops.project.api.service.ServiceProjectResource
import com.tencent.devops.project.pojo.ProjectVO
import com.tencent.devops.turbo.dao.mongotemplate.TbsPrivateDaySummaryDao
import com.tencent.devops.turbo.dao.repository.BaseDataRepository
import com.tencent.devops.turbo.dao.repository.TbsDaySummaryRepository
import com.tencent.devops.turbo.dao.repository.TurboEngineConfigRepository
import com.tencent.devops.turbo.dao.repository.TurboPlanRepository
import com.tencent.devops.turbo.dto.TBSDaySummaryDto
import com.tencent.devops.turbo.model.TTbsDaySummaryEntity
import com.tencent.devops.turbo.model.TTbsPrivateDaySummaryEntity
import com.tencent.devops.turbo.sdk.TBSSdkApi
import org.quartz.Job
import org.quartz.JobExecutionContext
import org.slf4j.LoggerFactory
import org.springframework.beans.factory.annotation.Autowired
import org.springframework.stereotype.Component
import java.time.LocalDate
import java.time.LocalDateTime

@Suppress("SpringJavaAutowiredMembersInspection")
@Component
class TBSDaySummaryJob @Autowired constructor(
    private val client: Client,
    private val baseDataRepository: BaseDataRepository,
    private val tbsDaySummaryRepository: TbsDaySummaryRepository,
    private val tbsPrivateDaySummaryDao: TbsPrivateDaySummaryDao,
    private val turboEngineConfigRepository: TurboEngineConfigRepository,
    private val turboPlanRepository: TurboPlanRepository
) : Job {

    companion object {
        private val logger = LoggerFactory.getLogger(this::class.java)
        private const val PAGE_SIZE = 3000
    }

    /**
     * 执行入口
     */
    override fun execute(context: JobExecutionContext) {
        logger.info("TBS day summary job start executing: ${JsonUtil.toJson(context.jobDetail)}")

        val jobParam = context.jobDetail.jobDataMap
        val statisticsDateStr = if (jobParam.containsKey("statisticsDate")) {
            jobParam["statisticsDate"] as String
        } else {
            // 统计昨天
            val statLocalDate = LocalDate.now().minusDays(1)
            DateTimeUtils.localDate2DateStr(statLocalDate)
        }

        // 清理待统计的数据，防止重复统计
        tbsDaySummaryRepository.removeAllByDay(statisticsDateStr)
        // 清理待统计的私有资源数据，防止重复统计
        tbsPrivateDaySummaryDao.removeAllByDay(statisticsDateStr)

        val projectVOMap = mutableMapOf<String, ProjectVO>()
        // 缓存RPC未查询到项目信息的项目id，避免后续批次重复查询
        val notFoundProjectIds = mutableSetOf<String>()
        val queryParam = mapOf("day" to statisticsDateStr)

        // 采集公共资源统计数据
        val engineCodes = turboEngineConfigRepository.findAll().map { it.engineCode }
        this.collectAndSaveSummary(
            tag = "public",
            engineCodes = engineCodes,
            projectVOMap = projectVOMap,
            notFoundProjectIds = notFoundProjectIds,
            queryFunc = { engineCode -> TBSSdkApi.queryTbsDaySummary(engineCode, queryParam) },
            converter = { it },
            saveFunc = { entities -> tbsDaySummaryRepository.saveAll(entities) }
        )

        // 采集私有资源统计数据
        // 从配置表获取需要统计私有资源的引擎code
        val privateEngineCodes = baseDataRepository.findByParamCode(STAT_PRIVATE_ENGINE_CODE)
            .map { it.paramValue }.toSet()
        this.collectAndSaveSummary(
            tag = "private",
            engineCodes = privateEngineCodes,
            projectVOMap = projectVOMap,
            notFoundProjectIds = notFoundProjectIds,
            queryFunc = { engineCode -> TBSSdkApi.queryTBSPrivateSummary(engineCode, queryParam) },
            converter = TTbsPrivateDaySummaryEntity::from,
            saveFunc = { entities -> tbsPrivateDaySummaryDao.saveAll(entities) }
        )

        logger.info("TBS day summary job execution completed!")
    }

    /**
     * 按引擎采集TBS统计数据并落库
     * @param tag 统计类型标识（public/private），用于日志区分
     * @param queryFunc 按引擎code查询TBS统计接口
     * @param converter 实体转换器，公共资源传入恒等转换，私有资源传入TTbsPrivateDaySummaryEntity::from
     * @param saveFunc 批量落库
     */
    private fun <T> collectAndSaveSummary(
        tag: String,
        engineCodes: Collection<String>,
        projectVOMap: MutableMap<String, ProjectVO>,
        notFoundProjectIds: MutableSet<String>,
        queryFunc: (String) -> List<TBSDaySummaryDto>,
        converter: (TTbsDaySummaryEntity) -> T,
        saveFunc: (List<T>) -> Unit
    ) {
        engineCodes.forEach { engineCode ->
            logger.info("query $tag summary for engineCode: $engineCode")
            val dtoList = try {
                queryFunc(engineCode)
            } catch (e: Exception) {
                logger.error("query $tag summary error! engineCode: $engineCode", e)
                return@forEach
            }

            logger.info("$tag summaryDtoList size: ${dtoList.size}")
            if (dtoList.isEmpty()) {
                logger.warn("query $tag summary result is empty! engineCode: $engineCode")
                return@forEach
            }

            // 把TBS的接口数据整理成entity并关联方案和项目信息
            val entityList = this.enrichSummaryEntities(dtoList, projectVOMap, notFoundProjectIds, converter)
            saveFunc(entityList)
            logger.info("save $tag summary entity size: ${entityList.size}")
        }
    }

    /**
     * 将TBS接口返回的DTO列表转为实体，关联方案和项目信息，最后通过converter转换为目标类型
     */
    private fun <T> enrichSummaryEntities(
        daySummaryDtoList: List<TBSDaySummaryDto>,
        projectVOMap: MutableMap<String, ProjectVO>,
        notFoundProjectIds: MutableSet<String>,
        converter: (TTbsDaySummaryEntity) -> T
    ): List<T> {
        val summaryEntityList = this.dto2SummaryEntityList(daySummaryList = daySummaryDtoList)
        summaryEntityList.chunked(PAGE_SIZE).forEach { batch ->
            this.enrichPlanInfo(batch)
            this.enrichProjectInfo(batch, projectVOMap, notFoundProjectIds)
        }
        return summaryEntityList.map(converter)
    }

    /**
     * 批量关联方案信息，赋值planCreator/planName/projectId
     */
    private fun enrichPlanInfo(entities: List<TTbsDaySummaryEntity>) {
        val planIds = entities.map { it.planId }.toSet()
        val turboPlanList = turboPlanRepository.findByIdIn(planIds.toList())
        logger.info("turboPlanRepository.findByIdIn result size: ${turboPlanList.size}")
        val planEntityMap = turboPlanList.associateBy { it.id }
        for (entity in entities) {
            val planEntity = planEntityMap[entity.planId]
            entity.planCreator = planEntity?.createdBy
            entity.planName = planEntity?.planName
            entity.projectId = planEntity?.projectId
        }
    }

    /**
     * 批量关联项目组织架构信息，赋值projectName/bgName/deptName/centerName等
     */
    private fun enrichProjectInfo(
        entities: List<TTbsDaySummaryEntity>,
        projectVOMap: MutableMap<String, ProjectVO>,
        notFoundProjectIds: MutableSet<String>
    ) {
        // 补充未缓存的项目信息，跳过已确认不存在的项目避免重复RPC
        val projectIdSet = entities.mapNotNull { it.projectId }.toSet()
        val notInProjectMapKeySet = projectIdSet.subtract(projectVOMap.keys).subtract(notFoundProjectIds)
        if (notInProjectMapKeySet.isNotEmpty()) {
            val projectVOList = this.getProjectVOListByProjectIds(notInProjectMapKeySet.toList())
            if (projectVOList.isNotEmpty()) {
                projectVOMap.putAll(projectVOList.associateBy { it.englishName })
            }
            // 记录RPC未返回的项目id
            notFoundProjectIds.addAll(notInProjectMapKeySet.subtract(projectVOMap.keys))
        }
        // 赋值项目组织架构信息
        for (entity in entities) {
            val projectVO = projectVOMap[entity.projectId]
            entity.projectName = projectVO?.projectName
            entity.bgName = projectVO?.bgName
            entity.bgId = projectVO?.bgId?.toInt()
            entity.businessLineName = projectVO?.businessLineName
            entity.businessLineId = projectVO?.businessLineId?.toInt()
            entity.deptName = projectVO?.deptName
            entity.deptId = projectVO?.deptId?.toInt()
            entity.centerName = projectVO?.centerName
            entity.centerId = projectVO?.centerId?.toInt()
            entity.productId = projectVO?.productId
        }
    }

    /**
     * 把TBS的接口数据整理成entity
     */
    private fun dto2SummaryEntityList(daySummaryList: List<TBSDaySummaryDto>): List<TTbsDaySummaryEntity> {
        return daySummaryList.map { summary ->
            // distcc与其它不一样，它的projectId就是planId
            // projectId格式示例："60d54b87a26123319d011bob_cc"
            val stringArr = summary.projectId.split("_")
            val engineCode = when (stringArr.getOrNull(1)) {
                null -> "distcc"
                "cc" -> "disttask-cc"
                "ue4" -> "disttask-ue4"
                else -> stringArr[1]
            }
            TTbsDaySummaryEntity(
                day = summary.day,
                engineCode = engineCode,
                planId = stringArr[0],
                // user字段没有值即为disttask的统计数据，有值的是ue的用户数据
                user = if (engineCode == "disttask-ue4") summary.user else null,
                totalTime = summary.totalTime,
                totalTimeWithCpu = summary.totalTimeWithCpu,
                totalRecordNumber = summary.totalRecordNumber,
                createdDate = LocalDateTime.now()
            )
        }
    }

    /**
     * 根据项目id获取项目信息
     */
    private fun getProjectVOListByProjectIds(projectIds: List<String>): List<ProjectVO> {
        if (projectIds.isEmpty()) {
            return emptyList()
        }
        val result = client.get(ServiceProjectResource::class.java).listByProjectCodeList(projectIds)
        if (result.isNotOk() || result.data == null) {
            logger.error("ServiceProjectResource#get request is failed!")
            return emptyList()
        }
        return result.data ?: emptyList()
    }
}
