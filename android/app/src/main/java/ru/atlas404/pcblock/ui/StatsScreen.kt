package ru.atlas404.pcblock.ui

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import ru.atlas404.pcblock.R
import ru.atlas404.pcblock.proto.Format
import ru.atlas404.pcblock.proto.Proto
import ru.atlas404.pcblock.proto.TodayView

/**
 * «Статистика за сегодня» (FR-43): сидит ли сейчас, когда сел впервые, всего и осталось, «Во что играл»,
 * учёба, видео и соцсети, прочие программы и сайты, нарушения; внизу «Данные на HH:MM».
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun StatsScreen(child: Proto.Child?, onBack: () -> Unit) {
    BackHandler(onBack = onBack)
    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    Text(
                        child?.name ?: stringResource(R.string.stats_title),
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = stringResource(R.string.stats_back))
                    }
                },
            )
        },
    ) { pad ->
        val t = child?.today
        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .padding(pad),
            contentPadding = PaddingValues(horizontal = 16.dp, vertical = 12.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            item(key = "title") {
                Text(
                    stringResource(R.string.stats_title),
                    style = MaterialTheme.typography.titleLarge,
                    modifier = Modifier.semantics { heading() },
                )
            }
            if (t == null) {
                item(key = "none") { Text(stringResource(R.string.stats_none), style = MaterialTheme.typography.bodyLarge) }
                return@LazyColumn
            }
            val v = TodayView.of(t)
            item(key = "summary") { SummaryCard(t) }
            item(key = "games") {
                Section(stringResource(R.string.stats_games)) {
                    if (v.games.isEmpty()) {
                        Text(stringResource(R.string.stats_no_games), style = MaterialTheme.typography.bodyLarge)
                    } else {
                        for (g in v.games) ItemRow(g)
                        Text(
                            stringResource(R.string.stats_games_total, spentText(t.gamesSec)),
                            style = MaterialTheme.typography.titleSmall,
                        )
                    }
                }
            }
            item(key = "cats") {
                Section(null) {
                    Text(stringResource(R.string.stats_study, spentText(t.studySec)), style = MaterialTheme.typography.bodyLarge)
                    Text(stringResource(R.string.stats_video, spentText(t.videoSec)), style = MaterialTheme.typography.bodyLarge)
                }
            }
            if (v.programs.isNotEmpty() || t.morePrograms > 0) {
                item(key = "programs") {
                    Section(stringResource(R.string.stats_programs)) {
                        for (p in v.programs) ItemRow(p)
                        if (t.morePrograms > 0) MoreRow(t.morePrograms)
                    }
                }
            }
            if (v.sites.isNotEmpty() || t.moreSites > 0) {
                item(key = "sites") {
                    Section(stringResource(R.string.stats_sites)) {
                        for (s in v.sites) ItemRow(s)
                        if (t.moreSites > 0) MoreRow(t.moreSites)
                    }
                }
            }
            item(key = "violations") {
                Text(
                    if (t.bypass > 0) {
                        stringResource(R.string.stats_violations_bypass, t.violations, t.bypass)
                    } else {
                        stringResource(R.string.stats_violations, t.violations)
                    },
                    style = MaterialTheme.typography.bodyLarge,
                    color = if (t.violations > 0) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurface,
                )
            }
            item(key = "asof") {
                Text(
                    stringResource(R.string.data_at, clockText(t.asOfMs)),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

@Composable
private fun SummaryCard(t: Proto.Today) {
    Section(null) {
        Text(
            when {
                t.nowAt && t.sinceMs > 0 -> stringResource(R.string.stats_at_pc_since, clockText(t.sinceMs))
                !t.nowAt && t.sinceMs > 0 -> stringResource(R.string.stats_away_since, clockText(t.sinceMs))
                else -> stringResource(R.string.stats_away)
            },
            style = MaterialTheme.typography.titleMedium,
        )
        Text(
            if (t.firstMs > 0) stringResource(R.string.stats_first, clockText(t.firstMs)) else stringResource(R.string.stats_first_none),
            style = MaterialTheme.typography.bodyLarge,
        )
        Text(stringResource(R.string.stats_used, spentText(t.usedSec)), style = MaterialTheme.typography.bodyLarge)
        Text(
            if (t.leftSec < 0) stringResource(R.string.stats_unlimited) else stringResource(R.string.stats_left, durationText(t.leftSec)),
            style = MaterialTheme.typography.titleMedium,
            color = MaterialTheme.colorScheme.primary,
        )
    }
}

@Composable
private fun Section(title: String?, content: @Composable () -> Unit) {
    Card(
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerHigh),
        modifier = Modifier.fillMaxWidth(),
    ) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            if (title != null) {
                Text(title, style = MaterialTheme.typography.titleMedium, modifier = Modifier.semantics { heading() })
            }
            content()
        }
    }
}

/** Строка «Minecraft ... 45 мин»: название слева (обрезается), время справа; TalkBack читает одной фразой. */
@Composable
private fun ItemRow(item: Proto.Item) {
    val time = spentText(item.sec)
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clearAndSetSemantics { contentDescription = "${item.name}, $time" },
    ) {
        Text(
            item.name,
            style = MaterialTheme.typography.bodyLarge,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.weight(1f),
        )
        Spacer(Modifier.width(12.dp))
        Text(time, style = MaterialTheme.typography.bodyLarge)
    }
}

@Composable
private fun MoreRow(n: Int) {
    Text(
        stringResource(R.string.stats_more, n),
        style = MaterialTheme.typography.bodyMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
}

@Composable
private fun spentText(seconds: Long): String =
    Format.spent(seconds, stringResource(R.string.dur_min), stringResource(R.string.dur_hm), stringResource(R.string.dur_less_min))
