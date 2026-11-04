@file:OptIn(ExperimentalMaterial3Api::class)

package io.github.programgames.savesync

import android.Manifest
import android.content.Context
import android.net.Uri
import androidx.compose.ui.graphics.Color
import com.google.mlkit.vision.barcode.common.Barcode
import com.google.mlkit.vision.codescanner.GmsBarcodeScannerOptions
import com.google.mlkit.vision.codescanner.GmsBarcodeScanning
import io.github.programgames.savesync.core.parsePairing
import android.os.Build
import android.provider.OpenableColumns
import androidx.compose.foundation.selection.toggleable
import androidx.compose.material3.Switch
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.semantics.Role
import androidx.core.app.NotificationManagerCompat
import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Build
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.Checkbox
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.core.net.toUri
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.unit.dp
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Surface
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.foundation.layout.heightIn
import androidx.compose.runtime.rememberCoroutineScope
import kotlinx.coroutines.launch
import io.github.programgames.savesync.core.Problem
import io.github.programgames.savesync.core.System
import io.github.programgames.savesync.core.aliasExamples
import io.github.programgames.savesync.core.aliasSystem
import io.github.programgames.savesync.core.extension
import io.github.programgames.savesync.core.extensions
import io.github.programgames.savesync.core.withoutExtension
import io.github.programgames.savesync.core.problem
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

enum class Tab { GAMES, EMULATORS, SETTINGS }

@Composable
fun Root() {
    var tab by rememberSaveable { mutableStateOf(if (App.configured()) Tab.GAMES else Tab.SETTINGS) }
    // the open game, by its list key: it stays open when the phone rotates, and shows the entry as it is now (an action
    // reads its State again)
    var open by rememberSaveable { mutableStateOf<String?>(null) }
    var editing by rememberSaveable { mutableStateOf<String?>(null) }
    var hacks by rememberSaveable { mutableStateOf(false) }
    val o = App.entries.find { it.id == open }
    val ed = editing
    when {
        o != null -> {
            val close = {
                open = null
                App.message = null
            }
            BackHandler(onBack = close)
            DetailsScreen(o, close)
        }
        ed != null && App.emulators.any { it.id == ed } -> {
            BackHandler { editing = null }
            EmulatorScreen(ed) { editing = null }
        }
        hacks -> {
            BackHandler { hacks = false }
            HacksScreen { hacks = false }
        }
        else -> Scaffold(
            bottomBar = {
                NavigationBar {
                    for ((t, icon, label) in listOf(
                        Triple(Tab.GAMES, Icons.Filled.PlayArrow, R.string.tab_games),
                        Triple(Tab.EMULATORS, Icons.Filled.Build, R.string.tab_emulators),
                        Triple(Tab.SETTINGS, Icons.Filled.Settings, R.string.tab_settings),
                    )) {
                        NavigationBarItem(selected = tab == t, onClick = { tab = t }, icon = { Icon(icon, null) }, label = { Text(stringResource(label)) })
                    }
                }
            },
        ) { p ->
            when (tab) {
                Tab.GAMES -> GamesScreen(p, { open = it.id }, { tab = Tab.SETTINGS }, { tab = Tab.EMULATORS })
                Tab.EMULATORS -> EmulatorsScreen(p) { editing = it }
                Tab.SETTINGS -> SettingsScreen(p) { hacks = true }
            }
        }
    }
}

// problemText: what happened and what to do, for each Problem.
fun problemText(p: Problem) = when (p) {
    Problem.TOKEN -> R.string.p_token
    Problem.NOT_SERVER -> R.string.p_not_server
    Problem.TOO_LARGE -> R.string.p_too_large
    Problem.REJECTED -> R.string.p_rejected
    Problem.SERVER_ERROR -> R.string.p_server_error
    Problem.UNEXPECTED -> R.string.p_unexpected
    Problem.CERTIFICATE -> R.string.p_certificate
    Problem.NO_HOST -> R.string.p_no_host
    Problem.REFUSED -> R.string.p_refused
    Problem.NO_RESPONSE -> R.string.p_no_response
    Problem.VERSION_GONE -> R.string.p_version_gone
    Problem.VERSION_DAMAGED -> R.string.p_version_damaged
    Problem.SERVER_CHANGED -> R.string.p_server_changed
    Problem.DAMAGED -> R.string.p_damaged
    Problem.BUSY -> R.string.p_busy
    Problem.BACKUP -> R.string.p_backup
    Problem.WRITE -> R.string.p_write
    Problem.RESTORE -> R.string.p_restore
    Problem.NOT_WRITTEN -> R.string.p_not_written
    Problem.PARTIAL -> R.string.p_partial
    Problem.NOT_STARTED -> R.string.p_not_started
    Problem.CHANGED -> R.string.p_changed
    Problem.OPEN -> R.string.p_open
    Problem.FORMAT -> R.string.p_format
    Problem.PERMISSION -> R.string.p_permission
    Problem.NO_COPY -> R.string.p_no_copy
    Problem.OTHER -> R.string.p_other
}

// ErrorBox: what happened and what to do in plain words, Retry when the action can run again, and the technical detail
// (exception, address, HTTP code) only behind "Show details", to report it. lead: which action failed; note: one more
// line about the screen (hidden games).
@Composable
fun ErrorBox(t: Throwable, lead: String? = null, note: String? = null, retry: (() -> Unit)? = null) {
    var details by rememberSaveable { mutableStateOf(false) }
    val c = MaterialTheme.colorScheme.onErrorContainer
    Surface(Modifier.fillMaxWidth().padding(vertical = 4.dp), MaterialTheme.shapes.medium, MaterialTheme.colorScheme.errorContainer) {
        Column(Modifier.padding(start = 12.dp, end = 4.dp, top = 8.dp).semantics { liveRegion = LiveRegionMode.Polite }) {
            if (lead != null) Text(lead, Modifier.padding(end = 8.dp), color = c, fontWeight = FontWeight.Bold)
            Text(stringResource(problemText(problem(t))), Modifier.padding(end = 8.dp), color = c)
            note?.let { Text(it, Modifier.padding(top = 4.dp, end = 8.dp), color = c, style = MaterialTheme.typography.bodySmall) }
            FlowRow {
                val colors = ButtonDefaults.textButtonColors(contentColor = c)
                retry?.let { TextButton(it, colors = colors) { Text(stringResource(R.string.retry)) } }
                TextButton({ details = !details }, colors = colors) { Text(stringResource(if (details) R.string.hide_details else R.string.show_details)) }
            }
            if (details) {
                SelectionContainer {
                    Text(t.message ?: t.toString(), Modifier.padding(end = 8.dp, bottom = 8.dp), color = c, style = MaterialTheme.typography.bodySmall, fontFamily = FontFamily.Monospace)
                }
            }
        }
    }
}

@Composable
fun Title(text: String) = Text(text, style = MaterialTheme.typography.titleLarge, modifier = Modifier.padding(bottom = 8.dp))

// scanQr opens Google code scanner (QR codes only, on the phone, no CAMERA permission). done: the text, null when the
// user cancels, or the error (no Google Play services, scanner module not downloaded yet). Tests replace it.
var scanQr: (Context, (Result<String?>) -> Unit) -> Unit = { ctx, done ->
    runCatching {
        GmsBarcodeScanning.getClient(ctx, GmsBarcodeScannerOptions.Builder().setBarcodeFormats(Barcode.FORMAT_QR_CODE).build())
            .startScan()
            .addOnSuccessListener { done(Result.success(it.rawValue.orEmpty())) }
            .addOnCanceledListener { done(Result.success(null)) }
            .addOnFailureListener { done(Result.failure(it)) }
    }.onFailure { done(Result.failure(it)) }
}

@Composable
fun SettingsScreen(p: PaddingValues, onHacks: () -> Unit) {
    val ctx = LocalContext.current
    var url by rememberSaveable { mutableStateOf(App.settings.url) }
    var token by rememberSaveable { mutableStateOf(App.settings.token) }
    var show by remember { mutableStateOf(false) }
    var result by remember { mutableStateOf<Result<Int>?>(null) }
    var scanned by remember { mutableStateOf<Int?>(null) } // why the last scan filled nothing
    Column(Modifier.padding(p).verticalScroll(rememberScrollState()).padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Title(stringResource(R.string.settings_title))
        Button({
            scanned = null
            scanQr(ctx) { r ->
                val pairing = r.getOrNull()?.let(::parsePairing)
                scanned = when {
                    r.isFailure -> R.string.scan_unavailable
                    r.getOrNull() == null -> R.string.scan_cancelled
                    pairing == null -> R.string.scan_not_ours
                    else -> null
                }
                if (pairing != null) { // the token is never logged
                    url = pairing.url
                    token = pairing.token
                    App.settings.url = pairing.url
                    App.settings.token = pairing.token
                    App.serverChanged()
                    result = null
                    App.testConnection { result = it }
                }
            }
        }, enabled = App.busy == null) { Text(stringResource(R.string.scan_qr)) }
        Text(stringResource(scanned ?: R.string.scan_help), style = MaterialTheme.typography.bodySmall,
            color = if (scanned == null) Color.Unspecified else MaterialTheme.colorScheme.error)
        OutlinedTextField(
            url, {
                url = it
                App.settings.url = it
                App.serverChanged()
                result = null
            },
            Modifier.fillMaxWidth(), label = { Text(stringResource(R.string.server_address)) },
            placeholder = { Text("192.168.1.20:8080") }, supportingText = { Text(stringResource(R.string.server_address_help)) },
            singleLine = true, keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri),
        )
        OutlinedTextField(
            token, {
                token = it
                App.settings.token = it
                App.serverChanged()
                result = null
            },
            Modifier.fillMaxWidth(), label = { Text(stringResource(R.string.token)) }, singleLine = true,
            visualTransformation = if (show) VisualTransformation.None else PasswordVisualTransformation(),
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password),
            trailingIcon = { TextButton({ show = !show }) { Text(stringResource(if (show) R.string.hide else R.string.show)) } },
        )
        Button({ App.testConnection { result = it } }, enabled = App.busy == null && url.isNotBlank() && token.isNotBlank()) {
            Text(stringResource(R.string.test_connection))
        }
        App.busy?.let { Text(it) }
        result?.fold({ Text(pluralStringResource(R.plurals.connected, it, it)) }, { ErrorBox(it, retry = { App.testConnection { r -> result = r } }) })
        HorizontalDivider()
        BackgroundSection()
        HorizontalDivider()
        Text(stringResource(R.string.backups_title), style = MaterialTheme.typography.titleMedium)
        Text(stringResource(R.string.backups_where))
        Text(App.backupsDir.path, style = MaterialTheme.typography.bodySmall, fontFamily = FontFamily.Monospace)
        HorizontalDivider()
        Text(stringResource(R.string.hacks_title), style = MaterialTheme.typography.titleMedium)
        Text(stringResource(R.string.hacks_text))
        OutlinedButton(onHacks) { Text(stringResource(R.string.hacks_title)) }
        HorizontalDivider()
        Text(stringResource(R.string.version, BuildConfig.VERSION_NAME), style = MaterialTheme.typography.bodySmall)
    }
}

// BackgroundSection: the background upload switch (off by default) and the last run. Turning it on asks for the
// notifications (Android 13+); refused, the upload still runs and its problems show here only.
@Composable
private fun BackgroundSection() {
    val ctx = LocalContext.current
    fun notificationsOn() = NotificationManagerCompat.from(ctx).areNotificationsEnabled()
    var on by remember { mutableStateOf(App.settings.background) }
    var notifications by remember { mutableStateOf(notificationsOn()) }
    val ask = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { notifications = notificationsOn() }
    Text(stringResource(R.string.bg_title), style = MaterialTheme.typography.titleMedium)
    Row(
        Modifier.fillMaxWidth().toggleable(on, role = Role.Switch) { v ->
            on = v
            App.setBackground(v)
            if (v && Build.VERSION.SDK_INT >= 33 && !notificationsOn()) ask.launch(Manifest.permission.POST_NOTIFICATIONS)
        },
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(stringResource(R.string.bg_switch), Modifier.weight(1f).padding(end = 8.dp))
        Switch(on, null)
    }
    Text(stringResource(R.string.bg_help), style = MaterialTheme.typography.bodySmall)
    if (on && !notifications) Text(stringResource(R.string.bg_no_notification), style = MaterialTheme.typography.bodySmall)
    val last = App.lastRun
    Text(if (last == null) stringResource(R.string.bg_never) else pluralStringResource(R.plurals.bg_last, last.uploaded, dateText(last.time), last.uploaded))
    if (last != null && last.problems.isNotEmpty()) Text(last.problems, color = MaterialTheme.colorScheme.error)
}

// HacksScreen lists the ROM hacks that have their own save (ADR 0014): add one by its ROM file, or forget one (the
// mapping only, never a save). The Key comes from the ROM file name, as on the PC.
@Composable
fun HacksScreen(onBack: () -> Unit) {
    var picked by rememberSaveable { mutableStateOf<String?>(null) } // ROM file name
    var system by rememberSaveable { mutableStateOf<System?>(null) }
    var error by remember { mutableStateOf<String?>(null) }
    var removing by remember { mutableStateOf<String?>(null) }
    val picker = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        if (uri == null) return@rememberLauncherForActivityResult
        val name = runCatching {
            App.resolver.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME), null, null, null)?.use { c -> if (c.moveToFirst()) c.getString(0) else null }
        }.getOrNull() ?: uri.lastPathSegment.orEmpty().substringAfterLast('/').substringAfterLast(':')
        picked = name
        system = extensions[extension(name)] ?: system
        error = null
    }
    Scaffold(topBar = {
        TopAppBar(title = { Text(stringResource(R.string.hacks_title)) }, navigationIcon = {
            IconButton(onBack) { Icon(Icons.AutoMirrored.Filled.ArrowBack, stringResource(R.string.back)) }
        })
    }) { p ->
        Column(Modifier.padding(p).verticalScroll(rememberScrollState()).padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(stringResource(R.string.hacks_text))
            Text(stringResource(R.string.hacks_same_key), style = MaterialTheme.typography.bodySmall)
            HorizontalDivider(Modifier.padding(vertical = 8.dp))
            if (App.aliases.isEmpty()) Text(stringResource(R.string.no_hack))
            for ((name, key) in App.aliases.toSortedMap(String.CASE_INSENSITIVE_ORDER)) {
                Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                    Column(Modifier.weight(1f)) {
                        Text(name)
                        Text(systemName(aliasSystem(key)), style = MaterialTheme.typography.bodySmall)
                    }
                    TextButton({ removing = name }) { Text(stringResource(R.string.remove)) }
                }
            }
            HorizontalDivider(Modifier.padding(vertical = 8.dp))
            OutlinedButton({ picker.launch(arrayOf("*/*")) }) { Text(stringResource(R.string.hack_choose_rom)) }
            Text(picked ?: stringResource(R.string.hack_no_file), style = MaterialTheme.typography.bodySmall)
            FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                for (s in aliasExamples.keys) FilterChip(system == s, { system = s }, { Text(s.label) })
            }
            val name = picked?.let { withoutExtension(it) }
            Button({
                error = App.addAlias(system!!, name!!)
                if (error == null) picked = null
            }, enabled = name != null && system != null) { Text(stringResource(R.string.hack_add)) }
            error?.let { Text(it, color = MaterialTheme.colorScheme.error) }
        }
    }
    removing?.let { name ->
        AlertDialog(
            onDismissRequest = { removing = null },
            title = { Text(stringResource(R.string.hacks_title)) },
            text = { Text(stringResource(R.string.hack_remove_text, name)) },
            confirmButton = {
                TextButton({
                    removing = null
                    App.removeAlias(name)
                }) { Text(stringResource(R.string.remove)) }
            },
            dismissButton = { TextButton({ removing = null }) { Text(stringResource(R.string.cancel)) } },
        )
    }
}

enum class FolderState { NOT_SET, READY, MISSING, NO_PERMISSION, CHECKING }

private fun check(uri: String?): FolderState {
    if (uri == null) return FolderState.NOT_SET
    val u = uri.toUri()
    if (App.resolver.persistedUriPermissions.none { it.uri == u && it.isReadPermission && it.isWritePermission }) return FolderState.NO_PERMISSION
    return if (runCatching { children(App.resolver, u, rootId(u)) }.isSuccess) FolderState.READY else FolderState.MISSING
}

// emulatorState: the worst state of its folders. The saves folder of RetroArch is optional (saves next to the ROMs).
private fun emulatorState(e: Emulator): FolderState {
    val all = e.systems.flatMap { (s, f) ->
        listOf(
            if (s == System.N3DS) FolderState.READY else check(f.roms),
            if (f.nextToRom || e.profile == RETROARCH && f.saves == null) FolderState.READY else check(f.saves),
        )
    }
    return when {
        all.isEmpty() -> FolderState.NOT_SET
        FolderState.NO_PERMISSION in all -> FolderState.NO_PERMISSION
        FolderState.MISSING in all -> FolderState.MISSING
        FolderState.NOT_SET in all -> FolderState.NOT_SET
        else -> FolderState.READY
    }
}

@Composable
private fun stateLabel(s: FolderState) = stringResource(
    when (s) {
        FolderState.READY -> R.string.folder_ready
        FolderState.MISSING -> R.string.folder_missing
        FolderState.NO_PERMISSION -> R.string.folder_no_permission
        FolderState.NOT_SET -> R.string.folder_not_set
        FolderState.CHECKING -> R.string.folder_checking
    },
)

// StateLine: Ready in green, not chosen yet in grey (a step to do, not an error), the problems in red.
@Composable
private fun StateLine(s: FolderState) {
    Text(
        stateLabel(s),
        color = when (s) {
            FolderState.READY -> stateColor(Shown.IN_SYNC)
            FolderState.NOT_SET, FolderState.CHECKING -> MaterialTheme.colorScheme.onSurfaceVariant
            else -> MaterialTheme.colorScheme.error
        },
    )
}

fun Emulator.title() = label.ifBlank { profile().name.ifBlank { App.text(R.string.custom_profile) } }

@Composable
fun EmulatorsScreen(p: PaddingValues, onEdit: (String) -> Unit) {
    var adding by remember { mutableStateOf(false) }
    Column(Modifier.padding(p).verticalScroll(rememberScrollState()).padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Title(stringResource(R.string.emulators_title))
        if (App.emulators.isEmpty()) Text(stringResource(R.string.no_emulator))
        for (e in App.emulators) {
            val st = produceState(FolderState.CHECKING, e) { value = withContext(Dispatchers.IO) { emulatorState(e) } }.value
            Card({ onEdit(e.id) }, Modifier.fillMaxWidth()) {
                Column(Modifier.padding(16.dp)) {
                    Text(e.title(), style = MaterialTheme.typography.titleMedium)
                    Text(e.systems.keys.joinToString(" · ") { it.label })
                    StateLine(st)
                }
            }
        }
        Button({ adding = true }) { Text(stringResource(R.string.add_emulator)) }
        Text(stringResource(R.string.emulators_help), style = MaterialTheme.typography.bodySmall)
    }
    if (adding) {
        AlertDialog(
            onDismissRequest = { adding = false },
            title = { Text(stringResource(R.string.add_emulator)) },
            text = {
                Column(Modifier.verticalScroll(rememberScrollState())) {
                    ProfileList {
                        adding = false
                        onEdit(addEmulator(it))
                    }
                }
            },
            confirmButton = {},
            dismissButton = { TextButton({ adding = false }) { Text(stringResource(R.string.cancel)) } },
        )
    }
}

// addEmulator adds an entry of this Profile, with every System of it; returns its id.
fun addEmulator(pr: Profile): String {
    val e = Emulator(App.newId(pr.id), pr.id, if (pr == CUSTOM) App.text(R.string.custom_profile) else pr.name, pr.systems.keys.associateWith { Folders(nextToRom = pr.nextToRom) })
    App.saveEmulators(App.emulators + e)
    return e.id
}

// ProfileList: the emulators installed on this phone first, then the others.
@Composable
fun ProfileList(onPick: (Profile) -> Unit) {
    val pm = LocalContext.current.packageManager
    val (installed, others) = remember { profiles.partition { it.installed(pm) } }
    @Composable
    fun group(title: Int?, list: List<Profile>) {
        if (list.isEmpty()) return
        if (title != null) Text(stringResource(title), Modifier.padding(top = 8.dp).semantics { heading() }, style = MaterialTheme.typography.titleSmall)
        for (pr in list) {
            TextButton({ onPick(pr) }, Modifier.fillMaxWidth()) {
                Text(if (pr == CUSTOM) stringResource(R.string.custom_profile) else pr.name, Modifier.fillMaxWidth())
            }
        }
    }
    group(R.string.installed_emulators, installed)
    group(if (installed.isEmpty()) null else R.string.other_emulators, others)
}

@Composable
fun EmulatorScreen(id: String, onBack: () -> Unit) {
    val e = App.emulators.firstOrNull { it.id == id } ?: return
    var removing by remember { mutableStateOf(false) }
    Scaffold(topBar = {
        TopAppBar(title = { Text(e.title()) }, navigationIcon = {
            IconButton(onBack) { Icon(Icons.AutoMirrored.Filled.ArrowBack, stringResource(R.string.back)) }
        })
    }) { p ->
        Column(Modifier.padding(p).verticalScroll(rememberScrollState()).padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            EmulatorBody(id)
            HorizontalDivider(Modifier.padding(vertical = 8.dp))
            // at the bottom, in red, with a confirmation: never at the level of the main actions
            TextButton({ removing = true }, colors = ButtonDefaults.textButtonColors(contentColor = MaterialTheme.colorScheme.error)) {
                Text(stringResource(R.string.remove_emulator))
            }
        }
    }
    if (removing) {
        AlertDialog(
            onDismissRequest = { removing = false },
            title = { Text(stringResource(R.string.remove_emulator)) },
            text = { Text(stringResource(R.string.remove_emulator_text)) },
            confirmButton = {
                TextButton({
                    removing = false
                    onBack()
                    App.removeEmulator(id)
                }) { Text(stringResource(R.string.remove)) }
            },
            dismissButton = { TextButton({ removing = false }) { Text(stringResource(R.string.cancel)) } },
        )
    }
}

// EmulatorBody: the systems and folders of one emulator entry (Emulators screen, guided setup). The folder picker opens
// where this emulator usually keeps them; after a pick, one line says what the folder holds (App.folderResult).
@Composable
fun EmulatorBody(id: String) {
    val e = App.emulators.firstOrNull { it.id == id } ?: return
    val pr = e.profile()
    val custom = pr == CUSTOM
    val retro = pr.id == RETROARCH
    fun update(f: (Emulator) -> Emulator) = App.changeEmulator(id, f = f) // forgets the Bases of this entry (ADR 0026)
    fun updateSystem(s: System, f: (Folders) -> Folders) = update { it.copy(systems = it.systems + (s to f(it.systems[s] ?: Folders()))) }
    val scope = rememberCoroutineScope()
    var results by remember(id) { mutableStateOf(mapOf<System, String>()) }
    fun check(systems: Collection<System>) = scope.launch {
        val now = App.emulators.firstOrNull { it.id == id } ?: return@launch
        for (s in systems) {
            val r = withContext(Dispatchers.IO) { runCatching { App.folderResult(now, s) }.getOrNull() }
            results = if (r == null) results - s else results + (s to r)
        }
    }
    // "DS|roms": which folder the picker is for, kept if the activity restarts meanwhile; "*|saves": the one saves folder
    // of RetroArch, for every system
    var target by rememberSaveable { mutableStateOf<String?>(null) }
    var noPicker by remember { mutableStateOf(false) }
    val picker = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocumentTree()) { uri ->
        val t = target?.split('|')
        target = null
        if (uri != null && t != null) {
            App.takeFolder(uri)
            val now = App.emulators.firstOrNull { it.id == id }?.systems?.keys.orEmpty()
            val systems = if (t[0] == "*") now else now.filter { it.name == t[0] }
            update { x -> x.copy(systems = x.systems.mapValues { (s, f) -> if (s !in systems) f else if (t[1] == "saves") f.copy(saves = uri.toString()) else f.copy(roms = uri.toString()) }) }
            check(systems)
        }
    }
    fun pick(t: String, current: String?, usual: List<String>) {
        target = t
        noPicker = runCatching { picker.launch(startAt(current, usual)) }.isFailure // no folder picker on this phone
    }
    Text(stringResource(pr.hint))
    if (custom) {
        OutlinedTextField(e.label, { v -> App.changeEmulator(id, keepBases = true) { it.copy(label = v) } }, Modifier.fillMaxWidth(), label = { Text(stringResource(R.string.name)) }, singleLine = true)
    }
    if (pr.systems.size > 1) { // untick a system you don't play: it then asks for nothing
        Text(stringResource(R.string.systems_used), style = MaterialTheme.typography.titleSmall)
        FlowRow {
            for (s in pr.systems.keys) {
                val on = s in e.systems
                val can = !on || e.systems.size > 1 // at least one system
                Row(
                    Modifier.heightIn(min = 48.dp).toggleable(on, enabled = can, role = Role.Checkbox) { v ->
                        update {
                            val shared = if (retro) it.systems.values.firstNotNullOfOrNull { f -> f.saves } else null
                            it.copy(systems = if (v) it.systems + (s to Folders(saves = shared, nextToRom = pr.nextToRom)) else it.systems - s)
                        }
                    }.padding(end = 16.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Checkbox(on, null, enabled = can)
                    Text(s.label)
                }
            }
        }
    }
    if (noPicker) Text(stringResource(R.string.no_picker), color = MaterialTheme.colorScheme.error)
    if (retro) {
        val saves = e.systems.values.firstNotNullOfOrNull { it.saves }
        FolderRow(stringResource(R.string.retroarch_saves), saves) { pick("*|saves", saves, listOfNotNull(pr.savesAt)) }
    }
    for ((s, f) in e.systems.toSortedMap()) {
        HorizontalDivider(Modifier.padding(vertical = 8.dp))
        Text(systemName(s), Modifier.semantics { heading() }, style = MaterialTheme.typography.titleMedium)
        val variants = pr.systems[s].orEmpty()
        if (variants.size > 1) {
            Text(stringResource(R.string.core))
            FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp)) { // 4 GB cores: wider than a small phone
                variants.forEachIndexed { i, v -> FilterChip(f.variant == i, { updateSystem(s) { it.copy(variant = i) } }, { Text(v.label) }) }
            }
        }
        if (custom) {
            OutlinedTextField(f.ext, { v -> updateSystem(s) { it.copy(ext = v) } }, label = { Text(stringResource(R.string.extension)) }, singleLine = true)
        }
        if (s != System.N3DS) {
            FolderRow(stringResource(R.string.rom_folder), f.roms) { pick("${s.name}|roms", f.roms, romsAt(s)) }
            if (!retro) { // RetroArch: the scan looks next to the ROMs too
                Row(
                    Modifier.fillMaxWidth().heightIn(min = 48.dp).toggleable(f.nextToRom, role = Role.Checkbox) { on -> updateSystem(s) { it.copy(nextToRom = on) } },
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Checkbox(f.nextToRom, null)
                    Text(stringResource(R.string.next_to_rom))
                }
            }
        }
        if (!f.nextToRom && !retro) {
            FolderRow(stringResource(if (s == System.N3DS) R.string.azahar_folder else R.string.save_folder), f.saves) {
                pick("${s.name}|saves", f.saves, listOfNotNull(pr.savesAt))
            }
        }
        results[s]?.let { Text(it, Modifier.semantics { liveRegion = LiveRegionMode.Polite }, fontWeight = FontWeight.Medium) }
    }
}

@Composable
private fun FolderRow(label: String, uri: String?, pick: () -> Unit) {
    val st = produceState(FolderState.CHECKING, uri) { value = withContext(Dispatchers.IO) { check(uri) } }.value
    Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        Column(Modifier.weight(1f).padding(end = 8.dp)) {
            Text(label)
            if (uri != null) {
                val u = uri.toUri()
                val sd = !rootId(u).startsWith("primary:") // a removable card: the same folder names as the phone
                Text((if (sd) stringResource(R.string.sd_card) + " · " else "") + folderLabel(u), style = MaterialTheme.typography.bodySmall, fontFamily = FontFamily.Monospace)
            }
            StateLine(st)
        }
        OutlinedButton(pick) { Text(stringResource(R.string.choose)) }
    }
}

@Composable
fun systemName(s: System) = stringResource(
    when (s) {
        System.DS -> R.string.system_ds
        System.GB -> R.string.system_gb
        System.GBA -> R.string.system_gba
        System.N3DS -> R.string.system_3ds
    },
)

@Composable
fun Notice(text: String, button: String, onClick: () -> Unit) {
    Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.Center, horizontalAlignment = Alignment.CenterHorizontally) {
        Text(text)
        Spacer(Modifier.height(16.dp))
        Button(onClick) { Text(button) }
    }
}

