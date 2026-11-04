/*
 * From AdAway, https://github.com/AdAway/AdAway (commit fa16432c), app/src/main/java/org/adaway/vpn/dns/.
 * AdAway is licensed under the GNU General Public License, version 3; the original file has no header.
 * Author of the original: Bruce BUJON.
 *
 * Adapted for RaiseSteady Guard, 2026: package renamed, Timber replaced by android.util.Log. The tunnel
 * always gets one fake IPv4 and one fake IPv6 DNS address (every query goes to our own filter, not to a
 * mapped server), each with its own route. The network's own DNS servers are kept only as the upstream
 * for "family DNS filter off", refreshed on demand; there is no hard-coded fallback server (the original
 * fell back to 1.1.1.1).
 */
package com.raisesteady.guard.vpn.adaway;

import static android.content.Context.CONNECTIVITY_SERVICE;
import static android.net.NetworkCapabilities.TRANSPORT_CELLULAR;
import static android.net.NetworkCapabilities.TRANSPORT_VPN;
import static android.net.NetworkCapabilities.TRANSPORT_WIFI;
import static java.util.Collections.emptyList;

import android.content.Context;
import android.net.ConnectivityManager;
import android.net.LinkProperties;
import android.net.Network;
import android.net.NetworkCapabilities;
import android.net.VpnService;
import android.util.Log;

import java.net.InetAddress;
import java.util.ArrayList;
import java.util.List;
import java.util.Optional;
import java.util.concurrent.CopyOnWriteArrayList;

/**
 * This class is in charge of the fake DNS addresses of the tunnel and of the network DNS servers.
 * <p>
 * Fake DNS addresses are registered as VPN interface DNS to capture DNS traffic.
 *
 * @author Bruce BUJON (bruce.bujon(at)gmail(dot)com)
 */
public class DnsServerMapper {
    private static final String TAG = "GuardVpn";
    /**
     * The TEST NET addresses blocks, defined in RFC5735.
     */
    private static final String[] TEST_NET_ADDRESS_BLOCKS = {
            "192.0.2.0/24", // TEST-NET-1
            "198.51.100.0/24", // TEST-NET-2
            "203.0.113.0/24" // TEST-NET-3
    };
    /**
     * This IPv6 address prefix for documentation, defined in RFC3849.
     */
    private static final String IPV6_ADDRESS_PREFIX_RESERVED_FOR_DOCUMENTATION = "2001:db8::/32";
    /**
     * VPN network IPv6 interface prefix length.
     */
    private static final int IPV6_PREFIX_LENGTH = 120;
    /**
     * The network DNS servers (upstream when the family DNS filter is off).
     */
    private final List<InetAddress> dnsServers;
    /**
     * The fake DNS addresses given to the tunnel (IPv4 first).
     */
    private final List<InetAddress> fakeDnsAddresses;

    /**
     * Constructor.
     */
    public DnsServerMapper() {
        this.dnsServers = new CopyOnWriteArrayList<>();
        this.fakeDnsAddresses = new CopyOnWriteArrayList<>();
    }

    /**
     * Configure the VPN.
     * <p>
     * Add one interface address and one fake DNS server per IP family, each with its own route.
     *
     * @param context The application context.
     * @param builder The builder of the VPN to configure.
     */
    public void configureVpn(Context context, VpnService.Builder builder) {
        refreshNetworkDnsServers(context);
        Subnet ipv4Subnet = addIpv4Address(builder);
        Subnet ipv6Subnet = addIpv6Address(builder);
        this.fakeDnsAddresses.clear();
        InetAddress fake4 = ipv4Subnet.getAddress(1);
        InetAddress fake6 = ipv6Subnet.getAddress(1);
        builder.addDnsServer(fake4);
        builder.addRoute(fake4, 32);
        builder.addDnsServer(fake6);
        builder.addRoute(fake6, 128);
        this.fakeDnsAddresses.add(fake4);
        this.fakeDnsAddresses.add(fake6);
        Log.i(TAG, "Fake DNS servers " + fake4.getHostAddress() + " and " + fake6.getHostAddress());
    }

    /**
     * The fake DNS addresses of the tunnel (IPv4, then IPv6).
     */
    public List<InetAddress> getFakeDnsAddresses() {
        return new ArrayList<>(this.fakeDnsAddresses);
    }

    /**
     * The first network DNS server, if any.
     */
    public Optional<InetAddress> getDefaultDnsServerAddress() {
        return this.dnsServers.isEmpty() ? Optional.empty() : Optional.of(this.dnsServers.get(0));
    }

    /**
     * The network DNS servers (a copy).
     */
    public List<InetAddress> getNetworkDnsServerAddresses() {
        return new ArrayList<>(this.dnsServers);
    }

    /**
     * Read the DNS servers of the underlying (non-VPN) network again, after a network change.
     *
     * @param context The application context.
     */
    public void refreshNetworkDnsServers(Context context) {
        List<InetAddress> servers = getNetworkDnsServers(context);
        this.dnsServers.clear();
        this.dnsServers.addAll(servers);
        Log.d(TAG, "Network DNS servers: " + servers.size());
    }

    /**
     * Get the DNS server addresses from the device networks.
     *
     * @param context The application context.
     * @return The DNS server addresses, an empty collection if no network.
     */
    private List<InetAddress> getNetworkDnsServers(Context context) {
        ConnectivityManager connectivityManager = (ConnectivityManager) context.getSystemService(CONNECTIVITY_SERVICE);
        Network activeNetwork = connectivityManager.getActiveNetwork();
        if (activeNetwork == null) {
            return getAnyNonVpnNetworkDns(connectivityManager);
        } else if (isNotVpnNetwork(connectivityManager, activeNetwork)) {
            return getNetworkDnsServers(connectivityManager, activeNetwork);
        } else {
            return getDnsFromNonVpnNetworkWithMatchingTransportType(connectivityManager, activeNetwork);
        }
    }

    /**
     * Get the DNS server addresses of any network without VPN capability.
     *
     * @param connectivityManager The connectivity manager.
     * @return The DNS server addresses, an empty collection if no applicable DNS server found.
     */
    @SuppressWarnings("deprecation") // getAllNetworks: still the way to list networks on API 29.
    private List<InetAddress> getAnyNonVpnNetworkDns(ConnectivityManager connectivityManager) {
        for (Network network : connectivityManager.getAllNetworks()) {
            if (isNotVpnNetwork(connectivityManager, network)) {
                List<InetAddress> dnsServers = getNetworkDnsServers(connectivityManager, network);
                if (!dnsServers.isEmpty()) {
                    return dnsServers;
                }
            }
        }
        return emptyList();
    }

    /**
     * Get the DNS server addresses of a network with the same transport type as the active network except VPN.
     *
     * @param connectivityManager The connectivity manager.
     * @param activeNetwork       The active network to filter similar transport type.
     * @return The DNS server addresses, an empty collection if no applicable DNS server found.
     */
    @SuppressWarnings("deprecation") // getAllNetworks: still the way to list networks on API 29.
    private List<InetAddress> getDnsFromNonVpnNetworkWithMatchingTransportType(
            ConnectivityManager connectivityManager,
            Network activeNetwork
    ) {
        // Get active network transport
        NetworkCapabilities activeNetworkCapabilities = connectivityManager.getNetworkCapabilities(activeNetwork);
        if (activeNetworkCapabilities == null) {
            return emptyList();
        }
        int activeNetworkTransport = -1;
        if (activeNetworkCapabilities.hasTransport(TRANSPORT_CELLULAR)) {
            activeNetworkTransport = TRANSPORT_CELLULAR;
        } else if (activeNetworkCapabilities.hasTransport(TRANSPORT_WIFI)) {
            activeNetworkTransport = TRANSPORT_WIFI;
        }
        // Check all network to find one without VPN and matching transport
        for (Network network : connectivityManager.getAllNetworks()) {
            NetworkCapabilities networkCapabilities = connectivityManager.getNetworkCapabilities(network);
            if (networkCapabilities == null) {
                continue;
            }
            if (networkCapabilities.hasTransport(activeNetworkTransport) && !networkCapabilities.hasTransport(TRANSPORT_VPN)) {
                List<InetAddress> dns = getNetworkDnsServers(connectivityManager, network);
                if (!dns.isEmpty()) {
                    return dns;
                }
            }
        }
        return emptyList();
    }

    /**
     * Get the DNS server addresses of a network.
     *
     * @param connectivityManager The connectivity manager.
     * @param network             The network to get DNS server addresses.
     * @return The DNS server addresses, an empty collection if no network.
     */
    private List<InetAddress> getNetworkDnsServers(ConnectivityManager connectivityManager, Network network) {
        LinkProperties linkProperties = connectivityManager.getLinkProperties(network);
        if (linkProperties == null) {
            return emptyList();
        }
        return linkProperties.getDnsServers();
    }

    /**
     * Check a network does not have VPN transport.
     *
     * @param connectivityManager The connectivity manager.
     * @param network             The network to check.
     * @return <code>true</code> if a network is not a VPN, <code>false</code> otherwise.
     */
    private boolean isNotVpnNetwork(ConnectivityManager connectivityManager, Network network) {
        if (network == null) {
            return false;
        }
        NetworkCapabilities networkCapabilities = connectivityManager.getNetworkCapabilities(network);
        return networkCapabilities != null && !networkCapabilities.hasTransport(TRANSPORT_VPN);
    }

    /**
     * Add IPv4 network address to the VPN.
     *
     * @param builder The build of the VPN to configure.
     * @return The IPv4 address of the VPN network.
     */
    private Subnet addIpv4Address(VpnService.Builder builder) {
        for (String addressBlock : TEST_NET_ADDRESS_BLOCKS) {
            try {
                Subnet subnet = Subnet.parse(addressBlock);
                InetAddress address = subnet.getAddress(0);
                builder.addAddress(address, subnet.prefixLength);
                return subnet;
            } catch (IllegalArgumentException e) {
                Log.w(TAG, "Failed to add " + addressBlock + " network address to tunnel interface.", e);
            }
        }
        throw new IllegalStateException("Failed to add any IPv4 address for TEST-NET to tunnel interface.");
    }

    /**
     * Add IPv6 network address to the VPN.
     *
     * @param builder The build of the VPN to configure.
     * @return The IPv6 address of the VPN network.
     */
    private Subnet addIpv6Address(VpnService.Builder builder) {
        Subnet subnet = Subnet.parse(IPV6_ADDRESS_PREFIX_RESERVED_FOR_DOCUMENTATION);
        builder.addAddress(subnet.address, IPV6_PREFIX_LENGTH);
        return subnet;
    }
}
