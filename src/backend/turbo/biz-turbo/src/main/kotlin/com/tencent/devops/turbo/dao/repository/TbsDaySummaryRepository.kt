package com.tencent.devops.turbo.dao.repository

import com.tencent.devops.turbo.model.TTbsDaySummaryEntity
import org.springframework.data.mongodb.repository.MongoRepository
import org.springframework.stereotype.Repository

@Repository
interface TbsDaySummaryRepository : MongoRepository<TTbsDaySummaryEntity, String>
