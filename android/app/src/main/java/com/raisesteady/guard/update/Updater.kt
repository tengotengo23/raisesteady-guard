package com.raisesteady.guard.update

import android.app.PendingIntent
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.pm.PackageInstaller
import android.content.pm.PackageManager
import android.content.pm.Signature
import android.os.Build
import android.util.Log
import androidx.work.Constraints
import androidx.work.CoroutineWorker
import androidx.work.ExistingWorkPolicy
import androidx.work.NetworkType
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import com.raisesteady.guard.BuildConfig
import com.raisesteady.guard.GuardApp
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.withContext
import okhttp3.CookieJar
import okhttp3.OkHttpClient
import okhttp3.Request
import java.io.File
import java.io.IOException
import java.security.MessageDigest
import java.util.concurrent.TimeUnit

/**
 * The self-updater (P2-260): checks update.json on GitHub, verifies it, downloads the APK and installs
 * it with PackageInstaller. Silent; no off switch; a failed or partial download never replaces anything.
 */
object Updater {
    const val TAG = "GuardUpdate"
    private const val WORK = "guard-update"

    /** The source in use: the release one, or (debug builds only) the one a test set. */
    fun config(app: GuardApp): UpdateConfig =
        if (BuildConfig.DEBUG) app.debugUpdateConfig ?: UpdateConfig.RELEASE else UpdateConfig.RELEASE

    /** At start: a check now when one is due, else keep the pending one (or make it again if it was lost). */
    fun atStart(context: Context, status: UpdateStatus, now: Long) {
        schedule(context, maxOf(0L, status.nextCheckAt - now), ExistingWorkPolicy.KEEP)
    }

    /** Debug builds and tests: check now. */
    fun checkNow(context: Context) = schedule(context, 0L, ExistingWorkPolicy.REPLACE)

    private fun schedule(context: Context, delayMs: Long, policy: ExistingWorkPolicy) {
        val req = OneTimeWorkRequestBuilder<UpdateWorker>()
            .setInitialDelay(delayMs, TimeUnit.MILLISECONDS)
            .setConstraints(Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build())
            .build()
        WorkManager.getInstance(context).enqueueUniqueWork(WORK, policy, req)
    }

    /** The next check, chained after the running one (REPLACE would cancel the worker that calls it). */
    internal fun scheduleNext(context: Context, delayMs: Long) = schedule(context, delayMs, ExistingWorkPolicy.APPEND_OR_REPLACE)

    /**
     * Plain GETs: no cookies, no cache, no identifiers; the User-Agent is only "RaiseSteadyGuard".
     * Every hop, redirects included, must pass UpdateCheck.hopAllowed (S4); https never falls back to http.
     */
    private fun client(cfg: UpdateConfig, readSeconds: Long, callSeconds: Long) = OkHttpClient.Builder()
        .cookieJar(CookieJar.NO_COOKIES)
        .cache(null)
        .followSslRedirects(false)
        .connectTimeout(10, TimeUnit.SECONDS)
        .readTimeout(readSeconds, TimeUnit.SECONDS)
        .callTimeout(callSeconds, TimeUnit.SECONDS)
        .addNetworkInterceptor { chain ->
            val req = chain.request()
            if (!UpdateCheck.hopAllowed(req.url.toString(), cfg)) throw IOException("hop to a host that is not allowed")
            chain.proceed(req.newBuilder().header("User-Agent", "RaiseSteadyGuard").build())
        }
        .build()

    /**
     * Android's confirm screen when it wants a tap (Android 11 and older, or Guard not the installer of record).
     * Held in memory only and offered on the PIN-gated parent page (Parent.kt), never as a notice on the child's
     * phone (M3). Lost with the process; the next check makes a fresh one.
     */
    val pendingInstall = MutableStateFlow<Intent?>(null)

    private fun get(url: String): Request = Request.Builder().url(url).get().build()

    private fun fetchSmall(small: OkHttpClient, url: String, max: Int = 256 * 1024): ByteArray =
        small.newCall(get(url)).execute().use { r ->
            if (!r.isSuccessful) throw IOException("HTTP ${r.code}")
            val body = r.body
            if (body.contentLength() > max) throw IOException("too large")
            val bytes = body.byteStream().readNBytesCompat(max + 1)
            if (bytes.size > max) throw IOException("too large")
            bytes
        }

    private fun java.io.InputStream.readNBytesCompat(n: Int): ByteArray {
        val out = java.io.ByteArrayOutputStream()
        val buf = ByteArray(8192)
        while (out.size() < n) {
            val k = read(buf, 0, minOf(buf.size, n - out.size()))
            if (k < 0) break
            out.write(buf, 0, k)
        }
        return out.toByteArray()
    }

    /** One check. Returns true unless it failed (network, a rule broken, the install refused at once). */
    suspend fun runOnce(app: GuardApp): Pair<Boolean, String> = withContext(Dispatchers.IO) {
        val cfg = config(app)
        val dir = File(app.noBackupFilesDir, "update").apply { mkdirs() }
        val part = File(dir, "download.part")
        part.delete() // never trust a file left from an earlier run
        try {
            val small = client(cfg, 60, 60)
            val body = fetchSmall(small, cfg.manifestUrl)
            val sig = fetchSmall(small, cfg.sigUrl, 4096).decodeToString()
            val installed = installedVersion(app)
            when (val v = UpdateCheck.check(body, sig, cfg, installed.first, installed.second)) {
                is UpdateCheck.Verdict.UpToDate -> true to v.note
                is UpdateCheck.Verdict.Reject -> false to v.note
                is UpdateCheck.Verdict.Offer -> {
                    val got = client(cfg, 60, 20 * 60).newCall(get(v.apk.url)).execute().use { r ->
                        if (!r.isSuccessful) throw IOException("HTTP ${r.code}")
                        if (r.body.contentLength() > v.limit) return@use null
                        part.outputStream().use { out -> UpdateCheck.copyCapped(r.body.byteStream(), out, v.limit) }
                    } ?: return@withContext false to "download larger than allowed"
                    if (!UpdateCheck.fileMatches(v, got.first, got.second)) return@withContext false to "download does not match sha256"
                    val apkCerts = archiveCerts(app, part) ?: return@withContext false to "APK unreadable or another package"
                    if (!UpdateCheck.certMatches(apkCerts, ownCerts(app), v.apk.certSha256)) return@withContext false to "signing certificate does not match"
                    install(app, part)
                    true to "installing ${v.apk.version}"
                }
            }
        } catch (e: Exception) {
            false to "check failed: ${e.javaClass.simpleName}"
        } finally {
            part.delete()
        }
    }

    private fun installedVersion(app: GuardApp): Pair<String, Long> {
        val p = app.packageManager.getPackageInfo(app.packageName, 0)
        return (p.versionName ?: "0.0.0") to p.longVersionCode
    }

    private fun certHashes(sigs: Array<Signature>?): List<String> =
        sigs.orEmpty().map { s -> MessageDigest.getInstance("SHA-256").digest(s.toByteArray()).joinToString("") { "%02x".format(it) } }

    /** The APK's signers, or null if it is not Guard. A signer with rotation history counts as its current certificate. */
    private fun archiveCerts(app: GuardApp, apk: File): List<String>? {
        val info = app.packageManager.getPackageArchiveInfo(apk.path, PackageManager.GET_SIGNING_CERTIFICATES) ?: return null
        if (info.packageName != app.packageName) return null
        val s = info.signingInfo ?: return null
        return certHashes(if (s.hasMultipleSigners()) s.apkContentsSigners else s.signingCertificateHistory?.takeLast(1)?.toTypedArray())
    }

    private fun ownCerts(app: GuardApp): List<String> {
        val s = app.packageManager.getPackageInfo(app.packageName, PackageManager.GET_SIGNING_CERTIFICATES).signingInfo ?: return emptyList()
        return certHashes(if (s.hasMultipleSigners()) s.apkContentsSigners else s.signingCertificateHistory)
    }

    /**
     * Hands the checked APK to PackageInstaller: silent on Android 12+ when Guard is the installer of record;
     * otherwise Android answers PENDING_USER_ACTION and the parent page offers the install (InstallResultReceiver).
     */
    private fun install(context: Context, apk: File) {
        val installer = context.packageManager.packageInstaller
        // S3: drop Guard's earlier sessions (a tap never given) so they never pile up; a fresh one follows.
        pendingInstall.value = null
        UpdateCheck.staleSessions(installer.mySessions.map { it.sessionId to it.appPackageName }, context.packageName)
            .forEach { runCatching { installer.abandonSession(it) } }
        val params = PackageInstaller.SessionParams(PackageInstaller.SessionParams.MODE_FULL_INSTALL).apply {
            setAppPackageName(context.packageName)
            setSize(apk.length())
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) setRequireUserAction(PackageInstaller.SessionParams.USER_ACTION_NOT_REQUIRED)
        }
        val id = installer.createSession(params)
        try {
            installer.openSession(id).use { session ->
                session.openWrite("base.apk", 0, apk.length()).use { out ->
                    apk.inputStream().use { it.copyTo(out) }
                    session.fsync(out)
                }
                val result = Intent(context, InstallResultReceiver::class.java).setPackage(context.packageName)
                val flags = PendingIntent.FLAG_UPDATE_CURRENT or
                    (if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) PendingIntent.FLAG_MUTABLE else 0)
                session.commit(PendingIntent.getBroadcast(context, 70, result, flags).intentSender)
            }
        } catch (e: Exception) {
            installer.abandonSession(id)
            throw e
        }
    }
}

/** Runs one check, records it and chains the next one (6 h plus 0-30 min, or the back-off). */
class UpdateWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result {
        val app = applicationContext as GuardApp
        val (ok, note) = Updater.runOnce(app)
        Log.i(Updater.TAG, note) // one line; no family data
        val now = System.currentTimeMillis()
        val s = app.updates.update { UpdateTiming.record(it, ok, note, now) }
        Updater.scheduleNext(app, s.nextCheckAt - now)
        return Result.success()
    }
}

/** PackageInstaller's answer. Success restarts Guard (BootReceiver handles MY_PACKAGE_REPLACED). */
class InstallResultReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        when (val status = intent.getIntExtra(PackageInstaller.EXTRA_STATUS, PackageInstaller.STATUS_FAILURE)) {
            PackageInstaller.STATUS_PENDING_USER_ACTION -> {
                @Suppress("DEPRECATION")
                val confirm = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                    intent.getParcelableExtra(Intent.EXTRA_INTENT, Intent::class.java)
                } else {
                    intent.getParcelableExtra(Intent.EXTRA_INTENT)
                }
                // Only the PIN-gated parent page offers it (M3); nothing appears on the child's screen.
                Log.i(Updater.TAG, "install waits for a parent")
                Updater.pendingInstall.value = confirm
            }
            PackageInstaller.STATUS_SUCCESS -> {
                Updater.pendingInstall.value = null
                Log.i(Updater.TAG, "installed")
            }
            else -> {
                Updater.pendingInstall.value = null
                Log.i(Updater.TAG, "install refused: status $status") // the running version stays
            }
        }
    }
}
