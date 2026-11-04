package com.raisesteady.guard.dns

import android.util.Log
import okhttp3.Call
import okhttp3.Callback
import okhttp3.Dns
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import java.io.IOException
import java.net.DatagramPacket
import java.net.DatagramSocket
import java.net.InetAddress
import java.net.UnknownHostException
import java.util.concurrent.ExecutorService
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit

/**
 * AdGuard DNS Family over DNS over HTTPS (RFC 8484, POST application/dns-message), as the agent
 * (core.go) and iOS (Model.swift). The host name is never looked up: OkHttp gets the fixed
 * bootstrap addresses. After 4 s without an answer, one retry goes as plain UDP to the same
 * addresses; if that fails too, the caller answers SERVFAIL (sites do not open, as on Windows and Linux).
 */
class DohUpstream(
    private val url: String = URL,
    private val bootstrap: List<InetAddress> = BOOTSTRAP.map { InetAddress.getByName(it) },
    timeoutMs: Long = TIMEOUT_MS,
    private val udpTimeoutMs: Int = UDP_TIMEOUT_MS,
) : Upstream {
    private val host = url.substringAfter("://").substringBefore('/')
    private val udpRetry: ExecutorService = Executors.newFixedThreadPool(2) { r -> Thread(r, "guard-dns-udp").apply { isDaemon = true } }

    private val client = OkHttpClient.Builder()
        .dns(object : Dns {
            override fun lookup(hostname: String): List<InetAddress> =
                if (hostname.equals(host, ignoreCase = true)) bootstrap else throw UnknownHostException(hostname)
        })
        .callTimeout(timeoutMs, TimeUnit.MILLISECONDS)
        .connectTimeout(timeoutMs, TimeUnit.MILLISECONDS)
        .retryOnConnectionFailure(true)
        .build()

    override fun query(message: ByteArray, callback: (ByteArray?) -> Unit) {
        val request = Request.Builder()
            .url(url)
            .header("Accept", DNS_MESSAGE)
            .post(message.toRequestBody(DNS_MESSAGE.toMediaType()))
            .build()
        client.newCall(request).enqueue(object : Callback {
            override fun onFailure(call: Call, e: IOException) {
                Log.d(TAG, "DoH failed, retrying over UDP", e)
                udpRetry.execute { callback(udp(message)) }
            }

            override fun onResponse(call: Call, response: Response) {
                val body = response.use { if (it.isSuccessful) it.body.bytes() else null }
                if (body != null && body.size >= 12) callback(body) else udpRetry.execute { callback(udp(message)) }
            }
        })
    }

    /** Plain DNS to the same servers, IPv4 first; null when none answers. */
    private fun udp(message: ByteArray): ByteArray? {
        for (server in bootstrap.sortedBy { if (it is java.net.Inet4Address) 0 else 1 }) {
            try {
                DatagramSocket().use { socket ->
                    socket.soTimeout = udpTimeoutMs
                    socket.send(DatagramPacket(message, message.size, server, 53))
                    val buf = ByteArray(4096)
                    val p = DatagramPacket(buf, buf.size)
                    socket.receive(p)
                    return buf.copyOf(p.length)
                }
            } catch (e: IOException) {
                Log.d(TAG, "UDP to ${server.hostAddress} failed", e)
            }
        }
        return null
    }

    fun shutdown() {
        udpRetry.shutdownNow()
        client.dispatcher.executorService.shutdown()
        client.connectionPool.evictAll()
    }

    companion object {
        private const val TAG = "GuardDns"
        const val URL = "https://family.adguard-dns.com/dns-query"

        /** guard-ios/Shared/Model.swift:9. */
        val BOOTSTRAP = listOf("94.140.14.15", "94.140.15.16", "2a10:50c0::bad1:ff", "2a10:50c0::bad2:ff")
        const val TIMEOUT_MS = 4_000L
        const val UDP_TIMEOUT_MS = 2_000
        private const val DNS_MESSAGE = "application/dns-message"
    }
}
