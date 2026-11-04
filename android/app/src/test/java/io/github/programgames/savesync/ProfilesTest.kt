package io.github.programgames.savesync

import org.junit.Assert.assertEquals
import org.junit.Test
import org.w3c.dom.Element
import java.io.File
import javax.xml.parsers.DocumentBuilderFactory

// Android 11+ hides an app that is not in <queries>: every package of the Profiles must be there, and only them.
class ProfilesTest {
    @Test
    fun manifestQueriesEveryPackage() {
        val doc = DocumentBuilderFactory.newInstance().newDocumentBuilder().parse(File("src/main/AndroidManifest.xml"))
        val q = (doc.getElementsByTagName("queries").item(0) as Element).getElementsByTagName("package")
        val listed = (0 until q.length).map { (q.item(it) as Element).getAttribute("android:name") }.toSet()
        assertEquals(profiles.flatMap { it.packages }.toSet(), listed)
    }
}
