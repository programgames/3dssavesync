package io.github.programgames.savesync.core

import java.net.URI
import java.net.URLDecoder

// withScheme adds http:// to an address typed without a scheme ("192.168.1.20:8080"): a home server, the usual case.
fun withScheme(url: String): String {
    val u = url.trim()
    return if (u.isEmpty() || Regex("^https?://", RegexOption.IGNORE_CASE).containsMatchIn(u)) u else "http://$u"
}

// Pairing is the server read from the QR code of the PC settings page (client/setup_web.go, phonePayload).
data class Pairing(val url: String, val token: String)

private const val PAIRING_PREFIX = "3dssavesync://serveur?"

// parsePairing reads 3dssavesync://serveur?adresse=<address>&jeton=<token>, both URL-encoded. null: another QR code,
// no token, or an address the app can't use. By hand: java.net.URI refuses a scheme that starts with a digit.
fun parsePairing(text: String): Pairing? {
    val t = text.trim()
    if (!t.startsWith(PAIRING_PREFIX)) return null
    val q = runCatching {
        t.removePrefix(PAIRING_PREFIX).split('&').associate { it.substringBefore('=') to URLDecoder.decode(it.substringAfter('=', ""), "UTF-8") }
    }.getOrNull() ?: return null // bad %-escape
    val url = q["adresse"].orEmpty().trim()
    val token = q["jeton"].orEmpty().trim()
    val u = runCatching { URI(url) }.getOrNull()
    if (token.isEmpty() || u == null || u.scheme !in setOf("http", "https") || u.authority.isNullOrEmpty()) return null
    return Pairing(url, token)
}
