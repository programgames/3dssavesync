package io.github.programgames.savesync.core

import org.json.JSONObject
import java.io.File
import java.io.FileOutputStream
import java.io.IOException
import java.security.MessageDigest
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

fun hex(b: ByteArray) = b.joinToString("") { (it.toInt() and 0xFF).toString(16).padStart(2, '0') }

fun sha256(b: ByteArray): String = hex(MessageDigest.getInstance("SHA-256").digest(b))

// SaveFile is the Save file of a Game, in a folder the user chose (SAF on the phone, memory in the tests).
interface SaveFile {
    val name: String // file name, for the Backup copies and the messages
    val location: String // where it is, for the messages (folder and name)

    fun read(): ByteArray? // null: no file

    fun write(content: ByteArray) // creates the file, or replaces its whole content

    fun modified(): Long? // last change, ms since 1970; null if unknown
}

// Game is a Key and its Save file in one emulator of this phone. emulator: id of the emulator entry, which owns the
// Bases and the folder of the Backup copies.
data class Game(
    val system: System,
    val key: String,
    val name: String,
    val file: SaveFile,
    val format: Format,
    val emulator: String,
)

// Local is the Local save: its Version covers the SRAM only (decode).
class Local(val version: Version, val raw: ByteArray, val sram: ByteArray, val blank: Boolean)

fun readLocal(g: Game, srv: Version?): Local? {
    val raw = g.file.read() ?: return null
    val sram = decode(g.system, g.format, raw, srv)
    return Local(Version(sha256(sram), sram.size.toLong()), raw, sram, g.system != System.N3DS && blank(sram)) // a 3DS main is never blank
}

// seen is the Local save as the State sees it: a Blank save counts as absent, unless the server has the same Save.
fun seen(l: Local?, srv: Version?): Version? = l?.version?.takeIf { !l.blank || srv != null && it.hash == srv.hash }

// Bases: Key -> Hash of the last Sync, one JSON file per emulator, like bases/<emulator>.json on the PC.
class Bases(private val dir: File) {
    fun of(emulator: String): Map<String, String> {
        val f = File(dir, "$emulator.json")
        if (!f.exists()) return emptyMap()
        val o = JSONObject(f.readText())
        return o.keys().asSequence().associateWith { o.getString(it) }
    }

    fun set(emulator: String, key: String, hash: String) {
        val m = of(emulator) + (key to hash)
        dir.mkdirs()
        val f = File(dir, "$emulator.json")
        val tmp = File(dir, "$emulator.json.tmp")
        val text = JSONObject(m).toString(2)
        writeVerified(tmp, text.toByteArray(), replace = true)
        // rename replaces the target on Android (Linux); on Windows (JVM tests) it does not.
        if (!tmp.renameTo(f) && !(f.delete() && tmp.renameTo(f))) throw IOException("can't replace $f")
    }
}

// writeVerified writes a file, syncs it to the disk, then reads it back. replace = false: never overwrites.
fun writeVerified(f: File, content: ByteArray, replace: Boolean = false) {
    if (!replace && !f.createNewFile()) throw IOException("$f already exists")
    try {
        FileOutputStream(f).use {
            it.write(content)
            it.fd.sync()
        }
        if (!f.readBytes().contentEquals(content)) throw IOException("$f: the written file doesn't match")
    } catch (e: IOException) {
        f.delete() // only this file, which this call created and could not finish
        throw e
    }
}

// Backups: Backup copies in a folder of the app, <root>/<emulator>/<name>-<date><ext>, like the PC.
class Backups(val root: File) {
    fun copy(emulator: String, name: String, ext: String, raw: ByteArray): File {
        val dir = File(root, emulator)
        dir.mkdirs()
        val base = name + SimpleDateFormat("-yyyyMMdd-HHmmss", Locale.ROOT).format(Date())
        for (i in 0..9) {
            val f = File(dir, if (i == 0) "$base$ext" else "$base-$i$ext")
            if (f.exists()) continue
            try {
                writeVerified(f, raw)
                return f
            } catch (e: IOException) {
                throw SyncException(Problem.BACKUP, e.toString())
            }
        }
        throw SyncException(Problem.BACKUP, "$dir: 10 copies of $base already")
    }
}

// Status of a Game for the list and the details. state null: nothing to sync (no save on either side).
class Status(val local: Local?, val server: Version?, val base: String?, val state: State?)

enum class Outcome { NOTHING, IN_SYNC, UPLOADED, DOWNLOADED, CONFLICT, NOT_REPLACED }

// Summary of Sync all. stopped: the next games were not tried.
data class Summary(
    val uploaded: Int,
    val downloaded: Int,
    val inSync: Int,
    val conflicts: List<String>,
    val notReplaced: List<String>, // an existing save the user did not confirm: not replaced without a new look
    val failures: List<Pair<String, Throwable>>,
    val stopped: Boolean,
)

// writeProblems: a write on this phone failed, so the storage may be at fault for the next games too.
private val writeProblems = setOf(Problem.BACKUP, Problem.WRITE, Problem.RESTORE, Problem.NOT_WRITTEN)

// Choice on a Conflict, for the Current version the user saw: if the server changed since, nothing is done.
data class Choice(val keepPhone: Boolean, val serverHash: String)

// Syncer runs the Syncs of one pass: GET /saves is read once, and again after an Upload or a 412.
class Syncer(private val remote: Remote, private val bases: Bases, private val backups: Backups) {
    private var list: List<Version>? = null
    private val named = mutableSetOf<String>()

    fun refresh() {
        list = null
    }

    fun serverList(): List<Version> = list ?: remote.list().also { list = it }

    fun serverVersion(key: String) = serverList().find { it.code == key }

    fun status(g: Game): Status {
        val srv = serverVersion(g.key)
        val l = readLocal(g, srv)
        val seen = seen(l, srv)
        val base = bases.of(g.emulator)[g.key]
        return Status(l, srv, base, if (seen == null && srv == null) null else state(seen, srv, base))
    }

    // sync runs the Sync of one Game, like syncGame on the PC. A Conflict without a matching choice changes nothing.
    // mayReplace = false: a Download over an existing save is not done (NOT_REPLACED): the user did not confirm it.
    // Only a choice to take this same server Version stands for that confirmation; keeping the phone save never does.
    fun sync(g: Game, choice: Choice? = null, mayReplace: Boolean = false): Outcome {
        repeat(3) { attempt -> // a 412 starts a new pass with the new Current version
            if (attempt > 0) refresh()
            val srv = serverVersion(g.key)
            if (srv != null && srv.name.isEmpty()) nameOnServer(g)
            val l = readLocal(g, srv)
            val seen = seen(l, srv)
            if (seen == null && srv == null) return Outcome.NOTHING
            var st = state(seen, srv, bases.of(g.emulator)[g.key])
            if (st == State.CONFLICT) {
                if (choice == null || choice.serverHash != srv!!.hash) return Outcome.CONFLICT
                st = if (choice.keepPhone) State.TO_UPLOAD else State.TO_DOWNLOAD
            }
            when (st) {
                State.IN_SYNC -> {
                    bases.set(g.emulator, g.key, l!!.version.hash)
                    return Outcome.IN_SYNC
                }
                State.TO_UPLOAD -> if (upload(g, l!!, srv)) return Outcome.UPLOADED
                State.TO_DOWNLOAD -> try {
                    val takeServer = choice != null && !choice.keepPhone && choice.serverHash == srv!!.hash
                    if (l != null && !mayReplace && !takeServer) return Outcome.NOT_REPLACED
                    download(g, l, srv!!)
                    return Outcome.DOWNLOADED
                } catch (e: SyncException) {
                    // Content other than the list: a Version arrived after the list was read, so read again.
                    if (e.problem != Problem.DAMAGED || attempt == 2) throw e
                }
                State.CONFLICT -> {}
            }
        }
        throw SyncException(Problem.BUSY, "PUT /saves/${g.key}: 412 three times")
    }

    // syncAll runs Sync all: one Sync per Game, never a Conflict choice. confirmed: the Games whose save on this phone
    // the user agreed to see replaced; any other existing save is not replaced (NOT_REPLACED). It stops when the
    // server or the network fails, or a write on this phone fails: the next games are not tried.
    fun syncAll(games: List<Game>, confirmed: Set<Game>, progress: (Int, Game) -> Unit = { _, _ -> }): Summary {
        var up = 0
        var down = 0
        var same = 0
        val conflicts = mutableListOf<String>()
        val notReplaced = mutableListOf<String>()
        val failures = mutableListOf<Pair<String, Throwable>>()
        for ((i, g) in games.withIndex()) {
            progress(i, g)
            try {
                when (sync(g, mayReplace = g in confirmed)) {
                    Outcome.UPLOADED -> up++
                    Outcome.DOWNLOADED -> down++
                    Outcome.IN_SYNC -> same++
                    Outcome.CONFLICT -> conflicts += g.name
                    Outcome.NOT_REPLACED -> notReplaced += g.name
                    Outcome.NOTHING -> {}
                }
            } catch (t: Throwable) {
                failures += g.name to t
                val p = problem(t)
                if (isGlobal(p) || p in writeProblems) return Summary(up, down, same, conflicts, notReplaced, failures, true)
            }
        }
        return Summary(up, down, same, conflicts, notReplaced, failures, false)
    }

    // nameOnServer gives the game name to the server once per pass (ADR 0018); a failure never blocks the Sync.
    private fun nameOnServer(g: Game) {
        if (named.add(g.key)) runCatching { remote.setName(g.key, g.name) }
    }

    private fun upload(g: Game, l: Local, srv: Version?): Boolean {
        if (!remote.put(g.key, l.sram, srv)) return false
        if (srv == null) nameOnServer(g) // first Upload: the server does not have the name yet
        refresh() // the list no longer has the Current version of this Game
        bases.set(g.emulator, g.key, l.version.hash)
        return true
    }

    private fun download(g: Game, l: Local?, srv: Version) {
        val o = remote.get(g.key)
        if (sha256(o) != srv.hash) throw SyncException(Problem.DAMAGED, "GET /saves/${g.key}: hash ${sha256(o)} instead of ${srv.hash}")
        val content = encode(g.system, g.format, o, l?.raw)
        // The file must give back the received SRAM: else the next pass would see another save, and upload it.
        if (sha256(decode(g.system, g.format, content, srv)) != srv.hash) {
            throw FormatException("${g.file.location}: a ${content.size}-byte file wouldn't give back the ${o.size}-byte save")
        }
        replace(g, l?.raw, content, backups)
        bases.set(g.emulator, g.key, srv.hash)
    }
}

// replace writes a new Save file: verified Backup copy of the old one first, then the new content, read back and
// compared. On a mismatch the old content is written back. SAF has no atomic rename on every provider: this order
// keeps a verified copy outside the folder before the file is touched (ADR 0026). old: the file as the pass read it,
// null if there was none; a file that differs now (the emulator wrote it) is not touched: no copy holds it.
fun replace(g: Game, old: ByteArray?, content: ByteArray, backups: Backups): File? {
    val copy = old?.let { backups.copy(g.emulator, g.name, "." + g.file.name.substringAfterLast('.', "sav"), it) }
    val now = g.file.read()
    if (if (old == null) now != null else now?.contentEquals(old) != true) {
        throw SyncException(Problem.CHANGED, "${g.file.location}: changed since it was read (${old?.size} bytes, now ${now?.size})")
    }
    val written = runCatching { g.file.write(content) }.exceptionOrNull()
    val back = if (written == null) runCatching { g.file.read() }.getOrNull() else null
    if (back != null && back.contentEquals(content)) return copy
    val why = written?.toString() ?: "${g.file.location}: the written file doesn't match (${back?.size} bytes read, ${content.size} written)"
    if (old == null) {
        // A new file: emptied, so the next pass sees a blank save (absent), never a Local save to upload.
        val emptied = runCatching {
            g.file.write(ByteArray(0))
            g.file.read()?.isEmpty() != false
        }.getOrDefault(false)
        throw SyncException(Problem.NOT_WRITTEN, if (emptied) why else "$why\n${g.file.location}: an incomplete file may remain")
    }
    // A write that failed before it touched the file leaves the old save: nothing to write back.
    val restored = runCatching {
        g.file.read().contentEquals(old) || run {
            g.file.write(old)
            g.file.read().contentEquals(old)
        }
    }.getOrDefault(false)
    if (restored) throw SyncException(Problem.WRITE, "$why\nbackup copy: $copy")
    throw SyncException(Problem.RESTORE, "$why\nbackup copy: $copy")
}

// splitDuplicates keeps the items alone for their Key and for their Save file; the others are Duplicates, never
// synced (ADR 0005, 0014, 0015): the app never guesses which ROM is the right one.
fun <T> splitDuplicates(items: List<T>, key: (T) -> String, file: (T) -> String): Pair<List<T>, List<T>> {
    val byKey = items.groupingBy(key).eachCount()
    val byFile = items.groupingBy { file(it).lowercase() }.eachCount()
    return items.partition { byKey[key(it)] == 1 && byFile[file(it).lowercase()] == 1 }
}
