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

    fun modified(): Long? // last change, ms since 1970, as last listed (no storage access); null if unknown

    fun modifiedNow(): Long? = modified() // last change read now from the storage; null or 0 if unknown

    fun exists(): Boolean = read() != null // as last listed where a listing exists (no full read)
}

// Game is a Key and its Save file in one emulator of this phone. emulator: id of the emulator entry, which owns the
// Bases and the folder of the Backup copies. archive: set for a 3DS game never started in Azahar (no folder yet).
data class Game(
    val system: System,
    val key: String,
    val name: String,
    val file: SaveFile,
    val format: Format,
    val emulator: String,
    val archive: NewArchive? = null,
)

// formats3DS: Azahar's 00000001.metadata of a 3DS game, as client/sync.go (ADR 0009): ArchiveFormatInfo (total_size,
// directory count, file count as u32 LE, duplicate_data, 3 padding bytes). Read on Moon and Ultra Sun, same value
// assumed for the 8 Pokémon games. A 3DS game without a known format is never created: it could reformat its save.
private val pokemonFormat = byteArrayOf(0, 0, 4, 0, 1, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0) // 0x40000 bytes, 1 dir, 1 file, duplicated
val formats3DS: Map<String, ByteArray> = titles3DS.keys.associateWith { pokemonFormat }

// NewArchive is the Azahar folder of a 3DS game never started: title/<high>/<low>/data/00000001 is missing (ADR 0009).
// Its main file is the Save file of the Game. SAF on the phone, memory in the tests.
interface NewArchive {
    fun exists(): Boolean // data/00000001 is there now: Azahar started the game since the listing

    fun hasMetadata(): Boolean // data/00000001.metadata is there

    fun create(metadata: ByteArray) // the missing folders down to data/, the .metadata (read back), then data/00000001

    fun undo(): Boolean // deletes main and what create made, and nothing else; false: something remains
}

// Local is the Local save: its Version covers the SRAM only (decode).
class Local(val version: Version, val raw: ByteArray, val sram: ByteArray, val blank: Boolean)

fun readLocal(g: Game, srv: Version?): Local? {
    val raw = g.file.read() ?: return null
    val sram = decode(g.system, g.format, raw, srv)
    // A 3DS main is never blank, except an empty one: a failed first write leaves it so, and it must not be uploaded.
    return Local(Version(sha256(sram), sram.size.toLong()), raw, sram, (g.system != System.N3DS || raw.isEmpty()) && blank(sram))
}

// seen is the Local save as the State sees it: a Blank save counts as absent, unless the server has the same Save.
fun seen(l: Local?, srv: Version?): Version? = l?.version?.takeIf { !l.blank || srv != null && it.hash == srv.hash }

// Bases: Key -> Hash of the last Sync, one JSON file per emulator, like bases/<emulator>.json on the PC. replaced: Key ->
// Hash of the Local save that the last Download of the Key replaced, in replaced/<emulator>.json (ADR 0026, "came back").
class Bases(private val dir: File) {
    fun of(emulator: String): Map<String, String> = readMap(File(dir, "$emulator.json"))

    fun set(emulator: String, key: String, hash: String) = writeAtomic(File(dir, "$emulator.json"), JSONObject(of(emulator) + (key to hash)).toString(2))

    fun replaced(emulator: String): Map<String, String> = readMap(File(dir, "replaced/$emulator.json"))

    // setReplaced keeps the Hash of the save a Download replaced; null forgets it.
    fun setReplaced(emulator: String, key: String, hash: String?) {
        val m = replaced(emulator)
        val n = if (hash == null) m - key else m + (key to hash)
        if (n != m) writeAtomic(File(dir, "replaced/$emulator.json"), JSONObject(n).toString(2))
    }

    // clear forgets every Base of an emulator entry and its replaced Hashes: its folders changed, or it was removed.
    fun clear(emulator: String) {
        for (f in listOf(File(dir, "$emulator.json"), File(dir, "replaced/$emulator.json"))) {
            if (f.exists() && !f.delete()) throw IOException("can't delete $f")
        }
    }
}

private fun readMap(f: File): Map<String, String> {
    if (!f.exists()) return emptyMap()
    val o = JSONObject(f.readText())
    return o.keys().asSequence().associateWith { o.getString(it) }
}

// writeAtomic replaces a file of the app with text: a verified temporary file, then a rename, so it is always whole.
fun writeAtomic(f: File, text: String) {
    f.parentFile!!.mkdirs()
    val tmp = File(f.parentFile, f.name + ".tmp")
    writeVerified(tmp, text.toByteArray(), replace = true)
    // rename replaces the target on Android (Linux); on Windows (JVM tests) it does not.
    if (!tmp.renameTo(f) && !(f.delete() && tmp.renameTo(f))) throw IOException("can't replace $f")
}

// PendingWrite is the marker of a write of a Save file (ADR 0026, "interrupted write"): written before the write,
// removed after the read-back, so one left means the write did not finish. backup: path of the Backup copy of the save
// it replaces, null if there was none; hash: SHA-256 of the content being written.
data class PendingWrite(val backup: String?, val hash: String)

// Pending keeps the PendingWrite markers in app-private storage, in one JSON file, by Key and Save file: a marker follows
// the file, whatever the emulator entry, and only a verified write removes it.
class Pending(private val file: File) {
    private fun id(g: Game) = g.key + "|" + g.file.location

    private fun all() = if (file.exists()) JSONObject(file.readText()) else JSONObject()

    fun get(g: Game): PendingWrite? = all().optJSONObject(id(g))?.let { PendingWrite(it.optString("backup").ifEmpty { null }, it.optString("hash")) }

    fun set(g: Game, p: PendingWrite) = writeAtomic(file, all().put(id(g), JSONObject().put("backup", p.backup ?: "").put("hash", p.hash)).toString(2))

    fun remove(g: Game) {
        val a = all()
        if (a.remove(id(g)) != null) writeAtomic(file, a.toString(2))
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

// Status of a Game for the list and the details. state null: nothing to sync (no save on either side). cameBack: a
// Conflict because the save came back to the one a Download replaced; interrupted: a write of this file did not
// finish (state Conflict, only two choices). Its backup is null when the Backup copy is not there.
class Status(
    val local: Local?,
    val server: Version?,
    val base: String?,
    val state: State?,
    val cameBack: Boolean = false,
    val interrupted: PendingWrite? = null,
)

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

// Uploads: result of a background pass (Syncer.uploadAll).
data class Uploads(val uploaded: Int, val recent: List<String>, val conflicts: List<String>, val failures: List<Pair<String, Throwable>>)

// writeProblems: a write on this phone failed, so the storage may be at fault for the next games too.
private val writeProblems = setOf(Problem.BACKUP, Problem.WRITE, Problem.RESTORE, Problem.NOT_WRITTEN, Problem.PARTIAL)

// Choice on a Conflict, for the Current version the user saw: if the server changed since, nothing is done.
data class Choice(val keepPhone: Boolean, val serverHash: String)

// OPEN_MS: a save changed less long ago may still be open in the emulator.
const val OPEN_MS = 30_000L

// checkClosed refuses a write while the emulator may still have the game open: it keeps the save in memory and would
// write it back over the new one. Android can't see if another app runs, so: a save changed less than OPEN_MS ago,
// or a save with no known date before the user saw the reminder to close the emulator. Reads stay allowed.
fun checkClosed(f: SaveFile, now: Long, reminded: Boolean) {
    val m = f.modifiedNow()?.takeIf { it in 1..now + OPEN_MS } // 0 (some providers) or far in the future: unknown
    if (m != null && now - m < OPEN_MS) throw SyncException(Problem.OPEN, "${f.location}: changed ${(now - m) / 1000} s ago")
    if (m == null && !reminded) throw SyncException(Problem.OPEN, "${f.location}: date of the last change unknown, reminder not shown")
}

// Syncer runs the Syncs of one pass: GET /saves is read once, and again after an Upload or a 412. reminded: the user saw
// the reminder to close the emulator (checkClosed); clock: now, in ms since 1970.
class Syncer(
    private val remote: Remote,
    private val bases: Bases,
    private val backups: Backups,
    private val pending: Pending,
    private val reminded: () -> Boolean,
    private val clock: () -> Long = { java.lang.System.currentTimeMillis() },
) {
    private var list: List<Version>? = null
    private val named = mutableSetOf<String>()

    fun refresh() {
        list = null
    }

    fun serverList(): List<Version> = list ?: remote.list().also { list = it }

    fun serverVersion(key: String) = serverList().find { it.code == key }

    // interrupted: the marker of a write of this Save file that did not finish (ADR 0026), or null. A marker whose write
    // did finish (the file holds the content written) is removed: the app stopped after the write, before the removal.
    private fun interrupted(g: Game): PendingWrite? {
        val p = pending.get(g) ?: return null
        if (g.file.read()?.let { sha256(it) } != p.hash) return p
        runCatching { pending.remove(g) }
        return null
    }

    // cameBack: the Local save is again the one that the last Download replaced, while the server still has the version
    // that Download wrote (the Base): an emulator left open wrote its old save back. A Conflict, never To upload.
    private fun cameBack(g: Game, seen: Version?, srv: Version?, base: String?) =
        seen != null && srv != null && base == srv.hash && seen.hash != base && seen.hash == bases.replaced(g.emulator)[g.key]

    fun status(g: Game): Status {
        val srv = serverVersion(g.key)
        val base = bases.of(g.emulator)[g.key]
        interrupted(g)?.let { p ->
            val local = runCatching { readLocal(g, srv) }.getOrNull() // a cut file may not decode
            return Status(local, srv, base, State.CONFLICT, interrupted = p.copy(backup = p.backup?.takeIf { File(it).isFile }))
        }
        val l = readLocal(g, srv)
        val seen = seen(l, srv)
        val back = cameBack(g, seen, srv, base)
        return Status(l, srv, base, if (seen == null && srv == null) null else if (back) State.CONFLICT else state(seen, srv, base), cameBack = back)
    }

    // sync runs the Sync of one Game, like syncGame on the PC. A Conflict without a matching choice changes nothing.
    // mayReplace = false: a Download over an existing save is not done (NOT_REPLACED): the user did not confirm it.
    // Only a choice to take this same server Version stands for that confirmation; keeping the phone save never does.
    // An interrupted write is a Conflict whose only choice here is the server Version (restoreCopy is the other one).
    fun sync(g: Game, choice: Choice? = null, mayReplace: Boolean = false): Outcome {
        repeat(3) { attempt -> // a 412 starts a new pass with the new Current version
            if (attempt > 0) refresh()
            val srv = serverVersion(g.key)
            if (srv != null && srv.name.isEmpty()) nameOnServer(g)
            val p = interrupted(g)
            val l = if (p == null) readLocal(g, srv) else null
            val seen = seen(l, srv)
            if (p == null && seen == null && srv == null) return Outcome.NOTHING
            val base = bases.of(g.emulator)[g.key]
            var st = if (p != null || cameBack(g, seen, srv, base)) State.CONFLICT else state(seen, srv, base)
            if (st == State.CONFLICT) {
                if (choice == null || srv == null || choice.serverHash != srv.hash) return Outcome.CONFLICT
                if (p != null && choice.keepPhone) return Outcome.CONFLICT // a cut file is never uploaded
                st = if (choice.keepPhone) State.TO_UPLOAD else State.TO_DOWNLOAD
            }
            when (st) {
                State.IN_SYNC -> { // the rule "came back" stays: the emulator may write its old save back later
                    bases.set(g.emulator, g.key, l!!.version.hash)
                    return Outcome.IN_SYNC
                }
                State.TO_UPLOAD -> if (upload(g, l!!, srv)) return Outcome.UPLOADED
                State.TO_DOWNLOAD -> try {
                    val takeServer = choice != null && !choice.keepPhone && choice.serverHash == srv!!.hash
                    if (l != null && !mayReplace && !takeServer) return Outcome.NOT_REPLACED
                    // after an interrupted write, the new file takes its shape from the Backup copy, not from the cut file
                    if (p == null) download(g, l?.raw, srv!!) else download(g, g.file.read(), srv!!, p.backup?.let { File(it) }?.takeIf { it.isFile }?.readBytes())
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

    // uploadOnly is the background pass of one Game: the same State as sync, but only an Upload is done. Never a
    // Download, never a Conflict choice, never a write on this phone except the Base after the server took the Upload.
    // To download: NOT_REPLACED. A save changed less than OPEN_MS ago is not uploaded (OPEN): the emulator may still be
    // writing it, so the content read may be cut. The date is read after the content, so a write during the read shows.
    fun uploadOnly(g: Game): Outcome {
        if (interrupted(g) != null) return Outcome.CONFLICT
        repeat(3) { attempt ->
            if (attempt > 0) refresh()
            val srv = serverVersion(g.key)
            if (srv != null && srv.name.isEmpty()) nameOnServer(g)
            val l = readLocal(g, srv)
            val seen = seen(l, srv)
            if (seen == null && srv == null) return Outcome.NOTHING
            val base = bases.of(g.emulator)[g.key]
            when (if (cameBack(g, seen, srv, base)) State.CONFLICT else state(seen, srv, base)) {
                State.IN_SYNC -> return Outcome.IN_SYNC
                State.TO_DOWNLOAD -> return Outcome.NOT_REPLACED
                State.CONFLICT -> return Outcome.CONFLICT
                State.TO_UPLOAD -> {
                    checkClosed(g.file, clock(), reminded = true) // an unknown date is fine: an Upload only reads
                    if (upload(g, l!!, srv)) return Outcome.UPLOADED
                }
            }
        }
        throw SyncException(Problem.BUSY, "PUT /saves/${g.key}: 412 three times")
    }

    // uploadAll runs the background pass: uploadOnly on every Game. recent: saves changed less than OPEN_MS ago, tried
    // again on the next pass. It stops when the server or the network fails.
    fun uploadAll(games: List<Game>): Uploads {
        var up = 0
        val recent = mutableListOf<String>()
        val conflicts = mutableListOf<String>()
        val failures = mutableListOf<Pair<String, Throwable>>()
        for (g in games) {
            try {
                when (uploadOnly(g)) {
                    Outcome.UPLOADED -> up++
                    Outcome.CONFLICT -> conflicts += g.name
                    else -> {}
                }
            } catch (t: Throwable) {
                val p = problem(t)
                if (p == Problem.OPEN) recent += g.name else failures += g.name to t
                if (isGlobal(p)) break
            }
        }
        return Uploads(up, recent, conflicts, failures)
    }

    // nameOnServer gives the game name to the server once per pass (ADR 0018); a failure never blocks the Sync.
    private fun nameOnServer(g: Game) {
        if (named.add(g.key)) runCatching { remote.setName(g.key, g.name) }
    }

    // restoreCopy is the other choice on an interrupted write: its Backup copy goes back, through replace (a Backup copy
    // of the file as it is now, then the copy, read back). The Base does not change. false: no interrupted write now.
    fun restoreCopy(g: Game): Boolean {
        val p = interrupted(g) ?: return false
        val copy = p.backup?.let { File(it) }?.takeIf { it.isFile }
            ?: throw SyncException(Problem.NO_COPY, "${g.file.location}: backup copy ${p.backup} not found")
        checkClosed(g.file, clock(), reminded())
        replace(g, g.file.read(), copy.readBytes(), backups, pending)
        return true
    }

    private fun upload(g: Game, l: Local, srv: Version?): Boolean {
        if (!remote.put(g.key, l.sram, srv)) return false
        if (srv == null) nameOnServer(g) // first Upload: the server does not have the name yet
        refresh() // the list no longer has the Current version of this Game
        bases.set(g.emulator, g.key, l.version.hash)
        bases.setReplaced(g.emulator, g.key, null)
        return true
    }

    // download writes the server Version. old: the Save file as read, null if none. template: the file whose Clock
    // trailer, MBC2 size and DeSmuME footer the new one keeps (encode), and whose SRAM the rule "came back" remembers.
    private fun download(g: Game, old: ByteArray?, srv: Version, template: ByteArray? = old) {
        checkClosed(g.file, clock(), reminded())
        val o = remote.get(g.key)
        if (sha256(o) != srv.hash) throw SyncException(Problem.DAMAGED, "GET /saves/${g.key}: hash ${sha256(o)} instead of ${srv.hash}")
        val content = encode(g.system, g.format, o, template)
        // The file must give back the received SRAM: else the next pass would see another save, and upload it.
        if (sha256(decode(g.system, g.format, content, srv)) != srv.hash) {
            throw FormatException("${g.file.location}: a ${content.size}-byte file wouldn't give back the ${o.size}-byte save")
        }
        val a = g.archive
        if (a != null && !a.exists()) createArchive(g, a, content, backups, pending) else replace(g, old, content, backups, pending)
        bases.setReplaced(g.emulator, g.key, template?.let { runCatching { sha256(decode(g.system, g.format, it, srv)) }.getOrNull() })
        bases.set(g.emulator, g.key, srv.hash)
    }
}

// replace writes a new Save file: verified Backup copy of the old one first, then the new content, read back and
// compared. On a mismatch the old content is written back. SAF has no atomic rename on every provider: this order
// keeps a verified copy outside the folder before the file is touched (ADR 0026). old: the file as the pass read it,
// null if there was none; a file that differs now (the emulator wrote it) is not touched: no copy holds it.
// pending: a PendingWrite marker is written before the file is touched and removed once the file is verified (ADR 0026).
// A marker left by an earlier interrupted write keeps its Backup copy: that one holds the save from before it.
fun replace(g: Game, old: ByteArray?, content: ByteArray, backups: Backups, pending: Pending? = null): File? {
    val copy = old?.let { backups.copy(g.emulator, g.name, "." + g.file.name.substringAfterLast('.', "sav"), it) }
    val now = g.file.read()
    if (if (old == null) now != null else now?.contentEquals(old) != true) {
        throw SyncException(Problem.CHANGED, "${g.file.location}: changed since it was read (${old?.size} bytes, now ${now?.size})")
    }
    val earlier = pending?.get(g)
    pending?.set(g, PendingWrite(if (earlier != null) earlier.backup else copy?.path, sha256(content)))
    // verified: the file is again as before this call (or empty): the marker of this call can go.
    fun verified() {
        if (earlier == null) pending?.let { runCatching { it.remove(g) } }
    }
    val written = runCatching { g.file.write(content) }.exceptionOrNull()
    val back = if (written == null) runCatching { g.file.read() }.getOrNull() else null
    if (back != null && back.contentEquals(content)) {
        pending?.let { runCatching { it.remove(g) } } // a marker left anyway goes at the next read: the hash matches
        return copy
    }
    val why = written?.toString() ?: "${g.file.location}: the written file doesn't match (${back?.size} bytes read, ${content.size} written)"
    if (old == null) {
        // A new file: emptied, so the next pass sees a blank save (absent), never a Local save to upload. No file (never
        // created, or created under another name): nothing to empty, and a write would create one more.
        val emptied = runCatching {
            g.file.read() == null || run {
                g.file.write(ByteArray(0))
                g.file.read()?.isEmpty() != false
            }
        }.getOrDefault(false)
        if (emptied) verified()
        throw SyncException(Problem.NOT_WRITTEN, if (emptied) why else "$why\n${g.file.location}: an incomplete file may remain")
    }
    // A write that failed before it touched the file leaves the old save: nothing to write back.
    val restored = runCatching {
        g.file.read().contentEquals(old) || run {
            g.file.write(old)
            g.file.read().contentEquals(old)
        }
    }.getOrDefault(false)
    if (restored) {
        verified()
        throw SyncException(Problem.WRITE, "$why\nbackup copy: $copy")
    }
    throw SyncException(Problem.RESTORE, "$why\nbackup copy: $copy")
}

// createArchive writes the first save of a 3DS game never started in Azahar, like download in client/sync.go: its folder
// and a .metadata of known format, then main through replace (no old save: no Backup copy). Never over an existing
// .metadata. On a failure, what this call created is deleted again: a folder with a .metadata and no full main would be
// a broken save for Azahar, while no folder is a game never started, which Azahar formats on its first start.
fun createArchive(g: Game, a: NewArchive, content: ByteArray, backups: Backups, pending: Pending? = null) {
    val format = formats3DS[g.key.removePrefix("3DS-")]
    if (format == null || a.hasMetadata()) {
        throw SyncException(Problem.NOT_STARTED, "${g.file.location}: no folder, ${if (format == null) "format unknown" else "00000001.metadata already there"}")
    }
    try {
        a.create(format)
        replace(g, null, content, backups, pending)
    } catch (e: Exception) {
        // CHANGED: a main appeared in the new folder (the emulator wrote it): it is not this app's, so it stays.
        if (e is SyncException && e.problem == Problem.CHANGED) throw e
        if (runCatching { a.undo() }.getOrDefault(false)) throw e
        val folder = g.file.location.substringBeforeLast('/')
        throw SyncException(Problem.PARTIAL, "$e\n$folder (and 00000001.metadata next to it): created, couldn't be removed")
    }
}

// splitDuplicates keeps the items alone for their Key and for their Save file; the others are Duplicates, never
// synced (ADR 0005, 0014, 0015): the app never guesses which ROM is the right one.
fun <T> splitDuplicates(items: List<T>, key: (T) -> String, file: (T) -> String): Pair<List<T>, List<T>> {
    val byKey = items.groupingBy(key).eachCount()
    val byFile = items.groupingBy { file(it).lowercase() }.eachCount()
    return items.partition { byKey[key(it)] == 1 && byFile[file(it).lowercase()] == 1 }
}
