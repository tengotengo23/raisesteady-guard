package com.raisesteady.guard.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.raisesteady.guard.ui.theme.GuardDimens
import com.raisesteady.guard.ui.theme.LocalGuardColors

/**
 * A card as in kit.css: radius 18 dp, 1 dp line border, and in light mode the RaiseSteady navy
 * offset shadow (4 dp down, accent at 15 %). No fixed heights, so text can grow at font scale 2.0.
 */
@Composable
fun GuardCard(modifier: Modifier = Modifier, content: @Composable ColumnScope.() -> Unit) {
    val c = LocalGuardColors.current
    val shape = RoundedCornerShape(GuardDimens.cardRadius)
    Box(modifier.fillMaxWidth()) {
        if (!c.isDark) {
            Box(
                Modifier
                    .matchParentSize()
                    .offset(y = GuardDimens.shadowOffset)
                    .background(c.accent.copy(alpha = GuardDimens.SHADOW_ALPHA), shape),
            )
        }
        Column(
            Modifier
                .fillMaxWidth()
                .background(c.surface, shape)
                .border(GuardDimens.cardBorder, c.line, shape)
                .padding(GuardDimens.gap),
            verticalArrangement = Arrangement.spacedBy(8.dp),
            content = content,
        )
    }
}

