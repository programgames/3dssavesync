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

    @Test
    fun frenchHasEveryString() {
        val en = read("src/main/res/values/strings.xml")
        val fr = read("src/main/res/values-fr/strings.xml")
        assertEquals(en.keys, fr.keys)
        assertEquals(en, fr)
    }
}
