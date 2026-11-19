package com.raisesteady.guard.update

import com.raisesteady.guard.update.UpdateCheck.Verdict
import org.bouncycastle.math.ec.rfc8032.Ed25519
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import java.security.SecureRandom
import java.util.Base64
import kotlin.random.Random

/** Every self-updater rule (P2-260 spec), with a test key; the release key is never needed here. */
class UpdateCheckTest {
    private val sk = ByteArray(Ed25519.SECRET_KEY_SIZE).also { Ed25519.generatePrivateKey(SecureRandom(), it) }
    private val pk = ByteArray(Ed25519.PUBLIC_KEY_SIZE).also { Ed25519.generatePublicKey(sk, 0, it, 0) }
    private val config = UpdateConfig("https://example.test/update.json", pk, UpdateConfig.PREFIX)

    private val cert = "2f1543e39da9c5b0195c2f73643d6b203d6185ac8cab5f526b5eca021817bca5"
    private val apkBytes = ByteArray(5000) { (it % 251).toByte() }
    private val apkSha = UpdateCheck.sha256Hex(apkBytes)

    private fun manifest(
        version: String = "0.5.0",
        code: Long = 5000,
        url: String = UpdateConfig.PREFIX + "v$version/raisesteady-guard-android-$version.apk",
        name: String = "raisesteady-guard-android-$version.apk",
        size: Long = apkBytes.size.toLong(),
        sha: String = apkSha,
        certSha: String = cert,
        v: Int = 1,
    ) = """{"v":$v,"version":"$version","date":"2026-11-19","android":{"name":"$name","url":"$url",""" +
        """"sha256":"$sha","size":$size,"version":"$version","versionCode":$code,"cert_sha256":"$certSha"}}"""

    private fun sign(body: ByteArray, key: ByteArray = sk): String {
        val sig = ByteArray(Ed25519.SIGNATURE_SIZE)
        Ed25519.sign(key, 0, body, 0, body.size, sig, 0)
        return Base64.getEncoder().encodeToString(sig)
    }

    private fun run(json: String, installed: String = "0.4.0", installedCode: Long = 4000): Verdict {
        val body = json.toByteArray()
        return UpdateCheck.check(body, sign(body), config, installed, installedCode)
    }

    private fun assertReject(v: Verdict) = assertTrue("expected Reject, got $v", v is Verdict.Reject)

    @Test fun goodManifestIsOffered() {
        val v = run(manifest())
        assertTrue(v is Verdict.Offer)
        v as Verdict.Offer
        assertEquals("0.5.0", v.apk.version)
        assertEquals(7500L, v.limit)
    }

    @Test fun badSignatureIsRejected() {
        val body = manifest().toByteArray()
        val otherSk = ByteArray(Ed25519.SECRET_KEY_SIZE).also { Ed25519.generatePrivateKey(SecureRandom(), it) }
        assertReject(UpdateCheck.check(body, sign(body, otherSk), config, "0.4.0", 4000))
        assertReject(UpdateCheck.check(body, "not base64 !!", config, "0.4.0", 4000))
        assertReject(UpdateCheck.check(body, "", config, "0.4.0", 4000))
        assertReject(UpdateCheck.check(body, Base64.getEncoder().encodeToString(ByteArray(10)), config, "0.4.0", 4000))
    }

    @Test fun tamperedBodyIsRejected() {
        val body = manifest().toByteArray()
        val sig = sign(body)
        val tampered = manifest(version = "0.6.0", code = 6000).toByteArray()
        assertReject(UpdateCheck.check(tampered, sig, config, "0.4.0", 4000))
        val oneByte = body.copyOf().also { it[it.size - 3] = (it[it.size - 3] + 1).toByte() }
        assertReject(UpdateCheck.check(oneByte, sig, config, "0.4.0", 4000))
        val extraSpace = (manifest() + " ").toByteArray()
        assertReject(UpdateCheck.check(extraSpace, sig, config, "0.4.0", 4000))
    }

    @Test fun signatureWithWrongKeyLengthFails() {
        val body = manifest().toByteArray()
        assertFalse(UpdateCheck.signatureValid(body, sign(body), ByteArray(31)))
    }

    @Test fun olderOrEqualVersionIsNotOffered() {
        assertTrue(run(manifest(), installed = "0.5.0", installedCode = 5000) is Verdict.UpToDate)
        assertTrue(run(manifest(), installed = "0.6.0", installedCode = 6000) is Verdict.UpToDate)
        assertTrue(run(manifest(version = "0.3.9", code = 3009)) is Verdict.UpToDate)
        // A higher installed versionCode wins even if the name looks older.
        assertTrue(run(manifest(), installed = "0.4.0", installedCode = 9000) is Verdict.UpToDate)
    }

    @Test fun versionCodeMustMatchVersion() {
        assertReject(run(manifest(code = 5001)))
        assertReject(run(manifest(version = "0.5", code = 5000)))
        assertReject(run(manifest(version = "1.1000.0", code = 2000000)))
        assertEquals(4_002_001L, UpdateCheck.versionCode("4.2.1"))
        assertNull(UpdateCheck.versionCode("1.2.3-beta"))
    }

    @Test fun wrongHostOrPathIsRejected() {
        assertReject(run(manifest(url = "https://evil.example/v0.5.0/raisesteady-guard-android-0.5.0.apk")))
        assertReject(run(manifest(url = "http://github.com/tengotengo23/raisesteady-guard/releases/download/v0.5.0/raisesteady-guard-android-0.5.0.apk")))
        assertReject(run(manifest(url = "https://github.com/someone/raisesteady-guard/releases/download/v0.5.0/raisesteady-guard-android-0.5.0.apk")))
        assertReject(run(manifest(url = UpdateConfig.PREFIX + "v0.5.0/../../x/raisesteady-guard-android-0.5.0.apk")))
        assertReject(run(manifest(url = UpdateConfig.PREFIX + "v0.5.0/raisesteady-guard-android-0.5.0.apk?x=1")))
        assertReject(run(manifest(url = "https://github.com.evil.example/tengotengo23/raisesteady-guard/releases/download/v0.5.0/raisesteady-guard-android-0.5.0.apk")))
    }

    @Test fun nameMustMatchUrl() {
        assertReject(run(manifest(name = "other.apk")))
    }

    @Test fun oversizedIsRejected() {
        assertReject(run(manifest(size = UpdateCheck.MAX_BYTES)))
        assertReject(run(manifest(size = 0)))
        assertReject(run(manifest(size = -1)))
        assertTrue(UpdateCheck.downloadLimit(60L * 1024 * 1024) < UpdateCheck.MAX_BYTES)
        assertEquals(150L, UpdateCheck.downloadLimit(100))
    }

    @Test fun downloadOverTheLimitIsDropped() {
        val offer = run(manifest(size = 1000)) as Verdict.Offer // limit 1500, file 5000
        assertNull(UpdateCheck.copyCapped(ByteArrayInputStream(apkBytes), ByteArrayOutputStream(), offer.limit))
        val ok = UpdateCheck.copyCapped(ByteArrayInputStream(apkBytes), ByteArrayOutputStream(), 5000)
        assertEquals(apkSha to 5000L, ok)
    }

    @Test fun wrongHashIsRejected() {
        val offer = run(manifest()) as Verdict.Offer
        val out = ByteArrayOutputStream()
        val got = UpdateCheck.copyCapped(ByteArrayInputStream(apkBytes), out, offer.limit)!!
        assertTrue(UpdateCheck.fileMatches(offer, got.first, got.second))
        val tampered = apkBytes.copyOf().also { it[0] = 99 }
        val bad = UpdateCheck.copyCapped(ByteArrayInputStream(tampered), ByteArrayOutputStream(), offer.limit)!!
        assertFalse(UpdateCheck.fileMatches(offer, bad.first, bad.second))
        assertFalse(UpdateCheck.fileMatches(offer, got.first, 0))
        assertReject(run(manifest(sha = "abc")))
    }

    @Test fun certificateRules() {
        assertTrue(UpdateCheck.certMatches(listOf(cert), listOf(cert), cert))
        assertTrue(UpdateCheck.certMatches(listOf(cert.uppercase()), listOf(cert), cert.chunked(2).joinToString(":")))
        val other = "0".repeat(64)
        assertFalse(UpdateCheck.certMatches(listOf(other), listOf(cert), cert)) // wrong signer
        assertFalse(UpdateCheck.certMatches(listOf(cert, other), listOf(cert), cert)) // extra signer
        assertFalse(UpdateCheck.certMatches(listOf(cert), listOf(other), cert)) // installed app signed otherwise
        assertFalse(UpdateCheck.certMatches(emptyList(), listOf(cert), cert))
        assertFalse(UpdateCheck.certMatches(listOf(cert), listOf(cert), "nope"))
        assertReject(run(manifest(certSha = "nope")))
    }

    @Test fun unreadableOrUnknownFormatIsRejected() {
        assertReject(run("not json"))
        assertReject(run(manifest(v = 2)))
        assertTrue(run("""{"v":1,"version":"0.5.0"}""") is Verdict.UpToDate)
    }

    @Test fun timingFollowsTheSpec() {
        val h = UpdateTiming.HOUR
        repeat(50) {
            val d = UpdateTiming.nextDelayMs(0, Random(it))
            assertTrue(d in 6 * h..6 * h + 30 * 60_000L)
        }
        assertEquals(h, UpdateTiming.nextDelayMs(1))
        assertEquals(6 * h, UpdateTiming.nextDelayMs(2))
        assertEquals(24 * h, UpdateTiming.nextDelayMs(3))
        assertEquals(24 * h, UpdateTiming.nextDelayMs(10))
    }

    @Test fun redirectHopsKeepTheHostRules() {
        val rel = UpdateConfig.RELEASE
        assertTrue(UpdateCheck.hopAllowed(rel.manifestUrl, rel))
        assertTrue(UpdateCheck.hopAllowed("https://objects.githubusercontent.com/github-production-release-asset/x?sig=1", rel))
        assertTrue(UpdateCheck.hopAllowed("https://release-assets.githubusercontent.com/a/b", rel))
        assertFalse(UpdateCheck.hopAllowed("http://objects.githubusercontent.com/x", rel)) // https to http
        assertFalse(UpdateCheck.hopAllowed("http://github.com/x", rel))
        assertFalse(UpdateCheck.hopAllowed("https://evil.example/x", rel)) // another host
        assertFalse(UpdateCheck.hopAllowed("https://githubusercontent.com.evil.example/x", rel))
        assertFalse(UpdateCheck.hopAllowed("https://evilgithubusercontent.com/x", rel))
        assertFalse(UpdateCheck.hopAllowed("https://user:pw@github.com/x", rel)) // userinfo
        assertFalse(UpdateCheck.hopAllowed("ftp://github.com/x", rel))
        assertFalse(UpdateCheck.hopAllowed("not a url", rel))
        // A debug test source: its own host and scheme only, nothing else.
        val dbg = UpdateConfig("http://10.0.2.2:8000/update.json", pk, "http://10.0.2.2:8000/dl/")
        assertTrue(UpdateCheck.hopAllowed("http://10.0.2.2:8000/dl/v0.5.0/a.apk", dbg))
        assertFalse(UpdateCheck.hopAllowed("http://10.0.2.2:9000/x", dbg))
        assertFalse(UpdateCheck.hopAllowed("http://github.com/x", dbg))
    }

    @Test fun oldInstallerSessionsAreAbandoned() {
        val sessions = listOf(1 to "com.raisesteady.guard", 2 to "other.app", 3 to "com.raisesteady.guard", 4 to null)
        assertEquals(listOf(1, 3), UpdateCheck.staleSessions(sessions, "com.raisesteady.guard"))
        assertTrue(UpdateCheck.staleSessions(emptyList(), "com.raisesteady.guard").isEmpty())
    }

    @Test fun lastCheckMovesOnlyOnSuccess() {
        val h = UpdateTiming.HOUR
        val ok1 = UpdateTiming.record(UpdateStatus(), true, "up to date", 1000, Random(1))
        assertEquals(1000L, ok1.lastCheck)
        assertEquals(0, ok1.failures)
        val bad1 = UpdateTiming.record(ok1, false, "check failed", 5000)
        assertEquals(1000L, bad1.lastCheck) // the parent never sees a fresh time for a failed check
        assertEquals(5000L, bad1.lastFailure)
        assertEquals(1, bad1.failures)
        assertEquals(5000 + h, bad1.nextCheckAt)
        val bad2 = UpdateTiming.record(bad1, false, "check failed", 9000)
        assertEquals(1000L, bad2.lastCheck)
        assertEquals(2, bad2.failures)
        val ok2 = UpdateTiming.record(bad2, true, "up to date", 20000, Random(2))
        assertEquals(20000L, ok2.lastCheck)
        assertEquals(0, ok2.failures)
        assertEquals(9000L, ok2.lastFailure)
        assertEquals(0L, UpdateTiming.record(UpdateStatus(), false, "x", 7).lastCheck) // never succeeded: "not yet"
    }

    @Test fun releaseConfigIsTheSpecOne() {
        assertEquals(32, UpdateConfig.RELEASE.publicKey.size)
        assertTrue(UpdateConfig.RELEASE.manifestUrl.startsWith("https://github.com/tengotengo23/raisesteady-guard/"))
        assertEquals(UpdateConfig.RELEASE.manifestUrl + ".sig", UpdateConfig.RELEASE.sigUrl)
    }
}
