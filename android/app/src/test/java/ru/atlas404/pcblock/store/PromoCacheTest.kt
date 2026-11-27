package ru.atlas404.pcblock.store

import org.junit.Assert.assertEquals
import org.junit.Test
import ru.atlas404.pcblock.proto.Proto

/** Хранилище в памяти для тестов (в приложении — SharedPreferences, Prefs). */
class MapKv : Kv {
    val m = HashMap<String, Any>()
    override fun getString(key: String): String? = m[key] as? String
    override fun putString(key: String, value: String?) {
        if (value == null) m.remove(key) else m[key] = value
    }
    override fun getInt(key: String, def: Int): Int = m[key] as? Int ?: def
    override fun putInt(key: String, value: Int) {
        m[key] = value
    }
}

/** Кэш «Промокодов» (FR-45): последний список и время ответа для «данные на HH:MM» и офлайна. */
class PromoCacheTest {
    @Test
    fun keepsLastListAndTime() {
        val kv = MapKv()
        val c = PromoCache(kv)
        assertEquals("", c.raw)
        assertEquals(0L, c.ts)
        val list = Proto.PromoList(listOf(Proto.PromoActive("SUMMER", 30, 5)), emptyList(), 0, 0)
        c.save(Proto.promosJson(list), 1763490009500)
        val again = PromoCache(kv)
        assertEquals(list, Proto.parsePromos(again.raw))
        assertEquals(1763490009500, again.ts)
        again.clear()
        assertEquals("", PromoCache(kv).raw)
        assertEquals(0L, PromoCache(kv).ts)
    }

    @Test
    fun brokenStoreIsEmpty() {
        val kv = MapKv()
        kv.putString(PromoCache.KEY, "{битый")
        assertEquals("", PromoCache(kv).raw)
        assertEquals(0L, PromoCache(kv).ts)
    }
}
