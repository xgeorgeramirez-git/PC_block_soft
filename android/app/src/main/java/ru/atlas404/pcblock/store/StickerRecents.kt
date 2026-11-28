package ru.atlas404.pcblock.store

import org.json.JSONArray
import org.json.JSONException
import ru.atlas404.pcblock.proto.Proto
import ru.atlas404.pcblock.proto.RecentStickers

/** Последние выбранные стикеры (FR-41, D-103): JSON-массив номеров, новые первыми, не больше 7. */
class StickerRecents(private val kv: Kv) {
    companion object {
        const val KEY = "recentStickers"
    }

    val list: List<String>
        get() {
            val raw = kv.getString(KEY) ?: return emptyList()
            return try {
                val arr = JSONArray(raw)
                (0 until arr.length()).mapNotNull { i -> (arr.opt(i) as? String)?.takeIf { it.isNotEmpty() && Proto.stickerOk(it) } }
                    .distinct().take(RecentStickers.SIZE)
            } catch (e: JSONException) {
                emptyList()
            }
        }

    fun push(id: String) {
        val next = RecentStickers.push(list, id)
        kv.putString(KEY, JSONArray(next).toString())
    }

    fun clear() = kv.putString(KEY, null)
}
