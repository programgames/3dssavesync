package io.github.programgames.savesync.core

import java.nio.ByteBuffer
import java.nio.ByteOrder

// Save formats: an emulator's Save file <-> the SRAM that is hashed, compared and synced, as client/saveformat.go:
// Clock trailer (ADR 0008, 0010), MBC2 (ADR 0012), DeSmuME footer (.dsv), gpSP and Beetle GBA .srm (ADR 0011).
// GBA EEPROM: the mGBA order, like the PC; no conversion (ADR 0012).

// Format is the shape of the Save files of one emulator for one System.
data class Format(
    val ext: String = ".sav",
    val srm: String? = null, // "gpSP" or "Beetle GBA": fixed-size .srm (ADR 0011)
    val mbc2: Int = 512, // size of a new MBC2 file: 256 in mGBA, 512 elsewhere
    val dsvFooter: Boolean = false, // a new DS file gets the DeSmuME footer; an existing file keeps its own form
)

class FormatException(message: String) : Exception(message)

// trailer returns the length of the Clock trailer after the SRAM of a GB or GBA file: 16, 44 or 48 bytes, 0 if none.
fun trailer(system: System, size: Int): Int {
    val r = size % 512
    return if ((system == System.GB || system == System.GBA) && size > r && (r == 16 || r == 44 || r == 48)) r else 0
}

// blank reports only 0x00 or only 0xFF (also true when empty).
fun blank(o: ByteArray): Boolean = o.all { it == o[0] && (it == 0.toByte() || it == 0xFF.toByte()) }

// MBC2 (Game Boy): server form = 512 bytes, one 4-bit value per byte, high nibble at 0. A GB file of 256 or 512
// bytes can only be MBC2.
fun isMBC2(system: System, n: Int) = system == System.GB && (n == 256 || n == 512)

fun normalizeMBC2(f: ByteArray) = ByteArray(512) {
    if (f.size == 256) ((f[it / 2].toInt() shr (it % 2 * 4)) and 0x0F).toByte() else (f[it].toInt() and 0x0F).toByte()
}

// mbc2File converts the server form to a file of n bytes: 256 (mGBA, two values per byte) or 512 (high nibble 0xF).
fun mbc2File(s: ByteArray, n: Int): ByteArray {
    val f = ByteArray(n)
    for (i in s.indices) {
        val v = s[i].toInt() and 0x0F
        if (n == 256) f[i / 2] = (f[i / 2].toInt() or (v shl (i % 2 * 4))).toByte() else f[i] = (v or 0xF0).toByte()
    }
    return f
}

// gpSP and Beetle GBA write a fixed-size .srm; the SRAM size comes from the server Version (ADR 0011).
val fixedSRM = mapOf("gpSP" to 0x20000, "Beetle GBA" to 0x22000)

// srmZone returns the start of the n-byte SRAM in the .srm: EEPROM at 0x20000 in Beetle GBA, else 0.
fun srmZone(core: String, n: Int) = if (core == "Beetle GBA" && n <= 0x2000) 0x20000 else 0

fun sramSRM(core: String, raw: ByteArray, srv: Version?): ByteArray {
    val size = fixedSRM.getValue(core)
    if (raw.size != size) throw FormatException("unexpected $core .srm size: ${raw.size} bytes instead of $size")
    if (srv == null) {
        if (blank(raw)) return raw
        throw FormatException("$core: sync this game from another emulator first, so the server knows its save size")
    }
    val n = srv.size.toInt()
    val d = srmZone(core, n)
    if (n <= 0 || n > 0x20000 || !blank(raw.copyOfRange(0, d)) || !blank(raw.copyOfRange(d + n, raw.size))) {
        throw FormatException("$core: the .srm has data beyond the $n bytes of the server version")
    }
    return raw.copyOfRange(d, d + n)
}

fun srm(core: String, sram: ByteArray): ByteArray {
    if (sram.isEmpty() || sram.size > 0x20000) throw FormatException("unknown GBA save size: ${sram.size} bytes")
    val o = ByteArray(fixedSRM.getValue(core)) { 0xFF.toByte() }
    sram.copyInto(o, srmZone(core, sram.size))
    return o
}

// DeSmuME .dsv: SRAM + 122-byte footer (client/dsv.go). 82-byte text, 6 u32 LE, 16-byte cookie.
private const val DSV_TEXT = "|<--Snip above here to create a raw sav by excluding this DeSmuME savedata footer:"
private const val DSV_COOKIE = "|-DESMUME SAVE-|"
private const val DSV_FOOTER = 122
private val dsvChips = listOf(
    512 to 1, (8 shl 10) to 2, (64 shl 10) to 2, (32 shl 10) to 2, (256 shl 10) to 3, (512 shl 10) to 3, (1 shl 20) to 3,
    (2 shl 20) to 3, (4 shl 20) to 3, (8 shl 20) to 3, (16 shl 20) to 3, (32 shl 20) to 3, (64 shl 20) to 3,
)

private fun le(o: ByteArray) = ByteBuffer.wrap(o).order(ByteOrder.LITTLE_ENDIAN)

private fun hasDsvFooter(raw: ByteArray) =
    raw.size >= DSV_FOOTER && String(raw, raw.size - DSV_COOKIE.length, DSV_COOKIE.length, Charsets.ISO_8859_1) == DSV_COOKIE

// sramDSV returns the SRAM of a file with the DeSmuME footer.
private fun sramDSV(raw: ByteArray): ByteArray {
    val n = raw.size - DSV_FOOTER
    if (le(raw).getInt(n + 86) != n || le(raw).getInt(n + 102) != 0) throw FormatException("DeSmuME footer unknown")
    return raw.copyOfRange(0, n)
}

private fun dsv(sram: ByteArray): ByteArray {
    val i = dsvChips.indexOfFirst { it.first == sram.size }
    if (i < 0) throw FormatException("DeSmuME doesn't know this save size: ${sram.size} bytes")
    val (size, address) = dsvChips[i]
    val f = le(ByteArray(24))
    for (v in listOf(size, size, i + 1, address, size, 0)) f.putInt(v)
    return sram + DSV_TEXT.toByteArray(Charsets.ISO_8859_1) + f.array() + DSV_COOKIE.toByteArray(Charsets.ISO_8859_1)
}

// decode returns the SRAM of a Save file. A 3DS main file is its own SRAM. srv: Current version or null; it gives the SRAM size of a fixed-size .srm.
fun decode(system: System, format: Format, raw: ByteArray, srv: Version?): ByteArray {
    if (format.srm != null) return sramSRM(format.srm, raw, srv)
    if (system == System.DS && hasDsvFooter(raw)) return sramDSV(raw)
    val sram = raw.copyOfRange(0, raw.size - trailer(system, raw.size))
    return if (isMBC2(system, sram.size)) normalizeMBC2(sram) else sram
}

// encode returns the Save file to write for a received SRAM. old: the current file, or null. Its Clock trailer stays
// (it belongs to this phone), its MBC2 size and DeSmuME footer too; without it, the shape of the emulator (Format).
fun encode(system: System, format: Format, sram: ByteArray, old: ByteArray?): ByteArray {
    if (format.srm != null) return srm(format.srm, sram)
    if (isMBC2(system, sram.size)) {
        return mbc2File(normalizeMBC2(sram), if (old != null && isMBC2(system, old.size)) old.size else format.mbc2)
    }
    if (system == System.DS) {
        val footer = if (old != null) hasDsvFooter(old) else format.dsvFooter
        if (!footer) return sram.copyOf()
        if (old != null && old.size - DSV_FOOTER == sram.size) return sram + old.copyOfRange(sram.size, old.size)
        return dsv(sram)
    }
    return if (old == null) sram.copyOf() else sram + old.copyOfRange(old.size - trailer(system, old.size), old.size)
}
