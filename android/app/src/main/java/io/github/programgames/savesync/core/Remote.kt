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
// PUT. url: "http://…" or "https://…", never a fixed scheme.
class Remote(url: String, private val token: String, private val client: String = "android", private val timeoutMs: Int = 30_000) {
    private val base = url.trim().trimEnd('/')

    // request returns the status and the body. 401 is an exception: no request can go on without the token.
    fun request(method: String, path: String, body: ByteArray? = null, headers: Map<String, String> = emptyMap()): Pair<Int, ByteArray> {
        val c = try {
            URL(base + path).openConnection() as HttpURLConnection
        } catch (e: IOException) {
            throw NetworkException(e)
        }
        try {
            c.requestMethod = method
            c.connectTimeout = timeoutMs
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

    // setName gives the game name to the server, for the History (ADR 0018). true if accepted.
    fun setName(key: String, name: String): Boolean = request("PUT", "/saves/$key/name", name.toByteArray(Charsets.UTF_8)).first == 204
}

// Problem is why an action failed, for the message to the user (the UI has one text per Problem).
enum class Problem {
    TOKEN, NOT_SERVER, TOO_LARGE, REJECTED, SERVER_ERROR, UNEXPECTED, // HTTP status
    CERTIFICATE, NO_HOST, REFUSED, NO_RESPONSE, // network
    DAMAGED, BUSY, BACKUP, WRITE, RESTORE, FORMAT, PERMISSION, OTHER, // this phone
}

// problem classifies an error; global = the server or the network is at fault, so the next games would fail too.
fun problem(e: Throwable): Problem = when (e) {
    is HttpException -> when {
        e.code == 401 -> Problem.TOKEN
        e.code == 404 -> Problem.NOT_SERVER
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
