/*
 * From AdAway, https://github.com/AdAway/AdAway (commit fa16432c), app/src/main/java/org/adaway/vpn/dns/.
 * AdAway is licensed under the GNU General Public License, version 3; the original file has no header.
 * Author of the original: Bruce BUJON.
 *
 * Adapted for RaiseSteady Guard, 2026: package renamed, Timber replaced by android.util.Log; the response
 * is read into a 4096-byte buffer (EDNS answers fit) and passed on at its real length (the original passed
 * the whole 1024-byte buffer, with zero padding); the duplicated descriptor is closed with the query.
 */
package com.raisesteady.guard.vpn.adaway;

import static android.system.OsConstants.POLLIN;

import android.os.ParcelFileDescriptor;
import android.system.StructPollfd;

import java.io.IOException;
import java.net.DatagramPacket;
import java.net.DatagramSocket;
import java.util.Arrays;
import java.util.function.Consumer;

import android.util.Log;

/**
 * This class represents a DNS query.
 *
 * @author Bruce BUJON
 */
class DnsQuery implements AutoCloseable {
    private static final String TAG = "GuardVpn";
    private static final int MAX_RESPONSE_SIZE = 4096;
    /**
     * The socket used to query DNS server.
     */
    private final DatagramSocket socket;
    /**
     * The callback to call with the query response data.
     */
    private final Consumer<byte[]> callback;
    /**
     * The pollfd related to the query to poll the OS with.
     */
    private final StructPollfd pollfd;
    /**
     * The duplicated socket descriptor behind {@link #pollfd} (closed with the query).
     */
    private final ParcelFileDescriptor socketFd;
    /**
     * The query creation time, UNIX timestamp in seconds).
     */
    private final long time;

    /**
     * Constructor.
     *
     * @param socket   The socket used to query DNS server.
     * @param callback The callback to call with the query response data.
     */
    DnsQuery(DatagramSocket socket, Consumer<byte[]> callback) {
        this.socket = socket;
        this.callback = callback;
        this.time = System.currentTimeMillis() / 1000;
        this.pollfd = new StructPollfd();
        this.socketFd = ParcelFileDescriptor.fromDatagramSocket(this.socket);
        this.pollfd.fd = this.socketFd.getFileDescriptor();
        this.pollfd.events = (short) POLLIN;
    }

    /**
     * Check whether a query is older than a timestamp.
     *
     * @param timestamp The UNIX timestamp (in seconds) to compare query time with.
     * @return <code>true</code> if the query is older than the given timestamp, <code>false</code> otherwise.
     */
    boolean isOlderThan(long timestamp) {
        return this.time < timestamp;
    }

    /**
     * Get the pollfd related to the query to poll the OS with.
     * @return The pollfd related to the query to poll the OS with.
     */
    StructPollfd getPollfd() {
        return this.pollfd;
    }

    /**
     * Check whether the query is answered, meaning its socket has received data to read.
     *
     * @return {@code true} if there is data to read from {@link #socket}, {@code false} otherwise.
     */
    boolean isAnswered() {
        return (this.pollfd.revents & POLLIN) != 0;
    }

    /**
     * Read DNS query response and notify callback.
     */
    void handleResponse() {
        try {
            byte[] responseData = new byte[MAX_RESPONSE_SIZE];
            DatagramPacket responsePacket = new DatagramPacket(responseData, responseData.length);
            this.socket.receive(responsePacket);
            this.callback.accept(Arrays.copyOf(responseData, responsePacket.getLength()));
        } catch (IOException e) {
            Log.w(TAG, "Could not handle DNS response.", e);
        } finally {
            close();
        }
    }

    @Override
    public void close() {
        this.socket.close();
        try {
            this.socketFd.close();
        } catch (IOException e) {
            Log.w(TAG, "Could not close the query descriptor.", e);
        }
    }
}
