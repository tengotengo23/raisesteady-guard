package com.raisesteady.guard.data

import kotlinx.serialization.Serializable

/** Wrong PIN tries: five lock PIN checks for 15 minutes (as pin.js and the agent). */
@Serializable
data class PinLock(val fails: Int = 0, val until: Long = 0)

/** What changes while Guard runs (state.json). Instants are epoch milliseconds. */
@Serializable
data class State(
    /** Game-time apps are paused until then (0 = no pause). */
    val pauseUntil: Long = 0,
    /** More time: Game-time apps are open until then, whatever else applies (0 = none). */
    val moreTimeUntil: Long = 0,
    val pinLock: PinLock = PinLock(),
    /** Settings pages are open until then after the right PIN (0 = locked). */
    val settingsUnlockedUntil: Long = 0,
    val alwaysOnVpn: Boolean = false,
    val lockdownVpn: Boolean = false,
) {
    fun paused(now: Long) = pauseUntil > now
    fun moreTime(now: Long) = moreTimeUntil > now

    /** More time (PIN): opens Game-time apps and ends a pause for good (iOS rule). */
    fun withMoreTime(minutes: Int, now: Long) = copy(moreTimeUntil = now + minutes * 60_000L, pauseUntil = 0)

    /** Pause now (PIN): ends any more time. */
    fun withPause(until: Long) = copy(pauseUntil = until, moreTimeUntil = 0)

    fun resumed() = copy(pauseUntil = 0)
}
