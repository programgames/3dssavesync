package io.github.programgames.savesync

import android.content.Context
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.text.TextLayoutResult
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasClickAction
import androidx.compose.ui.test.hasSetTextAction
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.isDialog
import androidx.compose.ui.test.isSelectable
import androidx.compose.ui.test.junit4.createEmptyComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.hasScrollToNodeAction
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performScrollToIndex
import androidx.compose.ui.test.performScrollToNode
import androidx.compose.ui.test.performTextClearance
import androidx.compose.ui.test.performTextInput
import androidx.test.core.app.ActivityScenario
import androidx.test.core.app.ApplicationProvider
import io.github.programgames.savesync.core.Format
import io.github.programgames.savesync.core.Game
import io.github.programgames.savesync.core.Local
import io.github.programgames.savesync.core.Problem
import io.github.programgames.savesync.core.SaveFile
import io.github.programgames.savesync.core.State
import io.github.programgames.savesync.core.Status
import io.github.programgames.savesync.core.Summary
import io.github.programgames.savesync.core.SyncException
import io.github.programgames.savesync.core.System
import io.github.programgames.savesync.core.Version
import io.github.programgames.savesync.core.mainSunMoon
import io.github.programgames.savesync.core.sha256
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import java.io.IOException
import java.net.InetAddress
import java.net.ServerSocket
import kotlin.concurrent.thread

// MemFile is a Save file in memory: the tests check that nothing was written without a confirmation.
class MemFile(override val name: String, var content: ByteArray?) : SaveFile {
    var writes = 0
    override val location get() = "saves/$name"
    override fun read() = content
    override fun write(content: ByteArray) {
        writes++
        this.content = content
    }
    override fun modified() = 1_700_000_000_000L
}

// Smoke tests of every screen, on the JVM (Robolectric): English, normal screen. UiFrSmallTest runs them again in
// French on a small screen.
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE) // real text measures, for the checks of cut texts
open class UiTest {
    @get:Rule
    val rule = createEmptyComposeRule()

    protected val ctx: Context get() = ApplicationProvider.getApplicationContext()
    private fun s(id: Int, vararg a: Any) = ctx.getString(id, *a)
    private var scenario: ActivityScenario<MainActivity>? = null
    private var server: ServerSocket? = null
    private val requests = mutableListOf<String>()

    @After
    fun close() {
        scenario?.close()
        server?.close()
    }

    // start launches the app after a first start with these settings. entries: the game list as read, else not read.
    // noReminder: "Don't show again" was chosen on the reminder to close the emulator.
    private fun start(
        url: String = "",
        token: String = "",
        emulators: List<Emulator> = emptyList(),
        entries: List<Entry>? = null,
        noReminder: Boolean = true,
    ): ActivityScenario<MainActivity> {
        App.init(ctx)
        App.settings.noReminder = noReminder
        App.settings.url = url
        App.settings.token = token
        App.saveEmulators(emulators)
        App.serverChanged()
        App.entries = entries.orEmpty()
        App.loaded = entries != null
        App.serverError = null
        App.scanProblems = emptyList()
        App.busy = null
        App.summary = null
        App.message = null
        return ActivityScenario.launch(MainActivity::class.java).also { scenario = it }
    }

    // fakeServer runs on this computer only: GET /saves is empty, GET /saves/<key> returns save; PUTs are recorded.
    private fun fakeServer(save: ByteArray): String {
        val ss = ServerSocket(0, 50, InetAddress.getByName("127.0.0.1"))
        thread(isDaemon = true) {
            while (true) {
                val c = runCatching { ss.accept() }.getOrNull() ?: break
                c.use {
                    val r = it.getInputStream().bufferedReader()
                    val line = r.readLine() ?: return@use
                    while (!r.readLine().isNullOrEmpty()) Unit // headers
                    synchronized(requests) { requests += line }
                    val body = if (line.split(" ")[1] == "/saves") "[]".toByteArray() else save
                    it.getOutputStream().write("HTTP/1.1 200 OK\r\nContent-Length: ${body.size}\r\nConnection: close\r\n\r\n".toByteArray() + body)
                }
            }
        }
        server = ss
        return "http://127.0.0.1:${ss.localPort}"
    }

    private fun waitFor(text: String, substring: Boolean = false) =
        rule.waitUntil(15_000) { rule.onAllNodesWithText(text, substring).fetchSemanticsNodes().isNotEmpty() }

    // fits: every Text with this text is inside the screen width and shows all of it (not cut, at most maxLines lines).
    private fun fits(text: String, maxLines: Int = 1) {
        val nodes = rule.onAllNodesWithText(text, useUnmergedTree = true).fetchSemanticsNodes()
        assertTrue("no $text", nodes.isNotEmpty())
        val w = ctx.resources.displayMetrics.widthPixels
        for (n in nodes) {
            val x = n.positionInRoot.x // not clipped by a scrolled parent, unlike boundsInWindow
            assertTrue("$text at $x, width ${n.size.width}, screen $w", x >= 0f && x + n.size.width <= w + 1 && n.size.width > 0)
            val l = mutableListOf<TextLayoutResult>()
            n.config[SemanticsActions.GetTextLayoutResult].action!!(l)
            // didOverflowWidth is not usable here: the paragraph is as wide as the constraints, not as the text
            val t = l.single()
            val widest = (0 until t.lineCount).maxOf { t.getLineRight(it) - t.getLineLeft(it) }
            assertTrue(
                "$text cut: ${t.lineCount} lines, $widest px wide in ${n.size}",
                !t.didOverflowHeight && t.lineCount <= maxLines && widest <= n.size.width + 1 && t.getLineEnd(t.lineCount - 1, true) == text.length,
            )
        }
    }

    private val dialog = hasAnyAncestor(isDialog())

    @Test
    fun firstStartShowsTheSettingsAndAClearConnectionError() {
        val sc = start()
        rule.onNodeWithText(s(R.string.settings_title)).assertIsDisplayed()
        rule.onNodeWithText(s(R.string.test_connection)).assertIsNotEnabled()
        val fields = rule.onAllNodes(hasSetTextAction())
        fields.assertCountEquals(2)
        fields[0].performTextInput("127.0.0.1:1")
        rule.onNodeWithText(s(R.string.address_scheme)).assertIsDisplayed()
        fields[0].performTextClearance()
        fields[0].performTextInput("http://127.0.0.1:1") // nothing listens on port 1: refused at once
        fields[1].performTextInput("secret-token")
        // the token is masked until Show
        rule.onNode(SemanticsMatcher.keyIsDefined(SemanticsProperties.Password)).assertExists()
        val shown = { fields[1].fetchSemanticsNode().config[SemanticsProperties.EditableText].text }
        assertFalse(shown().contains("secret"))
        rule.onNodeWithText(s(R.string.show)).performClick()
        assertEquals("secret-token", shown())
        rule.onNodeWithText(s(R.string.test_connection)).assertIsEnabled().performClick()
        waitFor(s(R.string.p_refused))
        rule.onNodeWithText("ConnectException", substring = true).assertExists() // the technical line

        sc.recreate() // rotation: the settings stay
        rule.onNodeWithText("http://127.0.0.1:1").assertExists()
        rule.onNodeWithText(s(R.string.test_connection)).assertIsEnabled()
    }

    @Test
    fun emulatorsAddEveryProfile() {
        val sc = start(url = "http://127.0.0.1:1", token = "t")
        rule.onNodeWithText(s(R.string.need_emulator)).assertIsDisplayed()
        rule.onNodeWithText(s(R.string.add_emulator)).performClick()
        rule.onNodeWithText(s(R.string.no_emulator)).assertIsDisplayed()
        for (pr in profiles) {
            val name = if (pr == CUSTOM) s(R.string.custom_profile) else pr.name
            rule.onNodeWithText(s(R.string.add_emulator)).performScrollTo().performClick()
            rule.onNode(hasText(name) and hasClickAction() and dialog).performClick()
            rule.onNodeWithText(s(pr.hint)).assertExists()
            waitFor(s(R.string.folder_not_set))
            for (v in pr.systems.values.flatten().filter { it.label.isNotEmpty() }) fits(v.label) // the core chips
            if (pr == CUSTOM) for (sys in pr.systems.keys) fits(sys.label)
            for (sys in pr.systems.keys) rule.onNodeWithText(systemNameText(sys)).assertExists()
            sc.recreate() // rotation: still on this emulator
            rule.onNodeWithText(s(pr.hint)).assertExists()
            rule.onNodeWithContentDescription(s(R.string.back)).performClick()
        }
        rule.onNode(hasText(s(R.string.emulators_title)) and !hasClickAction()).assertIsDisplayed() // the title, not the tab
        assertEquals(profiles.size, App.emulators.size)
        rule.waitUntil(15_000) { rule.onAllNodesWithText(s(R.string.folder_not_set)).fetchSemanticsNodes().size == profiles.size }
    }

    private fun systemNameText(sys: System) = s(
        when (sys) {
            System.DS -> R.string.system_ds
            System.GB -> R.string.system_gb
            System.GBA -> R.string.system_gba
            System.N3DS -> R.string.system_3ds
        },
    )

    // The fake game list: one game per State, a Duplicate, one without save (hidden) and one Unknown.
    private val emu = Emulator("retroarch", "retroarch", "RetroArch", mapOf(System.GBA to Folders()))
    private val sram = ByteArray(0x8000) { (it % 7).toByte() }
    private val sun = mainSunMoon(12, 5, "Ash")
    private fun local(b: ByteArray) = Local(Version(sha256(b), b.size.toLong()), b, b, blank = false)
    private val other = Version("b".repeat(64), 0x8000, client = "pc", date = "20261103T101500Z", age = 3600)
    private val files = mutableMapOf<String, MemFile>()

    private fun entry(name: String, sys: System, key: String, l: Local?, srv: Version?, base: String?, state: State?, error: Throwable? = null): Entry {
        val f = MemFile("$name.sav", l?.raw).also { files[name] = it }
        return Entry(emu, sys, name, "$name.rom", Game(sys, key, name, f, Format(), emu.id), status = if (error == null) Status(l, srv, base, state) else null, error = error)
    }

    private fun games() = listOf(
        entry("Pokémon Émeraude", System.GBA, "GBA-BPEF", local(sram), null, null, State.TO_UPLOAD),
        entry("Zelda", System.GB, "GB-ZELDA-1234", local(sram), Version(sha256(sram), 0x8000), sha256(sram), State.IN_SYNC),
        entry("Mario Kart DS", System.DS, "AMCE", local(sram), other, sha256(sram), State.TO_DOWNLOAD),
        entry("Pokemon Soleil", System.N3DS, "3DS-0004000000164800", local(sun), other.copy(size = sun.size.toLong()), "c".repeat(64), State.CONFLICT),
        entry("Metroid", System.GBA, "GBA-AMTE", null, null, null, null),
        entry("Wario", System.GBA, "GBA-RWZE", null, null, null, null, error = IOException("read error")),
        Entry(emu, System.GB, "Tetris", "Tetris.gb", null, dups = listOf("Tetris.gb", "Tetris (1).gb")),
    )

    private fun chip(label: String) = rule.onNode(hasText(label) and isSelectable()).performScrollTo()

    @Test
    fun gamesListFiltersSearchAndSummary() {
        start(url = "http://127.0.0.1:1", token = "t", emulators = listOf(emu), entries = games())
        for (n in listOf("Pokémon Émeraude", "Zelda", "Mario Kart DS", "Pokemon Soleil", "Wario", "Tetris")) {
            rule.onNode(hasScrollToNodeAction()).performScrollToNode(hasText(n)) // the list composes only what fits
        }
        rule.onNode(hasScrollToNodeAction()).performScrollToNode(hasText(ctx.resources.getQuantityString(R.plurals.hidden_games, 1, 1)))
        rule.onAllNodesWithText("Metroid").assertCountEquals(0)
        rule.onNode(hasScrollToNodeAction()).performScrollToIndex(0)
        for (t in listOf(R.string.sync_all, R.string.refresh, R.string.tab_games, R.string.s_to_upload, R.string.s_in_sync, R.string.s_to_download, R.string.s_conflict)) fits(s(t))

        chip(s(R.string.filter_todo)).performClick()
        for (n in listOf("Pokémon Émeraude", "Mario Kart DS", "Pokemon Soleil")) rule.onNodeWithText(n).assertExists()
        for (n in listOf("Zelda", "Wario", "Tetris")) rule.onAllNodesWithText(n).assertCountEquals(0)
        chip("3DS").performClick()
        rule.onNodeWithText("Pokemon Soleil").assertExists()
        rule.onAllNodesWithText("Mario Kart DS").assertCountEquals(0)
        chip(s(R.string.filter_all)).performClick()

        val search = rule.onNode(hasSetTextAction())
        search.performTextInput("pokemon") // accent-insensitive
        rule.onNodeWithText("Pokémon Émeraude").assertExists()
        rule.onNodeWithText("Pokemon Soleil").assertExists()
        rule.onAllNodesWithText("Zelda").assertCountEquals(0)
        search.performTextClearance()
        search.performTextInput("ÉMERAUDE")
        rule.onNodeWithText("Pokémon Émeraude").assertExists()
        search.performTextClearance()
        search.performTextInput("no such game")
        rule.onNodeWithText(s(R.string.no_match)).assertExists()
        search.performTextClearance()

        // Sync all asks before a download replaces a save of this phone; Cancel leaves everything as it was
        rule.onNodeWithText(s(R.string.sync_all)).performClick()
        rule.onNode(hasText("Mario Kart DS", substring = true) and dialog).assertExists()
        rule.onNode(hasText(s(R.string.cancel)) and dialog).performClick()
        assertEquals(null, App.busy)
        assertTrue(files.values.all { it.writes == 0 })

        App.summary = Summary(2, 1, 3, listOf("Pokemon Soleil"), listOf("Mario Kart DS"), listOf("Zelda" to SyncException(Problem.WRITE, "write detail")), stopped = true)
        rule.onNodeWithText(s(R.string.summary_title)).assertExists()
        rule.onNodeWithText(s(R.string.summary_counts, 2, 1, 3)).assertExists()
        rule.onNodeWithText(s(R.string.summary_conflicts, "Pokemon Soleil")).assertExists()
        rule.onNodeWithText(s(R.string.summary_not_replaced, "Mario Kart DS")).assertExists()
        rule.onNodeWithText(s(R.string.summary_stopped)).assertExists()
        rule.onNodeWithText(s(R.string.p_write)).assertExists()
        rule.onNodeWithText("write detail").assertExists()
        rule.onNode(hasText(s(R.string.ok)) and dialog).performClick()
        rule.onAllNodesWithText(s(R.string.summary_title)).assertCountEquals(0)
    }

    @Test
    fun gamesEmptyStates() {
        start(url = "http://127.0.0.1:1", token = "t", emulators = listOf(emu), entries = emptyList())
        rule.onNodeWithText(s(R.string.no_game)).assertExists()
        App.settings.token = ""
        scenario!!.recreate()
        rule.onNodeWithText(s(R.string.need_server)).assertExists() // the tab stays Games: the notice shows
    }

    private fun open(name: String) {
        rule.onNode(hasScrollToNodeAction()).performScrollToNode(hasText(name))
        rule.onNodeWithText(name).performClick()
        rule.onNodeWithContentDescription(s(R.string.back)).assertExists()
    }

    private fun back() = rule.onNodeWithContentDescription(s(R.string.back)).performClick()

    @Test
    fun detailsOfEveryState() {
        val url = fakeServer(mainSunMoon(30, 7, "Red"))
        start(url = url, token = "t", emulators = listOf(emu), entries = games())

        open("Pokémon Émeraude")
        rule.onNodeWithText(s(R.string.s_to_upload)).assertExists()
        rule.onNodeWithText(s(R.string.why_to_upload)).assertExists()
        rule.onNodeWithText(s(R.string.no_server_version)).assertExists()
        fits(s(R.string.upload))
        back()

        open("Zelda")
        rule.onNodeWithText(s(R.string.why_in_sync)).assertExists()
        rule.onNodeWithText(s(R.string.last_sync) + " " + s(R.string.base_both)).assertExists()
        rule.onAllNodesWithText(s(R.string.upload)).assertCountEquals(0)
        rule.onAllNodesWithText(s(R.string.download)).assertCountEquals(0)
        back()

        open("Mario Kart DS")
        rule.onNodeWithText(s(R.string.why_to_download)).assertExists()
        rule.onNodeWithText(s(R.string.uploaded_by, "pc")).assertExists()
        rule.onNodeWithText(s(R.string.download)).performScrollTo()
        fits(s(R.string.download))
        rule.onNodeWithText(s(R.string.download)).performClick()
        rule.onNodeWithText(s(R.string.confirm_download)).assertExists()
        rule.onNode(hasText(s(R.string.cancel)) and dialog).performClick()
        back()

        open("Pokemon Soleil")
        rule.onNodeWithText(s(R.string.choose_version)).assertExists()
        rule.onNodeWithText(s(R.string.this_phone)).assertExists()
        rule.onNodeWithText(s(R.string.server)).assertExists()
        rule.onNodeWithText(s(R.string.play_info, 12, 5, "Ash")).assertExists()
        waitFor(s(R.string.play_info, 30, 7, "Red")) // read from the server
        fits(s(R.string.keep_phone), 2)
        fits(s(R.string.take_server), 2)
        for (b in listOf(R.string.keep_phone, R.string.take_server)) {
            rule.onNodeWithText(s(b)).performScrollTo().performClick()
            rule.onNodeWithText(s(if (b == R.string.keep_phone) R.string.confirm_keep_phone else R.string.confirm_take_server)).assertExists()
            rule.onNode(hasText(s(R.string.cancel)) and dialog).performClick()
        }
        rule.onNodeWithText(s(R.string.nothing_lost)).assertExists()
        scenario!!.recreate() // rotation: the conflict stays open, nothing chosen
        rule.onNodeWithText(s(R.string.choose_version)).assertExists()
        back()

        open("Tetris")
        rule.onNodeWithText(s(R.string.s_duplicate)).assertExists()
        rule.onNodeWithText(s(R.string.why_duplicate, 2), substring = true).assertExists()
        rule.onAllNodesWithText(s(R.string.this_phone)).assertCountEquals(0)
        back()

        open("Wario")
        rule.onNodeWithText(s(R.string.why_unknown)).assertExists()
        rule.onNodeWithText("read error", substring = true).assertExists()
        back()

        // no tap on Confirm: no save written, no upload
        assertEquals(null, App.busy)
        assertTrue(files.values.all { it.writes == 0 })
        assertTrue(synchronized(requests) { requests.none { it.startsWith("PUT") } })
    }

    // The reminder to close the emulator comes once per session, before the first action that may write a save on
    // this phone; "Don't show again" is saved.
    @Test
    fun reminderBeforeTheFirstWrite() {
        start(url = "http://127.0.0.1:1", token = "t", emulators = listOf(emu), entries = games(), noReminder = false)
        rule.onNodeWithText(s(R.string.sync_all)).performClick()
        rule.onNodeWithText(s(R.string.reminder_text)).assertExists()
        fits(s(R.string.dont_show_again))
        rule.onNode(hasText(s(R.string.ok)) and dialog).performClick()
        rule.onNode(hasText("Mario Kart DS", substring = true) and dialog).assertExists() // then the usual confirmation
        rule.onNode(hasText(s(R.string.cancel)) and dialog).performClick()

        open("Mario Kart DS")
        rule.onNodeWithText(s(R.string.download)).performScrollTo().performClick()
        rule.onNodeWithText(s(R.string.confirm_download)).assertExists() // shown once: not again for this download
        rule.onNode(hasText(s(R.string.cancel)) and dialog).performClick()
        back()
        assertFalse(Settings(ctx).noReminder)

        App.reminded = false // the app starts again
        rule.onNodeWithText(s(R.string.sync_all)).performClick()
        rule.onNodeWithText(s(R.string.dont_show_again)).performClick()
        rule.onNode(hasText(s(R.string.ok)) and dialog).performClick()
        rule.onNode(hasText(s(R.string.cancel)) and dialog).performClick()
        assertTrue(Settings(ctx).noReminder)

        App.reminded = false // and again: "Don't show again" holds
        rule.onNodeWithText(s(R.string.sync_all)).performClick()
        rule.onAllNodesWithText(s(R.string.reminder_text)).assertCountEquals(0)
        rule.onNode(hasText("Mario Kart DS", substring = true) and dialog).assertExists()
        rule.onNode(hasText(s(R.string.cancel)) and dialog).performClick()
        assertTrue(files.values.all { it.writes == 0 })
    }
}

@Config(qualifiers = "fr-w320dp-h640dp")
class UiFrSmallTest : UiTest() {
    @Test
    fun french() = assertEquals("Tout synchroniser", ctx.getString(R.string.sync_all))
}
