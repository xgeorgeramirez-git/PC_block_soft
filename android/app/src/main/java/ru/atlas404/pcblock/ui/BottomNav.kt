package ru.atlas404.pcblock.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import ru.atlas404.pcblock.R

/**
 * Нижнее меню 72 dp (FR-45, макет phone2.py): «Дети», «Промокоды», «Настройки», линейные значки,
 * выбранный пункт — синий на светло-синей плашке. showPromos = false — компьютер старше 0.8.4
 * (вкладки нет, на «Детях» — «Обновите программу на компьютере до версии 0.8.4»).
 */
@Composable
fun BottomNav(tab: Tab, showPromos: Boolean, onTab: (Tab) -> Unit) {
    Column(
        Modifier
            .fillMaxWidth()
            .background(Tok.Surface)
            .navigationBarsPadding(),
    ) {
        HorizontalDivider(thickness = 1.dp, color = Tok.Line)
        Row(
            horizontalArrangement = Arrangement.SpaceAround,
            verticalAlignment = Alignment.CenterVertically,
            modifier = Modifier
                .fillMaxWidth()
                .height(72.dp)
                .selectableGroup(),
        ) {
            NavItem(tab == Tab.KIDS, AppIcons.Kids, stringResource(R.string.tab_kids)) { onTab(Tab.KIDS) }
            if (showPromos) NavItem(tab == Tab.PROMOS, AppIcons.Key, stringResource(R.string.tab_promos)) { onTab(Tab.PROMOS) }
            NavItem(tab == Tab.SETTINGS, AppIcons.Gear, stringResource(R.string.tab_settings)) { onTab(Tab.SETTINGS) }
        }
    }
}

@Composable
private fun RowScope.NavItem(selected: Boolean, icon: ImageVector, label: String, onClick: () -> Unit) {
    val color = if (selected) Tok.Accent else Tok.Muted
    Column(
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.Center,
        modifier = Modifier
            .weight(1f)
            .fillMaxHeight()
            .selectable(selected = selected, role = Role.Tab, onClick = onClick),
    ) {
        Box(
            contentAlignment = Alignment.Center,
            modifier = Modifier
                .size(width = 56.dp, height = 30.dp)
                .background(if (selected) Tok.AccentSoft else Color.Transparent, RoundedCornerShape(15.dp)),
        ) {
            Icon(icon, contentDescription = null, tint = color, modifier = Modifier.size(24.dp))
        }
        Spacer(Modifier.height(3.dp))
        Text(label, fontSize = 12.sp, fontWeight = if (selected) FontWeight.SemiBold else FontWeight.Medium, color = color)
    }
}
