package com.raisesteady.guard.ui.theme

import androidx.compose.animation.core.CubicBezierEasing
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp

/**
 * RaiseSteady Guard colour tokens, copied from kidguard/ext/ui/tokens.css (light :3-8, dark :18-20),
 * as guard-ios/Shared/Palette.swift. Add no colours that tokens.css does not have.
 */
@Immutable
data class GuardColors(
    val bg: Color,
    val surface: Color,
    val text: Color,
    val text2: Color,
    val accent: Color,
    val accentText: Color,
    val soft: Color,
    val line: Color,
    val focus: Color,
    val ok: Color,
    val warn: Color,
    val stop: Color,
    val isDark: Boolean,
)

val LightGuardColors = GuardColors(
    bg = Color(0xFFF7F6F1), surface = Color(0xFFFFFFFF), text = Color(0xFF1D1D2C), text2 = Color(0xFF4A4A68),
    accent = Color(0xFF282858), accentText = Color(0xFFFFFFFF), soft = Color(0xFFE9D9FF), line = Color(0xFFD9D8E6),
    focus = Color(0xFF3B3BB3), ok = Color(0xFF1F7A57), warn = Color(0xFF8A5A00), stop = Color(0xFFA3313A),
    isDark = false,
)

val DarkGuardColors = GuardColors(
    bg = Color(0xFF15152B), surface = Color(0xFF212142), text = Color(0xFFF2F1FA), text2 = Color(0xFFC3C2DC),
    accent = Color(0xFFFEDB71), accentText = Color(0xFF15152B), soft = Color(0xFF2E2A4F), line = Color(0xFF3A3A63),
    focus = Color(0xFF9FA0FF), ok = Color(0xFF7FD6B2), warn = Color(0xFFF3C969), stop = Color(0xFFFF9A9B),
    isDark = true,
)

val LocalGuardColors = staticCompositionLocalOf { LightGuardColors }

/** Shapes and sizes from kidguard/ext/ui/kit.css:19-63 (in dp; 48 dp touch size on Android). */
object GuardDimens {
    val cardRadius = 18.dp
    val choiceRadius = 12.dp
    val cardBorder = 1.dp
    val edge = 2.dp
    val focusRing = 3.dp
    val shadowOffset = 4.dp
    const val SHADOW_ALPHA = 0.15f
    val minTouch = 48.dp
    val gap = 16.dp
    val gapLarge = 28.dp
}

/** Motion from tokens.css:24-30: enter = 8 dp rise + fade, 220 ms; with animations removed, the same fade. */
object GuardMotion {
    const val ENTER_MS = 220
    val rise = 8.dp
    val easing = CubicBezierEasing(0.2f, 0.7f, 0.2f, 1f)
}
