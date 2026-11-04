package io.github.programgames.savesync

import android.content.Context
import androidx.core.content.edit
import io.github.programgames.savesync.core.Format
import io.github.programgames.savesync.core.System
import org.json.JSONArray
import org.json.JSONObject

// Folders of one System in an emulator entry: tree URIs chosen by the user. nextToRom: the saves sit next to each
// ROM (in the ROM folder) instead of a save folder.
data class Folders(
    val roms: String? = null,
    val saves: String? = null,
    val nextToRom: Boolean = false,
    val variant: Int = 0,
    val ext: String = ".sav", // custom Profile only
)

// Emulator is one entry of the Emulators screen. id: owns the Bases and the folder of the Backup copies; it never
// changes once the entry exists.
data class Emulator(val id: String, val profile: String, val label: String, val systems: Map<System, Folders>) {
    fun profile() = profile(profile)

    fun format(s: System): Format {
        val f = systems.getValue(s)
        if (profile == CUSTOM.id) {
            val ext = f.ext.trim().let { if (it.startsWith(".")) it else ".$it" }
            return Format(ext, dsvFooter = ext.equals(".dsv", ignoreCase = true)) // a new .dsv gets the DeSmuME footer
        }
        return variant(s).format
    }

    // variant: the core chosen for a System (the first one if the Profile has fewer now).
    fun variant(s: System): Variant {
        val v = profile().systems.getValue(s)
        return v[systems.getValue(s).variant.coerceIn(v.indices)]
    }
}

// Settings: server, token and emulators, in the private preferences of the app.
class Settings(context: Context) {
    private val p = context.getSharedPreferences("settings", Context.MODE_PRIVATE)

    var url: String
        get() = p.getString("url", "") ?: ""
        set(v) = p.edit { putString("url", v.trim()) }

    var token: String
        get() = p.getString("token", "") ?: ""
        set(v) = p.edit { putString("token", v.trim()) }

    // noReminder: the user chose "Don't show again" on the reminder to close the emulator.
    var noReminder: Boolean
        get() = p.getBoolean("noReminder", false)
        set(v) = p.edit { putBoolean("noReminder", v) }

    // background: the user turned on the background upload (Background.kt). Off by default.
    var background: Boolean
        get() = p.getBoolean("background", false)
        set(v) = p.edit { putBoolean("background", v) }

    // lastRun: the last background pass. notified: the problems of the last notification, so the same ones are not
    // notified again.
    var lastRun: LastRun?
        get() = if (p.contains("lastRunTime")) LastRun(p.getLong("lastRunTime", 0), p.getInt("lastRunUploaded", 0), p.getString("lastRunProblems", "") ?: "") else null
        set(v) = p.edit {
            if (v == null) remove("lastRunTime") else putLong("lastRunTime", v.time).putInt("lastRunUploaded", v.uploaded).putString("lastRunProblems", v.problems)
        }

    var notified: String
        get() = p.getString("notified", "") ?: ""
        set(v) = p.edit { putString("notified", v) }

    // aliases: ROM name without extension -> Key of its own save (ADR 0014), as "alias" in the PC config.json.
    var aliases: Map<String, String>
        get() = JSONObject(p.getString("aliases", "{}") ?: "{}").let { o -> o.keys().asSequence().associateWith { o.getString(it) } }
        set(v) = p.edit { putString("aliases", JSONObject(v).toString()) }

    var emulators: List<Emulator>
        get() {
            val a = JSONArray(p.getString("emulators", "[]"))
            return (0 until a.length()).map { i ->
                val o = a.getJSONObject(i)
                val s = o.getJSONObject("systems")
                Emulator(
                    o.getString("id"), o.getString("profile"), o.optString("label"),
                    s.keys().asSequence().associate { k ->
                        val f = s.getJSONObject(k)
                        System.valueOf(k) to Folders(
                            f.optString("roms").ifEmpty { null }, f.optString("saves").ifEmpty { null },
                            f.optBoolean("nextToRom"), f.optInt("variant"), f.optString("ext", ".sav"),
                        )
                    },
                )
            }
        }
        set(list) {
            val a = JSONArray()
            for (e in list) {
                val s = JSONObject()
                for ((k, f) in e.systems) {
                    s.put(k.name, JSONObject().put("roms", f.roms ?: "").put("saves", f.saves ?: "").put("nextToRom", f.nextToRom).put("variant", f.variant).put("ext", f.ext))
                }
                a.put(JSONObject().put("id", e.id).put("profile", e.profile).put("label", e.label).put("systems", s))
            }
            p.edit { putString("emulators", a.toString()) }
        }
}
