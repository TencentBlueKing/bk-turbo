package com.tencent.devops.turbo.model

import org.springframework.data.mongodb.core.index.Indexed
import org.springframework.data.mongodb.core.mapping.Document
import org.springframework.data.mongodb.core.mapping.Field
import java.time.LocalDateTime


@Document(collection = "t_base_data")
data class BaseDataEntity(
    /**
     * 唯一标识
     */
    @Indexed
    @Field("param_code")
    val paramCode: String,

    /**
     * 参数名称、描述等信息
     */
    @Field("param_name")
    val paramName: String,

    /**
     * 参数值
     */
    @Field("param_value")
    val paramValue: String,

    @Field("param_type")
    val paramType: String? = null,

    @Field("param_status")
    val paramStatus: String? = null,

    @Field("updated_by")
    var updatedBy: String,
    @Field("updated_date")
    var updatedDate: LocalDateTime,
    @Field("created_by")
    var createdBy: String,
    @Field("created_date")
    var createdDate: LocalDateTime
)
