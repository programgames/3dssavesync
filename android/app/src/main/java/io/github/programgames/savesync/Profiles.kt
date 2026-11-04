package io.github.programgames.savesync

import android.content.pm.PackageManager
import io.github.programgames.savesync.core.Format
import io.github.programgames.savesync.core.System

// Emulator Profiles: what the app knows about an Android emulator, as data. Each System lists its save Variants (a
// RetroArch core, for instance); the first one is the default. The custom Profile covers the others
// (docs/guide-android.md).

data class Variant(val label: String, val format: Format)

data class Profile(
    val id: String,
    val name: String,
    val systems: Map<System, List<Variant>>,
    val hint: Int, // string resource: where this emulator keeps its saves
    val nextToRom: Boolean = false, // default: the saves sit next to the ROMs
    // Android packages of the emulator: the installed ones come first when the user adds an emulator. Each one must be
    // in the <queries> of AndroidManifest.xml, else Android 11+ hides it.
    val packages: List<String> = emptyList(),
    val savesAt: String? = null, // its usual save folder in the internal storage: the folder picker opens there
)

private fun one(ext: String) = listOf(Variant("", Format(ext)))

// RETROARCH: one save folder for the whole entry; the scan finds each save in it, in the folder of the core, or next to
// the ROM (Model.scan).
const val RETROARCH = "retroarch"

val CUSTOM = Profile("custom", "", mapOf(System.DS to one(".sav"), System.GB to one(".sav"), System.GBA to one(".sav")), R.string.hint_custom)

val profiles = listOf(
    Profile(
        RETROARCH, "RetroArch",
        mapOf(
            // RetroArch cores, as on the PC (client/profiles.go): .srm, except DeSmuME (.dsv with its footer). The label
            // is also the name of the core's folder in RetroArch/saves.
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
        packages = listOf("com.retroarch", "com.retroarch.aarch64", "com.retroarch.ra32"),
        savesAt = "RetroArch/saves",
    ),
    Profile("melonds", "melonDS", mapOf(System.DS to one(".sav")), R.string.hint_melonds, nextToRom = true, packages = listOf("me.magnum.melonds", "me.magnum.melondualds")),
    // DraStic: DraStic/backup/<ROM name>.dsv; a new file gets the DeSmuME footer, an existing one keeps its own form.
    Profile(
        "drastic", "DraStic", mapOf(System.DS to listOf(Variant("", Format(".dsv", dsvFooter = true)))), R.string.hint_drastic,
        packages = listOf("com.dsemu.drastic"), savesAt = "DraStic/backup",
    ),
    Profile("noods", "NooDS", mapOf(System.DS to one(".sav"), System.GBA to one(".sav")), R.string.hint_noods, nextToRom = true, packages = listOf("com.hydra.noods")),
    // Pizza Boy keeps its data in Android/data (closed to other apps since Android 11): the hint says so. .sav is not
    // checked on a real phone yet.
    Profile(
        "pizzaboy", "Pizza Boy", mapOf(System.GB to one(".sav"), System.GBA to one(".sav")), R.string.hint_pizzaboy,
        packages = listOf("it.dbtecno.pizzaboy", "it.dbtecno.pizzaboypro", "it.dbtecno.pizzaboygba", "it.dbtecno.pizzaboygbapro"),
    ),
    Profile("azahar", "Azahar", mapOf(System.N3DS to one("")), R.string.hint_azahar, packages = listOf("org.azahar_emu.azahar", "io.github.lime3ds.android")),
    CUSTOM,
)

fun profile(id: String) = profiles.find { it.id == id } ?: CUSTOM

@Suppress("DEPRECATION") // getPackageInfo(String, Int): the only form before Android 13
fun Profile.installed(pm: PackageManager) = packages.any { runCatching { pm.getPackageInfo(it, 0) }.isSuccess }

// romsAt: the usual ROM folders of a System in the internal storage, for the folder picker.
fun romsAt(s: System) = listOf("roms/" + mapOf(System.DS to "nds", System.GB to "gb", System.GBA to "gba", System.N3DS to "3ds").getValue(s), "roms")
