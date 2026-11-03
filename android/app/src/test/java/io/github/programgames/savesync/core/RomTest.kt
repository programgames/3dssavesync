package io.github.programgames.savesync.core

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import java.io.ByteArrayOutputStream
import java.util.zip.ZipEntry
import java.util.zip.ZipOutputStream

class RomTest {
    private fun header(at: Int, bytes: ByteArray, size: Int = 0x200) = ByteArray(size).also { bytes.copyInto(it, at) }

    private fun gb(title: String, checksum: Int) = header(0x134, title.toByteArray(Charsets.ISO_8859_1)).also {
        it[0x14E] = (checksum shr 8).toByte()
        it[0x14F] = checksum.toByte()
    }

    private fun read(name: String, content: ByteArray) = readRom(name) { content.inputStream() }

    // Keys from fake headers, the same forms as client/rom.go and 3ds/source/key.c (ADR 0007).
    @Test
    fun keys() {
        assertEquals(Rom(System.DS, "IRBF", "Pokemon Noire 2"), read("Pokemon Noire 2.nds", header(0x0C, "IRBF".toByteArray())))
        assertEquals("GBA-BPEF", read("Emeraude.GBA", header(0xAC, "BPEF".toByteArray()))!!.key)
        assertEquals("GB-POKEMON_BLUE-56A4", read("Bleu.gb", gb("POKEMON BLUE", 0x56A4))!!.key)
        // lowercase uppercased, other bytes (>= 0x80 too) become _, the title ends at 0
        assertEquals("GB-PM_CRYSTAL_E-0A1B", read("c.gbc", gb("pméCRYSTAL-E", 0x0A1B))!!.key)
        assertEquals("GB-AB-0001", read("t.gb", gb("AB\u0000CD", 1))!!.key)
        assertEquals("GB-AB-0001", read("t.gb", gb("AB", 1))!!.key)
    }

    @Test
    fun outOfScope() {
        assertNull(read("readme.txt", ByteArray(0)))
        assertNull(read("homebrew.nds", header(0x0C, "####".toByteArray()))!!.key)
        assertNull(read("empty.nds", ByteArray(0))!!.key) // shorter than the header: zeros, invalid code
        assertNull(read("notitle.gb", gb("", 0x1234))!!.key)
    }

    // .zip: the first DS/GB/GBA ROM inside; name = the zip, internal = the inner file.
    @Test
    fun zip() {
        val o = ByteArrayOutputStream()
        ZipOutputStream(o).use { z ->
            z.putNextEntry(ZipEntry("readme.txt"))
            z.write("hello".toByteArray())
            z.putNextEntry(ZipEntry("dir/Pokemon Platine (F).nds"))
            z.write(header(0x0C, "CPUF".toByteArray(), 0x4000))
        }
        assertEquals(Rom(System.DS, "CPUF", "Platine", "Pokemon Platine (F)"), read("Platine.zip", o.toByteArray()))
        val empty = ByteArrayOutputStream().also { ZipOutputStream(it).use { z -> z.putNextEntry(ZipEntry("a.txt")) } }
        assertNull(read("other.zip", empty.toByteArray()))
    }
}
