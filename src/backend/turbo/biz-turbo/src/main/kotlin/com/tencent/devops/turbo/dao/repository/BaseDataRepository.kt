package com.tencent.devops.turbo.dao.repository

import com.tencent.devops.turbo.model.BaseDataEntity
import org.springframework.data.mongodb.repository.MongoRepository
import org.springframework.stereotype.Repository

@Repository
interface BaseDataRepository : MongoRepository<BaseDataEntity, String> {

    /**
     * 根据参数标识代码查询
     */
    fun findFirstByParamCode(paramCode: String): BaseDataEntity?
}
