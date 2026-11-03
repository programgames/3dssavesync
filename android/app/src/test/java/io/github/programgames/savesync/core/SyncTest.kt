package io.github.programgames.savesync.core

import org.junit.After
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
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
class MemoryFile(override val name: String, var content: ByteArray? = null, var bad: Int = 0) : SaveFile {
    override val location get() = "test/$name"

    override fun read() = content?.copyOf()

    override fun write(content: ByteArray) {
        this.content = if (bad-- > 0) content.copyOf(content.size / 2) else content.copyOf()
    }

    override fun modified(): Long? = null
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

    @Test
    fun duplicates() {
        val (kept, dup) = splitDuplicates(
            listOf("A" to "x.sav", "B" to "y.sav", "B" to "z.sav", "C" to "X.SAV", "D" to "d.sav"), { it.first }, { it.second },
        )
        assertEquals(listOf("D" to "d.sav"), kept)
        assertEquals(4, dup.size)
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
        ).directory(File("../server")).redirectErrorStream(true).redirectOutput(File(tmp.root, "server.log")).start()
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

    private fun syncer(name: String) = Syncer(Remote(url, "test-token"), Bases(File(tmp.root, "bases-$name")), Backups(File(tmp.root, "copies-$name")))

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
        assertEquals(Outcome.DOWNLOADED, b.sync(gb))
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

    @Test
    fun wrongToken() {
        val e = runCatching { Remote(url, "bad").list() }.exceptionOrNull()!!
        assertEquals(Problem.TOKEN, problem(e))
        val refused = runCatching { Remote("http://127.0.0.1:1", "x", timeoutMs = 2000).list() }.exceptionOrNull()!!
        assertEquals(Problem.REFUSED, problem(refused))
    }
}
