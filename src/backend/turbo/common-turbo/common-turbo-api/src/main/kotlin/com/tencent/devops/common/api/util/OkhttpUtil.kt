package com.tencent.devops.common.api.util

import com.tencent.devops.common.api.exception.TurboException
import com.tencent.devops.common.api.exception.code.TURBO_THIRDPARTY_SYSTEM_FAIL
import okhttp3.MediaType
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.Request
import okhttp3.RequestBody
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import org.slf4j.LoggerFactory
import org.springframework.http.HttpMethod
import org.springframework.http.HttpStatus
import java.io.File
import java.io.FileOutputStream
import java.util.concurrent.TimeUnit

@Suppress("NestedBlockDepth","MaxLineLength")
object OkhttpUtil {

    private val logger = LoggerFactory.getLogger(OkhttpUtil::class.java)
    private const val MAX_RETRY_COUNT = 30
    // 每次等待60秒
    private const val WAIT_TIME_RETRY = 60
    private val okHttpClient = okhttp3.OkHttpClient.Builder()
        .connectTimeout(3L, TimeUnit.SECONDS)
        .readTimeout(3L, TimeUnit.SECONDS)
        .writeTimeout(3L, TimeUnit.SECONDS)
        .build()

    private fun addHeadersToRequest(requestBuilder: Request.Builder, headers: Map<String, String>) {
        headers.forEach { (key, value) ->
            requestBuilder.addHeader(key, value)
        }
    }

    private fun buildRequestBody(mediaType: MediaType?, body: String): RequestBody {
        return body.toRequestBody(mediaType)
    }

    private fun needSleep(sleepMillis: Long) {
        try {
            Thread.sleep(sleepMillis)
        } catch (e: Exception) {
            logger.error(e.message, e)
        }
    }

    private fun buildRequest(
        method: HttpMethod,
        url: String,
        body: String,
        headers: Map<String, String>
    ): Request {
        val requestBuilder = Request.Builder().url(url)
        when (method) {
            HttpMethod.GET -> requestBuilder.get()
            HttpMethod.DELETE -> requestBuilder.delete()
            HttpMethod.POST, HttpMethod.PUT -> {
                val mediaType = "application/json; charset=utf-8".toMediaType()
                val requestBody = buildRequestBody(mediaType, body)
                when (method) {
                    HttpMethod.POST -> requestBuilder.post(requestBody)
                    HttpMethod.PUT -> requestBuilder.put(requestBody)
                    else -> {
                        throw TurboException(
                            errorCode = TURBO_THIRDPARTY_SYSTEM_FAIL,
                            errorMessage = "http method not correct!"
                        )
                    }
                }
            }
            else -> {
                throw TurboException(
                    errorCode = TURBO_THIRDPARTY_SYSTEM_FAIL,
                    errorMessage = "http method not correct!"
                )
            }
        }
        addHeadersToRequest(requestBuilder, headers)
        return requestBuilder.build()
    }

    private fun executeRequest(
        method: HttpMethod,
        url: String,
        body: String = "",
        queryParam: Map<String, Any> = mutableMapOf(),
        headers: Map<String, String> = mutableMapOf()
    ): String {
        // 查询参数统一优先处理
        val urlWithQueryParam = if (queryParam.isNotEmpty()) {
            "$url?${queryParam.entries.joinToString("&") { (key, value) -> "$key=$value" }}"
        } else {
            url
        }
        val request = buildRequest(method, urlWithQueryParam, body, headers)
        var retryCount = 0
        var responseContent: String
        var response: Response
        do {
            response = okHttpClient.newCall(request).execute()
            responseContent = response.body!!.string()
            if (!response.isSuccessful) {
                retryCount++
                logger.warn("${method.name} request failed, url: $urlWithQueryParam, message: ${response
                    .message}, content:$responseContent")
                logger.warn("start waiting for retry time: %d s", WAIT_TIME_RETRY)
                needSleep(WAIT_TIME_RETRY * 1000L)
            }
        } while (retryCount < MAX_RETRY_COUNT && !response.isSuccessful)
        if (!response.isSuccessful) {
            throw TurboException(
                errorCode = TURBO_THIRDPARTY_SYSTEM_FAIL,
                errorMessage = "Failed to launch third party system."
            )
        }

        return responseContent
    }

    fun doHttpGet(url: String, queryParam: Map<String, Any>, headers: Map<String, String> = mapOf()): String {
        return executeRequest(method = HttpMethod.GET, url= url, queryParam = queryParam, headers = headers)
    }

    fun doHttpPost(url: String, jsonBody: String, headers: Map<String, String> = mapOf()): String {
        return executeRequest(method = HttpMethod.POST, url = url, body = jsonBody, headers = headers)
    }

    fun doHttpPut(url: String, body: String, headers: Map<String, String> = mapOf()): String {
        return executeRequest(method = HttpMethod.PUT, url = url, body = body, headers = headers)
    }

    fun doHttp(request: Request): Response {
        return okHttpClient.newCall(request).execute()
    }

    fun doHttpDelete(url: String, body: String, headers: Map<String, String> = mapOf()): String {
        return executeRequest(HttpMethod.DELETE, url = url, body = body, headers = headers)
    }

    fun downloadFile(url: String, destPath: File) {
        val request = Request.Builder()
            .url(url)
            .get()
            .build()
        okHttpClient.newCall(request).execute().use { response ->
            if (response.code == HttpStatus.NOT_FOUND.value()) {
                logger.warn("The file $url is not exist")
                throw RuntimeException("文件不存在")
            }
            if (!response.isSuccessful) {
                logger.warn("fail to download the file from $url because of ${response.message} and code ${response.code}")
                throw RuntimeException("获取文件失败")
            }
            if (!destPath.parentFile.exists()) destPath.parentFile.mkdirs()
            val buf = ByteArray(4096)
            response.body!!.byteStream().use { bs ->
                var len = bs.read(buf)
                FileOutputStream(destPath).use { fos ->
                    while (len != -1) {
                        fos.write(buf, 0, len)
                        len = bs.read(buf)
                    }
                }
            }
        }
    }

    fun downloadFile(response: Response, destPath: File) {
        if (response.code == HttpStatus.NOT_MODIFIED.value()) {
            logger.info("file is newest, do not download to $destPath")
            return
        }
        if (!response.isSuccessful) {
            logger.warn("fail to download the file because of ${response.message} and code ${response.code}")
            throw RuntimeException("获取文件失败")
        }
        if (!destPath.parentFile.exists()) destPath.parentFile.mkdirs()
        val buf = ByteArray(4096)
        response.body!!.byteStream().use { bs ->
            var len = bs.read(buf)
            FileOutputStream(destPath).use { fos ->
                while (len != -1) {
                    fos.write(buf, 0, len)
                    len = bs.read(buf)
                }
            }
        }
    }
}
