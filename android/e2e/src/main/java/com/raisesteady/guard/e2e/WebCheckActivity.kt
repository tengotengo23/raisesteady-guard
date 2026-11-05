package com.raisesteady.guard.e2e

import android.app.Activity
import android.os.Bundle
import android.webkit.WebResourceError
import android.webkit.WebResourceRequest
import android.webkit.WebView
import android.webkit.WebViewClient
import java.util.concurrent.atomic.AtomicReference

/**
 * Loads one address in a WebView and records what happened ("loaded", or "error <code>"), for
 * the filter test on images without Chrome (API 29 google_apis). Runs in the test app's UID,
 * so its lookups go through Guard's VPN.
 */
class WebCheckActivity : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val web = WebView(this)
        setContentView(web)
        web.webViewClient = object : WebViewClient() {
            override fun onReceivedError(view: WebView, request: WebResourceRequest, error: WebResourceError) {
                if (request.isForMainFrame) result.set("error ${error.errorCode}")
            }

            override fun onPageFinished(view: WebView, url: String) {
                result.compareAndSet(null, "loaded")
            }
        }
        web.loadUrl(intent.getStringExtra(EXTRA_URL) ?: "about:blank")
    }

    companion object {
        const val EXTRA_URL = "url"

        /** Set once per load; the test clears it before starting the activity. */
        val result = AtomicReference<String?>(null)
    }
}
