package com.raisesteady.guard.build

import groovy.json.JsonOutput
import groovy.json.JsonSlurper
import org.gradle.api.DefaultTask
import org.gradle.api.GradleException
import org.gradle.api.file.ConfigurableFileCollection
import org.gradle.api.file.DirectoryProperty
import org.gradle.api.tasks.InputFiles
import org.gradle.api.tasks.Internal
import org.gradle.api.tasks.OutputDirectory
import org.gradle.api.tasks.PathSensitive
import org.gradle.api.tasks.PathSensitivity
import org.gradle.api.tasks.TaskAction
import java.io.BufferedOutputStream
import java.io.DataOutputStream
import java.io.File

/**
 * Copies the shared Guard data (the same files Windows, Linux, iOS and the extension use) into the
 * app's assets, and packs the block lists into `lists.bin`. Reads only repo files; the output is
 * byte for byte the same for the same input. Never edit generated data by hand: change
 * kidguard/shared or tools/build-lists.mjs instead.
 *
 * lists.bin (big-endian): "RSGL", int version (1), int category count, then per category:
 * int id, int count, count x long (sorted, unique FNV-1a 64 hashes of lower-case domains).
 * Ids: adult 1, gambling 2, bypass 3, social 4, doh 5, topic 6.
 */
abstract class SharedDataTask : DefaultTask() {
    /** kidguard/ (the folder above kidguard/android). */
    @get:Internal
    abstract val kidguardDir: DirectoryProperty

    /** Every file read below (declared so the task reruns when one changes). */
    @get:InputFiles
    @get:PathSensitive(PathSensitivity.RELATIVE)
    abstract val sources: ConfigurableFileCollection

    @get:OutputDirectory
    abstract val outputDir: DirectoryProperty

    @TaskAction
    fun run() {
        if (Fnv64.hash("foobar") != FOOBAR) throw GradleException("Fnv64 self-test failed")
        val root = kidguardDir.get().asFile
        val out = outputDir.get().asFile
        out.deleteRecursively()
        out.mkdirs()
        for (rel in COPIED) {
            val src = root.resolve(rel)
            if (!src.isFile) throw GradleException("missing shared data file: $rel")
            src.copyTo(out.resolve(src.name), overwrite = true)
        }

        val categories = linkedMapOf<Int, Set<String>>()
        for ((id, name) in RULE_CATEGORIES) categories[id] = ruleDomains(root.resolve("ext/rules/$name.json"))
        val doh = json(root.resolve("shared/doh.json")) as Map<*, *>
        categories[DOH] = (doh["domains"] as List<*>).map { norm(it as String) }.toSortedSet()
        val topics = json(root.resolve("shared/topics.json")) as Map<*, *>
        categories[TOPIC] = (topics["domains"] as List<*>).map { norm(it as String) }.toSortedSet()

        // Social: the ruleset holds the feeds and their asset domains; the feeds also carry a minimum age.
        val feeds = (json(root.resolve("shared/social-domains.json")) as Map<*, *>)["feeds"] as List<*>
        val social = feeds.map { it as Map<*, *> }
            .map { norm(it["domain"] as String) to ((it["minAge"] as Number?)?.toInt() ?: 13) }
            .sortedBy { it.first }
        categories[SOCIAL] = (categories.getValue(SOCIAL) + social.map { it.first }).toSortedSet()

        // Helplines are never blocked, by any rule: fail the build if a list holds one or a parent of one.
        val helplines = (topics["helplines"] as List<*>).map { norm(it as String) }
        for ((id, domains) in categories) {
            for (h in helplines) {
                val hit = suffixes(h).firstOrNull { it in domains }
                if (hit != null) throw GradleException("category $id lists $hit, which would block the helpline $h")
            }
        }

        DataOutputStream(BufferedOutputStream(out.resolve("lists.bin").outputStream())).use { o ->
            o.write("RSGL".toByteArray(Charsets.US_ASCII))
            o.writeInt(1)
            o.writeInt(categories.size)
            for ((id, domains) in categories) {
                val hashes = domains.map { Fnv64.hash(it) }.distinct().sorted()
                if (hashes.size != domains.size) throw GradleException("hash collision in category $id")
                o.writeInt(id)
                o.writeInt(hashes.size)
                for (h in hashes) o.writeLong(h)
            }
        }
        out.resolve("social.json").writeText(
            JsonOutput.toJson(social.map { linkedMapOf("domain" to it.first, "minAge" to it.second) }) + "\n",
        )
        out.resolve("doh-ips.json").writeText(JsonOutput.toJson((doh["ips"] as List<*>).map { it as String }.sorted()) + "\n")
    }

    private fun json(f: File): Any {
        if (!f.isFile) throw GradleException("missing shared data file: $f")
        return JsonSlurper().parse(f, "UTF-8")
    }

    /** Every requestDomains entry of a ruleset (each domain is there twice: redirect and block rule). */
    private fun ruleDomains(f: File): Set<String> {
        val out = sortedSetOf<String>()
        for (rule in json(f) as List<*>) {
            val cond = (rule as Map<*, *>)["condition"] as Map<*, *>
            (cond["requestDomains"] as List<*>?)?.forEach { out += norm(it as String) }
        }
        if (out.isEmpty()) throw GradleException("no domains in $f")
        return out
    }

    private fun norm(s: String) = s.trim().lowercase().removeSuffix(".")

    /** The host and every parent with at least two labels. */
    private fun suffixes(host: String): List<String> {
        val p = host.split('.')
        return (0 until p.size - 1).map { p.subList(it, p.size).joinToString(".") }
    }

    companion object {
        const val SOCIAL = 4
        const val DOH = 5
        const val TOPIC = 6
        private const val FOOBAR = -0x7a6bbe8e08c69818L // 0x85944171f73967e8, the FNV-1a 64 test vector

        val RULE_CATEGORIES = listOf(1 to "adult", 2 to "gambling", 3 to "bypass", SOCIAL to "social")

        /** Copied as they are (paths relative to kidguard/). */
        val COPIED = listOf(
            "shared/age-bands.json",
            "shared/topics.json",
            "shared/learning-domains.json",
            "shared/android-apps.json",
            // Android's own terms: the installer's legal text with an Android privacy paragraph (review S7).
            "android/terms/terms-en.txt",
            "android/terms/terms-ka.txt",
            "android/terms/terms-ru.txt",
        )

        /** Every file the task reads. */
        val INPUTS: List<String> = COPIED + RULE_CATEGORIES.map { "ext/rules/${it.second}.json" } +
            listOf("shared/doh.json", "shared/social-domains.json")
    }
}

/** FNV-1a, 64 bit, over the UTF-8 bytes of the lower-case name. The app has the same code (lists/Fnv64.kt). */
object Fnv64 {
    private const val OFFSET = -0x340d631b7bdddcdbL // 0xcbf29ce484222325
    private const val PRIME = 0x100000001b3L

    fun hash(s: String): Long {
        var h = OFFSET
        for (b in s.lowercase().toByteArray(Charsets.UTF_8)) {
            h = h xor (b.toLong() and 0xff)
            h *= PRIME
        }
        return h
    }
}
