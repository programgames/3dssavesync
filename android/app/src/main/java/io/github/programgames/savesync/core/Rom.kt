package io.github.programgames.savesync.core

import org.json.JSONObject
import java.io.File
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

// shortName is the name the lists show, as the 3DS app (main.c make_short_name): "Pokemon - Version Diamant (France)
// (Rev 5)" -> "Pokemon Diamant". The full ROM name stays in the details.
fun shortName(name: String): String =
    name.replaceFirst(" - Version ", " ").substringBefore(" (").trim().ifEmpty { name }

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

// RomKeys is what readRom found in each ROM file at the last pass, by document, with the size and date of the file
// (romkeys.json in app-private storage): a refresh or a background pass opens only the new or changed ROMs. A file the
// cache can't vouch for (no size or date from the provider, damaged cache) is read again; save keeps the ROMs of this
// pass only. ponytail: size + date, not the content; a ROM replaced by another of the same size and date keeps the old
// Key. Add a hash of the header if a provider is seen to keep the date on a new file.
class RomKeys(private val file: File) {
    private val old = runCatching { JSONObject(file.readText()) }.getOrNull() ?: JSONObject()
    private val now = JSONObject()

    // rom: readRom of a ROM file (id: its document; name: its file name), from the cache when the file did not change.
    fun rom(id: String, name: String, size: Long?, modified: Long?, read: () -> Rom?): Rom? {
        val stamp = if (size == null || modified == null || modified <= 0) null else "$size|$modified|"
        val cached = stamp?.let { old.optString(id).takeIf { v -> v.startsWith(it) } }?.removePrefix(stamp)?.split('|')
        val r = if (cached?.size == 2) {
            runCatching { cached[0].takeIf { it.isNotEmpty() }?.let { Rom(System.valueOf(it), cached[1].ifEmpty { null }, withoutExtension(name)) } }
        } else null
        val rom = if (r != null && r.isSuccess) r.getOrNull() else read()
        if (stamp != null) now.put(id, stamp + (rom?.system?.name ?: "") + "|" + (rom?.key ?: ""))
        return rom
    }

    // save writes the cache when it changed; a failure only costs a full read next time.
    fun save() {
        if (now.toString() != old.toString()) runCatching { writeAtomic(file, now.toString()) }
    }
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

// Aliases (ADR 0014): a ROM hack that keeps the code of its original game gets its own Key. As client/cases.go, the
// same ROM file name gives the same Key on the PC, the 3DS and this phone. aliasExamples: the Systems with Aliases
// (a 3DS save follows the title ID), with a Key for the help text.
val aliasExamples = mapOf(System.DS to "0RAD", System.GBA to "GBA-0RAD", System.GB to "GB-MONHACK-0001")

// aliasKey returns the Key of the Alias of this ROM (file name without extension), else the header Key. null: the
// Alias is not valid, the ROM is not synced (never under the header Key, the one of the original game).
fun aliasKey(aliases: Map<String, String>, system: System, name: String, key: String): String? {
    val a = aliases[name] ?: return key
    if (system !in aliasExamples) return key
    return a.trim().uppercase().takeIf { system.shape.matches(it) }
}

// newAlias returns the Key of the Alias of this ROM name: the one it has, else a new one derived from the name (a Key
// already taken by another Alias gives the next derivation). IllegalArgumentException: no Alias possible.
fun newAlias(aliases: Map<String, String>, system: System, name: String): String {
    require(system in aliasExamples) { "${system.label} games don't use aliases." }
    require(name.isNotBlank() && name.none { it in "=\r\n" }) { "This ROM name can't be used as an alias: \"$name\"" }
    aliases[name]?.uppercase()?.takeIf { system.shape.matches(it) }?.let { return it }
    val taken = aliases.filterKeys { it != name }.values.map { it.uppercase() }.toSet()
    val signs = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
    for (i in 0..Int.MAX_VALUE) {
        val d = java.security.MessageDigest.getInstance("SHA-256").digest("$name\u0000$i".toByteArray())
        val h = d.map { it.toInt() and 0xFF }
        val suffix = "${signs[h[0] % 36]}${signs[h[1] % 36]}${signs[h[2] % 36]}"
        val key = when (system) {
            System.DS -> "0$suffix"
            System.GBA -> "GBA-0$suffix"
            else -> "GB-ALIAS-" + hex(d.copyOfRange(0, 2)).uppercase()
        }
        if (key !in taken) return key
    }
    error("no free key")
}

// aliasSystem returns the System of an Alias Key (the console shown in the list).
fun aliasSystem(key: String): System = key.trim().uppercase().let { if (it.startsWith("GBA-")) System.GBA else if (it.startsWith("GB-")) System.GB else System.DS }
