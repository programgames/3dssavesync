package io.github.programgames.savesync.core

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

private fun putU16(d: ByteArray, i: Int, v: Int) {
    d[i] = v.toByte()
    d[i + 1] = (v shr 8).toByte()
}

private fun putU32(d: ByteArray, i: Int, v: Long) {
    for (k in 0 until 4) d[i + k] = (v shr (8 * k)).toByte()
}

private fun putUtf16(d: ByteArray, at: Int, s: String) = s.forEachIndexed { i, c -> putU16(d, at + 2 * i, c.code) }

private fun sum(d: ByteArray, from: Int, to: Int) = (from until to).sumOf { d[it].toInt() and 0xFF }

// mainSunMoon builds a synthetic Sun/Moon main: play time and trainer (client/pokemon_test.go).
fun mainSunMoon(h: Int, m: Int, name: String) = ByteArray(0x6BE00).also {
    putU16(it, 0x40C00, h)
    it[0x40C02] = m.toByte()
    putUtf16(it, 0x1238, name)
}

// One synthetic save per generation (valid checksums and signatures), the cases of client/pokemon_test.go.
class PokemonTest {
    @Test
    fun playInfo() {
        val g1 = ByteArray(0x8000) // J, unknown, é; 57 h 32
        byteArrayOf(0x89.toByte(), 0xE0.toByte(), 0xBC.toByte(), 0x50).copyInto(g1, 0x2598)
        g1[0x2CED] = 57
        g1[0x2CEF] = 32
        g1[0x3523] = sum(g1, 0x2598, 0x3523).inv().toByte()

        val g2 = ByteArray(0x8000) // Crystal: Jean; big-endian hours
        byteArrayOf(0x89.toByte(), 0xA4.toByte(), 0xA0.toByte(), 0xAD.toByte(), 0x50).copyInto(g2, 0x200B)
        g2[0x2052] = 1 // 295 = 0x0127
        g2[0x2053] = 0x27
        g2[0x2054] = 10
        putU16(g2, 0x2D0D, sum(g2, 0x2009, 0x2B83))

        val g3 = ByteArray(0x20000) // two valid halves, shifted sectors; half 1 has the highest counter
        for (k in 0 until 28) {
            val m = k / 14
            val id = (k % 14 + 3 + k / 14) % 14
            val sec = m * 0xE000 + (k % 14) * 0x1000
            putU16(g3, sec + 0xFF4, id)
            putU32(g3, sec + 0xFFC, (4 + 5 * m).toLong())
            if (id == 0) {
                byteArrayOf((0xBB + 9 * m).toByte(), 0x1B, 0xFF.toByte()).copyInto(g3, sec)
                putU16(g3, sec + 0x0E, 1 + 575 * m)
                g3[sec + 0x10] = (2 + 46 * m).toByte()
            }
        }

        val g4 = ByteArray(0x80000) // Platinum: partition 1 is more recent
        putU32(g4, 0x40000 + 0xCF2C - 0x0C, 0xCF2C)
        putU32(g4, 0x40000 + 0xCF2C - 0x08, 0x20060623)
        putU32(g4, 0xCF2C - 0x14, 1)
        putU32(g4, 0x40000 + 0xCF2C - 0x14, 2)
        listOf(0x134, 0x188, 0xFFFF).forEachIndexed { i, c -> putU16(g4, 0x40000 + 0x68 + 2 * i, c) }
        putU16(g4, 0x40000 + 0x68 + 0x22, 35)
        g4[0x40000 + 0x68 + 0x24] = 7

        val g5 = ByteArray(0x80000) // Black 2/White 2: CRC16-CCITT of block 0x25F00
        for (i in 0 until 0x94) g5[0x25F00 + i] = (i * 7).toByte()
        putU16(g5, 0x25FA2, crc16CCITT(g5, 0x25F00, 0x94))
        putUtf16(g5, 0x19404, "Jé")
        putU16(g5, 0x19404 + 4, 0xFFFF)
        putU16(g5, 0x19424, 15)
        g5[0x19426] = 41

        val g7 = ByteArray(0x6CC00) // Ultra: 13 characters at most
        putU16(g7, 0x41000, 999)
        g7[0x41002] = 30
        putUtf16(g7, 0x1438, "ABCDEFGHIJKLMNO")

        for ((name, d, expected) in listOf(
            Triple("Gen 1", g1, PlayInfo(57, 32, "J?é")),
            Triple("Gen 2 Crystal", g2, PlayInfo(295, 10, "Jean")),
            Triple("Gen 3", g3, PlayInfo(576, 48, "Jé")),
            Triple("Gen 4 Platinum", g4, PlayInfo(35, 7, "Jé")),
            Triple("Gen 5 Black 2/White 2", g5, PlayInfo(15, 41, "Jé")),
            Triple("Ultra", g7, PlayInfo(999, 30, "ABCDEFGHIJKLM")),
            Triple("Sun/Moon never started", ByteArray(0x6BE00), PlayInfo(0, 0, "")),
            Triple("mainSunMoon", mainSunMoon(152, 30, "Sacha"), PlayInfo(152, 30, "Sacha")),
        )) {
            assertEquals(name, expected, pokemonPlayInfo(d))
            assertTrue(name, pokemonSize(d.size.toLong())) // a server Version of this size is read for its play time
        }
        assertFalse(pokemonSize(0x10000)) // 64 KB: no Pokémon game, the details download nothing
        for (size in listOf(0x8000, 0x20000, 0x80000, 1024)) { // blank or unknown size: refused
            assertNull("$size blank bytes", pokemonPlayInfo(ByteArray(size)))
        }
    }
}
