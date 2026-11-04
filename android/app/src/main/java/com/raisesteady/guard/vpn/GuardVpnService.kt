package com.raisesteady.guard.vpn

import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.net.ConnectivityManager
import android.net.LinkProperties
import android.net.Network
import android.net.VpnService
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.util.Log
import androidx.core.app.ServiceCompat
import androidx.core.content.ContextCompat
import com.raisesteady.guard.GuardApp
import com.raisesteady.guard.R
import com.raisesteady.guard.dns.DnsDecider
import com.raisesteady.guard.dns.DohUpstream
import com.raisesteady.guard.dns.GuardResolver
import com.raisesteady.guard.dns.UdpUpstream
import com.raisesteady.guard.vpn.adaway.DnsServerMapper
import com.raisesteady.guard.vpn.adaway.VpnBuilder
import com.raisesteady.guard.vpn.adaway.VpnWorker
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow

/**
 * The site filter: a local VPN that only carries DNS (and packets to known DoH servers). Runs as a
 * foreground service of type systemExempted with the persistent notification (design 3.1, 4.5).
 */
class GuardVpnService : VpnService(), VpnWorker.Listener {

    /** What the status page reads. */
    data class Status(val running: Boolean = false, val alwaysOn: Boolean = false, val lockdown: Boolean = false)

    private val main = Handler(Looper.getMainLooper())
    private val mapper = DnsServerMapper()
    private var worker: VpnWorker? = null
    private var doh: DohUpstream? = null
    private var networkCallback: ConnectivityManager.NetworkCallback? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == ACTION_STOP) {
            stopFilter()
            ServiceCompat.stopForeground(this, ServiceCompat.STOP_FOREGROUND_REMOVE)
            stopSelf()
            return START_NOT_STICKY
        }
        goForeground()
        if (prepare(this) != null) {
            // The VPN permission is not (or no longer) given: nothing to run.
            Notices.filterOff(this)
            ServiceCompat.stopForeground(this, ServiceCompat.STOP_FOREGROUND_REMOVE)
            stopSelf()
            return START_NOT_STICKY
        }
        if (worker == null) startFilter()
        return START_STICKY
    }

    private fun goForeground() {
        val type = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) ServiceInfo.FOREGROUND_SERVICE_TYPE_SYSTEM_EXEMPTED else 0
        try {
            ServiceCompat.startForeground(this, Notices.ID_PROTECTION, Notices.protectionOn(this), type)
        } catch (e: RuntimeException) {
            // Verify on the emulator: systemExempted needs VPN consent; without it, the notice still shows.
            Log.w(TAG, "startForeground refused", e)
            Notices.notify(this, Notices.ID_PROTECTION, Notices.protectionOn(this))
        }
    }

    private fun startFilter() {
        val app = application as GuardApp
        val data = app.data
        val decider = DnsDecider(data.lists, data.social, data.helplines)
        val upstreamDoh = DohUpstream().also { doh = it }
        val config = VpnBuilder.Config(getString(R.string.app_name), Notices.openApp(this), data.dohIps)
        val w = VpnWorker(this, this, { loop ->
            val udp = UdpUpstream(loop) { mapper.networkDnsServerAddresses }
            GuardResolver(
                decider = decider,
                settings = { app.filterSettings.value },
                upstream = { s -> if (s.familyDns) upstreamDoh else udp },
                onBlocked = app::onBlocked,
            )
        }, mapper, config)
        worker = w
        w.start()
        watchNetwork()
        Notices.clearFilterOff(this)
    }

    private fun stopFilter() {
        networkCallback?.let { runCatching { getSystemService(ConnectivityManager::class.java).unregisterNetworkCallback(it) } }
        networkCallback = null
        worker?.stop()
        worker = null
        doh?.shutdown()
        doh = null
        status.value = Status()
    }

    /** The network's own DNS servers change with the network (upstream for "family DNS filter off"). */
    private fun watchNetwork() {
        val cb = object : ConnectivityManager.NetworkCallback() {
            override fun onAvailable(network: Network) = mapper.refreshNetworkDnsServers(this@GuardVpnService)
            override fun onLinkPropertiesChanged(network: Network, lp: LinkProperties) = mapper.refreshNetworkDnsServers(this@GuardVpnService)
        }
        getSystemService(ConnectivityManager::class.java).registerDefaultNetworkCallback(cb)
        networkCallback = cb
    }

    override fun onVpnStatus(status: VpnWorker.Status) {
        Log.i(TAG, "VPN $status")
        val running = status == VpnWorker.Status.RUNNING
        GuardVpnService.status.value = Status(running, running && isAlwaysOn, running && isLockdownEnabled)
        if (status == VpnWorker.Status.STOPPED) main.post { if (worker != null && prepare(this) != null) onRevoke() }
    }

    override fun onRestartNeeded() {
        main.post {
            stopFilter()
            startFilter()
        }
    }

    /** Another VPN started, or "Disconnect" in the system dialog. */
    override fun onRevoke() {
        stopFilter()
        Notices.filterOff(this)
        ServiceCompat.stopForeground(this, ServiceCompat.STOP_FOREGROUND_REMOVE)
        super.onRevoke()
    }

    override fun onDestroy() {
        stopFilter()
        super.onDestroy()
    }

    companion object {
        private const val TAG = "GuardVpn"
        const val ACTION_START = "com.raisesteady.guard.vpn.START"
        const val ACTION_STOP = "com.raisesteady.guard.vpn.STOP"

        private val status = MutableStateFlow(Status())
        val state: StateFlow<Status> get() = status

        /** Starts the filter (the VPN permission must already be given: VpnService.prepare returned null). */
        fun start(context: Context) {
            ContextCompat.startForegroundService(context, Intent(context, GuardVpnService::class.java).setAction(ACTION_START))
        }

        fun stop(context: Context) {
            context.startService(Intent(context, GuardVpnService::class.java).setAction(ACTION_STOP))
        }
    }
}
