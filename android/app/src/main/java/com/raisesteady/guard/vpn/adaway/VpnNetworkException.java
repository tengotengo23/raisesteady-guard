/*
 * From AdAway, https://github.com/AdAway/AdAway (commit fa16432c), app/src/main/java/org/adaway/vpn/worker/.
 * AdAway is licensed under the GNU General Public License, version 3; the original file has no header.
 *
 * Adapted for RaiseSteady Guard, 2026: package renamed.
 */
package com.raisesteady.guard.vpn.adaway;

public class VpnNetworkException extends Exception {
    public VpnNetworkException(String s) {
        super(s);
    }

    public VpnNetworkException(String s, Throwable t) {
        super(s, t);
    }

}
