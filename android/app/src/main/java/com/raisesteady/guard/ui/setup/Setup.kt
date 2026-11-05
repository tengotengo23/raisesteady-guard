package com.raisesteady.guard.ui.setup

import android.Manifest
import android.app.Activity
import android.net.VpnService
import android.os.Build
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.raisesteady.guard.R
import com.raisesteady.guard.data.Settings
import com.raisesteady.guard.guard.Health
import com.raisesteady.guard.pin.Pin
import com.raisesteady.guard.ui.Pages
import com.raisesteady.guard.ui.components.Body
import com.raisesteady.guard.ui.components.ChoiceGroup
import com.raisesteady.guard.ui.components.ChoiceRow
import com.raisesteady.guard.ui.components.Foot
import com.raisesteady.guard.ui.components.GuardCard
import com.raisesteady.guard.ui.components.LinkButton
import com.raisesteady.guard.ui.components.MainButton
import com.raisesteady.guard.ui.components.Mark
import com.raisesteady.guard.ui.components.Note
import com.raisesteady.guard.ui.components.Page
import com.raisesteady.guard.ui.components.ParentPinField
import com.raisesteady.guard.ui.components.SecondButton
import com.raisesteady.guard.ui.components.StatusLine
import com.raisesteady.guard.ui.components.SwitchRow
import com.raisesteady.guard.ui.components.Title
import com.raisesteady.guard.ui.guardApp
import com.raisesteady.guard.ui.open
import com.raisesteady.guard.ui.parent.AppsEditor
import com.raisesteady.guard.ui.parent.Checks
import com.raisesteady.guard.ui.parent.BedtimeEditor
import com.raisesteady.guard.ui.rememberHealth
import com.raisesteady.guard.vpn.GuardVpnService
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

const val STEPS = 13

/**
 * The setup (design 4.1), on the child's device, by the parent: 13 steps, each with "Step n of 13"
 * and Back. A permission step reads its part each second and moves on by itself when it turns on.
 * Settings are stored as the parent goes; "Turn protection on" (step 12) marks setup done.
 */
@Composable
fun SetupFlow(step: Int, onStep: (Int) -> Unit, onDone: () -> Unit) {
    val next = { onStep(step + 1) }
    val back = { onStep(step - 1) }
    when (step) {
        1 -> Welcome(next)
        2 -> Terms(step, back, next)
        3 -> Age(step, back, next)
        4 -> PinStep(step, back, next)
        5 -> FamilyDns(step, back, next)
        6 -> VpnStep(step, back, next)
        7 -> NotificationsStep(step, back, next)
        8 -> UsageStep(step, back, next)
        9 -> AccessibilityStep(step, back, next)
        10 -> AdminStep(step, back, next)
        11 -> AlwaysOnStep(step, back, next)
        12 -> AppsStep(step, back, next)
        else -> ChecksStep(step, back, onDone)
    }
}

/** The frame of steps 2 to 13: "Step n of 13", Back, the content, the buttons underneath. */
@Composable
private fun Step(step: Int, onBack: () -> Unit, buttons: @Composable ColumnScope.() -> Unit, content: @Composable ColumnScope.() -> Unit) {
    Page(key = "setup-$step", bottom = buttons) {
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            LinkButton(stringResource(R.string.nav_back), onBack)
            Foot(stringResource(R.string.step_of, step, STEPS), Modifier.weight(1f).testTag("step"))
        }
        content()
    }
}

@Composable
private fun Welcome(next: () -> Unit) {
    val ctx = LocalContext.current
    Page(key = "setup-1", bottom = { MainButton(stringResource(R.string.start), next) }) {
        Foot(stringResource(R.string.step_of, 1, STEPS), Modifier.testTag("step"))
        Body(stringResource(R.string.app_name))
        Title(stringResource(R.string.welcome_title))
        Body(stringResource(R.string.welcome_text))
        GuardCard { Body(stringResource(R.string.welcome_free)) }
        Foot(stringResource(R.string.about))
        LinkButton(stringResource(R.string.about_link), { open(ctx, Pages.readme(ctx)) })
    }
}

@Composable
private fun Terms(step: Int, back: () -> Unit, next: () -> Unit) {
    val ctx = LocalContext.current
    val lang = LocalConfiguration.current.locales[0].language.takeIf { it in setOf("ka", "ru") } ?: "en"
    val text = remember(lang) {
        runCatching { ctx.assets.open("terms-$lang.txt").use { it.reader().readText() } }.getOrDefault("")
    }
    Step(step, back, { MainButton(stringResource(R.string.agree), next) }) {
        Title(stringResource(R.string.terms_title))
        GuardCard {
            for (para in text.split("\n").map { it.trim() }.filter { it.isNotEmpty() }) {
                if (para.startsWith("# ")) com.raisesteady.guard.ui.components.Heading(para.removePrefix("# ")) else Body(para)
            }
        }
    }
}

@Composable
private fun Age(step: Int, back: () -> Unit, next: () -> Unit) {
    val ctx = LocalContext.current
    val app = ctx.guardApp
    val scope = rememberCoroutineScope()
    val settings by app.repo.settings.collectAsStateWithLifecycle()
    var picked by rememberSaveable { mutableStateOf(if (settings.pin != null) settings.bandId else null) }
    Step(step, back, {
        MainButton(stringResource(R.string.next), {
            val band = app.bands.byId(picked ?: return@MainButton)
            scope.launch {
                app.repo.updateSettings { Settings.forBand(band).copy(pin = it.pin, familyDns = it.familyDns) }
                next()
            }
        }, enabled = picked != null)
    }) {
        Title(stringResource(R.string.age_title))
        ChoiceGroup {
            for (b in app.bands.bands) {
                ChoiceRow(stringResource(R.string.band, b.minAge, b.maxAge), picked == b.id, { picked = b.id }, Modifier.testTag("band-${b.id}"))
            }
        }
        Note(stringResource(R.string.age_note))
    }
}

@Composable
private fun PinStep(step: Int, back: () -> Unit, next: () -> Unit) {
    val ctx = LocalContext.current
    val app = ctx.guardApp
    val scope = rememberCoroutineScope()
    var a by remember { mutableStateOf("") }
    var b by remember { mutableStateOf("") }
    var error by remember { mutableStateOf<String?>(null) }
    val bad = stringResource(R.string.pin_bad)
    Step(step, back, {
        MainButton(stringResource(R.string.next), {
            if (!Pin.valid(a) || a != b) {
                error = bad
                return@MainButton
            }
            scope.launch {
                val rec = withContext(Dispatchers.Default) { Pin.hash(a) }
                app.repo.updateSettings { it.copy(pin = rec) }
                next()
            }
        })
    }) {
        Title(stringResource(R.string.pin_title))
        Body(stringResource(R.string.pin_hint))
        ParentPinField(a, { a = it; error = null }, stringResource(R.string.pin1), null, Modifier.testTag("pin1"))
        ParentPinField(b, { b = it; error = null }, stringResource(R.string.pin2), error, Modifier.testTag("pin2"))
    }
}

@Composable
private fun FamilyDns(step: Int, back: () -> Unit, next: () -> Unit) {
    val app = LocalContext.current.guardApp
    val scope = rememberCoroutineScope()
    val settings by app.repo.settings.collectAsStateWithLifecycle()
    Step(step, back, { MainButton(stringResource(R.string.next), next) }) {
        Title(stringResource(R.string.dns_title))
        SwitchRow(stringResource(R.string.dns_switch), settings.familyDns, { on -> scope.launch { app.repo.updateSettings { it.copy(familyDns = on) } } })
        Body(stringResource(R.string.dns_text))
    }
}

/**
 * A step that asks for one Android permission: its "why" (the prominent disclosure), what to do,
 * and the button. When the part is on, a tick and Next; it moves on by itself when it turns on here.
 */
@Composable
private fun PermissionStep(
    step: Int,
    back: () -> Unit,
    next: () -> Unit,
    title: Int,
    granted: (Health) -> Boolean,
    action: String,
    onAction: () -> Unit,
    second: Pair<String, () -> Unit>? = null,
    content: @Composable ColumnScope.() -> Unit,
) {
    val health by rememberHealth()
    val on = health?.let(granted) == true
    var seenOff by remember { mutableStateOf(false) }
    LaunchedEffect(health) {
        val h = health ?: return@LaunchedEffect
        if (!granted(h)) seenOff = true else if (seenOff) next()
    }
    Step(step, back, {
        if (on) MainButton(stringResource(R.string.next), next) else MainButton(action, onAction)
        second?.let { (label, go) -> if (!on) SecondButton(label, go) }
    }) {
        Title(stringResource(title))
        content()
        if (on) StatusLine(Mark.OK, stringResource(R.string.is_on))
    }
}

@Composable
private fun VpnStep(step: Int, back: () -> Unit, next: () -> Unit) {
    val ctx = LocalContext.current
    val consent = rememberLauncherForActivityResult(ActivityResultContracts.StartActivityForResult()) { r ->
        if (r.resultCode == Activity.RESULT_OK) GuardVpnService.start(ctx)
    }
    PermissionStep(step, back, next, R.string.vpn_title, { it.vpnRunning }, stringResource(R.string.vpn_button), {
        val ask = VpnService.prepare(ctx)
        if (ask == null) GuardVpnService.start(ctx) else consent.launch(ask)
    }) {
        Body(stringResource(R.string.vpn_why))
        Note(stringResource(R.string.vpn_see))
    }
}

@Composable
private fun NotificationsStep(step: Int, back: () -> Unit, next: () -> Unit) {
    val ctx = LocalContext.current
    val ask = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { ok ->
        if (!ok) open(ctx, Pages.notifications(ctx))
    }
    PermissionStep(step, back, next, R.string.notif_title, { it.notifications }, stringResource(R.string.allow), {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) ask.launch(Manifest.permission.POST_NOTIFICATIONS) else open(ctx, Pages.notifications(ctx))
    }) {
        Body(stringResource(R.string.notif_why))
    }
}

@Composable
private fun UsageStep(step: Int, back: () -> Unit, next: () -> Unit) {
    val ctx = LocalContext.current
    PermissionStep(step, back, next, R.string.usage_title, { it.usageAccess }, stringResource(R.string.open_settings), { open(ctx, *Pages.usageAccess(ctx)) }) {
        Body(stringResource(R.string.usage_why))
        Note(stringResource(R.string.usage_steps))
    }
}

@Composable
private fun AccessibilityStep(step: Int, back: () -> Unit, next: () -> Unit) {
    val ctx = LocalContext.current
    PermissionStep(
        step, back, next, R.string.a11y_title, { it.accessibility }, stringResource(R.string.a11y_agree), { open(ctx, *Pages.accessibility(ctx)) },
        second = stringResource(R.string.not_now) to next,
    ) {
        Body(stringResource(R.string.a11y_description))
        Note(stringResource(R.string.a11y_steps))
        GuardCard {
            Note(stringResource(R.string.a11y_restricted))
            SecondButton(stringResource(R.string.open_app_info), { open(ctx, Pages.appInfo(ctx)) })
        }
    }
}

@Composable
private fun AdminStep(step: Int, back: () -> Unit, next: () -> Unit) {
    val ctx = LocalContext.current
    PermissionStep(step, back, next, R.string.admin_title, { it.admin }, stringResource(R.string.turn_on), { open(ctx, Pages.addAdmin(ctx)) }) {
        Body(stringResource(R.string.admin_why))
    }
}

@Composable
private fun AlwaysOnStep(step: Int, back: () -> Unit, next: () -> Unit) {
    val ctx = LocalContext.current
    PermissionStep(
        step, back, next, R.string.always_title, { it.alwaysOn && it.lockdown }, stringResource(R.string.always_button),
        { open(ctx, Pages.vpn()) }, second = stringResource(R.string.skip_now) to next,
    ) {
        Body(stringResource(R.string.always_why))
        Note(stringResource(R.string.always_steps))
    }
}

@Composable
private fun AppsStep(step: Int, back: () -> Unit, next: () -> Unit) {
    val ctx = LocalContext.current
    val app = ctx.guardApp
    val scope = rememberCoroutineScope()
    val settings by app.repo.settings.collectAsStateWithLifecycle()
    Step(step, back, {
        MainButton(stringResource(R.string.turn_protection_on), {
            scope.launch {
                app.repo.updateSettings { it.copy(setupDone = true) }
                if (VpnService.prepare(ctx) == null) GuardVpnService.start(ctx)
                app.recheck.tryEmit(Unit)
                next()
            }
        })
    }) {
        Title(stringResource(R.string.apps_title))
        Body(stringResource(R.string.apps_text))
        BedtimeEditor(settings) { f -> scope.launch { app.repo.updateSettings(f) } }
        AppsEditor(settings) { f -> scope.launch { app.repo.updateSettings(f) } }
    }
}

@Composable
private fun ChecksStep(step: Int, back: () -> Unit, done: () -> Unit) {
    Step(step, back, { MainButton(stringResource(R.string.done), done) }) {
        Title(stringResource(R.string.checks_title))
        Checks(forSetup = true)
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Note(stringResource(R.string.advice_play))
            Note(stringResource(R.string.advice_guest))
        }
    }
}
