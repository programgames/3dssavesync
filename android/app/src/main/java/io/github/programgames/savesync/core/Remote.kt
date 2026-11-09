package io.github.programgames.savesync.core

import java.io.FileNotFoundException
import java.io.IOException
import java.net.ConnectException
import java.net.HttpURLConnection
import java.net.URL
import java.net.UnknownHostException
import javax.net.ssl.SSLException

// HttpException: a server response other than the expected one. The message keeps the technical detail.
class HttpException(val request: String, val code: Int) : IOException("$request: HTTP $code")

// Remote is the server, same protocol as the PC (client/remote.go): Bearer token, X-Client, GET /saves, conditional
// PUT. url: "http://…" or "https://…", never a fixed scheme; without one, http:// (withScheme). The connection gives
// up after 5 s (a home server away from home answers nothing); timeoutMs is the read timeout, long for big saves.
class Remote(url: String, private val token: String, private val client: String = "android", private val timeoutMs: Int = 30_000) {
    private val base = withScheme(url).trimEnd('/')

    // request returns the status and the body. 401 is an exception: no request can go on without the token.
    fun request(method: String, path: String, body: ByteArray? = null, headers: Map<String, String> = emptyMap()): Pair<Int, ByteArray> {
        val c = try {
            URL(base + path).openConnection() as HttpURLConnection
        } catch (e: IOException) {
            throw NetworkException(e)
        }
        try {
            c.requestMethod = method
            c.connectTimeout = minOf(timeoutMs, CONNECT_MS)
            c.readTimeout = timeoutMs
            c.instanceFollowRedirects = false
            c.setRequestProperty("Authorization", "Bearer $token")
            c.setRequestProperty("X-Client", client)
            headers.forEach { (k, v) -> c.setRequestProperty(k, v) }
            if (body != null) {
                c.doOutput = true
                c.setFixedLengthStreamingMode(body.size)
                c.outputStream.use { it.write(body) }
            }
            val st = c.responseCode
            if (st == 401) throw HttpException("$method $path", 401)
            val stream = if (st < 400) c.inputStream else c.errorStream
            return st to (stream?.use { it.readBytes() } ?: ByteArray(0))
        } catch (e: HttpException) {
            throw e
        } catch (e: IOException) { // cut-off response, timeout, no route: the network is at fault
            // Some HttpURLConnection versions throw on a 401 without a WWW-Authenticate header.
            if (e.message.orEmpty().contains("authentication challenge", ignoreCase = true)) throw HttpException("$method $path", 401)
            throw NetworkException(e)
        } finally {
            c.disconnect()
        }
    }

    // list returns the Current version of every Game (GET /saves).
    fun list(): List<Version> {
        val (st, o) = request("GET", "/saves")
        if (st != 200) throw HttpException("GET /saves", st)
        return try {
            parseList(String(o, Charsets.UTF_8))
        } catch (e: org.json.JSONException) { // Wi-Fi portal page, wrong address: unexpected response
            throw HttpException("GET /saves (not a list: ${e.message})", st)
        }
    }

    // put is a conditional Upload: If-Match = Current version seen, or If-None-Match: * when none. false on 412.
    fun put(key: String, sram: ByteArray, seen: Version?): Boolean {
        val cond = if (seen == null) mapOf("If-None-Match" to "*") else mapOf("If-Match" to "\"${seen.hash}\"")
        val (st, _) = request("PUT", "/saves/$key", sram, cond)
        if (st == 412) return false
        if (st != 200) throw HttpException("PUT /saves/$key", st)
        return true
    }

    fun get(key: String): ByteArray {
        val (st, o) = request("GET", "/saves/$key")
        if (st != 200) throw HttpException("GET /saves/$key", st)
        return o
    }

    // history returns the Versions of a Game on the server, newest first (the Current version first). A name the PC
    // would not accept is left out: it goes into the address of the next request.
    fun history(key: String): List<Version> {
        val (st, o) = request("GET", "/saves/$key/history")
        if (st != 200) throw HttpException("GET /saves/$key/history", st)
        return try {
            parseList(String(o, Charsets.UTF_8)).filter { versionName.matches(it.file) }.reversed()
        } catch (e: org.json.JSONException) {
            throw HttpException("GET /saves/$key/history (not a list: ${e.message})", st)
        }
    }

    // version reads one Version of the History.
    fun version(key: String, file: String): ByteArray {
        val (st, o) = request("GET", "/saves/$key/history/$file")
        if (st == 404) throw SyncException(Problem.VERSION_GONE, "GET /saves/$key/history/$file: HTTP 404")
        if (st != 200) throw HttpException("GET /saves/$key/history/$file", st)
        return o
    }

    // setName gives the game name to the server, for the History (ADR 0018). true if accepted.
    fun setName(key: String, name: String): Boolean = request("PUT", "/saves/$key/name", name.toByteArray(Charsets.UTF_8)).first == 204
}

const val CONNECT_MS = 5_000

private val versionName =Regex("^[0-9]{6}-[0-9]{8}T[0-9]{6}Z-[a-z0-9]{1,16}-[0-9a-f]{64}\\.sav$") // client/history.go

// RESTORE_CLIENT: X-Client of a Restore, so the History shows who put the Version back (client/history.go).
const val RESTORE_CLIENT = "restauration"

// restoreVersion makes an old Version the Current version again (ADR 0016): it reads it, checks its Hash, then uploads
// it with If-Match = the Current version the user saw, so an Upload that came meanwhile is never overwritten. Only the
// server changes: no file of this phone and no Base. remote: X-Client RESTORE_CLIENT.
fun restoreVersion(remote: Remote, key: String, v: Version, current: Version) {
    val o = remote.version(key, v.file)
    val h = sha256(o)
    if (h != v.hash) throw SyncException(Problem.VERSION_DAMAGED, "GET /saves/$key/history/${v.file}: SHA-256 ${h.take(12)}… instead of ${v.hash.take(12)}…")
    if (!remote.put(key, o, current)) throw SyncException(Problem.SERVER_CHANGED, "PUT /saves/$key: HTTP 412, the current version is no longer ${current.hash.take(12)}…")
}

// Problem is why an action failed, for the message to the user (the UI has one text per Problem).
enum class Problem {
    TOKEN, NOT_SERVER, TOO_LARGE, REJECTED, SERVER_ERROR, UNEXPECTED, // HTTP status
    CERTIFICATE, NO_HOST, REFUSED, NO_RESPONSE, // network
    VERSION_GONE, VERSION_DAMAGED, SERVER_CHANGED, SAVE_GONE, // History and Restore; one save gone from the server
    DAMAGED, BUSY, BACKUP, WRITE, RESTORE, NOT_WRITTEN, PARTIAL, NOT_STARTED, CHANGED, OPEN, FORMAT, PERMISSION, NO_COPY, // this phone
    NEVER_STARTED, EXTRA_FILES, ARCHIVE_PATH, // a 3DS save archive folder (ADR 0028)
    OTHER,
}

// problem classifies an error; global = the server or the network is at fault, so the next games would fail too.
fun problem(e: Throwable): Problem = when (e) {
    is HttpException -> when {
        e.code == 401 -> Problem.TOKEN
        // 404 on the list: not a 3dssavesync server; on one game: that save was removed from the server since the list
        e.code == 404 -> if (e.request == "GET /saves") Problem.NOT_SERVER else Problem.SAVE_GONE
        e.code == 413 -> Problem.TOO_LARGE
        e.code == 400 -> Problem.REJECTED
        e.code >= 500 -> Problem.SERVER_ERROR
        else -> Problem.UNEXPECTED
    }
    is SyncException -> e.problem
    is FormatException -> Problem.FORMAT
    is NetworkException -> when (e.cause) {
        is SSLException -> Problem.CERTIFICATE
        is UnknownHostException -> Problem.NO_HOST
        is ConnectException -> Problem.REFUSED
        else -> Problem.NO_RESPONSE
    }
    is SecurityException, is FileNotFoundException -> Problem.PERMISSION
    else -> Problem.OTHER
}

// NetworkException: the request got no full response (DNS, refused, timeout, TLS, cut connection).
class NetworkException(cause: IOException) : IOException(cause.toString(), cause)

fun isGlobal(p: Problem) = p.ordinal <= Problem.NO_RESPONSE.ordinal && p != Problem.TOO_LARGE && p != Problem.REJECTED

// SyncException: a failure found by this app, with its Problem and its technical detail.
class SyncException(val problem: Problem, detail: String) : Exception(detail)
