package ru.atlas404.pcblock.proto

/**
 * Сетка стикеров экрана сообщения (FR-41, D-103): «Без стикера» и 7 стикеров, которые родитель выбирал
 * последними (новые первыми); пока выбранных меньше 7 — следующие по порядку номеров. Стикер попадает
 * в последние после принятой компьютером отправки (R-15, ответ noter ok:true).
 */
object RecentStickers {
    const val SIZE = 7

    /** Ровно 7 (если в наборе не меньше): выбранные из набора без повторов, добивка — первые по номеру. */
    fun grid(recent: List<String>, all: List<String>): List<String> {
        val known = all.toSet()
        val chosen = recent.filter { it in known }.distinct().take(SIZE)
        val fill = all.distinct().sorted().filter { it !in chosen }.take(SIZE - chosen.size)
        return chosen + fill
    }

    /** id в начало без повтора, не больше 7; «Без стикера» ("") и кривой номер список не меняют. */
    fun push(recent: List<String>, id: String): List<String> {
        if (id.isEmpty() || !Proto.stickerOk(id)) return recent
        return (listOf(id) + recent.filter { it != id }).take(SIZE)
    }
}
