package com.tencent.devops.turbo.dao.mongotemplate

import com.tencent.devops.turbo.model.TTbsDaySummaryEntity
import org.slf4j.LoggerFactory
import org.springframework.beans.factory.annotation.Autowired
import org.springframework.data.domain.Sort
import org.springframework.data.mongodb.core.MongoTemplate
import org.springframework.data.mongodb.core.aggregation.Aggregation
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
        pageNum: Int,
        pageSize: Int
    ): List<TTbsDaySummaryEntity> {
        logger.info("findByDay startDate: $startDate, endDate: $endDate, filterPlanIdNin: $filterPlanIdNin")

        val criteria = Criteria.where("day").gte(startDate).lte(endDate)
            .and("user").`is`(null)

        filterPlanIdNin.takeIf { it.isNotEmpty() }.let { criteria.and("plan_id").nin(filterPlanIdNin) }

        val match = Aggregation.match(criteria)
        val sort = Aggregation.sort(Sort.Direction.DESC, "day")
        val group = Aggregation.group("plan_id", "project_id", "engine_code")
            .sum("total_time_with_cpu").`as`("total_time_with_cpu")
            .first("project_id").`as`("project_id")
            .first("project_name").`as`("project_name")
            .first("plan_id").`as`("plan_id")
            .first("plan_name").`as`("plan_name")
            .first("engine_code").`as`("engine_code")
            .first("plan_creator").`as`("plan_creator")
            .first("product_id").`as`("product_id")
            .first("bg_name").`as`("bg_name")
            .first("dept_name").`as`("dept_name")
            .first("center_name").`as`("center_name")
            .first("bg_id").`as`("bg_id")
            .first("dept_id").`as`("dept_id")
            .first("center_id").`as`("center_id")

        val skip = Aggregation.skip((pageNum * pageSize).toLong())
        val limit = Aggregation.limit(pageSize.toLong())

        val aggregation = Aggregation.newAggregation(match, sort, group, skip, limit)
        return mongoTemplate.aggregate(aggregation, COLLECTION_NAME, TTbsDaySummaryEntity::class.java).mappedResults
    }
}
