/*
 * From AdAway, https://github.com/AdAway/AdAway (commit fa16432c), app/src/main/java/org/adaway/vpn/worker/.
 * AdAway is licensed under the GNU General Public License, version 3; the original file has no header.
 * Author of the original: Bruce BUJON.
 *
 * Adapted for RaiseSteady Guard, 2026: package renamed, Timber replaced by android.util.Log. Only Guard
 * itself is left out of the VPN (its own upstream lookups never loop); no user exclusions, no
 * allowBypass() (an app cannot bind around the filter); routes for every DoH server address, so those
 * packets reach the filter (port 53 is answered, the rest is dropped); MTU 1500; the session and
 * configure intent come from the caller.
 */
package com.raisesteady.guard.vpn.adaway;

import static android.system.OsConstants.AF_INET;
import static android.system.OsConstants.AF_INET6;

import android.app.PendingIntent;
import android.content.pm.PackageManager;
import android.net.VpnService;
import android.os.ParcelFileDescriptor;
import android.util.Log;

import java.net.Inet4Address;
import java.net.InetAddress;
import java.util.List;

/**
 * This utility class is in charge of establishing a new VPN interface.
 *
 * @author Bruce BUJON
 */
public final class VpnBuilder {
    private static final String TAG = "GuardVpn";
    private static final int MTU = 1500;

    /**
     * What the caller adds to the tunnel: the session name shown by Android, the intent opened from the
     * system VPN dialog, and extra addresses routed into the tunnel (DoH servers).
     */
    public static final class Config {
        private final String session;
        private final PendingIntent configureIntent;
        private final List<InetAddress> routedAddresses;

        public Config(String session, PendingIntent configureIntent, List<InetAddress> routedAddresses) {
            this.session = session;
            this.configureIntent = configureIntent;
            this.routedAddresses = routedAddresses;
        }

        String session() {
            return this.session;
        }

        PendingIntent configureIntent() {
            return this.configureIntent;
        }

        List<InetAddress> routedAddresses() {
            return this.routedAddresses;
        }
    }

    /**
     * Private constructor.
     */
    private VpnBuilder() {

    }

    /**
     * Establish the VPN interface.
     *
     * @param service         The VPN service to create interface.
     * @param dnsServerMapper The DNS server mapper used to configure VPN address and routes.
     * @param config          Session name, configure intent and extra routes.
     * @return The VPN interface, or null when the app is not (or no longer) allowed to run a VPN.
     */
    public static ParcelFileDescriptor establish(VpnService service, DnsServerMapper dnsServerMapper, Config config) {
        Log.d(TAG, "Establishing VPN…");
        VpnService.Builder builder = service.new Builder();
        // Configure VPN address and DNS servers
        dnsServerMapper.configureVpn(service, builder);
        // Packets to DoH servers enter the tunnel too
        for (InetAddress address : config.routedAddresses()) {
            try {
                builder.addRoute(address, address instanceof Inet4Address ? 32 : 128);
            } catch (IllegalArgumentException e) {
                Log.w(TAG, "Skipping route " + address.getHostAddress(), e);
            }
        }
        // Guard's own traffic (its upstream DNS lookups) goes directly
        try {
            builder.addDisallowedApplication(service.getPackageName());
        } catch (PackageManager.NameNotFoundException e) {
            throw new IllegalStateException("Own package not found.", e);
        }
        // Set file descriptor in blocking mode as worker has a dedicated thread
        builder.setBlocking(true);
        // Set the VPN to unmetered
        builder.setMetered(false);
        builder.setMtu(MTU);
        // Explicitly allow both families to prevent a family being blocked if no DNS server is found with it
        builder.allowFamily(AF_INET);
        builder.allowFamily(AF_INET6);
        // Create a new interface.
        ParcelFileDescriptor pfd = builder
                .setSession(config.session())
                .setConfigureIntent(config.configureIntent())
                .establish();
        Log.i(TAG, pfd == null ? "VPN not prepared." : "VPN established.");
        return pfd;
    }
}
