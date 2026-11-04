/*
 * From AdAway, https://github.com/AdAway/AdAway (commit fa16432c), app/src/main/java/org/adaway/vpn/dns/.
 * AdAway is licensed under the GNU General Public License, version 3; the original file has no header.
 * Author of the original: Bruce BUJON.
 *
 * Adapted for RaiseSteady Guard, 2026: package renamed, androidx annotation dropped.
 */
package com.raisesteady.guard.vpn.adaway;

import static java.lang.Integer.parseInt;
import static java.util.Objects.requireNonNull;

import java.net.InetAddress;
import java.net.UnknownHostException;

/**
 * This class represents a sub network.
 *
 * @author Bruce BUJON (bruce.bujon(at)gmail(dot)com)
 */
class Subnet {
    /**
     * The subnet address.
     */
    final InetAddress address;
    /**
     * The subnet prefix length.
     */
    final int prefixLength;

    /**
     * Parse a subnet string representation.
     *
     * @param subnet The subnet string representation.
     * @return The parsed subnet.
     */
    static Subnet parse(String subnet) {
        String[] parts = subnet.split("/");
        try {
            return new Subnet(InetAddress.getByName(parts[0]), parseInt(parts[1]));
        } catch (UnknownHostException e) {
            throw new IllegalStateException("Failed to parse hardcoded documentation subnet.", e);
        }
    }

    private Subnet(InetAddress address, int prefixLength) {
        this.address = requireNonNull(address, "Subnet address cannot be null.");
        this.prefixLength = prefixLength;
    }

    /**
     * Get an address of the subnet.
     *
     * @param index The host index.
     * @return An address of the subnet.
     */
    InetAddress getAddress(int index) {
        byte[] address = this.address.getAddress();
        address[address.length - 1] = (byte) (index + 1);
        try {
            return InetAddress.getByAddress(address);
        } catch (UnknownHostException e) {
            throw new IllegalStateException("Failed to create address " + index + " in subnet " + this);
        }
    }

    @Override
    public String toString() {
        return this.address + "/" + this.prefixLength;
    }
}
