package ru.atlas404.pcblock.store

import org.json.JSONException
import org.json.JSONObject

/**
 * Последний список «Промокодов» с компьютера (FR-45, протокол, раздел 5г): raw — JSON вида поля promos
 * (Proto.promosJson / Proto.parsePromos), ts — время ответа компьютера («данные на HH:MM»). Компьютер
 * не на связи — экран показывает этот список. Одна запись: список и время меняются вместе.
 */
class PromoCache(private val kv: Kv) {
    companion object {
        const val KEY = "promos"
    }

    private fun read(): Pair<String, Long> {
        val s = kv.getString(KEY) ?: return "" to 0L
        return try {
            val o = JSONObject(s)
            o.optString("raw", "") to o.optLong("ts", 0)
        } catch (e: JSONException) {
            "" to 0L
        }
    }

    val raw: String get() = read().first
    val ts: Long get() = read().second

    fun save(raw: String, ts: Long) = kv.putString(KEY, JSONObject().put("raw", raw).put("ts", ts).toString())

    fun clear() = kv.putString(KEY, null)
}
