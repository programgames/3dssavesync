@file:OptIn(ExperimentalMaterial3Api::class)

package io.github.programgames.savesync

import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
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
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.material3.Surface
import androidx.compose.material3.Switch
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.ui.semantics.CustomAccessibilityAction
import androidx.compose.ui.semantics.customActions
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.material3.FilledTonalButton
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
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
import androidx.compose.runtime.mutableIntStateOf
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
import io.github.programgames.savesync.core.Problem
import io.github.programgames.savesync.core.RESTORE_CLIENT
import io.github.programgames.savesync.core.Summary
import io.github.programgames.savesync.core.System
import io.github.programgames.savesync.core.aliasExamples
import io.github.programgames.savesync.core.withoutExtension
import io.github.programgames.savesync.core.Version
import io.github.programgames.savesync.core.problem
import java.io.File
import java.text.DateFormat
import java.text.Normalizer
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.TimeZone

// Colors of the States, like the 3DS app: To upload yellow, To download cyan, Conflict red, In sync green. Each one
// reads at 4.5:1 or more on the light or dark surface, inside its label too (ColorsTest). null: the grey of the theme.
fun stateColor(s: Shown, dark: Boolean): Color? = when (s) {
    Shown.TO_UPLOAD -> if (dark) Color(0xFFFFC94D) else Color(0xFF7D5400)
    Shown.TO_DOWNLOAD -> if (dark) Color(0xFF4DD0E1) else Color(0xFF00666F)
    Shown.CONFLICT, Shown.INTERRUPTED -> if (dark) Color(0xFFEF7070) else Color(0xFFB71C1C)
    Shown.IN_SYNC -> if (dark) Color(0xFF81C784) else Color(0xFF1E6B23)
    else -> null
}

@Composable
fun stateColor(s: Shown): Color = stateColor(s, isSystemInDarkTheme()) ?: MaterialTheme.colorScheme.onSurfaceVariant

const val LABEL_TINT = 0.14f // the background of a StatusLabel: its colour at 14 %

// StatusLabel: the State as a coloured label (dot and word, never the colour alone). Not a chip: it does nothing.
@Composable
fun StatusLabel(s: Shown, modifier: Modifier = Modifier) {
    val c = stateColor(s)
    Surface(modifier, shape = RoundedCornerShape(50), color = c.copy(alpha = LABEL_TINT)) {
        Row(Modifier.padding(horizontal = 8.dp, vertical = 2.dp), verticalAlignment = Alignment.CenterVertically) {
            Box(Modifier.size(8.dp).background(c, CircleShape))
            Text(stateWord(s), Modifier.padding(start = 6.dp), color = c, style = MaterialTheme.typography.labelLarge)
        }
    }
}

// One word per State, the same in the list and the details.
@Composable
fun stateWord(s: Shown) = stringResource(
    when (s) {
        Shown.TO_UPLOAD -> R.string.s_to_upload
        Shown.TO_DOWNLOAD -> R.string.s_to_download
        Shown.CONFLICT -> R.string.s_conflict
        Shown.INTERRUPTED -> R.string.s_interrupted
        Shown.IN_SYNC -> R.string.s_in_sync
        Shown.DUPLICATE -> R.string.s_duplicate
        Shown.UNKNOWN -> R.string.s_unknown
        Shown.NOT_SYNCED -> R.string.s_not_synced
        Shown.NONE -> R.string.s_none
    },
)

// fold: lowercase without accents, for the search ("pokemon" finds "Pokémon").
fun fold(s: String) = Normalizer.normalize(s, Normalizer.Form.NFD).replace(marks, "").lowercase(Locale.ROOT)

private val marks = Regex("\\p{Mn}+") // compiled once: the search folds every name at each keystroke

private val todo = setOf(Shown.TO_UPLOAD, Shown.TO_DOWNLOAD, Shown.CONFLICT, Shown.INTERRUPTED) // the "To do" chip

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
    // filtered again only when the list, the chip or the search changes: not at each progress line of a sync
    val entries = App.entries
    val shown = remember(entries) { entries.filter { it.shown != Shown.NONE } }
    val visible = remember(shown, filter, query) {
        val q = fold(query.trim())
        shown.filter {
            when (filter) {
                "ALL" -> true
                "TODO" -> it.shown in todo
                else -> it.system.name == filter
            } && (fold(it.short).contains(q) || fold(it.name).contains(q))
        }
    }
    val several = App.emulators.size > 1
    val systems = App.emulators.flatMap { it.systems.keys }.distinct().sorted() // a chip only for a system some emulator has
    // why Sync all can't run now; null: it can (or an action runs, and the progress line says it)
    val why = when {
        App.busy != null || !App.loaded -> null
        App.serverError != null -> R.string.sync_off_server
        App.entries.none { it.shown == Shown.TO_UPLOAD || it.shown == Shown.TO_DOWNLOAD } -> R.string.sync_off_nothing
        else -> null
    }
    Column(Modifier.padding(p).padding(horizontal = 16.dp)) {
        Text(stringResource(R.string.tab_games), Modifier.padding(top = 16.dp).semantics { heading() }, style = MaterialTheme.typography.titleLarge)
        // FlowRow: on a small screen or with a large font, the reason goes below the button instead of being cut
        FlowRow(horizontalArrangement = Arrangement.spacedBy(12.dp), verticalArrangement = Arrangement.Center) {
            Button({
                // one reminder for the whole pass, even if it only uploads: a download may come up during the pass
                beforeWrite({ reminder = it }) {
                    val replaced = App.entries.filter { it.shown == Shown.TO_DOWNLOAD && it.status?.local != null }
                    if (replaced.isEmpty()) App.syncAll(emptyList()) else confirm = replaced
                }
            }, enabled = App.busy == null && App.loaded && why == null) { Text(stringResource(R.string.sync_all)) }
            why?.let { Text(stringResource(it), Modifier.align(Alignment.CenterVertically), color = MaterialTheme.colorScheme.onSurfaceVariant) }
        }
        App.busy?.let {
            LinearProgressIndicator(Modifier.fillMaxWidth().padding(top = 8.dp))
            Text(it, Modifier.semantics { liveRegion = LiveRegionMode.Polite }, style = MaterialTheme.typography.bodySmall)
        }
        val refresh = stringResource(R.string.refresh)
        // pull down to read the folders and the server again; TalkBack and keyboards get the same as an action
        PullToRefreshBox(
            App.busy != null, { App.refresh() },
            Modifier.weight(1f).semantics { customActions = listOf(CustomAccessibilityAction(refresh) { App.refresh(); true }) },
        ) {
            LazyColumn(Modifier.fillMaxSize()) {
                // the banners, search and chips scroll with the games: with a large font they would leave no room
                item {
                    UpdateCard()
                    val offline = App.entries.count { it.noLocal }
                    App.serverError?.let { ErrorBox(it, note = if (offline > 0) stringResource(R.string.hidden_offline) else null, retry = { App.refresh() }) }
                    App.message?.let { (text, t) -> if (t != null) ErrorBox(t, text) }
                    for (line in App.scanProblems) Text(line, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
                    for (line in App.scanNotes) Text(line, color = MaterialTheme.colorScheme.onSurfaceVariant, style = MaterialTheme.typography.bodySmall)
                    if (shown.isNotEmpty()) {
                        OutlinedTextField(query, { query = it }, Modifier.fillMaxWidth().padding(top = 8.dp), placeholder = { Text(stringResource(R.string.search)) }, singleLine = true)
                        Row(Modifier.horizontalScroll(rememberScrollState()).padding(vertical = 4.dp), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                            val chips = listOf("ALL" to stringResource(R.string.filter_all), "TODO" to stringResource(R.string.filter_todo)) +
                                if (systems.size > 1) systems.map { it.name to it.label } else emptyList()
                            for ((f, label) in chips) FilterChip(filter == f, { filter = f }, { Text(label) })
                        }
                    }
                }
                items(visible, key = { it.id }) { e -> GameRow(e, several) { onOpen(e) } }
                item {
                    val hidden = App.entries.size - shown.size
                    if (App.loaded && App.busy == null && App.entries.isEmpty()) {
                        Column(Modifier.fillMaxWidth().padding(vertical = 24.dp), horizontalAlignment = Alignment.CenterHorizontally) {
                            Text(stringResource(R.string.no_game))
                            OutlinedButton(goEmulators, Modifier.padding(top = 12.dp)) { Text(stringResource(R.string.open_emulators)) }
                        }
                    }
                    if (visible.isEmpty() && shown.isNotEmpty()) Text(stringResource(R.string.no_match), Modifier.padding(vertical = 16.dp))
                    // offline: the banner says why games are hidden
                    if (hidden > 0 && App.serverError == null) Text(pluralStringResource(R.plurals.hidden_games, hidden, hidden), Modifier.padding(vertical = 16.dp), style = MaterialTheme.typography.bodySmall)
                }
            }
        }
    }
    confirm?.let { list ->
        Confirm(
            pluralStringResource(R.plurals.confirm_replace_title, list.size, list.size), stringResource(R.string.replace_and_sync), { App.syncAll(list) }, { confirm = null },
            stringResource(R.string.confirm_replace, list.joinToString("\n") { "• " + it.short + if (several) " · " + it.emulator.listName() else "" }),
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
    Confirm(stringResource(R.string.reminder_title), stringResource(R.string.emulator_closed), {
        App.remind(never)
        then()
    }, close) {
        Text(stringResource(R.string.reminder_text))
        Row(Modifier.toggleable(never, role = Role.Checkbox) { never = it }.padding(top = 8.dp), verticalAlignment = Alignment.CenterVertically) {
            Checkbox(never, null)
            Text(stringResource(R.string.dont_show_again))
        }
    }
}

// GameRow: the short name on the full width, then the State, the system and the emulator. One focus stop for
// TalkBack ("Mario Kart DS, DS, melonDS, To download, button"): the row merges its texts, and the State is its state
// description, not one more text.
@Composable
private fun GameRow(e: Entry, several: Boolean, onClick: () -> Unit) {
    val state = stateWord(e.shown)
    Column(
        Modifier.fillMaxWidth().heightIn(min = 48.dp).semantics(mergeDescendants = true) { stateDescription = state }
            .clickable(onClick = onClick).padding(vertical = 8.dp),
    ) {
        Text(e.short, maxLines = 2, overflow = TextOverflow.Ellipsis, style = MaterialTheme.typography.bodyLarge)
        FlowRow(Modifier.padding(top = 4.dp), horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            StatusLabel(e.shown, Modifier.clearAndSetSemantics {})
            Text(
                e.system.label,
                Modifier.align(Alignment.CenterVertically).border(BorderStroke(1.dp, MaterialTheme.colorScheme.outline), RoundedCornerShape(4.dp)).padding(horizontal = 4.dp),
                style = MaterialTheme.typography.labelMedium,
            )
            if (several) Text(e.emulator.listName(), Modifier.align(Alignment.CenterVertically), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
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
                for ((name, t) in s.failures) ErrorBox(t, name)
            }
        },
        confirmButton = { TextButton({ App.summary = null }) { Text(stringResource(R.string.close)) } },
    )
}

private val serverDate = SimpleDateFormat("yyyyMMdd'T'HHmmss'Z'", Locale.ROOT).apply { timeZone = TimeZone.getTimeZone("UTC") }

fun dateText(ms: Long) = DateFormat.getDateTimeInstance(DateFormat.SHORT, DateFormat.SHORT).format(Date(ms))

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
    stringResource(R.string.uploaded_by, clientLabel(v.client)),
    sizeText(v.size),
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
    var restoreCopy by remember { mutableStateOf(false) } // interrupted write: put the Backup copy back, to confirm
    var reminder by remember { mutableStateOf<(() -> Unit)?>(null) } // action to run after the reminder
    var history by rememberSaveable { mutableStateOf(false) }
    val hackable = e.game == null && !e.sharedFile && e.system in aliasExamples
    if (history && e.game != null) {
        BackHandler { history = false }
        return HistoryScreen(e) { history = false }
    }
    // the name in the page, not in the bar: a long name wraps instead of being cut
    Scaffold(topBar = {
        TopAppBar(title = {}, navigationIcon = {
            IconButton(onBack) { Icon(Icons.AutoMirrored.Filled.ArrowBack, stringResource(R.string.back)) }
        })
    }) { p ->
        Column(Modifier.padding(p).verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(bottom = 16.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Text(e.short, Modifier.semantics { heading() }, style = MaterialTheme.typography.headlineSmall)
            Column {
                Text("${systemName(e.system)} · ${e.emulator.listName()}", style = MaterialTheme.typography.bodyMedium)
                // the full ROM file name, with its tags; a 3DS game has no ROM file here
                if (e.game != null && e.system != System.N3DS) Text(stringResource(R.string.file_row, e.rom), style = MaterialTheme.typography.bodySmall)
            }
            Text(stateWord(e.shown), color = stateColor(e.shown), style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold)
            Text(
                when (e.shown) {
                    Shown.TO_UPLOAD -> stringResource(R.string.why_to_upload)
                    Shown.TO_DOWNLOAD -> stringResource(R.string.why_to_download)
                    Shown.CONFLICT -> stringResource(if (st?.cameBack == true) R.string.why_came_back else R.string.why_conflict)
                    Shown.INTERRUPTED -> stringResource(R.string.why_interrupted)
                    Shown.IN_SYNC -> stringResource(R.string.why_in_sync)
                    Shown.DUPLICATE -> stringResource(if (e.sharedFile) R.string.why_shared_file else R.string.why_duplicate, e.dups.size) +
                        if (hackable) "" else "\n" + e.dups.joinToString("\n") { "• $it" }
                    Shown.UNKNOWN -> stringResource(R.string.why_unknown)
                    Shown.NOT_SYNCED -> stringResource(if (e.off && e.extdata) R.string.why_extdata_off else if (e.off) R.string.why_off else R.string.why_not_started)
                    Shown.NONE -> stringResource(R.string.why_none)
                },
            )
            // an extdata has no switch of its own: it follows the one of its game (ADR 0031)
            if (e.extdata) Text(stringResource(R.string.extdata_help), style = MaterialTheme.typography.bodySmall)
            else if (e.system == System.N3DS && e.game != null) {
                // on or off by title ID, kept in the settings (ADR 0028); a new game is on
                Row(
                    Modifier.fillMaxWidth().heightIn(min = 48.dp).toggleable(!e.off, enabled = App.busy == null, role = Role.Switch) { App.setSynced(e, it) },
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Text(stringResource(R.string.sync_this_game), Modifier.weight(1f).padding(end = 8.dp))
                    Switch(!e.off, null, enabled = App.busy == null)
                }
                Text(stringResource(R.string.sync_this_game_help), style = MaterialTheme.typography.bodySmall)
            }
            if (hackable) {
                // same Key: one of the ROMs may be a ROM hack. Its Alias gives it the Key of the PC (ADR 0014).
                for (rom in e.dups) {
                    Text("• $rom")
                    OutlinedButton({
                        App.message = App.addAlias(e.system, withoutExtension(rom))?.let { App.text(R.string.alias_failed) to IllegalArgumentException(it) }
                        if (App.message == null) onBack() // the list is read again: the two ROMs, each with its Key
                    }, Modifier.named(stringResource(R.string.own_save), rom), enabled = App.busy == null) { Text(stringResource(R.string.own_save)) }
                }
            }
            // a server not read: the list says it once, with Retry; here only the why line above
            e.error?.let { ErrorBox(it) }
            App.message?.let { (text, t) ->
                if (t == null) Text(text, Modifier.semantics { liveRegion = LiveRegionMode.Polite }, color = MaterialTheme.colorScheme.primary) else ErrorBox(t, text)
            }
            App.busy?.let { LinearProgressIndicator(Modifier.fillMaxWidth()) }
            if (e.game == null || st == null) return@Column // Duplicate, or nothing read: no cards

            val phoneLines = phoneLines(e) + listOfNotNull(e.game.file.location.takeIf { local != null })
            val serverText = if (server == null) listOf(stringResource(R.string.no_server_version)) else serverLines(server, play)

            if (e.shown == Shown.INTERRUPTED) {
                // only the server save or the Backup copy: the file on this phone may be cut (ADR 0026)
                InfoCard(stringResource(R.string.this_phone), if (local != null) phoneLines else listOf(e.game.file.location))
                if (server != null) {
                    ChoiceCard(stringResource(R.string.server), serverText, stringResource(R.string.take_server), Modifier.fillMaxWidth()) {
                        beforeWrite({ reminder = it }) { ask = false }
                    }
                }
                st.interrupted?.backup?.let { copy ->
                    ChoiceCard(stringResource(R.string.backup_copy), listOf(File(copy).name), stringResource(R.string.restore_copy), Modifier.fillMaxWidth()) {
                        beforeWrite({ reminder = it }) { restoreCopy = true }
                    }
                }
            } else if (e.shown == Shown.CONFLICT && server != null) {
                Section(stringResource(R.string.choose_version))
                // one card above the other: side by side, the buttons were cut on a phone 320 dp wide
                ChoiceCard(stringResource(R.string.this_phone), phoneLines, stringResource(R.string.keep_phone), Modifier.fillMaxWidth()) { ask = true }
                ChoiceCard(stringResource(R.string.server), serverText, stringResource(R.string.take_server), Modifier.fillMaxWidth()) {
                    beforeWrite({ reminder = it }) { ask = false }
                }
                Text(stringResource(R.string.nothing_lost), style = MaterialTheme.typography.bodySmall)
            } else {
                InfoCard(stringResource(R.string.this_phone), phoneLines)
                InfoCard(stringResource(R.string.server), serverText)
                when (e.shown) {
                    Shown.TO_UPLOAD -> Button({ App.act(e) }, enabled = App.busy == null) { Text(stringResource(R.string.upload)) }
                    Shown.TO_DOWNLOAD -> Button({ beforeWrite({ reminder = it }) { if (local != null) download = true else App.act(e) } }, enabled = App.busy == null) { Text(stringResource(R.string.download)) }
                    else -> {}
                }
            }
            // the server Versions and the Backup copies of this phone: shown even without a server version
            OutlinedButton({
                App.message = null
                history = true
            }, enabled = App.busy == null) { Text(stringResource(R.string.show_history)) }
        }
    }
    reminder?.let { Reminder(it) { reminder = null } }
    ask?.let { phone ->
        Confirm(
            stringResource(if (phone) R.string.keep_phone else R.string.take_server), stringResource(if (phone) R.string.upload else R.string.replace), // the verb of the outcome
            { App.act(e, Choice(keepPhone = phone, serverHash = server!!.hash)) }, { ask = null },
            stringResource(if (phone) R.string.confirm_keep_phone else R.string.confirm_take_server),
        )
    }
    if (restoreCopy) {
        Confirm(stringResource(R.string.restore_copy), stringResource(R.string.restore_copy), { App.restoreCopy(e) }, { restoreCopy = false }, stringResource(R.string.confirm_restore_copy))
    }
    if (download) {
        Confirm(stringResource(R.string.confirm_download_title), stringResource(R.string.replace), { App.act(e, mayReplace = true) }, { download = false }, stringResource(R.string.confirm_download))
    }
}

// phoneLines: the save on this phone, as the details and the Restore confirmation show it.
@Composable
private fun phoneLines(e: Entry): List<String> {
    val local = e.status?.local ?: return listOf(stringResource(if (e.game?.archive != null) R.string.never_started_azahar else R.string.no_local_save))
    return listOfNotNull(
        e.game?.file?.modified()?.let { stringResource(R.string.modified_on, dateText(it)) },
        sizeText(local.version.size),
        if (local.blank) stringResource(R.string.blank_save) else null,
        local.play?.let { playText(it) },
    )
}

// clientLabel: the device that uploaded a Version (X-Client), as the user names it.
@Composable
fun clientLabel(c: String) = when (c) {
    "3ds" -> "3DS"
    "pc" -> "PC"
    "android" -> "Android"
    RESTORE_CLIENT -> stringResource(R.string.client_restore)
    "" -> "?"
    else -> c
}

@Composable
fun sizeText(n: Long) = if (n >= 1024) stringResource(R.string.size_kib, (n + 512) / 1024) else pluralStringResource(R.plurals.size, n.toInt(), n)

@Composable
private fun versionLine(v: Version) = stringResource(R.string.version_no, v.number) + " · " + ageText(v.age)

// HistoryScreen: the server Versions of a Game, newest first. Restore makes an old one the Current version again, on the
// server only: this phone then gets it through the usual Download, with its Backup copy and its checks (ADR 0016).
// Below, the Backup copies of this phone: the app folder is out of reach of the file managers (Android 11+), so a copy
// is put back or saved to a folder the user picks from here.
@Composable
fun HistoryScreen(e: Entry, onBack: () -> Unit) {
    var load by remember { mutableIntStateOf(0) } // the list is read again after a 412
    val hasServer = e.status?.server != null
    val list = produceState<Result<List<Version>>?>(null, load) {
        value = null
        value = if (hasServer) runCatching { App.history(e) } else Result.success(emptyList())
    }.value
    val copies = produceState(emptyList<File>(), App.busy) { if (App.busy == null) value = App.copies(e) }.value
    var selected by rememberSaveable { mutableStateOf<String?>(null) } // server name of the selected Version
    var selectedCopy by rememberSaveable { mutableStateOf<String?>(null) } // path of the selected Backup copy
    var askCopy by remember { mutableStateOf<File?>(null) }
    var reminder by remember { mutableStateOf<(() -> Unit)?>(null) } // action to run after the reminder
    var toSave by remember { mutableStateOf<File?>(null) }
    val saver = rememberLauncherForActivityResult(ActivityResultContracts.CreateDocument("application/octet-stream")) { uri ->
        val f = toSave
        if (uri != null && f != null) App.saveCopy(f, uri)
    }
    var ask by remember { mutableStateOf(false) }
    val versions = list?.getOrNull().orEmpty()
    val current = versions.firstOrNull()
    val v = versions.find { it.file == selected }
    // the play time of the selected Version only: one download, on demand
    val play = produceState<PlayInfo?>(null, v?.file) {
        value = null
        value = v?.let { App.versionPlayInfo(e, it) }
    }.value
    Scaffold(topBar = {
        TopAppBar(title = { Text(stringResource(R.string.history)) }, navigationIcon = {
            IconButton(onBack) { Icon(Icons.AutoMirrored.Filled.ArrowBack, stringResource(R.string.back)) }
        })
    }) { p ->
        LazyColumn(Modifier.padding(p).padding(horizontal = 16.dp)) {
            item {
                Text(e.short, Modifier.semantics { heading() }, style = MaterialTheme.typography.titleMedium)
                if (list == null || App.busy != null) LinearProgressIndicator(Modifier.fillMaxWidth().padding(vertical = 4.dp))
                App.busy?.let { Text(it, Modifier.semantics { liveRegion = LiveRegionMode.Polite }, style = MaterialTheme.typography.bodySmall) }
                App.message?.let { (text, t) ->
                    if (t == null) Text(text, Modifier.semantics { liveRegion = LiveRegionMode.Polite }, color = MaterialTheme.colorScheme.primary) else ErrorBox(t, text)
                }
                Section(stringResource(R.string.server))
                Text(stringResource(R.string.history_help), Modifier.padding(vertical = 4.dp), style = MaterialTheme.typography.bodySmall)
                if (list == null) Text(stringResource(R.string.reading_history), style = MaterialTheme.typography.bodySmall)
                list?.exceptionOrNull()?.let { ErrorBox(it, stringResource(R.string.history_failed), retry = { load++ }) }
                if (list?.isSuccess == true && versions.isEmpty()) Text(stringResource(R.string.no_versions), Modifier.padding(vertical = 8.dp))
            }
            items(versions, key = { it.file }) { x ->
                val sel = x.file == selected
                Row(
                    Modifier.fillMaxWidth().heightIn(min = 48.dp).background(if (sel) MaterialTheme.colorScheme.secondaryContainer else Color.Transparent)
                        .selectable(sel) {
                            selected = x.file
                            App.message = null
                        }.padding(8.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Column(Modifier.weight(1f)) {
                        Text(versionLine(x))
                        Text(clientLabel(x.client) + " · " + sizeText(x.size), style = MaterialTheme.typography.bodySmall)
                    }
                    if (x == current) Text(stringResource(R.string.current), color = stateColor(Shown.IN_SYNC), style = MaterialTheme.typography.labelMedium)
                }
                if (sel) {
                    Column(Modifier.padding(8.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                        play?.let { Text(playText(it), style = MaterialTheme.typography.bodySmall) }
                        when {
                            x == current -> Text(stringResource(R.string.version_current_help), style = MaterialTheme.typography.bodySmall)
                            x.hash == current?.hash -> Text(stringResource(R.string.version_same), style = MaterialTheme.typography.bodySmall)
                            else -> {
                                Text(stringResource(R.string.version_restore_help), style = MaterialTheme.typography.bodySmall)
                                Button({ ask = true }, enabled = App.busy == null) { Text(stringResource(R.string.restore)) }
                            }
                        }
                    }
                }
                HorizontalDivider()
            }
            item {
                Section(stringResource(R.string.this_phone))
                Text(stringResource(if (copies.isEmpty()) R.string.no_copies else R.string.copies_help), Modifier.padding(vertical = 4.dp), style = MaterialTheme.typography.bodySmall)
            }
            items(copies, key = { it.path }) { f ->
                val sel = f.path == selectedCopy
                Column(
                    Modifier.fillMaxWidth().heightIn(min = 48.dp).background(if (sel) MaterialTheme.colorScheme.secondaryContainer else Color.Transparent)
                        .selectable(sel) {
                            selectedCopy = f.path
                            App.message = null
                        }.padding(8.dp),
                ) {
                    Text(stringResource(R.string.copy_of, dateText(f.lastModified())))
                    Text(sizeText(f.length()), style = MaterialTheme.typography.bodySmall)
                }
                if (sel) {
                    FlowRow(Modifier.padding(8.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        Button({ beforeWrite({ reminder = it }) { askCopy = f } }, enabled = App.busy == null) { Text(stringResource(R.string.put_back)) }
                        OutlinedButton({
                            toSave = f
                            saver.launch(f.name)
                        }, enabled = App.busy == null) { Text(stringResource(R.string.save_copy)) }
                    }
                }
                HorizontalDivider()
            }
        }
    }
    reminder?.let { Reminder(it) { reminder = null } }
    askCopy?.let { f ->
        Confirm(stringResource(R.string.put_back), stringResource(R.string.replace), {
            App.putBack(e, f) { t -> if (t == null) onBack() } // the details show the new State: To upload
        }, { askCopy = null }) {
            Text(stringResource(R.string.this_copy), style = MaterialTheme.typography.titleSmall)
            Text(stringResource(R.string.copy_of, dateText(f.lastModified())) + " · " + sizeText(f.length()))
            Text(stringResource(R.string.this_phone), Modifier.padding(top = 8.dp), style = MaterialTheme.typography.titleSmall)
            for (l in phoneLines(e)) Text(l)
            Text(stringResource(R.string.confirm_put_back), Modifier.padding(top = 8.dp))
        }
    }
    if (ask && v != null && current != null) {
        Confirm(stringResource(R.string.restore_title), stringResource(R.string.restore), {
            App.restore(e, v, current) { t ->
                when {
                    t == null -> onBack() // the details show the new State: To download, with the usual Download
                    problem(t) == Problem.SERVER_CHANGED -> { // the list again: choose again
                        selected = null
                        load++
                    }
                }
            }
        }, { ask = false }) {
            Text(stringResource(R.string.this_version), style = MaterialTheme.typography.titleSmall)
            Text(versionLine(v) + " · " + clientLabel(v.client))
            play?.let { Text(playText(it)) }
            Text(stringResource(R.string.this_phone), Modifier.padding(top = 8.dp), style = MaterialTheme.typography.titleSmall)
            for (l in phoneLines(e)) Text(l)
            Text(stringResource(R.string.restore_explain), Modifier.padding(top = 8.dp))
        }
    }
}

@Composable
private fun InfoCard(title: String, lines: List<String>) {
    OutlinedCard(Modifier.fillMaxWidth()) {
        Column(Modifier.padding(12.dp)) {
            Text(title, Modifier.semantics { heading() }, style = MaterialTheme.typography.titleSmall)
            for (l in lines) Text(l, style = MaterialTheme.typography.bodySmall)
        }
    }
}

@Composable
private fun ChoiceCard(title: String, lines: List<String>, action: String, modifier: Modifier, onClick: () -> Unit) {
    OutlinedCard(modifier) {
        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(title, Modifier.semantics { heading() }, style = MaterialTheme.typography.titleSmall)
            for (l in lines) Text(l, style = MaterialTheme.typography.bodySmall)
            // tonal: the choices weigh the same, none is the recommended one
            FilledTonalButton(onClick, enabled = App.busy == null) { Text(action) }
        }
    }
}
