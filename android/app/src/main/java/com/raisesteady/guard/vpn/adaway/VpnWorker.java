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
 * Taken from AdAway, https://github.com/AdAway/AdAway (commit fa16432c), app/src/main/java/org/adaway/vpn/worker/.
 * Adapted for RaiseSteady Guard, 2026: package renamed, Timber replaced by android.util.Log; works with
 * any android.net.VpnService plus a Listener (status and restart) and a DnsPacketProxy.Resolver; the
 * device write queue is thread-safe and a wake-up pipe is polled too, so answers from other threads
 * (DNS over HTTPS) are written at once; the watchdog stays off (as AdAway's default); stops when the VPN
 * permission is gone (establish returns null) instead of failing.
 */
package com.raisesteady.guard.vpn.adaway;

import static android.system.OsConstants.EAGAIN;
import static android.system.OsConstants.ENETUNREACH;
import static android.system.OsConstants.EPERM;
import static android.system.OsConstants.F_GETFL;
import static android.system.OsConstants.F_SETFL;
import static android.system.OsConstants.O_NONBLOCK;
import static android.system.OsConstants.POLLIN;
import static android.system.OsConstants.POLLOUT;

import android.net.VpnService;
import android.os.Build;
import android.os.ParcelFileDescriptor;
import android.system.ErrnoException;
import android.system.Os;
import android.system.StructPollfd;
import android.util.Log;

import org.pcap4j.packet.IpPacket;

import java.io.FileDescriptor;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.IOException;
import java.net.DatagramPacket;
import java.net.DatagramSocket;
import java.util.Arrays;
import java.util.Queue;
import java.util.concurrent.ConcurrentLinkedQueue;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.atomic.AtomicReference;
import java.util.function.Consumer;

/**
 * Runs the tunnel: reads packets from the device, hands DNS queries to the proxy and writes answers back.
 */
public class VpnWorker implements DnsPacketProxy.EventLoop {
    private static final String TAG = "GuardVpn";
    /**
     * Maximum packet size is constrained by the MTU, which is given as a signed short.
     */
    private static final int MAX_PACKET_SIZE = Short.MAX_VALUE;

    /**
     * The VPN states reported to the listener.
     */
    public enum Status { STARTING, RUNNING, RECONNECTING_NETWORK_ERROR, STOPPING, STOPPED }

    /**
     * The service side: status changes and restarts.
     */
    public interface Listener {
        void onVpnStatus(Status status);

        /**
         * The tunnel interface went down: start the VPN again.
         */
        void onRestartNeeded();
    }

    private final VpnService vpnService;
    private final Listener listener;
    private final VpnBuilder.Config config;
    /**
     * The queue of packets to send to the device (written from any thread).
     */
    private final Queue<byte[]> deviceWrites;
    /**
     * The queue of DNS queries.
     */
    private final DnsQueryQueue dnsQueryQueue;
    // The fake DNS addresses and the network DNS servers
    private final DnsServerMapper dnsServerMapper;
    // The object where we actually handle packets.
    private final DnsPacketProxy dnsPacketProxy;

    private final VpnConnectionThrottler connectionThrottler;
    private final VpnConnectionMonitor connectionMonitor;

    // Watch dog that checks our connection is alive.
    private final VpnWatchdog vpnWatchDog;

    /**
     * The VPN worker executor (<code>null</code> if not started).
     */
    private final AtomicReference<ExecutorService> executor;
    /**
     * The VPN network interface, (<code>null</code> if not established).
     */
    private final AtomicReference<ParcelFileDescriptor> vpnNetworkInterface;
    /**
     * Wake-up pipe: [read end, write end] (<code>null</code> while not running).
     */
    private final AtomicReference<FileDescriptor[]> wakePipe;

    /**
     * Constructor.
     *
     * @param vpnService      The VPN service, also used as {@link android.content.Context}.
     * @param listener        Gets status changes and restart requests.
     * @param resolverFactory Makes the resolver that answers DNS queries (it may send UDP through this worker).
     * @param dnsServerMapper The fake DNS addresses and the network DNS servers.
     * @param config          Session, configure intent and extra routes.
     */
    public VpnWorker(VpnService vpnService, Listener listener,
                     java.util.function.Function<DnsPacketProxy.EventLoop, DnsPacketProxy.Resolver> resolverFactory,
                     DnsServerMapper dnsServerMapper, VpnBuilder.Config config) {
        this.vpnService = vpnService;
        this.listener = listener;
        this.config = config;
        this.deviceWrites = new ConcurrentLinkedQueue<>();
        this.dnsQueryQueue = new DnsQueryQueue();
        this.dnsServerMapper = dnsServerMapper;
        this.dnsPacketProxy = new DnsPacketProxy(this, resolverFactory.apply(this));
        this.connectionThrottler = new VpnConnectionThrottler();
        this.connectionMonitor = new VpnConnectionMonitor(listener::onRestartNeeded);
        this.vpnWatchDog = new VpnWatchdog();
        this.executor = new AtomicReference<>(null);
        this.vpnNetworkInterface = new AtomicReference<>(null);
        this.wakePipe = new AtomicReference<>(null);
    }

    /**
     * Start the VPN worker.
     * Kill the current worker and restart it if already running.
     */
    public void start() {
        Log.d(TAG, "Starting VPN thread…");
        ExecutorService executor = Executors.newFixedThreadPool(2);
        executor.submit(this::work);
        executor.submit(this.connectionMonitor::monitor);
        setExecutor(executor);
        Log.i(TAG, "VPN thread started.");
    }

    /**
     * Stop the VPN worker.
     */
    public void stop() {
        Log.d(TAG, "Stopping VPN thread.");
        this.connectionMonitor.reset();
        this.connectionMonitor.stop();
        forceCloseTunnel();
        setExecutor(null);
        Log.i(TAG, "VPN thread stopped.");
    }

    /**
     * Keep track of the worker executor.<br>
     * Shut the previous one down in exists.
     *
     * @param executor The new worker executor, <code>null</code> if no executor any more.
     */
    private void setExecutor(ExecutorService executor) {
        ExecutorService oldExecutor = this.executor.getAndSet(executor);
        if (oldExecutor != null) {
            Log.d(TAG, "Shutting down VPN executor…");
            oldExecutor.shutdownNow();
            Log.d(TAG, "VPN executor shut down.");
        }
    }

    /**
     * Force close the tunnel connection.
     */
    private void forceCloseTunnel() {
        ParcelFileDescriptor networkInterface = this.vpnNetworkInterface.get();
        if (networkInterface != null) {
            try {
                networkInterface.close();
            } catch (IOException e) {
                Log.w(TAG, "Failed to close VPN network interface.", e);
            }
        }
    }

    private void work() {
        Log.d(TAG, "Starting work…");
        // Initialize the watchdog (off, as AdAway's default)
        this.vpnWatchDog.initialize(false);
        // Try connecting the vpn continuously
        while (true) {
            try {
                this.connectionThrottler.throttle();
                this.listener.onVpnStatus(Status.STARTING);
                if (!runVpn()) {
                    Log.i(TAG, "VPN permission is gone.");
                    break;
                }
                Log.i(TAG, "Told to stop");
                this.listener.onVpnStatus(Status.STOPPING);
                break;
            } catch (InterruptedException e) {
                Log.d(TAG, "Failed to wait for connexion throttling.", e);
                Thread.currentThread().interrupt();
                break;
            } catch (VpnNetworkException | IOException e) {
                if (Thread.currentThread().isInterrupted()) {
                    break;
                }
                Log.w(TAG, "Network exception in vpn thread, reconnecting…", e);
                // If an exception was thrown, notify status and try again
                this.listener.onVpnStatus(Status.RECONNECTING_NETWORK_ERROR);
            }
        }
        this.listener.onVpnStatus(Status.STOPPED);
        Log.d(TAG, "Exiting work.");
    }

    /**
     * @return false when the VPN could not be established because the app may not run a VPN any more.
     */
    private boolean runVpn() throws IOException, VpnNetworkException {
        // Allocate the buffer for a single packet.
        byte[] packet = new byte[MAX_PACKET_SIZE];

        ParcelFileDescriptor established = VpnBuilder.establish(this.vpnService, this.dnsServerMapper, this.config);
        if (established == null) {
            return false;
        }
        FileDescriptor[] pipe = openWakePipe();
        // Authenticate and configure the virtual network interface.
        try (ParcelFileDescriptor pfd = established;
             // Read and write views of the tunnel device
             FileInputStream inputStream = new FileInputStream(pfd.getFileDescriptor());
             FileOutputStream outputStream = new FileOutputStream(pfd.getFileDescriptor())) {
            // Store reference to network interface to close it externally on demand
            this.vpnNetworkInterface.set(pfd);
            this.wakePipe.set(pipe);
            // Initialize connection monitor
            this.connectionMonitor.initialize();

            // Update address to ping with default DNS server
            this.dnsServerMapper.getDefaultDnsServerAddress().ifPresent(this.vpnWatchDog::setTarget);

            // Now we are connected. Set the flag and show the message.
            this.listener.onVpnStatus(Status.RUNNING);

            // We keep forwarding packets till something goes wrong.
            boolean deviceOpened = true;
            while (deviceOpened && !Thread.currentThread().isInterrupted()) {
                deviceOpened = doOne(inputStream, outputStream, packet, pipe[0]);
            }
        } finally {
            this.wakePipe.set(null);
            closeQuietly(pipe[0]);
            closeQuietly(pipe[1]);
        }
        return true;
    }

    private static FileDescriptor[] openWakePipe() throws IOException {
        try {
            FileDescriptor[] pipe = Os.pipe();
            // A full pipe must never block a writer (Os.fcntlInt is API 30; on 29 the pipe's 64 KB
            // buffer would need 65,536 unread wake-ups first, while the loop reads after each poll).
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
                Os.fcntlInt(pipe[1], F_SETFL, Os.fcntlInt(pipe[1], F_GETFL, 0) | O_NONBLOCK);
            }
            return pipe;
        } catch (ErrnoException e) {
            throw new IOException("Failed to open the wake-up pipe.", e);
        }
    }

    private static void closeQuietly(FileDescriptor fd) {
        try {
            Os.close(fd);
        } catch (ErrnoException e) {
            Log.d(TAG, "Failed to close a pipe end.", e);
        }
    }

    private boolean doOne(FileInputStream inputStream, FileOutputStream fileOutputStream, byte[] packet,
                          FileDescriptor wakeFd) throws IOException, VpnNetworkException {
        // Create poll FD on tunnel
        StructPollfd deviceFd = new StructPollfd();
        deviceFd.fd = inputStream.getFD();
        deviceFd.events = (short) POLLIN;
        if (!this.deviceWrites.isEmpty()) {
            deviceFd.events |= (short) POLLOUT;
        }
        // Poll FD on the wake-up pipe (answers queued by other threads)
        StructPollfd wakePollFd = new StructPollfd();
        wakePollFd.fd = wakeFd;
        wakePollFd.events = (short) POLLIN;
        // Create poll FD on each DNS query socket
        StructPollfd[] queryFds = this.dnsQueryQueue.getQueryFds();
        StructPollfd[] polls = new StructPollfd[2 + queryFds.length];
        polls[0] = deviceFd;
        polls[1] = wakePollFd;
        System.arraycopy(queryFds, 0, polls, 2, queryFds.length);
        boolean deviceReadyToWrite;
        boolean deviceReadyToRead;
        try {
            int numberOfEvents = Os.poll(polls, this.vpnWatchDog.getPollTimeout());
            if (numberOfEvents == 0) {
                this.vpnWatchDog.handleTimeout();
                return true;
            }
            deviceReadyToWrite = (deviceFd.revents & POLLOUT) != 0;
            deviceReadyToRead = (deviceFd.revents & POLLIN) != 0;
            if ((wakePollFd.revents & POLLIN) != 0) {
                drain(wakeFd);
            }
        } catch (ErrnoException e) {
            throw new IOException("Failed to wait for event on file descriptors. Error number: " + e.errno, e);
        }

        // Need to do this before reading from the device, otherwise a new insertion there could
        // invalidate one of the sockets we want to read from either due to size or time out
        // constraints
        this.dnsQueryQueue.handleResponses();
        if (deviceReadyToWrite) {
            writeToDevice(fileOutputStream);
        }
        if (deviceReadyToRead) {
            return readPacketFromDevice(inputStream, packet) != -1;
        }
        return true;
    }

    /**
     * One read after POLLIN (the read end is blocking): what is left wakes the next poll at once.
     */
    private static void drain(FileDescriptor fd) {
        byte[] buffer = new byte[4096];
        try {
            Os.read(fd, buffer, 0, buffer.length);
        } catch (ErrnoException | IOException e) {
            Log.d(TAG, "Wake-up pipe read failed.", e);
        }
    }

    private void writeToDevice(FileOutputStream fileOutputStream) throws IOException {
        try {
            byte[] ipPacketData;
            while ((ipPacketData = this.deviceWrites.poll()) != null) {
                fileOutputStream.write(ipPacketData);
            }
        } catch (IOException e) {
            throw new IOException("Failed to write to tunnel output stream.", e);
        }
    }

    private int readPacketFromDevice(FileInputStream inputStream, byte[] packet) throws IOException {
        // Read the outgoing packet from the input stream.
        int length = inputStream.read(packet);
        if (length < 0) {
            Log.d(TAG, "Tunnel input stream closed.");
        } else if (length > 0) {
            byte[] readPacket = Arrays.copyOf(packet, length);
            vpnWatchDog.handlePacket(readPacket);
            dnsPacketProxy.handleDnsRequest(readPacket);
        }
        return length;
    }

    @Override
    public void forwardPacket(DatagramPacket outPacket, Consumer<byte[]> callback) throws IOException {
        DatagramSocket dnsSocket = null;
        try {
            dnsSocket = new DatagramSocket();
            // Packets to be sent to the real DNS server will need to be protected from the VPN
            this.vpnService.protect(dnsSocket);
            dnsSocket.send(outPacket);
            // Enqueue DNS query
            this.dnsQueryQueue.addQuery(dnsSocket, callback);
        } catch (IOException e) {
            if (dnsSocket != null) {
                dnsSocket.close();
            }
            if (e.getCause() instanceof ErrnoException) {
                ErrnoException errnoExc = (ErrnoException) e.getCause();
                if ((errnoExc.errno == ENETUNREACH) || (errnoExc.errno == EPERM)) {
                    throw new IOException("Cannot send message:", e);
                }
            }
            Log.w(TAG, "handleDnsRequest: Could not send packet to upstream", e);
        }
    }

    @Override
    public void queueDeviceWrite(IpPacket ipOutPacket) {
        byte[] rawData = ipOutPacket.getRawData();
        if (rawData == null) {
            return;
        }
        this.deviceWrites.add(rawData);
        FileDescriptor[] pipe = this.wakePipe.get();
        if (pipe != null) {
            try {
                Os.write(pipe[1], new byte[]{1}, 0, 1);
            } catch (ErrnoException e) {
                if (e.errno != EAGAIN) {
                    Log.w(TAG, "Failed to wake the VPN thread.", e);
                }
            } catch (IOException e) {
                Log.w(TAG, "Failed to wake the VPN thread.", e);
            }
        }
    }
}
