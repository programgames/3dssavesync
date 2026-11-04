package io.github.programgames.savesync.core

import org.junit.After
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File
import java.net.ServerSocket
import java.net.URL

// MemoryFile is a fake Save file. bad: number of next writes that keep only half of the content (provider failure).
// fail: number of next writes that throw before they touch the file (permission lost, folder gone). date: last change.
open class MemoryFile(
    override val name: String,
    var content: ByteArray? = null,
    var bad: Int = 0,
    var fail: Int = 0,
    var date: Long? = null,
) : SaveFile {
    override val location get() = "test/$name"

    override fun read() = content?.copyOf()

    override fun write(content: ByteArray) {
        if (fail-- > 0) throw java.io.IOException("$location: write refused")
        this.content = if (bad-- > 0) content.copyOf(content.size / 2) else content.copyOf()
    }

    override fun modified(): Long? = date
}

// The emulator may still be open: a save changed less than 30 s ago is never replaced, an unknown date only after the
// reminder to close the emulator.
class CheckClosedTest {
    private val now = 1_800_000_000_000L

    private fun refused(date: Long?, reminded: Boolean): Boolean {
        val e = runCatching { checkClosed(MemoryFile("a.sav", date = date), now, reminded) }.exceptionOrNull() ?: return false
        assertEquals(Problem.OPEN, (e as SyncException).problem)
        return true
    }

    @Test
    fun recent() {
        assertTrue(refused(now - 1_000, reminded = true))
        assertTrue(refused(now - 29_999, reminded = true))
        assertTrue(refused(now + 5_000, reminded = true)) // a few seconds ahead: clocks of the provider
    }

    @Test
    fun old() {
        assertFalse(refused(now - 30_000, reminded = false))
        assertFalse(refused(now - 3_600_000, reminded = false))
    }

    @Test
    fun unknown() {
        for (d in listOf(null, 0L, now + 3_600_000)) { // no date, 0 (some providers), far in the future
            assertTrue("$d", refused(d, reminded = false))
            assertFalse("$d", refused(d, reminded = true))
        }
    }
}

class ReplaceTest {
    @get:Rule
    val tmp = TemporaryFolder()

    private fun game(f: MemoryFile) = Game(System.DS, "IRBF", "Pokemon Noire 2", f, Format(), "drastic")

    // Backup copy (verified) first, then the new content, read back.
    @Test
    fun replaced() {
        val backups = Backups(tmp.root)
        val f = MemoryFile("Pokemon Noire 2.dsv", random(1024, 1))
        val copy = replace(game(f), f.content, random(1024, 2), backups)!!
        assertArrayEquals(random(1024, 2), f.content)
        assertArrayEquals(random(1024, 1), copy.readBytes())
        assertEquals("drastic", copy.parentFile!!.name)
        assertTrue(copy.name, Regex("""Pokemon Noire 2-\d{8}-\d{6}\.dsv""").matches(copy.name))
        val second = replace(game(f), f.content, random(1024, 3), backups)!! // same second: numbered
        assertTrue(second.name, second.name == copy.name.replace(".dsv", "-1.dsv") || second.name != copy.name)
        assertNull(replace(game(MemoryFile("new.sav")), null, random(8, 1), backups)) // no file before: no copy
    }

    // The written file does not match: the old content goes back, the error says so.
    @Test
    fun restored() {
        val f = MemoryFile("a.sav", random(1024, 1), bad = 1)
        val e = runCatching { replace(game(f), f.content, random(1024, 2), Backups(tmp.root)) }.exceptionOrNull() as SyncException
        assertEquals(Problem.WRITE, e.problem)
        assertArrayEquals(random(1024, 1), f.content)
        assertEquals(1, File(tmp.root, "drastic").listFiles()!!.size)
    }

    // The old content cannot be written back either: RESTORE, and the message names the Backup copy.
    @Test
    fun restoreFailed() {
        val f = MemoryFile("a.sav", random(1024, 1), bad = 2)
        val e = runCatching { replace(game(f), f.content, random(1024, 2), Backups(tmp.root)) }.exceptionOrNull() as SyncException
        assertEquals(Problem.RESTORE, e.problem)
        val copy = File(tmp.root, "drastic").listFiles()!!.single()
        assertTrue(e.message!!.contains(copy.path))
        assertArrayEquals(random(1024, 1), copy.readBytes())
    }

    // The write fails before it touches the file: the old save is still there, so WRITE, not RESTORE.
    @Test
    fun writeRefused() {
        val f = MemoryFile("a.sav", random(1024, 1), fail = 9)
        val e = runCatching { replace(game(f), f.content, random(1024, 2), Backups(tmp.root)) }.exceptionOrNull() as SyncException
        assertEquals(Problem.WRITE, e.problem)
        assertArrayEquals(random(1024, 1), f.content)
    }

    // The emulator wrote the save after it was read: the Backup copy doesn't hold it, so nothing is written.
    @Test
    fun changedMeanwhile() {
        val f = MemoryFile("a.sav", random(1024, 3))
        val e = runCatching { replace(game(f), random(1024, 1), random(1024, 2), Backups(tmp.root)) }.exceptionOrNull() as SyncException
        assertEquals(Problem.CHANGED, e.problem)
        assertArrayEquals(random(1024, 3), f.content)
        val n = MemoryFile("b.sav", random(8, 3)) // created after the read: no Backup copy of it
        val e2 = runCatching { replace(game(n), null, random(8, 2), Backups(tmp.root)) }.exceptionOrNull() as SyncException
        assertEquals(Problem.CHANGED, e2.problem)
        assertArrayEquals(random(8, 3), n.content)
    }

    // A first file that can't be written fully is emptied: blank counts as absent, never as a Local save to upload.
    @Test
    fun newFileFailed() {
        val f = MemoryFile("new.sav", bad = 1)
        val e = runCatching { replace(game(f), null, random(1024, 2), Backups(tmp.root)) }.exceptionOrNull() as SyncException
        assertEquals(Problem.NOT_WRITTEN, e.problem)
        assertEquals(0, f.content!!.size)
    }

    // The marker of a write (ADR 0026): there during the write, with the Backup copy and the hash of the new content,
    // gone once the file is verified (new content, or old one put back); it stays when the old one can't be put back.
    @Test
    fun pendingMarker() {
        val pending = Pending(File(tmp.root, "pending.json"))
        var during: PendingWrite? = null
        lateinit var g: Game
        val f = object : MemoryFile("a.sav", random(1024, 1)) {
            override fun write(content: ByteArray) {
                during = pending.get(g)
                super.write(content)
            }
        }
        g = game(f)
        val copy = replace(g, f.content, random(1024, 2), Backups(tmp.root), pending)!!
        assertEquals(PendingWrite(copy.path, sha256(random(1024, 2))), during)
        assertNull(pending.get(g))

        f.bad = 1 // written back: verified, no marker
        assertEquals(Problem.WRITE, (runCatching { replace(g, f.content, random(1024, 3), Backups(tmp.root), pending) }.exceptionOrNull() as SyncException).problem)
        assertNull(pending.get(g))

        f.bad = 2 // not written back: the marker stays, with its Backup copy
        assertEquals(Problem.RESTORE, (runCatching { replace(g, f.content, random(1024, 3), Backups(tmp.root), pending) }.exceptionOrNull() as SyncException).problem)
        val left = pending.get(g)!!
        assertArrayEquals(random(1024, 2), File(left.backup!!).readBytes())

        // a write over the cut file keeps the copy of the first one; it goes once a write is verified
        f.bad = 1
        runCatching { replace(g, f.content, random(1024, 4), Backups(tmp.root), pending) }
        assertEquals(left.backup, pending.get(g)!!.backup)
        replace(g, f.content, random(1024, 4), Backups(tmp.root), pending)
        assertNull(pending.get(g))
    }

    // Bases.clear forgets the Bases and the replaced Hashes of one emulator entry, nothing of the others.
    @Test
    fun basesClear() {
        val b = Bases(tmp.root)
        b.set("e", "K", "h1")
        b.setReplaced("e", "K", "h0")
        b.set("f", "K", "h2")
        b.setReplaced("f", "K", "h3")
        b.clear("e")
        assertTrue(b.of("e").isEmpty())
        assertTrue(b.replaced("e").isEmpty())
        assertEquals(mapOf("K" to "h2"), b.of("f"))
        assertEquals(mapOf("K" to "h3"), b.replaced("f"))
        b.setReplaced("f", "K", null)
        assertTrue(b.replaced("f").isEmpty())
        b.clear("none") // nothing to delete
    }

    @Test
    fun duplicates() {
        val (kept, dup) = splitDuplicates(
            listOf("A" to "x.sav", "B" to "y.sav", "B" to "z.sav", "C" to "X.SAV", "D" to "d.sav"), { it.first }, { it.second },
        )
        assertEquals(listOf("D" to "d.sav"), kept)
        assertEquals(4, dup.size)
    }
}

// MemoryArchive is a fake Azahar folder of a 3DS game never started: data/00000001, its .metadata, and main (the
// MemoryFile). undoFails: the provider refuses to delete.
class MemoryArchive(val main: MemoryFile, var metadata: ByteArray? = null, var folder: Boolean = false, var undoFails: Boolean = false) : NewArchive {
    private var created = false

    override fun exists() = folder

    override fun hasMetadata() = metadata != null

    override fun create(metadata: ByteArray) {
        this.metadata = metadata.copyOf()
        folder = true
        created = true
    }

    override fun undo(): Boolean {
        if (undoFails) return false
        if (created) { // only what create made
            main.content = null
            metadata = null
            folder = false
        }
        return true
    }
}

// A 3DS game never started in Azahar, like "jeu jamais lancé dans Azahar" in client/launcher_test.go (ADR 0009).
class CreateArchiveTest {
    @get:Rule
    val tmp = TemporaryFolder()

    private val pcMetadata = byteArrayOf(0, 0, 4, 0, 1, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0) // client/sync.go formats3DS

    private fun game(a: MemoryArchive, id: String = "0004000000055E00") = Game(System.N3DS, "3DS-$id", "Pokemon Y", a.main, Format(""), "azahar", a)

    private fun fails(a: MemoryArchive, id: String = "0004000000055E00") =
        runCatching { createArchive(game(a, id), a, random(415232, 2), Backups(tmp.root)) }.exceptionOrNull() as SyncException

    @Test
    fun created() {
        assertEquals(titles3DS.keys, formats3DS.keys)
        val a = MemoryArchive(MemoryFile("main"))
        createArchive(game(a), a, random(415232, 2), Backups(tmp.root))
        assertTrue(a.folder)
        assertArrayEquals(pcMetadata, a.metadata) // the 16 bytes of the PC
        assertArrayEquals(random(415232, 2), a.main.content)
        assertTrue(tmp.root.listFiles()!!.isEmpty()) // no old save: no Backup copy
    }

    // An existing .metadata without the folder, or an unknown format: nothing is created, as on the PC.
    @Test
    fun refused() {
        val a = MemoryArchive(MemoryFile("main"), metadata = random(16, 1))
        assertEquals(Problem.NOT_STARTED, fails(a).problem)
        assertArrayEquals(random(16, 1), a.metadata)
        assertFalse(a.folder)
        assertNull(a.main.content)

        val other = MemoryArchive(MemoryFile("main"))
        assertEquals(Problem.NOT_STARTED, fails(other, "00040000000EC300").problem)
        assertFalse(other.folder)
        assertNull(other.metadata)
    }

    // main can't be written: the folder and the .metadata go away again, so Azahar sees a game never started.
    @Test
    fun failedMidway() {
        val a = MemoryArchive(MemoryFile("main", bad = 1))
        assertEquals(Problem.NOT_WRITTEN, fails(a).problem)
        assertFalse(a.folder)
        assertNull(a.metadata)
        assertNull(a.main.content)
    }

    // They can't be removed either: PARTIAL names the folder; the empty main left counts as no save, never uploaded.
    @Test
    fun failedMidwayNotUndone() {
        val a = MemoryArchive(MemoryFile("main", bad = 1), undoFails = true)
        val e = fails(a)
        assertEquals(Problem.PARTIAL, e.problem)
        assertTrue(e.message!!, e.message!!.contains("test (and 00000001.metadata"))
        assertEquals(0, a.main.content!!.size)
        val l = readLocal(game(a), null)!!
        assertTrue(l.blank)
        assertNull(seen(l, null))
    }
}

// End to end against the real server/server.py on 127.0.0.1, like client/launcher_test.go startPythonServer.
class PythonServerTest {
    @get:Rule
    val tmp = TemporaryFolder()
    private lateinit var server: Process
    private lateinit var url: String

    @Before
    fun start() {
        val python = listOf("python3", "python").firstOrNull { runCatching { ProcessBuilder(it, "--version").start().waitFor() == 0 }.getOrDefault(false) }
        assumeTrue("Python not found", python != null)
        val port = ServerSocket(0).use { it.localPort }
        val data = File(tmp.root, "data").path.replace("\\", "\\\\")
        server = ProcessBuilder(
            python, "-c",
            "import socket, server; socket.getfqdn = lambda nom='': nom; server.creer(('127.0.0.1', $port), '$data', 'test-token').serve_forever()",
        ).directory(File("../../server")).redirectErrorStream(true).redirectOutput(File(tmp.root, "server.log")).start()
        url = "http://127.0.0.1:$port"
        val limit = java.lang.System.currentTimeMillis() + 60_000
        while (true) {
            if (runCatching { (URL("$url/saves").openConnection().apply { connectTimeout = 1000 }).getInputStream() }.exceptionOrNull() !is java.net.ConnectException) break
            assertTrue("server not started: " + File(tmp.root, "server.log").readText(), server.isAlive && java.lang.System.currentTimeMillis() < limit)
            Thread.sleep(100)
        }
    }

    @After
    fun stop() {
        if (::server.isInitialized) server.destroy()
    }

    private fun syncer(name: String, reminded: () -> Boolean = { true }, clock: () -> Long = { java.lang.System.currentTimeMillis() }) =
        Syncer(Remote(url, "test-token"), Bases(File(tmp.root, "bases-$name")), Backups(File(tmp.root, "copies-$name")), Pending(File(tmp.root, "pending-$name.json")), reminded, clock)

    private fun gba(emulator: String, f: MemoryFile) = Game(System.GBA, "GBA-BPEF", "Pokemon Emeraude", f, Format(), emulator)

    @Test
    fun syncFlow() {
        val a = syncer("a")
        val b = syncer("b")
        val sram = random(128 shl 10, 1)
        val fa = MemoryFile("Pokemon Emeraude.sav", sram + random(16, 9)) // mGBA Clock trailer: stays on this phone
        val fb = MemoryFile("Pokemon Emeraude.sav")
        val ga = gba("mgba", fa)
        val gb = gba("pizzaboy", fb)

        assertEquals(State.TO_UPLOAD, a.status(ga).state)
        assertEquals(Outcome.UPLOADED, a.sync(ga))
        val v = a.serverVersion("GBA-BPEF")!!
        assertEquals(sha256(sram), v.hash) // SRAM only
        assertEquals("android", v.client)
        assertEquals("Pokemon Emeraude", v.name) // ADR 0018
        assertEquals(State.IN_SYNC, a.status(ga).state)
        assertNull(b.status(gba("pizzaboy", MemoryFile("none.sav"))).local)

        assertEquals(Outcome.DOWNLOADED, b.sync(gb))
        assertArrayEquals(sram, fb.content) // no trailer on this side
        assertEquals(Outcome.IN_SYNC, b.sync(gb))

        // A plays: B downloads, after a Backup copy of its own file.
        val sram2 = random(128 shl 10, 2)
        fa.content = sram2 + random(16, 9)
        assertEquals(Outcome.UPLOADED, a.sync(ga))
        b.refresh()
        assertEquals(State.TO_DOWNLOAD, b.status(gb).state)
        assertEquals(Outcome.NOT_REPLACED, b.sync(gb, mayReplace = false)) // Sync all without confirmation
        assertArrayEquals(sram, fb.content)
        assertEquals(Outcome.DOWNLOADED, b.sync(gb, mayReplace = true)) // confirmed by the user
        assertArrayEquals(sram2, fb.content)
        assertArrayEquals(sram, File(tmp.root, "copies-b/pizzaboy").listFiles()!!.single().readBytes())

        // Both play: Conflict, nothing replaced without a choice for the version the user saw.
        fa.content = random(128 shl 10, 3) + random(16, 9)
        fb.content = random(128 shl 10, 4)
        assertEquals(Outcome.UPLOADED, a.sync(ga))
        b.refresh()
        assertEquals(State.CONFLICT, b.status(gb).state)
        assertEquals(Outcome.CONFLICT, b.sync(gb))
        assertEquals(Outcome.CONFLICT, b.sync(gb, Choice(keepPhone = false, serverHash = sha256(sram2))))
        assertArrayEquals(random(128 shl 10, 4), fb.content)
        assertEquals(Outcome.DOWNLOADED, b.sync(gb, Choice(keepPhone = false, serverHash = sha256(random(128 shl 10, 3)))))
        assertArrayEquals(random(128 shl 10, 3), fb.content)

        // 412: B uploads with an old list while A uploaded meanwhile; the new pass sees the Conflict.
        fb.content = random(128 shl 10, 5)
        assertEquals(State.TO_UPLOAD, b.status(gb).state) // list read now
        fa.content = random(128 shl 10, 6) + random(16, 9)
        assertEquals(Outcome.UPLOADED, a.sync(ga))
        assertEquals(Outcome.CONFLICT, b.sync(gb))
        assertEquals(Outcome.UPLOADED, b.sync(gb, Choice(keepPhone = true, serverHash = sha256(random(128 shl 10, 6)))))
        a.refresh()
        assertEquals(State.TO_DOWNLOAD, a.status(ga).state)

        // A Blank save counts as absent: Download, after a copy of the blank file.
        val fc = MemoryFile("blank.sav", ByteArray(128 shl 10))
        assertEquals(State.TO_DOWNLOAD, a.status(gba("other", fc)).state)
    }

    // A choice to keep the phone save never downloads, even when the State became To download meanwhile.
    @Test
    fun keepPhoneNeverDownloads() {
        val a = syncer("a")
        val b = syncer("b")
        val fa = MemoryFile("e.sav", random(32 shl 10, 1))
        val fb = MemoryFile("e.sav")
        assertEquals(Outcome.UPLOADED, a.sync(gba("mgba", fa)))
        assertEquals(Outcome.DOWNLOADED, b.sync(gba("pizzaboy", fb)))
        fa.content = random(32 shl 10, 2)
        assertEquals(Outcome.UPLOADED, a.sync(gba("mgba", fa)))
        b.refresh()
        val choice = Choice(keepPhone = true, serverHash = sha256(random(32 shl 10, 2)))
        assertEquals(Outcome.NOT_REPLACED, b.sync(gba("pizzaboy", fb), choice, mayReplace = false))
        assertArrayEquals(random(32 shl 10, 1), fb.content)
    }

    // The written file must give back the received SRAM: a trailer that would no longer be found is refused.
    @Test
    fun downloadKeepsShape() {
        val a = syncer("a")
        val b = syncer("b")
        assertEquals(Outcome.UPLOADED, a.sync(gba("mgba", MemoryFile("e.sav", random(1000, 1)))))
        val old = ByteArray(32 shl 10) + random(16, 9) // blank, with a Clock trailer
        val fb = MemoryFile("e.sav", old)
        val e = runCatching { b.sync(gba("pizzaboy", fb), mayReplace = true) }.exceptionOrNull()!!
        assertEquals(Problem.FORMAT, problem(e))
        assertArrayEquals(old, fb.content)
    }

    // Sync all never replaces a save the user did not confirm (here: blank, created after the list was shown), and
    // stops after a failed write on this phone.
    @Test
    fun syncAll() {
        val a = syncer("a")
        val b = syncer("b")
        assertEquals(Outcome.UPLOADED, a.sync(gba("mgba", MemoryFile("e.sav", random(32 shl 10, 1)))))
        val fb = MemoryFile("e.sav", ByteArray(32 shl 10))
        val gb = gba("pizzaboy", fb)
        val s = b.syncAll(listOf(gb), emptySet())
        assertEquals(listOf("Pokemon Emeraude"), s.notReplaced)
        assertArrayEquals(ByteArray(32 shl 10), fb.content)
        assertEquals(1, b.syncAll(listOf(gb), setOf(gb)).downloaded)
        assertArrayEquals(random(32 shl 10, 1), fb.content)

        val bad = gba("other", MemoryFile("e.sav", ByteArray(32 shl 10), bad = 2))
        val next = Game(System.GBA, "GBA-AXVE", "Pokemon Ruby", MemoryFile("r.sav", random(32 shl 10, 5)), Format(), "other")
        val r = b.syncAll(listOf(bad, next), setOf(bad))
        assertEquals(Problem.RESTORE, problem(r.failures.single().second))
        assertTrue(r.stopped)
        assertNull(b.serverVersion("GBA-AXVE"))
    }

    // Sync all refuses the downloads while the emulator may be open, and goes on with the next games.
    @Test
    fun emulatorOpen() {
        assertEquals(Outcome.UPLOADED, syncer("a").sync(gba("mgba", MemoryFile("e.sav", random(32 shl 10, 1)))))
        var now = 1_800_000_000_000L
        var reminded = false
        val b = syncer("b", { reminded }, { now })
        val recent = gba("recent", MemoryFile("e.sav", ByteArray(32 shl 10), date = now - 1_000)) // blank: To download
        val absent = gba("absent", MemoryFile("e.sav")) // no file: no date
        val all = listOf(recent, absent)

        var s = b.syncAll(all, all.toSet())
        assertEquals(listOf(Problem.OPEN, Problem.OPEN), s.failures.map { problem(it.second) })
        assertFalse(s.stopped)
        assertArrayEquals(ByteArray(32 shl 10), recent.file.read())
        assertNull(absent.file.read())

        reminded = true // the reminder was shown: the unknown date is allowed, the recent change is not
        s = b.syncAll(all, all.toSet())
        assertEquals(1, s.downloaded)
        assertEquals(listOf("Pokemon Emeraude" to Problem.OPEN), s.failures.map { it.first to problem(it.second) })
        assertArrayEquals(ByteArray(32 shl 10), recent.file.read())

        now += 30_000 // the emulator was closed
        assertEquals(Outcome.DOWNLOADED, b.sync(recent, mayReplace = true))
        assertArrayEquals(random(32 shl 10, 1), recent.file.read())
    }

    // A 3DS game never started in Azahar: To download, its first Download creates the folder; the next one is a normal
    // Download over the save it wrote, after a Backup copy.
    @Test
    fun neverStarted3DS() {
        val a = syncer("a")
        val b = syncer("b")
        fun y(f: MemoryFile, archive: NewArchive? = null) = Game(System.N3DS, "3DS-0004000000055E00", "Pokemon Y", f, Format(""), "azahar", archive)
        val pc = MemoryFile("main", random(415232, 1))
        assertEquals(Outcome.UPLOADED, a.sync(y(pc)))
        val arch = MemoryArchive(MemoryFile("main"))
        val g = y(arch.main, arch)
        assertEquals(State.TO_DOWNLOAD, b.status(g).state)
        assertEquals(1, b.syncAll(listOf(g), emptySet()).downloaded) // no save on this phone: no confirmation needed
        assertArrayEquals(random(415232, 1), arch.main.content)
        assertArrayEquals(formats3DS.getValue("0004000000055E00"), arch.metadata)
        assertEquals(Outcome.IN_SYNC, b.sync(g))

        pc.content = random(415232, 2)
        assertEquals(Outcome.UPLOADED, a.sync(y(pc)))
        b.refresh()
        assertEquals(Outcome.DOWNLOADED, b.sync(g, mayReplace = true)) // the folder exists now: no second .metadata
        assertArrayEquals(random(415232, 2), arch.main.content)
        assertArrayEquals(random(415232, 1), File(tmp.root, "copies-b/azahar").listFiles()!!.single().readBytes())
    }

    // History and Restore (ADR 0016): newest first; a Restore is a new Version by "restauration" over the Current version
    // the user saw, only on the server; the phone gets it through the usual Download. Every failure changes nothing.
    @Test
    fun historyAndRestore() {
        val a = syncer("a")
        val f = MemoryFile("e.sav", random(32 shl 10, 1))
        val g = gba("mgba", f)
        assertEquals(Outcome.UPLOADED, a.sync(g))
        f.content = random(32 shl 10, 2)
        assertEquals(Outcome.UPLOADED, a.sync(g))
        val remote = Remote(url, "test-token", RESTORE_CLIENT)
        val h = remote.history("GBA-BPEF")
        assertEquals(listOf(2, 1), h.map { it.number })
        assertEquals(listOf(sha256(random(32 shl 10, 2)), sha256(random(32 shl 10, 1))), h.map { it.hash })
        assertEquals(listOf("android", "android"), h.map { it.client })
        assertEquals(32L shl 10, h[1].size)
        assertTrue(remote.history("GBA-AXVE").isEmpty())

        restoreVersion(remote, "GBA-BPEF", h[1], h[0])
        val after = remote.history("GBA-BPEF")
        assertEquals(listOf(3, 2, 1), after.map { it.number }) // nothing deleted
        assertEquals(RESTORE_CLIENT, after[0].client)
        assertEquals(h[1].hash, after[0].hash)
        assertArrayEquals(random(32 shl 10, 2), f.content) // the phone save is not touched
        a.refresh()
        assertEquals(State.TO_DOWNLOAD, a.status(g).state)
        assertEquals(Outcome.DOWNLOADED, a.sync(g, mayReplace = true)) // the usual Download, with its Backup copy
        assertArrayEquals(random(32 shl 10, 1), f.content)
        assertArrayEquals(random(32 shl 10, 2), File(tmp.root, "copies-a/mgba").listFiles()!!.single().readBytes())

        fun fails(v: Version, current: Version, r: Remote = remote) = problem(runCatching { restoreVersion(r, "GBA-BPEF", v, current) }.exceptionOrNull()!!)
        assertEquals(Problem.SERVER_CHANGED, fails(after[1], h[0])) // 412: the current version seen is no longer current
        assertEquals(Problem.VERSION_GONE, fails(after[1].copy(file = "000099" + after[1].file.drop(6)), after[0]))
        assertEquals(Problem.VERSION_DAMAGED, fails(after[1].copy(hash = "0".repeat(64)), after[0]))
        assertEquals(Problem.REFUSED, fails(after[1], after[0], Remote("http://127.0.0.1:1", "x", RESTORE_CLIENT, 2000)))
        assertEquals(after.map { it.file }, remote.history("GBA-BPEF").map { it.file }) // no new Version
    }

    // Background upload: only the Uploads. A To download game and a Conflict game stay as they are, with their Bases;
    // a save changed less than 30 s ago waits for the next pass.
    @Test
    fun backgroundUploadsOnly() {
        val a = syncer("a")
        val now = 1_800_000_000_000L
        val b = syncer("b", { true }, { now })
        fun g(key: String, emulator: String, f: MemoryFile) = Game(System.GBA, key, key, f, Format(), emulator)
        val fa1 = MemoryFile("1.sav", random(32 shl 10, 1))
        val fa2 = MemoryFile("2.sav", random(32 shl 10, 2))
        assertEquals(Outcome.UPLOADED, a.sync(g("GBA-BPEF", "mgba", fa1)))
        assertEquals(Outcome.UPLOADED, a.sync(g("GBA-AXVE", "mgba", fa2)))
        val down = MemoryFile("1.sav")
        val conflict = MemoryFile("2.sav")
        val gDown = g("GBA-BPEF", "pizzaboy", down)
        val gConflict = g("GBA-AXVE", "pizzaboy", conflict)
        assertEquals(2, b.syncAll(listOf(gDown, gConflict), emptySet()).downloaded)
        fa1.content = random(32 shl 10, 3)
        fa2.content = random(32 shl 10, 4)
        assertEquals(Outcome.UPLOADED, a.sync(g("GBA-BPEF", "mgba", fa1)))
        assertEquals(Outcome.UPLOADED, a.sync(g("GBA-AXVE", "mgba", fa2)))
        conflict.content = random(32 shl 10, 5) // played on both sides
        val up = g("GBA-BPRE", "pizzaboy", MemoryFile("3.sav", random(32 shl 10, 6)))
        val recent = g("GBA-AXPE", "pizzaboy", MemoryFile("4.sav", random(32 shl 10, 7), date = now - 1_000))
        val bases = Bases(File(tmp.root, "bases-b"))
        val before = bases.of("pizzaboy")
        b.refresh()

        val r = b.uploadAll(listOf(gDown, gConflict, up, recent))
        assertEquals(1, r.uploaded)
        assertEquals(listOf("GBA-AXVE"), r.conflicts)
        assertEquals(listOf("GBA-AXPE"), r.recent)
        assertTrue(r.failures.isEmpty())
        assertArrayEquals(random(32 shl 10, 1), down.content) // no Download
        assertArrayEquals(random(32 shl 10, 5), conflict.content)
        assertEquals(before + ("GBA-BPRE" to sha256(random(32 shl 10, 6))), bases.of("pizzaboy")) // only the Upload's Base
        assertEquals(sha256(random(32 shl 10, 3)), b.serverVersion("GBA-BPEF")!!.hash)
        assertEquals(sha256(random(32 shl 10, 4)), b.serverVersion("GBA-AXVE")!!.hash)
        assertNull(b.serverVersion("GBA-AXPE"))
        assertFalse(File(tmp.root, "copies-b").exists()) // no Backup copy: nothing was replaced
    }

    // An emulator left open writes its old save back after a Download (ADR 0026): Conflict with its reason, never To
    // upload, never uploaded in the background. The rule stays while In sync; an Upload ends it.
    @Test
    fun emulatorWroteOldSaveBack() {
        val a = syncer("a")
        val b = syncer("b")
        val server = random(32 shl 10, 1)
        val old = random(32 shl 10, 9) // the progress on this phone before the Download
        val fb = MemoryFile("e.sav", old)
        val gb = gba("pizzaboy", fb)
        val bases = Bases(File(tmp.root, "bases-b"))
        assertEquals(Outcome.UPLOADED, a.sync(gba("mgba", MemoryFile("e.sav", server))))
        assertEquals(Outcome.DOWNLOADED, b.sync(gb, Choice(keepPhone = false, serverHash = sha256(server))))
        assertEquals(mapOf("GBA-BPEF" to sha256(old)), bases.replaced("pizzaboy"))
        assertEquals(Outcome.IN_SYNC, b.sync(gb))
        assertFalse(b.status(gb).cameBack)

        fb.content = old // the emulator writes its old save back
        val st = b.status(gb)
        assertEquals(State.CONFLICT, st.state)
        assertTrue(st.cameBack)
        assertEquals(Outcome.CONFLICT, b.sync(gb, mayReplace = true))
        assertEquals(listOf("Pokemon Emeraude"), b.syncAll(listOf(gb), setOf(gb)).conflicts)
        val bg = b.uploadAll(listOf(gb))
        assertEquals(0, bg.uploaded)
        assertEquals(listOf("Pokemon Emeraude"), bg.conflicts)
        assertEquals(sha256(server), b.serverVersion("GBA-BPEF")!!.hash)

        // "Use the server save" writes it again (Backup copy first); the rule holds again
        assertEquals(Outcome.DOWNLOADED, b.sync(gb, Choice(keepPhone = false, serverHash = sha256(server))))
        assertArrayEquals(server, fb.content)
        fb.content = old
        assertTrue(b.status(gb).cameBack)
        // "Keep this phone's save" is the user's choice: uploaded, and the rule ends
        assertEquals(Outcome.UPLOADED, b.sync(gb, Choice(keepPhone = true, serverHash = sha256(server))))
        assertTrue(bases.replaced("pizzaboy").isEmpty())
        fb.content = random(32 shl 10, 3) // played on this phone: To upload as usual
        assertEquals(State.TO_UPLOAD, b.status(gb).state)
        assertEquals(1, b.uploadAll(listOf(gb)).uploaded)
    }

    // A write that did not finish (the app was killed between the marker and its removal, ADR 0026): Interrupted, a
    // Conflict that is never uploaded and never kept as it is. "Use the server save" is a normal Download (the new file
    // takes its shape from the Backup copy); "Restore the backup copy" writes the copy back. A marker whose write did
    // finish is dropped.
    @Test
    fun interruptedWrite() {
        val a = syncer("a")
        val b = syncer("b")
        val server = random(32 shl 10, 1)
        assertEquals(Outcome.UPLOADED, a.sync(gba("mgba", MemoryFile("e.sav", server))))
        val old = random(32 shl 10, 9) + random(16, 5) // with an RTC trailer
        val copy = File(tmp.root, "copy.sav").apply { writeBytes(old) }
        val fb = MemoryFile("e.sav", old.copyOf(1000)) // cut by the kill
        val gb = gba("pizzaboy", fb)
        val pending = Pending(File(tmp.root, "pending-b.json"))
        pending.set(gb, PendingWrite(copy.path, sha256(server + random(16, 5))))

        val st = b.status(gb)
        assertEquals(State.CONFLICT, st.state)
        assertEquals(copy.path, st.interrupted!!.backup)
        assertEquals(Outcome.CONFLICT, b.sync(gb, mayReplace = true))
        assertEquals(Outcome.CONFLICT, b.sync(gb, Choice(keepPhone = true, serverHash = sha256(server))))
        assertEquals(listOf("Pokemon Emeraude"), b.uploadAll(listOf(gb)).conflicts)
        assertEquals(sha256(server), b.serverVersion("GBA-BPEF")!!.hash) // nothing uploaded
        assertArrayEquals(old.copyOf(1000), fb.content)

        assertTrue(b.restoreCopy(gb))
        assertArrayEquals(old, fb.content)
        assertNull(pending.get(gb))
        assertArrayEquals(old.copyOf(1000), File(tmp.root, "copies-b/pizzaboy").listFiles()!!.single().readBytes()) // the cut file
        assertNull(b.status(gb).interrupted)
        assertFalse(b.restoreCopy(gb)) // nothing to restore any more

        fb.content = old.copyOf(1000)
        pending.set(gb, PendingWrite(copy.path, sha256(server + random(16, 5))))
        assertEquals(Outcome.DOWNLOADED, b.sync(gb, Choice(keepPhone = false, serverHash = sha256(server))))
        assertArrayEquals(server + random(16, 5), fb.content) // the trailer of the copy, not of the cut file
        assertNull(pending.get(gb))
        assertEquals(State.IN_SYNC, b.status(gb).state)

        pending.set(gb, PendingWrite(null, sha256(fb.content!!))) // killed after the write, before the removal
        assertNull(b.status(gb).interrupted)
        assertNull(pending.get(gb))

        copy.delete() // no copy left: only the server save is offered
        fb.content = old.copyOf(1000)
        pending.set(gb, PendingWrite(copy.path, "x"))
        assertNull(b.status(gb).interrupted!!.backup)
        assertEquals(Problem.NO_COPY, problem(runCatching { b.restoreCopy(gb) }.exceptionOrNull()!!))
        assertArrayEquals(old.copyOf(1000), fb.content)
    }

    @Test
    fun wrongToken() {
        val e = runCatching { Remote(url, "bad").list() }.exceptionOrNull()!!
        assertEquals(Problem.TOKEN, problem(e))
        val refused = runCatching { Remote("http://127.0.0.1:1", "x", timeoutMs = 2000).list() }.exceptionOrNull()!!
        assertEquals(Problem.REFUSED, problem(refused))
    }
}
