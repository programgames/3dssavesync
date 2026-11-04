package io.github.programgames.savesync

import org.junit.Assert.assertEquals
import org.junit.Test
import org.w3c.dom.Element
import java.io.File
import javax.xml.parsers.DocumentBuilderFactory

// Every English string has its French one, with the same placeholders (%1$d, %2$s…).
class StringsTest {
    private fun read(path: String): Map<String, List<String>> {
        val doc = DocumentBuilderFactory.newInstance().newDocumentBuilder().parse(File(path))
        val out = sortedMapOf<String, List<String>>()
        for (tag in listOf("string", "plurals")) {
            val nodes = doc.getElementsByTagName(tag)
            for (i in 0 until nodes.length) {
                val e = nodes.item(i) as Element
                if (e.getAttribute("translatable") == "false") continue
                val text = if (tag == "string") e.textContent else (e.getElementsByTagName("item").item(0) as Element).textContent
                out[e.getAttribute("name")] = Regex("%\\d\\$[0-9]*[a-z]").findAll(text).map { it.value }.sorted().toList()
            }
        }
        return out
    }

    // French typography: a no-break space (U+00A0, &#160; in the file) before : ; ? ! and inside « », so that a sign
    // never starts a line. And "vous", never the familiar "tu".
    @Test
    fun frenchTypography() {
        val doc = DocumentBuilderFactory.newInstance().newDocumentBuilder().parse(File("src/main/res/values-fr/strings.xml"))
        val bad = mutableListOf<String>()
        for (tag in listOf("string", "item")) {
            val nodes = doc.getElementsByTagName(tag)
            for (i in 0 until nodes.length) {
                val t = nodes.item(i).textContent
                if (Regex("[ \\t][:;?!»]|«[ \\t]|«[^\\u00A0]|[^\\u00A0]»").containsMatchIn(t)) bad += t
                if (Regex("\\b(tu|toi|ton|ta|tes|te)\\b", RegexOption.IGNORE_CASE).containsMatchIn(t)) bad += t
            }
        }
        assertEquals(emptyList<String>(), bad)
    }

    @Test
    fun frenchHasEveryString() {
        val en = read("src/main/res/values/strings.xml")
        val fr = read("src/main/res/values-fr/strings.xml")
        assertEquals(en.keys, fr.keys)
        assertEquals(en, fr)
    }
}
