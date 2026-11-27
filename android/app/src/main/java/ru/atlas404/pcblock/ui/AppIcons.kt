package ru.atlas404.pcblock.ui

import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.graphics.vector.addPathNodes
import androidx.compose.ui.unit.dp

/**
 * Линейные значки телефона (ТЗ 8.0 п. 4): один встроенный набор, пути из макета (plans/design-0.8.4/kit2.py),
 * штрих 1,8 по сетке 24. Цвет задаёт tint у Icon. Монета копилки — цветная (tint = Color.Unspecified).
 */
object AppIcons {
    private fun circle(cx: Float, cy: Float, r: Float): String =
        "M${cx - r} ${cy}a$r $r 0 1 0 ${2 * r} 0a$r $r 0 1 0 ${-2 * r} 0"

    private fun rect(x: Float, y: Float, w: Float, h: Float, r: Float): String =
        "M${x + r} ${y}h${w - 2 * r}a$r $r 0 0 1 $r ${r}v${h - 2 * r}a$r $r 0 0 1 ${-r} ${r}" +
            "h${-(w - 2 * r)}a$r $r 0 0 1 ${-r} ${-r}v${-(h - 2 * r)}a$r $r 0 0 1 $r ${-r}z"

    private fun line(name: String, vararg paths: String, width: Float = 1.8f): ImageVector {
        val b = ImageVector.Builder(
            name = name,
            defaultWidth = 24.dp,
            defaultHeight = 24.dp,
            viewportWidth = 24f,
            viewportHeight = 24f,
        )
        for (d in paths) {
            b.addPath(
                pathData = addPathNodes(d),
                fill = null,
                stroke = SolidColor(Color.Black),
                strokeLineWidth = width,
                strokeLineCap = StrokeCap.Round,
                strokeLineJoin = StrokeJoin.Round,
            )
        }
        return b.build()
    }

    val Kids: ImageVector by lazy {
        line(
            "kids",
            circle(9f, 8f, 3.5f),
            "M2.5 20c0.6 -3.6 3.2 -5.5 6.5 -5.5s5.9 1.9 6.5 5.5M16 4.8a3.2 3.2 0 0 1 0 6.4M18 14.6c2 0.6 3.2 2.4 3.5 5.4",
        )
    }
    val Key: ImageVector by lazy { line("key", circle(8f, 15f, 4f), "M11 12l8 -8M16 7l2 2M14 9l2 2") }
    val Gear: ImageVector by lazy {
        line(
            "gear",
            circle(12f, 12f, 3f),
            "M12 2.5v3M12 18.5v3M2.5 12h3M18.5 12h3M5.3 5.3l2.1 2.1M16.6 16.6l2.1 2.1M5.3 18.7l2.1 -2.1M16.6 7.4l2.1 -2.1",
        )
    }
    val Msg: ImageVector by lazy { line("msg", "M4 5h16v11H9l-5 4z") }
    val Chart: ImageVector by lazy { line("chart", "M5 20V11M12 20V5M19 20v-7") }
    val Unlock: ImageVector by lazy { line("unlock", rect(5f, 11f, 14f, 9f, 2f), "M8 11V8a4 4 0 0 1 7.5 -2") }
    val Grid: ImageVector by lazy {
        line(
            "grid",
            rect(4f, 4f, 7f, 7f, 1.5f),
            rect(13f, 4f, 7f, 7f, 1.5f),
            rect(4f, 13f, 7f, 7f, 1.5f),
            rect(13f, 13f, 7f, 7f, 1.5f),
        )
    }
    val Chevron: ImageVector by lazy { line("chev", "M9 6l6 6 -6 6") }
    val Trash: ImageVector by lazy { line("trash", "M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1 -13M9 7V4h6v3") }
    val Info: ImageVector by lazy { line("info", circle(12f, 12f, 9f), "M12 11v5M12 8h0.01") }

    /** Монета с часами (копилка как валюта): заливка #FFC53A, край #B87800, стрелки #8A5A00. */
    val Coin: ImageVector by lazy {
        ImageVector.Builder(name = "coin", defaultWidth = 24.dp, defaultHeight = 24.dp, viewportWidth = 24f, viewportHeight = 24f)
            .addPath(
                pathData = addPathNodes(circle(12f, 12f, 10f)),
                fill = SolidColor(Tok.Coin),
                stroke = SolidColor(Tok.CoinEdge),
                strokeLineWidth = 1.5f,
            )
            .addPath(
                pathData = addPathNodes("M12 7.5V12l3 2"),
                fill = null,
                stroke = SolidColor(Tok.CoinHand),
                strokeLineWidth = 1.8f,
                strokeLineCap = StrokeCap.Round,
                strokeLineJoin = StrokeJoin.Round,
            )
            .build()
    }
}
