package ru.atlas404.pcblock.proto

import org.json.JSONException
import org.json.JSONObject

/**
 * Названия звуков сообщения (FR-41, D-104): assets/sounds/message/names.json — {"1":"Поляк",…}.
 * Номер без названия — «Звук N» (шаблон note_sound_n из strings.xml).
 */
object SoundNames {
    /** Номер 1…99 → название без краевых пробелов; кривой файл — пусто, кривые строки пропускаются. */
    fun parse(json: String): Map<Int, String> {
        val t = json.trim()
        if (!t.startsWith("{")) return emptyMap()
        val o = try {
            JSONObject(t)
        } catch (e: JSONException) {
            return emptyMap()
        }
        val out = HashMap<Int, String>()
        for (k in o.keys()) {
            val n = k.toIntOrNull() ?: continue
            if (n !in 1..Proto.MAX_SOUND || k != n.toString()) continue
            val name = (o.opt(k) as? String)?.trim().orEmpty()
            if (name.isNotEmpty()) out[n] = name
        }
        return out
    }

    /** Название звука или «Звук N» по шаблону fallbackTpl. */
    fun label(n: Int, names: Map<Int, String>, fallbackTpl: String): String =
        names[n] ?: String.format(java.util.Locale.ROOT, fallbackTpl, n)
}
