package com.raisesteady.guard.ui.parent

import android.app.Activity
import android.net.VpnService
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.raisesteady.guard.R
import com.raisesteady.guard.data.Settings
import com.raisesteady.guard.guard.ParentPin
import com.raisesteady.guard.lists.SiteName
import com.raisesteady.guard.pin.Pin
import com.raisesteady.guard.rules.PauseChoice
import com.raisesteady.guard.rules.Schedule
import com.raisesteady.guard.ui.Fmt
import com.raisesteady.guard.ui.appLabel
import com.raisesteady.guard.ui.components.Body
import com.raisesteady.guard.ui.components.Foot
import com.raisesteady.guard.ui.components.GuardCard
import com.raisesteady.guard.ui.components.GuardField
import com.raisesteady.guard.ui.components.GuardText
import com.raisesteady.guard.ui.components.Heading
import com.raisesteady.guard.ui.components.MainButton
import com.raisesteady.guard.ui.components.Note
import com.raisesteady.guard.ui.components.Page
import com.raisesteady.guard.ui.components.ParentPinField
import com.raisesteady.guard.ui.components.PillRow
import com.raisesteady.guard.ui.components.SecondButton
import com.raisesteady.guard.ui.components.Section
import com.raisesteady.guard.ui.components.SwitchRow
import com.raisesteady.guard.ui.components.Title
import com.raisesteady.guard.ui.guardApp
import com.raisesteady.guard.ui.lockSettingsNow
import com.raisesteady.guard.ui.theme.LocalGuardColors
import com.raisesteady.guard.ui.turnProtectionOff
import com.raisesteady.guard.vpn.GuardVpnService
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import java.time.Instant
import java.time.LocalDate

/** "Enter the parent PIN" before the parent page (five wrong tries lock 15 minutes). */
@Composable
fun PinScreen(onOk: () -> Unit, onCancel: () -> Unit) {
    val ctx = LocalContext.current
    val scope = rememberCoroutineScope()
    var pin by remember { mutableStateOf("") }
    var error by remember { mutableStateOf<String?>(null) }
    Page(key = "pin", bottom = {
        MainButton(stringResource(R.string.next), {
            scope.launch {
                error = ParentPin.check(ctx, pin)
                if (error == null) onOk() else pin = ""
            }
        })
        SecondButton(stringResource(R.string.cancel), onCancel)
    }) {
        Title(stringResource(R.string.enter_pin))
        ParentPinField(pin, { pin = it; error = null }, stringResource(R.string.pin_label), error, Modifier.testTag("pin"))
    }
}

/** The parent page (design 4.4), after the PIN. */
@Composable
fun ParentScreen(onApps: () -> Unit, onClose: () -> Unit, onOff: () -> Unit) {
    val ctx = LocalContext.current
    val app = ctx.guardApp
    val scope = rememberCoroutineScope()
    val settings by app.repo.settings.collectAsStateWithLifecycle()
    val state by app.repo.state.collectAsStateWithLifecycle()
    var now by remember { mutableLongStateOf(app.clock.now().toEpochMilli()) }
    LaunchedEffect(Unit) {
        while (true) {
            delay(15_000)
            now = app.clock.now().toEpochMilli()
        }
    }
    val edit: Edit = { f -> scope.launch { app.repo.updateSettings(f) } }
    fun changeState(f: (com.raisesteady.guard.data.State) -> com.raisesteady.guard.data.State) = scope.launch {
        app.repo.updateState(f)
        now = app.clock.now().toEpochMilli()
        app.recheck.tryEmit(Unit)
    }
    val schedule = Schedule(app.clock.zone())
    val vpnConsent = rememberLauncherForActivityResult(ActivityResultContracts.StartActivityForResult()) { r ->
        if (r.resultCode == Activity.RESULT_OK) GuardVpnService.start(ctx)
    }

    Page(key = "parent", bottom = { SecondButton(stringResource(R.string.close), onClose) }) {
        Title(stringResource(R.string.parent_title))

        Section(stringResource(R.string.pause_title)) {
            if (state.paused(now)) {
                Body(stringResource(R.string.paused_until, Fmt.time(ctx, Instant.ofEpochMilli(state.pauseUntil))))
                MainButton(stringResource(R.string.resume), { changeState { it.resumed() } })
            } else {
                Note(stringResource(R.string.pause_text))
                PillRow(
                    listOf(
                        PauseChoice.MIN_30 to R.string.pause_30, PauseChoice.HOUR_1 to R.string.pause_60,
                        PauseChoice.HOUR_2 to R.string.pause_120, PauseChoice.UNTIL_6 to R.string.pause_morning,
                    ).map { (choice, label) ->
                        stringResource(label) to { changeState { it.withPause(schedule.pauseEnd(choice, app.clock.now()).toEpochMilli()) } }
                    },
                )
            }
        }

        Section(stringResource(R.string.more_title)) {
            if (state.moreTime(now)) Body(stringResource(R.string.extra_until, Fmt.time(ctx, Instant.ofEpochMilli(state.moreTimeUntil))))
            Note(stringResource(R.string.more_text))
            PillRow(
                listOf(15 to R.string.give_15, 30 to R.string.give_30).map { (m, label) ->
                    stringResource(label) to { changeState { it.withMoreTime(m, app.clock.now().toEpochMilli()) } }
                },
            )
        }

        ActivitySection(schedule.date(Instant.ofEpochMilli(now)))

        Section(stringResource(R.string.apps_title)) {
            Note(stringResource(R.string.apps_text))
            SecondButton(stringResource(R.string.apps_change), onApps)
        }

        Section(stringResource(R.string.times_title)) { TimesEditor(settings, edit) }

        SitesSection(settings, edit)

        Section(stringResource(R.string.dns_title)) {
            SwitchRow(stringResource(R.string.dns_switch), settings.familyDns, { on -> edit { it.copy(familyDns = on) } })
            Note(stringResource(R.string.dns_text))
        }

        Section(stringResource(R.string.checks_parent)) {
            Checks(forSetup = false, onVpn = {
                val ask = VpnService.prepare(ctx)
                if (ask == null) GuardVpnService.start(ctx) else vpnConsent.launch(ask)
            })
            // The full list for the parent (the status page, which the child sees, leaves out the adb line).
            Heading(stringResource(R.string.limits_title))
            Note(stringResource(R.string.limits_text) + " " + stringResource(R.string.limits_adb))
        }

        // Director's decision D2: Lock now here too, not only on the notice.
        Section(stringResource(R.string.lock_title)) {
            if (state.settingsUnlockedUntil > now) {
                Body(stringResource(R.string.lock_open, Fmt.time(ctx, Instant.ofEpochMilli(state.settingsUnlockedUntil))))
                MainButton(stringResource(R.string.lock_now), {
                    lockSettingsNow(ctx)
                    now = app.clock.now().toEpochMilli()
                }, Modifier.testTag("lock-now"))
            } else {
                Body(stringResource(R.string.lock_closed))
            }
        }

        ChangePinSection()

        TurnOffSection(onOff = { scope.launch { turnProtectionOff(ctx); onOff() } })

        Foot(stringResource(R.string.about))
    }
}

/** Activity (design 3.5): today (game time used, apps used most) and the last 7 days as bars with the number written. */
@Composable
private fun ActivitySection(today: LocalDate) {
    val ctx = LocalContext.current
    val app = ctx.guardApp
    val c = LocalGuardColors.current
    val usage by app.repo.usage.collectAsStateWithLifecycle()
    Section(stringResource(R.string.activity_title)) {
        val t = usage.days[today.toString()]
        GuardText(stringResource(R.string.today), style = MaterialTheme.typography.titleMedium, color = c.text)
        if (t == null || (t.poolMinutes == 0 && t.apps.isEmpty())) {
            Note(stringResource(R.string.rp_none))
        } else {
            Body(stringResource(R.string.rp_game, Fmt.minutes(ctx, t.poolMinutes)))
            if (t.apps.isNotEmpty()) {
                Note(stringResource(R.string.rp_apps))
                for ((pkg, min) in t.apps.entries.sortedByDescending { it.value }.take(5)) {
                    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                        Body(appLabel(ctx, pkg), Modifier.weight(1f))
                        GuardText(Fmt.minutes(ctx, min), Modifier.testTag("report-today-$pkg"), MaterialTheme.typography.bodyLarge, c.text)
                    }
                }
            }
            if (t.blockedLookups > 0) Note(stringResource(R.string.rp_blocked, t.blockedLookups))
        }
        GuardText(stringResource(R.string.week), style = MaterialTheme.typography.titleMedium, color = c.text)
        val days = (6 downTo 0).map { today.minusDays(it.toLong()) }
        val max = days.maxOf { usage.days[it.toString()]?.poolMinutes ?: 0 }.coerceAtLeast(1)
        for (d in days) {
            val m = usage.days[d.toString()]?.poolMinutes ?: 0
            val dayText = Fmt.day(ctx, d)
            val minText = Fmt.minutes(ctx, m)
            Column(Modifier.fillMaxWidth().clearAndSetSemantics { contentDescription = "$dayText: $minText" }, verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Row(Modifier.fillMaxWidth()) {
                    Note(dayText, Modifier.weight(1f))
                    Note(minText, color = c.text)
                }
                Box(Modifier.fillMaxWidth().height(10.dp).background(c.line, RoundedCornerShape(5.dp))) {
                    if (m > 0) Box(Modifier.fillMaxWidth(m.toFloat() / max).height(10.dp).background(c.accent, RoundedCornerShape(5.dp)))
                }
            }
        }
        Foot(stringResource(R.string.rp_note))
    }
}

/** Sites (Strings.swift sites_*): Block and Allow lists, at most 500 each. */
@Composable
private fun SitesSection(settings: Settings, edit: Edit) {
    var field by remember { mutableStateOf("") }
    var error by remember { mutableStateOf<String?>(null) }
    val bad = stringResource(R.string.site_bad)
    val full = stringResource(R.string.site_full, Settings.MAX_SITES)
    fun add(block: Boolean) {
        val host = SiteName.clean(field)
        if (host.isEmpty()) {
            error = bad
            return
        }
        val list = if (block) settings.block else settings.allow
        if (host !in list && list.size >= Settings.MAX_SITES) {
            error = full
            return
        }
        edit {
            if (block) it.copy(block = (it.block + host).distinct(), allow = it.allow - host)
            else it.copy(allow = (it.allow + host).distinct(), block = it.block - host)
        }
        field = ""
        error = null
    }
    Section(stringResource(R.string.sites_title)) {
        // The topic filter (S10), as in the extension: on for every age, the parent can switch it off.
        SwitchRow(stringResource(R.string.topics_switch), settings.topicFilter, { on -> edit { it.copy(topicFilter = on) } }, Modifier.testTag("topics-switch"))
        Note(stringResource(R.string.topics_note))
        Note(stringResource(R.string.sites_text))
        GuardField(field, { field = it; error = null }, stringResource(R.string.site_field), Modifier.testTag("site"), error, androidx.compose.ui.text.input.KeyboardType.Uri)
        PillRow(listOf(stringResource(R.string.site_block) to { add(true) }, stringResource(R.string.site_allow) to { add(false) }))
        SiteList(stringResource(R.string.blocked_list), settings.block) { h -> edit { it.copy(block = it.block - h) } }
        SiteList(stringResource(R.string.allowed_list), settings.allow) { h -> edit { it.copy(allow = it.allow - h) } }
    }
}

@Composable
private fun SiteList(title: String, sites: List<String>, onRemove: (String) -> Unit) {
    Heading(title)
    if (sites.isEmpty()) Note(stringResource(R.string.list_empty))
    for (h in sites) {
        Column(Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Body(h)
            SecondButton(stringResource(R.string.site_remove, h), { onRemove(h) }, fill = false)
        }
    }
}

/** Change PIN: the current PIN first (same lockout as the PIN screen), then the new one twice. */
@Composable
private fun ChangePinSection() {
    val ctx = LocalContext.current
    val app = ctx.guardApp
    val scope = rememberCoroutineScope()
    var current by remember { mutableStateOf("") }
    var a by remember { mutableStateOf("") }
    var b by remember { mutableStateOf("") }
    var currentError by remember { mutableStateOf<String?>(null) }
    var error by remember { mutableStateOf<String?>(null) }
    var done by remember { mutableStateOf(false) }
    val bad = stringResource(R.string.pin_bad)
    Section(stringResource(R.string.change_pin)) {
        ParentPinField(current, { current = it; currentError = null; done = false }, stringResource(R.string.pin_current), currentError, Modifier.testTag("pin-current"))
        ParentPinField(a, { a = it; error = null; done = false }, stringResource(R.string.pin1), null, Modifier.testTag("pin-new1"))
        ParentPinField(b, { b = it; error = null; done = false }, stringResource(R.string.pin2), error, Modifier.testTag("pin-new2"))
        MainButton(stringResource(R.string.save), {
            if (!Pin.valid(a) || a != b) {
                error = bad
                return@MainButton
            }
            scope.launch {
                currentError = ParentPin.check(ctx, current)
                if (currentError != null) {
                    current = ""
                    return@launch
                }
                val rec = kotlinx.coroutines.withContext(kotlinx.coroutines.Dispatchers.Default) { Pin.hash(a) }
                app.repo.updateSettings { it.copy(pin = rec) }
                current = ""
                a = ""
                b = ""
                done = true
            }
        }, Modifier.testTag("pin-save"))
        if (done) Body(stringResource(R.string.pin_changed))
    }
}

@Composable
private fun TurnOffSection(onOff: () -> Unit) {
    var confirm by remember { mutableStateOf(false) }
    GuardCard {
        if (!confirm) {
            SecondButton(stringResource(R.string.remove), { confirm = true })
        } else {
            Body(stringResource(R.string.remove_confirm))
            MainButton(stringResource(R.string.remove), onOff)
            SecondButton(stringResource(R.string.cancel), { confirm = false })
        }
    }
}

/** Apps and games, from the parent page. */
@Composable
fun AppsScreen(onBack: () -> Unit) {
    val app = LocalContext.current.guardApp
    val scope = rememberCoroutineScope()
    val settings by app.repo.settings.collectAsStateWithLifecycle()
    Page(key = "apps", bottom = { SecondButton(stringResource(R.string.done), onBack) }) {
        Title(stringResource(R.string.apps_title))
        Body(stringResource(R.string.apps_text))
        AppsEditor(settings) { f -> scope.launch { app.repo.updateSettings(f) } }
    }
}

/** After "Turn protection off". */
@Composable
fun OffScreen(onSetup: () -> Unit) {
    Page(key = "off", bottom = { MainButton(stringResource(R.string.setup_again), onSetup) }) {
        Title(stringResource(R.string.remove))
        GuardCard { Body(stringResource(R.string.removed)) }
        Foot(stringResource(R.string.about))
    }
}

