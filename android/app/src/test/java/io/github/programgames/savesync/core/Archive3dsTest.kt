package io.github.programgames.savesync.core

import org.json.JSONArray
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File
import java.io.FileNotFoundException
import java.io.IOException

// MemoryFolder is a fake Azahar save folder. files null: no folder (game never started). cut: number of next writes
// that keep only half of the content, then fail their read-back like SafFolder.
class MemoryFolder(var files: MutableMap<String, ByteArray>? = mutableMapOf(), var cut: Int = 0, var date: Long? = 1_700_000_000_000L) : ArchiveFolder {
    val writes = mutableListOf<String>()

    override val location = "test/data/00000001"

    override fun files() = files?.map { ArchiveFile(it.key, it.value.copyOf()) }

    override fun write(path: String, content: ByteArray) {
        val f = files ?: throw FileNotFoundException("$location: no folder")
        writes += path
        f[path] = if (cut-- > 0) content.copyOf(content.size / 2) else content.copyOf()
        if (!f.getValue(path).contentEquals(content)) throw IOException("$location/$path: the written file doesn't match")
    }

    override fun modified() = date

    override fun modifiedNow() = date
}

fun memoryFolder(files: List<ArchiveFile>?) = MemoryFolder(files?.associate { it.path to it.data }?.toMutableMap())

// The 3dssavesync archive, byte for byte the one of client/archive3ds.go: the shared cases of tests/archive3ds/, read in
// place, and a reader that refuses anything else.
class Archive3dsTest {
    private fun bytes(hex: String) = ByteArray(hex.length / 2) { hex.substring(2 * it, 2 * it + 2).toInt(16).toByte() }

    private fun problemOf(block: () -> Unit): Problem = problem(runCatching(block).exceptionOrNull() ?: error("no error"))

    @Test
    fun sharedCases() {
        val cases = JSONArray(File("../../tests/archive3ds/cases.json").readText(Charsets.UTF_8))
        assertTrue(cases.length() >= 8)
        for (i in 0 until cases.length()) {
            val c = cases.getJSONObject(i)
            val name = c.getString("name")
            val list = c.getJSONArray("files")
            val files = (0 until list.length()).map { list.getJSONObject(it).let { f -> ArchiveFile(f.getString("path"), bytes(f.getString("hex"))) } }
            if (c.optBoolean("error")) {
                assertEquals(name, Problem.ARCHIVE_PATH, problemOf { packArchive(files) })
                continue
            }
            val tar = packArchive(files)
            assertEquals(name, c.getString("sha256"), sha256(tar))
            val back = unpackArchive(tar)
            assertEquals(name, files.map { it.path }.sorted(), back.map { it.path })
            for (f in back) assertArrayEquals(name, files.single { it.path == f.path }.data, f.data)
            assertArrayEquals(name, tar, packArchive(back.reversed())) // the order of the files never matters
        }
    }

    private val good = packArchive(listOf(ArchiveFile("a", byteArrayOf(1, 2, 3)), ArchiveFile("b/c", ByteArray(600) { it.toByte() })))

    // edit changes bytes of the header at `header`, then computes its checksum again: only the edit is wrong.
    private fun edit(at: Int, text: String, header: Int = 0): ByteArray {
        val d = good.copyOf()
        text.toByteArray(Charsets.ISO_8859_1).copyInto(d, header + at)
        var sum = 0
        for (i in 0 until 512) sum += if (i in 148 until 156) ' '.code else d[header + i].toInt() and 0xFF
        (sum.toString(8).padStart(6, '0') + "\u0000 ").toByteArray(Charsets.ISO_8859_1).copyInto(d, header + 148)
        return d
    }

    @Test
    fun refusesAnythingElse() {
        assertArrayEquals(good, edit(0, "a")) // the same edit as the writer: still the same archive
        assertEquals(2, unpackArchive(good).size)
        val second = 1024 // header of "b/c": after "a" and its padded data
        val bad = mapOf(
            "empty" to ByteArray(0),
            "one end block" to good.copyOf(good.size - 512),
            "data past the end" to good + ByteArray(512),
            "cut in the data" to good.copyOf(second + 512 + 100),
            "checksum" to good.copyOf().also { it[0] = 'z'.code.toByte() },
            "folder entry" to edit(156, "5"),
            "link" to edit(156, "2"),
            "GNU magic" to edit(257, "ustar "),
            "prefix" to edit(345, "x"),
            "parent folder" to edit(0, "../a"),
            "absolute path" to edit(0, "/a"),
            "size past the end" to edit(124, "77777777777"),
            "size not octal" to edit(124, "0000000009"),
            "not sorted" to edit(0, "c"),
            "duplicate" to edit(0, "b/c"),
            "second header" to edit(156, "5", second),
        )
        for ((why, d) in bad) assertEquals(why, Problem.DAMAGED, problemOf { unpackArchive(d) })
    }
}

// A 3DSA game (ADR 0028): its Save file is the archive of its Azahar folder, so replace writes the whole folder after a
// .tar Backup copy, and refuses a damaged version, a folder never created and files it would have to delete.
class ArchiveSaveTest {
    @get:Rule
    val tmp = TemporaryFolder()

    private val v1 = listOf(ArchiveFile("main", random(3000, 1)), ArchiveFile("sub/extra.bin", random(10, 2)))
    private val v2 = listOf(ArchiveFile("main", random(3000, 3)), v1[1], ArchiveFile("sub/new/b.bin", random(700, 4)))

    private fun game(f: MemoryFolder) = Game(System.N3DS, "3DSA-00040000000EC300", "3DS 00040000000EC300", ArchiveSave(f), Format(""), "azahar")

    private val pending get() = Pending(File(tmp.root, "pending.json"))

    private fun copies() = File(tmp.root, "azahar").listFiles().orEmpty().toList()

    private fun problemOf(block: () -> Unit): SyncException = runCatching(block).exceptionOrNull() as? SyncException ?: error("no SyncException")

    // Download: a verified .tar of the folder first, then each changed file, read back; the marker goes.
    @Test
    fun writesEveryFileAfterABackupCopy() {
        val f = memoryFolder(v1)
        val g = game(f)
        val old = g.file.read()!!
        assertArrayEquals(packArchive(v1), old)
        val copy = replace(g, old, packArchive(v2), Backups(tmp.root), pending)!!
        assertTrue(copy.name, Regex("""3DS 00040000000EC300-\d{8}-\d{6}\.tar""").matches(copy.name))
        assertArrayEquals(old, copy.readBytes())
        assertArrayEquals(packArchive(v2), g.file.read())
        assertEquals(listOf("main", "sub/new/b.bin"), f.writes) // the unchanged file is not written again
        assertNull(pending.get(g))
        assertEquals(listOf(copy), Backups(tmp.root).of(g)) // the History lists it
    }

    // A damaged version: nothing written, no Backup copy.
    @Test
    fun damagedRefused() {
        val f = memoryFolder(v1)
        val g = game(f)
        val damaged = packArchive(v2).let { it.copyOf(it.size - 1) }
        assertEquals(Problem.DAMAGED, problemOf { replace(g, g.file.read(), damaged, Backups(tmp.root), pending) }.problem)
        assertTrue(f.writes.isEmpty())
        assertTrue(copies().isEmpty())
        assertArrayEquals(packArchive(v1), g.file.read())
    }

    // A game never started in Azahar: no folder, and the app never creates it.
    @Test
    fun neverStartedNotCreated() {
        val f = MemoryFolder(null)
        val g = game(f)
        assertNull(g.file.read())
        assertEquals(Problem.NEVER_STARTED, problemOf { replace(g, null, packArchive(v1), Backups(tmp.root), pending) }.problem)
        assertNull(f.files)
        assertNull(pending.get(g))
    }

    // Files of the folder that the version doesn't have: never deleted, so nothing is written; the message names them.
    @Test
    fun extraFilesStay() {
        val f = memoryFolder(v2)
        val g = game(f)
        val e = problemOf { replace(g, g.file.read(), packArchive(v1), Backups(tmp.root), pending) }
        assertEquals(Problem.EXTRA_FILES, e.problem)
        assertTrue(e.message!!, e.message!!.contains("sub/new/b.bin"))
        assertTrue(f.writes.isEmpty())
        assertTrue(copies().isEmpty())
        assertArrayEquals(packArchive(v2), g.file.read())
    }

    // A folder without any file holds no save: nothing to upload; a Download fills it, without a Backup copy.
    @Test
    fun emptyFolder() {
        val f = MemoryFolder()
        val g = game(f)
        assertNull(g.file.read())
        assertNull(readLocal(g, null))
        assertNull(replace(g, null, packArchive(v1), Backups(tmp.root), pending))
        assertArrayEquals(packArchive(v1), g.file.read())
    }

    // A write that doesn't read back: the old files go back (WRITE); the Backup copy stays.
    @Test
    fun badWritePutsTheOldFilesBack() {
        val f = memoryFolder(v1).apply { cut = 1 }
        val g = game(f)
        val old = g.file.read()!!
        assertEquals(Problem.WRITE, problemOf { replace(g, old, packArchive(v2), Backups(tmp.root), pending) }.problem)
        assertArrayEquals(old, g.file.read())
        assertArrayEquals(old, copies().single().readBytes())
        assertNull(pending.get(g))
    }

    // A path the ustar name field can't hold: the game isn't synced, with the file named.
    @Test
    fun longPath() {
        val f = memoryFolder(listOf(ArchiveFile("d/" + "x".repeat(99), byteArrayOf(1))))
        val e = problemOf { game(f).file.read() }
        assertEquals(Problem.ARCHIVE_PATH, e.problem)
        assertFalse(isGlobal(e.problem))
    }
}
