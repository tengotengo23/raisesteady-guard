package com.raisesteady.guard.dns

/** Where the filter asks for names it lets through. */
fun interface Upstream {
    /**
     * Send one DNS message; [callback] gets the answer, or null when the upstream cannot answer
     * (the filter then answers SERVFAIL). The callback may run on another thread.
     */
    fun query(message: ByteArray, callback: (ByteArray?) -> Unit)
}
