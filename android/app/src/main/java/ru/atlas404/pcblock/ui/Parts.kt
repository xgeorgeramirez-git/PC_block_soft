package ru.atlas404.pcblock.ui

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import ru.atlas404.pcblock.R
import ru.atlas404.pcblock.proto.Problem

/** Белая карточка с линией 1 dp и скруглением 18 dp (макет phones_more.py, card). */
@Composable
fun AppCard(modifier: Modifier = Modifier, padding: Dp = 16.dp, gap: Dp = 12.dp, content: @Composable ColumnScope.() -> Unit) {
    Column(
        verticalArrangement = Arrangement.spacedBy(gap),
        modifier = modifier
            .fillMaxWidth()
            .clip(Tok.CardShape)
            .background(Tok.Surface)
            .border(1.dp, Tok.Line, Tok.CardShape)
            .padding(padding),
        content = content,
    )
}

/** Главная кнопка экрана: синяя, 50 dp, скругление 12 dp. */
@Composable
fun MainButton(text: String, onClick: () -> Unit, modifier: Modifier = Modifier, enabled: Boolean = true) {
    Button(
        onClick = onClick,
        enabled = enabled,
        shape = Tok.ButtonShape,
        colors = ButtonDefaults.buttonColors(
            containerColor = Tok.Accent,
            contentColor = Color.White,
            disabledContainerColor = Tok.Track,
            disabledContentColor = Tok.Muted,
        ),
        contentPadding = PaddingValues(horizontal = 12.dp),
        modifier = modifier
            .fillMaxWidth()
            .heightIn(min = 50.dp),
    ) { Text(text, fontSize = 16.sp, fontWeight = FontWeight.SemiBold, textAlign = TextAlign.Center) }
}

/** Белая кнопка с рамкой; danger — красный текст и розовая рамка («Обнулить копилку»). */
@Composable
fun PlainButton(text: String, onClick: () -> Unit, modifier: Modifier = Modifier, enabled: Boolean = true, danger: Boolean = false) {
    OutlinedButton(
        onClick = onClick,
        enabled = enabled,
        shape = Tok.ButtonShape,
        border = BorderStroke(1.dp, if (danger) Tok.DangerLine else Tok.Line),
        colors = ButtonDefaults.outlinedButtonColors(
            containerColor = Tok.Surface,
            contentColor = if (danger) Tok.RemainRed else Tok.Ink,
            disabledContainerColor = Tok.Surface,
            disabledContentColor = Tok.Muted,
        ),
        contentPadding = PaddingValues(horizontal = 12.dp),
        modifier = modifier
            .fillMaxWidth()
            .heightIn(min = 50.dp),
    ) { Text(text, fontSize = 16.sp, fontWeight = FontWeight.SemiBold, textAlign = TextAlign.Center) }
}

/** Верх вложенного экрана: «Назад» и заголовок (экраны «Копилка», «Ждут решения»). */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SubTopBar(title: String, onBack: () -> Unit, backEnabled: Boolean = true) {
    TopAppBar(
        title = {
            Text(
                title,
                fontSize = 22.sp,
                fontWeight = FontWeight.Bold,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.semantics { heading() },
            )
        },
        navigationIcon = {
            IconButton(onClick = onBack, enabled = backEnabled) {
                Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = stringResource(R.string.note_back), tint = Tok.Accent)
            }
        },
        colors = TopAppBarDefaults.topAppBarColors(containerColor = Tok.Ground, titleContentColor = Tok.Ink),
    )
}

/** Заголовок вкладки: 24–26 sp, жирный, заголовок для TalkBack. */
@Composable
fun ScreenTitle(text: String, modifier: Modifier = Modifier, size: Int = 24) {
    Text(
        text,
        fontSize = size.sp,
        fontWeight = FontWeight.Bold,
        color = Tok.Ink,
        modifier = modifier.semantics { heading() },
    )
}

/** Мелкий вторичный текст 13 sp. */
@Composable
fun MutedText(text: String, modifier: Modifier = Modifier, size: Int = 13) {
    Text(text, fontSize = size.sp, fontWeight = FontWeight.Medium, color = Tok.Muted, modifier = modifier)
}

/**
 * Постоянная строка итога (TalkBack зачитывает сама): «Отправляем…», ответ компьютера или ошибка.
 * Пусто — строка остаётся (не прыгает раскладка).
 */
@Composable
fun StatusLine(busyText: String?, okText: String?, problem: Problem?, lastSeenMs: Long, modifier: Modifier = Modifier) {
    Row(
        verticalAlignment = Alignment.CenterVertically,
        modifier = modifier
            .fillMaxWidth()
            .heightIn(min = 24.dp)
            .semantics { liveRegion = LiveRegionMode.Polite },
    ) {
        when {
            busyText != null -> {
                CircularProgressIndicator(modifier = Modifier.size(18.dp), strokeWidth = 2.dp, color = Tok.Accent)
                Spacer(Modifier.width(8.dp))
                Text(busyText, fontSize = 15.sp, color = Tok.Ink)
            }
            problem != null -> Text(problemText(problem, lastSeenMs), fontSize = 15.sp, fontWeight = FontWeight.Medium, color = Tok.RemainRed)
            okText != null -> Text(okText, fontSize = 15.sp, fontWeight = FontWeight.Medium, color = Tok.PlusText)
        }
    }
}

/** Почему кнопки недоступны (ТЗ 8.0 п. 5): «Компьютер не на связи» или «… с HH:MM». */
@Composable
fun OfflineHint(sinceMs: Long, modifier: Modifier = Modifier) {
    Row(verticalAlignment = Alignment.CenterVertically, modifier = modifier.fillMaxWidth()) {
        Icon(AppIcons.Info, contentDescription = null, tint = Tok.RemainRed, modifier = Modifier.size(20.dp))
        Spacer(Modifier.width(8.dp))
        Text(
            if (sinceMs > 0) stringResource(R.string.pc_offline_since, clockText(sinceMs)) else stringResource(R.string.pc_offline_short),
            fontSize = 14.sp,
            fontWeight = FontWeight.Medium,
            color = Tok.RemainRed,
        )
    }
}

/** Аватар-буква 40 dp; фон по порядку ребёнка. */
@Composable
fun Avatar(name: String, index: Int, size: Dp = 40.dp) {
    Box(
        contentAlignment = Alignment.Center,
        modifier = Modifier
            .size(size)
            .background(Tok.avatar(index), CircleShape)
            .clearAndSetSemantics { },
    ) {
        Text(
            name.trim().take(1).uppercase(),
            fontSize = (size.value * 0.42f).sp,
            fontWeight = FontWeight.Bold,
            color = Tok.Ink,
        )
    }
}

/** Монета копилки. */
@Composable
fun CoinIcon(size: Dp = 20.dp) {
    Icon(AppIcons.Coin, contentDescription = null, tint = Color.Unspecified, modifier = Modifier.size(size))
}

/**
 * Полоса остатка с 8 делениями (как шкала окна): дорожка #E3E8F2, заполнение цветом тона.
 * Только украшение: число и подпись рядом несут смысл.
 */
@Composable
fun RemainBar(fraction: Float, color: Color, height: Dp, modifier: Modifier = Modifier) {
    val shape = RoundedCornerShape(height)
    Box(
        modifier = modifier
            .fillMaxWidth()
            .height(height)
            .clip(shape)
            .background(Tok.Track)
            .clearAndSetSemantics { },
    ) {
        Box(
            Modifier
                .fillMaxWidth(fraction.coerceIn(0f, 1f))
                .fillMaxHeight()
                .background(color),
        )
        Row(Modifier.fillMaxSize()) {
            repeat(8) { i ->
                Spacer(Modifier.weight(1f))
                if (i < 7) Box(Modifier.width(2.dp).fillMaxHeight().background(Tok.Surface))
            }
        }
    }
}

/** Строка-ссылка карточки: значок, текст (и подпись), стрелка; линия сверху, высота ≥ 52 dp. */
@Composable
fun LinkRow(icon: ImageVector, text: String, onClick: () -> Unit, sub: String? = null, enabled: Boolean = true, color: Color = Tok.Ink) {
    Column(Modifier.fillMaxWidth()) {
        HorizontalDivider(thickness = 1.dp, color = Tok.Line)
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(12.dp),
            modifier = Modifier
                .fillMaxWidth()
                .heightIn(min = 52.dp)
                .clickable(enabled = enabled, role = Role.Button, onClick = onClick)
                .padding(horizontal = 4.dp),
        ) {
            Icon(icon, contentDescription = null, tint = if (enabled) color else Tok.Muted, modifier = Modifier.size(22.dp))
            Column(Modifier.weight(1f).padding(vertical = 6.dp)) {
                Text(text, fontSize = 16.sp, fontWeight = FontWeight.SemiBold, color = if (enabled) color else Tok.Muted)
                if (sub != null) MutedText(sub)
            }
            Icon(AppIcons.Chevron, contentDescription = null, tint = Tok.Muted, modifier = Modifier.size(18.dp))
        }
    }
}
