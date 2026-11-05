package com.raisesteady.guard.guard

import android.accessibilityservice.AccessibilityService
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.content.pm.ApplicationInfo
import android.os.Handler
import android.os.Looper
import android.os.SystemClock
import android.util.Log
import android.view.accessibility.AccessibilityEvent
import android.view.accessibility.AccessibilityNodeInfo
import android.view.accessibility.AccessibilityWindowInfo
import androidx.core.content.ContextCompat
import com.raisesteady.guard.BuildConfig
import com.raisesteady.guard.GuardApp
import com.raisesteady.guard.data.DayTotals
import com.raisesteady.guard.data.Level
import com.raisesteady.guard.rules.Policy
import com.raisesteady.guard.rules.Schedule
import com.raisesteady.guard.rules.UsageMath
import com.raisesteady.guard.rules.Verdict
import com.raisesteady.guard.vpn.Notices
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.drop
import kotlinx.coroutines.flow.merge
import kotlinx.coroutines.launch

/**
 * Sees which app is on screen (design 3.3, 3.6). Game time, bedtime, homework time and pause:
 * on each window change it asks Policy, and opens BlockActivity over a closed app; while a
 * Game-time app is in front, one timer fires at the next moment that can close it (at most 30 s
 * apart, as usage events can come late). Settings guard: covers our own pages in Settings with
 * PinGateActivity. Reads window content only in the packages of SettingsGuard.PACKAGES.
 */
class GuardAccessibilityService : AccessibilityService() {

    private val main = Handler(Looper.getMainLooper())
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
    private lateinit var app: GuardApp
    private lateinit var usage: UsageReader
    private val timeline = ForegroundTimeline()

    /** The package of the app in front (not counting the keyboard and System UI). */
    private var front: String? = null
    private var exempt: Set<String> = emptySet()
    private var transient: Set<String> = emptySet()
    private var marks = SettingsGuard.Marks.FALLBACK
    /** The title of the window in front, from its last state change (content changes carry none). */
    private var frontTitle: String? = null
    private val gameCache = HashMap<String, Boolean>()
    private var lastCoverMs = 0L
    private var rootRetries = 0
    private var lastBlock: Pair<String, Long>? = null
    private var warnedDay: String? = null
    private var lastReportMs = 0L
    private var lastBlockedCount = 0

    private val tick = Runnable { recheck() }

    private val screen = object : BroadcastReceiver() {
        override fun onReceive(context: Context, intent: Intent) {
            when (intent.action) {
                Intent.ACTION_SCREEN_OFF -> timeline.screenOff(now())
                else -> recheck()
            }
        }
    }
    private val packages = PackageReceiver { pkg ->
        gameCache.remove(pkg)
        if (pkg == front) recheck()
    }

    override fun onServiceConnected() {
        super.onServiceConnected()
        app = application as GuardApp
        usage = UsageReader(this) { app.debugClockOffsetMs.get() }
        exempt = exemptPackages()
        transient = transientPackages()
        marks = SettingsGuard.marks(this)
        restoreTimeline()
        ContextCompat.registerReceiver(
            this, screen,
            IntentFilter().apply { addAction(Intent.ACTION_SCREEN_ON); addAction(Intent.ACTION_USER_PRESENT); addAction(Intent.ACTION_SCREEN_OFF) },
            ContextCompat.RECEIVER_NOT_EXPORTED,
        )
        ContextCompat.registerReceiver(
            this, packages,
            IntentFilter(Intent.ACTION_PACKAGE_ADDED).apply { addDataScheme("package") },
            ContextCompat.RECEIVER_NOT_EXPORTED,
        )
        // Settings, state and clock changes (more time, pause, Lock now) apply at once.
        scope.launch { merge(app.recheck, app.repo.state.drop(1), app.repo.settings.drop(1)).collect { recheck() } }
        // "Turn protection off" on the parent page: the service switches itself off.
        scope.launch { app.turnOff.collect { disableSelf() } }
        main.postDelayed(saveTick, TIMELINE_MS)
        running = true
        Notices.clearLimitsOff(this)
        Log.i(TAG, "connected")
    }

    override fun onAccessibilityEvent(event: AccessibilityEvent) {
        if (!::app.isInitialized) return
        val type = event.eventType
        if (type == AccessibilityEvent.TYPE_WINDOW_CONTENT_CHANGED) {
            // Only in the guarded packages: a list there (Device admin apps, Apps) fills in after the page opens.
            val pkg = event.packageName?.toString() ?: return
            if (pkg == front && pkg in SettingsGuard.PACKAGES) contentChanged()
            return
        }
        if (type != AccessibilityEvent.TYPE_WINDOW_STATE_CHANGED && type != AccessibilityEvent.TYPE_WINDOWS_CHANGED) return
        val pkg = (if (type == AccessibilityEvent.TYPE_WINDOW_STATE_CHANGED) event.packageName else null)?.toString()
            ?: runCatching { rootInActiveWindow?.packageName?.toString() }.getOrNull()
            ?: return
        if (pkg in transient) return
        val title = if (type == AccessibilityEvent.TYPE_WINDOW_STATE_CHANGED) event.text.joinToString(" ").ifBlank { null } else null
        onFront(pkg, title)
    }

    private fun onFront(pkg: String, title: String?) {
        if (pkg != front) {
            rootRetries = 0
            frontTitle = null
        }
        if (title != null) frontTitle = title
        front = pkg
        timeline.moved(pkg, now())
        when {
            pkg == packageName -> main.removeCallbacks(tick)
            !app.repo.settings.value.setupDone -> main.removeCallbacks(tick) // nothing is limited or guarded before setup is done
            pkg in SettingsGuard.PACKAGES -> guardSettings(pkg, title)
            else -> enforce(pkg)
        }
    }

    private val contentCheck = Runnable {
        front?.takeIf { it in SettingsGuard.PACKAGES && app.repo.settings.value.setupDone }?.let {
            rootRetries = 0
            guardSettings(it, null)
        }
    }

    /** At most one look every [CONTENT_MS] while a guarded page changes. */
    private fun contentChanged() {
        if (!main.hasCallbacks(contentCheck)) main.postDelayed(contentCheck, CONTENT_MS)
    }

    /** Checks the app in front again (timer, screen on, settings or state changed). */
    private fun recheck() {
        main.removeCallbacks(tick)
        val pkg = front ?: return
        when {
            pkg == packageName -> Unit
            !app.repo.settings.value.setupDone -> Unit
            pkg in SettingsGuard.PACKAGES -> guardSettings(pkg, null)
            else -> enforce(pkg)
        }
    }

    // --- Game time, bedtime, homework, pause ---

    private fun now() = app.clock.now().toEpochMilli()

    private fun isGame(pkg: String): Boolean = gameCache.getOrPut(pkg) {
        runCatching { packageManager.getApplicationInfo(pkg, 0).category == ApplicationInfo.CATEGORY_GAME }.getOrDefault(false)
    }

    private fun level(pkg: String): Level {
        val s = app.repo.settings.value
        return app.catalog.level(pkg, isGame(pkg), s, app.bands.byId(s.bandId).minAge)
    }

    private fun enforce(pkg: String) {
        main.removeCallbacks(tick)
        val level = if (pkg in exempt) Level.ALLOWED else level(pkg)
        if (level == Level.ALLOWED) {
            if (app.gameLeftMin != null) {
                app.gameLeftMin = null
                Notices.refreshProtection(this)
            }
            return
        }
        val s = app.repo.settings.value
        val st = app.repo.state.value
        val nowI = app.clock.now()
        val schedule = Schedule(app.clock.zone())
        val policy = Policy(schedule, exempt)
        val pool = if (level == Level.GAME_TIME) poolUsedMs(schedule, nowI.toEpochMilli()) else 0L
        when (val v = policy.decide(pkg, level, nowI, s, st, pool)) {
            is Verdict.Closed -> {
                app.gameLeftMin = null
                block(pkg, v)
            }
            Verdict.Open -> {
                val dailyMs = schedule.dailyMinutes(s, nowI) * 60_000L
                val leftMs = dailyMs - pool
                app.gameLeftMin = if (st.moreTime(nowI.toEpochMilli())) null else UsageMath.minutes(leftMs.coerceAtLeast(0) + 59_999)
                warnFiveMinutes(schedule, nowI.toEpochMilli(), leftMs, st.moreTime(nowI.toEpochMilli()))
                val next = policy.nextCheck(nowI, s, st, pool).toEpochMilli() - nowI.toEpochMilli()
                main.postDelayed(tick, next.coerceIn(MIN_DELAY_MS, MAX_DELAY_MS))
            }
        }
        Notices.refreshProtection(this)
        writeReport(schedule)
    }

    /** Milliseconds of the Game time pool used today (usage events, or our own record without usage access). */
    private fun poolUsedMs(schedule: Schedule, nowMs: Long): Long {
        val day = schedule.day(java.time.Instant.ofEpochMilli(nowMs))
        val from = day.start.toEpochMilli()
        val events = if (usage.hasAccess()) usage.events(from, nowMs) else timeline.since(from)
        val intervals = UsageMath.intervals(events, nowMs)
        val games = intervals.map { it.pkg }.toSet().filter { it !in exempt && level(it) == Level.GAME_TIME }.toSet()
        return UsageMath.poolMs(intervals, games, day)
    }

    private fun warnFiveMinutes(schedule: Schedule, nowMs: Long, leftMs: Long, moreTime: Boolean) {
        val today = schedule.date(java.time.Instant.ofEpochMilli(nowMs)).toString()
        if (!moreTime && leftMs in 1..FIVE_MIN_MS && warnedDay != today) {
            warnedDay = today
            Notices.fiveMinutes(this)
        }
    }

    private fun block(pkg: String, v: Verdict.Closed) {
        val t = SystemClock.elapsedRealtime()
        val last = lastBlock
        if (last != null && last.first == pkg && t - last.second < RELAUNCH_MS) {
            // Just closed: look again when the pause between two starts is over (the app may have come back).
            main.postDelayed(tick, RELAUNCH_MS - (t - last.second) + 100)
            return
        }
        lastBlock = pkg to t
        Log.i(TAG, "closed $pkg: ${v.reason}")
        startActivity(BlockActivity.intent(this, pkg, v.reason, v.until?.toEpochMilli() ?: 0L))
    }

    /** usage.json: today's minutes, at most once a minute (design 3.5). No site names. */
    private fun writeReport(schedule: Schedule) {
        // Throttled on the uptime clock: a debug clock offset (or a clock change) never stops the writes.
        val up = SystemClock.elapsedRealtime()
        if (lastReportMs != 0L && up - lastReportMs < REPORT_MS) return
        lastReportMs = up
        val nowMs = now()
        val nowI = java.time.Instant.ofEpochMilli(nowMs)
        val day = schedule.day(nowI)
        val events = if (usage.hasAccess()) usage.events(day.start.toEpochMilli(), nowMs) else timeline.since(day.start.toEpochMilli())
        val intervals = UsageMath.intervals(events, nowMs)
        val perApp = UsageMath.perApp(intervals, day).filterKeys { it !in exempt }.mapValues { UsageMath.minutes(it.value) }.filterValues { it > 0 }
        val games = intervals.map { it.pkg }.toSet().filter { it !in exempt && level(it) == Level.GAME_TIME }.toSet()
        val pool = UsageMath.minutes(UsageMath.poolMs(intervals, games, day))
        val blocked = app.blockedLookups.get()
        val delta = (blocked - lastBlockedCount).coerceAtLeast(0)
        lastBlockedCount = blocked
        val date = schedule.date(nowI)
        scope.launch(Dispatchers.IO) {
            app.repo.updateUsage { u ->
                val old = u.days[date.toString()] ?: DayTotals()
                u.with(date.toString(), DayTotals(pool, perApp, old.blockedLookups + delta), date)
            }
        }
    }

    // --- Settings guard ---

    private fun guardSettings(pkg: String, title: String?) {
        val nowMs = now()
        val until = app.repo.state.value.settingsUnlockedUntil
        val unlocked = until > nowMs
        if (unlocked) {
            main.removeCallbacks(tick)
            main.postDelayed(tick, (until - nowMs).coerceIn(MIN_DELAY_MS, SettingsGuard.UNLOCK_MS))
            return
        }
        val win = topWindowOf(pkg)
        if (win == null) {
            // The window is not ready yet: look again shortly (a few times; content changes look again too).
            if (rootRetries++ < ROOT_RETRIES) main.postDelayed(tick, ROOT_RETRY_MS)
            return
        }
        rootRetries = 0
        val titles = listOf(title ?: frontTitle, win.second)
        // The window's own search finds our label in rows the tree walk may not reach yet.
        val found = runCatching { win.first.findAccessibilityNodeInfosByText(SettingsGuard.LABEL).isNotEmpty() }.getOrDefault(false)
        val texts = SettingsGuard.texts(win.first).let { if (found) it + SettingsGuard.LABEL else it }
        if (!SettingsGuard.shouldCover(pkg, titles, texts, marks, unlocked)) {
            if (BuildConfig.DEBUG) Log.d(TAG, "not covered: $pkg, titles $titles, texts $texts")
            return
        }
        val t = SystemClock.elapsedRealtime()
        if (t - lastCoverMs < COVER_MS) {
            main.postDelayed(contentCheck, COVER_MS - (t - lastCoverMs) + 100)
            return
        }
        lastCoverMs = t
        Log.i(TAG, "covered a page of $pkg")
        startActivity(PinGateActivity.intent(this))
    }

    /**
     * The root and title of [pkg]'s topmost application window. The active window can lag behind
     * while a page opens, so the window list is read first.
     */
    private fun topWindowOf(pkg: String): Pair<AccessibilityNodeInfo, String?>? {
        val list = runCatching { windows }.getOrNull().orEmpty()
        val w = list.filter { it.type == AccessibilityWindowInfo.TYPE_APPLICATION }
            .sortedByDescending { it.layer }
            .firstOrNull()
        val root = runCatching { w?.root }.getOrNull()
        if (root != null && root.packageName?.toString() == pkg) return root to w?.title?.toString()
        val active = runCatching { rootInActiveWindow }.getOrNull() ?: return null
        return if (active.packageName?.toString() == pkg) active to null else null
    }

    // --- Packages that are never limited ---

    private fun exemptPackages(): Set<String> = Exempt.packages(this)

    /** Windows that sit over the app in front without replacing it: the keyboard and System UI. */
    private fun transientPackages(): Set<String> = Exempt.transient(this)

    // --- The record kept without usage access (design 3.3, D3) ---

    /** Each minute: the foreground record goes to timeline.json, so it survives our process dying. */
    private val saveTick = object : Runnable {
        override fun run() {
            saveTimeline()
            main.postDelayed(this, TIMELINE_MS)
        }
    }

    private fun saveTimeline() {
        val nowMs = now()
        val from = Schedule(app.clock.zone()).dayStart(java.time.Instant.ofEpochMilli(nowMs)).toEpochMilli()
        val rec = timeline.record(from, nowMs)
        app.scope.launch(Dispatchers.IO) { app.repo.saveTimeline(rec) }
    }

    private fun restoreTimeline() {
        val nowMs = now()
        val from = Schedule(app.clock.zone()).dayStart(java.time.Instant.ofEpochMilli(nowMs)).toEpochMilli()
        val rec = kotlinx.coroutines.runBlocking(Dispatchers.IO) { app.repo.readTimeline() }
        timeline.restore(rec, from)
    }

    override fun onInterrupt() = Unit

    override fun onUnbind(intent: Intent?): Boolean {
        stop()
        return super.onUnbind(intent)
    }

    override fun onDestroy() {
        stop()
        super.onDestroy()
    }

    private fun stop() {
        if (!running) return
        running = false
        main.removeCallbacks(tick)
        main.removeCallbacks(contentCheck)
        main.removeCallbacks(saveTick)
        runCatching { saveTimeline() }
        scope.cancel()
        runCatching { unregisterReceiver(screen) }
        runCatching { unregisterReceiver(packages) }
        app.gameLeftMin = null
    }

    companion object {
        private const val TAG = "GuardA11y"
        private const val MIN_DELAY_MS = 1_000L
        private const val MAX_DELAY_MS = 30_000L
        private const val FIVE_MIN_MS = 5 * 60_000L
        private const val RELAUNCH_MS = 1_500L
        private const val COVER_MS = 1_000L
        private const val REPORT_MS = 60_000L
        private const val CONTENT_MS = 300L
        private const val ROOT_RETRIES = 5
        private const val ROOT_RETRY_MS = 300L
        private const val TIMELINE_MS = 60_000L

        /** The service is connected (the system bound it). */
        @Volatile var running = false
            private set
    }
}
