package ru.atlas404.pcblock.proto

import org.junit.Assert.assertEquals
import org.junit.Test

/** «Без стикера» + 7 последних (FR-41, D-103): новые первыми, добивка — первые по номеру. */
class RecentStickersTest {
    private val all = (1..74).map { it.toString().padStart(3, '0') }

    @Test
    fun emptyGivesFirstSeven() {
        assertEquals(listOf("001", "002", "003", "004", "005", "006", "007"), RecentStickers.grid(emptyList(), all))
    }

    @Test
    fun chosenFirstThenFill() {
        assertEquals(listOf("033", "002", "050", "001", "003", "004", "005"), RecentStickers.grid(listOf("033", "002", "050"), all))
    }

    @Test
    fun unknownAndRepeatsDropped() {
        assertEquals(
            listOf("033", "001", "002", "003", "004", "005", "006"),
            RecentStickers.grid(listOf("999", "033", "033", "abc", ""), all),
        )
    }

    @Test
    fun allUnsortedIsSortedForFill() {
        assertEquals(listOf("003", "001", "002"), RecentStickers.grid(listOf("003"), listOf("002", "003", "001")))
    }

    @Test
    fun pushRaisesAndCuts() {
        var r = emptyList<String>()
        for (id in listOf("001", "002", "003", "004", "005", "006", "007", "008")) r = RecentStickers.push(r, id)
        assertEquals(listOf("008", "007", "006", "005", "004", "003", "002"), r)
        r = RecentStickers.push(r, "005")
        assertEquals(listOf("005", "008", "007", "006", "004", "003", "002"), r)
        // «Без стикера» и кривой номер в последние не попадают.
        assertEquals(r, RecentStickers.push(r, ""))
        assertEquals(r, RecentStickers.push(r, "12"))
    }

    @Test
    fun gridOfSevenChosen() {
        val chosen = listOf("070", "060", "050", "040", "030", "020", "010")
        assertEquals(chosen, RecentStickers.grid(chosen + "001", all))
    }
}
