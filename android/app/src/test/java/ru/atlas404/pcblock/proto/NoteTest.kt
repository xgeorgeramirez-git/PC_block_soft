package ru.atlas404.pcblock.proto

import org.json.JSONObject
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Assume.assumeTrue
import org.junit.Test
import ru.atlas404.pcblock.store.Kv
import ru.atlas404.pcblock.store.NoteBook
import ru.atlas404.pcblock.store.SavedNote
import java.util.Random

/** Сообщение с телефона (протокол, раздел 5а; FR-41, S57). */
class NoteTest {
    private val peer = Vectors.peer()

    private fun note(text: String = "Иди ужинать", showSec: Int = 0, seq: Long = 18, sticker: String = "", sound: Int = 1) =
        Proto.noteData(Vectors.PC, Vectors.PHONE, seq, 1763490000000, 3, "AAECAwQFBgcICQoLDA0ODw", text, showSec, sticker, sound)

    private fun pcSigned(json: String) = Crypto.seal(SoftSigner.pc(), json.toByteArray(Charsets.UTF_8))

    private fun noterJson(data: ByteArray, ok: Boolean = true, t: String = "noter", extra: String = "") =
        "{\"v\":1,\"t\":\"$t\",\"pc\":\"pc-vector\",\"phone\":\"phone-vector\",\"seq\":18," +
            "\"cmd\":\"${Crypto.hashData(data)}\",\"ts\":1763490000500,\"ok\":$ok$extra}"

    private fun rejects(block: () -> Unit) {
        try {
            block()
            fail("ожидался отказ")
        } catch (e: ProtoException) {
            // ожидаемо
        }
    }

    // ---------- сборка и подпись ----------

    @Test
    fun noteDataHasProtocolFieldsInOrder() {
        val s = String(note(), Charsets.UTF_8)
        assertEquals(
            "{\"v\":1,\"t\":\"note\",\"pc\":\"pc-vector\",\"phone\":\"phone-vector\",\"seq\":18,\"ts\":1763490000000," +
                "\"child\":3,\"id\":\"AAECAwQFBgcICQoLDA0ODw\",\"text\":\"Иди ужинать\",\"showSec\":0,\"sound\":1}",
            s,
        )
        val o = JSONObject(s)
        assertEquals("Иди ужинать", o.getString("text"))
        // «Без стикера» — поля sticker нет совсем (протокол 5а), звук уходит всегда.
        assertFalse(o.has("sticker"))
    }

    @Test
    fun noteDataWithStickerAndSoundInProtocolOrder() {
        assertEquals(
            "{\"v\":1,\"t\":\"note\",\"pc\":\"pc-vector\",\"phone\":\"phone-vector\",\"seq\":18,\"ts\":1763490000000," +
                "\"child\":3,\"id\":\"AAECAwQFBgcICQoLDA0ODw\",\"text\":\"Иди ужинать\",\"showSec\":30,\"sticker\":\"033\",\"sound\":2}",
            String(note(showSec = 30, sticker = "033", sound = 2), Charsets.UTF_8),
        )
        // «Без звука» — sound 0, поле есть (нет поля компьютер понимает как «Звук 1»).
        val silent = JSONObject(String(note(sound = 0), Charsets.UTF_8))
        assertEquals(0, silent.getInt("sound"))
        assertFalse(silent.has("sticker"))
        // Номер, которого нет в наборе этого телефона, — не ошибка вида (набор компьютера может быть новее).
        assertEquals("999", JSONObject(String(note(sticker = "999", sound = 99), Charsets.UTF_8)).getString("sticker"))
    }

    @Test
    fun noteDataRefusesBadStickerOrSound() {
        val bad = listOf<() -> Unit>(
            { note(sticker = "33") },
            { note(sticker = "0333") },
            { note(sticker = "abc") },
            { note(sticker = " 33") },
            { note(sticker = "\u0660\u0663\u0663") }, // не ASCII-цифры
            { note(sound = -1) },
            { note(sound = 100) },
        )
        for (b in bad) {
            try {
                b()
                fail("ожидался отказ")
            } catch (e: IllegalArgumentException) {
                // ожидаемо
            }
        }
        assertTrue(Proto.stickerOk("033"))
        assertTrue(Proto.stickerOk(""))
        assertFalse(Proto.stickerOk("33"))
    }

    @Test
    fun noteSignatureVerifiesWithOwnKey() {
        val signer = SoftSigner.phone()
        val data = note(showSec = 30)
        val s = Crypto.seal(signer, data)
        assertArrayEquals(data, Crypto.open(Crypto.parsePublic(signer.publicDer), s))
        // Чужой ключ (компьютера) подпись телефона не принимает.
        rejects { Crypto.open(peer.pcPub, s) }
    }

    @Test
    fun noteTextEscapedLikeGo() {
        assertEquals("\"a\\u003cb\\u003e\\u0026\\\"\\\\\"", Proto.goQuote("a<b>&\"\\"))
        assertEquals("\"раз\\nдва\"", Proto.goQuote("раз\nдва"))
        assertEquals("\"\\u2028\\ufffd😀\"", Proto.goQuote(" \uD800😀"))
        val o = JSONObject(String(note("Ужин <готов> & остывает\nбыстрее"), Charsets.UTF_8))
        assertEquals("Ужин <готов> & остывает\nбыстрее", o.getString("text"))
    }

    @Test
    fun noteIdIsSixteenRandomBytes() {
        val a = Proto.newNoteId(Random(1))
        val b = Proto.newNoteId(Random(2))
        assertEquals(16, B64.dec(a).size)
        assertTrue(a != b)
    }

    @Test
    fun noteDataRefusesBadInput() {
        for (bad in listOf<() -> Unit>({ note("") }, { note("x".repeat(301)) }, { note(" a ") }, { note(showSec = 15) })) {
            try {
                bad()
                fail("ожидался отказ")
            } catch (e: IllegalArgumentException) {
                // ожидаемо
            }
        }
    }

    // ---------- проверка текста ----------

    @Test
    fun textLengthAndLines() {
        assertTrue(Proto.noteOk("x".repeat(300)))
        assertFalse(Proto.noteOk("x".repeat(301)))
        assertTrue(Proto.noteOk("  " + "x".repeat(300) + " \n "))
        assertFalse(Proto.noteOk(""))
        assertFalse(Proto.noteOk(" \n\t "))
        assertTrue(Proto.noteOk("1\n2\n3\n4\n5\n6"))
        assertFalse(Proto.noteOk("1\n2\n3\n4\n5\n6\n7"))
        assertTrue(Proto.noteOk("😀".repeat(300))) // знак Unicode, а не половинка UTF-16
        assertFalse(Proto.noteOk("😀".repeat(301)))
        assertEquals(12, Proto.noteLength("  Иди ужинать! \n"))
        assertEquals("раз\nдва", Proto.cleanNote(" раз\r\nдва\u0007 "))
        assertEquals("a b", Proto.cleanNote("a\tb"))
        // \r\n считается одним переводом строки.
        assertTrue(Proto.noteOk("1\r\n2\r\n3\r\n4\r\n5\r\n6"))
    }

    @Test
    fun invisibleCharsRemovedLikeGo() {
        // Как NoteText в Go: Cf (U+200B, U+200E, U+200F, U+FEFF, U+2060, U+00AD, U+202E, тег U+E0001)
        // убраны, кроме U+200D; знак замены U+FFFD и одинокая половинка UTF-16 — тоже.
        val invisible = "\u200b\u200e\u200f\ufeff\u2060\u00ad\u202e" + String(Character.toChars(0xE0001)) + "\ufffd\ud800 \n"
        assertEquals("", Proto.cleanNote(invisible))
        assertFalse(Proto.noteOk(invisible))
        assertEquals(Problem.NOTE_RANGE, Proto.noteProblem(invisible))
        assertEquals("Текст от 1 до 300 знаков", Strings[Problem.NOTE_RANGE.resName])
        assertEquals("Иди ужинать", Proto.cleanNote("\u200bИди\u2060 ужинать\ufeff"))
        assertEquals("👨\u200d👩", Proto.cleanNote("\u200e👨\u200d👩"))
        assertNull(Proto.noteProblem("Иди ужинать"))
        // Невидимые знаки не считаются в 300.
        assertTrue(Proto.noteOk("x".repeat(300) + "\u200b".repeat(20)))
    }

    // ---------- набор стикеров и звуков (файлы assets, не константа) ----------

    @Test
    fun stickerAndSoundSetsComeFromFileNames() {
        assertEquals(
            listOf("001", "017", "033", "113"),
            Proto.stickerIds(arrayOf("033.tgs", "033.png", "README.md", "113.tgs", "001.tgs", "17.tgs", "017.tgs", "abc.tgs", "0333.tgs")),
        )
        assertEquals(emptyList<String>(), Proto.stickerIds(null))
        assertEquals(
            listOf(1, 2, 5, 10, 99),
            Proto.soundIds(arrayOf("10.mp3", "2.mp3", "README.md", "1.mp3", "0.mp3", "100.mp3", "5.mp3", "x.mp3", "99.mp3", "3.wav", "01.mp3")),
        )
        assertEquals(emptyList<Int>(), Proto.soundIds(null))
    }

    /**
     * Папка assets репозитория (её берёт APK, app/build.gradle.kts): у каждого стикера есть превью,
     * «Звук 1» (звук по умолчанию) есть. Число не зашито: новый файл — новый стикер или звук (D-101).
     */
    @Test
    fun repoAssetsHaveStickersWithPreviewsAndSounds() {
        val root = listOf("../../assets", "../assets", "assets").map { java.io.File(it) }
            .firstOrNull { java.io.File(it, "stickers").isDirectory } ?: error("папка assets не найдена")
        val stickers = Proto.stickerIds(java.io.File(root, "stickers").list())
        assertTrue(stickers.size > Proto.STICKER_GRID)
        for (id in stickers) assertTrue("нет превью $id.png", java.io.File(root, "stickers/$id.png").isFile)
        val sounds = Proto.soundIds(java.io.File(root, "sounds/message").list())
        assertEquals(Proto.DEFAULT_SOUND, sounds.first())
    }

    // ---------- повтор после обрыва ----------

    @Test
    fun retryWithoutAnswerKeepsSameId() {
        val r = NoteRetry()
        var n = 0
        val gen = { "id${++n}" }
        val a = r.idFor(3, "Иди ужинать", 0, "", 1, gen)
        // Ответа не было (обрыв сети): повтор того же сообщения — тот же id.
        assertEquals(a, r.idFor(3, "Иди ужинать", 0, "", 1, gen))
        assertEquals(a, r.idFor(3, "Иди ужинать", 0, "", 1, gen))
        // Другой текст, ребёнок или показ — другое сообщение.
        val b = r.idFor(3, "Иди ужинать!", 0, "", 1, gen)
        assertTrue(a != b)
        val c = r.idFor(4, "Иди ужинать!", 0, "", 1, gen)
        assertTrue(b != c)
        val d = r.idFor(4, "Иди ужинать!", 30, "", 1, gen)
        assertTrue(c != d)
        // Компьютер ответил: следующее такое же — новое сообщение.
        r.answered()
        val e = r.idFor(4, "Иди ужинать!", 30, "", 1, gen)
        assertTrue(d != e)
        assertEquals(5, n)
    }

    @Test
    fun retrySameIdOnlyWithSameStickerAndSound() {
        val r = NoteRetry()
        var n = 0
        val gen = { "id${++n}" }
        val a = r.idFor(3, "Иди ужинать", 0, "033", 2, gen)
        assertEquals(a, r.idFor(3, "Иди ужинать", 0, "033", 2, gen))
        // Другой стикер — другое сообщение.
        val b = r.idFor(3, "Иди ужинать", 0, "017", 2, gen)
        assertTrue(a != b)
        // «Без стикера» — тоже другое.
        val c = r.idFor(3, "Иди ужинать", 0, "", 2, gen)
        assertTrue(b != c)
        // Другой звук — другое сообщение.
        val d = r.idFor(3, "Иди ужинать", 0, "", 0, gen)
        assertTrue(c != d)
        assertEquals(d, r.idFor(3, "Иди ужинать", 0, "", 0, gen))
        assertEquals(4, n)
    }

    // ---------- ответ noter ----------

    @Test
    fun noterAcceptedForOwnNote() {
        val data = note()
        val r = Proto.openNoteReply(peer, pcSigned(noterJson(data)), data)
        assertTrue(r.ok)
        assertEquals("", r.error)
    }

    @Test
    fun noterRefusalCarriesErrorAndLastSeq() {
        val data = note()
        val r = Proto.openNoteReply(peer, pcSigned(noterJson(data, ok = false, extra = ",\"error\":\"replay\",\"lastSeq\":40")), data)
        assertFalse(r.ok)
        assertEquals("replay", r.error)
        assertEquals(40L, r.lastSeq)
        assertEquals("Текст от 1 до 300 знаков", Strings[Problem.fromNote("range").resName])
        assertEquals(Problem.STALE, Problem.fromNote("replay"))
    }

    @Test
    fun noterWithForeignHashRejected() {
        val other = note("Другое сообщение")
        rejects { Proto.openNoteReply(peer, pcSigned(noterJson(other)), note()) }
    }

    @Test
    fun noterOfWrongTypeOrKeyRejected() {
        val data = note()
        rejects { Proto.openNoteReply(peer, pcSigned(noterJson(data, t = "cmdr")), data) }
        rejects { Proto.openNoteReply(peer, Crypto.seal(SoftSigner.phone(), noterJson(data).toByteArray()), data) }
        rejects { Proto.openNoteReply(peer, pcSigned(noterJson(data).replace("phone-vector", "phone-other")), data) }
        rejects { Proto.openNoteReply(peer, pcSigned(noterJson(data).replace("pc-vector", "pc-other")), data) }
    }

    // ---------- состояния в отчёте ----------

    private val reportWithNotes =
        "{\"v\":1,\"t\":\"report\",\"pc\":\"pc-vector\",\"phone\":\"phone-vector\",\"ts\":1763490002000," +
            "\"children\":[{\"id\":3,\"name\":\"Миша\",\"usedSec\":3600,\"leftSec\":1800,\"extraMin\":0,\"blocked\":false}]," +
            "\"notes\":[{\"id\":\"n1\",\"child\":3,\"state\":\"queued\",\"atMs\":1}," +
            "{\"id\":\"n2\",\"child\":3,\"state\":\"shown\",\"atMs\":2}," +
            "{\"id\":\"n3\",\"child\":3,\"state\":\"read\",\"atMs\":3}," +
            "{\"id\":\"n4\",\"child\":3,\"state\":\"expired\",\"atMs\":4}," +
            "{\"id\":\"n5\",\"child\":3,\"state\":\"lost\",\"atMs\":5}," +
            "{\"child\":3,\"state\":\"read\"}]}"

    @Test
    fun reportNotesParsedFromSignedReport() {
        val rep = Proto.openReport(peer, pcSigned(reportWithNotes), 0)
        assertEquals(
            listOf(
                Proto.NoteStatus("n1", 3, Proto.NoteState.QUEUED, 1),
                Proto.NoteStatus("n2", 3, Proto.NoteState.SHOWN, 2),
                Proto.NoteStatus("n3", 3, Proto.NoteState.READ, 3),
                Proto.NoteStatus("n4", 3, Proto.NoteState.EXPIRED, 4),
            ),
            rep.notes,
        )
        // Подделанное сервером состояние без подписи компьютера не принимается.
        val s = pcSigned(reportWithNotes)
        val forged = reportWithNotes.replace("\"queued\"", "\"read\"")
        rejects { Proto.openReport(peer, Signed(B64.enc(forged.toByteArray()), s.sig), 0) }
        // Отчёт без notes (компьютер 0.8.1) — пустой список.
        assertEquals(emptyList<Proto.NoteStatus>(), Proto.openReport(peer, Vectors.signed("report"), 0).notes)
    }

    @Test
    fun marksFromStates() {
        assertEquals(NoteMark.SENT, NoteMark.of(null, 0))
        assertEquals(NoteMark.SENT, NoteMark.of(Proto.NoteState.QUEUED, 0))
        assertEquals(NoteMark.DELIVERED, NoteMark.of(Proto.NoteState.SHOWN, 10))
        assertEquals(NoteMark.READ, NoteMark.of(Proto.NoteState.READ, 0))
        assertEquals(NoteMark.SHOWN, NoteMark.of(Proto.NoteState.READ, 10))
        assertEquals(NoteMark.NOT_DELIVERED, NoteMark.of(Proto.NoteState.EXPIRED, 60))
        val texts = NoteMark.values().map { Strings[it.resName] }
        assertEquals(listOf("Отправлено", "Ждёт: компьютер не в сети", "Доставлено", "Прочитано", "Показано", "Не доставлено"), texts)
    }

    @Test
    fun offlineTextsForNote() {
        assertEquals(Problem.NOTE_PC_OFFLINE, Problem.fromRelayNote("pc_offline"))
        assertEquals(Problem.BUSY, Problem.fromRelayNote("busy"))
        assertEquals("Компьютер не на связи с 19:05. Сообщение не отправлено", Strings["err_note_pc_offline_since"].format("19:05"))
        assertEquals("12 из 300", Strings["note_counter"].format(12, 300))
    }

    // ---------- вектор Go ----------

    @Test
    fun goNoteVectorMatches() {
        assumeTrue("вектора note в vectors.json ещё нет", Vectors.json.has("noteJson") && Vectors.json.has("note"))
        val goBytes = Vectors.s("noteJson").toByteArray(Charsets.UTF_8)
        // Подпись Go проверяется ключом телефона, байты совпадают.
        assertArrayEquals(goBytes, Crypto.open(Crypto.parsePublicB64(Vectors.s("phonePub")), Vectors.signed("note")))
        // Вектор 0.8.2 без sticker и sound: телефон 0.8.3 шлёт звук всегда, поэтому те же поля дают
        // те же байты плюс «"sound":1» (нет поля — тот же «Звук 1», протокол 5а).
        val o = JSONObject(Vectors.s("noteJson"))
        assertFalse(o.has("sticker") || o.has("sound"))
        val ours = Proto.noteData(
            o.getString("pc"), o.getString("phone"), o.getLong("seq"), o.getLong("ts"), o.getLong("child"),
            o.getString("id"), o.getString("text"), o.getInt("showSec"), "", 1,
        )
        assertEquals(Vectors.s("noteJson").removeSuffix("}") + ",\"sound\":1}", String(ours, Charsets.UTF_8))
        if (Vectors.json.has("noteHash")) assertEquals(Vectors.s("noteHash"), Crypto.hashData(goBytes))
        if (Vectors.json.has("noter")) {
            val r = Proto.openNoteReply(Vectors.peer(), Vectors.signed("noter"), goBytes)
            assertTrue(r.ok)
        }
    }

    // ---------- хранение на телефоне ----------

    private class MapKv : Kv {
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

    @Test
    fun showChoiceRemembered() {
        val kv = MapKv()
        assertEquals(0, NoteBook(kv).showSec) // по умолчанию «Пока не нажмёт «Понял»»
        NoteBook(kv).showSec = 30
        assertEquals(30, NoteBook(kv).showSec)
        NoteBook(kv).showSec = 15
        assertEquals(0, NoteBook(kv).showSec)
        kv.putInt(NoteBook.SHOW_SEC, 999)
        assertEquals(0, NoteBook(kv).showSec)
    }

    @Test
    fun soundChoiceRemembered() {
        val kv = MapKv()
        assertEquals(1, NoteBook(kv).sound) // по умолчанию «Звук 1»
        NoteBook(kv).sound = 0
        assertEquals(0, NoteBook(kv).sound) // «Без звука» тоже запоминается
        NoteBook(kv).sound = 4
        assertEquals(4, NoteBook(kv).sound)
        assertEquals(4, kv.getInt(NoteBook.SOUND, -1))
        kv.putInt(NoteBook.SOUND, 100)
        assertEquals(1, NoteBook(kv).sound)
        kv.putInt(NoteBook.SOUND, -1)
        assertEquals(1, NoteBook(kv).sound)
        assertEquals("noteSound", NoteBook.SOUND)
    }

    @Test
    fun keepsLastTenAndThreePerChild() {
        val kv = MapKv()
        val book = NoteBook(kv)
        for (i in 1..12) book.add(SavedNote("n$i", if (i % 2 == 0) 3 else 4, "t$i", 0, i * 1000L))
        val all = NoteBook(kv).all()
        assertEquals(10, all.size)
        assertEquals("n3", all.first().id)
        assertEquals(listOf("n12", "n10", "n8"), book.forChild(3).map { it.id })
    }

    @Test
    fun statesComeFromReportAndStay() {
        val kv = MapKv()
        val book = NoteBook(kv)
        book.add(SavedNote("n1", 3, "Иди ужинать", 10, 1000))
        book.add(SavedNote("n2", 3, "Уроки", 0, 2000))
        assertTrue(book.apply(listOf(Proto.NoteStatus("n1", 3, Proto.NoteState.READ, 5))))
        assertFalse(book.apply(listOf(Proto.NoteStatus("n1", 3, Proto.NoteState.READ, 5))))
        // Состояние чужого ребёнка с тем же id не применяется.
        assertFalse(book.apply(listOf(Proto.NoteStatus("n2", 4, Proto.NoteState.EXPIRED, 5))))
        val byId = NoteBook(kv).all().associateBy { it.id }
        assertEquals(NoteMark.SHOWN, NoteMark.of(byId["n1"]!!.state, byId["n1"]!!.showSec))
        assertNull(byId["n2"]!!.state)
        // Через сутки компьютер не присылает n1: отметка остаётся.
        assertFalse(book.apply(emptyList()))
        assertEquals(Proto.NoteState.READ, NoteBook(kv).all().first { it.id == "n1" }.state)
    }

    /** «Ждёт: компьютер не в сети» (FB-29, D-102): только пока отметки от компьютера нет и он не на связи. */
    @Test
    fun waitingOfflineMark() {
        assertEquals(NoteMark.WAITING_OFFLINE, NoteMark.of(null, 0, pcOnline = false))
        assertEquals(NoteMark.WAITING_OFFLINE, NoteMark.of(null, 30, pcOnline = false))
        assertEquals(NoteMark.SENT, NoteMark.of(null, 0, pcOnline = true))
        assertEquals(NoteMark.SENT, NoteMark.of(Proto.NoteState.QUEUED, 0, pcOnline = false))
        assertEquals(NoteMark.DELIVERED, NoteMark.of(Proto.NoteState.SHOWN, 0, pcOnline = false))
        assertEquals(NoteMark.READ, NoteMark.of(Proto.NoteState.READ, 0, pcOnline = false))
        assertEquals(NoteMark.SHOWN, NoteMark.of(Proto.NoteState.READ, 10, pcOnline = false))
        assertEquals(NoteMark.NOT_DELIVERED, NoteMark.of(Proto.NoteState.EXPIRED, 0, pcOnline = false))
        assertEquals("Ждёт: компьютер не в сети", Strings[NoteMark.WAITING_OFFLINE.resName])
        for (m in NoteMark.values()) assertTrue(m.resName, Strings.all.containsKey(m.resName))
    }
}
