package ru.atlas404.pcblock.store

import org.junit.Assert.assertEquals
import org.junit.Test

/** Последние стикеры хранятся JSON-массивом (FR-41, D-103); кривое — пусто. */
class StickerRecentsTest {
    @Test
    fun pushAndRead() {
        val kv = MapKv()
        val r = StickerRecents(kv)
        assertEquals(emptyList<String>(), r.list)
        r.push("033")
        r.push("002")
        r.push("033")
        assertEquals(listOf("033", "002"), StickerRecents(kv).list)
        r.clear()
        assertEquals(emptyList<String>(), StickerRecents(kv).list)
    }

    @Test
    fun brokenStore() {
        val kv = MapKv()
        kv.putString(StickerRecents.KEY, "{битый")
        assertEquals(emptyList<String>(), StickerRecents(kv).list)
        kv.putString(StickerRecents.KEY, "[\"033\",5,\"abc\",\"033\",\"010\"]")
        assertEquals(listOf("033", "010"), StickerRecents(kv).list)
    }
}
