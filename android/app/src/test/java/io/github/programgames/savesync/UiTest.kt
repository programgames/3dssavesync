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
import androidx.compose.ui.test.assertIsOff
import androidx.compose.ui.test.assertIsOn
import androidx.work.Configuration
import androidx.work.WorkInfo
import androidx.work.WorkManager
import androidx.work.testing.SynchronousExecutor
import androidx.work.testing.WorkManagerTestInitHelper
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasClickAction
import androidx.compose.ui.test.hasSetTextAction
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.isDialog
import androidx.compose.ui.test.isSelectable
import androidx.compose.ui.test.isToggleable
import androidx.compose.ui.test.junit4.createEmptyComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onAllNodesWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.hasScrollToNodeAction
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performCustomAccessibilityActionWithLabel
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performScrollToIndex
import androidx.compose.ui.test.performScrollToNode
import androidx.compose.ui.test.performTextClearance
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.test.assertTextContains
import androidx.test.core.app.ActivityScenario
import androidx.test.core.app.ApplicationProvider
import android.app.Activity
import android.content.Intent
import android.content.pm.PackageInfo
import android.net.Uri
import android.os.Environment
import android.provider.DocumentsContract
import io.github.programgames.savesync.core.Bases
import io.github.programgames.savesync.core.Format
import io.github.programgames.savesync.core.Game
import io.github.programgames.savesync.core.Local
import io.github.programgames.savesync.core.NetworkException
import io.github.programgames.savesync.core.Pending
import io.github.programgames.savesync.core.PendingWrite
import io.github.programgames.savesync.core.Problem
import io.github.programgames.savesync.core.RESTORE_CLIENT
import io.github.programgames.savesync.core.SaveFile
import io.github.programgames.savesync.core.State
import io.github.programgames.savesync.core.Status
import io.github.programgames.savesync.core.Summary
import io.github.programgames.savesync.core.SyncException
import io.github.programgames.savesync.core.System
import io.github.programgames.savesync.core.Version
import io.github.programgames.savesync.core.mainSunMoon
import io.github.programgames.savesync.core.newAlias
import io.github.programgames.savesync.core.sha256
import org.junit.After
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Assume
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.Shadows.shadowOf
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import java.io.File
import java.io.IOException
import java.net.InetAddress
import java.net.ServerSocket
import java.net.URLEncoder
import kotlin.concurrent.thread

const val KART = "Mario Kart DS (Europe) (En,Fr,De,Es,It)" // a No-Intro name: the list shows "Mario Kart DS"

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
        aliases: Map<String, String> = emptyMap(),
        setup: Boolean = false, // the guided first run shows (no server, never finished)
    ): ActivityScenario<MainActivity> {
        App.init(ctx)
        App.settings.setupDone = !setup
        App.settings.aliases = aliases
        App.aliases = aliases
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

    // fakeServer runs on this computer only: by default GET /saves is empty and any other GET returns save. Every
    // request is recorded: its first line, then its headers.
    private fun fakeServer(save: ByteArray, respond: (String) -> Pair<Int, ByteArray> = { if (it.split(" ")[1] == "/saves") 200 to "[]".toByteArray() else 200 to save }): String {
        val ss = ServerSocket(0, 50, InetAddress.getByName("127.0.0.1"))
        thread(isDaemon = true) {
            while (true) {
                val c = runCatching { ss.accept() }.getOrNull() ?: break
                c.use {
                    val r = it.getInputStream().bufferedReader(Charsets.ISO_8859_1) // one char per byte: the body is skipped exactly
                    val line = r.readLine() ?: return@use
                    val headers = generateSequence { r.readLine()?.takeIf { h -> h.isNotEmpty() } }.toList()
                    val length = headers.find { h -> h.startsWith("Content-Length:", ignoreCase = true) }?.substringAfter(':')?.trim()?.toInt() ?: 0
                    var left = length.toLong() // the PUT body, read before the response
                    while (left > 0) left -= r.skip(left).takeIf { n -> n > 0 } ?: break
                    synchronized(requests) { requests += (listOf(line) + headers).joinToString("\n") }
                    val (st, body) = respond(line)
                    it.getOutputStream().write("HTTP/1.1 $st X\r\nContent-Length: ${body.size}\r\nConnection: close\r\n\r\n".toByteArray() + body)
                }
            }
        }
        server = ss
        return "http://127.0.0.1:${ss.localPort}"
    }

    private fun waitFor(text: String, substring: Boolean = false) = try {
        rule.waitUntil(15_000) { rule.onAllNodesWithText(text, substring).fetchSemanticsNodes().isNotEmpty() }
    } catch (e: Throwable) {
        throw AssertionError("no \"$text\" in:\n" + rule.onAllNodes(SemanticsMatcher("all") { true }).fetchSemanticsNodes().mapNotNull { n -> n.config.getOrElseNullable(SemanticsProperties.Text) { null }?.joinToString() }.joinToString("\n"), e)
    }

    // fits: every Text with this text is inside the screen width and shows all of it (not cut, at most maxLines lines).
    private fun fits(text: String, lines: Int = 1) {
        val maxLines = if (large) lines * 2 else lines // 200 %: twice the lines, never cut
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

    private val large get() = ctx.resources.configuration.fontScale >= 1.5f

    // onlyNormalFont: a test of the logic, not of the layout; at 200 % its rows would need scrolling at every step.
    private fun onlyNormalFont() = Assume.assumeFalse(large)

    // seen: the game row exists, once the list has scrolled to it (a lazy list composes only what fits)
    private fun seen(name: String) {
        rule.onNode(hasScrollToNodeAction()).performScrollToNode(hasText(name))
        rule.onNodeWithText(name).assertExists()
    }

    @Test
    fun firstStartShowsTheSettingsAndAClearConnectionError() {
        val sc = start(setup = true)
        rule.onNodeWithText(s(R.string.welcome_title)).assertIsDisplayed()
        rule.onNodeWithText(s(R.string.later)).performClick() // put off: the list says what is missing
        assertTrue(App.settings.setupDone)
        rule.onNodeWithText(s(R.string.need_server)).assertIsDisplayed()
        rule.onNodeWithText(s(R.string.open_settings)).performClick()
        rule.onNodeWithText(s(R.string.settings_title)).assertIsDisplayed()
        rule.onNodeWithText(s(R.string.test_connection)).assertIsNotEnabled()
        val fields = rule.onAllNodes(hasSetTextAction())
        fields.assertCountEquals(2)
        fields[0].performTextInput("127.0.0.1:1") // no scheme: http:// is added; nothing listens on port 1, refused at once
        fields[1].performTextInput("secret-token")
        // the token is masked until Show
        rule.onNode(SemanticsMatcher.keyIsDefined(SemanticsProperties.Password)).assertExists()
        val shown = { fields[1].fetchSemanticsNode().config[SemanticsProperties.EditableText].text }
        assertFalse(shown().contains("secret"))
        rule.onNodeWithText(s(R.string.show)).performClick()
        assertEquals("secret-token", shown())
        rule.onNodeWithText(s(R.string.test_connection)).performScrollTo().assertIsEnabled().performClick()
        waitFor(s(R.string.p_refused))
        rule.onAllNodesWithText("ConnectException", substring = true).assertCountEquals(0) // the technical line: on demand
        rule.onNodeWithText(s(R.string.show_details)).performScrollTo().performClick()
        rule.onNodeWithText("ConnectException", substring = true).assertExists()
        rule.onNodeWithText(s(R.string.retry)).assertExists()

        sc.recreate() // rotation: the settings stay
        rule.onNodeWithText("127.0.0.1:1").assertExists()
        rule.onNodeWithText(s(R.string.test_connection)).assertIsEnabled()
    }

    // The guided first run: welcome, server by QR code, the installed emulator first, its folders, done. Back from the
    // folders forgets an entry without folders; every step can be skipped; Settings starts it again.
    @Test
    fun guidedFirstRun() {
        val real = scanQr
        try {
            val url = fakeServer(ByteArray(0))
            scanQr = { _, done -> done(Result.success("3dssavesync://serveur?adresse=" + URLEncoder.encode(url, "UTF-8") + "&jeton=fake-token")) }
            shadowOf(ctx.packageManager).installPackage(PackageInfo().apply { packageName = "me.magnum.melonds" })
            val sc = start(setup = true)
            fits(s(R.string.welcome_title))
            fits(s(R.string.setup_start))
            rule.onNodeWithText(s(R.string.setup_start)).performClick()

            rule.onNodeWithText(s(R.string.setup_step, 1, 3)).assertExists()
            rule.onNodeWithText(s(R.string.next)).assertIsNotEnabled()
            rule.onAllNodes(hasSetTextAction()).assertCountEquals(0) // typing is the other way, behind a button
            fits(s(R.string.type_address))
            rule.onNodeWithText(s(R.string.scan_qr)).performClick()
            waitFor(ctx.resources.getQuantityString(R.plurals.connected, 0, 0))
            rule.onNodeWithText(s(R.string.next)).performScrollTo().assertIsEnabled().performClick()

            rule.onNodeWithText(s(R.string.setup_step, 2, 3)).assertExists()
            rule.onNodeWithText(s(R.string.installed_emulators)).assertExists()
            rule.onNode(hasText("melonDS") and hasClickAction()).performClick()
            rule.onNodeWithText(s(R.string.setup_step, 3, 3)).assertExists()
            rule.onNodeWithText(s(R.string.hint_melonds)).assertExists()
            assertEquals(listOf("melonds"), App.emulators.map { it.id })
            sc.recreate() // rotation: still this step
            rule.onNodeWithText(s(R.string.hint_melonds)).assertExists()
            sc.onActivity { it.onBackPressedDispatcher.onBackPressed() } // a wrong tap: back to the choice, the entry without folders is gone
            rule.onNodeWithText(s(R.string.setup_step, 2, 3)).assertExists()
            assertEquals(emptyList<Emulator>(), App.emulators)
            rule.onNode(hasText("melonDS") and hasClickAction()).performClick()
            rule.onNodeWithText(s(R.string.later)).performScrollTo().performClick() // no folder yet: later
            rule.onNodeWithText(s(R.string.setup_done_title)).assertExists()
            rule.onNodeWithText(s(R.string.setup_done_text)).assertExists()
            rule.onNodeWithText(s(R.string.show_games)).performClick()
            assertTrue(App.settings.setupDone)
            rule.onNode(hasText(s(R.string.tab_games)) and !hasClickAction()).assertIsDisplayed()

            rule.onNodeWithText(s(R.string.tab_settings)).performClick()
            rule.onNodeWithText(s(R.string.setup_again)).performScrollTo().performClick()
            rule.onNodeWithText(s(R.string.welcome_title)).assertIsDisplayed()
            rule.onNodeWithText(s(R.string.setup_start)).performClick()
            rule.onNodeWithText(s(R.string.skip)).performClick()
            rule.onNodeWithText(s(R.string.skip)).performScrollTo().performClick()
            rule.onNodeWithText(s(R.string.setup_done_title)).assertExists()
        } finally {
            scanQr = real
        }
    }

    // "Scan the PC's QR code": the scanner can't run here, scanQr is replaced. A good code fills, saves and tests; a
    // cancel, a missing scanner or another code fills nothing and says why.
    @Test
    fun scanTheQrCodeFillsTheSettings() {
        val real = scanQr
        var answer: Result<String?> = Result.success(null)
        scanQr = { _, done -> done(answer) }
        try {
            val url = fakeServer(ByteArray(0))
            start()
            val scan = rule.onNodeWithText(s(R.string.scan_qr))
            scan.performClick()
            rule.onNodeWithText(s(R.string.scan_cancelled)).assertExists()
            answer = Result.failure(IllegalStateException("no Google Play services"))
            scan.performClick()
            rule.onNodeWithText(s(R.string.scan_unavailable)).assertExists()
            answer = Result.success("https://saves.example.com")
            scan.performClick()
            rule.onNodeWithText(s(R.string.scan_not_ours)).assertExists()
            assertEquals("", App.settings.url)
            answer = Result.success("3dssavesync://serveur?adresse=" + URLEncoder.encode(url, "UTF-8") + "&jeton=fake-token")
            scan.performClick()
            waitFor(ctx.resources.getQuantityString(R.plurals.connected, 0, 0))
            assertEquals(url, App.settings.url)
            assertEquals("fake-token", App.settings.token)
            rule.onNodeWithText(url).assertExists()
            rule.onNodeWithText(s(R.string.scan_help)).assertExists()
            assertTrue(synchronized(requests) { requests.any { it.startsWith("GET /saves") && it.contains("Bearer fake-token") } })
        } finally {
            scanQr = real
        }
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
            if (pr.systems.size > 1) for (sys in pr.systems.keys) fits(sys.label) // the system boxes
            for (sys in pr.systems.keys) rule.onNodeWithText(systemNameText(sys)).assertExists()
            sc.recreate() // rotation: still on this emulator
            rule.onNodeWithText(s(pr.hint)).assertExists()
            rule.onNodeWithContentDescription(s(R.string.back)).performClick()
        }
        rule.onNode(hasText(s(R.string.emulators_title)) and !hasClickAction()).assertIsDisplayed() // the title, not the tab
        assertEquals(profiles.size, App.emulators.size)
        rule.waitUntil(15_000) { rule.onAllNodesWithText(s(R.string.folder_not_set)).fetchSemanticsNodes().size == profiles.size }
    }

    // The emulators installed on this phone come first. The folder picker opens where the emulator keeps its saves
    // (RetroArch/saves) when that folder exists, else where the picker wants; the systems not played can be unticked,
    // never the last one.
    @Test
    fun installedFirstAndPickerStartsAtTheUsualFolder() {
        shadowOf(ctx.packageManager).installPackage(PackageInfo().apply { packageName = "com.retroarch.aarch64" })
        File(Environment.getExternalStorageDirectory(), "RetroArch/saves").mkdirs()
        val sc = start(url = "http://127.0.0.1:1", token = "t")
        rule.onNodeWithText(s(R.string.tab_emulators)).performClick()
        rule.onNodeWithText(s(R.string.add_emulator)).performClick()
        val installed = rule.onNode(hasText(s(R.string.installed_emulators)) and dialog).fetchSemanticsNode().positionInRoot.y
        val others = rule.onNode(hasText(s(R.string.other_emulators)) and dialog).fetchSemanticsNode().positionInRoot.y
        val retroarch = rule.onNode(hasText("RetroArch") and hasClickAction() and dialog).fetchSemanticsNode().positionInRoot.y
        val melon = rule.onNode(hasText("melonDS") and hasClickAction() and dialog).fetchSemanticsNode().positionInRoot.y
        assertTrue(installed < retroarch && retroarch < others && others < melon)
        rule.onNode(hasText("RetroArch") and hasClickAction() and dialog).performClick()

        fun picked(): Intent = sc.run {
            var i: Intent? = null
            onActivity { i = shadowOf(it).nextStartedActivityForResult.intent }
            i!!
        }
        rule.onAllNodes(hasText(s(R.string.choose)) and hasClickAction())[0].performScrollTo().performClick() // the saves folder
        val saves = picked()
        assertEquals(Intent.ACTION_OPEN_DOCUMENT_TREE, saves.action)
        assertEquals(DocumentsContract.buildDocumentUri(EXTERNAL_STORAGE, "primary:RetroArch/saves"), saves.getParcelableExtra<Uri>(DocumentsContract.EXTRA_INITIAL_URI))
        rule.onAllNodes(hasText(s(R.string.choose)) and hasClickAction())[1].performScrollTo().performClick() // no roms folder
        assertEquals(null, picked().getParcelableExtra<Uri>(DocumentsContract.EXTRA_INITIAL_URI))

        // a GBA-only player unticks DS and GB; GBA, the last one, stays
        for (sys in listOf("DS", "GB")) rule.onNode(hasText(sys) and isToggleable()).performScrollTo().performClick()
        assertEquals(setOf(System.GBA), App.emulators.single().systems.keys)
        rule.onNode(hasText("GBA") and isToggleable()).performScrollTo().assertIsNotEnabled()
        rule.onAllNodesWithText(s(R.string.system_ds)).assertCountEquals(0)
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
        entry(KART, System.DS, "AMCE", local(sram), other, sha256(sram), State.TO_DOWNLOAD), // the list shows "Mario Kart DS"
        entry("Pokemon Soleil", System.N3DS, "3DS-0004000000164800", local(sun), other.copy(size = sun.size.toLong()), "c".repeat(64), State.CONFLICT),
        entry("Metroid", System.GBA, "GBA-AMTE", null, null, null, null),
        entry("Wario", System.GBA, "GBA-RWZE", null, null, null, null, error = IOException("read error")),
        Entry(emu, System.GB, "Tetris", "Tetris.gb", null, dups = listOf("Tetris.gb", "Tetris (1).gb")),
    )

    private fun chip(label: String): androidx.compose.ui.test.SemanticsNodeInteraction {
        // the chips of the games list are at the top of the list
        if (rule.onAllNodes(hasScrollToNodeAction()).fetchSemanticsNodes().isNotEmpty()) rule.onNode(hasScrollToNodeAction()).performScrollToIndex(0)
        return rule.onNode(hasText(label) and isSelectable()).performScrollTo()
    }

    private val azahar = Emulator("azahar", "azahar", "", mapOf(System.N3DS to Folders()))

    @Test
    fun gamesListFiltersSearchAndSummary() {
        start(url = "http://127.0.0.1:1", token = "t", emulators = listOf(emu, azahar), entries = games())
        for (n in listOf("Pokémon Émeraude", "Zelda", "Mario Kart DS", "Pokemon Soleil", "Wario", "Tetris")) {
            rule.onNode(hasScrollToNodeAction()).performScrollToNode(hasText(n)) // the list composes only what fits
        }
        rule.onAllNodesWithText(KART).assertCountEquals(0) // the short name only, as on the 3DS
        rule.onNode(hasScrollToNodeAction()).performScrollToNode(hasText(ctx.resources.getQuantityString(R.plurals.hidden_games, 1, 1)))
        rule.onAllNodesWithText("Metroid").assertCountEquals(0)
        rule.onNode(hasScrollToNodeAction()).performScrollToIndex(0)
        for (t in listOf(R.string.sync_all, R.string.tab_games)) fits(s(t))
        rule.onNodeWithText(s(R.string.sync_all)).assertIsEnabled()
        // each row is one TalkBack stop: its texts, its State as the state description, a click
        for (t in listOf(R.string.s_to_upload, R.string.s_in_sync, R.string.s_to_download, R.string.s_conflict)) {
            rule.onNode(hasScrollToNodeAction()).performScrollToNode(state(s(t)))
            rule.onNode(state(s(t)) and hasClickAction()).assertExists()
        }
        rule.onNode(state(s(R.string.s_to_download))).assertTextContains("Mario Kart DS").assertTextContains("DS")
        rule.onAllNodesWithText(s(R.string.s_to_download)).assertCountEquals(0) // said once, as the state
        rule.onNode(hasScrollToNodeAction()).performScrollToIndex(0)
        // the system chips: only those of the emulators (GBA, 3DS), not DS or GB
        chip("GBA")
        for (sys in listOf("DS", "GB")) rule.onAllNodes(hasText(sys) and isSelectable()).assertCountEquals(0)

        chip(s(R.string.filter_todo)).performClick()
        for (n in listOf("Pokémon Émeraude", "Mario Kart DS", "Pokemon Soleil")) seen(n)
        for (n in listOf("Zelda", "Wario", "Tetris")) rule.onAllNodesWithText(n).assertCountEquals(0)
        chip("3DS").performClick()
        seen("Pokemon Soleil")
        rule.onAllNodesWithText("Mario Kart DS").assertCountEquals(0)
        chip(s(R.string.filter_all)).performClick()

        fun search(text: String) { // the field is at the top of the list
            rule.onNode(hasScrollToNodeAction()).performScrollToIndex(0)
            rule.onNode(hasSetTextAction()).performTextClearance()
            if (text.isNotEmpty()) rule.onNode(hasSetTextAction()).performTextInput(text)
        }
        search("pokemon") // accent-insensitive
        seen("Pokémon Émeraude")
        seen("Pokemon Soleil")
        rule.onAllNodesWithText("Zelda").assertCountEquals(0)
        search("ÉMERAUDE")
        seen("Pokémon Émeraude")
        search("no such game")
        seen(s(R.string.no_match))
        search("")

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
        rule.onAllNodesWithText("write detail").assertCountEquals(0)
        rule.onNode(hasText(s(R.string.show_details)) and dialog).performScrollTo().performClick()
        rule.onNodeWithText("write detail").assertExists()
        rule.onNode(hasText(s(R.string.close)) and dialog).performClick()
        rule.onAllNodesWithText(s(R.string.summary_title)).assertCountEquals(0)
    }

    @Test
    fun gamesEmptyStates() {
        start(url = "http://127.0.0.1:1", token = "t", emulators = listOf(emu), entries = emptyList())
        rule.onNodeWithText(s(R.string.no_game)).assertExists()
        fits(s(R.string.open_emulators))
        rule.onNodeWithText(s(R.string.open_emulators)).performClick()
        rule.onNode(hasText(s(R.string.emulators_title)) and !hasClickAction()).assertIsDisplayed()
        rule.onNodeWithText(s(R.string.tab_games)).performClick()
        App.settings.token = ""
        scenario!!.recreate()
        rule.onNodeWithText(s(R.string.need_server)).assertExists() // the tab stays Games: the notice shows
    }

    // Sync all says why it can't run. A server not read: one banner with Retry, the games without a save on this
    // phone hidden (a server not read can't say they have none); Retry, or the refresh action of the list, reads again.
    @OptIn(androidx.compose.ui.test.ExperimentalTestApi::class)
    @Test
    fun syncAllReasonsAndServerNotRead() {
        val zelda = games()[1]
        start(url = "http://127.0.0.1:1", token = "t", emulators = listOf(emu), entries = listOf(zelda))
        rule.onNodeWithText(s(R.string.sync_off_nothing)).assertIsDisplayed()
        rule.onNodeWithText(s(R.string.sync_all)).assertIsNotEnabled()

        val metroid = games()[4].copy(noLocal = true)
        App.entries = listOf(zelda.copy(status = null), metroid)
        App.serverError = NetworkException(java.net.ConnectException("Connection refused"))
        rule.onNodeWithText(s(R.string.sync_off_server)).assertIsDisplayed()
        rule.onNodeWithText(s(R.string.p_refused)).assertIsDisplayed()
        rule.onNodeWithText(s(R.string.hidden_offline)).assertIsDisplayed()
        rule.onAllNodesWithText(s(R.string.p_refused)).assertCountEquals(1) // said once
        fits(s(R.string.retry))
        fits(s(R.string.show_details))
        seen("Zelda") // its save is on this phone: Unknown
        rule.onAllNodesWithText("Metroid").assertCountEquals(0)
        rule.onAllNodesWithText(ctx.resources.getQuantityString(R.plurals.hidden_games, 1, 1)).assertCountEquals(0)
        open("Zelda") // the details don't repeat the banner
        rule.onNodeWithText(s(R.string.why_unknown)).assertExists()
        rule.onAllNodesWithText(s(R.string.p_refused)).assertCountEquals(0)
        back()

        // Retry reads again: nothing listens on port 1, and no ROM folder is set, so no game, with the way to fix it
        rule.onNode(hasScrollToNodeAction()).performScrollToIndex(0)
        rule.onNodeWithText(s(R.string.retry)).performScrollTo().performClick()
        waitFor(s(R.string.open_emulators))
        // pull to refresh, as an accessibility action
        val refresh = rule.onNode(SemanticsMatcher.keyIsDefined(SemanticsActions.CustomActions)).fetchSemanticsNode().config[SemanticsActions.CustomActions]
        assertEquals(listOf(s(R.string.refresh)), refresh.map { it.label })
        App.serverError = null
        rule.onNode(SemanticsMatcher.keyIsDefined(SemanticsActions.CustomActions)).performCustomAccessibilityActionWithLabel(s(R.string.refresh))
        waitFor(s(R.string.p_refused))
    }

    private fun state(word: String) = SemanticsMatcher.expectValue(SemanticsProperties.StateDescription, word)

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
        fits(s(R.string.s_to_upload))
        rule.onNodeWithText(s(R.string.why_to_upload)).assertExists()
        rule.onNodeWithText(s(R.string.no_server_version)).assertExists()
        fits(s(R.string.upload))
        back()

        open("Zelda")
        fits(s(R.string.s_in_sync))
        rule.onNodeWithText(s(R.string.why_in_sync)).assertExists()
        rule.onAllNodesWithText(s(R.string.upload)).assertCountEquals(0)
        rule.onAllNodesWithText(s(R.string.download)).assertCountEquals(0)
        back()

        open("Mario Kart DS")
        fits("Mario Kart DS", 2) // the title: the short name, whole
        fits(s(R.string.s_to_download))
        rule.onNodeWithText(s(R.string.file_row, "$KART.rom")).assertExists() // the full file name, once
        rule.onNodeWithText(s(R.string.why_to_download)).assertExists()
        rule.onNodeWithText(s(R.string.uploaded_by, "PC")).assertExists()
        rule.onNodeWithText(s(R.string.download)).performScrollTo()
        fits(s(R.string.download))
        rule.onNodeWithText(s(R.string.download)).performClick()
        rule.onNodeWithText(s(R.string.confirm_download)).assertExists()
        rule.onNode(hasText(s(R.string.cancel)) and dialog).performClick()
        back()

        open("Pokemon Soleil")
        fits(s(R.string.s_conflict))
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
        rule.onNodeWithText(s(R.string.show_details)).performScrollTo().performClick()
        rule.onNodeWithText("read error", substring = true).assertExists()
        back()

        // no tap on Confirm: no save written, no upload
        assertEquals(null, App.busy)
        assertTrue(files.values.all { it.writes == 0 })
        assertTrue(synchronized(requests) { requests.none { it.startsWith("PUT") } })
    }

    // History and Restore (ADR 0016): newest first, the play time of the selected Version only, a confirmation with both
    // play times. A 412 reads the list again; a Restore changes only the server, then the game shows To download.
    @Test
    fun historyAndRestore() {
        onlyNormalFont()
        val key = "3DS-0004000000164800"
        val old = mainSunMoon(3, 4, "Red")
        val v1 = "000001-20261101T080000Z-3ds-${sha256(old)}.sav"
        val v2 = "000002-20261103T101500Z-android-${sha256(sun)}.sav"
        var current = sha256(sun)
        var puts = 0
        fun json(file: String, client: String, age: Int) =
            """{"code":"$key","version":"$file","hash":"${file.takeLast(68).take(64)}","taille":${sun.size},"date":"${file.substring(7, 23)}","client":"$client","age":$age}"""
        val url = fakeServer(sun) { line ->
            val path = line.split(" ")[1]
            when {
                line.startsWith("PUT") -> if (++puts == 1) 412 to ByteArray(0) else 200 to ByteArray(0).also { current = sha256(old) }
                path == "/saves" -> 200 to "[${json(if (current == sha256(old)) v1 else v2, "android", 60)}]".toByteArray()
                path == "/saves/$key/history" -> 200 to "[${json(v1, "3ds", 2 * 86400)},${json(v2, "android", 3600)}]".toByteArray()
                path == "/saves/$key/history/$v1" -> 200 to old
                path == "/saves/$key/history/$v2" -> 200 to sun
                else -> 404 to ByteArray(0)
            }
        }
        val game = entry("Pokemon Soleil", System.N3DS, key, local(sun), Version(sha256(sun), sun.size.toLong(), client = "android"), sha256(sun), State.IN_SYNC)
        Bases(File(ctx.filesDir, "bases")).set(emu.id, key, sha256(sun)) // last Sync: this version
        start(url = url, token = "t", emulators = listOf(emu), entries = listOf(game))
        open("Pokemon Soleil")
        rule.onNodeWithText(s(R.string.show_history)).performScrollTo()
        fits(s(R.string.show_history))
        rule.onNodeWithText(s(R.string.show_history)).performClick()

        val row1 = s(R.string.version_no, 1) + " · " + s(R.string.age_d, 2)
        val row2 = s(R.string.version_no, 2) + " · " + s(R.string.age_h, 1)
        waitFor(row2)
        val kib = s(R.string.size_kib, (sun.size + 512) / 1024)
        rule.onNodeWithText("Android · $kib", useUnmergedTree = true).assertExists()
        rule.onNodeWithText("3DS · $kib", useUnmergedTree = true).assertExists()
        fits(s(R.string.current))
        assertTrue(rule.onNodeWithText(row2).fetchSemanticsNode().positionInRoot.y < rule.onNodeWithText(row1).fetchSemanticsNode().positionInRoot.y)
        assertTrue(synchronized(requests) { requests.none { "/history/" in it } }) // no version read before a tap

        rule.onNodeWithText(row2).performClick()
        rule.onNodeWithText(s(R.string.version_current_help)).assertExists()
        rule.onAllNodesWithText(s(R.string.restore)).assertCountEquals(0)
        rule.onNodeWithText(row1).performClick()
        waitFor(s(R.string.play_info, 3, 4, "Red")) // read for the selected version only
        rule.onNodeWithText(s(R.string.version_restore_help)).assertExists()
        scenario!!.recreate() // rotation: still the History, same version selected
        waitFor(s(R.string.play_info, 3, 4, "Red"))
        rule.onNode(hasScrollToNodeAction()).performScrollToNode(hasText(s(R.string.restore)))
        fits(s(R.string.restore))

        // the confirmation: this version and the phone save, with both play times; Cancel changes nothing
        rule.onNodeWithText(s(R.string.restore)).performClick()
        rule.onNodeWithText(s(R.string.restore_title)).assertExists()
        rule.onNode(hasText("$row1 · 3DS") and dialog).assertExists()
        rule.onNode(hasText(s(R.string.play_info, 3, 4, "Red")) and dialog).assertExists()
        rule.onNode(hasText(s(R.string.play_info, 12, 5, "Ash")) and dialog).assertExists()
        rule.onNode(hasText(s(R.string.restore_explain)) and dialog).performScrollTo().assertExists()
        rule.onNode(hasText(s(R.string.cancel)) and dialog).performClick()
        assertTrue(synchronized(requests) { requests.none { it.startsWith("PUT") } })

        // 412: a device uploaded meanwhile; nothing restored, the list is read again
        fun lists() = synchronized(requests) { requests.count { it.startsWith("GET /saves/$key/history ") } }
        val read = lists()
        rule.onNodeWithText(s(R.string.restore)).performClick()
        rule.onNode(hasText(s(R.string.restore)) and dialog).performClick()
        waitFor(s(R.string.p_server_changed))
        rule.onNodeWithText(s(R.string.restore_failed)).assertExists()
        rule.onNodeWithText(s(R.string.show_details)).performClick()
        rule.onNodeWithText("HTTP 412", substring = true).assertExists()
        rule.waitUntil(15_000) { lists() == read + 1 }
        waitFor(row1)
        rule.onAllNodesWithText(s(R.string.version_restore_help)).assertCountEquals(0)

        // Restore: only the server changes; the details show To download, with the usual Download
        rule.onNodeWithText(row1).performClick()
        rule.onNode(hasScrollToNodeAction()).performScrollToNode(hasText(s(R.string.restore)))
        rule.onNodeWithText(s(R.string.restore)).performClick()
        rule.onNode(hasText(s(R.string.restore)) and dialog).performClick()
        waitFor(s(R.string.restored, 1))
        rule.onNodeWithText(s(R.string.s_to_download)).assertExists()
        rule.onNodeWithText(s(R.string.download)).performScrollTo().assertExists()
        val put = synchronized(requests) { requests.filter { it.startsWith("PUT /saves/$key ") } }
        assertEquals(2, put.size)
        assertTrue(put.joinToString(), put.all { "X-Client: $RESTORE_CLIENT" in it && "If-Match: \"${sha256(sun)}\"" in it })
        assertTrue(files.values.all { it.writes == 0 })
    }

    // The reminder to close the emulator comes once per session, before the first action that may write a save on
    // this phone; "Don't show again" is saved.
    @Test
    fun reminderBeforeTheFirstWrite() {
        start(url = "http://127.0.0.1:1", token = "t", emulators = listOf(emu), entries = games(), noReminder = false)
        rule.onNodeWithText(s(R.string.sync_all)).performClick()
        rule.onNodeWithText(s(R.string.reminder_text)).assertExists()
        fits(s(R.string.dont_show_again))
        rule.onNode(hasText(s(R.string.emulator_closed)) and dialog).performClick()
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
        rule.onNodeWithText(s(R.string.dont_show_again)).performScrollTo().performClick() // 200 %: below the text
        rule.onNode(hasText(s(R.string.emulator_closed)) and dialog).performClick()
        rule.onNode(hasText(s(R.string.cancel)) and dialog).performClick()
        assertTrue(Settings(ctx).noReminder)

        App.reminded = false // and again: "Don't show again" holds
        rule.onNodeWithText(s(R.string.sync_all)).performClick()
        rule.onAllNodesWithText(s(R.string.reminder_text)).assertCountEquals(0)
        rule.onNode(hasText("Mario Kart DS", substring = true) and dialog).assertExists()
        rule.onNode(hasText(s(R.string.cancel)) and dialog).performClick()
        assertTrue(files.values.all { it.writes == 0 })
    }

    // ROM hacks (ADR 0014): the list, an addition by ROM file (console from the extension, Key of the PC), and a
    // removal that only forgets the mapping, after a confirmation.
    @Test
    fun romHacks() {
        val sc = start(url = "http://127.0.0.1:1", token = "t", aliases = mapOf("Radical Red (4.10)" to "GBA-0EE3"))
        rule.onNodeWithText(s(R.string.tab_settings)).performClick()
        rule.onNode(hasText(s(R.string.open_hacks)) and hasClickAction()).performScrollTo().performClick()
        rule.onNodeWithText(s(R.string.hacks_same_key)).assertExists()
        rule.onNodeWithText("Radical Red (4.10)").assertExists()
        rule.onNodeWithText(s(R.string.system_gba)).assertExists()
        rule.onNodeWithText(s(R.string.hack_add)).assertIsNotEnabled() // no ROM chosen yet
        fits(s(R.string.hack_choose_rom))
        for (sys in listOf("DS", "GB", "GBA")) fits(sys)

        rule.onNodeWithText(s(R.string.hack_choose_rom)).performScrollTo().performClick()
        sc.onActivity { a ->
            val sh = shadowOf(a)
            val asked = sh.nextStartedActivityForResult
            assertEquals(Intent.ACTION_OPEN_DOCUMENT, asked.intent.action)
            sh.receiveResult(asked.intent, Activity.RESULT_OK, Intent().setData(Uri.parse("file:///roms/Hack.nds")))
        }
        rule.onNodeWithText("Hack.nds").assertExists()
        rule.onNodeWithText(s(R.string.hack_add)).performScrollTo().assertIsEnabled().performClick()
        assertEquals(mapOf("Radical Red (4.10)" to "GBA-0EE3", "Hack" to "0KP0"), App.settings.aliases)
        rule.onNodeWithText("Hack").assertExists()
        rule.onNodeWithText(s(R.string.hack_no_file)).assertExists()

        sc.recreate() // rotation: still on the ROM hacks
        rule.onNodeWithText("Radical Red (4.10)").assertExists()
        rule.onAllNodes(hasText(s(R.string.remove)) and hasClickAction())[1].performClick() // sorted: Hack, then Radical Red
        rule.onNodeWithText(s(R.string.hack_remove_title)).assertExists()
        rule.onNodeWithText(s(R.string.hack_remove_text, "Radical Red (4.10)")).assertExists()
        rule.onNodeWithContentDescription(s(R.string.action_on, s(R.string.remove), "Hack")).assertExists() // TalkBack: which one
        rule.onNode(hasText(s(R.string.cancel)) and dialog).performClick()
        assertEquals(2, App.settings.aliases.size)
        rule.onAllNodes(hasText(s(R.string.remove)) and hasClickAction())[1].performClick()
        rule.onNode(hasText(s(R.string.remove)) and dialog).performClick()
        assertEquals(mapOf("Hack" to "0KP0"), App.settings.aliases)
        rule.onAllNodesWithText("Radical Red (4.10)").assertCountEquals(0)
        back()
        rule.onNodeWithText(s(R.string.settings_title)).assertExists()
    }

    // Background upload: off by default; on, the periodic work is enqueued; off again, cancelled. The last run shows.
    @Test
    fun backgroundUploadSetting() {
        WorkManagerTestInitHelper.initializeTestWorkManager(ctx, Configuration.Builder().setExecutor(SynchronousExecutor()).build())
        start()
        val work = { WorkManager.getInstance(ctx).getWorkInfosForUniqueWork(UPLOAD_WORK).get() }
        rule.onNodeWithText(s(R.string.bg_never)).performScrollTo().assertIsDisplayed()
        fits(s(R.string.bg_title))
        fits(s(R.string.bg_switch), 2)
        val row = rule.onNodeWithText(s(R.string.bg_switch))
        row.performScrollTo().assertIsOff().performClick().assertIsOn()
        assertTrue(App.settings.background)
        assertEquals(WorkInfo.State.ENQUEUED, work().single().state)
        row.performClick().assertIsOff()
        assertFalse(App.settings.background)
        assertEquals(WorkInfo.State.CANCELLED, work().single().state)

        val problem = s(R.string.bg_conflict, "Pokemon Emeraude")
        App.lastRun = LastRun(1_800_000_000_000L, 2, problem)
        val line = ctx.resources.getQuantityString(R.plurals.bg_last, 2, dateText(1_800_000_000_000L), 2)
        waitFor(line)
        fits(line, 2)
        fits(problem, 3)
    }

    // A change of extension, core or "next to the ROMs" forgets the Bases of the entry, with its replaced Hashes; a new
    // name does not. Removing the entry deletes them (ADR 0026).
    @Test
    fun emulatorChangesForgetTheBases() {
        onlyNormalFont()
        val custom = Emulator("custom", CUSTOM.id, "Mine", mapOf(System.GBA to Folders()))
        val melon = Emulator("melonds", "melonds", "", mapOf(System.DS to Folders(nextToRom = true)))
        val ra = Emulator("retroarch", "retroarch", "", mapOf(System.GBA to Folders()))
        val bases = Bases(File(ctx.filesDir, "bases"))
        for (id in listOf("custom", "melonds", "retroarch")) {
            bases.set(id, "GBA-BPEF", "h")
            bases.setReplaced(id, "GBA-BPEF", "o")
        }
        fun kept(id: String): Boolean {
            rule.waitForIdle()
            App.waitIo()
            rule.waitForIdle()
            return File(ctx.filesDir, "bases/$id.json").exists()
        }
        start(url = "http://127.0.0.1:1", token = "t", emulators = listOf(custom, melon, ra))
        rule.onNodeWithText(s(R.string.tab_emulators)).performClick()

        rule.onNodeWithText("Mine").performClick()
        rule.onAllNodes(hasSetTextAction())[0].performTextInput("s") // its name
        assertTrue(kept("custom"))
        rule.onAllNodes(hasSetTextAction())[1].performTextInput("x") // its extension
        assertFalse(kept("custom"))
        assertTrue(bases.replaced("custom").isEmpty())
        back()

        rule.onNodeWithText("melonDS").performClick()
        rule.onNode(isToggleable()).performClick() // saves next to the ROMs
        assertFalse(kept("melonds"))
        back()

        rule.onNodeWithText("RetroArch").performClick()
        chip("gpSP").performClick()
        assertFalse(kept("retroarch"))
        bases.set("retroarch", "GBA-BPEF", "h")
        rule.onNodeWithText(s(R.string.remove_emulator)).performScrollTo().performClick()
        rule.onNode(hasText(s(R.string.remove)) and dialog).performClick()
        assertFalse(kept("retroarch"))
        assertEquals(listOf("custom", "melonds"), App.emulators.map { it.id })
    }

    // The save-safety cases of the details (ADR 0026): a save that came back after a Download is a Conflict with its
    // reason; an interrupted write offers only the server save and the backup copy, which goes back after a confirmation.
    @Test
    fun interruptedWriteAndCameBack() {
        val key = "GBA-BPEF"
        val url = fakeServer(sram) { line ->
            if (line.split(" ")[1] == "/saves") 200 to """[{"code":"$key","hash":"${sha256(sram)}","taille":${sram.size},"client":"pc","nom":"Emerald"}]""".toByteArray()
            else 200 to sram
        }
        val old = ByteArray(0x8000) { 3 }
        val came = Entry(
            emu, System.GBA, "Ruby", "Ruby.gba", Game(System.GBA, "GBA-AXVE", "Ruby", MemFile("Ruby.sav", old), Format(), emu.id),
            status = Status(local(old), other, other.hash, State.CONFLICT, cameBack = true),
        )
        val copy = File(ctx.filesDir, "copy.sav").apply { writeBytes(old) }
        val f = MemFile("Emerald.sav", old.copyOf(100)) // cut by the kill
        val game = Game(System.GBA, key, "Emerald", f, Format(), emu.id)
        val marker = PendingWrite(copy.path, "x")
        Pending(File(ctx.filesDir, "pending.json")).set(game, marker)
        val cut = Entry(emu, System.GBA, "Emerald", "Emerald.gba", game, status = Status(null, Version(sha256(sram), sram.size.toLong(), key), null, State.CONFLICT, interrupted = marker))
        start(url = url, token = "t", emulators = listOf(emu), entries = listOf(came, cut))
        rule.onNode(state(s(R.string.s_interrupted))).assertExists()

        open("Ruby")
        rule.onNodeWithText(s(R.string.why_came_back)).assertExists()
        rule.onNodeWithText(s(R.string.keep_phone)).performScrollTo().assertExists()
        back()

        open("Emerald")
        rule.onNodeWithText(s(R.string.why_interrupted)).assertExists()
        rule.onAllNodesWithText(s(R.string.keep_phone)).assertCountEquals(0)
        rule.onNodeWithText(s(R.string.take_server)).performScrollTo().assertExists()
        rule.onNodeWithText(s(R.string.restore_copy)).performScrollTo()
        fits(s(R.string.restore_copy), 2)
        rule.onNodeWithText(s(R.string.restore_copy)).performClick()
        rule.onNodeWithText(s(R.string.confirm_restore_copy)).assertExists()
        rule.onNode(hasText(s(R.string.restore_copy)) and dialog and hasClickAction()).performClick()
        waitFor(s(R.string.done_copy_restored))
        assertArrayEquals(old, f.content)
        waitFor(s(R.string.keep_phone)) // read again: a plain Conflict now (no Base)
        rule.onAllNodesWithText(s(R.string.restore_copy)).assertCountEquals(0)
        assertTrue(synchronized(requests) { requests.none { it.startsWith("PUT") } })
    }

    // A Duplicate (same Key): each ROM can get its own save, the Key of the PC for this ROM name; the list is read
    // again.
    @Test
    fun duplicateGetsItsOwnSave() {
        start(url = "http://127.0.0.1:1", token = "t", emulators = listOf(emu), entries = games())
        open("Tetris")
        rule.onNodeWithText("• Tetris (1).gb").assertExists()
        fits(s(R.string.own_save), 2)
        rule.onAllNodesWithText(s(R.string.own_save))[1].performScrollTo().performClick()
        val key = App.settings.aliases["Tetris (1)"]!!
        assertEquals(newAlias(emptyMap(), System.GB, "Tetris (1)"), key)
        assertTrue(key.startsWith("GB-ALIAS-"))
        assertEquals(1, App.settings.aliases.size)
        rule.onAllNodesWithContentDescription(s(R.string.back)).assertCountEquals(0) // back on the list, read again
    }
}

// Two entries of one emulator (melonDS on two ROM folders) with the same game: each row and the details say which entry,
// by its folder; a new name replaces the folder and keeps the Bases (same folders, same formats).
@RunWith(RobolectricTestRunner::class)
class TwoEntriesTest {
    @get:Rule
    val rule = createEmptyComposeRule()
    private val ctx: Context get() = ApplicationProvider.getApplicationContext()
    private fun s(id: Int, vararg a: Any) = ctx.getString(id, *a)
    private fun tree(path: String) = DocumentsContract.buildTreeDocumentUri(EXTERNAL_STORAGE, "primary:$path").toString()

    @Test
    fun sameGameInTwoEntries() {
        val a = Emulator("melonds", "melonds", "melonDS", mapOf(System.DS to Folders(roms = tree("roms/nds"), nextToRom = true)))
        val b = Emulator("melonds2", "melonds", "melonDS", mapOf(System.DS to Folders(roms = tree("Download/nds"), nextToRom = true)))
        fun row(e: Emulator) = Entry(e, System.DS, KART, "$KART.nds", Game(System.DS, "AMCE", KART, MemFile("$KART.sav", null), Format(), e.id), status = Status(null, Version("a".repeat(64), 10), null, State.TO_DOWNLOAD))
        App.init(ctx)
        App.settings.setupDone = true
        App.settings.url = "http://127.0.0.1:1"
        App.settings.token = "t"
        App.saveEmulators(listOf(a, b))
        App.entries = listOf(row(a), row(b))
        App.loaded = true
        App.busy = null
        App.serverError = null
        Bases(File(ctx.filesDir, "bases")).set("melonds2", "AMCE", "h")
        ActivityScenario.launch(MainActivity::class.java).use {
            rule.onNodeWithText("melonDS (roms/nds)", substring = true).assertExists()
            rule.onNodeWithText("melonDS (Download/nds)", substring = true).assertExists()

            rule.onNodeWithText(s(R.string.tab_emulators)).performClick()
            rule.onNodeWithText("melonDS (Download/nds)").performClick()
            rule.onNode(hasSetTextAction()).performTextClearance()
            rule.onNode(hasSetTextAction()).performTextInput("Tablet")
            rule.onNodeWithContentDescription(s(R.string.back)).performClick()
            rule.onNodeWithText("Tablet").assertExists()
            rule.onNodeWithText("melonDS").assertExists() // alone with its name now: no folder
            rule.waitForIdle()
            App.waitIo()
            assertTrue(File(ctx.filesDir, "bases/melonds2.json").exists())

            // the list reads the folders again after a change: the rows as they were, with the new name
            App.entries = listOf(row(App.emulators[0]), row(App.emulators[1]))
            App.loaded = true
            rule.onNodeWithText(s(R.string.tab_games)).performClick()
            rule.onNodeWithText("Tablet", substring = true).assertExists()
            rule.onNodeWithText("Tablet", substring = true).performClick()
            rule.onNodeWithText("Nintendo DS · Tablet").assertExists()
        }
    }

    // checks: what the Accessibility Test Framework would check, on the screen as it is (ATF itself does not run under
    // Robolectric): every control has a name, is at least 48 dp in each direction, and no two controls share a name.
    private fun checks() {
        val min = 48 * ctx.resources.displayMetrics.density - 1
        val names = mutableListOf<String>()
        for (n in rule.onAllNodes(hasClickAction()).fetchSemanticsNodes()) {
            val c = n.config
            val name = (c.getOrElseNullable(SemanticsProperties.ContentDescription) { null } ?: c.getOrElseNullable(SemanticsProperties.Text) { null }?.map { it.text })
                ?.joinToString(" ").orEmpty()
            assertTrue("a control without a name at ${n.boundsInRoot}", name.isNotBlank())
            val t = n.touchBoundsInRoot // clipped by the screen: empty below the fold
            if (t.isEmpty) continue
            assertTrue("$name: ${t.width} x ${t.height} px", maxOf(t.width, n.size.width.toFloat()) >= min && maxOf(t.height, n.size.height.toFloat()) >= min)
            names += name + "|" + c.getOrElseNullable(SemanticsProperties.StateDescription) { null }
        }
        assertEquals(names.distinct(), names)
    }

    // The main screens pass the checks; the titles are headings, for TalkBack's heading navigation.
    @Test
    fun accessibilityChecks() {
        val e = Emulator("melonds", "melonds", "", mapOf(System.DS to Folders(roms = tree("roms/nds"), nextToRom = true)))
        App.init(ctx)
        App.settings.setupDone = true
        App.settings.url = "http://127.0.0.1:1"
        App.settings.token = "t"
        App.saveEmulators(listOf(e))
        App.entries = listOf(Entry(e, System.DS, KART, "$KART.nds", Game(System.DS, "AMCE", KART, MemFile("$KART.sav", null), Format(), e.id), status = Status(null, Version("a".repeat(64), 10), null, State.TO_DOWNLOAD)))
        App.loaded = true
        App.busy = null
        App.serverError = null
        ActivityScenario.launch(MainActivity::class.java).use {
            checks() // Games
            rule.onNode(hasText(s(R.string.tab_games)) and SemanticsMatcher.keyIsDefined(SemanticsProperties.Heading)).assertExists()
            rule.onNodeWithText("Mario Kart DS").performClick()
            checks() // details
            rule.onNode(hasText("Mario Kart DS") and SemanticsMatcher.keyIsDefined(SemanticsProperties.Heading)).assertExists()
            rule.onNodeWithContentDescription(s(R.string.back)).performClick()
            rule.onNodeWithText(s(R.string.tab_emulators)).performClick()
            checks()
            rule.onNodeWithText("melonDS").performClick()
            checks() // one emulator, its folders: "Choose" says which folder
            rule.onNodeWithContentDescription(s(R.string.action_on, s(R.string.choose), s(R.string.rom_folder))).assertExists()
            rule.onNodeWithContentDescription(s(R.string.back)).performClick()
            rule.onNodeWithText(s(R.string.tab_settings)).performClick()
            checks()
            for (h in listOf(R.string.tab_settings, R.string.settings_title, R.string.bg_title, R.string.backups_title, R.string.hacks_title)) {
                rule.onNode(hasText(s(h)) and SemanticsMatcher.keyIsDefined(SemanticsProperties.Heading)).assertExists()
            }
        }
    }
}

// The largest font size of Android 14+ (200 %), on the same small screen: texts wrap, nothing is cut.
@Config(qualifiers = "fr-w320dp-h640dp", fontScale = 2f)
class UiFrLargeFontTest : UiTest()

@Config(qualifiers = "fr-w320dp-h640dp")
class UiFrSmallTest : UiTest() {
    @Test
    fun french() = assertEquals("Tout synchroniser", ctx.getString(R.string.sync_all))
}
