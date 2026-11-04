package io.github.programgames.savesync

import android.annotation.SuppressLint
import android.content.ContentResolver
import android.content.Context
import android.content.Intent
import android.net.Uri
import androidx.compose.runtime.getValue
import androidx.core.net.toUri
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import io.github.programgames.savesync.core.Backups
import io.github.programgames.savesync.core.Bases
import io.github.programgames.savesync.core.Choice
import io.github.programgames.savesync.core.Game
import io.github.programgames.savesync.core.Outcome
import io.github.programgames.savesync.core.PlayInfo
import io.github.programgames.savesync.core.RESTORE_CLIENT
import io.github.programgames.savesync.core.Remote
import io.github.programgames.savesync.core.State
import io.github.programgames.savesync.core.Status
import io.github.programgames.savesync.core.Summary
import io.github.programgames.savesync.core.Syncer
import io.github.programgames.savesync.core.Version
import io.github.programgames.savesync.core.System
import io.github.programgames.savesync.core.aliasKey
import io.github.programgames.savesync.core.extension
import io.github.programgames.savesync.core.extensions
import io.github.programgames.savesync.core.formats3DS
import io.github.programgames.savesync.core.newAlias
import io.github.programgames.savesync.core.pokemonPlayInfo
import io.github.programgames.savesync.core.readRom
import io.github.programgames.savesync.core.restoreVersion
import io.github.programgames.savesync.core.splitDuplicates
import io.github.programgames.savesync.core.titles3DS
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.asCoroutineDispatcher
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.io.File
import java.util.concurrent.Executors

// Shown is what the list shows for a Game: its State, or why it has none.
enum class Shown { CONFLICT, TO_DOWNLOAD, TO_UPLOAD, IN_SYNC, DUPLICATE, UNKNOWN, NONE }

// Entry is one ROM of one emulator. game null: Duplicate (never synced); dups: the ROM files of the Duplicate.
data class Entry(
    val emulator: Emulator,
    val system: System,
    val name: String,
    val rom: String,
    val game: Game?,
    val dups: List<String> = emptyList(),
    val sharedFile: Boolean = false, // Duplicate because the ROMs would use one Save file (else: same Key)
    val status: Status? = null,
    val error: Throwable? = null,
) {
    val id get() = emulator.id + "|" + system + "|" + rom // one line of the list

    val shown: Shown
        get() = when {
            game == null -> Shown.DUPLICATE
            error != null || status == null -> Shown.UNKNOWN
            else -> when (status.state) {
                State.CONFLICT -> Shown.CONFLICT
                State.TO_DOWNLOAD -> Shown.TO_DOWNLOAD
                State.TO_UPLOAD -> Shown.TO_UPLOAD
                State.IN_SYNC -> Shown.IN_SYNC
                null -> Shown.NONE
            }
        }
}

// App holds the state of the screens and runs every file and network access on one background thread, so two actions
// never touch the same save at once.
@SuppressLint("StaticFieldLeak") // context is the application context: it lives as long as the process
object App {
    lateinit var settings: Settings
    private lateinit var context: Context
    private val io = Executors.newSingleThreadExecutor().asCoroutineDispatcher()
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main)
    private var syncer: Syncer? = null

    var emulators by mutableStateOf(listOf<Emulator>())
    var aliases by mutableStateOf(mapOf<String, String>()) // ROM hacks (ADR 0014)
    var entries by mutableStateOf(listOf<Entry>())
    var serverError by mutableStateOf<Throwable?>(null)
    var scanProblems by mutableStateOf(listOf<String>())
    var busy by mutableStateOf<String?>(null) // what runs now, for the progress line
    var summary by mutableStateOf<Summary?>(null)
    var message by mutableStateOf<Pair<String, Throwable?>?>(null) // result of the last action on one game
    var loaded by mutableStateOf(false)
    var reminded = false // the reminder to close the emulator was shown since the app started

    fun init(c: Context) {
        if (::context.isInitialized && context === c.applicationContext) return // a new Application: the UI tests
        context = c.applicationContext
        reminded = false
        settings = Settings(context)
        emulators = settings.emulators
        aliases = settings.aliases
    }

    val resolver: ContentResolver get() = context.contentResolver
    val backupsDir: File get() = File(context.getExternalFilesDir(null) ?: context.filesDir, "copies")
    private val basesDir get() = File(context.filesDir, "bases")

    fun text(id: Int, vararg args: Any): String = context.getString(id, *args)

    fun configured() = settings.url.isNotBlank() && settings.token.isNotBlank()

    // mustRemind: the reminder to close the emulator comes before the first write on this phone (checkClosed).
    fun mustRemind() = !reminded && !settings.noReminder

    fun remind(never: Boolean) {
        reminded = true
        if (never) settings.noReminder = true
    }

    fun saveEmulators(list: List<Emulator>) {
        settings.emulators = list
        emulators = list
    }

    // addAlias gives this ROM its own save (ADR 0014): the Key of the PC for the same ROM name. The list must be read
    // again. Returns the error text, or null.
    fun addAlias(system: System, name: String): String? = runCatching {
        saveAliases(aliases + (name to newAlias(aliases, system, name)))
    }.exceptionOrNull()?.let { it.message ?: it.toString() }

    // removeAlias only forgets the mapping: no save is touched.
    fun removeAlias(name: String) = saveAliases(aliases - name)

    private fun saveAliases(a: Map<String, String>) {
        settings.aliases = a
        aliases = a
        loaded = false
    }

    // newId never reuses an id that has Bases: a new entry could point at other folders than the old one.
    fun newId(profile: String): String = generateSequence(1) { it + 1 }.map { if (it == 1) profile else "$profile$it" }
        .first { id -> emulators.none { it.id == id } && !File(basesDir, "$id.json").exists() }

    fun takeFolder(uri: Uri) =
        resolver.takePersistableUriPermission(uri, Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_GRANT_WRITE_URI_PERMISSION)

    private fun run(label: String, block: suspend () -> Unit) {
        if (busy != null) return
        busy = label
        scope.launch {
            try {
                block()
            } catch (t: Throwable) { // never a crash: the error shows on the screen
                message = text(R.string.done_failed) to t
            } finally {
                busy = null
            }
        }
    }

    // serverChanged: new address or token, the list and the States must be read again.
    fun serverChanged() {
        syncer = null
        loaded = false
    }

    private fun newSyncer() = Syncer(Remote(settings.url, settings.token), Bases(basesDir), Backups(backupsDir), { !mustRemind() }).also { syncer = it }

    // refresh reads the ROM folders, the server list and every Local save.
    fun refresh() = run(text(R.string.reading)) {
        message = null
        val s = newSyncer()
        val (found, problems) = withContext(io) { scanAll() }
        scanProblems = problems
        serverError = withContext(io) { runCatching { s.serverList() }.exceptionOrNull() }
        entries = withContext(io) { statuses(found, s) }
        loaded = true
    }

    private fun statuses(list: List<Entry>, s: Syncer): List<Entry> = list.mapIndexed { i, e ->
        busy = text(R.string.reading_saves, i + 1, list.size)
        if (e.game == null || serverError != null) e.copy(status = null, error = null)
        else runCatching { s.status(e.game) }.fold({ e.copy(status = it, error = null) }, { e.copy(status = null, error = it) })
    }

    // syncAll syncs every Game with a State; Conflicts and Duplicates wait for the user. confirmed: the games whose save
    // on this phone the user agreed to see replaced (the list of the confirmation); no other save is replaced.
    fun syncAll(confirmed: List<Entry>) = run(text(R.string.syncing)) {
        val s = syncer ?: newSyncer()
        val todo = entries.mapNotNull { e -> e.game?.takeIf { e.status?.state != null } }
        val ok = confirmed.mapNotNull { it.game }.toSet()
        summary = withContext(io) { s.syncAll(todo, ok) { i, g -> busy = text(R.string.syncing_game, i + 1, todo.size, g.name) } }
        s.refresh()
        serverError = withContext(io) { runCatching { s.serverList() }.exceptionOrNull() }
        entries = withContext(io) { statuses(entries, s) }
    }

    // act runs the action of one Game (or the choice of a Conflict), then reads its State again. mayReplace: the user
    // confirmed that the server version replaces the save on this phone; else no existing save is replaced.
    fun act(e: Entry, choice: Choice? = null, mayReplace: Boolean = false) = run(text(R.string.syncing)) {
        val s = syncer ?: newSyncer()
        val r = withContext(io) { runCatching { s.sync(e.game!!, choice, mayReplace) } }
        message = r.fold({
            text(
                when (it) {
                    Outcome.UPLOADED -> R.string.done_uploaded
                    Outcome.DOWNLOADED -> R.string.done_downloaded
                    Outcome.CONFLICT -> R.string.done_conflict
                    Outcome.NOT_REPLACED -> R.string.done_not_replaced
                    else -> R.string.done_in_sync
                },
            ) to null
        }, { text(R.string.done_failed) to it })
        reread(e, s)
    }

    // reread reads the State of one Game again, after an action changed the server or this phone.
    private suspend fun reread(e: Entry, s: Syncer) {
        s.refresh()
        val again = withContext(io) { runCatching { s.status(e.game!!) } }
        entries = entries.map { if (it.game?.key == e.game?.key && it.emulator.id == e.emulator.id) it.copy(status = again.getOrNull(), error = again.exceptionOrNull()) else it }
    }

    // history reads the server Versions of a Game, newest first (ADR 0016).
    suspend fun history(e: Entry): List<Version> = withContext(io) { Remote(settings.url, settings.token).history(e.game!!.key) }

    // versionPlayInfo reads one Version of the History for its Pokémon play time; null if not a Pokémon save or not read.
    suspend fun versionPlayInfo(e: Entry, v: Version): PlayInfo? = withContext(io) {
        runCatching { pokemonPlayInfo(Remote(settings.url, settings.token).version(e.game!!.key, v.file)) }.getOrNull()
    }

    // restore makes an old Version the Current version on the server, then reads the State of the game again: this phone
    // gets it only through the usual Download, with its Backup copy and its checks (ADR 0016). done: the error, or null.
    fun restore(e: Entry, v: Version, current: Version, done: (Throwable?) -> Unit) = run(text(R.string.restoring)) {
        val r = withContext(io) { runCatching { restoreVersion(Remote(settings.url, settings.token, RESTORE_CLIENT), e.game!!.key, v, current) } }
        message = r.fold({ text(R.string.restored, v.number) to null }, { text(R.string.restore_failed) to it })
        if (r.isSuccess) reread(e, syncer ?: newSyncer())
        done(r.exceptionOrNull())
    }

    // serverPlayInfo reads the Current version to show its Pokémon play time (Conflict screen, details).
    suspend fun serverPlayInfo(e: Entry): PlayInfo? = withContext(io) {
        runCatching { pokemonPlayInfo(Remote(settings.url, settings.token).get(e.game!!.key)) }.getOrNull()
    }

    // testConnection reads GET /saves: the number of games, or the error.
    fun testConnection(done: (Result<Int>) -> Unit) = run(text(R.string.testing)) {
        done(withContext(io) { runCatching { Remote(settings.url, settings.token, timeoutMs = 10_000).list().size } })
    }

    internal fun scanAll(): Pair<List<Entry>, List<String>> { // internal: SafTest
        val all = mutableListOf<Entry>()
        val problems = mutableListOf<String>()
        for (e in emulators) {
            val found = mutableListOf<Pair<Entry, String>>() // entry, save file identity
            for ((s, f) in e.systems) {
                try {
                    if (s == System.N3DS) scan3DS(e, f, found, problems) else scan(e, s, f, found, problems)
                } catch (t: Throwable) {
                    problems += text(R.string.folder_problem, "${e.label} ${s.name}: $t")
                }
            }
            val (kept, dups) = splitDuplicates(found, { it.first.game!!.key }, { it.second })
            all += kept.map { it.first }
            // One line per Key, else per shared Save file, with the ROM files of the Duplicate.
            val byKey = dups.groupBy { it.first.game!!.key }.values.partition { it.size > 1 }
            val byFile = byKey.second.flatten().groupBy { it.second.lowercase() }.values
            for ((group, shared) in byKey.first.map { it to false } + byFile.map { it to true }) {
                all += group.first().first.copy(game = null, dups = group.map { it.first.rom }, sharedFile = shared)
            }
        }
        return all.sortedBy { it.name.lowercase() } to problems
    }

    // scan walks the ROM folder of one System (subfolders too, hidden ones skipped) and pairs each ROM with its Save.
    private fun scan(e: Emulator, s: System, f: Folders, out: MutableList<Pair<Entry, String>>, problems: MutableList<String>) {
        val aliases = aliases
        val romTree = (f.roms ?: return).toUri()
        val saveTree = if (f.nextToRom) romTree else (f.saves ?: return).toUri()
        val format = e.format(s)
        val saves = mutableMapOf<String, List<Doc>>()
        fun walk(parent: String, depth: Int) {
            for (d in children(resolver, romTree, parent)) {
                if (d.name.startsWith(".")) continue
                if (d.folder) {
                    if (depth < 5) walk(d.id, depth + 1)
                    continue
                }
                val ext = extension(d.name)
                if (ext != ".zip" && extensions[ext] != s) continue
                val uri = android.provider.DocumentsContract.buildDocumentUriUsingTree(romTree, d.id)
                val rom = readRom(d.name) { resolver.openInputStream(uri)!! } ?: continue
                if (rom.system != s || rom.key == null) continue
                val name = rom.name // Android emulators name the save after the ROM or the .zip
                // the Alias of a ROM hack, as on the PC; an invalid one: never synced under the header Key
                val key = aliasKey(aliases, s, name, rom.key)
                if (key == null) {
                    problems += text(R.string.alias_invalid, name, aliases[name].orEmpty())
                    continue
                }
                val file = name + format.ext
                val folder = if (f.nextToRom) parent else rootId(saveTree)
                val doc = saves.getOrPut(folder) { children(resolver, saveTree, folder).filter { !it.folder } }
                    .let { l -> l.find { it.name == file } ?: l.find { it.name.equals(file, ignoreCase = true) } }
                val g = Game(s, key, name, SafFile(resolver, saveTree, folder, file, doc?.id, doc?.modified), format, e.id)
                out += Entry(e, s, name, d.name, g) to "$saveTree|$folder|$file"
            }
        }
        walk(rootId(romTree), 0)
    }

    // scan3DS finds the 3DS Pokémon games in the Azahar user folder (or its sdmc folder): the title ID is in the path,
    // no ROM is read. A game never started in Azahar has no folder: listed when its format is known, as on the PC (its
    // first Download creates the folder, ADR 0009). No Nintendo 3DS/<id0>/<id1> folder: Azahar was never set up there.
    private fun scan3DS(e: Emulator, f: Folders, out: MutableList<Pair<Entry, String>>, problems: MutableList<String>) {
        val tree = (f.saves ?: return).toUri()
        fun child(parent: String?, name: String) =
            parent?.let { p -> children(resolver, tree, p).find { it.folder && it.name.equals(name, ignoreCase = true) }?.id }
        val root = rootId(tree)
        val zeros = "0".repeat(32)
        val user = listOf("Nintendo 3DS", zeros, zeros).fold(child(root, "sdmc") ?: root, ::child)
            ?: return run { problems += text(R.string.azahar_not_set_up, e.title()) }
        for ((id, name) in titles3DS) {
            val lo = id.lowercase()
            val path = listOf("title", lo.substring(0, 8), lo.substring(8), "data", "00000001")
            var dir = user
            var found = 0 // folders of path that exist
            for (n in path) dir = child(dir, n)?.also { found++ } ?: break
            val g = if (found == path.size) {
                val main = children(resolver, tree, dir).find { !it.folder && it.name == "main" }
                Game(System.N3DS, "3DS-$id", name, SafFile(resolver, tree, dir, "main", main?.id, main?.modified), e.format(System.N3DS), e.id)
            } else {
                if (formats3DS[id] == null) continue // unknown format: never created, as on the PC
                val file = SafFile(resolver, tree, null, "main", null, path = (user.substringAfter(':') + "/" + path.joinToString("/")))
                Game(System.N3DS, "3DS-$id", name, file, e.format(System.N3DS), e.id, SafArchive(resolver, tree, dir, path.subList(found, path.size - 1), file))
            }
            out += Entry(e, System.N3DS, name, id, g) to "$tree|3DS-$id"
        }
    }
}
