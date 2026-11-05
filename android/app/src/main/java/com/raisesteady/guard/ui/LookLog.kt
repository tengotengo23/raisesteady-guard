package com.raisesteady.guard.ui

import android.util.Log
import com.raisesteady.guard.BuildConfig

/**
 * Debug builds only: what the look tests read (design 4.6). Every text that overflowed its box
 * (GuardText) and every screen entry with its motion kind and time. Release builds record nothing.
 * The e2e tests read it with `am broadcast -n com.raisesteady.guard/.debug.DebugStatusReceiver --ez look true`.
 */
object LookLog {
    private const val TAG = "GuardLook"
    private val overflowed = LinkedHashSet<String>()
    private val motions = ArrayList<String>()

    fun overflow(text: String) {
        if (!BuildConfig.DEBUG) return
        synchronized(this) { overflowed += text.take(60) }
        Log.w(TAG, "overflow: ${text.take(60)}")
    }

    /** One screen entry: "fade" (animations removed) or "rise+fade", its set length and how long it took. */
    fun motion(kind: String, specMs: Int, tookMs: Long) {
        if (!BuildConfig.DEBUG) return
        synchronized(this) { motions += "$kind $specMs ms (took $tookMs ms)" }
        Log.i(TAG, "enter: $kind $specMs ms, took $tookMs ms")
    }

    @Synchronized fun report(): String = "overflow=${overflowed.size}${overflowed.joinToString("") { " [$it]" }}\nmotion=${motions.joinToString("; ")}"

    @Synchronized fun clear() {
        overflowed.clear()
        motions.clear()
    }
}
