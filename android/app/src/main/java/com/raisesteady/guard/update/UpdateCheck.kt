package com.raisesteady.guard.update

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import org.bouncycastle.math.ec.rfc8032.Ed25519
import java.io.InputStream
import java.io.OutputStream
import java.security.MessageDigest
import java.util.Base64
import kotlin.random.Random

/** Where updates come from and the key that signs them (P2-260 spec). Debug builds may swap it for tests. */
data class UpdateConfig(val manifestUrl: String, val publicKey: ByteArray, val urlPrefix: String) {
    val sigUrl: String get() = "$manifestUrl.sig"

    companion object {
        const val PREFIX = "https://github.com/tengotengo23/raisesteady-guard/releases/download/"
        val RELEASE = UpdateConfig(
            manifestUrl = "https://github.com/tengotengo23/raisesteady-guard/releases/latest/download/update.json",
            publicKey = Base64.getDecoder().decode("HRfw+iOYKPkWrl92DL2ytpXVasidCrMbesThZ99acKQ="),
            urlPrefix = PREFIX,
        )
    }

    override fun equals(other: Any?) = other is UpdateConfig && manifestUrl == other.manifestUrl &&
        publicKey.contentEquals(other.publicKey) && urlPrefix == other.urlPrefix
    override fun hashCode() = manifestUrl.hashCode()
}

@Serializable
data class AndroidRelease(
    val name: String,
    val url: String,
    val sha256: String,
    val size: Long,
    val version: String,
    val versionCode: Long,
    @SerialName("cert_sha256") val certSha256: String,
)

@Serializable
data class UpdateManifest(val v: Int, val version: String = "", val date: String = "", val android: AndroidRelease? = null)

/**
 * The rules an update must pass (P2-260 spec), as plain functions so the JVM tests cover each one.
 * Anything that fails a rule is ignored: the running version stays.
 */
object UpdateCheck {
    /** No APK is ever accepted at or above this size. */
    const val MAX_BYTES = 64L * 1024 * 1024

    private val json = Json { ignoreUnknownKeys = true }
    private val HEX64 = Regex("^[0-9a-f]{64}$")
    private val TAG_AND_NAME = Regex("^v\\d{1,4}\\.\\d{1,4}\\.\\d{1,4}/raisesteady-guard-android-\\d{1,4}\\.\\d{1,4}\\.\\d{1,4}\\.apk$")
    private val VERSION = Regex("^(\\d{1,4})\\.(\\d{1,4})\\.(\\d{1,4})$")

    sealed class Verdict {
        /** A newer, signed APK to download; [limit] is the most bytes the download may have. */
        data class Offer(val apk: AndroidRelease, val limit: Long) : Verdict()
        /** Nothing to do (no newer version). Not a failure. */
        data class UpToDate(val note: String) : Verdict()
        /** The manifest broke a rule: ignored, logged, counted as a failure for the back-off. */
        data class Reject(val note: String) : Verdict()
    }

    /** Ed25519 over the exact bytes of update.json; [sigText] is the base64 of update.json.sig. */
    fun signatureValid(body: ByteArray, sigText: String, publicKey: ByteArray): Boolean {
        if (publicKey.size != Ed25519.PUBLIC_KEY_SIZE) return false
        val sig = try {
            Base64.getDecoder().decode(sigText.trim())
        } catch (_: IllegalArgumentException) {
            return false
        }
        if (sig.size != Ed25519.SIGNATURE_SIZE) return false
        return try {
            Ed25519.verify(sig, 0, publicKey, 0, body, 0, body.size)
        } catch (_: RuntimeException) {
            false
        }
    }

    /** "1.2.3" to its versionCode (major*1e6 + minor*1e3 + patch, as app/build.gradle.kts), or null. */
    fun versionCode(version: String): Long? {
        val m = VERSION.matchEntire(version) ?: return null
        val (a, b, c) = m.destructured
        if (b.toInt() > 999 || c.toInt() > 999) return null
        return a.toLong() * 1_000_000 + b.toLong() * 1_000 + c.toLong()
    }

    /** The URL is a release asset of the Guard repository: the prefix, then exactly "vX.Y.Z/raisesteady-guard-android-A.B.C.apk". */
    fun urlAllowed(url: String, prefix: String): Boolean {
        if (!url.startsWith(prefix)) return false
        val rest = url.substring(prefix.length)
        return TAG_AND_NAME.matches(rest)
    }

    /**
     * Every request and redirect hop (S4): https, no user part, and a host of GitHub's release downloads
     * (github.com or *.githubusercontent.com). A debug test source may also use its own host and scheme;
     * release builds always use UpdateConfig.RELEASE, so there the rule is the plain one.
     */
    fun hopAllowed(url: String, config: UpdateConfig): Boolean {
        val u = runCatching { java.net.URI(url) }.getOrNull() ?: return false
        if (u.rawUserInfo != null) return false
        val host = u.host?.lowercase() ?: return false
        val scheme = u.scheme?.lowercase() ?: return false
        if (scheme == "https" && (host == "github.com" || host.endsWith(".githubusercontent.com"))) return true
        // The configured source itself (only differs from GitHub in debug test builds), same scheme as configured.
        return listOf(config.manifestUrl, config.urlPrefix).any { own ->
            val o = runCatching { java.net.URI(own) }.getOrNull()
            o != null && o.host?.lowercase() == host && o.scheme?.lowercase() == scheme && u.port == o.port
        }
    }

    /** Guard's own PackageInstaller sessions to abandon before a new one, so they never pile up (S3). */
    fun staleSessions(sessions: List<Pair<Int, String?>>, packageName: String): List<Int> =
        sessions.filter { it.second == packageName }.map { it.first }

    /** The most bytes a download may have: 1.5 times the stated size, and always under 64 MB. */
    fun downloadLimit(size: Long): Long = minOf(size + size / 2, MAX_BYTES - 1)

    /** Checks update.json and its signature against the installed version. */
    fun check(
        body: ByteArray,
        sigText: String,
        config: UpdateConfig,
        installedVersion: String,
        installedCode: Long,
    ): Verdict {
        if (!signatureValid(body, sigText, config.publicKey)) return Verdict.Reject("signature does not verify")
        val m = try {
            json.decodeFromString(UpdateManifest.serializer(), body.decodeToString())
        } catch (e: Exception) {
            return Verdict.Reject("manifest unreadable: ${e.javaClass.simpleName}")
        }
        if (m.v != 1) return Verdict.Reject("manifest format ${m.v} unknown")
        val a = m.android ?: return Verdict.UpToDate("no Android release in the manifest")
        val code = versionCode(a.version) ?: return Verdict.Reject("version ${a.version} unreadable")
        if (code != a.versionCode) return Verdict.Reject("versionCode ${a.versionCode} does not match ${a.version}")
        val installed = versionCode(installedVersion) ?: installedCode
        if (code <= installed || code <= installedCode) return Verdict.UpToDate("${a.version} is not newer than $installedVersion")
        if (!urlAllowed(a.url, config.urlPrefix)) return Verdict.Reject("download address not allowed")
        if (!a.url.endsWith("/" + a.name)) return Verdict.Reject("name does not match the address")
        if (a.size <= 0 || a.size >= MAX_BYTES) return Verdict.Reject("size ${a.size} out of range")
        if (!HEX64.matches(a.sha256)) return Verdict.Reject("sha256 malformed")
        if (!HEX64.matches(normalCert(a.certSha256))) return Verdict.Reject("cert_sha256 malformed")
        return Verdict.Offer(a, downloadLimit(a.size))
    }

    /** Copies at most [limit] bytes; returns the SHA-256 (hex) and length, or null when the source is longer. */
    fun copyCapped(input: InputStream, output: OutputStream, limit: Long): Pair<String, Long>? {
        val md = MessageDigest.getInstance("SHA-256")
        val buf = ByteArray(64 * 1024)
        var total = 0L
        while (true) {
            val n = input.read(buf)
            if (n < 0) break
            total += n
            if (total > limit) return null
            md.update(buf, 0, n)
            output.write(buf, 0, n)
        }
        return hex(md.digest()) to total
    }

    /** The downloaded file matches the manifest: within the limit, and the same SHA-256. */
    fun fileMatches(offer: Verdict.Offer, sha256: String, length: Long): Boolean =
        length in 1..offer.limit && sha256.equals(offer.apk.sha256, ignoreCase = true)

    /**
     * The APK has exactly one signer, its certificate's SHA-256 equals cert_sha256, and the installed
     * app is signed by that same certificate (Android refuses anything else in any case).
     */
    fun certMatches(apkCerts: List<String>, installedCerts: List<String>, expected: String): Boolean {
        val want = normalCert(expected)
        if (!HEX64.matches(want)) return false
        val apk = apkCerts.map(::normalCert).distinct()
        return apk == listOf(want) && installedCerts.map(::normalCert).contains(want)
    }

    fun normalCert(s: String): String = s.replace(":", "").trim().lowercase()

    fun sha256Hex(bytes: ByteArray): String = hex(MessageDigest.getInstance("SHA-256").digest(bytes))

    private fun hex(b: ByteArray): String = b.joinToString("") { "%02x".format(it) }
}

/** When to check next (P2-260 spec): every 6 h plus 0-30 min; after failures 1 h, then 6 h, then 24 h. */
object UpdateTiming {
    const val HOUR = 3_600_000L
    const val JITTER = 30 * 60_000L

    fun nextDelayMs(failures: Int, random: Random = Random.Default): Long = when {
        failures <= 0 -> 6 * HOUR + random.nextLong(JITTER + 1)
        failures == 1 -> HOUR
        failures == 2 -> 6 * HOUR
        else -> 24 * HOUR
    }

    /** The status after one check (S6): lastCheck moves only on success; a failure keeps its own time and count. */
    fun record(s: UpdateStatus, ok: Boolean, note: String, now: Long, random: Random = Random.Default): UpdateStatus {
        val failures = if (ok) 0 else s.failures + 1
        return s.copy(
            lastCheck = if (ok) now else s.lastCheck,
            lastFailure = if (ok) s.lastFailure else now,
            failures = failures,
            nextCheckAt = now + nextDelayMs(failures, random),
            lastNote = note,
        )
    }
}
