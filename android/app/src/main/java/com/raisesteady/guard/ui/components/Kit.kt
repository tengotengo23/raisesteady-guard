package com.raisesteady.guard.ui.components

import android.animation.ValueAnimator
import android.app.TimePickerDialog
import android.os.SystemClock
import android.text.format.DateFormat
import androidx.annotation.DrawableRes
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.RadioButton
import androidx.compose.material3.RadioButtonDefaults
import androidx.compose.material3.Surface
import androidx.compose.material3.Switch
import androidx.compose.material3.SwitchDefaults
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.MotionDurationScale
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.testTagsAsResourceId
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.tween
import com.raisesteady.guard.BuildConfig
import com.raisesteady.guard.R
import com.raisesteady.guard.data.Window
import com.raisesteady.guard.ui.Fmt
import com.raisesteady.guard.ui.LookLog
import com.raisesteady.guard.ui.theme.GuardDimens
import com.raisesteady.guard.ui.theme.GuardMotion
import com.raisesteady.guard.ui.theme.LocalGuardColors
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

// The look of kidguard/ext/ui/kit.css in Compose. No fixed heights anywhere: at font scale 2.0
// text grows and rows wrap. Every text goes through GuardText, which tells the debug look log
// when a text does not fit its box.

/** Text that reports a visual overflow (cut text) to the look tests in debug builds. */
@Composable
fun GuardText(
    text: String,
    modifier: Modifier = Modifier,
    style: TextStyle = MaterialTheme.typography.bodyLarge,
    color: Color = Color.Unspecified,
    textAlign: TextAlign? = null,
) {
    Text(
        text, modifier, color = color, style = style, textAlign = textAlign,
        onTextLayout = { if (BuildConfig.DEBUG && it.hasVisualOverflow) LookLog.overflow(text) },
    )
}

@Composable
fun Title(text: String, modifier: Modifier = Modifier) =
    GuardText(text, modifier.semantics { heading() }, MaterialTheme.typography.headlineMedium, LocalGuardColors.current.text)

@Composable
fun Heading(text: String, modifier: Modifier = Modifier) =
    GuardText(text, modifier.semantics { heading() }, MaterialTheme.typography.titleLarge, LocalGuardColors.current.text)

@Composable
fun Body(text: String, modifier: Modifier = Modifier) =
    GuardText(text, modifier, MaterialTheme.typography.bodyLarge, LocalGuardColors.current.text)

@Composable
fun Note(text: String, modifier: Modifier = Modifier, color: Color = LocalGuardColors.current.text2) =
    GuardText(text, modifier, MaterialTheme.typography.bodyMedium, color)

@Composable
fun Foot(text: String, modifier: Modifier = Modifier) =
    GuardText(text, modifier, MaterialTheme.typography.bodySmall, LocalGuardColors.current.text2)

/** One of our own RS emoji (res/drawable/rs_*.xml), never a system emoji. Decorative: the title says the same. */
@Composable
fun RsEmoji(@DrawableRes id: Int, modifier: Modifier = Modifier, size: Dp = 64.dp) =
    Image(painterResource(id), contentDescription = null, modifier = modifier.size(size))

/** The main pill button (min height 48 dp, text wraps). */
@Composable
fun MainButton(text: String, onClick: () -> Unit, modifier: Modifier = Modifier, enabled: Boolean = true) {
    val c = LocalGuardColors.current
    Button(
        onClick = onClick,
        enabled = enabled,
        shape = RoundedCornerShape(50),
        colors = ButtonDefaults.buttonColors(
            containerColor = c.accent, contentColor = c.accentText,
            disabledContainerColor = c.line, disabledContentColor = c.text2,
        ),
        modifier = modifier.fillMaxWidth().heightIn(min = GuardDimens.minTouch),
    ) { GuardText(text, style = MaterialTheme.typography.labelLarge, textAlign = TextAlign.Center) }
}

/** A second pill button with a 2 dp outline. */
@Composable
fun SecondButton(text: String, onClick: () -> Unit, modifier: Modifier = Modifier, fill: Boolean = true) {
    val c = LocalGuardColors.current
    OutlinedButton(
        onClick = onClick,
        shape = RoundedCornerShape(50),
        border = BorderStroke(GuardDimens.edge, c.accent),
        colors = ButtonDefaults.outlinedButtonColors(contentColor = c.text),
        modifier = (if (fill) modifier.fillMaxWidth() else modifier).heightIn(min = GuardDimens.minTouch),
    ) { GuardText(text, style = MaterialTheme.typography.labelLarge, textAlign = TextAlign.Center) }
}

/** A quiet text button (links, "Not now"). */
@Composable
fun LinkButton(text: String, onClick: () -> Unit, modifier: Modifier = Modifier) {
    val c = LocalGuardColors.current
    TextButton(onClick = onClick, modifier = modifier.heightIn(min = GuardDimens.minTouch)) {
        GuardText(text, style = MaterialTheme.typography.labelLarge, color = c.accent)
    }
}

/** Small pill buttons in a row that wraps (pause lengths, more time). */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun PillRow(items: List<Pair<String, () -> Unit>>) {
    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        for ((label, action) in items) SecondButton(label, action, fill = false)
    }
}

/** A choice card (kit.css): radius 12, 2 dp line edge; chosen = accent edge and soft fill. TalkBack: a radio button. */
@Composable
fun ChoiceRow(text: String, selected: Boolean, onClick: () -> Unit, modifier: Modifier = Modifier) {
    val c = LocalGuardColors.current
    val shape = RoundedCornerShape(GuardDimens.choiceRadius)
    Row(
        modifier
            .fillMaxWidth()
            .heightIn(min = GuardDimens.minTouch)
            .background(if (selected) c.soft else c.surface, shape)
            .border(GuardDimens.edge, if (selected) c.accent else c.line, shape)
            .selectable(selected = selected, onClick = onClick, role = Role.RadioButton)
            .padding(horizontal = 12.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        RadioButton(selected = selected, onClick = null, colors = RadioButtonDefaults.colors(selectedColor = c.accent, unselectedColor = c.text2))
        GuardText(text, Modifier.weight(1f), MaterialTheme.typography.bodyLarge, c.text)
    }
}

/** Choice rows as one group (TalkBack reads "1 of 5"). */
@Composable
fun ChoiceGroup(modifier: Modifier = Modifier, content: @Composable ColumnScope.() -> Unit) =
    Column(modifier.selectableGroup(), verticalArrangement = Arrangement.spacedBy(8.dp), content = content)

/** A switch with its label; the whole row toggles (TalkBack: a switch). */
@Composable
fun SwitchRow(text: String, checked: Boolean, onChange: (Boolean) -> Unit, modifier: Modifier = Modifier) {
    val c = LocalGuardColors.current
    Row(
        modifier
            .fillMaxWidth()
            .heightIn(min = GuardDimens.minTouch)
            .toggleable(value = checked, onValueChange = onChange, role = Role.Switch)
            .padding(vertical = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        GuardText(text, Modifier.weight(1f), MaterialTheme.typography.bodyLarge, c.text)
        Switch(
            checked = checked, onCheckedChange = null,
            colors = SwitchDefaults.colors(
                checkedThumbColor = c.accentText, checkedTrackColor = c.accent,
                uncheckedThumbColor = c.text2, uncheckedTrackColor = c.surface, uncheckedBorderColor = c.text2,
            ),
        )
    }
}

enum class Mark { OK, WARN, STOP }

/** A status line: a tick (RS emoji) when fine, a coloured dot when not; an optional "Fix" button under the text. */
@Composable
fun StatusLine(mark: Mark, text: String, action: Pair<String, () -> Unit>? = null) {
    val c = LocalGuardColors.current
    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(12.dp)) {
        if (mark == Mark.OK) {
            RsEmoji(R.drawable.rs_check, Modifier.padding(top = 2.dp), 24.dp)
        } else {
            Box(
                Modifier.padding(top = 6.dp, start = 6.dp, end = 6.dp).size(12.dp)
                    .background(if (mark == Mark.WARN) c.warn else c.stop, CircleShape),
            )
        }
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            GuardText(text, style = MaterialTheme.typography.bodyLarge, color = c.text)
            action?.let { (label, go) -> SecondButton(label, go, fill = false) }
        }
    }
}

/** "Game time: 45 min a day" with Less and More (15-minute steps). */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun Stepper(text: String, onMinus: () -> Unit, onPlus: () -> Unit, minusEnabled: Boolean, plusEnabled: Boolean) {
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Body(text)
        FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            StepButton("−", stringResource(R.string.less_time), onMinus, minusEnabled)
            StepButton("+", stringResource(R.string.more_time_step), onPlus, plusEnabled)
        }
    }
}

@Composable
private fun StepButton(sign: String, label: String, onClick: () -> Unit, enabled: Boolean) {
    val c = LocalGuardColors.current
    OutlinedButton(
        onClick = onClick, enabled = enabled, shape = RoundedCornerShape(50),
        border = BorderStroke(GuardDimens.edge, if (enabled) c.accent else c.line),
        colors = ButtonDefaults.outlinedButtonColors(contentColor = c.text),
        modifier = Modifier.heightIn(min = GuardDimens.minTouch).semantics { contentDescription = label },
    ) { GuardText(sign, style = MaterialTheme.typography.titleLarge) }
}

/** "Bedtime starts: 21:00": opens Android's time picker (accessible, follows 12 or 24 hours). */
@Composable
fun TimeRow(label: String, hm: String, onChange: (String) -> Unit) {
    val ctx = LocalContext.current
    SecondButton(stringResource(R.string.time_value, label, Fmt.clock(ctx, hm)), {
        val m = Window.minutes(hm) ?: 0
        TimePickerDialog(ctx, { _, h, min -> onChange(Fmt.hm(h, min)) }, m / 60, m % 60, DateFormat.is24HourFormat(ctx)).show()
    })
}

/** The PIN field (digits only, hidden; TalkBack reads its label; the error shows under it). */
@Composable
fun ParentPinField(value: String, onChange: (String) -> Unit, label: String, error: String?, modifier: Modifier = Modifier) {
    val c = LocalGuardColors.current
    OutlinedTextField(
        value = value,
        onValueChange = { v -> onChange(v.filter(Char::isDigit).take(8)) },
        label = { GuardText(label, style = MaterialTheme.typography.bodyMedium) },
        singleLine = true,
        isError = error != null,
        supportingText = error?.let { { GuardText(it, style = MaterialTheme.typography.bodyMedium, color = c.stop) } },
        visualTransformation = PasswordVisualTransformation(),
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.NumberPassword),
        colors = fieldColors(),
        modifier = modifier.fillMaxWidth(),
    )
}

/** A plain text field (site address, app search). */
@Composable
fun GuardField(value: String, onChange: (String) -> Unit, label: String, modifier: Modifier = Modifier, error: String? = null, keyboard: KeyboardType = KeyboardType.Text) {
    val c = LocalGuardColors.current
    OutlinedTextField(
        value = value, onValueChange = onChange,
        label = { GuardText(label, style = MaterialTheme.typography.bodyMedium) },
        singleLine = true,
        isError = error != null,
        supportingText = error?.let { { GuardText(it, style = MaterialTheme.typography.bodyMedium, color = c.stop) } },
        keyboardOptions = KeyboardOptions(keyboardType = keyboard),
        colors = fieldColors(),
        modifier = modifier.fillMaxWidth(),
    )
}

@Composable
private fun fieldColors() = LocalGuardColors.current.let { c ->
    OutlinedTextFieldDefaults.colors(
        focusedBorderColor = c.focus, unfocusedBorderColor = c.text2, focusedLabelColor = c.focus, unfocusedLabelColor = c.text2,
        cursorColor = c.text, focusedTextColor = c.text, unfocusedTextColor = c.text, errorBorderColor = c.stop, errorLabelColor = c.stop,
    )
}

/** A card with a heading (a section of a page). */
@Composable
fun Section(title: String, content: @Composable ColumnScope.() -> Unit) = GuardCard {
    Heading(title)
    content()
}

/**
 * A page: our background, safe area, scrolling column. Test tags are resource ids (UiAutomator).
 * [bottom] stays under the scrolling part (the main buttons of a setup step).
 */
@Composable
fun Page(key: Any, bottom: (@Composable ColumnScope.() -> Unit)? = null, content: @Composable ColumnScope.() -> Unit) {
    val c = LocalGuardColors.current
    Surface(Modifier.fillMaxSize().semantics { testTagsAsResourceId = true }, color = c.bg) {
        Column(Modifier.safeDrawingPadding().imePadding()) {
            Enter(key, Modifier.weight(1f)) {
                Column(
                    Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(GuardDimens.gap),
                    verticalArrangement = Arrangement.spacedBy(GuardDimens.gap),
                    content = content,
                )
            }
            if (bottom != null) {
                Column(
                    Modifier.fillMaxWidth().background(c.bg).padding(horizontal = GuardDimens.gap, vertical = 12.dp).testTag("bottom"),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                    content = bottom,
                )
            }
        }
    }
}

/** Compose would scale our durations by "Animation duration scale" (0 with Remove animations): ours stay 220 ms. */
private object FixedMotionDurationScale : MotionDurationScale {
    override val scaleFactor: Float get() = 1f
}

/**
 * Enter motion (tokens.css:24-30): 8 dp rise + fade, 220 ms, cubic (0.2, 0.7, 0.2, 1). With
 * "Remove animations" on, the same 220 ms fade without the rise, never nothing at all.
 */
@Composable
fun Enter(key: Any, modifier: Modifier = Modifier, content: @Composable () -> Unit) {
    val alpha = remember(key) { Animatable(0f) }
    val rise = remember(key) { Animatable(1f) }
    val risePx = with(LocalDensity.current) { GuardMotion.rise.toPx() }
    LaunchedEffect(key) {
        val reduced = !ValueAnimator.areAnimatorsEnabled()
        val start = SystemClock.uptimeMillis()
        val spec = tween<Float>(GuardMotion.ENTER_MS, easing = GuardMotion.easing)
        withContext(FixedMotionDurationScale) {
            if (reduced) {
                rise.snapTo(0f)
                alpha.animateTo(1f, spec)
            } else {
                coroutineScope {
                    launch { rise.animateTo(0f, spec) }
                    alpha.animateTo(1f, spec)
                }
            }
        }
        LookLog.motion(if (reduced) "fade" else "rise+fade", GuardMotion.ENTER_MS, SystemClock.uptimeMillis() - start)
    }
    Box(modifier.fillMaxWidth().graphicsLayer { this.alpha = alpha.value; translationY = rise.value * risePx }) { content() }
}
