package com.raisesteady.guard.dns

import com.raisesteady.guard.vpn.adaway.DnsPacketProxy
import java.io.IOException
import java.net.DatagramPacket
import java.net.InetAddress

/**
 * The family DNS filter is off: plain DNS to the network's own servers, sent through the VPN
 * worker's query queue (AdAway's poll loop). Call it only on the VPN worker thread (the resolver is
 * called there). With no network DNS server, or a send error, the answer is null (SERVFAIL).
 * An unanswered query times out in the queue after 10 s and the device asks again.
 */
class UdpUpstream(
    private val loop: DnsPacketProxy.EventLoop,
    private val servers: () -> List<InetAddress>,
) : Upstream {
    override fun query(message: ByteArray, callback: (ByteArray?) -> Unit) {
        val server = servers().firstOrNull() ?: return callback(null)
        try {
            loop.forwardPacket(DatagramPacket(message, message.size, server, 53)) { callback(it) }
        } catch (e: IOException) {
            callback(null)
        }
    }
}
