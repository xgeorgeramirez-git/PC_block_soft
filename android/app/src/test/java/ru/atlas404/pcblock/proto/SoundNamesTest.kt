package ru.atlas404.pcblock.proto

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/** Названия звуков сообщения (FR-41, D-104): assets/sounds/message/names.json, нет названия — «Звук N». */
class SoundNamesTest {
    private val tpl = Strings["note_sound_n"]

    private val namesFile: File by lazy {
        listOf("../../assets/sounds/message/names.json", "../assets/sounds/message/names.json", "assets/sounds/message/names.json")
            .map(::File).firstOrNull { it.exists() } ?: error("names.json не найден")
    }

    @Test
    fun customerNames() {
        val names = SoundNames.parse(namesFile.readText(Charsets.UTF_8))
        assertEquals("Поляк", SoundNames.label(1, names, tpl))
        assertEquals("Клоун", SoundNames.label(3, names, tpl))
        assertEquals("Смех", SoundNames.label(12, names, tpl))
        assertEquals("Звук 13", SoundNames.label(13, names, tpl))
        // У каждого mp3 есть название (набор пополняется вместе с names.json).
        val mp3 = Proto.soundIds(namesFile.parentFile.list())
        assertTrue(mp3.isNotEmpty())
        for (n in mp3) assertTrue("нет названия звука $n", names.containsKey(n))
    }

    @Test
    fun brokenIsEmpty() {
        assertTrue(SoundNames.parse("").isEmpty())
        assertTrue(SoundNames.parse("{битый").isEmpty())
        assertTrue(SoundNames.parse("[1]").isEmpty())
        assertEquals("Звук 2", SoundNames.label(2, emptyMap(), tpl))
    }

    @Test
    fun badEntriesSkipped() {
        val names = SoundNames.parse("{\"1\":\"  Гонг \",\"0\":\"Ноль\",\"100\":\"Сто\",\"x\":\"Икс\",\"2\":\"\",\"3\":5,\"4\":\"Свист\"}")
        assertEquals(mapOf(1 to "Гонг", 4 to "Свист"), names)
    }
}
