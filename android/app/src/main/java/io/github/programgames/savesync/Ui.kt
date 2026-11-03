@file:OptIn(ExperimentalMaterial3Api::class)

package io.github.programgames.savesync

import android.net.Uri
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
import io.github.programgames.savesync.core.Problem
import io.github.programgames.savesync.core.System
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
                Tab.SETTINGS -> SettingsScreen(p)
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
    Problem.DAMAGED -> R.string.p_damaged
    Problem.BUSY -> R.string.p_busy
    Problem.BACKUP -> R.string.p_backup
    Problem.WRITE -> R.string.p_write
    Problem.RESTORE -> R.string.p_restore
    Problem.NOT_WRITTEN -> R.string.p_not_written
    Problem.CHANGED -> R.string.p_changed
    Problem.FORMAT -> R.string.p_format
    Problem.PERMISSION -> R.string.p_permission
    Problem.OTHER -> R.string.p_other
}

// ErrorText: the text for the user, then the technical detail on its own line (to report it).
@Composable
fun ErrorText(t: Throwable, lead: String? = null) {
    Column(Modifier.padding(vertical = 4.dp)) {
        if (lead != null) Text(lead, color = MaterialTheme.colorScheme.error)
        Text(stringResource(problemText(problem(t))), color = MaterialTheme.colorScheme.error)
        Text(t.message ?: t.toString(), style = MaterialTheme.typography.bodySmall, fontFamily = FontFamily.Monospace)
    }
}

@Composable
fun Title(text: String) = Text(text, style = MaterialTheme.typography.titleLarge, modifier = Modifier.padding(bottom = 8.dp))

@Composable
fun SettingsScreen(p: PaddingValues) {
    var url by rememberSaveable { mutableStateOf(App.settings.url) }
    var token by rememberSaveable { mutableStateOf(App.settings.token) }
    var show by remember { mutableStateOf(false) }
    var result by remember { mutableStateOf<Result<Int>?>(null) }
    Column(Modifier.padding(p).verticalScroll(rememberScrollState()).padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Title(stringResource(R.string.settings_title))
        OutlinedTextField(
            url, {
                url = it
                App.settings.url = it
                App.serverChanged()
                result = null
            },
            Modifier.fillMaxWidth(), label = { Text(stringResource(R.string.server_address)) },
            placeholder = { Text("http://…:8080") }, supportingText = { Text(stringResource(R.string.server_address_help)) },
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
        val valid = url.trim().startsWith("http://") || url.trim().startsWith("https://")
        if (url.isNotBlank() && !valid) Text(stringResource(R.string.address_scheme), color = MaterialTheme.colorScheme.error)
        Button({ App.testConnection { result = it } }, enabled = App.busy == null && valid && token.isNotBlank()) {
            Text(stringResource(R.string.test_connection))
        }
        App.busy?.let { Text(it) }
        result?.fold({ Text(pluralStringResource(R.plurals.connected, it, it)) }, { ErrorText(it) })
        HorizontalDivider()
        Text(stringResource(R.string.backups_title), style = MaterialTheme.typography.titleMedium)
        Text(stringResource(R.string.backups_where))
        Text(App.backupsDir.path, style = MaterialTheme.typography.bodySmall, fontFamily = FontFamily.Monospace)
        HorizontalDivider()
        Text(stringResource(R.string.version, BuildConfig.VERSION_NAME), style = MaterialTheme.typography.bodySmall)
    }
}

enum class FolderState { NOT_SET, READY, MISSING, NO_PERMISSION, CHECKING }

private fun check(uri: String?): FolderState {
    if (uri == null) return FolderState.NOT_SET
    val u = uri.toUri()
    if (App.resolver.persistedUriPermissions.none { it.uri == u && it.isReadPermission && it.isWritePermission }) return FolderState.NO_PERMISSION
    return if (runCatching { children(App.resolver, u, rootId(u)) }.isSuccess) FolderState.READY else FolderState.MISSING
}

// emulatorState: the worst state of its folders.
private fun emulatorState(e: Emulator): FolderState {
    val all = e.systems.flatMap { (s, f) -> listOf(if (s == System.N3DS) FolderState.READY else check(f.roms), if (f.nextToRom) FolderState.READY else check(f.saves)) }
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

@Composable
private fun StateLine(s: FolderState) {
    Text(stateLabel(s), color = if (s == FolderState.READY || s == FolderState.CHECKING) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.error)
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
                    for (pr in profiles) {
                        TextButton({
                            adding = false
                            val e = Emulator(App.newId(pr.id), pr.id, if (pr == CUSTOM) App.text(R.string.custom_profile) else pr.name, pr.systems.keys.associateWith { Folders(nextToRom = pr.nextToRom) })
                            App.saveEmulators(App.emulators + e)
                            onEdit(e.id)
                        }, Modifier.fillMaxWidth()) { Text(if (pr == CUSTOM) stringResource(R.string.custom_profile) else pr.name, Modifier.fillMaxWidth()) }
                    }
                }
            },
            confirmButton = {},
            dismissButton = { TextButton({ adding = false }) { Text(stringResource(R.string.cancel)) } },
        )
    }
}

@Composable
fun EmulatorScreen(id: String, onBack: () -> Unit) {
    val e = App.emulators.firstOrNull { it.id == id } ?: return
    val pr = e.profile()
    val custom = pr == CUSTOM
    fun update(f: (Emulator) -> Emulator) {
        App.saveEmulators(App.emulators.map { if (it.id == id) f(it) else it })
        App.loaded = false // the game list must be read again
    }
    fun updateSystem(s: System, f: (Folders) -> Folders) = update { it.copy(systems = it.systems + (s to f(it.systems[s] ?: Folders()))) }
    // "DS|saves": which folder the picker is for (kept if the activity restarts meanwhile)
    var target by rememberSaveable { mutableStateOf<String?>(null) }
    val picker = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocumentTree()) { uri ->
        val t = target?.split('|')
        target = null
        if (uri != null && t != null) {
            App.takeFolder(uri)
            updateSystem(System.valueOf(t[0])) { if (t[1] == "saves") it.copy(saves = uri.toString()) else it.copy(roms = uri.toString()) }
        }
    }
    var removing by remember { mutableStateOf(false) }
    Scaffold(topBar = {
        TopAppBar(title = { Text(e.title()) }, navigationIcon = {
            IconButton(onBack) { Icon(Icons.AutoMirrored.Filled.ArrowBack, stringResource(R.string.back)) }
        })
    }) { p ->
        Column(Modifier.padding(p).verticalScroll(rememberScrollState()).padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(stringResource(pr.hint))
            if (custom) {
                OutlinedTextField(e.label, { v -> update { it.copy(label = v) } }, Modifier.fillMaxWidth(), label = { Text(stringResource(R.string.name)) }, singleLine = true)
                Row(verticalAlignment = Alignment.CenterVertically) {
                    for (s in CUSTOM.systems.keys) {
                        Checkbox(s in e.systems, { on -> update { it.copy(systems = if (on) it.systems + (s to Folders()) else it.systems - s) } })
                        Text(s.label)
                    }
                }
            }
            for ((s, f) in e.systems.toSortedMap()) {
                HorizontalDivider(Modifier.padding(vertical = 8.dp))
                Text(systemName(s), style = MaterialTheme.typography.titleMedium)
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
                    FolderRow(stringResource(R.string.rom_folder), f.roms) {
                        target = "${s.name}|roms"
                        picker.launch(null)
                    }
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Checkbox(f.nextToRom, { on -> updateSystem(s) { it.copy(nextToRom = on) } })
                        Text(stringResource(R.string.next_to_rom))
                    }
                }
                if (!f.nextToRom) {
                    FolderRow(stringResource(if (s == System.N3DS) R.string.azahar_folder else R.string.save_folder), f.saves) {
                        target = "${s.name}|saves"
                        picker.launch(null)
                    }
                }
            }
            HorizontalDivider(Modifier.padding(vertical = 8.dp))
            OutlinedButton({ removing = true }) { Text(stringResource(R.string.remove_emulator)) }
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
                    App.saveEmulators(App.emulators.filter { it.id != id })
                    App.loaded = false
                }) { Text(stringResource(R.string.remove)) }
            },
            dismissButton = { TextButton({ removing = false }) { Text(stringResource(R.string.cancel)) } },
        )
    }
}

@Composable
private fun FolderRow(label: String, uri: String?, pick: () -> Unit) {
    val st = produceState(FolderState.CHECKING, uri) { value = withContext(Dispatchers.IO) { check(uri) } }.value
    Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        Column(Modifier.weight(1f)) {
            Text(label)
            if (uri != null) Text(folderLabel(uri.toUri()), style = MaterialTheme.typography.bodySmall, fontFamily = FontFamily.Monospace)
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

