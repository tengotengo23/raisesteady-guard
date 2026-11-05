package com.raisesteady.guard.ui.parent

import android.os.Build
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.raisesteady.guard.R
import com.raisesteady.guard.data.Homework
import com.raisesteady.guard.data.Level
import com.raisesteady.guard.data.Settings
import com.raisesteady.guard.data.Window
import com.raisesteady.guard.guard.Health
import com.raisesteady.guard.rules.Schedule
import com.raisesteady.guard.ui.InstalledApp
import com.raisesteady.guard.ui.Pages
import com.raisesteady.guard.ui.components.GuardField
import com.raisesteady.guard.ui.components.GuardText
import com.raisesteady.guard.ui.components.Mark
import com.raisesteady.guard.ui.components.Note
import com.raisesteady.guard.ui.components.StatusLine
import com.raisesteady.guard.ui.components.Stepper
import com.raisesteady.guard.ui.components.SwitchRow
import com.raisesteady.guard.ui.components.TimeRow
import com.raisesteady.guard.ui.guardApp
import com.raisesteady.guard.ui.open
import com.raisesteady.guard.ui.rememberHealth
import com.raisesteady.guard.ui.rememberInstalledApps
import com.raisesteady.guard.ui.theme.GuardDimens
import com.raisesteady.guard.ui.theme.LocalGuardColors
import com.raisesteady.guard.ui.unlockSettings
import kotlinx.coroutines.launch

typealias Edit = ((Settings) -> Settings) -> Unit

private fun stepMinutes(m: Int, up: Boolean): Int {
    val s = Settings.GAME_MINUTES_STEP
    val next = if (up) (m / s + 1) * s else ((m + s - 1) / s - 1) * s
    return next.coerceIn(Settings.MIN_GAME_MINUTES, Settings.MAX_GAME_MINUTES)
}

/** "Game time: 45 min a day" (15 to 240 in steps of 15). */
@Composable
fun GameMinutesEditor(settings: Settings, edit: Edit) {
    val m = settings.gameMinutes
    Stepper(
        stringResource(R.string.game_minutes, m),
        { edit { it.copy(gameMinutes = stepMinutes(it.gameMinutes, false)) } },
        { edit { it.copy(gameMinutes = stepMinutes(it.gameMinutes, true)) } },
        minusEnabled = m > Settings.MIN_GAME_MINUTES, plusEnabled = m < Settings.MAX_GAME_MINUTES,
    )
}

/** Game time, the bedtime switch and its two times (setup step 12, parent Times). */
@Composable
fun BedtimeEditor(settings: Settings, edit: Edit) {
    var bad by rememberSaveable { mutableStateOf(false) }
    GameMinutesEditor(settings, edit)
    SwitchRow(stringResource(R.string.bedtime_switch), settings.bedtimeOn, { on -> edit { it.copy(bedtimeOn = on) } })
    if (settings.bedtimeOn) {
        TimeRow(stringResource(R.string.bedtime_from), settings.bedtime.start) { t ->
            bad = !Schedule.validNight(t, settings.bedtime.end)
            if (!bad) edit { it.copy(bedtime = it.bedtime.copy(start = t)) }
        }
        TimeRow(stringResource(R.string.bedtime_to), settings.bedtime.end) { t ->
            bad = !Schedule.validNight(settings.bedtime.start, t)
            if (!bad) edit { it.copy(bedtime = it.bedtime.copy(end = t)) }
        }
        if (bad) Note(stringResource(R.string.night_bad), color = LocalGuardColors.current.stop)
    }
}

/** Weekend times and homework time (parent Times; copy from Strings.swift weekend_* and study_*). */
@Composable
fun TimesEditor(settings: Settings, edit: Edit) {
    var nightBad by rememberSaveable { mutableStateOf(false) }
    var homeBad by rememberSaveable { mutableStateOf(false) }
    BedtimeEditor(settings, edit)
    SwitchRow(stringResource(R.string.weekend_on), settings.weekend.on, { on -> edit { it.copy(weekend = it.weekend.copy(on = on)) } })
    if (settings.weekend.on) {
        Note(stringResource(R.string.weekend_note))
        val wm = settings.weekend.minutes
        Stepper(
            stringResource(R.string.weekend_minutes, wm),
            { edit { it.copy(weekend = it.weekend.copy(minutes = stepMinutes(it.weekend.minutes, false))) } },
            { edit { it.copy(weekend = it.weekend.copy(minutes = stepMinutes(it.weekend.minutes, true))) } },
            minusEnabled = wm > Settings.MIN_GAME_MINUTES, plusEnabled = wm < Settings.MAX_GAME_MINUTES,
        )
        val wb = settings.weekend.bedtime
        TimeRow(stringResource(R.string.weekend_bed_from), wb.start) { t ->
            nightBad = !Schedule.validNight(t, wb.end)
            if (!nightBad) edit { it.copy(weekend = it.weekend.copy(bedtime = Window(t, it.weekend.bedtime.end))) }
        }
        TimeRow(stringResource(R.string.weekend_bed_to), wb.end) { t ->
            nightBad = !Schedule.validNight(wb.start, t)
            if (!nightBad) edit { it.copy(weekend = it.weekend.copy(bedtime = Window(it.weekend.bedtime.start, t))) }
        }
        if (nightBad) Note(stringResource(R.string.night_bad), color = LocalGuardColors.current.stop)
    }
    SwitchRow(stringResource(R.string.homework_on), settings.homework.on, { on -> edit { it.copy(homework = it.homework.copy(on = on)) } })
    if (settings.homework.on) {
        Note(stringResource(R.string.homework_note))
        val h: Homework = settings.homework
        TimeRow(stringResource(R.string.homework_from), h.start) { t ->
            homeBad = !Schedule.validHomework(t, h.end)
            if (!homeBad) edit { it.copy(homework = it.homework.copy(start = t)) }
        }
        TimeRow(stringResource(R.string.homework_to), h.end) { t ->
            homeBad = !Schedule.validHomework(h.start, t)
            if (!homeBad) edit { it.copy(homework = it.homework.copy(end = t)) }
        }
        if (homeBad) Note(stringResource(R.string.homework_bad), color = LocalGuardColors.current.stop)
    }
}

/**
 * The installed apps with their level: Allowed, Game time, Blocked (as pc-apps.json). A search
 * field filters by name. Each app's three choices wrap under its name at large text.
 */
@Composable
fun AppsEditor(settings: Settings, edit: Edit) {
    val ctx = LocalContext.current
    val app = ctx.guardApp
    val apps by rememberInstalledApps()
    var query by rememberSaveable { mutableStateOf("") }
    val ageMin = app.bands.byId(settings.bandId).minAge
    GuardField(query, { query = it }, stringResource(R.string.apps_search), Modifier.testTag("app-search"))
    val list = apps ?: return
    val shown = list.filter { query.isBlank() || it.label.contains(query.trim(), ignoreCase = true) }
    if (shown.isEmpty()) Note(stringResource(R.string.apps_none))
    for (a in shown) {
        val level = app.catalog.level(a.pkg, a.isGame, settings, ageMin)
        AppRow(a, level) { l -> edit { it.copy(appLevels = it.appLevels + (a.pkg to l)) } }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun AppRow(a: InstalledApp, level: Level, onLevel: (Level) -> Unit) {
    val c = LocalGuardColors.current
    Column(Modifier.fillMaxWidth().testTag("app-${a.pkg}"), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            a.icon?.let { Image(it, contentDescription = null, modifier = Modifier.size(40.dp)) } ?: Spacer(Modifier.size(40.dp))
            GuardText(a.label, Modifier.weight(1f).semantics { heading() }, MaterialTheme.typography.bodyLarge, c.text)
        }
        FlowRow(Modifier.selectableGroup(), horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            for ((l, label) in listOf(Level.ALLOWED to R.string.level_allowed, Level.GAME_TIME to R.string.level_game, Level.BLOCKED to R.string.level_blocked)) {
                LevelPill(stringResource(label), level == l, { onLevel(l) }, Modifier.testTag("level-${l.name}"))
            }
        }
    }
}

@Composable
private fun LevelPill(text: String, selected: Boolean, onClick: () -> Unit, modifier: Modifier) {
    val c = LocalGuardColors.current
    val shape = RoundedCornerShape(50)
    GuardText(
        text,
        modifier
            .heightIn(min = GuardDimens.minTouch)
            .background(if (selected) c.soft else c.surface, shape)
            .border(GuardDimens.edge, if (selected) c.accent else c.line, shape)
            .selectable(selected = selected, onClick = onClick, role = Role.RadioButton)
            .padding(horizontal = 16.dp, vertical = 12.dp),
        MaterialTheme.typography.labelLarge, c.text,
    )
}

/**
 * Protection checks (setup step 13, parent page): each line with a tick, or what is wrong and
 * "Fix". After setup, Fix unlocks the guarded Settings pages for 10 minutes first (the parent is
 * already past the PIN here).
 */
@Composable
fun Checks(forSetup: Boolean, onVpn: () -> Unit = {}) {
    val ctx = LocalContext.current
    val app = ctx.guardApp
    val scope = rememberCoroutineScope()
    val h: Health = rememberHealth().value ?: return
    val fixLabel = stringResource(R.string.fix)
    fun fix(vararg intents: android.content.Intent): Pair<String, () -> Unit> = fixLabel to {
        scope.launch {
            if (app.repo.settings.value.setupDone) unlockSettings(ctx)
            open(ctx, *intents)
        }
    }
    Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
        if (!forSetup) {
            if (!h.vpnRunning) StatusLine(Mark.STOP, stringResource(R.string.miss_vpn), fixLabel to onVpn)
            if (!h.accessibility) StatusLine(Mark.STOP, stringResource(R.string.miss_a11y), fix(*Pages.accessibility(ctx)))
            if (!h.admin) StatusLine(Mark.STOP, stringResource(R.string.miss_admin), fixLabel to { open(ctx, Pages.addAdmin(ctx)) })
            if (!h.usageAccess) StatusLine(Mark.WARN, stringResource(R.string.miss_usage), fix(*Pages.usageAccess(ctx)))
            if (!h.notifications) StatusLine(Mark.WARN, stringResource(R.string.miss_notif), fix(Pages.notifications(ctx)))
            if (!(h.alwaysOn && h.lockdown)) StatusLine(Mark.WARN, stringResource(R.string.miss_always), fix(Pages.vpn()))
        }
        h.privateDnsHost?.let { StatusLine(Mark.STOP, stringResource(R.string.check_dns_bad, it), fix(*Pages.privateDns())) }
            ?: StatusLine(Mark.OK, stringResource(R.string.check_dns_ok))
        if (h.autoTime) StatusLine(Mark.OK, stringResource(R.string.check_time_ok)) else StatusLine(Mark.WARN, stringResource(R.string.check_time_bad), fix(Pages.dateTime()))
        if (h.unusedAppPauseOff) {
            StatusLine(Mark.OK, stringResource(R.string.check_unused_ok))
        } else {
            StatusLine(Mark.WARN, stringResource(R.string.check_unused_bad), fix(Pages.unusedApps(ctx)))
        }
        if (Build.MANUFACTURER.lowercase() in Pages.batteryKillers) {
            StatusLine(Mark.WARN, stringResource(R.string.check_battery), stringResource(R.string.battery_settings) to { open(ctx, Pages.battery()) })
        }
    }
}
