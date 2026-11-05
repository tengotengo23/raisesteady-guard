package com.raisesteady.guard.ui.status

import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.raisesteady.guard.R
import com.raisesteady.guard.guard.Health
import com.raisesteady.guard.rules.Schedule
import com.raisesteady.guard.ui.Fmt
import com.raisesteady.guard.ui.components.Body
import com.raisesteady.guard.ui.components.Foot
import com.raisesteady.guard.ui.components.GuardCard
import com.raisesteady.guard.ui.components.MainButton
import com.raisesteady.guard.ui.components.Mark
import com.raisesteady.guard.ui.components.Note
import com.raisesteady.guard.ui.components.Page
import com.raisesteady.guard.ui.components.Section
import com.raisesteady.guard.ui.components.StatusLine
import com.raisesteady.guard.ui.components.Title
import com.raisesteady.guard.ui.guardApp
import com.raisesteady.guard.ui.rememberHealth

/** Whether every part runs: the title says "Protection is on" only then (always-on VPN and the clock are warnings). */
fun Health.allOn() = vpnRunning && complete

/**
 * The status page (design 4.2): what anyone opening the app sees, the child too. Missing parts
 * first, then what is on, what the parent can see, Android's limits, and "For the parent".
 */
@Composable
fun StatusScreen(onParent: () -> Unit) {
    val ctx = LocalContext.current
    val app = ctx.guardApp
    val settings by app.repo.settings.collectAsStateWithLifecycle()
    val usage by app.repo.usage.collectAsStateWithLifecycle()
    val health by rememberHealth()
    val h = health
    val band = app.bands.byId(settings.bandId)
    Page(key = "status", bottom = { MainButton(stringResource(R.string.for_parent), onParent) }) {
        Body(stringResource(R.string.app_name))
        Title(stringResource(if (h == null || h.allOn()) R.string.status_on else R.string.status_incomplete), Modifier.testTag("status-title"))
        GuardCard {
            if (h != null) {
                if (!h.vpnRunning) StatusLine(Mark.STOP, stringResource(R.string.miss_vpn))
                if (!h.accessibility) StatusLine(Mark.STOP, stringResource(R.string.miss_a11y))
                if (!h.admin) StatusLine(Mark.STOP, stringResource(R.string.miss_admin))
                h.privateDnsHost?.let { StatusLine(Mark.STOP, stringResource(R.string.check_dns_bad, it)) }
                if (!h.usageAccess) StatusLine(Mark.WARN, stringResource(R.string.miss_usage))
                if (!h.notifications) StatusLine(Mark.WARN, stringResource(R.string.miss_notif))
                if (!(h.alwaysOn && h.lockdown)) StatusLine(Mark.WARN, stringResource(R.string.miss_always))
                if (!h.autoTime) StatusLine(Mark.WARN, stringResource(R.string.check_time_bad))
            }
            StatusLine(Mark.OK, stringResource(R.string.st_sites))
            StatusLine(Mark.OK, stringResource(R.string.st_safe))
            StatusLine(Mark.OK, stringResource(R.string.st_youtube))
            if (band.minAge < 13) StatusLine(Mark.OK, stringResource(R.string.st_social))
            val schedule = Schedule(app.clock.zone())
            val now = app.clock.now()
            val daily = schedule.dailyMinutes(settings, now)
            val used = usage.days[schedule.date(now).toString()]?.poolMinutes ?: 0
            StatusLine(Mark.OK, stringResource(R.string.st_game, daily, (daily - used).coerceAtLeast(0)))
            if (settings.bedtimeOn) {
                val night = schedule.bedtime(settings, schedule.dayKind(settings, now))
                StatusLine(Mark.OK, stringResource(R.string.st_bedtime, Fmt.clock(ctx, night.start), Fmt.clock(ctx, night.end)))
            }
            StatusLine(Mark.OK, stringResource(if (settings.familyDns) R.string.st_dns_on else R.string.st_dns_off))
        }
        Section(stringResource(R.string.parent_see_title)) { Body(stringResource(R.string.parent_see)) }
        Section(stringResource(R.string.limits_title)) { Note(stringResource(R.string.limits_text)) }
        Body(stringResource(R.string.honest))
        Foot(stringResource(R.string.about))
    }
}
