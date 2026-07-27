package com.tencent.devops.turbo.dao.mongotemplate

import com.tencent.devops.turbo.model.TTbsPrivateDaySummaryEntity
import org.slf4j.LoggerFactory
import org.springframework.beans.factory.annotation.Autowired
import org.springframework.data.domain.Sort
import org.springframework.data.mongodb.core.MongoTemplate
import org.springframework.data.mongodb.core.aggregation.Aggregation
import org.springframework.data.mongodb.core.aggregation.AggregationOptions
import org.springframework.data.mongodb.core.query.Criteria
import org.springframework.data.mongodb.core.query.Query
import org.springframework.stereotype.Repository

@Repository
class TbsPrivateDaySummaryDao @Autowired constructor(
    private val mongoTemplate: MongoTemplate
) {
    companion object {
        private val logger = LoggerFactory.getLogger(this::class.java)
        private const val COLLECTION_NAME = "t_tbs_private_day_summary_entity"
    }

    /**
     * 根据日期删除私有资源统计数据
     */
    fun removeAllByDay(day: String) {
        val query = Query(Criteria.where("day").`is`(day))
        val result = mongoTemplate.remove(query, COLLECTION_NAME)
        logger.info("removeAllByDay day: $day, deleted count: ${result.deletedCount}")
    }

    /**
     * 批量保存私有资源统计数据
     */
    fun saveAll(entities: List<TTbsPrivateDaySummaryEntity>) {
        if (entities.isNotEmpty()) {
            mongoTemplate.insert(entities, COLLECTION_NAME)
            logger.info("save private summary entity size: ${entities.size}")
        }
    }

    /**
     * 根据日期查询私有机器资源统计（按项目和引擎分组汇总，不分页）
     * 用于在成本分摊时从总资源中扣除私有资源部分
     */
    fun findByDay(
        startDate: String,
        endDate: String,
        filterProjectIdNin: Set<String>
    ): List<TTbsPrivateDaySummaryEntity> {
        logger.info("private findByDay startDate: $startDate, endDate: $endDate")

        val criteria = Criteria.where("day").gte(startDate).lte(endDate)
            .and("user").`is`(null)

        if (filterProjectIdNin.isNotEmpty()) {
            criteria.and("project_id").nin(filterProjectIdNin)
        }

        val match = Aggregation.match(criteria)
        val sort = Aggregation.sort(Sort.Direction.DESC, "day", "created_date")
        val group = Aggregation.group("project_id", "engine_code")
            .sum("total_time_with_cpu").`as`("total_time_with_cpu")
            .first("project_id").`as`("project_id")
            .first("engine_code").`as`("engine_code")

        val options = AggregationOptions.Builder().allowDiskUse(true).build()
        val aggregation = Aggregation.newAggregation(match, sort, group).withOptions(options)
        return mongoTemplate.aggregate(aggregation, COLLECTION_NAME, TTbsPrivateDaySummaryEntity::class.java).mappedResults
    }
}
