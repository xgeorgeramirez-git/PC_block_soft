package ru.atlas404.pcblock.proto

/**
 * Разбор «Статистики за сегодня» для экрана (FR-43): блок «Во что играл» — всё с вкладкой game
 * (программы и сайты), дальше прочие программы и сайты; каждый список по убыванию времени.
 */
data class TodayView(val games: List<Proto.Item>, val programs: List<Proto.Item>, val sites: List<Proto.Item>) {
    companion object {
        const val GAME = "game"

        fun of(t: Proto.Today): TodayView {
            val byTime = compareByDescending<Proto.Item> { it.sec }.thenBy { it.name }
            return TodayView(
                games = (t.programs + t.sites).filter { it.cat == GAME }.sortedWith(byTime),
                programs = t.programs.filter { it.cat != GAME }.sortedWith(byTime),
                sites = t.sites.filter { it.cat != GAME }.sortedWith(byTime),
            )
        }
    }
}
