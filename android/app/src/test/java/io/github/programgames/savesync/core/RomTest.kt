package io.github.programgames.savesync.core

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertThrows
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.ByteArrayOutputStream
import java.io.File
import java.util.zip.ZipEntry
import java.util.zip.ZipOutputStream

class RomTest {
    @get:Rule
    val tmp = TemporaryFolder()

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

    // aliasKey: the cases of TestAliasKey in client/cases_test.go, same ROM names, same Keys.
    @Test
    fun aliasKeys() {
        val a = mapOf("Hack DS" to "0rad", "Hack GBA" to " GBA-0RAD ", "Hack GB" to "GB-MONHACK-0001", "Trop long" to "GBA-BPRE-RADICALRED", "Mauvais Système" to "0RAD")
        assertEquals("0RAD", aliasKey(a, System.DS, "Hack DS", "ADAF"))
        assertEquals("GBA-0RAD", aliasKey(a, System.GBA, "Hack GBA", "GBA-BPRE"))
        assertEquals("GB-MONHACK-0001", aliasKey(a, System.GB, "Hack GB", "GB-POKEMON_RED-91E6"))
        assertEquals("GBA-BPRE", aliasKey(a, System.GBA, "Sans alias", "GBA-BPRE"))
        assertNull(aliasKey(a, System.GBA, "Trop long", "GBA-BPRE"))
        assertNull(aliasKey(a, System.GBA, "Mauvais Système", "GBA-BPRE"))
        assertEquals("3DS-0004000000055D00", aliasKey(a, System.N3DS, "Hack DS", "3DS-0004000000055D00")) // never a 3DS Alias
    }

    // newAlias: the Key derived from the ROM name by SHA-256, byte for byte the one of addAlias in client/cases.go
    // (literal Keys computed by the Go code), so the hack syncs between the PC, the 3DS and this phone.
    @Test
    fun newAliasKeys() {
        assertEquals("GBA-0EE3", newAlias(emptyMap(), System.GBA, "Radical Red (4.10)"))
        assertEquals("0KP0", newAlias(emptyMap(), System.DS, "Hack"))
        assertEquals("GBA-0KP0", newAlias(emptyMap(), System.GBA, "Hack"))
        assertEquals("GB-ALIAS-8019", newAlias(emptyMap(), System.GB, "Hack"))
        assertEquals("0Y0G", newAlias(emptyMap(), System.DS, "Pokémon Blaze Black 2")) // UTF-8, as Go
        // the Alias it has is kept; a Key taken by another Alias gives the next derivation
        assertEquals("GBA-0EE3", newAlias(mapOf("Radical Red (4.10)" to "gba-0ee3"), System.GBA, "Radical Red (4.10)"))
        assertEquals("GBA-03DD", newAlias(mapOf("Autre hack" to "GBA-0EE3"), System.GBA, "Radical Red (4.10)"))
        for (name in listOf("", "  ", "a=b", "a\nb")) assertThrows(IllegalArgumentException::class.java) { newAlias(emptyMap(), System.GBA, name) }
        assertThrows(IllegalArgumentException::class.java) { newAlias(emptyMap(), System.N3DS, "Hack") }
        assertEquals(System.GBA, aliasSystem("GBA-0EE3"))
        assertEquals(System.GB, aliasSystem("GB-ALIAS-8019"))
        assertEquals(System.DS, aliasSystem("0KP0"))
    }

    // A ROM hack and its original game: one Key, a Duplicate (none synced). With an Alias for the hack: two games,
    // each with its Key. The same steps as the scan of the app (aliasKey, then splitDuplicates).
    @Test
    fun duplicateThenAlias() {
        val code = "BPRE".toByteArray()
        val roms = listOf("Pokemon FireRed.gba", "Radical Red (4.10).gba").map { read(it, header(0xAC, code))!! }
        fun scan(aliases: Map<String, String>) =
            splitDuplicates(roms.map { it.name to aliasKey(aliases, it.system, it.name, it.key!!)!! }, { it.second }, { it.first + ".srm" })
        val (none, dups) = scan(emptyMap())
        assertEquals(0, none.size)
        assertEquals(2, dups.size)
        val aliases = mapOf("Radical Red (4.10)" to newAlias(emptyMap(), System.GBA, "Radical Red (4.10)"))
        val (kept, left) = scan(aliases)
        assertEquals(listOf("Pokemon FireRed" to "GBA-BPRE", "Radical Red (4.10)" to "GBA-0EE3"), kept)
        assertEquals(0, left.size)
        assertNotEquals(kept[0].second, kept[1].second)
    }

    // The display name of the 3DS app (main.c make_short_name): no " - Version", no "(...)" tags.
    @Test
    fun shortNames() {
        for ((name, short) in listOf(
            "Pokemon - Version Diamant (France) (Rev 5)" to "Pokemon Diamant",
            "Mario Kart DS (Europe) (En,Fr,De,Es,It)" to "Mario Kart DS",
            "Mario & Luigi - Bowser's Inside Story (Europe)" to "Mario & Luigi - Bowser's Inside Story",
            "Radical Red (4.10)" to "Radical Red",
            "Tetris" to "Tetris",
            "(Proto) Game" to "(Proto) Game", // a leading tag stays, as on the 3DS
        )) assertEquals(name, short, shortName(name))
    }

    // The cache of the ROM Keys (romkeys.json): a ROM file is opened again only when its size or date changed, when the
    // provider gives no date, or when the cache is damaged (it is then made again).
    @Test
    fun romKeys() {
        val f = File(tmp.root, "romkeys.json")
        var opens = 0
        fun scan(size: Long?, date: Long?, code: String = "IRBF", name: String = "Noire 2.nds"): Rom? {
            val keys = RomKeys(f)
            return keys.rom("doc1", name, size, date) {
                opens++
                read(name, header(0x0C, code.toByteArray()))
            }.also { keys.save() }
        }
        assertEquals(Rom(System.DS, "IRBF", "Noire 2"), scan(10, 100))
        assertEquals(1, opens)
        assertEquals(Rom(System.DS, "IRBF", "Noire 2"), scan(10, 100))
        assertEquals(1, opens) // same size and date: not opened
        assertEquals("Blanche", scan(10, 100, name = "Blanche.nds")!!.name) // the name always comes from the file
        assertEquals(1, opens)
        assertEquals("IRAF", scan(10, 200, "IRAF")!!.key) // new date
        assertEquals("IRAF", scan(12, 200, "IRAF")!!.key) // new size
        assertEquals(3, opens)
        scan(12, null, "IRAF")
        scan(12, 0, "IRAF") // some providers: no date, or 0
        scan(12, 0, "IRAF")
        assertEquals(6, opens)
        assertNull(scan(1, 1, name = "notes.zip")) // not a ROM: remembered too
        assertNull(scan(1, 1, name = "notes.zip"))
        assertEquals(7, opens)
        for (bad in listOf("{broken", """{"doc1":"1|1|NOPE|X"}""", """{"doc1":"1|1"}""", "[]")) {
            f.writeText(bad)
            assertEquals("IRBF", scan(1, 1)!!.key)
        }
        assertEquals(11, opens)
        assertEquals("IRBF", scan(1, 1)!!.key)
        assertEquals(11, opens) // made again
        RomKeys(f).save() // a pass that saw no ROM keeps no old line
        assertEquals("{}", f.readText())
    }
}
