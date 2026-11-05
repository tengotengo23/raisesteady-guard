package com.raisesteady.guard

import java.io.File

/** Paths for JVM tests: the generated assets (system property guard.assets) and kidguard/ itself. */
object TestData {
    val assets: File by lazy {
        val p = System.getProperty("guard.assets") ?: error("guard.assets is not set (run through Gradle)")
        File(p).also { check(it.isDirectory) { "no asset folder at $it" } }
    }

    /** kidguard/ (tests run in kidguard/android/app). */
    val kidguard: File by lazy { File(System.getProperty("user.dir") ?: ".").resolve("../..").canonicalFile }

    fun asset(name: String): File = assets.resolve(name).also { check(it.isFile) { "missing asset $name" } }

    /** The real lists, social ages and helplines, loaded once for all tests. */
    val data: com.raisesteady.guard.lists.GuardData by lazy { com.raisesteady.guard.lists.GuardData.load(assets) }
}
