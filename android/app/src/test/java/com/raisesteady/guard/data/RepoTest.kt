package com.raisesteady.guard.data

import com.raisesteady.guard.guard.ForegroundTimeline
import com.raisesteady.guard.rules.UsageMath
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

/** The files Guard keeps survive the process: a new Repo on the same folder reads them back (D3). */
class RepoTest {
    @get:Rule val dir = TemporaryFolder()
    private val min = 60_000L

    @Test fun theForegroundRecordIsReadBackByANewProcess() = runBlocking {
        val t = ForegroundTimeline()
        t.moved("game", 0)
        t.moved("notes", 6 * min)
        val firstJob = SupervisorJob()
        Repo(dir.root, CoroutineScope(firstJob + Dispatchers.IO)).saveTimeline(t.record(0, 8 * min))
        firstJob.cancelAndJoin() // the process dies

        val secondJob = SupervisorJob()
        val rec = Repo(dir.root, CoroutineScope(secondJob + Dispatchers.IO)).readTimeline()
        secondJob.cancelAndJoin()
        val restarted = ForegroundTimeline().apply { restore(rec, 0) }
        val per = UsageMath.intervals(restarted.since(0), 30 * min).groupBy { it.pkg }.mapValues { e -> e.value.sumOf { it.lengthMs } }
        assertEquals(6 * min, per["game"])
        assertEquals(2 * min, per["notes"])
    }
}
