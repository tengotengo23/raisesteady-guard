package com.raisesteady.guard.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.em
import androidx.compose.ui.unit.sp

/** Type in sp (scales with the font size setting): title 24, heading 20, body 16, callout 15, footnote 13; line height 1.5. */
private val GuardTypography = Typography(
    headlineMedium = TextStyle(fontSize = 24.sp, lineHeight = 1.5.em, fontWeight = FontWeight.Bold),
    titleLarge = TextStyle(fontSize = 20.sp, lineHeight = 1.5.em, fontWeight = FontWeight.Bold),
    bodyLarge = TextStyle(fontSize = 16.sp, lineHeight = 1.5.em),
    bodyMedium = TextStyle(fontSize = 15.sp, lineHeight = 1.5.em),
    bodySmall = TextStyle(fontSize = 13.sp, lineHeight = 1.5.em),
    labelLarge = TextStyle(fontSize = 16.sp, lineHeight = 1.5.em, fontWeight = FontWeight.SemiBold),
)

@Composable
fun GuardTheme(dark: Boolean = isSystemInDarkTheme(), content: @Composable () -> Unit) {
    val c = if (dark) DarkGuardColors else LightGuardColors
    val scheme = if (dark) {
        darkColorScheme(
            primary = c.accent, onPrimary = c.accentText, background = c.bg, onBackground = c.text,
            surface = c.surface, onSurface = c.text, onSurfaceVariant = c.text2, secondaryContainer = c.soft,
            onSecondaryContainer = c.text, outline = c.line, error = c.stop,
        )
    } else {
        lightColorScheme(
            primary = c.accent, onPrimary = c.accentText, background = c.bg, onBackground = c.text,
            surface = c.surface, onSurface = c.text, onSurfaceVariant = c.text2, secondaryContainer = c.soft,
            onSecondaryContainer = c.text, outline = c.line, error = c.stop,
        )
    }
    CompositionLocalProvider(LocalGuardColors provides c) {
        MaterialTheme(colorScheme = scheme, typography = GuardTypography, content = content)
    }
}
