package io.github.programgames.savesync

import io.github.programgames.savesync.core.Format
import io.github.programgames.savesync.core.System

// Emulator Profiles: what the app knows about an Android emulator, as data. Each System lists its save Variants (a
// RetroArch core, for instance); the first one is the default. Only open-source emulators whose save folder the
// Android folder picker can open are built in (docs/guide-android.md); the others go through the custom Profile.

data class Variant(val label: String, val format: Format)

data class Profile(
    val id: String,
    val name: String,
    val systems: Map<System, List<Variant>>,
    val hint: Int, // string resource: where this emulator keeps its saves
    val nextToRom: Boolean = false, // default: the saves sit next to the ROMs
)

private fun one(ext: String) = listOf(Variant("", Format(ext)))

val CUSTOM = Profile("custom", "", mapOf(System.DS to one(".sav"), System.GB to one(".sav"), System.GBA to one(".sav")), R.string.hint_custom)

val profiles = listOf(
    Profile(
        "retroarch", "RetroArch",
        mapOf(
            // RetroArch cores, as on the PC (client/profiles.go): .srm, except DeSmuME (.dsv with its footer).
            System.DS to listOf(Variant("melonDS DS", Format(".srm")), Variant("DeSmuME", Format(".dsv", dsvFooter = true))),
            System.GB to listOf(
                Variant("Gambatte", Format(".srm")), Variant("SameBoy", Format(".srm")),
                Variant("mGBA", Format(".srm", mbc2 = 256)), Variant("VBA-M", Format(".srm")),
            ),
            System.GBA to listOf(
                Variant("mGBA", Format(".srm")), Variant("gpSP", Format(".srm", "gpSP")),
                Variant("Beetle GBA", Format(".srm", "Beetle GBA")), Variant("VBA-M", Format(".srm")),
            ),
        ),
        R.string.hint_retroarch,
    ),
    Profile("azahar", "Azahar", mapOf(System.N3DS to one("")), R.string.hint_azahar),
    Profile("melonds", "melonDS", mapOf(System.DS to one(".sav")), R.string.hint_melonds, nextToRom = true),
    Profile("noods", "NooDS", mapOf(System.DS to one(".sav"), System.GBA to one(".sav")), R.string.hint_noods, nextToRom = true),
    CUSTOM,
)

fun profile(id: String) = profiles.find { it.id == id } ?: CUSTOM
