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
        val v = profile().systems.getValue(s)
        return v[f.variant.coerceIn(v.indices)].format
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
