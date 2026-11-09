package io.github.programgames.savesync

import android.annotation.SuppressLint
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.ContentResolver
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Build
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import io.github.programgames.savesync.core.Problem
import io.github.programgames.savesync.core.Uploads
import io.github.programgames.savesync.core.isGlobal
import io.github.programgames.savesync.core.problem
import androidx.compose.runtime.getValue
import androidx.core.net.toUri
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import io.github.programgames.savesync.core.ARCHIVE_KEY
import io.github.programgames.savesync.core.ArchiveSave
import io.github.programgames.savesync.core.Backups
import io.github.programgames.savesync.core.Bases
import io.github.programgames.savesync.core.Choice
import io.github.programgames.savesync.core.Game
import io.github.programgames.savesync.core.Outcome
import io.github.programgames.savesync.core.Pending
import io.github.programgames.savesync.core.PlayInfo
import io.github.programgames.savesync.core.RELEASE_URL
import io.github.programgames.savesync.core.RESTORE_CLIENT
import io.github.programgames.savesync.core.Release
import io.github.programgames.savesync.core.Remote
import io.github.programgames.savesync.core.downloadUpdate
import io.github.programgames.savesync.core.fetchUrl
import io.github.programgames.savesync.core.newer
import io.github.programgames.savesync.core.parseRelease
import androidx.core.content.FileProvider
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import io.github.programgames.savesync.core.RomKeys
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
import io.github.programgames.savesync.core.notRead
import io.github.programgames.savesync.core.pokemonPlayInfo
import io.github.programgames.savesync.core.pokemonSize
import io.github.programgames.savesync.core.readRom
import io.github.programgames.savesync.core.restoreVersion
import io.github.programgames.savesync.core.shortName
import io.github.programgames.savesync.core.skip3DS
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

// installApk opens Android's installer on a downloaded APK (through FileProvider: Android 7+ refuses file://
// addresses). The first time, Android asks to allow this app to install apps.
fun installApk(ctx: Context, f: File) {
    val uri = FileProvider.getUriForFile(ctx, ctx.packageName + ".update", f)
    ctx.startActivity(
        Intent(Intent.ACTION_VIEW).setDataAndType(uri, "application/vnd.android.package-archive")
            .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_ACTIVITY_NEW_TASK),
    )
}

// Shown is what the list shows for a Game: its State, or why it has none. INTERRUPTED: a write of its save on this
// phone did not finish (a Conflict with only two choices, ADR 0026). NOT_SYNCED: a 3DS game turned off, or never
// started in Azahar (ADR 0028).
enum class Shown { CONFLICT, INTERRUPTED, TO_DOWNLOAD, TO_UPLOAD, IN_SYNC, DUPLICATE, UNKNOWN, NOT_SYNCED, NONE }

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
    val noLocal: Boolean = false, // server not read and no save on this phone: hidden until the server answers
    val off: Boolean = false, // a 3DS game the user turned off (Settings.off3DS): never synced
    val notStarted: Boolean = false, // a 3DSA game without its folder in Azahar: never synced, its folder never created
) {
    val id get() = emulator.id + "|" + system + "|" + rom // one line of the list

    val short get() = shortName(name) // the name the list shows, as on the 3DS

    val shown: Shown
        get() = when {
            game == null -> Shown.DUPLICATE
            off || notStarted -> Shown.NOT_SYNCED
            noLocal && status == null -> Shown.NONE
            error != null || status == null -> Shown.UNKNOWN
            status.interrupted != null -> Shown.INTERRUPTED
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

    var emulators by mutableStateOf(listOf<Emulator>())
    var aliases by mutableStateOf(mapOf<String, String>()) // ROM hacks (ADR 0014)
    var entries by mutableStateOf(listOf<Entry>())
    var serverError by mutableStateOf<Throwable?>(null)
    var scanProblems by mutableStateOf(listOf<String>())
    var scanNotes by mutableStateOf(listOf<String>()) // files the scan did not read (.7z…): not a problem, not notified
    var busy by mutableStateOf<String?>(null) // what runs now, for the progress line
    var summary by mutableStateOf<Summary?>(null)
    var message by mutableStateOf<Pair<String, Throwable?>?>(null) // result of the last action on one game
    var loaded by mutableStateOf(false)
    var reminded = false // the reminder to close the emulator was shown since the app started
    var lastRun by mutableStateOf<LastRun?>(null) // the last background pass
    var update by mutableStateOf<Release?>(null) // a newer version of the app (checkUpdate)
    var updating by mutableStateOf(false) // its download runs
    var updateError by mutableStateOf<Throwable?>(null) // its download or its check failed

    // Tests replace them: never GitHub, never Android's installer.
    var fetch: (String, Int) -> ByteArray = ::fetchUrl
    var install: (Context, File) -> Unit = ::installApk

    fun init(c: Context) {
        if (::context.isInitialized && context === c.applicationContext) return // a new Application: the UI tests
        context = c.applicationContext
        reminded = false
        settings = Settings(context)
        emulators = settings.emulators
        aliases = settings.aliases
        lastRun = settings.lastRun
        releaseFolders() // grants left by an older version, or by a pick the app did not save
    }

    fun setBackground(on: Boolean) {
        settings.background = on
        scheduleUploads(context, on)
    }

    // backgroundPass is the background upload (UploadWorker): the folders are read again, then only the Uploads are
    // done (Syncer.uploadAll). It runs on the io thread, so never at the same time as an action of the screens.
    suspend fun backgroundPass() {
        if (!settings.background || !configured()) return
        val (u, scanned) = withContext(io) {
            val (found, problems) = scanAll()
            syncer { false }.uploadAll(found.filter { !it.off && !it.notStarted }.mapNotNull { it.game }) to problems
        }
        record(u, scanned)
    }

    // record keeps the result of a background pass for the settings screen, and notifies only a new problem: one that
    // the last pass did not have (by kind: a Conflict of a game, a Problem, a folder line). A server out of reach (away
    // from home, Wi-Fi off) is never notified: the settings screen shows it, and such a pass says nothing of the other
    // problems, so the known ones stay known. A problem that goes away is notified again if it comes back.
    internal fun record(u: Uploads, scanned: List<String>, now: Long = java.lang.System.currentTimeMillis()) {
        val lines = u.conflicts.map { text(R.string.bg_conflict, it) } + u.failures.map { (name, t) ->
            val p = problem(t)
            if (isGlobal(p)) text(problemText(p)) else "$name: " + text(problemText(p)) // the server, not this game
        } + scanned
        val run = LastRun(now, u.uploaded, lines.joinToString("\n"))
        settings.lastRun = run
        lastRun = run
        if (u.failures.any { problem(it.second) in unreachable }) return
        val kinds = u.conflicts.map { "conflict $it" } + u.failures.map { problem(it.second).name } + scanned
        val known = settings.notified.split('\n').toSet()
        settings.notified = kinds.joinToString("\n")
        if (kinds.any { it !in known }) notifyProblems(run.problems)
    }

    // unreachable: the server can't be reached, which is normal away from home with a server on the home network.
    private val unreachable = setOf(Problem.NO_HOST, Problem.REFUSED, Problem.NO_RESPONSE)

    // notifyProblems shows one notification; without the permission (Android 13+), the settings screen only.
    private fun notifyProblems(problems: String) {
        val nm = context.getSystemService(NotificationManager::class.java)
        if (Build.VERSION.SDK_INT >= 26) {
            nm.createNotificationChannel(NotificationChannel(UPLOAD_WORK, text(R.string.bg_title), NotificationManager.IMPORTANCE_DEFAULT))
        }
        if (!NotificationManagerCompat.from(context).areNotificationsEnabled()) return
        val open = PendingIntent.getActivity(context, 0, Intent(context, MainActivity::class.java), PendingIntent.FLAG_IMMUTABLE)
        val n = NotificationCompat.Builder(context, UPLOAD_WORK)
            .setSmallIcon(android.R.drawable.stat_notify_sync_noanim)
            .setContentTitle(text(R.string.bg_problems_title))
            .setContentText(problems.substringBefore('\n'))
            .setStyle(NotificationCompat.BigTextStyle().bigText(problems))
            .setContentIntent(open)
            .setAutoCancel(true)
            .build()
        nm.notify(1, n)
    }

    // checkUpdate offers the latest GitHub release if it is newer than this app. GitHub is read at most once a day (the
    // release of the day is kept); a failure offers nothing, and the next start reads again. Off the io thread: a slow
    // GitHub never holds up a sync. A build of a developer ("dev") never asks.
    fun checkUpdate(current: String = BuildConfig.VERSION_NAME) {
        if (!settings.checkUpdates || current == "dev") return
        scope.launch {
            val r = withContext(Dispatchers.IO) { runCatching { latestRelease() }.getOrNull() }
            update = r?.takeIf { settings.checkUpdates && it.apk != null && it.sums != null && newer(current, it.tag) } // the switch may be off by now
        }
    }

    private fun latestRelease(): Release {
        val day = SimpleDateFormat("yyyy-MM-dd", Locale.ROOT).format(Date())
        if (settings.updateDay == day && settings.updateRelease.isNotEmpty()) return parseRelease(settings.updateRelease)
        val json = String(fetch(RELEASE_URL, 1 shl 22), Charsets.UTF_8)
        val r = parseRelease(json) // an unreadable answer is not kept
        settings.updateRelease = json
        settings.updateDay = day
        return r
    }

    // installUpdate downloads and checks the APK of the offered release, then opens Android's installer on it. ctx: the
    // screen, for the installer.
    fun installUpdate(ctx: Context) {
        val r = update ?: return
        if (updating) return
        updating = true
        updateError = null
        scope.launch {
            val f = withContext(Dispatchers.IO) { runCatching { downloadUpdate(r, File(context.cacheDir, "update"), fetch) } }
            updating = false
            f.fold({ runCatching { install(ctx, it) }.onFailure { e -> updateError = e } }, { updateError = it })
        }
    }

    val resolver: ContentResolver get() = context.contentResolver
    val backupsDir: File get() = File(context.getExternalFilesDir(null) ?: context.filesDir, "copies")
    private val basesDir get() = File(context.filesDir, "bases")
    private val pendingFile get() = File(context.filesDir, "pending.json") // markers of the writes in progress (ADR 0026)

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
        releaseFolders()
    }

    // releaseFolders gives back the folder grants that no emulator entry uses any more (a new pick, an entry or a system
    // removed): Android keeps 128 to 512 of them, and when full it drops the oldest one, which can be a folder in use.
    private fun releaseFolders() {
        val used = emulators.flatMap { it.systems.values }.flatMap { listOfNotNull(it.roms, it.saves) }.toSet()
        for (p in runCatching { resolver.persistedUriPermissions.toList() }.getOrDefault(emptyList())) {
            if (p.uri.toString() in used) continue
            val flags = (if (p.isReadPermission) Intent.FLAG_GRANT_READ_URI_PERMISSION else 0) or (if (p.isWritePermission) Intent.FLAG_GRANT_WRITE_URI_PERMISSION else 0)
            runCatching { resolver.releasePersistableUriPermission(p.uri, flags) }
        }
    }

    // changeEmulator saves a change of one emulator entry; the list must be read again. Any change but its name forgets
    // its Bases (ADR 0026): a Base of the old folders, core or extension must never turn a save into To upload.
    fun changeEmulator(id: String, keepBases: Boolean = false, f: (Emulator) -> Emulator) {
        if (!keepBases) forgetBases(id)
        saveEmulators(emulators.map { if (it.id == id) f(it) else it })
        loaded = false
    }

    // removeEmulator forgets an entry and deletes its Bases; no file of its folders and no Backup copy is touched.
    fun removeEmulator(id: String) {
        forgetBases(id)
        saveEmulators(emulators.filter { it.id != id })
        loaded = false
    }

    // forgetBases runs on the io thread, after the action that runs now: a sync in progress can't write a Base after it.
    private fun forgetBases(id: String) = scope.launch {
        withContext(io) { runCatching { Bases(basesDir).clear(id) } }.onFailure { message = text(R.string.done_failed) to it }
    }

    // waitIo: the tests wait for the actions queued on the io thread.
    internal fun waitIo() = kotlinx.coroutines.runBlocking(io) {}

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

    // takeFolder keeps the access to a picked folder after a restart. Refused (a provider without persistent access):
    // the folder still works until the app stops, then shows "Access lost" (Emulators).
    fun takeFolder(uri: Uri) = runCatching {
        resolver.takePersistableUriPermission(uri, Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_GRANT_WRITE_URI_PERMISSION)
    }

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
        loaded = false
    }

    // remote is the server of the settings; syncer, one pass of Syncs on it, with its own read of GET /saves (one per
    // action: the list is never older than the action). reminded: the user saw the reminder to close the emulator.
    private fun remote(client: String = "android") = Remote(settings.url, settings.token, client)

    private fun syncer(reminded: () -> Boolean = { !mustRemind() }) = Syncer(remote(), Bases(basesDir), Backups(backupsDir), Pending(pendingFile), reminded)

    // refresh reads the ROM folders, the server list and every Local save.
    fun refresh() = run(text(R.string.reading)) {
        message = null
        val s = syncer()
        val (found, problems, notes) = withContext(io) { scanAll() }
        scanProblems = problems
        scanNotes = notes
        serverError = withContext(io) { runCatching { s.serverList() }.exceptionOrNull() }
        entries = withContext(io) { statuses(found, s) }
        loaded = true
    }

    private fun statuses(list: List<Entry>, s: Syncer): List<Entry> = list.mapIndexed { i, e ->
        busy = text(R.string.reading_saves, i + 1, list.size)
        if (e.game == null || e.off || e.notStarted) named(e.copy(status = null, error = null), s)
        else if (serverError != null) e.copy(status = null, error = null, noLocal = !runCatching { e.game.file.exists() }.getOrDefault(true))
        else named(runCatching { s.status(e.game) }.fold({ e.copy(status = it, error = null) }, { e.copy(status = null, error = it) }), s)
    }.sortedBy { it.name.lowercase() }

    // named: a 3DSA game has no name on this phone (no ROM): the name a 3DS or a PC gave the server, if any (ADR 0018).
    private fun named(e: Entry, s: Syncer): Entry {
        val key = e.game?.key?.takeIf { it.startsWith(ARCHIVE_KEY) && serverError == null } ?: return e
        val n = runCatching { s.serverVersion(key)?.name }.getOrNull()
        return if (n.isNullOrBlank()) e else e.copy(name = n)
    }

    // setSynced turns a 3DS game on or off, by title ID, in every Azahar entry (ADR 0028). On: its State is read again.
    fun setSynced(e: Entry, on: Boolean) {
        settings.off3DS = if (on) settings.off3DS - e.rom else settings.off3DS + e.rom
        entries = entries.map { if (it.system == System.N3DS && it.rom == e.rom) it.copy(off = !on, status = null, error = null) else it }
        if (on && !e.notStarted) run(text(R.string.reading)) { reread(e, syncer()) }
    }

    // syncAll syncs every Game with a State; Conflicts and Duplicates wait for the user. confirmed: the games whose save
    // on this phone the user agreed to see replaced (the list of the confirmation); no other save is replaced.
    fun syncAll(confirmed: List<Entry>) = run(text(R.string.syncing)) {
        val s = syncer()
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
        val s = syncer()
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

    // restoreCopy puts back the Backup copy of an interrupted write (ADR 0026), then reads the State of the game again.
    fun restoreCopy(e: Entry) = run(text(R.string.syncing)) {
        val s = syncer()
        val r = withContext(io) { runCatching { s.restoreCopy(e.game!!) } }
        message = r.fold({ text(if (it) R.string.done_copy_restored else R.string.done_not_replaced) to null }, { text(R.string.done_failed) to it })
        reread(e, s)
    }

    // copies: the Backup copies of a Game on this phone, newest first (History screen).
    suspend fun copies(e: Entry): List<File> = withContext(io) { runCatching { Backups(backupsDir).of(e.game!!) }.getOrDefault(emptyList()) }

    // putBack writes a Backup copy back as the save of this phone (History screen); the game is then To upload.
    fun putBack(e: Entry, copy: File, done: (Throwable?) -> Unit) = run(text(R.string.syncing)) {
        val s = syncer()
        val r = withContext(io) { runCatching { s.putBack(e.game!!, copy) } }
        message = r.fold({ text(R.string.done_put_back) to null }, { text(R.string.done_failed) to it })
        reread(e, s)
        done(r.exceptionOrNull())
    }

    // saveCopy writes a Backup copy to a file the user created (Android's file picker): a folder they can open.
    fun saveCopy(copy: File, to: Uri) = run(text(R.string.saving_copy)) {
        val r = withContext(io) {
            runCatching { resolver.openOutputStream(to, "w")!!.use { it.write(copy.readBytes()) } }
        }
        message = r.fold({ text(R.string.done_copy_saved) to null }, { text(R.string.copy_not_saved) to it })
    }

    // reread reads the State of one Game again, after an action changed the server or this phone.
    private suspend fun reread(e: Entry, s: Syncer) {
        s.refresh()
        val again = withContext(io) { runCatching { s.status(e.game!!) } }
        entries = entries.map { if (it.game?.key == e.game?.key && it.emulator.id == e.emulator.id) it.copy(status = again.getOrNull(), error = again.exceptionOrNull()) else it }
    }

    // history reads the server Versions of a Game, newest first (ADR 0016).
    suspend fun history(e: Entry): List<Version> = withContext(io) { remote().history(e.game!!.key) }

    // versionPlayInfo reads one Version of the History for its Pokémon play time; null if not a Pokémon save or not read.
    suspend fun versionPlayInfo(e: Entry, v: Version): PlayInfo? = if (!pokemonSize(v.size)) null else withContext(io) {
        runCatching { pokemonPlayInfo(remote().version(e.game!!.key, v.file)) }.getOrNull()
    }

    // restore makes an old Version the Current version on the server, then reads the State of the game again: this phone
    // gets it only through the usual Download, with its Backup copy and its checks (ADR 0016). done: the error, or null.
    fun restore(e: Entry, v: Version, current: Version, done: (Throwable?) -> Unit) = run(text(R.string.restoring)) {
        val r = withContext(io) { runCatching { restoreVersion(remote(RESTORE_CLIENT), e.game!!.key, v, current) } }
        message = r.fold({ text(R.string.restored, v.number) to null }, { text(R.string.restore_failed) to it })
        if (r.isSuccess) reread(e, syncer())
        done(r.exceptionOrNull())
    }

    // serverPlayInfo reads the Current version to show its Pokémon play time (Conflict screen, details). Only a save of a
    // Pokémon size is downloaded: the details of any other game read nothing.
    suspend fun serverPlayInfo(e: Entry): PlayInfo? = if (e.status?.server?.size?.let(::pokemonSize) != true) null else withContext(io) {
        runCatching { pokemonPlayInfo(remote().get(e.game!!.key)) }.getOrNull()
    }

    // testConnection reads GET /saves: the number of games, or the error.
    fun testConnection(done: (Result<Int>) -> Unit) = run(text(R.string.testing)) {
        done(withContext(io) { runCatching { Remote(settings.url, settings.token, timeoutMs = 10_000).list().size } })
    }

    private val romKeysFile get() = File(context.filesDir, "romkeys.json") // the Keys of the ROMs read (RomKeys)

    // scanAll reads the folders of every emulator entry: the games, the problems, and the notes (files not read).
    internal fun scanAll(): Triple<List<Entry>, List<String>, List<String>> { // internal: SafTest
        val all = mutableListOf<Entry>()
        val problems = mutableListOf<String>()
        val notes = mutableListOf<String>()
        val keys = RomKeys(romKeysFile)
        for (e in emulators) {
            val found = mutableListOf<Pair<Entry, String>>() // entry, save file identity
            val skipped = mutableMapOf<String, String>() // document -> extension, once even in a folder of two systems
            for ((s, f) in e.systems) {
                try {
                    if (s == System.N3DS) scan3DS(e, f, found, problems) else scan(e, s, f, found, problems, keys, skipped)
                } catch (t: Throwable) {
                    problems += text(R.string.folder_problem, "${e.listName()} ${s.label}") // human words only: no exception text
                }
            }
            skippedNote(e, skipped)?.let { notes += it }
            val (kept, dups) = splitDuplicates(found, { it.first.game!!.key }, { it.second })
            all += kept.map { it.first }
            // One line per Key, else per shared Save file, with the ROM files of the Duplicate.
            val byKey = dups.groupBy { it.first.game!!.key }.values.partition { it.size > 1 }
            val byFile = byKey.second.flatten().groupBy { it.second.lowercase() }.values
            for ((group, shared) in byKey.first.map { it to false } + byFile.map { it to true }) {
                all += group.first().first.copy(game = null, dups = group.map { it.first.rom }, sharedFile = shared)
            }
        }
        keys.save()
        return Triple(all.sortedBy { it.name.lowercase() }, problems, notes)
    }

    // skippedNote: "melonDS: 3 files not read (.7z, .rar)…", or null when the scan read every ROM-like file.
    private fun skippedNote(e: Emulator, skipped: Map<String, String>): String? {
        if (skipped.isEmpty()) return null
        val exts = skipped.values.toSortedSet().joinToString(", ")
        return context.resources.getQuantityString(R.plurals.files_not_read, skipped.size, e.listName(), skipped.size, exts)
    }

    // folderResult is the line under the folders of one System after a pick: how many games have a save there, or why
    // there is none. null: a folder is still to choose. Reads the folders like a refresh (call it off the main thread).
    internal fun folderResult(e: Emulator, s: System): String? {
        val f = e.systems[s] ?: return null
        val trees = listOfNotNull(f.roms.takeIf { s != System.N3DS }, f.saves.takeIf { !f.nextToRom }).map { it.toUri() }
        if (trees.any { rootId(it).contains("Android/data", ignoreCase = true) }) return text(R.string.folder_android_data)
        if (if (s == System.N3DS) f.saves == null else f.roms == null) return null
        val found = mutableListOf<Pair<Entry, String>>()
        val problems = mutableListOf<String>()
        val skipped = mutableMapOf<String, String>()
        try {
            if (s == System.N3DS) scan3DS(e, f, found, problems) else scan(e, s, f, found, problems, RomKeys(romKeysFile), skipped) // read only
        } catch (_: Exception) {
            return text(R.string.folder_unreadable)
        }
        problems.firstOrNull()?.let { return it }
        val saves = found.count { it.first.game!!.file.exists() }
        val line = when {
            found.isEmpty() -> text(R.string.folder_no_game)
            saves > 0 -> context.resources.getQuantityString(R.plurals.folder_found, saves, saves, found.size)
            e.profile == RETROARCH && f.saves == null -> text(R.string.folder_no_save_retroarch)
            else -> text(R.string.folder_no_save)
        }
        return listOfNotNull(line, skippedNote(e, skipped)).joinToString("\n")
    }

    // scan walks the ROM folder of one System (subfolders too, hidden ones skipped) and pairs each ROM with its Save.
    // RetroArch: its saves folder is optional; the Save is in the folder of the core there ("Sort saves into folders by
    // core"), at its root, or next to the ROM ("Write saves to content directory"); "Sort saves into folders by content
    // directory" adds a folder named after the ROM's folder, in the core folder or at the root. Several of them: the
    // newest, the one RetroArch writes now. None: a new Save goes to the first of these places that exists.
    // skipped: the ROM-like files not read (notRead), by document.
    private fun scan(e: Emulator, s: System, f: Folders, out: MutableList<Pair<Entry, String>>, problems: MutableList<String>, keys: RomKeys, skipped: MutableMap<String, String>) {
        val aliases = aliases
        val romTree = (f.roms ?: return).toUri()
        val retro = e.profile == RETROARCH
        val saveTree = when {
            retro -> f.saves?.toUri()
            f.nextToRom -> romTree
            else -> (f.saves ?: return).toUri()
        }
        val core = if (retro && saveTree != null) {
            val label = e.variant(s).label
            children(resolver, saveTree, rootId(saveTree)).find { it.folder && it.name.equals(label, ignoreCase = true) }?.id
        } else null
        val format = e.format(s)
        val lists = mutableMapOf<String, List<Doc>>() // "tree|folder" -> what it holds
        fun list(tree: Uri, folder: String) = lists.getOrPut("$tree|$folder") { children(resolver, tree, folder) }
        fun files(tree: Uri, folder: String) = list(tree, folder).filter { !it.folder }
        // sub: the folder of this name in a folder of the saves tree (RetroArch content directory), any case
        fun sub(folder: String?, name: String) = folder?.let { p -> list(saveTree!!, p).find { it.folder && it.name.equals(name, ignoreCase = true) }?.id }
        fun walk(parent: String, dirName: String, depth: Int) {
            for (d in children(resolver, romTree, parent)) {
                if (d.name.startsWith(".")) continue
                if (d.folder) {
                    if (depth < 5) walk(d.id, d.name, depth + 1)
                    continue
                }
                val ext = extension(d.name)
                if (ext in notRead) skipped[d.id] = ext
                if (ext != ".zip" && extensions[ext] != s) continue
                val uri = android.provider.DocumentsContract.buildDocumentUriUsingTree(romTree, d.id)
                val rom = keys.rom(uri.toString(), d.name, d.size, d.modified) { readRom(d.name) { resolver.openInputStream(uri)!! } } ?: continue
                if (rom.system != s || rom.key == null) continue
                val name = rom.name // Android emulators name the save after the ROM or the .zip
                // the Alias of a ROM hack, as on the PC; an invalid one: never synced under the header Key
                val key = aliasKey(aliases, s, name, rom.key)
                if (key == null) {
                    problems += text(R.string.alias_invalid, name, aliases[name].orEmpty())
                    continue
                }
                val file = name + format.ext
                val places = when {
                    retro -> listOfNotNull(
                        sub(core, dirName)?.let { saveTree!! to it }, core?.let { saveTree!! to it },
                        saveTree?.let { t -> sub(rootId(t), dirName)?.let { t to it } }, saveTree?.let { it to rootId(it) }, romTree to parent,
                    )
                    f.nextToRom -> listOf(romTree to parent)
                    else -> listOf(saveTree!! to rootId(saveTree))
                }
                val found = places.mapNotNull { (t, folder) ->
                    files(t, folder).let { l -> l.find { it.name == file } ?: l.find { it.name.equals(file, ignoreCase = true) } }?.let { Triple(t, folder, it) }
                }
                // ponytail: the newest date decides between two copies; a copy made by a file manager gets a new date.
                // Ask (a Conflict "two files") if real phones show such copies.
                val (tree, folder, doc) = found.maxByOrNull { it.third.modified ?: 0L } ?: places.first().let { Triple(it.first, it.second, null) }
                val g = Game(s, key, name, SafFile(resolver, tree, folder, file, doc?.id, doc?.modified), format, e.id)
                out += Entry(e, s, name, d.name, g) to "$tree|$folder|$file"
            }
        }
        walk(rootId(romTree), rootId(romTree).substringAfter(':').substringAfterLast('/'), 0)
    }

    // scan3DS finds the 3DS games in the Azahar user folder (or its sdmc folder): the title ID is in the path, no ROM is
    // read. The 8 Pokémon games: their main file, Key 3DS- (ADR 0009); one never started in Azahar has no folder: listed
    // when its format is known, as on the PC (its first Download creates the folder). Every other game with a folder in
    // title/00040000: its whole save archive, Key 3DSA- (ADR 0028); one never started is listed as not synced, and its
    // folder is never created. No Nintendo 3DS/<id0>/<id1> folder: Azahar was never set up there.
    private fun scan3DS(e: Emulator, f: Folders, out: MutableList<Pair<Entry, String>>, problems: MutableList<String>) {
        val tree = (f.saves ?: return).toUri()
        fun child(parent: String?, name: String) =
            parent?.let { p -> children(resolver, tree, p).find { it.folder && it.name.equals(name, ignoreCase = true) }?.id }
        val root = rootId(tree)
        val zeros = "0".repeat(32)
        val user = listOf("Nintendo 3DS", zeros, zeros).fold(child(root, "sdmc") ?: root, ::child)
            ?: return run { problems += text(R.string.azahar_not_set_up, e.listName()) }
        val off = settings.off3DS
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
            out += Entry(e, System.N3DS, name, id, g, off = id in off) to "$tree|3DS-$id"
        }
        val apps = child(child(user, "title"), "00040000") ?: return
        for (d in children(resolver, tree, apps)) {
            if (!d.folder || !lowId.matches(d.name)) continue
            val id = "00040000" + d.name.uppercase()
            if (id in titles3DS || id in skip3DS) continue
            val dir = listOf("data", "00000001").fold(d.id as String?, ::child)
            val path = user.substringAfter(':') + "/title/00040000/" + d.name + "/data/00000001" // for the messages
            val g = Game(System.N3DS, "$ARCHIVE_KEY$id", "3DS $id", ArchiveSave(SafFolder(resolver, tree, dir, path)), e.format(System.N3DS), e.id)
            out += Entry(e, System.N3DS, g.name, id, g, off = id in off, notStarted = dir == null) to "$tree|$ARCHIVE_KEY$id"
        }
    }

    private val lowId = Regex("^[0-9A-Fa-f]{8}$") // the low word of a title ID, a folder in title/00040000
}
