package io.github.programgames.savesync

import android.content.ContentResolver
import android.net.Uri
import android.provider.DocumentsContract
import android.provider.DocumentsContract.Document
import io.github.programgames.savesync.core.SaveFile
import java.io.FileNotFoundException
import java.io.FileOutputStream

// Storage Access Framework: the app sees only the folders the user chose (ACTION_OPEN_DOCUMENT_TREE), never deletes
// anything, and writes only the Save files of these folders.

// Doc is one entry of a folder: its document id inside the tree, its name, folder or not, last change.
data class Doc(val id: String, val name: String, val folder: Boolean, val modified: Long?)

fun children(r: ContentResolver, tree: Uri, parent: String): List<Doc> {
    val uri = DocumentsContract.buildChildDocumentsUriUsingTree(tree, parent)
    val cols = arrayOf(Document.COLUMN_DOCUMENT_ID, Document.COLUMN_DISPLAY_NAME, Document.COLUMN_MIME_TYPE, Document.COLUMN_LAST_MODIFIED)
    val c = r.query(uri, cols, null, null, null) ?: throw FileNotFoundException("$tree: folder not readable")
    return c.use {
        buildList {
            while (it.moveToNext()) {
                add(Doc(it.getString(0), it.getString(1) ?: "", it.getString(2) == Document.MIME_TYPE_DIR, if (it.isNull(3)) null else it.getLong(3)))
            }
        }
    }
}

// rootId is the document id of the folder the user chose.
fun rootId(tree: Uri): String = DocumentsContract.getTreeDocumentId(tree)

// folderLabel shows a chosen folder: "RetroArch/saves" for "primary:RetroArch/saves".
fun folderLabel(tree: Uri): String = rootId(tree).substringAfter(':').ifEmpty { "/" }

// SafFile is a Save file in a chosen folder. id: its document id when the folder listing found it.
class SafFile(
    private val r: ContentResolver,
    private val tree: Uri,
    private val parent: String,
    override val name: String,
    private var id: String?,
    private var known: Long? = null,
) : SaveFile {
    override val location get() = parent.substringAfter(':') + "/" + name

    // find reads the folder again: the emulator may have created the file since the listing.
    private fun find(): String? {
        val all = children(r, tree, parent).filter { !it.folder }
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

    // write replaces the whole content ("wt": truncate), synced to the disk. The caller reads it back (replace).
    override fun write(content: ByteArray) {
        val doc = find() ?: create()
        val uri = DocumentsContract.buildDocumentUriUsingTree(tree, doc)
        r.openFileDescriptor(uri, "wt")!!.use { pfd ->
            FileOutputStream(pfd.fileDescriptor).use {
                it.write(content)
                it.flush()
                it.fd.sync()
            }
        }
    }

    private fun create(): String {
        val uri = DocumentsContract.createDocument(r, DocumentsContract.buildDocumentUriUsingTree(tree, parent), "application/octet-stream", name)
            ?: throw FileNotFoundException("$location: can't create the file")
        val doc = DocumentsContract.getDocumentId(uri)
        val created = children(r, tree, parent).find { it.id == doc }?.name
        // Some providers add " (1)" or an extension: the emulator would never read that file.
        if (created != name) throw FileNotFoundException("$location: the folder created \"$created\" instead")
        id = doc
        return doc
    }

    override fun modified(): Long? = known

    // modifiedNow lists the folder again: the date of the listing may be old (checkClosed).
    override fun modifiedNow(): Long? {
        find()
        return known
    }
}
