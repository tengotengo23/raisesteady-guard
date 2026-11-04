package com.raisesteady.guard.lists

import org.junit.Assert.assertEquals
import org.junit.Test

class SiteNameTest {
    @Test fun cleansTypedAddresses() {
        assertEquals("example.com", SiteName.clean("  https://www.Example.com/path?q=1#x "))
        assertEquals("example.com", SiteName.clean("http://example.com:8080"))
        assertEquals("bbc.co.uk", SiteName.clean("bbc.co.uk."))
        assertEquals("sub.example.org", SiteName.clean("sub.example.org/"))
    }

    @Test fun rejectsWhatIsNotAHostName() {
        assertEquals("", SiteName.clean("not a site"))
        assertEquals("", SiteName.clean("localhost"))
        assertEquals("", SiteName.clean(""))
        assertEquals("", SiteName.clean("-bad-.com"))
        assertEquals("", SiteName.clean("a".repeat(250) + ".com"))
    }

    @Test fun queryName() {
        assertEquals("www.example.com", SiteName.queryName("WWW.Example.COM."))
    }
}
