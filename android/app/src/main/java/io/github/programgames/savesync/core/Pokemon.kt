package io.github.programgames.savesync.core

import java.nio.ByteBuffer
import java.nio.ByteOrder

// Play time and trainer of the Pokémon games, from Red/Blue to Ultra Moon, to compare two Versions on a Conflict.
// Port of client/pokemon.go (same offsets, same tables, from PKHeX). The game is recognized by its size, then by a
// checksum or a signature.

// PlayInfo: play time and trainer name ("" = game not started yet).
data class PlayInfo(val hours: Int, val minutes: Int, val trainer: String)

private val charsG12 = mapOf(0x7F to ' ', 0xBA to 'à', 0xBB to 'è', 0xBC to 'é', 0xBD to 'ù')
private val charsG1 = mapOf(0xBE to 'À', 0xC6 to 'È', 0xC7 to 'É')
private val charsG2 = mapOf(0xBF to 'ç', 0xC6 to 'ë', 0xC7 to 'ï', 0xC8 to 'â', 0xC9 to 'ô', 0xCA to 'û', 0xCB to 'ê', 0xCC to 'î')
private val charsG3 = mapOf(
    0x00 to ' ', 0x01 to 'À', 0x04 to 'Ç', 0x05 to 'È', 0x06 to 'É', 0x16 to 'à', 0x19 to 'ç', 0x1A to 'è', 0x1B to 'é',
    0x1C to 'ê', 0x1D to 'ë', 0x20 to 'î', 0x21 to 'ï', 0x24 to 'ô', 0x26 to 'ù', 0x28 to 'û',
)
private val charsG4 = mapOf(
    0x1DE to ' ', 0x15F to 'À', 0x166 to 'Ç', 0x167 to 'È', 0x168 to 'É', 0x17F to 'à', 0x181 to 'â', 0x186 to 'ç',
    0x187 to 'è', 0x188 to 'é', 0x189 to 'ê', 0x18A to 'ë', 0x18D to 'î', 0x18E to 'ï', 0x193 to 'ô', 0x198 to 'ù', 0x19A to 'û',
)
private val rangesG12 = listOf(Triple(0x80, 'A', 26), Triple(0xA0, 'a', 26), Triple(0xF6, '0', 10))
private val rangesG3 = listOf(Triple(0xBB, 'A', 26), Triple(0xD5, 'a', 26), Triple(0xA1, '0', 10))
private val rangesG4 = listOf(Triple(0x121, '0', 10), Triple(0x12B, 'A', 26), Triple(0x145, 'a', 26))

// char decodes one character: range of letters or digits, else tables, else '?'.
private fun char(c: Int, ranges: List<Triple<Int, Char, Int>>, vararg tables: Map<Int, Char>): Char {
    for ((first, rune, n) in ranges) if (c >= first && c < first + n) return rune + (c - first)
    for (t in tables) t[c]?.let { return it }
    return '?'
}

private fun u8(d: ByteArray, i: Int) = d[i].toInt() and 0xFF
private fun u16(d: ByteArray, i: Int) = u8(d, i) or (u8(d, i + 1) shl 8)
private fun u32(d: ByteArray, i: Int) = ByteBuffer.wrap(d, i, 4).order(ByteOrder.LITTLE_ENDIAN).int.toLong() and 0xFFFFFFFFL

// name8 decodes a name in bytes (Gen 1 to 3): n characters at most, up to the end code.
private fun name8(d: ByteArray, at: Int, n: Int, end: Int, conv: (Int) -> Char): String {
    val b = StringBuilder()
    for (i in 0 until n) {
        val c = u8(d, at + i)
        if (c == end) break
        b.append(conv(c))
    }
    return b.toString()
}

// name16 decodes a name in 16-bit words (Gen 4: own table; Gen 5 to 7: UTF-16), up to 0 or 0xFFFF.
private fun name16(d: ByteArray, at: Int, n: Int, conv: (Int) -> Char): String {
    val b = StringBuilder()
    for (i in 0 until n) {
        val c = u16(d, at + 2 * i)
        if (c == 0 || c == 0xFFFF) break
        b.append(conv(c))
    }
    return b.toString()
}

private fun sum(d: ByteArray, from: Int, to: Int): Int {
    var s = 0
    for (i in from until to) s += u8(d, i)
    return s
}

// gen12 reads Gen 1 and 2 (32 KiB), recognized by their checksum (Gen 1, then Gold/Silver and Crystal).
private fun gen12(d: ByteArray): PlayInfo? {
    if (sum(d, 0x2598, 0x3523).inv() and 0xFF == u8(d, 0x3523)) {
        return PlayInfo(u8(d, 0x2CED), u8(d, 0x2CEF), name8(d, 0x2598, 7, 0x50) { char(it, rangesG12, charsG12, charsG1) })
    }
    for ((end, checksum, hours) in listOf(Triple(0x2D68, 0x2D69, 0x2053), Triple(0x2B82, 0x2D0D, 0x2052))) {
        val s = sum(d, 0x2009, end + 1)
        if (s != 0 && s and 0xFFFF == u16(d, checksum)) { // zero sum: blank file
            val h = (u8(d, hours) shl 8) or u8(d, hours + 1) // big-endian
            return PlayInfo(h, u8(d, hours + 2), name8(d, 0x200B, 7, 0x50) { char(it, rangesG12, charsG12, charsG2) })
        }
    }
    return null
}

// gen3 reads Gen 3 (128 KiB): two halves of 14 sectors; the valid half with the highest counter; section 0.
private fun gen3(d: ByteArray): PlayInfo? {
    var s0 = -1
    var best = 0L
    for (half in 0 until 2) {
        var seen = 0
        var zero = -1
        for (k in 0 until 14) {
            val o = half * 0xE000 + k * 0x1000
            val id = u16(d, o + 0xFF4)
            if (id >= 14 || seen and (1 shl id) != 0) break
            seen = seen or (1 shl id)
            if (id == 0) zero = o
        }
        if (seen == 0x3FFF && (s0 < 0 || u32(d, zero + 0xFFC) > best)) {
            s0 = zero
            best = u32(d, zero + 0xFFC)
        }
    }
    if (s0 < 0) return null
    return PlayInfo(u16(d, s0 + 0x0E), u8(d, s0 + 0x10), name8(d, s0, 7, 0xFF) { char(it, rangesG3, charsG3) })
}

// gen4 reads Gen 4 (512 KiB): footer of the general block of partition 1 (size, signature); active block by counters.
private fun gen4(d: ByteArray): PlayInfo? {
    for ((g, trainer) in listOf(0xC100 to 0x64, 0xCF2C to 0x68, 0xF628 to 0x64)) { // DP, Pt, HGSS
        val p0 = g - 0x14
        val p1 = 0x40000 + g - 0x14
        val sig = u32(d, p1 + 0x0C)
        if (u32(d, p1 + 0x08) != g.toLong() || sig != 0x20060623L && sig != 0x20070903L) continue
        // ponytail: highest major then minor counter, like pokemon.go; PKHeX also handles 0xFFFFFFFE
        val a0 = u32(d, p0)
        val a1 = u32(d, p1)
        val b = if (a0 == 0xFFFFFFFFL || a1 != 0xFFFFFFFFL && (a1 > a0 || a1 == a0 && u32(d, p1 + 4) > u32(d, p0 + 4))) 0x40000 else 0
        val t = b + trainer
        return PlayInfo(u16(d, t + 0x22), u8(d, t + 0x24), name16(d, t, 8) { char(it, rangesG4, charsG4) })
    }
    return null
}

fun crc16CCITT(d: ByteArray, from: Int, n: Int): Int {
    var c = 0xFFFF
    for (i in from until from + n) {
        c = c xor (u8(d, i) shl 8)
        repeat(8) { c = if (c and 0x8000 != 0) (c shl 1) xor 0x1021 else c shl 1 }
        c = c and 0xFFFF
    }
    return c
}

// gen5 reads Gen 5 (512 KiB): valid checksum block (Black/White or Black 2/White 2); trainer data at 0x19400.
private fun gen5(d: ByteArray): PlayInfo? {
    if (crc16CCITT(d, 0x23F00, 0x8C) != u16(d, 0x23F9A) && crc16CCITT(d, 0x25F00, 0x94) != u16(d, 0x25FA2)) return null
    return PlayInfo(u16(d, 0x19424), u8(d, 0x19426), name16(d, 0x19404, 8) { it.toChar() })
}

// Gen 6 and 7 (main of the 3DS games): known by size; play time offset, trainer offset.
private val pokemonFormats = mapOf(
    0x65600 to (0x01800 to 0x14048), // X/Y
    0x76000 to (0x01800 to 0x14048), // Omega Ruby/Alpha Sapphire
    0x6BE00 to (0x40C00 to 0x01238), // Sun/Moon
    0x6CC00 to (0x41000 to 0x01438), // Ultra Sun/Ultra Moon
)

private fun gen67(d: ByteArray): PlayInfo? {
    val (time, trainer) = pokemonFormats[d.size] ?: return null
    return PlayInfo(u16(d, time), u8(d, time + 2), name16(d, trainer, 13) { it.toChar() })
}

// pokemonSize: a Save of this size may be a Pokémon game; only then is a server Version read for its play time.
fun pokemonSize(size: Long) = size == 0x8000L || size == 0x20000L || size == 0x80000L || size.toInt() in pokemonFormats

// pokemonPlayInfo returns the play time and trainer of a Save (SRAM only) of a recognized Pokémon game, else null.
fun pokemonPlayInfo(d: ByteArray?): PlayInfo? = when (d?.size) {
    null -> null
    0x8000 -> gen12(d)
    0x20000 -> gen3(d)
    0x80000 -> gen4(d) ?: gen5(d)
    else -> gen67(d)
}
