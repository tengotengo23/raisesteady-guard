package com.raisesteady.guard.e2e

import android.content.Intent
import android.net.Uri
import android.webkit.WebViewClient
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.uiautomator.By
import com.raisesteady.guard.e2e.E2e.GUARD
import com.raisesteady.guard.e2e.E2e.device
import com.raisesteady.guard.e2e.E2e.resolves
import com.raisesteady.guard.e2e.E2e.shell
import com.raisesteady.guard.e2e.E2e.shot
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Assume.assumeTrue
import org.junit.BeforeClass
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import java.net.InetAddress

/**
 * Design 6.2 test 2. Runs from the e2e app (another UID), so its lookups pass through Guard's VPN.
 * Guard must be installed with VPN consent pre-approved (tools/e2e.sh: appops ACTIVATE_VPN allow).
 * Until SetupTest exists (W6), the debug hook starts the filter for age band 9-12.
 */
@RunWith(AndroidJUnit4::class)
class FilterTest {
    @get:Rule val shots = E2e.Shots()

    companion object {
        @BeforeClass
        @JvmStatic
        fun startFilter() {
            shell("am start -n $GUARD/.debug.DebugFilterActivity --es band 9-12 --ez familyDns true")
            val deadline = System.currentTimeMillis() + 30_000
            while (resolves("pornhub.com")) {
                if (System.currentTimeMillis() > deadline) fail("the filter did not start within 30 s")
                Thread.sleep(500)
            }
        }

        private fun addresses(host: String) = InetAddress.getAllByName(host).map { it.hostAddress }.toSet()
    }

    @Test fun adultAndBypassDomainsDoNotResolve() {
        assertFalse(resolves("pornhub.com"))
        assertFalse(resolves("www.pornhub.com"))
        assertFalse(resolves("protonvpn.com"))
    }

    @Test fun socialIsBlockedUnder13() {
        assertFalse(resolves("tiktok.com"))
    }

    @Test fun safeSearchAndRestrictedMode() {
        assertEquals(addresses("forcesafesearch.google.com"), addresses("www.google.com"))
        assertEquals(addresses("restrict.youtube.com"), addresses("www.youtube.com"))
    }

    @Test fun ordinaryAndHelplineSitesResolve() {
        assertTrue(resolves("example.org"))
        assertTrue(resolves("findahelpline.com"))
    }

    /** On every image: a WebView in the test app opens example.org but cannot find an adult site. */
    @Test fun webViewCannotOpenAnAdultSite() {
        assertEquals("loaded", load("https://example.org/"))
        assertEquals("error ${WebViewClient.ERROR_HOST_LOOKUP}", load("https://pornhub.com/"))
    }

    private fun load(url: String): String? {
        WebCheckActivity.result.set(null)
        val ctx = E2e.instr.targetContext
        ctx.startActivity(
            Intent(ctx, WebCheckActivity::class.java).putExtra(WebCheckActivity.EXTRA_URL, url)
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TASK),
        )
        val deadline = System.currentTimeMillis() + 30_000
        while (WebCheckActivity.result.get() == null && System.currentTimeMillis() < deadline) Thread.sleep(200)
        return WebCheckActivity.result.get()
    }

    /** Chrome's own first-run questions, when the command-line flags were not read. */
    private fun skipChromeFirstRun() {
        repeat(6) {
            val b = listOf("Use without an account", "Accept & continue", "No thanks", "No, thanks", "Got it", "Continue")
                .firstNotNullOfOrNull { device.findObject(By.text(it)) } ?: return
            b.click()
            Thread.sleep(1500)
        }
    }

    @Test fun chromeShowsTheSiteCannotBeReached() {
        val chrome = "com.android.chrome"
        assumeTrue("Chrome is not on this image (API 29 google_apis): the WebView test covers it", shell("pm path $chrome").isNotBlank())
        val ctx = E2e.instr.context
        fun open(url: String) = ctx.startActivity(
            Intent(Intent.ACTION_VIEW, Uri.parse(url)).setPackage(chrome).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
        )
        open("https://pornhub.com")
        Thread.sleep(3000)
        skipChromeFirstRun()
        // Chrome writes "This site can't be reached" (typographic or straight apostrophe) and the error code.
        val shown = E2e.waitText("be reached", 20_000) || E2e.waitText("DNS_PROBE_FINISHED_NXDOMAIN", 5_000)
        shot("chrome-blocked")
        assertTrue("Chrome did not show the DNS error page", shown)
        // Google's results page has no SafeSearch label (run 41914882360); its SafeSearch page says
        // the setting is locked when the network forces it (forcesafesearch.google.com).
        open("https://www.google.com/safesearch")
        skipChromeFirstRun()
        val label = E2e.waitText("SafeSearch", 20_000)
        Thread.sleep(2000)
        shot("chrome-safesearch")
        assertTrue("no SafeSearch page in Chrome", label)
        assertTrue(
            "Chrome's SafeSearch page does not say it is locked",
            device.hasObject(By.textContains("locked")) || device.hasObject(By.textContains("network")),
        )
    }
}
