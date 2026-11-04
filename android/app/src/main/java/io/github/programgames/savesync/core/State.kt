package io.github.programgames.savesync.core

import org.json.JSONArray

// State of a Game (docs/design.md §1, ADR 0001). fixture = the word of tests/cas_etat.csv.
enum class State(val fixture: String) {
    IN_SYNC("A_JOUR"),
    TO_UPLOAD("A_ENVOYER"),
    TO_DOWNLOAD("A_RECUPERER"),
    CONFLICT("CONFLIT"),
}

// Version is a Save as the server or the phone sees it (Hash and size of the SRAM). Same fields as GET /saves.
data class Version(
    val hash: String,
    val size: Long,
    val code: String = "",
    val client: String = "",
    val date: String = "",
    val age: Long? = null,
    val name: String = "", // game name given by a Client (ADR 0018); empty on an older server
    val file: String = "", // the server's name of this Version: <seq>-<date>-<client>-<hash>.sav
) {
    val number get() = file.substringBefore('-').toIntOrNull() ?: 0 // <seq>, never renumbered by the server
}

// state compares Local save, Current version and Base. null = absent; null or empty base = never synced.
// Same rules, same order as client/sync.go state() and 3ds/source/state.c.
fun state(local: Version?, server: Version?, base: String?): State = when {
    local != null && server != null && local.hash == server.hash -> State.IN_SYNC
    server == null -> State.TO_UPLOAD
    local == null -> State.TO_DOWNLOAD
    local.size != server.size -> State.CONFLICT
    local.hash == base -> State.TO_DOWNLOAD
    server.hash == base -> State.TO_UPLOAD
    else -> State.CONFLICT
}

// parseList reads the GET /saves list (tests/saves.json). A missing field is empty, never read from another game.
fun parseList(json: String): List<Version> {
    val a = JSONArray(json)
    return (0 until a.length()).map {
        val o = a.getJSONObject(it)
        Version(
            hash = o.optString("hash"),
            size = o.optLong("taille"),
            code = o.optString("code"),
            client = o.optString("client"),
            date = o.optString("date"),
            age = if (o.has("age")) o.getLong("age") else null,
            name = o.optString("nom"),
            file = o.optString("version"),
        )
    }
}
