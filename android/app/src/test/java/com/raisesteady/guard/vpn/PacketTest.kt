package com.raisesteady.guard.vpn

import com.raisesteady.guard.dns.Answers
import com.raisesteady.guard.vpn.adaway.DnsPacketProxy
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.pcap4j.packet.IpPacket
import org.pcap4j.packet.IpSelector
import org.pcap4j.packet.IpV4Packet
import org.pcap4j.packet.IpV6Packet
import org.pcap4j.packet.UdpPacket
import org.xbill.DNS.Message
import org.xbill.DNS.Rcode
import java.net.DatagramPacket
import java.net.InetAddress
import java.util.function.Consumer

/**
 * IP/UDP packets as they arrive from the tunnel (hex fixtures with real checksums) through
 * AdAway's DnsPacketProxy and a resolver that blocks everything: the reply must swap addresses
 * and ports and carry valid checksums. Non-DNS packets to routed addresses are dropped.
 */
class PacketTest {
    private val written = mutableListOf<IpPacket>()
    private val resolved = mutableListOf<ByteArray>()

    private val loop = object : DnsPacketProxy.EventLoop {
        override fun forwardPacket(packet: DatagramPacket, callback: Consumer<ByteArray>) = error("not used")
        override fun queueDeviceWrite(packet: IpPacket) { written += packet }
    }
    private val proxy = DnsPacketProxy(loop) { query, reply ->
        resolved += query
        reply.accept(Answers.nxdomain(Message(query)))
    }

    private fun hex(s: String) = ByteArray(s.length / 2) { s.substring(it * 2, it * 2 + 2).toInt(16).toByte() }

    private fun sum(bytes: ByteArray, start: Long = 0): Int {
        var s = start
        var i = 0
        while (i < bytes.size) {
            val hi = bytes[i].toInt() and 0xff
            val lo = if (i + 1 < bytes.size) bytes[i + 1].toInt() and 0xff else 0
            s += (hi shl 8) or lo
            i += 2
        }
        while (s shr 16 != 0L) s = (s and 0xffff) + (s shr 16)
        return s.toInt()
    }

    /** The one's complement sum over pseudo header + UDP segment is 0xffff when the checksum is right. */
    private fun udpChecksumOk(src: InetAddress, dst: InetAddress, udp: ByteArray): Boolean {
        val pseudo = src.address + dst.address +
            if (src.address.size == 4) byteArrayOf(0, 17, (udp.size shr 8).toByte(), udp.size.toByte())
            else byteArrayOf(0, 0, (udp.size shr 8).toByte(), udp.size.toByte(), 0, 0, 0, 17)
        return sum(pseudo + udp) == 0xffff
    }

    private fun roundTrip(hexPacket: String): IpPacket {
        val raw = hex(hexPacket)
        val request = IpSelector.newPacket(raw, 0, raw.size) as IpPacket
        proxy.handleDnsRequest(raw)
        assertEquals(1, written.size)
        val reply = written.single()
        val back = IpSelector.newPacket(reply.rawData, 0, reply.rawData.size) as IpPacket
        assertEquals(request.header.srcAddr, back.header.dstAddr)
        assertEquals(request.header.dstAddr, back.header.srcAddr)
        val qUdp = request.payload as UdpPacket
        val rUdp = back.payload as UdpPacket
        assertEquals(qUdp.header.srcPort, rUdp.header.dstPort)
        assertEquals(qUdp.header.dstPort, rUdp.header.srcPort)
        assertTrue("UDP checksum", udpChecksumOk(back.header.srcAddr, back.header.dstAddr, rUdp.rawData))
        assertEquals(rUdp.rawData.size, rUdp.header.lengthAsInt)
        val dns = Message(rUdp.payload.rawData)
        assertEquals(Message(qUdp.payload.rawData).header.id, dns.header.id)
        assertEquals(Rcode.NXDOMAIN, dns.header.rcode)
        return back
    }

    @Test fun ipv4QueryToTheFakeServer() {
        val back = roundTrip(V4) as IpV4Packet
        assertEquals(0xffff, sum(back.header.rawData)) // header checksum
        assertEquals(back.rawData.size, back.header.totalLengthAsInt)
    }

    @Test fun ipv6QueryToTheFakeServer() {
        val back = roundTrip(V6) as IpV6Packet
        assertEquals(back.rawData.size - 40, back.header.payloadLengthAsInt)
    }

    @Test fun dnsToADohServerAddressIsAnsweredByTheFilter() {
        roundTrip(DOH53)
        assertArrayEquals(resolved.single(), (IpSelector.newPacket(hex(DOH53), 0, hex(DOH53).size).payload.payload).rawData)
    }

    @Test fun tcpAndQuicToRoutedAddressesAreDropped() {
        proxy.handleDnsRequest(hex(TCP))
        proxy.handleDnsRequest(hex(QUIC))
        proxy.handleDnsRequest(byteArrayOf(0x45, 0, 0)) // not a packet
        assertTrue(written.isEmpty())
        assertTrue(resolved.isEmpty())
    }

    companion object {
        // 10.0.0.2:40000 -> 192.0.2.2:53, A example.org, id 0x1234
        const val V4 = "450000390001400040116eaf0a000002c00002029c4000350025b282123401000001000000000000076578616d706c65036f72670000010001"
        // [2001:db8::1]:40001 -> [2001:db8::2]:53, AAAA pornhub.com, id 0x4321
        const val V6 = "600000000025114020010db800000000000000000000000120010db80000000000000000000000029c4100350025d91843210100000100000000000007706f726e68756203636f6d00001c0001"
        // TCP SYN 10.0.0.2:40002 -> 192.0.2.2:53
        const val TCP = "450000280001400040066ecb0a000002c00002029c42003500000001000000005002ffff47660000"
        // UDP 10.0.0.2:40003 -> 94.140.14.15:443 (QUIC to a DoH server)
        const val QUIC = "45000031000140004011c41e0a0000025e8c0e0f9c4301bb001dab18400000000000000000000000000000000000000000"
        // UDP 10.0.0.2:40004 -> 8.8.8.8:53, A example.org, id 0x0101
        const val DOH53 = "4500003900014000401120a20a000002080808089c440035002575a4010101000001000000000000076578616d706c65036f72670000010001"
    }
}
