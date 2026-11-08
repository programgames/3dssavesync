package io.github.programgames.savesync.core

import org.json.JSONObject
import java.io.ByteArrayOutputStream
import java.io.File
import java.io.IOException
import java.net.HttpURLConnection
import java.net.URL

// The app updates itself from the GitHub releases, as the PC does (client/selfupdate.go): the latest release, its APK
// and SHA256SUMS; the APK is checked against SHA256SUMS before Android's installer opens it. Android installs it only
// if the same key signed it, and keeps the data of the app, the Backup copies too.

const val RELEASE_URL = "https://api.github.com/repos/programgames/3dssavesync/releases/latest"
const val APK_NAME = "3dssavesync-android.apk"

// Release: the latest GitHub release. apk, sums: the download addresses, null when the release has no such file.
data class Release(val tag: String, val page: String, val apk: String?, val sums: String?) {
    val version get() = tag.removePrefix("v")
}

fun parseRelease(json: String): Release {
    val o = JSONObject(json)
    val assets = o.optJSONArray("assets")
    fun url(name: String) = assets?.let { a -> (0 until a.length()).map { a.getJSONObject(it) }.find { it.optString("name") == name } }
        ?.optString("browser_download_url")?.ifEmpty { null }
    return Release(o.getString("tag_name"), o.optString("html_url"), url(APK_NAME), url("SHA256SUMS"))
}

// readVersion: "v3.10.2-rc.1" -> [3, 10, 2] and pre = true; null when it is not a version (like the PC).
private fun readVersion(s: String): Pair<List<Int>, Boolean>? {
    val (n, suffix) = s.removePrefix("v").split("-", limit = 2).let { it[0] to it.getOrNull(1) }
    val p = n.split(".").map { if (it.isEmpty() || !it.all(Char::isDigit)) return null else it.toInt() }
    if (p.size != 3 || suffix?.isEmpty() == true) return null
    return p to (suffix != null)
}

// newer: remote comes after current. "dev" (a build of a developer) is never older.
fun newer(current: String, remote: String): Boolean {
    val (a, pa) = readVersion(current) ?: return false
    val (b, pb) = readVersion(remote) ?: return false
    for (i in 0..2) if (a[i] != b[i]) return a[i] < b[i]
    return pa && !pb
}

// validSum: the "<sha256>  <name>" line of SHA256SUMS matches o.
fun validSum(sums: String, name: String, o: ByteArray): Boolean {
    val line = sums.lines().map { it.trim().split(Regex("\\s+")) }.find { it.size == 2 && it[1].removePrefix("*") == name } ?: return false
    return line[0].equals(sha256(o), ignoreCase = true)
}

// downloadUpdate downloads SHA256SUMS and the APK, checks the APK, and writes it alone in dir (a folder of this app:
// the older downloads go). Any failure leaves nothing to install. fetch: url, size limit -> body.
fun downloadUpdate(r: Release, dir: File, fetch: (String, Int) -> ByteArray = ::fetchUrl): File {
    dir.listFiles()?.forEach { it.delete() }
    val sums = String(fetch(r.sums ?: throw IOException("${r.tag}: no SHA256SUMS"), 1 shl 16), Charsets.UTF_8)
    val apk = fetch(r.apk ?: throw IOException("${r.tag}: no $APK_NAME"), 64 shl 20)
    if (!validSum(sums, APK_NAME, apk)) throw IOException("$APK_NAME: SHA-256 ${sha256(apk).take(12)}… is not the one of SHA256SUMS")
    dir.mkdirs()
    return File(dir, "3dssavesync-${r.version}.apk").apply { writeBytes(apk) }
}

// fetchUrl reads a GitHub address (the downloads redirect to another https host). max: the largest body accepted.
fun fetchUrl(url: String, max: Int): ByteArray {
    val c = URL(url).openConnection() as HttpURLConnection
    try {
        c.connectTimeout = CONNECT_MS
        c.readTimeout = 60_000
        c.setRequestProperty("User-Agent", "3dssavesync-android")
        if (c.responseCode != 200) throw IOException("GET $url: HTTP ${c.responseCode}")
        val out = ByteArrayOutputStream()
        val buf = ByteArray(1 shl 16)
        c.inputStream.use { s ->
            while (true) {
                val n = s.read(buf)
                if (n < 0) break
                out.write(buf, 0, n)
                if (out.size() > max) throw IOException("GET $url: more than $max bytes")
            }
        }
        return out.toByteArray()
    } finally {
        c.disconnect()
    }
}
