@file:OptIn(ExperimentalMaterial3Api::class)

package io.github.programgames.savesync

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Checkbox
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import io.github.programgames.savesync.core.Choice
import io.github.programgames.savesync.core.PlayInfo
import io.github.programgames.savesync.core.Summary
import io.github.programgames.savesync.core.System
import io.github.programgames.savesync.core.Version
import io.github.programgames.savesync.core.pokemonPlayInfo
import java.text.DateFormat
import java.text.Normalizer
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.TimeZone

// Colors of the States, like the 3DS app: To upload yellow, To download cyan, Conflict red, In sync green.
@Composable
fun stateColor(s: Shown): Color {
    val dark = isSystemInDarkTheme()
    return when (s) {
        Shown.TO_UPLOAD -> if (dark) Color(0xFFFFC94D) else Color(0xFF9A6700)
        Shown.TO_DOWNLOAD -> if (dark) Color(0xFF4DD0E1) else Color(0xFF00838F)
        Shown.CONFLICT -> if (dark) Color(0xFFEF7070) else Color(0xFFC62828)
        Shown.IN_SYNC -> if (dark) Color(0xFF81C784) else Color(0xFF2E7D32)
        else -> Color.Gray
    }
}

// One word per State, the same in the list and the details.
@Composable
fun stateWord(s: Shown) = stringResource(
    when (s) {
        Shown.TO_UPLOAD -> R.string.s_to_upload
        Shown.TO_DOWNLOAD -> R.string.s_to_download
        Shown.CONFLICT -> R.string.s_conflict
        Shown.IN_SYNC -> R.string.s_in_sync
        Shown.DUPLICATE -> R.string.s_duplicate
        Shown.UNKNOWN -> R.string.s_unknown
        Shown.NONE -> R.string.s_none
    },
)

// fold: lowercase without accents, for the search ("pokemon" finds "Pokémon").
fun fold(s: String) = Normalizer.normalize(s, Normalizer.Form.NFD).replace(Regex("\\p{Mn}+"), "").lowercase(Locale.ROOT)

@Composable
fun GamesScreen(p: PaddingValues, onOpen: (Entry) -> Unit, goSettings: () -> Unit, goEmulators: () -> Unit) {
    when {
        !App.configured() -> return Box(Modifier.padding(p)) { Notice(stringResource(R.string.need_server), stringResource(R.string.open_settings), goSettings) }
        App.emulators.isEmpty() -> return Box(Modifier.padding(p)) { Notice(stringResource(R.string.need_emulator), stringResource(R.string.add_emulator), goEmulators) }
    }
    LaunchedEffect(App.loaded) { if (!App.loaded) App.refresh() }
    var filter by rememberSaveable { mutableStateOf("ALL") }
    var query by rememberSaveable { mutableStateOf("") }
    var confirm by remember { mutableStateOf<List<Entry>?>(null) }
    var reminder by remember { mutableStateOf<(() -> Unit)?>(null) } // action to run after the reminder
    val shown = App.entries.filter { it.shown != Shown.NONE }
    val visible = shown.filter {
        when (filter) {
            "ALL" -> true
            "TODO" -> it.shown in listOf(Shown.TO_UPLOAD, Shown.TO_DOWNLOAD, Shown.CONFLICT)
            else -> it.system.name == filter
        } && fold(it.name).contains(fold(query.trim()))
    }
    val several = App.emulators.size > 1
    Column(Modifier.padding(p).padding(horizontal = 16.dp)) {
        Text(stringResource(R.string.tab_games), Modifier.padding(top = 16.dp), style = MaterialTheme.typography.titleLarge)
        // FlowRow: on a small screen or with longer words (French), the second button goes below instead of being cut
        FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedButton({ App.refresh() }, enabled = App.busy == null) { Text(stringResource(R.string.refresh)) }
            Button({
                // one reminder for the whole pass, even if it only uploads: a download may come up during the pass
                beforeWrite({ reminder = it }) {
                    val replaced = App.entries.filter { it.shown == Shown.TO_DOWNLOAD && it.status?.local != null }
                    if (replaced.isEmpty()) App.syncAll(emptyList()) else confirm = replaced
                }
            }, enabled = App.busy == null && App.loaded && App.serverError == null) { Text(stringResource(R.string.sync_all)) }
        }
        App.busy?.let {
            LinearProgressIndicator(Modifier.fillMaxWidth().padding(top = 8.dp))
            Text(it, style = MaterialTheme.typography.bodySmall)
        }
        App.serverError?.let { ErrorText(it, stringResource(R.string.server_not_read)) }
        App.message?.let { (text, t) -> if (t != null) ErrorText(t, text) }
        for (line in App.scanProblems) Text(stringResource(R.string.folder_problem, line), color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
        OutlinedTextField(query, { query = it }, Modifier.fillMaxWidth().padding(top = 8.dp), placeholder = { Text(stringResource(R.string.search)) }, singleLine = true)
        Row(Modifier.horizontalScroll(rememberScrollState()).padding(vertical = 4.dp), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
            for ((f, label) in listOf("ALL" to stringResource(R.string.filter_all), "TODO" to stringResource(R.string.filter_todo)) + System.entries.map { it.name to it.label }) {
                FilterChip(filter == f, { filter = f }, { Text(label) })
            }
        }
        LazyColumn(Modifier.weight(1f)) {
            items(visible, key = { it.id }) { e -> GameRow(e, several) { onOpen(e) } }
            item {
                val hidden = App.entries.size - shown.size
                if (App.loaded && App.busy == null && App.entries.isEmpty()) Text(stringResource(R.string.no_game), Modifier.padding(vertical = 16.dp))
                if (visible.isEmpty() && shown.isNotEmpty()) Text(stringResource(R.string.no_match), Modifier.padding(vertical = 16.dp))
                if (hidden > 0) Text(pluralStringResource(R.plurals.hidden_games, hidden, hidden), Modifier.padding(vertical = 16.dp), style = MaterialTheme.typography.bodySmall)
            }
        }
    }
    confirm?.let { list ->
        AlertDialog(
            onDismissRequest = { confirm = null },
            title = { Text(stringResource(R.string.sync_all)) },
            text = { Text(stringResource(R.string.confirm_replace, list.joinToString("\n") { "• " + it.name })) },
            confirmButton = {
                TextButton({
                    confirm = null
                    App.syncAll(list)
                }) { Text(stringResource(R.string.sync)) }
            },
            dismissButton = { TextButton({ confirm = null }) { Text(stringResource(R.string.cancel)) } },
        )
    }
    reminder?.let { Reminder(it) { reminder = null } }
    App.summary?.let { SummaryDialog(it) }
}

// beforeWrite runs an action that may write a save on this phone, after the reminder if it is due (show).
fun beforeWrite(show: (() -> Unit) -> Unit, action: () -> Unit) = if (App.mustRemind()) show(action) else action()

// Reminder asks to close the emulator before the first write on this phone since the app started (checkClosed).
@Composable
fun Reminder(then: () -> Unit, close: () -> Unit) {
    var never by remember { mutableStateOf(false) }
    AlertDialog(
        onDismissRequest = close,
        title = { Text(stringResource(R.string.reminder_title)) },
        text = {
            Column(Modifier.verticalScroll(rememberScrollState())) {
                Text(stringResource(R.string.reminder_text))
                Row(Modifier.toggleable(never, role = Role.Checkbox) { never = it }.padding(top = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                    Checkbox(never, null)
                    Text(stringResource(R.string.dont_show_again))
                }
            }
        },
        confirmButton = {
            TextButton({
                App.remind(never)
                close()
                then()
            }) { Text(stringResource(R.string.ok)) }
        },
        dismissButton = { TextButton(close) { Text(stringResource(R.string.cancel)) } },
    )
}

@Composable
private fun GameRow(e: Entry, several: Boolean, onClick: () -> Unit) {
    val c = stateColor(e.shown)
    Row(Modifier.fillMaxWidth().clickable(onClick = onClick).padding(vertical = 10.dp), verticalAlignment = Alignment.CenterVertically) {
        Box(Modifier.size(8.dp).background(c, CircleShape))
        Text(stateWord(e.shown), Modifier.padding(start = 8.dp).width(96.dp), color = c, style = MaterialTheme.typography.bodySmall, maxLines = 1)
        Text(
            e.system.label, Modifier.border(BorderStroke(1.dp, Color.Gray), RoundedCornerShape(4.dp)).padding(horizontal = 4.dp),
            style = MaterialTheme.typography.labelSmall,
        )
        Column(Modifier.padding(start = 10.dp)) {
            Text(e.name, maxLines = 2)
            if (several) Text(e.emulator.title(), style = MaterialTheme.typography.bodySmall, color = Color.Gray)
        }
    }
    HorizontalDivider()
}

@Composable
private fun SummaryDialog(s: Summary) {
    AlertDialog(
        onDismissRequest = { App.summary = null },
        title = { Text(stringResource(R.string.summary_title)) },
        text = {
            Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Text(stringResource(R.string.summary_counts, s.uploaded, s.downloaded, s.inSync))
                if (s.conflicts.isNotEmpty()) Text(stringResource(R.string.summary_conflicts, s.conflicts.joinToString(", ")))
                if (s.notReplaced.isNotEmpty()) Text(stringResource(R.string.summary_not_replaced, s.notReplaced.joinToString(", ")))
                if (s.stopped) Text(stringResource(R.string.summary_stopped), color = MaterialTheme.colorScheme.error)
                for ((name, t) in s.failures) ErrorText(t, name)
            }
        },
        confirmButton = { TextButton({ App.summary = null }) { Text(stringResource(R.string.ok)) } },
    )
}

private val serverDate = SimpleDateFormat("yyyyMMdd'T'HHmmss'Z'", Locale.ROOT).apply { timeZone = TimeZone.getTimeZone("UTC") }

private fun dateText(ms: Long) = DateFormat.getDateTimeInstance(DateFormat.SHORT, DateFormat.SHORT).format(Date(ms))

@Composable
fun ageText(age: Long?): String = when {
    age == null -> ""
    age < 60 -> stringResource(R.string.age_now)
    age < 3600 -> stringResource(R.string.age_min, age / 60)
    age < 86400 -> stringResource(R.string.age_h, age / 3600)
    else -> stringResource(R.string.age_d, age / 86400)
}

@Composable
fun playText(p: PlayInfo) =
    if (p.trainer.isEmpty()) stringResource(R.string.play_not_started, p.hours, p.minutes) else stringResource(R.string.play_info, p.hours, p.minutes, p.trainer)

@Composable
private fun serverLines(v: Version, play: PlayInfo?): List<String> = listOfNotNull(
    runCatching { serverDate.parse(v.date)!!.time }.getOrNull()?.let { stringResource(R.string.uploaded_on, dateText(it), ageText(v.age)) },
    stringResource(R.string.uploaded_by, v.client),
    pluralStringResource(R.plurals.size, v.size.toInt(), v.size),
    play?.let { playText(it) },
)

@Composable
fun DetailsScreen(e: Entry, onBack: () -> Unit) {
    val st = e.status
    val local = st?.local
    val server = st?.server
    val play = produceState<PlayInfo?>(null, server?.hash) { value = if (server != null && e.game != null) App.serverPlayInfo(e) else null }.value
    var ask by remember { mutableStateOf<Boolean?>(null) } // Conflict choice to confirm: true = keep the phone save
    var download by remember { mutableStateOf(false) }
    var reminder by remember { mutableStateOf<(() -> Unit)?>(null) } // action to run after the reminder
    Scaffold(topBar = {
        TopAppBar(title = { Text(e.name, maxLines = 1) }, navigationIcon = {
            IconButton(onBack) { Icon(Icons.AutoMirrored.Filled.ArrowBack, stringResource(R.string.back)) }
        })
    }) { p ->
        Column(Modifier.padding(p).verticalScroll(rememberScrollState()).padding(16.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Text("${systemName(e.system)} · ${e.emulator.title()} · ${e.rom}", style = MaterialTheme.typography.bodySmall)
            Text(stateWord(e.shown), color = stateColor(e.shown), style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold)
            Text(
                when (e.shown) {
                    Shown.TO_UPLOAD -> stringResource(R.string.why_to_upload)
                    Shown.TO_DOWNLOAD -> stringResource(R.string.why_to_download)
                    Shown.CONFLICT -> stringResource(R.string.why_conflict)
                    Shown.IN_SYNC -> stringResource(R.string.why_in_sync)
                    Shown.DUPLICATE -> stringResource(if (e.sharedFile) R.string.why_shared_file else R.string.why_duplicate, e.dups.size) +
                        "\n" + e.dups.joinToString("\n") { "• $it" }
                    Shown.UNKNOWN -> stringResource(R.string.why_unknown)
                    Shown.NONE -> stringResource(R.string.why_none)
                },
            )
            e.error?.let { ErrorText(it) }
            if (e.shown == Shown.UNKNOWN) App.serverError?.let { ErrorText(it, stringResource(R.string.server_not_read)) }
            App.message?.let { (text, t) -> if (t == null) Text(text, color = MaterialTheme.colorScheme.primary) else ErrorText(t, text) }
            App.busy?.let { LinearProgressIndicator(Modifier.fillMaxWidth()) }
            if (e.game == null || st == null) return@Column // Duplicate, or nothing read: no cards

            val phoneLines = if (local == null) listOf(stringResource(R.string.no_local_save)) else listOfNotNull(
                e.game.file.modified()?.let { stringResource(R.string.modified_on, dateText(it)) },
                pluralStringResource(R.plurals.size, local.version.size.toInt(), local.version.size),
                if (local.blank) stringResource(R.string.blank_save) else null,
                pokemonPlayInfo(local.sram)?.let { playText(it) },
            ) + e.game.file.location
            val serverText = if (server == null) listOf(stringResource(R.string.no_server_version)) else serverLines(server, play)

            if (e.shown == Shown.CONFLICT && server != null) {
                Text(stringResource(R.string.choose_version), style = MaterialTheme.typography.titleMedium)
                // one card above the other: side by side, the buttons were cut on a phone 320 dp wide
                ChoiceCard(stringResource(R.string.this_phone), phoneLines, stringResource(R.string.keep_phone), Modifier.fillMaxWidth()) { ask = true }
                ChoiceCard(stringResource(R.string.server), serverText, stringResource(R.string.take_server), Modifier.fillMaxWidth()) {
                    beforeWrite({ reminder = it }) { ask = false }
                }
                Text(stringResource(R.string.nothing_lost), style = MaterialTheme.typography.bodySmall)
            } else {
                InfoCard(stringResource(R.string.this_phone), phoneLines)
                InfoCard(stringResource(R.string.server), serverText)
                st?.let {
                    val b = it.base
                    Text(
                        stringResource(R.string.last_sync) + " " + stringResource(
                            when {
                                b == null -> R.string.base_never
                                b == local?.version?.hash && b == server?.hash -> R.string.base_both
                                b == local?.version?.hash -> R.string.base_phone
                                b == server?.hash -> R.string.base_server
                                else -> R.string.base_differs
                            },
                        ),
                        style = MaterialTheme.typography.bodySmall,
                    )
                }
                when (e.shown) {
                    Shown.TO_UPLOAD -> Button({ App.act(e) }, enabled = App.busy == null) { Text(stringResource(R.string.upload)) }
                    Shown.TO_DOWNLOAD -> Button({ beforeWrite({ reminder = it }) { if (local != null) download = true else App.act(e) } }, enabled = App.busy == null) { Text(stringResource(R.string.download)) }
                    else -> {}
                }
            }
        }
    }
    reminder?.let { Reminder(it) { reminder = null } }
    ask?.let { phone ->
        AlertDialog(
            onDismissRequest = { ask = null },
            title = { Text(stringResource(if (phone) R.string.keep_phone else R.string.take_server)) },
            text = { Text(stringResource(if (phone) R.string.confirm_keep_phone else R.string.confirm_take_server)) },
            confirmButton = {
                TextButton({
                    ask = null
                    App.act(e, Choice(keepPhone = phone, serverHash = server!!.hash))
                }) { Text(stringResource(R.string.confirm)) }
            },
            dismissButton = { TextButton({ ask = null }) { Text(stringResource(R.string.cancel)) } },
        )
    }
    if (download) {
        AlertDialog(
            onDismissRequest = { download = false },
            title = { Text(stringResource(R.string.download)) },
            text = { Text(stringResource(R.string.confirm_download)) },
            confirmButton = {
                TextButton({
                    download = false
                    App.act(e, mayReplace = true)
                }) { Text(stringResource(R.string.download)) }
            },
            dismissButton = { TextButton({ download = false }) { Text(stringResource(R.string.cancel)) } },
        )
    }
}

@Composable
private fun InfoCard(title: String, lines: List<String>) {
    OutlinedCard(Modifier.fillMaxWidth()) {
        Column(Modifier.padding(12.dp)) {
            Text(title, style = MaterialTheme.typography.titleSmall)
            for (l in lines) Text(l, style = MaterialTheme.typography.bodySmall)
        }
    }
}

@Composable
private fun ChoiceCard(title: String, lines: List<String>, action: String, modifier: Modifier, onClick: () -> Unit) {
    OutlinedCard(modifier) {
        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(title, style = MaterialTheme.typography.titleSmall)
            for (l in lines) Text(l, style = MaterialTheme.typography.bodySmall)
            Button(onClick, enabled = App.busy == null) { Text(action) }
        }
    }
}
