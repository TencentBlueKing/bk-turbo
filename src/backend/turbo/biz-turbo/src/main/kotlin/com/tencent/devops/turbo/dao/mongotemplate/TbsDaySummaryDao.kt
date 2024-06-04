package com.tencent.devops.turbo.dao.mongotemplate

import com.tencent.devops.turbo.model.TTbsDaySummaryEntity
import org.slf4j.LoggerFactory
import org.springframework.beans.factory.annotation.Autowired
import org.springframework.data.domain.Sort
import org.springframework.data.mongodb.core.MongoTemplate
import org.springframework.data.mongodb.core.aggregation.Aggregation
import org.springframework.data.mongodb.core.aggregation.AggregationOptions
import org.springframework.data.mongodb.core.query.Criteria
import org.springframework.stereotype.Repository

@Repository
class TbsDaySummaryDao @Autowired constructor(
    private val mongoTemplate: MongoTemplate
) {
    companion object {
        private val logger = LoggerFactory.getLogger(this::class.java)
        private const val COLLECTION_NAME = "t_tbs_day_summary_entity"
    }

    /**
     * 根据日期查询机器资源统计
     */
    fun findByDay(
        startDate: String,
        endDate: String,
        filterPlanIdNin: Set<String>,
        filterProjectIdNin: Set<String>,
        pageNum: Int,
        pageSize: Int
    ): List<TTbsDaySummaryEntity> {
        logger.info("findByDay startDate: $startDate, endDate: $endDate, filterPlanIdNin: $filterPlanIdNin")

        val criteria = Criteria.where("day").gte(startDate).lte(endDate)
            .and("user").`is`(null)

        // 过滤方案id
        filterPlanIdNin.takeIf { it.isNotEmpty() }.let { criteria.and("plan_id").nin(filterPlanIdNin) }
        // 过滤项目id
        filterProjectIdNin.takeIf { it.isNotEmpty() }.let { criteria.and("project_id").nin(filterProjectIdNin) }

        val match = Aggregation.match(criteria)
        val sort = Aggregation.sort(Sort.Direction.DESC, "day", "created_date")
        val group = Aggregation.group("project_id", "engine_code")
            .sum("total_time_with_cpu").`as`("total_time_with_cpu")
            .first("project_id").`as`("project_id")
            .first("project_name").`as`("project_name")
            .first("engine_code").`as`("engine_code")
            .first("product_id").`as`("product_id")
            .first("bg_name").`as`("bg_name")
            .first("dept_name").`as`("dept_name")
            .first("center_name").`as`("center_name")
            .first("bg_id").`as`("bg_id")
            .first("dept_id").`as`("dept_id")
            .first("center_id").`as`("center_id")

        val skip = Aggregation.skip((pageNum * pageSize).toLong())
        val limit = Aggregation.limit(pageSize.toLong())
        val options = AggregationOptions.Builder().allowDiskUse(true).build()
        val aggregation = Aggregation.newAggregation(match, sort, group, skip, limit).withOptions(options)
        return mongoTemplate.aggregate(aggregation, COLLECTION_NAME, TTbsDaySummaryEntity::class.java).mappedResults
    }

    /**
     * 统计C++使用量
     */
    fun findByDayForCPlus(
        startDate: String,
        endDate: String,
        filterPlanIdNin: Set<String>,
        filterProjectIdNin: Set<String>,
        pageNum: Int,
        pageSize: Int
    ): List<TTbsDaySummaryEntity> {
        logger.info("findByDayForCPlus startDate: $startDate, endDate: $endDate, filterPlanIdNin: $filterPlanIdNin")

        val criteria = Criteria.where("day").gte(startDate).lte(endDate)
            .and("user").`is`(null)
            .and("engine_code").ne("disttask-ue4")
            .apply { if (filterPlanIdNin.isNotEmpty()) and("plan_id").nin(filterPlanIdNin) }
            .apply { if (filterProjectIdNin.isNotEmpty()) and("project_id").nin(filterProjectIdNin) }

        val match = Aggregation.match(criteria)
        val sort = Aggregation.sort(Sort.Direction.DESC, "day", "created_date")
        val group = Aggregation.group("project_id")
            .first("project_id").`as`("project_id")
            .first("project_name").`as`("project_name")
            .first("engine_code").`as`("engine_code")
            .first("product_id").`as`("product_id")
            .first("bg_name").`as`("bg_name")
            .sum("total_time_with_cpu").`as`("total_time_with_cpu")

        val skip = Aggregation.skip((pageNum * pageSize).toLong())
        val limit = Aggregation.limit(pageSize.toLong())
        val options = AggregationOptions.Builder().allowDiskUse(true).build()
        val aggregation = Aggregation.newAggregation(match, sort, group, skip, limit).withOptions(options)
        return mongoTemplate.aggregate(aggregation, COLLECTION_NAME, TTbsDaySummaryEntity::class.java).mappedResults
    }

    /**
     * 统计UE用户数
     */
    fun findByDayForUE(
        startDate: String,
        endDate: String,
        filterPlanIdNin: Set<String>,
        filterProjectIdNin: Set<String>,
        pageNum: Int,
        pageSize: Int
    ): List<TTbsDaySummaryEntity> {
        logger.info("findByDayForUE startDate: $startDate, endDate: $endDate, filterPlanIdNin: $filterPlanIdNin")
        val criteria = Criteria.where("day").gte(startDate).lte(endDate)
            .and("user").ne(null)
            .and("engine_code").`is`("disttask-ue4")
            .apply {
                if (filterPlanIdNin.isNotEmpty()) and("plan_id").nin(filterPlanIdNin)
                if (filterProjectIdNin.isNotEmpty()) and("project_id").nin(filterProjectIdNin)
            }

        val match = Aggregation.match(criteria)
        val sort = Aggregation.sort(Sort.Direction.DESC, "day", "created_date")
        val group1 = Aggregation.group("project_id", "user")
            .first("project_id").`as`("project_id")
            .first("project_name").`as`("project_name")
            .first("engine_code").`as`("engine_code")
            .first("bg_name").`as`("bg_name")

        val group2 = Aggregation.group("project_id")
            .first("project_id").`as`("project_id")
            .first("project_name").`as`("project_name")
            .first("engine_code").`as`("engine_code")
            .first("bg_name").`as`("bg_name")
            // 放这里暂存 用户数
            .count().`as`("total_time_with_cpu")

        val skip = Aggregation.skip((pageNum * pageSize).toLong())
        val limit = Aggregation.limit(pageSize.toLong())
        val options = AggregationOptions.Builder().allowDiskUse(true).build()
        val aggregation = Aggregation.newAggregation(match, sort, group1, group2, skip, limit).withOptions(options)
        return mongoTemplate.aggregate(aggregation, COLLECTION_NAME, TTbsDaySummaryEntity::class.java).mappedResults
    }
}
