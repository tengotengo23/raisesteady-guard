/*
 * From AdAway, https://github.com/AdAway/AdAway (commit fa16432c), app/src/main/java/org/adaway/vpn/worker/.
 * AdAway is licensed under the GNU General Public License, version 3; the original file has no header.
 * Author of the original: Bruce BUJON.
 *
 * Adapted for RaiseSteady Guard, 2026: package renamed, Timber replaced by android.util.Log; the restart
 * goes through a callback to our own service (was VpnServiceControls).
 */
package com.raisesteady.guard.vpn.adaway;

import static java.lang.Integer.parseInt;
import static java.util.Objects.requireNonNull;

import java.net.NetworkInterface;
import java.net.SocketException;
import java.util.Enumeration;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

import android.util.Log;

/**
 * This class monitors the VPN network interface is still up while the VPN is running.
 *
 * @author Bruce BUJON (bruce.bujon(at)gmail(dot)com)
 */
public class VpnConnectionMonitor {
    private static final int CONNECTION_CHECK_DELAY_MS = 10_000;
    private static final Pattern TUNNEL_PATTERN = Pattern.compile("tun([0-9]+)");
    private static final String TAG = "GuardVpn";
    /**
     * Called when the interface went down: the service starts the VPN again.
     */
    private final Runnable restart;
    /**
     * Whether the monitor is running (<code>true</code> if running, <code>false</code> if stopped).
     */
    private final AtomicBoolean running;
    /**
     * The network interface to monitor (<code>null</code> if not initialized).
     */
    private NetworkInterface networkInterface;

    /**
     * Constructor.
     *
     * @param restart Called when the interface went down.
     */
    VpnConnectionMonitor(Runnable restart) {
        this.restart = restart;
        this.running = new AtomicBoolean(true);
        this.networkInterface = null;
    }

    private static NetworkInterface findVpnNetworkInterface() {
        try {
            NetworkInterface vpnNetworkInterface = null;
            Enumeration<NetworkInterface> networkInterfaces = NetworkInterface.getNetworkInterfaces();
            while (networkInterfaces.hasMoreElements()) {
                NetworkInterface networkInterface = networkInterfaces.nextElement();
                vpnNetworkInterface = pickLastVpnNetworkInterface(vpnNetworkInterface, networkInterface);
            }
            if (vpnNetworkInterface == null) {
                throw new IllegalStateException("Failed to find a network interface.");
            }
            return vpnNetworkInterface;
        } catch (SocketException e) {
            throw new IllegalStateException("Failed to find VPN network interface.", e);
        }
    }

    private static NetworkInterface pickLastVpnNetworkInterface(NetworkInterface current, NetworkInterface other) {
        // Ensure other is a tunnel interface, otherwise picks current
        Matcher otherMatcher = TUNNEL_PATTERN.matcher(other.getName());
        if (!otherMatcher.matches()) {
            return current;
        }
        // If other is a tunnel interface and no current interface, picks other.
        if (current == null) {
            return other;
        }
        // Ensure current is still a tunnel interface (it happens to change), otherwise picks other
        Matcher currentMatcher = TUNNEL_PATTERN.matcher(current.getName());
        if (!currentMatcher.matches()) {
            Log.e(TAG, "Current interface " + current.getName() + " is no more a tunnel interface.");
            return other;
        }
        // Compare current and ot then pick the last one
        int currentTunnelNumber = parseInt(requireNonNull(currentMatcher.group(1)));
        int otherTunnelNumber = parseInt(requireNonNull(otherMatcher.group(1)));
        return otherTunnelNumber > currentTunnelNumber ? other : current;
    }

    /**
     * Initialize the monitor once the VPN connection is up.
     */
    void initialize() {
        Log.d(TAG, "Initializing connection monitor…");
        this.networkInterface = findVpnNetworkInterface();
        Log.d(TAG, "Connection monitor initialized to watch interface " + this.networkInterface.getName() + ".");
    }

    /**
     * Monitor the VPN network interface is still up while the VPN is running.
     */
    void monitor() {
        try {
            while (this.running.get()) {
                if (this.networkInterface != null && !this.networkInterface.isUp()) {
                    stop();
                    Log.i(TAG, "VPN network interface "
                            + (this.networkInterface == null ? "unset" : this.networkInterface.getName())
                            + " is down. Starting VPN service…");
                    this.restart.run();
                }
                try {
                    Thread.sleep(CONNECTION_CHECK_DELAY_MS);
                } catch (InterruptedException e) {
                    Log.d(TAG, "Stop monitoring.");
                    Thread.currentThread().interrupt();
                    break;
                }
            }
        } catch (SocketException e) {
            Log.w(TAG, "Failed to test the VPN network interface. Starting VPN service…", e);
            reset();
            this.restart.run();
        }
    }

    /**
     * Reset the connection monitor if the VPN connection changed.
     */
    void reset() {
        this.networkInterface = null;
    }

    /**
     * Stop the monitor.
     */
    void stop() {
        this.running.set(false);
    }
}
