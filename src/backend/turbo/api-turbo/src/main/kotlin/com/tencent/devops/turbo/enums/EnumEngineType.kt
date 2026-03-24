package com.tencent.devops.turbo.enums

enum class EnumEngineType(private val engineCode: String) {
    DISTTASK_CC("disttask-cc"),
    DISTTASK_UE4("disttask-ue4"),
    DISTTCC("distcc");

    fun getEngineCode(): String {
        return this.engineCode
    }
}
