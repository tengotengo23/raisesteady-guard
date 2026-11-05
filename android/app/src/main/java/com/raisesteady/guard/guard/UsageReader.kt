package com.raisesteady.guard.guard

import android.app.AppOpsManager
import android.app.usage.UsageEvents
import android.app.usage.UsageStatsManager
import android.content.Context
import android.os.Process
import com.raisesteady.guard.rules.UsageEvent
import kotlinx.serialization.Serializable

/**
 * Usage events from Android (UsageStatsManager: system-held, so they survive our process
 * dying), in Guard's time: [offsetMs] is the debug clock offset (always 0 in release builds).
 */
class UsageReader(private val context: Context, private val offsetMs: () -> Long) {

    /** Usage access ("Permit usage access") is given. */
    fun hasAccess(): Boolean = Health.usageAccess(context)

    /** Events between [fromMs] and [toMs] (Guard time). */
    fun events(fromMs: Long, toMs: Long): List<UsageEvent> {
        val off = offsetMs()
        val usm = context.getSystemService(UsageStatsManager::class.java) ?: return emptyList()
        val it = usm.queryEvents(fromMs - off, toMs - off) ?: return emptyList()
        val out = ArrayList<UsageEvent>()
        val e = UsageEvents.Event()
        while (it.hasNextEvent()) {
            it.getNextEvent(e)
            val kind = when (e.eventType) {
                UsageEvents.Event.ACTIVITY_RESUMED -> UsageEvent.Kind.RESUMED
                UsageEvents.Event.ACTIVITY_PAUSED, UsageEvents.Event.ACTIVITY_STOPPED -> UsageEvent.Kind.PAUSED
                UsageEvents.Event.SCREEN_NON_INTERACTIVE -> UsageEvent.Kind.SCREEN_OFF
                UsageEvents.Event.DEVICE_SHUTDOWN -> UsageEvent.Kind.SHUTDOWN
                else -> null
            } ?: continue
            out += UsageEvent(e.packageName ?: continue, e.className ?: "", kind, e.timeStamp + off)
        }
        return out
    }

    companion object {
        fun opAllowed(context: Context, op: String): Boolean {
            val ops = context.getSystemService(AppOpsManager::class.java) ?: return false
            @Suppress("DEPRECATION") // the check that works from API 29 to 37 for a special app access
            val mode = ops.unsafeCheckOpNoThrow(op, Process.myUid(), context.packageName)
            return mode == AppOpsManager.MODE_ALLOWED
        }
    }
}

/** One saved foreground event (timeline.json). */
@Serializable
data class SavedEvent(val pkg: String, val kind: UsageEvent.Kind, val timeMs: Long)

/** timeline.json: today's foreground record and when it was saved. */
@Serializable
data class TimelineRecord(val events: List<SavedEvent> = emptyList(), val front: String? = null, val savedMs: Long = 0)

/**
 * The accessibility service's own record of which app is in front: used to count when usage
 * access is switched off (design 3.3). Kept in memory and saved each minute to timeline.json
 * (director's decision D3), so the minutes survive our process dying. Events of today only.
 */
class ForegroundTimeline {
    private val events = ArrayList<UsageEvent>()
    private var front: String? = null

    @Synchronized fun moved(pkg: String, nowMs: Long) {
        if (pkg == front) return
        front?.let { events += UsageEvent(it, "", UsageEvent.Kind.PAUSED, nowMs) }
        events += UsageEvent(pkg, "", UsageEvent.Kind.RESUMED, nowMs)
        front = pkg
    }

    @Synchronized fun screenOff(nowMs: Long) {
        events += UsageEvent("", "", UsageEvent.Kind.SCREEN_OFF, nowMs)
        front = null
    }

    @Synchronized fun since(fromMs: Long): List<UsageEvent> {
        events.removeAll { it.timeMs < fromMs - DAY_MS }
        return events.filter { it.timeMs >= fromMs } + listOfNotNull(
            // An app already in front when the day began counts from the start of the day.
            front?.takeIf { f -> events.none { it.timeMs >= fromMs && it.pkg == f } }?.let { UsageEvent(it, "", UsageEvent.Kind.RESUMED, fromMs) },
        )
    }

    /** What to save at [nowMs]: the events since [fromMs] (the start of today) and the app in front. */
    @Synchronized fun record(fromMs: Long, nowMs: Long): TimelineRecord =
        TimelineRecord(since(fromMs).map { SavedEvent(it.pkg, it.kind, it.timeMs) }, front, nowMs)

    /**
     * Puts a saved record back after the process started again. The app that was in front counts
     * up to the moment of the save (nothing is known after it); events before [fromMs] (an older
     * day) are dropped. Events seen since the start are kept after the saved ones.
     */
    @Synchronized fun restore(rec: TimelineRecord, fromMs: Long) {
        if (rec.savedMs < fromMs) return
        val saved = rec.events.filter { it.timeMs >= fromMs }.map { UsageEvent(it.pkg, "", it.kind, it.timeMs) }.toMutableList()
        rec.front?.let { saved += UsageEvent(it, "", UsageEvent.Kind.PAUSED, rec.savedMs) }
        val newer = events.filter { it.timeMs >= rec.savedMs }
        events.clear()
        events += saved
        events += newer
    }

    private companion object {
        const val DAY_MS = 24 * 60 * 60 * 1000L
    }
}
