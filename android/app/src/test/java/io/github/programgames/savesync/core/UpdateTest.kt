package io.github.programgames.savesync.core

import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.IOException

// The update of the app from the GitHub releases (Update.kt), as the PC does it (client/selfupdate_test.go).
class UpdateTest {
    @get:Rule
    val tmp = TemporaryFolder()

    @Test
    fun newerVersions() {
        assertTrue(newer("3.11.4", "v3.11.5"))
        assertTrue(newer("3.11.4", "v3.12.0"))
        assertTrue(newer("3.12.0-rc.1", "v3.12.0"))
        assertFalse(newer("3.11.4", "v3.11.4"))
        assertFalse(newer("3.11.4", "v3.10.9"))
        assertFalse(newer("3.11.10", "v3.11.9")) // numbers, not text
        assertFalse(newer("dev", "v9.0.0")) // a build of a developer never updates itself
        assertFalse(newer("3.11.4", "nightly"))
    }

    private val json = """{"tag_name":"v3.12.0","html_url":"https://github.com/o/r/releases/tag/v3.12.0","assets":[
        {"name":"3dssavesync.cia","browser_download_url":"https://x/cia","size":7},
        {"name":"3dssavesync-android.apk","browser_download_url":"https://x/apk","size":2103936},
        {"name":"SHA256SUMS","browser_download_url":"https://x/sums","size":1020}]}"""

    @Test
    fun readsTheRelease() {
        val r = parseRelease(json)
        assertEquals(Release("v3.12.0", "https://github.com/o/r/releases/tag/v3.12.0", "https://x/apk", "https://x/sums"), r)
        assertEquals("3.12.0", r.version)
        assertNull(parseRelease("""{"tag_name":"v3.12.0","assets":[]}""").apk) // no APK in this release: nothing to offer
    }

    @Test
    fun downloadChecksTheSum() {
        val apk = "new apk".toByteArray()
        val good = sha256(apk) + "  3dssavesync-android.apk\n" + "0".repeat(64) + "  3dssavesync.cia\n"
        val r = parseRelease(json)
        val dir = tmp.newFolder("update")
        tmp.newFile("update/3dssavesync-3.11.0.apk") // an older download is removed
        val f = downloadUpdate(r, dir) { url, _ -> if (url == "https://x/sums") good.toByteArray() else apk }
        assertArrayEquals(apk, f.readBytes())
        assertEquals(listOf(f.name), dir.list()!!.toList())

        // a damaged download (or a sum of another file): nothing is kept to install
        val bad = "0".repeat(64) + "  3dssavesync-android.apk\n"
        try {
            downloadUpdate(r, dir) { url, _ -> if (url == "https://x/sums") bad.toByteArray() else apk }
            fail("damaged APK accepted")
        } catch (e: IOException) {
            assertTrue(e.message!!, e.message!!.contains("SHA256SUMS"))
        }
        assertEquals(emptyList<String>(), dir.list()!!.toList())
    }
}
