package io.github.programgames.savesync

import android.Manifest
import android.content.ContentResolver
import android.content.Context
import android.content.pm.ProviderInfo
import android.database.Cursor
import android.database.MatrixCursor
import android.net.Uri
import android.os.Bundle
import android.os.CancellationSignal
import android.os.ParcelFileDescriptor
import android.provider.DocumentsContract
import android.provider.DocumentsContract.Document
import android.provider.DocumentsContract.Root
import android.provider.DocumentsProvider
import androidx.test.core.app.ApplicationProvider
import io.github.programgames.savesync.core.Backups
import io.github.programgames.savesync.core.Format
import io.github.programgames.savesync.core.Game
import io.github.programgames.savesync.core.Pending
import io.github.programgames.savesync.core.PendingWrite
import io.github.programgames.savesync.core.Problem
import io.github.programgames.savesync.core.SyncException
import io.github.programgames.savesync.core.System
import io.github.programgames.savesync.core.checkClosed
import io.github.programgames.savesync.core.createArchive
import io.github.programgames.savesync.core.formats3DS
import io.github.programgames.savesync.core.replace
import io.github.programgames.savesync.core.sha256
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import org.junit.runner.RunWith
import org.robolectric.Robolectric
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.Implementation
import org.robolectric.annotation.Implements
import org.robolectric.shadows.ShadowContentResolver
import java.io.ByteArrayOutputStream
import java.io.File
import java.io.FileNotFoundException
import java.util.zip.ZipEntry
import java.util.zip.ZipOutputStream

const val AUTH = "io.github.programgames.savesync.test.docs"

// DirDocs is a DocumentsProvider over a folder of this computer, like ExternalStorageProvider over the phone storage:
// document id "root:<relative path>", tree URIs checked by isChildDocument, unique names with " (1)", recursive delete.
// Mode of openDocument: ParcelFileDescriptor.parseMode, so "wt" truncates as on ExternalStorageProvider. Assumption:
// the providers the app writes to (the phone storage, an SD card) truncate on "wt"; a provider that ignored the "t"
// would leave the end of a longer old save, and replace would see it on the read-back (mismatch, old one written back).
class DirDocs : DocumentsProvider() {
    lateinit var dir: File
    var before: (op: String, f: File) -> Unit = { _, _ -> } // op: read, write, create, delete; may throw or change f
    var date: (File) -> Long? = { it.lastModified() } // COLUMN_LAST_MODIFIED; some providers give 0 or nothing

    private fun file(id: String) = File(dir, id.removePrefix("root:"))
    private fun id(f: File) = "root:" + f.relativeTo(dir).invariantSeparatorsPath

    override fun onCreate() = true

    override fun queryRoots(projection: Array<out String>?): Cursor =
        MatrixCursor(projection ?: arrayOf(Root.COLUMN_ROOT_ID, Root.COLUMN_DOCUMENT_ID)).apply {
            newRow().add(Root.COLUMN_ROOT_ID, "root").add(Root.COLUMN_DOCUMENT_ID, "root:")
        }

    private fun cursor(p: Array<out String>?) = MatrixCursor(p ?: arrayOf(Document.COLUMN_DOCUMENT_ID, Document.COLUMN_DISPLAY_NAME, Document.COLUMN_MIME_TYPE, Document.COLUMN_LAST_MODIFIED))

    private fun row(c: MatrixCursor, f: File) {
        if (!f.exists()) throw FileNotFoundException("$f")
        c.newRow().add(Document.COLUMN_DOCUMENT_ID, id(f)).add(Document.COLUMN_DISPLAY_NAME, f.name)
            .add(Document.COLUMN_MIME_TYPE, if (f.isDirectory) Document.MIME_TYPE_DIR else "application/octet-stream")
            .add(Document.COLUMN_LAST_MODIFIED, date(f)).add(Document.COLUMN_SIZE, f.length())
    }

    override fun queryDocument(documentId: String, projection: Array<out String>?): Cursor = cursor(projection).also { row(it, file(documentId)) }

    override fun queryChildDocuments(parent: String, projection: Array<out String>?, sortOrder: String?): Cursor =
        cursor(projection).also { c -> (file(parent).listFiles() ?: throw FileNotFoundException(parent)).sortedBy { it.name }.forEach { row(c, it) } }

    override fun isChildDocument(parent: String, child: String): Boolean {
        val p = parent.removePrefix("root:")
        val c = child.removePrefix("root:")
        return p.isEmpty() || c == p || c.startsWith("$p/")
    }

    override fun openDocument(documentId: String, mode: String, signal: CancellationSignal?): ParcelFileDescriptor {
        val f = file(documentId)
        before(if ('w' in mode) "write" else "read", f)
        if (!f.isFile) throw FileNotFoundException("$f")
        return ParcelFileDescriptor.open(f, ParcelFileDescriptor.parseMode(mode))
    }

    override fun createDocument(parent: String, mimeType: String, displayName: String): String {
        val folder = file(parent)
        before("create", File(folder, displayName))
        val f = generateSequence(0) { it + 1 }.map { File(folder, if (it == 0) displayName else "$displayName ($it)") }.first { !it.exists() }
        if (!(if (mimeType == Document.MIME_TYPE_DIR) f.mkdir() else f.createNewFile())) throw FileNotFoundException("$f")
        return id(f)
    }

    override fun deleteDocument(documentId: String) {
        val f = file(documentId)
        before("delete", f)
        if (!f.deleteRecursively()) throw FileNotFoundException("$f")
    }
}

// BundleQueries: Robolectric sends ContentResolver.query(uri, projection, selection, args, sort) to the same method of
// the provider, which DocumentsProvider refuses; Android turns it into the Bundle query, as here.
@Implements(ContentResolver::class)
class BundleQueries : ShadowContentResolver() {
    @Implementation
    override fun query(uri: Uri, projection: Array<String>?, selection: String?, selectionArgs: Array<String>?, sortOrder: String?): Cursor? =
        query(uri, projection, Bundle().apply {
            putString(ContentResolver.QUERY_ARG_SQL_SELECTION, selection)
            putStringArray(ContentResolver.QUERY_ARG_SQL_SELECTION_ARGS, selectionArgs)
            putString(ContentResolver.QUERY_ARG_SQL_SORT_ORDER, sortOrder)
        }, null)
}

// End-to-end tests of Saf.kt through a real DocumentsProvider (DirDocs): the scan of the chosen folders, replace with
// its Backup copy and read-back, the first file of a game, the Azahar folder of a 3DS game never started, the dates.
@RunWith(RobolectricTestRunner::class)
@Config(shadows = [BundleQueries::class])
class SafTest {
    @get:Rule
    val tmp = TemporaryFolder()

    private val ctx: Context get() = ApplicationProvider.getApplicationContext()
    private val r: ContentResolver get() = ctx.contentResolver
    private lateinit var docs: DirDocs
    private lateinit var root: File
    private val ops = mutableListOf<String>() // "op path", every access of the app to the provider

    @Before
    fun setUp() {
        val info = ProviderInfo().apply {
            authority = AUTH
            exported = true
            grantUriPermissions = true
            readPermission = Manifest.permission.MANAGE_DOCUMENTS
            writePermission = Manifest.permission.MANAGE_DOCUMENTS
        }
        docs = Robolectric.buildContentProvider(DirDocs::class.java).create(info).get()
        root = tmp.newFolder("storage")
        docs.dir = root
        docs.before = { op, f -> ops += "$op ${f.relativeTo(root).invariantSeparatorsPath}" }
        App.init(ctx)
    }

    private fun put(path: String, content: ByteArray = ByteArray(0)) = File(root, path).also {
        it.parentFile!!.mkdirs()
        it.writeBytes(content)
    }

    private fun tree(path: String): Uri = DocumentsContract.buildTreeDocumentUri(AUTH, "root:$path")

    private fun header(at: Int, code: String) = ByteArray(0x200).also { code.toByteArray().copyInto(it, at) }
    private fun nds(code: String) = header(0x0C, code)
    private fun gba(code: String) = header(0xAC, code)
    private fun save(seed: Int, size: Int = 32 shl 10) = ByteArray(size) { (it * seed + seed).toByte() }

    // saf is the Save file of a game in folder (relative to the tree root), as the scan builds it.
    private fun saf(treePath: String, folder: String, name: String): SafFile {
        val t = tree(treePath)
        val parent = "root:$folder"
        val d = children(r, t, parent).find { it.name == name }
        return SafFile(r, t, parent, name, d?.id, d?.modified)
    }

    private fun game(f: SafFile) = Game(System.GBA, "GBA-BPEF", "Emeraude", f, Format(".sav"), "test")

    private fun problem(block: () -> Unit): Problem {
        val e = runCatching(block).exceptionOrNull() ?: error("no error")
        return (e as? SyncException ?: throw e).problem
    }

    // The scan reads the real folders: ROMs in subfolders and in a .zip, hidden folders skipped, a save next to its ROM
    // (melonDS) or in the folder of one RetroArch core, never the save of another core; Duplicates by Key and by file.
    @Test
    fun scan() {
        put("roms/nds/Platine.nds", nds("CPUF"))
        put("roms/nds/Platine.sav", save(1)).setLastModified(1_700_000_000_000L)
        put("roms/nds/Gen5/Noire.zip", ByteArrayOutputStream().also { o ->
            ZipOutputStream(o).use { z ->
                z.putNextEntry(ZipEntry("Pokemon Noire (F).nds"))
                z.write(nds("IRBF"))
            }
        }.toByteArray())
        put("roms/nds/Gen5/Noire.sav", save(2))
        put("roms/nds/Gen5/Blanche.nds", nds("IRAF"))
        put("roms/nds/Blanche.sav", save(3)) // not next to its ROM: not its save
        put("roms/nds/A/Diamant.nds", nds("ADAF"))
        put("roms/nds/B/Diamant (F).nds", nds("ADAF"))
        put("roms/nds/.trash/Perle.nds", nds("APAF"))
        put("roms/nds/readme.txt", "hello".toByteArray())
        put("roms/gba/Emeraude.gba", gba("BPEF"))
        put("roms/gba/x/Rouge.gba", gba("BPRF"))
        put("roms/gba/y/Rouge.gba", gba("BPGF")) // other Key, same save file Rouge.srm
        put("RetroArch/saves/mGBA/Emeraude.srm", save(4))
        put("RetroArch/saves/gpSP/Emeraude.srm", save(5))
        App.aliases = emptyMap()
        App.saveEmulators(
            listOf(
                Emulator("melonds", "melonds", "", mapOf(System.DS to Folders(roms = tree("roms/nds").toString(), nextToRom = true))),
                Emulator("retroarch", "retroarch", "", mapOf(System.GBA to Folders(roms = tree("roms/gba").toString(), saves = tree("RetroArch/saves/mGBA").toString()))),
            ),
        )
        val (entries, problems) = App.scanAll()
        assertEquals(emptyList<String>(), problems)
        val by = entries.associateBy { it.emulator.id + " " + it.name }
        assertEquals(setOf("melonds Platine", "melonds Noire", "melonds Blanche", "melonds Diamant", "retroarch Emeraude", "retroarch Rouge"), by.keys)

        val platine = by.getValue("melonds Platine").game!!
        assertEquals("CPUF", platine.key)
        assertEquals("roms/nds/Platine.sav", platine.file.location)
        assertArrayEquals(save(1), platine.file.read())
        assertEquals(1_700_000_000_000L, platine.file.modified())
        val noire = by.getValue("melonds Noire")
        assertEquals("Noire.zip", noire.rom)
        assertEquals("IRBF", noire.game!!.key)
        assertEquals("roms/nds/Gen5/Noire.sav", noire.game.file.location)
        assertArrayEquals(save(2), noire.game.file.read())
        val blanche = by.getValue("melonds Blanche").game!!
        assertEquals("roms/nds/Gen5/Blanche.sav", blanche.file.location)
        assertNull(blanche.file.read())
        val diamant = by.getValue("melonds Diamant")
        assertNull(diamant.game)
        assertEquals(setOf("Diamant.nds", "Diamant (F).nds"), diamant.dups.toSet())
        assertFalse(diamant.sharedFile)

        val emeraude = by.getValue("retroarch Emeraude").game!!
        assertEquals("RetroArch/saves/mGBA/Emeraude.srm", emeraude.file.location)
        assertArrayEquals(save(4), emeraude.file.read())
        val rouge = by.getValue("retroarch Rouge")
        assertNull(rouge.game)
        assertTrue(rouge.sharedFile)
        assertFalse(ops.any { it.startsWith("write") || it.startsWith("create") || it.startsWith("delete") }) // a scan only reads
    }

    // replace: a verified Backup copy in the folder of the app, the file replaced, read back. The new save is shorter:
    // "wt" truncates (else the end of the old save would remain and the read-back would not match).
    @Test
    fun replaceOk() {
        val old = save(1)
        val new = save(2, 16 shl 10)
        val f = put("saves/Emeraude.sav", old)
        val copy = replace(game(saf("saves", "saves", "Emeraude.sav")), old, new, Backups(App.backupsDir))!!
        assertArrayEquals(new, f.readBytes())
        assertArrayEquals(old, copy.readBytes())
        assertEquals(File(App.backupsDir, "test"), copy.parentFile)
        assertTrue(copy.name.startsWith("Emeraude-") && copy.name.endsWith(".sav"))
        assertEquals(listOf("read saves/Emeraude.sav", "write saves/Emeraude.sav", "read saves/Emeraude.sav"), ops)
    }

    // breakWrites: the next n writes keep only half of the content (the provider reports no error; the next read shows it).
    private fun breakWrites(n: Int) {
        var left = n
        var broken: File? = null
        docs.before = { op, f ->
            ops += "$op ${f.relativeTo(root).invariantSeparatorsPath}"
            if (op == "read" && f == broken) {
                f.writeBytes(f.readBytes().copyOf(f.length().toInt() / 2))
                broken = null
            }
            if (op == "write" && left-- > 0) broken = f
        }
    }

    // The marker of a write (ADR 0026) is in place while the provider writes, under the same name for the Save file as
    // the next scan builds it: a write cut by a kill shows as Interrupted at the next start. Gone once verified.
    @Test
    fun pendingMarkerDuringWrite() {
        val old = save(1)
        put("saves/Emeraude.sav", old)
        val pending = Pending(File(tmp.root, "pending.json"))
        val again = game(saf("saves", "saves", "Emeraude.sav")) // as a new scan finds it
        var during: PendingWrite? = null
        docs.before = { op, _ -> if (op == "write") during = pending.get(again) }
        val copy = replace(game(saf("saves", "saves", "Emeraude.sav")), old, save(2), Backups(App.backupsDir), pending)!!
        assertEquals(PendingWrite(copy.path, sha256(save(2))), during)
        assertNull(pending.get(again))
    }

    // A write that the provider breaks: the old save is written back, the Backup copy stays.
    @Test
    fun replaceBrokenWriteRestores() {
        val old = save(1)
        val f = put("saves/Emeraude.sav", old)
        breakWrites(1)
        val backups = Backups(App.backupsDir)
        assertEquals(Problem.WRITE, problem { replace(game(saf("saves", "saves", "Emeraude.sav")), old, save(2), backups) })
        assertArrayEquals(old, f.readBytes())
        assertArrayEquals(old, File(App.backupsDir, "test").listFiles()!!.single().readBytes())
    }

    // Every write broken: the old save can't be written back (RESTORE); the Backup copy holds it.
    @Test
    fun replaceRestoreFails() {
        val old = save(1)
        put("saves/Emeraude.sav", old)
        breakWrites(99)
        assertEquals(Problem.RESTORE, problem { replace(game(saf("saves", "saves", "Emeraude.sav")), old, save(2), Backups(App.backupsDir)) })
        assertArrayEquals(old, File(App.backupsDir, "test").listFiles()!!.single().readBytes())
    }

    // The provider refuses the write (permission lost): the file is never touched, nothing to write back.
    @Test
    fun replaceRefused() {
        val old = save(1)
        val f = put("saves/Emeraude.sav", old)
        docs.before = { op, _ -> if (op == "write") throw FileNotFoundException("permission lost") }
        assertEquals(Problem.WRITE, problem { replace(game(saf("saves", "saves", "Emeraude.sav")), old, save(2), Backups(App.backupsDir)) })
        assertArrayEquals(old, f.readBytes())
    }

    // The emulator writes the save after the pass read it, after the Backup copy: nothing is written, its save stays.
    @Test
    fun replaceChangedMeanwhile() {
        val old = save(1)
        val emulator = save(3)
        val f = put("saves/Emeraude.sav", old)
        val file = saf("saves", "saves", "Emeraude.sav")
        docs.before = { op, g ->
            ops += op
            if (op == "read" && ops.size == 1) g.writeBytes(emulator)
        }
        assertEquals(Problem.CHANGED, problem { replace(game(file), old, save(2), Backups(App.backupsDir)) })
        assertArrayEquals(emulator, f.readBytes())
        assertEquals(listOf("read"), ops)
    }

    // First download of a game: the file is created with its exact name, no Backup copy.
    @Test
    fun newFile() {
        put("saves/other.sav")
        val file = saf("saves", "saves", "Emeraude.sav")
        assertNull(replace(game(file), null, save(2), Backups(App.backupsDir)))
        assertArrayEquals(save(2), File(root, "saves/Emeraude.sav").readBytes())
        assertEquals(listOf("other.sav", "Emeraude.sav").sorted(), File(root, "saves").list()!!.sorted())
        assertFalse(File(App.backupsDir, "test").exists())
    }

    // A first download broken midway: the new file is emptied (a blank save counts as absent: never uploaded), never
    // left with a part of the save. It is not deleted: the only deletion is the 3DS folder cleanup (ADR 0026).
    @Test
    fun newFileBroken() {
        put("saves/other.sav")
        breakWrites(1)
        assertEquals(Problem.NOT_WRITTEN, problem { replace(game(saf("saves", "saves", "Emeraude.sav")), null, save(2), Backups(App.backupsDir)) })
        assertEquals(0L, File(root, "saves/Emeraude.sav").length())
    }

    // The provider refuses to create the file: nothing is left in the folder.
    @Test
    fun newFileNotCreated() {
        put("saves/other.sav")
        docs.before = { op, _ -> if (op == "create") throw FileNotFoundException("full") }
        val e = runCatching { replace(game(saf("saves", "saves", "Emeraude.sav")), null, save(2), Backups(App.backupsDir)) }.exceptionOrNull() as SyncException
        assertEquals(Problem.NOT_WRITTEN, e.problem)
        assertEquals(listOf("other.sav"), File(root, "saves").list()!!.toList())
        assertFalse(e.message!!, "incomplete" in e.message!!) // nothing remains: the message must not say otherwise
    }

    // A provider that renames the new file ("Emeraude.sav (1)"): the emulator would never read it, so it is an error.
    // That empty file stays (no deletion outside the 3DS cleanup, ADR 0026); no second one is created to be emptied.
    @Test
    fun newFileRenamed() {
        put("saves/other.sav")
        val file = saf("saves", "saves", "Emeraude.sav")
        File(root, "saves/Emeraude.sav").mkdir() // a folder of the same name: the file gets another name
        assertEquals(Problem.NOT_WRITTEN, problem { replace(game(file), null, save(2), Backups(App.backupsDir)) })
        assertEquals(listOf("Emeraude.sav", "Emeraude.sav (1)", "other.sav"), File(root, "saves").list()!!.sorted())
        assertEquals(0L, File(root, "saves/Emeraude.sav (1)").length())
    }

    private val user = "azahar/sdmc/Nintendo 3DS/" + "0".repeat(32) + "/" + "0".repeat(32)

    // pokemonX is the Game of Pokémon X as scan3DS finds it in the Azahar folder: title/00040000 exists (Pokémon Y
    // was started), title/00040000/00055d00 doesn't.
    private fun pokemonX(): Game {
        put("$user/title/00040000/00055e00/data/00000001/main", save(7))
        App.saveEmulators(listOf(Emulator("azahar", "azahar", "", mapOf(System.N3DS to Folders(saves = tree("azahar").toString())))))
        val entries = App.scanAll().first
        val y = entries.single { it.rom == "0004000000055E00" }.game!!
        assertNull(y.archive)
        assertArrayEquals(save(7), y.file.read())
        return entries.single { it.rom == "0004000000055D00" }.game!!.also { assertTrue(it.archive != null) }
    }

    private fun listing(dir: File): List<String> = dir.walkTopDown().map { it.relativeTo(root).invariantSeparatorsPath }.sorted().toList()

    // A 3DS game never started: its folder, the .metadata of known format, then main.
    @Test
    fun archiveCreated() {
        val g = pokemonX()
        createArchive(g, g.archive!!, save(8), Backups(App.backupsDir))
        val data = File(root, "$user/title/00040000/00055d00/data")
        assertArrayEquals(formats3DS.getValue("0004000000055D00"), File(data, "00000001.metadata").readBytes())
        assertArrayEquals(save(8), File(data, "00000001/main").readBytes())
        assertEquals(listOf("00000001", "00000001.metadata"), data.list()!!.sorted())
        assertEquals("$user/title/00040000/00055d00/data/00000001/main", g.file.location)
    }

    // A .metadata already there (Azahar half set up): never written over, nothing created.
    @Test
    fun archiveMetadataThere() {
        put("$user/title/00040000/00055d00/data/00000001.metadata", byteArrayOf(9))
        val g = pokemonX()
        val before = listing(root)
        assertEquals(Problem.NOT_STARTED, problem { createArchive(g, g.archive!!, save(8), Backups(App.backupsDir)) })
        assertEquals(before, listing(root))
        assertArrayEquals(byteArrayOf(9), File(root, "$user/title/00040000/00055d00/data/00000001.metadata").readBytes())
    }

    // The write of main fails: everything this call created is deleted again, nothing else (Pokémon Y stays).
    @Test
    fun archiveUndone() {
        val g = pokemonX()
        val before = listing(root)
        docs.before = { op, f -> if (op == "write" && f.name == "main") throw FileNotFoundException("full") }
        assertEquals(Problem.NOT_WRITTEN, problem { createArchive(g, g.archive!!, save(8), Backups(App.backupsDir)) })
        assertEquals(before, listing(root))
        assertArrayEquals(save(7), File(root, "$user/title/00040000/00055e00/data/00000001/main").readBytes())
    }

    // The folder 00000001 can't be created after the .metadata: the .metadata and the folders go again.
    @Test
    fun archiveUndoneMidway() {
        val g = pokemonX()
        val before = listing(root)
        docs.before = { op, f -> if (op == "create" && f.name == "00000001") throw FileNotFoundException("full") }
        runCatching { createArchive(g, g.archive!!, save(8), Backups(App.backupsDir)) }.also { assertTrue(it.isFailure) }
        assertEquals(before, listing(root))
    }

    // Someone else put a file in a folder this call created: it stays, with its folders (PARTIAL); the rest goes.
    @Test
    fun archiveForeignFileStays() {
        val g = pokemonX()
        val data = File(root, "$user/title/00040000/00055d00/data")
        docs.before = { op, f ->
            if (op == "write" && f.name == "main") {
                File(data, "other").writeBytes(byteArrayOf(1))
                throw FileNotFoundException("full")
            }
        }
        assertEquals(Problem.PARTIAL, problem { createArchive(g, g.archive!!, save(8), Backups(App.backupsDir)) })
        assertEquals(listOf("other"), data.list()!!.toList())
    }

    // modifiedNow reads the folder again: the date of the scan may be old (the emulator wrote since).
    @Test
    fun modifiedNow() {
        val now = java.lang.System.currentTimeMillis()
        val f = put("saves/Emeraude.sav", save(1)).also { it.setLastModified(now - 3_600_000) }
        val file = saf("saves", "saves", "Emeraude.sav")
        checkClosed(file, now, reminded = false) // old: allowed
        f.setLastModified(now - 5_000)
        assertEquals(now - 3_600_000, file.modified())
        assertEquals(now - 5_000, file.modifiedNow())
        assertEquals(Problem.OPEN, problem { checkClosed(file, now, reminded = true) })
        docs.date = { 0L } // some providers: 0, unknown
        assertEquals(Problem.OPEN, problem { checkClosed(file, now, reminded = false) })
        checkClosed(file, now, reminded = true)
        docs.date = { null } // no date at all
        assertNull(file.modifiedNow())
        assertEquals(Problem.OPEN, problem { checkClosed(file, now, reminded = false) })
        checkClosed(file, now, reminded = true)
        f.delete()
        assertNull(file.modifiedNow())
    }
}
