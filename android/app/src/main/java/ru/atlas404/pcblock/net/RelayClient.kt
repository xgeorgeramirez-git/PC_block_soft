package ru.atlas404.pcblock.net

import org.json.JSONException
import org.json.JSONObject
import ru.atlas404.pcblock.proto.Proto
import ru.atlas404.pcblock.proto.Signed
import java.io.ByteArrayOutputStream
import java.io.IOException
import java.io.InputStream
import java.net.HttpURLConnection
import java.net.URI
import java.net.URL

/**
 * Ошибка обмена с сервером связи. code — код протокола (раздел 3: bad_code, too_many, pc_offline,
 * busy, unauthorized, …) или свой: network (нет связи), bad_response (ответ не разобран),
 * bad_address (адрес не https).
 */
class RelayException(val code: String) : Exception(code)

/** Конверт сервера {"id","from","kind","body"}. */
data class Msg(val id: String, val from: String, val kind: String, val body: JSONObject?)

data class PairResult(val pcId: String, val phoneId: String, val phoneToken: String, val reply: Msg)

data class Status(val serverNowMs: Long, val pcLastSeenMs: Long, val report: Signed?)

/**
 * Клиент сервера связи для телефона (протокол, раздел 3). Только HTTPS, тайм-ауты 15 с,
 * ответ не больше 64 КБ. Вызывать не из главного потока.
 */
class RelayClient(private val base: String) {
    companion object {
        const val TIMEOUT_MS = 15_000
        const val MAX_BODY = 64 * 1024

        const val NETWORK = "network"
        const val BAD_RESPONSE = "bad_response"
        const val BAD_ADDRESS = "bad_address"
        const val UNAUTHORIZED = "unauthorized"
        const val PC_OFFLINE = "pc_offline"
    }

    /** phone/pair: первое сообщение привязки; ответ компьютера — внутри. */
    fun pair(code: String, body: JSONObject): PairResult {
        val o = call("POST", "/v1/phone/pair", null, JSONObject().put("code", code).put("kind", Proto.KIND_PAIR1).put("body", body))
        return try {
            PairResult(
                pcId = o.getString("pcId"),
                phoneId = o.getString("phoneId"),
                phoneToken = o.getString("phoneToken"),
                reply = msg(o.getJSONObject("reply")),
            )
        } catch (e: JSONException) {
            throw RelayException(BAD_RESPONSE)
        }
    }

    /** phone/send: сообщение компьютеру и его ответ (сервер ждёт до 10 с). */
    fun send(token: String, kind: String, body: JSONObject): Msg {
        val o = call("POST", "/v1/phone/send", token, JSONObject().put("kind", kind).put("body", body))
        return try {
            msg(o.getJSONObject("reply"))
        } catch (e: JSONException) {
            throw RelayException(BAD_RESPONSE)
        }
    }

    /** phone/status: когда компьютер был на связи и его последний подписанный отчёт. */
    fun status(token: String): Status {
        val o = call("GET", "/v1/phone/status", token, null)
        return try {
            val rep = if (o.isNull("report")) null else Proto.signedFromJson(o.optJSONObject("report"))
            Status(o.optLong("serverNowMs", 0), o.optLong("pcLastSeenMs", 0), rep)
        } catch (e: Exception) {
            throw RelayException(BAD_RESPONSE)
        }
    }

    private fun msg(o: JSONObject) = Msg(
        id = o.optString("id", ""),
        from = o.optString("from", ""),
        kind = o.optString("kind", ""),
        body = o.optJSONObject("body"),
    )

    private fun call(method: String, path: String, token: String?, body: JSONObject?): JSONObject {
        val url = try {
            URI(base + path).toURL()
        } catch (e: Exception) {
            throw RelayException(BAD_ADDRESS)
        }
        if (url.protocol != "https") throw RelayException(BAD_ADDRESS)
        val conn = try {
            url.openConnection() as HttpURLConnection
        } catch (e: IOException) {
            throw RelayException(NETWORK)
        }
        try {
            conn.requestMethod = method
            conn.connectTimeout = TIMEOUT_MS
            conn.readTimeout = TIMEOUT_MS
            conn.instanceFollowRedirects = false
            conn.useCaches = false
            conn.setRequestProperty("Accept", "application/json")
            if (token != null) conn.setRequestProperty("Authorization", "Bearer $token")
            if (body != null) {
                val bytes = body.toString().toByteArray(Charsets.UTF_8)
                conn.doOutput = true
                conn.setRequestProperty("Content-Type", "application/json")
                conn.setFixedLengthStreamingMode(bytes.size)
                conn.outputStream.use { it.write(bytes) }
            }
            val status = conn.responseCode
            val stream = if (status in 200..299) conn.inputStream else conn.errorStream
            val text = stream?.use { readLimited(it) } ?: ""
            val obj = try {
                if (text.isBlank()) JSONObject() else JSONObject(text)
            } catch (e: JSONException) {
                if (status in 200..299) throw RelayException(BAD_RESPONSE) else JSONObject()
            }
            if (status in 200..299) return obj
            throw RelayException(obj.optString("error", "").ifEmpty { codeFor(status) })
        } catch (e: RelayException) {
            throw e
        } catch (e: IOException) {
            throw RelayException(NETWORK)
        } catch (e: RuntimeException) {
            // Системный стек может бросить SecurityException и т. п.: для родителя это «нет связи».
            throw RelayException(NETWORK)
        } finally {
            conn.disconnect()
        }
    }

    private fun readLimited(input: InputStream): String {
        val out = ByteArrayOutputStream()
        val buf = ByteArray(8192)
        while (true) {
            val n = input.read(buf)
            if (n < 0) break
            if (out.size() + n > MAX_BODY) throw RelayException(BAD_RESPONSE)
            out.write(buf, 0, n)
        }
        return out.toString("UTF-8")
    }

    private fun codeFor(status: Int): String = when (status) {
        400 -> "bad_request"
        401 -> UNAUTHORIZED
        403 -> "bad_code"
        404 -> "not_found"
        413 -> "too_large"
        429 -> "too_many"
        // 5xx без кода протокола — отвечал не сервер связи, а прокси перед ним: «нет связи».
        else -> if (status >= 500) NETWORK else BAD_RESPONSE
    }
}
