package io.github.programgames.savesync

import android.content.ContentResolver
import android.net.Uri
import android.os.Environment
import android.provider.DocumentsContract
import androidx.core.net.toUri
import java.io.File
import android.provider.DocumentsContract.Document
import io.github.programgames.savesync.core.ArchiveFile
import io.github.programgames.savesync.core.ArchiveFolder
import io.github.programgames.savesync.core.NewArchive
import io.github.programgames.savesync.core.SaveFile
import java.io.FileNotFoundException
import java.io.FileOutputStream
import java.io.IOException

// Storage Access Framework: the app sees only the folders the user chose (ACTION_OPEN_DOCUMENT_TREE) and writes only
// the Save files of these folders. It deletes only what it created itself in the same action (SafArchive.undo).

// Doc is one entry of a folder: its document id inside the tree, its name, folder or not, last change, size.
data class Doc(val id: String, val name: String, val folder: Boolean, val modified: Long?, val size: Long? = null)

fun children(r: ContentResolver, tree: Uri, parent: String): List<Doc> {
    val uri = DocumentsContract.buildChildDocumentsUriUsingTree(tree, parent)
    val cols = arrayOf(Document.COLUMN_DOCUMENT_ID, Document.COLUMN_DISPLAY_NAME, Document.COLUMN_MIME_TYPE, Document.COLUMN_LAST_MODIFIED, Document.COLUMN_SIZE)
    val c = r.query(uri, cols, null, null, null) ?: throw FileNotFoundException("$tree: folder not readable")
    return c.use {
        buildList {
            while (it.moveToNext()) {
                fun long(i: Int) = if (it.isNull(i)) null else it.getLong(i)
                add(Doc(it.getString(0), it.getString(1) ?: "", it.getString(2) == Document.MIME_TYPE_DIR, long(3), long(4)))
            }
        }
    }
}

// make creates a document of this exact name in a folder; made gets its id before the name is checked, so that the
// caller can delete it. Some providers add " (1)" or an extension: the emulator would never read that file.
fun make(r: ContentResolver, tree: Uri, parent: String, mime: String, name: String, made: (String) -> Unit): String {
    val uri = DocumentsContract.createDocument(r, DocumentsContract.buildDocumentUriUsingTree(tree, parent), mime, name)
        ?: throw FileNotFoundException("${parent.substringAfter(':')}/$name: can't create it")
    val doc = DocumentsContract.getDocumentId(uri)
    made(doc)
    val created = children(r, tree, parent).find { it.id == doc }?.name
    if (created != name) throw FileNotFoundException("${parent.substringAfter(':')}/$name: the folder created \"$created\" instead")
    return doc
}

// writeDoc replaces the whole content of a document ("wt": truncate; "w" may leave the end of a longer old file), synced
// to the disk. The callers read it back.
fun writeDoc(r: ContentResolver, uri: Uri, content: ByteArray) {
    r.openFileDescriptor(uri, "wt")!!.use { pfd ->
        FileOutputStream(pfd.fileDescriptor).use {
            it.write(content)
            it.flush()
            it.fd.sync()
        }
    }
}

// rootId is the document id of the folder the user chose.
fun rootId(tree: Uri): String = DocumentsContract.getTreeDocumentId(tree)

// startAt is where the folder picker opens (Android 8+, EXTRA_INITIAL_URI): the folder chosen before, else the first
// usual folder (path from the root of the internal storage) that exists. null: where the picker wants.
fun startAt(current: String?, usual: List<String>): Uri? {
    if (current != null) return runCatching { current.toUri().let { DocumentsContract.buildDocumentUriUsingTree(it, rootId(it)) } }.getOrNull()
    val root = Environment.getExternalStorageDirectory()
    return usual.firstOrNull { File(root, it).isDirectory }?.let { DocumentsContract.buildDocumentUri(EXTERNAL_STORAGE, "primary:$it") }
}

const val EXTERNAL_STORAGE = "com.android.externalstorage.documents" // the provider of the phone storage

// folderLabel shows a chosen folder: "RetroArch/saves" for "primary:RetroArch/saves".
fun folderLabel(tree: Uri): String = rootId(tree).substringAfter(':').ifEmpty { "/" }

// SafFile is a Save file in a chosen folder. id: its document id when the folder listing found it. parent null: the
// folder does not exist yet (SafArchive sets it); path: where it will be, for the messages.
class SafFile(
    private val r: ContentResolver,
    private val tree: Uri,
    var parent: String?,
    override val name: String,
    private var id: String?,
    private var known: Long? = null,
    private val path: String = "",
) : SaveFile {
    override val location get() = (parent?.substringAfter(':') ?: path) + "/" + name

    // find reads the folder again: the emulator may have created the file since the listing.
    private fun find(): String? {
        val all = children(r, tree, parent ?: return null).filter { !it.folder }
        val d = all.find { it.name == name } ?: all.find { it.name.equals(name, ignoreCase = true) }
        known = d?.modified
        return d?.id.also { id = it }
    }

    override fun read(): ByteArray? {
        id?.let {
            try {
                return r.openInputStream(DocumentsContract.buildDocumentUriUsingTree(tree, it))!!.use { s -> s.readBytes() }
            } catch (_: FileNotFoundException) { // renamed or removed since the listing: look again by name
            }
        }
        val found = find() ?: return null
        return r.openInputStream(DocumentsContract.buildDocumentUriUsingTree(tree, found))!!.use { it.readBytes() }
    }

    // write replaces the whole content (writeDoc). The caller reads it back (replace). Then the folder is listed again,
    // so the details show the new date (modified).
    override fun write(content: ByteArray) {
        writeDoc(r, DocumentsContract.buildDocumentUriUsingTree(tree, find() ?: create()), content)
        runCatching { find() } // only the date: the write itself succeeded
    }

    override fun exists() = id != null

    private fun create(): String {
        id = make(r, tree, parent ?: throw FileNotFoundException("$location: no folder"), "application/octet-stream", name) {}
        return id!!
    }

    override fun modified(): Long? = known

    // modifiedNow lists the folder again: the date of the listing may be old (checkClosed).
    override fun modifiedNow(): Long? {
        find()
        return known
    }
}

// SafFolder is the Azahar folder of a 3DS save archive (ArchiveFolder, ADR 0028). dir: its document id, null when the
// game was never started (no folder: never created). path: where it is, for the messages. It writes only the files of
// a version, each with "wt" then read back, and never deletes anything.
class SafFolder(private val r: ContentResolver, private val tree: Uri, private val dir: String?, private val path: String) : ArchiveFolder {
    private var known: Long? = null

    override val location get() = dir?.substringAfter(':') ?: path

    // walk lists every file below the folder, with its path from it; null: no folder.
    private fun walk(): List<Pair<String, Doc>>? {
        val top = try {
            children(r, tree, dir ?: return null)
        } catch (_: FileNotFoundException) { // removed since the scan
            return null
        }
        val out = mutableListOf<Pair<String, Doc>>()
        fun add(list: List<Doc>, prefix: String) {
            for (d in list) if (d.folder) add(children(r, tree, d.id), prefix + d.name + "/") else out += prefix + d.name to d
        }
        add(top, "")
        known = out.mapNotNull { it.second.modified }.maxOrNull()
        return out
    }

    private fun read(doc: String) = r.openInputStream(DocumentsContract.buildDocumentUriUsingTree(tree, doc))!!.use { it.readBytes() }

    override fun files() = walk()?.map { (p, d) -> ArchiveFile(p, read(d.id)) }

    override fun write(path: String, content: ByteArray) {
        val parts = path.split('/')
        var parent = dir ?: throw FileNotFoundException("$location: no folder")
        for (n in parts.dropLast(1)) {
            parent = children(r, tree, parent).find { it.folder && it.name == n }?.id ?: make(r, tree, parent, Document.MIME_TYPE_DIR, n) {}
        }
        val name = parts.last()
        val doc = children(r, tree, parent).find { !it.folder && it.name == name }?.id ?: make(r, tree, parent, "application/octet-stream", name) {}
        writeDoc(r, DocumentsContract.buildDocumentUriUsingTree(tree, doc), content)
        if (!read(doc).contentEquals(content)) throw IOException("$location/$path: the written file doesn't match")
    }

    override fun modified() = known

    override fun modifiedNow(): Long? {
        walk()
        return known
    }
}

// SafArchive is the Azahar folder of a 3DS game never started (NewArchive). base: document id of the deepest folder that
// exists below sdmc/Nintendo 3DS/<id0>/<id1>; missing: the folder names below it down to "data". main: its main file.
class SafArchive(
    private val r: ContentResolver,
    private val tree: Uri,
    private val base: String,
    private val missing: List<String>,
    private val main: SafFile,
) : NewArchive {
    private val made = mutableListOf<Pair<String, Boolean>>() // document ids that create made, in order; true: folder

    // child finds a folder (any case, like scan3DS) or a file (exact name) in a folder.
    private fun child(parent: String?, name: String, folder: Boolean) = parent?.let { p ->
        children(r, tree, p).find { it.folder == folder && if (folder) it.name.equals(name, ignoreCase = true) else it.name == name }?.id
    }

    private fun data() = missing.fold(base as String?) { p, n -> child(p, n, true) }

    // exists also points main at the folder: the next write goes there, like for any game Azahar started.
    override fun exists() = child(data(), "00000001", true)?.also { main.parent = it } != null

    override fun hasMetadata() = child(data(), "00000001.metadata", false) != null

    override fun create(metadata: ByteArray) {
        val dir = Document.MIME_TYPE_DIR
        val data = missing.fold(base) { p, n -> child(p, n, true) ?: make(r, tree, p, dir, n) { made += it to true } }
        val meta = make(r, tree, data, "application/octet-stream", "00000001.metadata") { made += it to false }
        val uri = DocumentsContract.buildDocumentUriUsingTree(tree, meta)
        writeDoc(r, uri, metadata)
        val back = r.openInputStream(uri)!!.use { it.readBytes() }
        if (!back.contentEquals(metadata)) throw IOException("${main.location}: 00000001.metadata doesn't match once written")
        main.parent = make(r, tree, data, dir, "00000001") { made += it to true }
    }

    // undo deletes main from the folder create made, then what create made, in reverse order. A folder only when empty:
    // a file that someone else put there stays, and undo returns false.
    override fun undo(): Boolean {
        val folder = main.parent?.takeIf { p -> made.any { it.first == p } }
        var all = folder == null || runCatching {
            child(folder, "main", false)?.let { delete(it) }
            child(folder, "main", false) == null
        }.getOrDefault(false)
        for ((doc, isFolder) in made.reversed()) {
            all = runCatching { (!isFolder || children(r, tree, doc).isEmpty()) && delete(doc) }.getOrDefault(false) && all
        }
        made.clear()
        main.parent = null
        return all
    }

    private fun delete(doc: String) = DocumentsContract.deleteDocument(r, DocumentsContract.buildDocumentUriUsingTree(tree, doc))
}
