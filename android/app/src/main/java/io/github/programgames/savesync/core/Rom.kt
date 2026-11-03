package io.github.programgames.savesync.core

import java.io.InputStream
import java.util.zip.ZipInputStream

// ROMs and Keys (ADR 0007), as client/rom.go. 3DS: no ROM, the title ID comes from the Azahar folder (titles3DS).

// System is the original console of a Game (GB covers GB and GBC). label: the name shown and used on the server.
enum class System(val label: String, val shape: Regex) {
    DS("DS", Regex("^[A-Z0-9]{4}$")),
    GB("GB", Regex("^GB-[A-Z0-9_]{1,15}-[0-9A-F]{4}$")),
    GBA("GBA", Regex("^GBA-[A-Z0-9]{4}$")),
    N3DS("3DS", Regex("^3DS-[0-9A-F]{16}$")),
}

// titles3DS lists the 8 synced 3DS Pokémon games (ADR 0009), as client/rom.go: one title ID per game, all regions.
val titles3DS = mapOf(
    "0004000000055D00" to "Pokemon X", "0004000000055E00" to "Pokemon Y",
    "000400000011C400" to "Pokemon Rubis Omega", "000400000011C500" to "Pokemon Saphir Alpha",
    "0004000000164800" to "Pokemon Soleil", "0004000000175E00" to "Pokemon Lune",
    "00040000001B5000" to "Pokemon Ultra-Soleil", "00040000001B5100" to "Pokemon Ultra-Lune",
)

// extensions maps a ROM extension to its System.
val extensions = mapOf(".nds" to System.DS, ".gb" to System.GB, ".gbc" to System.GB, ".gba" to System.GBA)

fun extension(file: String): String = file.substringAfterLast('.', "").let { if (it.isEmpty()) "" else ".$it" }.lowercase()

fun withoutExtension(file: String): String = if (file.contains('.')) file.substringBeforeLast('.') else file

// Rom is what a ROM tells. key null = out of scope (homebrew, invalid header).
// name: the ROM or the zip; internal: the file inside the zip ("" outside a zip).
data class Rom(val system: System, val key: String?, val name: String, val internal: String = "")

// readRom reads the header of a ROM file or of the first DS/GB/GBA ROM of a .zip. null: not a ROM.
fun readRom(fileName: String, open: () -> InputStream): Rom? {
    val ext = extension(fileName)
    val name = withoutExtension(fileName)
    if (ext == ".zip") {
        ZipInputStream(open()).use { z ->
            while (true) {
                val e = z.nextEntry ?: return null
                val inner = e.name.substringAfterLast('/')
                val system = extensions[extension(inner)] ?: continue
                return Rom(system, key(system, readHeader(z)), name, withoutExtension(inner))
            }
        }
    }
    val system = extensions[ext] ?: return null
    return Rom(system, open().use { key(system, readHeader(it)) }, name)
}

// readHeader reads the first 0x200 bytes; a ROM shorter than the header keeps zeros.
private fun readHeader(s: InputStream): ByteArray {
    val h = ByteArray(0x200)
    var n = 0
    while (n < h.size) {
        val r = s.read(h, n, h.size - n)
        if (r < 0) break
        n += r
    }
    return h
}

// key returns the Key read in a header: DS game code at 0x0C, GBA code at 0xAC, GB title and global checksum.
fun key(system: System, h: ByteArray): String? {
    val k = when (system) {
        System.DS -> String(h, 0x0C, 4, Charsets.ISO_8859_1)
        System.GBA -> "GBA-" + String(h, 0xAC, 4, Charsets.ISO_8859_1)
        System.N3DS -> return null
        System.GB -> {
            var title = h.copyOfRange(0x134, 0x143)
            val end = title.indexOf(0)
            if (end >= 0) title = title.copyOfRange(0, end)
            // byte by byte like key.c and rom.go: an uppercase of the String would change bytes >= 0x80
            val clean = title.map {
                var c = it.toInt() and 0xFF
                if (c in 'a'.code..'z'.code) c -= 'a'.code - 'A'.code
                if (c in 'A'.code..'Z'.code || c in '0'.code..'9'.code) c.toChar() else '_'
            }.joinToString("")
            "GB-$clean-" + hex(h.copyOfRange(0x14E, 0x150)).uppercase()
        }
    }
    return k.takeIf { system.shape.matches(it) }
}
