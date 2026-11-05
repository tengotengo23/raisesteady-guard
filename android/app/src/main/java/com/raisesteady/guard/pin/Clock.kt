package com.raisesteady.guard.pin

import java.time.Instant
import java.time.ZoneId

/** The time Guard uses. Debug builds may add a test offset (the e2e bedtime test); release builds never do. */
interface Clock {
    fun now(): Instant
    fun zone(): ZoneId
}

/** The device clock and time zone, plus [offsetMs] (always 0 in release builds). */
class DeviceClock(private val offsetMs: () -> Long = { 0L }) : Clock {
    override fun now(): Instant = Instant.ofEpochMilli(System.currentTimeMillis() + offsetMs())
    override fun zone(): ZoneId = ZoneId.systemDefault()
}

/** A clock for tests. */
class FixedClock(var instant: Instant, private val zone: ZoneId) : Clock {
    override fun now(): Instant = instant
    override fun zone(): ZoneId = zone
}
