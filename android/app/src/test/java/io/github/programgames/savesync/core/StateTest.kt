package io.github.programgames.savesync.core

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

// The shared fixtures of every client (tests/ at the root of the repo), read in place, never copied.
class StateTest {
    // Same table as 3ds/test/test_state.c and client/state_test.go: tests/cas_etat.csv, "-" = absent.
    @Test
    fun stateTable() {
        val lines = File("../tests/cas_etat.csv").readLines(Charsets.UTF_8).drop(1).filter { it.isNotBlank() }
        assertTrue(lines.size >= 12)
        fun obs(hash: String, size: String) = if (hash == "-") null else Version(hash, size.toLong())
        for (l in lines) {
            val c = l.split(';')
            val got = state(obs(c[1], c[2]), obs(c[3], c[4]), c[5].takeIf { it != "-" })
            assertEquals(c[0], c[6], got.fixture)
        }
    }

    // tests/saves.json, written by server/server.py and read by 3ds/test/test_list.c: same expectations.
    @Test
    fun serverList() {
        val l = parseList(File("../tests/saves.json").readText(Charsets.UTF_8))
        assertEquals(3, l.size)
        val h1 = "49abd65bbf7f7e40c7055093ed2e3fd75f2f602f2c5fcf955c213e3135eb03f7"
        val h2 = "07854d2fef297a06ba81685e660c332de36d5d18d546927d30daad6d7fda1541"
        with(l[0]) {
            assertEquals("3DS-0004000000055D00", code)
            assertEquals(h1, hash)
            assertEquals(1024L, size)
            assertEquals("20260101T120000Z", date)
            assertEquals("3ds", client)
            assertTrue(age!! > 0)
            assertEquals("Pokémon X", name)
        }
        with(l[1]) { // no name: never the one of the next game
            assertEquals("ADAF", code)
            assertEquals(h2, hash)
            assertEquals(524288L, size)
            assertEquals("20260102T081011Z", date)
            assertEquals("pc", client)
            assertTrue(age!! > 0)
            assertEquals("", name)
        }
        with(l[2]) {
            assertEquals("GBA-BPEF", code)
            assertEquals(131072L, size)
            assertEquals("3ds", client)
            assertEquals("Pokémon Émeraude {hack} \"FR\"", name)
        }
    }
}
