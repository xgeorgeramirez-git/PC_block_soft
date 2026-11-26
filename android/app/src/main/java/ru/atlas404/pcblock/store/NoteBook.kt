package ru.atlas404.pcblock.store

import org.json.JSONArray
import org.json.JSONException
import org.json.JSONObject
import ru.atlas404.pcblock.proto.Proto

/** Простое хранилище строк и чисел: в приложении SharedPreferences (Prefs), в тестах карта. */
interface Kv {
    fun getString(key: String): String?
    fun putString(key: String, value: String?)
    fun getInt(key: String, def: Int): Int
    fun putInt(key: String, value: Int)
}

/** Сообщение, принятое компьютером. state — последнее состояние из проверенного отчёта (null — ещё не было). */
data class SavedNote(
    val id: String,
    val child: Long,
    val text: String,
    val showSec: Int,
    val sentAtMs: Long,
    val state: Proto.NoteState? = null,
)

/**
 * Отправленные сообщения (последние 10), выбор «сколько показывать» и звук. Стикер не запоминается:
 * новое сообщение — «Без стикера» (FR-41). Состояние берётся только
 * из отчёта с подписью компьютера и запоминается: через сутки компьютер перестаёт его присылать,
 * а отметка у родителя остаётся.
 */
class NoteBook(private val kv: Kv) {
    companion object {
        const val KEEP = 10
        const val PER_CHILD = 3
        const val NOTES = "notes"
        const val SHOW_SEC = "noteShowSec"
        const val SOUND = "noteSound"
    }

    /** Последний выбранный звук; по умолчанию «Звук 1», «Без звука» (0) тоже запоминается. */
    var sound: Int
        get() = Proto.normalSound(kv.getInt(SOUND, Proto.DEFAULT_SOUND))
        set(v) = kv.putInt(SOUND, Proto.normalSound(v))

    /** Последний выбор показа; по умолчанию «Пока не нажмёт «Понял»» (0). */
    var showSec: Int
        get() = Proto.normalShowSec(kv.getInt(SHOW_SEC, 0))
        set(v) = kv.putInt(SHOW_SEC, Proto.normalShowSec(v))

    fun all(): List<SavedNote> {
        val raw = kv.getString(NOTES) ?: return emptyList()
        return try {
            val arr = JSONArray(raw)
            (0 until arr.length()).mapNotNull { i -> arr.optJSONObject(i)?.let(::fromJson) }
        } catch (e: JSONException) {
            emptyList()
        }
    }

    /** Последние сообщения ребёнка, новые сверху. */
    fun forChild(child: Long): List<SavedNote> =
        all().filter { it.child == child }.sortedByDescending { it.sentAtMs }.take(PER_CHILD)

    fun add(n: SavedNote) {
        save((all().filter { it.id != n.id } + n).sortedBy { it.sentAtMs }.takeLast(KEEP))
    }

    /** Состояния из отчёта. true — что-то поменялось. */
    fun apply(statuses: List<Proto.NoteStatus>): Boolean {
        if (statuses.isEmpty()) return false
        val byId = statuses.associateBy { it.id }
        var changed = false
        val out = all().map { n ->
            val st = byId[n.id]
            if (st != null && st.child == n.child && st.state != n.state) {
                changed = true
                n.copy(state = st.state)
            } else {
                n
            }
        }
        if (changed) save(out)
        return changed
    }

    fun clear() = kv.putString(NOTES, null)

    private fun save(list: List<SavedNote>) {
        val arr = JSONArray()
        for (n in list) arr.put(toJson(n))
        kv.putString(NOTES, arr.toString())
    }

    private fun toJson(n: SavedNote): JSONObject = JSONObject()
        .put("id", n.id)
        .put("child", n.child)
        .put("text", n.text)
        .put("showSec", n.showSec)
        .put("sentAt", n.sentAtMs)
        .put("state", n.state?.wire ?: "")

    private fun fromJson(o: JSONObject): SavedNote? {
        val id = o.optString("id", "")
        if (id.isEmpty()) return null
        return SavedNote(
            id = id,
            child = o.optLong("child", 0),
            text = o.optString("text", ""),
            showSec = Proto.normalShowSec(o.optInt("showSec", 0)),
            sentAtMs = o.optLong("sentAt", 0),
            state = Proto.NoteState.of(o.optString("state", "")),
        )
    }
}
