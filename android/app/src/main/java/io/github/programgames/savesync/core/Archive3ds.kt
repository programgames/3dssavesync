package io.github.programgames.savesync.core

import java.io.ByteArrayOutputStream

// 3dssavesync archive (ADR 0028): the files of a 3DS save archive as one deterministic ustar, so the 3DS, the PC and
// Android give the same bytes, hence the same Hash, for the same files. Port of client/archive3ds.go, byte for byte;
// tests/archive3ds/cases.json holds the shared cases.

// ARCHIVE_KEY starts the Key of every 3DS game but the 8 Pokémon games: its server versions are archives (ADR 0028).
const val ARCHIVE_KEY = "3DSA-"

// ArchiveFile is one file of a save archive: its path from the archive root ("main", "dir/file") and its bytes.
class ArchiveFile(val path: String, val data: ByteArray)

private const val BLOCK = 512

// Go compares strings byte by byte (UTF-8); Kotlin compares UTF-16 units, which order some characters otherwise.
private fun utf8(s: String) = s.toByteArray(Charsets.UTF_8)

private fun compareBytes(a: ByteArray, b: ByteArray): Int {
    for (i in 0 until minOf(a.size, b.size)) {
        val d = (a[i].toInt() and 0xFF) - (b[i].toInt() and 0xFF)
        if (d != 0) return d
    }
    return a.size - b.size
}

// validArchivePath: relative, '/' separators, no empty, "." or ".." part, at most 100 bytes (the ustar name field).
fun validArchivePath(p: String): Boolean {
    if (p.isEmpty() || utf8(p).size > 100 || p.startsWith("/") || '\\' in p || '\u0000' in p) return false
    return p.split('/').none { it.isEmpty() || it == "." || it == ".." }
}

// octal writes v as len-1 octal digits then NUL at h[at until at+len].
private fun octal(h: ByteArray, at: Int, len: Int, v: Long) {
    v.toString(8).padStart(len - 1, '0').toByteArray(Charsets.US_ASCII).copyInto(h, at)
    h[at + len - 1] = 0
}

private fun sum(h: ByteArray): Long {
    var s = 0L
    for (i in h.indices) s += if (i in 148 until 156) ' '.code else h[i].toInt() and 0xFF // the checksum field as spaces
    return s
}

// packArchive returns the archive of files, sorted by path. An invalid path or a duplicate is an error: the game is
// then not synced, with the reason (ARCHIVE_PATH).
fun packArchive(files: List<ArchiveFile>): ByteArray {
    val sorted = files.map { utf8(it.path) to it }.sortedWith { a, b -> compareBytes(a.first, b.first) }
    val out = ByteArrayOutputStream()
    for ((i, entry) in sorted.withIndex()) {
        val (name, f) = entry
        if (!validArchivePath(f.path)) throw SyncException(Problem.ARCHIVE_PATH, "3DS save file \"${f.path}\": path not supported (100 bytes at most)")
        if (i > 0 && sorted[i - 1].first.contentEquals(name)) throw SyncException(Problem.ARCHIVE_PATH, "3DS save file \"${f.path}\": twice")
        val h = ByteArray(BLOCK)
        name.copyInto(h, 0)
        octal(h, 100, 8, 420) // mode 0644
        octal(h, 108, 8, 0) // uid
        octal(h, 116, 8, 0) // gid
        octal(h, 124, 12, f.data.size.toLong())
        octal(h, 136, 12, 0) // mtime
        h[156] = '0'.code.toByte()
        "ustar\u000000".toByteArray(Charsets.US_ASCII).copyInto(h, 257) // magic and version
        octal(h, 329, 8, 0) // devmajor
        octal(h, 337, 8, 0) // devminor
        octal(h, 148, 7, sum(h)) // 6 digits and NUL
        h[155] = ' '.code.toByte()
        out.write(h)
        out.write(f.data)
        val pad = f.data.size % BLOCK
        if (pad != 0) out.write(ByteArray(BLOCK - pad))
    }
    out.write(ByteArray(2 * BLOCK))
    return out.toByteArray()
}

private fun zeros(d: ByteArray, from: Int, n: Int) = (from until from + n).all { d[it].toInt() == 0 }

// field: the bytes of a header field without its trailing NULs, one char per byte.
private fun field(d: ByteArray, from: Int, n: Int): String {
    var end = from + n
    while (end > from && d[end - 1].toInt() == 0) end--
    return String(d, from, end - from, Charsets.ISO_8859_1)
}

// unpackArchive reads an archive written by packArchive. Anything else (other entry types, unsorted or duplicate
// paths, a bad checksum, data past the end) is refused (DAMAGED): a damaged version is never written to a save.
fun unpackArchive(data: ByteArray): List<ArchiveFile> {
    fun damaged(why: String): Nothing = throw SyncException(Problem.DAMAGED, "3dssavesync archive: damaged ($why)")
    val files = mutableListOf<ArchiveFile>()
    var last: ByteArray? = null
    var off = 0
    while (true) {
        if (off.toLong() + BLOCK > data.size) damaged("cut at $off")
        if (zeros(data, off, BLOCK)) {
            if (data.size.toLong() != off.toLong() + 2 * BLOCK || !zeros(data, off + BLOCK, BLOCK)) damaged("end at $off")
            return files
        }
        val h = data.copyOfRange(off, off + BLOCK)
        val raw = field(h, 0, 100).toByteArray(Charsets.ISO_8859_1)
        val name = String(raw, Charsets.UTF_8)
        // a name that isn't UTF-8 can't be a file of this phone: refused (the Go reader would keep its bytes)
        if (!utf8(name).contentEquals(raw)) damaged("name not UTF-8 at $off")
        val size = field(h, 124, 11).toLongOrNull(8)
        if (size == null || size < 0 || h[156] != '0'.code.toByte() || field(h, 257, 6) != "ustar" || h[262].toInt() != 0 || h[345].toInt() != 0 || !validArchivePath(name)) {
            damaged("header at $off")
        }
        if (field(h, 148, 7).toLongOrNull(8) != sum(h)) damaged("checksum at $off")
        if (last != null && compareBytes(last, raw) >= 0) damaged("\"$name\" not in order")
        val start = off + BLOCK
        if (start + size > data.size) damaged("\"$name\" cut")
        val end = start + size.toInt()
        files += ArchiveFile(name, data.copyOfRange(start, end))
        last = raw
        off = end + (BLOCK - size.toInt() % BLOCK) % BLOCK
    }
}

// ArchiveFolder is the folder of a 3DS save archive in Azahar, title/00040000/<low>/data/00000001 (ADR 0028): SAF on
// the phone, memory in the tests. Paths go from the folder, with '/'.
interface ArchiveFolder {
    val location: String // where it is, for the messages

    fun files(): List<ArchiveFile>? // every file below the folder, read now; null: no folder (game never started)

    fun write(path: String, content: ByteArray) // creates the file (and its folders) or replaces its content, read back

    fun modified(): Long? // newest change of its files, as last listed

    fun modifiedNow(): Long? // the same, listed now
}

// ArchiveSave is the Save file of a 3DSA game: its content is the archive of its folder, so the usual Sync works on the
// whole folder as on one file: the Backup copy is a .tar of the folder, the marker of an interrupted write covers the
// folder, and replace reads the whole folder back (ADR 0026, 0028).
class ArchiveSave(val folder: ArchiveFolder) : SaveFile {
    override val name = "00000001.tar" // the Backup copies: <game>-<date>.tar

    override val location get() = folder.location

    // a folder without any file holds no save yet: never an archive of nothing to upload
    override fun read(): ByteArray? = folder.files()?.takeIf { it.isNotEmpty() }?.let(::packArchive)

    // write writes the files of the version that differ from the folder, each read back. Nothing is ever deleted.
    override fun write(content: ByteArray) {
        val (files, now) = plan(content)
        for (f in files) if (now[f.path]?.contentEquals(f.data) != true) folder.write(f.path, f.data)
    }

    override fun modified() = folder.modified()

    override fun modifiedNow() = folder.modifiedNow()

    // check refuses a version before anything is written (replace calls it first): a damaged archive (DAMAGED), a folder
    // that isn't there (NEVER_STARTED: the app never creates the folder of a 3DSA game), or a folder that holds files
    // the version doesn't have (EXTRA_FILES: the app never deletes a file of a save, ADR 0026; the user moves them).
    fun check(content: ByteArray) {
        plan(content)
    }

    private fun plan(content: ByteArray): Pair<List<ArchiveFile>, Map<String, ByteArray>> {
        val files = unpackArchive(content)
        val now = folder.files() ?: throw SyncException(Problem.NEVER_STARTED, "$location: no folder, the game was never started in Azahar")
        val extra = now.map { it.path } - files.map { it.path }.toSet()
        if (extra.isNotEmpty()) throw SyncException(Problem.EXTRA_FILES, "$location: not in this version: ${extra.joinToString(", ")}")
        return files to now.associate { it.path to it.data }
    }
}
