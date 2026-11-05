package com.raisesteady.guard.ui

/**
 * Which screen may follow which (pure, for the tests). The parent session is short-lived: the
 * parent page and what is reached from it ("apps", "off") come only after the PIN screen, are
 * never restored after the app was in the background, recreated or restarted, and close after
 * [IDLE_MS] without a touch. Once setup is done, its steps cannot be reached again (only step 12's
 * "Turn protection on" leads on to the last step, 13). Step 13 counts as part of the parent
 * session (its "Fix" unlocks the guarded Settings pages): it too ends on leaving and after
 * [IDLE_MS] (security review, 5 Nov).
 */
object Routes {
    const val STATUS = "status"
    const val PIN = "pin"
    const val PARENT = "parent"
    const val APPS = "apps"
    const val OFF = "off"
    const val FIRST_STEP = "setup:1"

    /** The parent page closes after 5 minutes without a touch. */
    const val IDLE_MS = 5 * 60_000L

    /** Routes behind the parent PIN. */
    fun gated(route: String) = route == PARENT || route == APPS || route == OFF

    fun isSetup(route: String) = route.startsWith("setup:")

    /** Screens that end with the parent session: the PIN-gated ones and, once setup is done, any setup step. */
    fun privileged(route: String, setupDone: Boolean) = gated(route) || (setupDone && isSetup(route))

    fun home(setupDone: Boolean) = if (setupDone) STATUS else FIRST_STEP

    /** The route to show when [wanted] is asked for while [current] is shown. */
    fun next(current: String, wanted: String, setupDone: Boolean): String = when {
        isSetup(wanted) && setupDone -> if (current == "setup:12" && wanted == "setup:13") wanted else STATUS
        wanted == PARENT -> if (current == PIN || current == APPS || current == PARENT) wanted else PIN
        wanted == APPS -> if (current == PARENT || current == APPS) wanted else PIN
        wanted == OFF -> if (current == PARENT || current == OFF) wanted else STATUS
        else -> wanted
    }

    /** The route after the activity was recreated or the process restored: never one behind the PIN. */
    fun restored(saved: String?, setupDone: Boolean): String = when {
        saved == null || gated(saved) -> home(setupDone)
        isSetup(saved) && setupDone -> STATUS
        else -> saved
    }

    /** Where the app goes when it leaves the screen (onStop): the parent session ends. */
    fun onLeave(current: String, setupDone: Boolean): String =
        if (privileged(current, setupDone) || current == PIN) home(setupDone) else current

    /** Whether the parent page (or the last setup step after setup) has been left untouched too long. */
    fun idle(current: String, setupDone: Boolean, lastTouchMs: Long, nowMs: Long): Boolean =
        privileged(current, setupDone) && nowMs - lastTouchMs >= IDLE_MS
}
