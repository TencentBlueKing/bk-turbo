package com.tencent.devops.common.util

import java.text.DecimalFormat

object MathUtil {

    fun roundToTwoDigits(input: Double): String {
        return String.format("%.2f", input)
    }

    /**
     * 秒转分钟
     */
    fun secondsToMinutes(seconds: Double): String {
        val minutes = seconds.toDouble() / 60
        val df = DecimalFormat("#.##")
        return df.format(minutes)
    }
}
