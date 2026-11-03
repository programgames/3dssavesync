package io.github.programgames.savesync.core

import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test
import java.util.Random

fun random(n: Int, seed: Long) = ByteArray(n).also { Random(seed).nextBytes(it) }

fun ff(n: Int) = ByteArray(n) { 0xFF.toByte() }

class SaveFormatTest {
    // encode(decode(x)) == x in every format, like client/saveformat_test.go.
    @Test
    fun roundTrip() {
        val sameboy = random(512, 5).map { (it.toInt() or 0xF0).toByte() }.toByteArray()
        val dsvOld = encode(System.DS, Format(dsvFooter = true), random(512 shl 10, 6), null)
        data class C(val name: String, val system: System, val format: Format, val raw: ByteArray, val srv: Version?, val sram: Int)
        for (c in listOf(
            C("DS .sav", System.DS, Format(), random(512 shl 10, 1), null, 512 shl 10),
            C("DS .dsv raw (DraStic)", System.DS, Format(".dsv"), random(512 shl 10, 1), null, 512 shl 10),
            C("DS .dsv DeSmuME footer", System.DS, Format(".dsv"), dsvOld, null, 512 shl 10),
            C("GB mGBA trailer", System.GB, Format(), random(32 shl 10, 2) + random(48, 99), null, 32 shl 10),
            C("GB VBA-M trailer", System.GB, Format(), random(32 shl 10, 2) + random(44, 99), null, 32 shl 10),
            C("GBA trailer", System.GBA, Format(), random(128 shl 10, 3) + random(16, 99), null, 128 shl 10),
            C("GBA EEPROM", System.GBA, Format(), random(8 shl 10, 4), null, 8 shl 10),
            C("MBC2 mGBA", System.GB, Format(), random(256, 5), null, 512),
            C("MBC2 SameBoy", System.GB, Format(), sameboy, null, 512),
            C("gpSP", System.GBA, Format(".srm", "gpSP"), srm("gpSP", random(64 shl 10, 7)), Version("", 64 shl 10), 64 shl 10),
            C("3DS main", System.N3DS, Format(""), random(0x80000 + 48, 9), null, 0x80000 + 48), // no trailer rule
            C("Beetle GBA EEPROM", System.GBA, Format(".srm", "Beetle GBA"), srm("Beetle GBA", random(8 shl 10, 8)), Version("", 8 shl 10), 8 shl 10),
        )) {
            val sram = decode(c.system, c.format, c.raw, c.srv)
            assertEquals(c.name, c.sram, sram.size)
            assertArrayEquals(c.name, c.raw, encode(c.system, c.format, sram, c.raw))
        }
    }

    // encode without a file: the emulator's own shape.
    @Test
    fun newFile() {
        val mbc2 = ByteArray(512).also { it[0] = 1; it[1] = 2 }
        assertArrayEquals("0123456789".toByteArray(), encode(System.GB, Format(), "0123456789".toByteArray(), null))
        assertArrayEquals(byteArrayOf(0x21) + ByteArray(255), encode(System.GB, Format(mbc2 = 256), mbc2, null))
        assertArrayEquals(byteArrayOf(0xF1.toByte(), 0xF2.toByte()) + ByteArray(510) { 0xF0.toByte() }, encode(System.GB, Format(), mbc2, null))
        val dsv = encode(System.DS, Format(".dsv", dsvFooter = true), ByteArray(512), null)
        assertEquals(512 + 122, dsv.size)
        assertArrayEquals(ByteArray(512), decode(System.DS, Format(".dsv"), dsv, null))
        assertArrayEquals(ByteArray(512), encode(System.DS, Format(".dsv"), ByteArray(512), null)) // DraStic: raw
    }

    // The existing file keeps its form: Clock trailer of this phone, MBC2 size, DeSmuME footer.
    @Test
    fun keepsOldShape() {
        val old = random(32 shl 10, 1) + random(48, 2)
        val got = encode(System.GB, Format(), random(32 shl 10, 3), old)
        assertArrayEquals(old.copyOfRange(32 shl 10, old.size), got.copyOfRange(32 shl 10, got.size))
        assertEquals(256, encode(System.GB, Format(mbc2 = 512), ByteArray(512), ByteArray(256)).size)
        val withFooter = encode(System.DS, Format(dsvFooter = true), ByteArray(8192), null)
        assertEquals(8192 + 122, encode(System.DS, Format(), random(8192, 4), withFooter).size)
    }

    @Test
    fun trailerRule() { // ADR 0010: remainder modulo 512
        assertEquals(48, trailer(System.GB, 32768 + 48))
        assertEquals(16, trailer(System.GBA, 512 + 16))
        assertEquals(0, trailer(System.GBA, 48)) // nothing before it: whole file
        assertEquals(0, trailer(System.GBA, 1536))
        assertEquals(0, trailer(System.DS, 512 + 16))
    }

    @Test
    fun mbc2() { // ADR 0012: mGBA 256 B and SameBoy 512 B give the same server form
        val mgba = ByteArray(256).also { it[0] = 0x21; it[255] = 0xA5.toByte() }
        val server = ByteArray(512).also { it[0] = 1; it[1] = 2; it[510] = 5; it[511] = 0xA }
        val sameboy = ByteArray(512) { (0xF0 or server[it].toInt()).toByte() }
        assertArrayEquals(server, normalizeMBC2(mgba))
        assertArrayEquals(server, normalizeMBC2(sameboy))
        assertArrayEquals(mgba, mbc2File(server, 256))
        assertArrayEquals(sameboy, mbc2File(server, 512))
        assertFalse(isMBC2(System.GB, 8192) || isMBC2(System.GBA, 512))
        assertTrue(isMBC2(System.GB, 256))
    }

    // gpSP and Beetle GBA (ADR 0011), the cases of client/srm_test.go.
    @Test
    fun srmCores() {
        for ((core, size, zone) in listOf(
            Triple("gpSP", 128 shl 10, 0), Triple("gpSP", 64 shl 10, 0), Triple("gpSP", 8 shl 10, 0), Triple("gpSP", 512, 0),
            Triple("Beetle GBA", 128 shl 10, 0), Triple("Beetle GBA", 32 shl 10, 0),
            Triple("Beetle GBA", 8 shl 10, 0x20000), Triple("Beetle GBA", 512, 0x20000),
        )) {
            val sram = random(size, size.toLong())
            val o = srm(core, sram)
            assertArrayEquals("$core $size", ff(zone) + sram + ff(fixedSRM.getValue(core) - zone - size), o)
            val srv = Version("", size.toLong())
            assertArrayEquals(sram, sramSRM(core, o, srv))
            val zero = ByteArray(o.size).also { sram.copyInto(it, zone) } // rest at 0x00: accepted
            assertArrayEquals(sram, sramSRM(core, zero, srv))
            for (i in listOf(zone + size, o.size - 1, 0)) { // data outside the server size: refused, nothing is cut
                if (i >= o.size || i in zone until zone + size) continue
                val broken = o.copyOf().also { it[i] = 0x12 }
                try {
                    sramSRM(core, broken, srv)
                    fail("$core $size: data at $i accepted")
                } catch (_: FormatException) {
                }
            }
            try {
                sramSRM(core, o, null)
                fail("$core $size: no server Version accepted")
            } catch (_: FormatException) {
            }
        }
        for (core in fixedSRM.keys) {
            assertTrue(blank(sramSRM(core, ff(fixedSRM.getValue(core)), null))) // blank .srm, no Version: passes
        }
    }

    // A 3DS main is never a Blank save (ADR 0009: Azahar's main of a game started once); the 8 Keys are valid.
    @Test
    fun threeDS() {
        val g = Game(System.N3DS, "3DS-0004000000175E00", "Pokemon Lune", MemoryFile("main", ByteArray(0x6BE00)), Format(""), "azahar")
        assertFalse(readLocal(g, null)!!.blank)
        assertTrue(titles3DS.keys.all { System.N3DS.shape.matches("3DS-$it") })
    }

    @Test
    fun blankSaves() {
        assertTrue(blank(ByteArray(8)) && blank(ff(8)) && blank(ByteArray(0)))
        assertFalse(blank(byteArrayOf(0, -1)) || blank(byteArrayOf(1, 1)))
    }
}
