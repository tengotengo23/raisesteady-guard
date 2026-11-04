/*
 * Derived from dns66:
 * Copyright (C) 2016-2019 Julian Andres Klode <jak@jak-linux.org>
 *
 * Derived from AdBuster:
 * Copyright (C) 2016 Daniel Brodie <dbrodie@gmail.com>
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, version 3.
 *
 * Contributions shall also be provided under any later versions of the
 * GPL.
 *
 * Taken from AdAway, https://github.com/AdAway/AdAway (commit fa16432c), app/src/main/java/org/adaway/vpn/dns/.
 * Adapted for RaiseSteady Guard, 2026: package renamed, Timber replaced by android.util.Log. The block,
 * allow and redirect decision (getHostEntry and AdAway's database) is replaced by a Resolver that
 * answers the whole DNS message (com.raisesteady.guard.dns.GuardResolver). Every UDP packet to port 53
 * in the tunnel is answered, whatever its address (the fake DNS servers and DoH server addresses);
 * every other packet (TCP, QUIC, other ports) is dropped, so encrypted DNS fails closed. Empty UDP
 * packets are dropped instead of forwarded. Answers may come from another thread.
 */
package com.raisesteady.guard.vpn.adaway;

import android.util.Log;

import org.pcap4j.packet.IpPacket;
import org.pcap4j.packet.IpSelector;
import org.pcap4j.packet.IpV4Packet;
import org.pcap4j.packet.IpV6Packet;
import org.pcap4j.packet.Packet;
import org.pcap4j.packet.UdpPacket;
import org.pcap4j.packet.UnknownPacket;
import org.pcap4j.packet.namednumber.IpNumber;

import java.io.IOException;
import java.net.Inet4Address;
import java.net.Inet6Address;
import java.util.function.Consumer;

/**
 * Creates and parses packets, and sends packets to a remote socket or the device using VpnWorker.
 */
public class DnsPacketProxy {
    private static final String TAG = "GuardVpn";
    private static final int DNS_PORT = 53;

    private final EventLoop eventLoop;
    private final Resolver resolver;

    public DnsPacketProxy(EventLoop eventLoop, Resolver resolver) {
        this.eventLoop = eventLoop;
        this.resolver = resolver;
    }

    /**
     * Handles a responsePayload from an upstream DNS server
     *
     * @param requestPacket   The original request packet
     * @param responsePayload The payload of the response
     */
    public void handleDnsResponse(IpPacket requestPacket, byte[] responsePayload) {
        UdpPacket udpOutPacket = (UdpPacket) requestPacket.getPayload();
        UdpPacket.Builder payLoadBuilder = new UdpPacket.Builder(udpOutPacket)
                .srcPort(udpOutPacket.getHeader().getDstPort())
                .dstPort(udpOutPacket.getHeader().getSrcPort())
                .srcAddr(requestPacket.getHeader().getDstAddr())
                .dstAddr(requestPacket.getHeader().getSrcAddr())
                .correctChecksumAtBuild(true)
                .correctLengthAtBuild(true)
                .payloadBuilder(
                        new UnknownPacket.Builder().rawData(responsePayload)
                );

        IpPacket ipOutPacket;
        if (requestPacket instanceof IpV4Packet) {
            ipOutPacket = new IpV4Packet.Builder((IpV4Packet) requestPacket)
                    .srcAddr((Inet4Address) requestPacket.getHeader().getDstAddr())
                    .dstAddr((Inet4Address) requestPacket.getHeader().getSrcAddr())
                    .correctChecksumAtBuild(true)
                    .correctLengthAtBuild(true)
                    .payloadBuilder(payLoadBuilder)
                    .build();

        } else {
            ipOutPacket = new IpV6Packet.Builder((IpV6Packet) requestPacket)
                    .srcAddr((Inet6Address) requestPacket.getHeader().getDstAddr())
                    .dstAddr((Inet6Address) requestPacket.getHeader().getSrcAddr())
                    .correctLengthAtBuild(true)
                    .payloadBuilder(payLoadBuilder)
                    .build();
        }

        this.eventLoop.queueDeviceWrite(ipOutPacket);
    }

    /**
     * Handles a DNS request: the resolver answers it (blocked, rewritten or from upstream).
     *
     * @param packetData The packet data to read
     * @throws IOException If some network error occurred
     */
    public void handleDnsRequest(byte[] packetData) throws IOException {
        IpPacket ipPacket;
        try {
            ipPacket = (IpPacket) IpSelector.newPacket(packetData, 0, packetData.length);
        } catch (Exception e) {
            Log.i(TAG, "handleDnsRequest: Discarding invalid IP packet", e);
            return;
        }

        // Only UDP: TCP (DNS over TCP, DoH, DoT) and anything else to a routed address is dropped.
        if (ipPacket.getHeader().getProtocol() != IpNumber.UDP) {
            return;
        }

        UdpPacket updPacket;
        Packet udpPayload;

        try {
            updPacket = (UdpPacket) ipPacket.getPayload();
            udpPayload = updPacket.getPayload();
        } catch (Exception e) {
            Log.i(TAG, "handleDnsRequest: Discarding unknown packet type", e);
            return;
        }

        // Only DNS: QUIC (DoH over HTTP/3), DoQ and other UDP to a routed address is dropped.
        if (updPacket.getHeader().getDstPort().valueAsInt() != DNS_PORT) {
            return;
        }

        if (udpPayload == null) {
            return;
        }

        byte[] dnsRawData = udpPayload.getRawData();
        this.resolver.resolve(dnsRawData, data -> handleDnsResponse(ipPacket, data));
    }

    /**
     * Answers DNS messages (the decision and the upstream live outside this file).
     */
    public interface Resolver {
        /**
         * Answer one DNS query.
         *
         * @param query The DNS message from the device.
         * @param reply Called once with the answer to send back; may be called from another thread, or
         *              never (the device then retries).
         * @throws IOException If the upstream cannot be reached at all.
         */
        void resolve(byte[] query, Consumer<byte[]> reply) throws IOException;
    }

    /**
     * Interface abstracting away VpnWorker.
     */
    public interface EventLoop {
        /**
         * Forward a packet to the VPN underlying network.
         *
         * @param packet   The packet to forward.
         * @param callback The callback to call with the packet response data.
         * @throws IOException If the packet could not be forwarded.
         */
        void forwardPacket(java.net.DatagramPacket packet, Consumer<byte[]> callback) throws IOException;

        /**
         * Write an IP packet to the local TUN device. Safe to call from any thread.
         *
         * @param packet The packet to write (a response to a DNS request)
         */
        void queueDeviceWrite(IpPacket packet);
    }
}
