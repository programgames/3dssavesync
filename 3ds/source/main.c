/* 3dssavesync 3DS: syncs the Saves with the Server (docs/design.md §4):
 * DS (TWiLight), GB/GBC (TWiLight), GBA (open_agb_firm) and every 3DS game with a save (ADR 0007 to 0009, 0028).
 * Top: list of the Games and their State. Bottom: details, messages, buttons. */
#include <3ds.h>
#include <dirent.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <strings.h>
#include <sys/stat.h>
#include <time.h>
#include <unistd.h>
#include "app_update.h"
#include "errors.h"
#include "files.h"
#include "filter.h"
#include "games.h"
#include "key.h"
#include "list.h"
#include "net.h"
#include "pairing.h"
#include "pokemon.h"
#include "qrcodegen.h"
#include "result.h"
#include "screen.h"
#include "settings.h"
#include "state.h"
#include "text.h"
#include "titles3ds.h"
#include "update.h"
#include "vc.h"
#include "views.h"

#define OLD_DIR "sdmc:/3ds/nds-save-sync" /* name before v3: settings and Bases taken over at first start */
#define FORWARDERS "sdmc:/mGBA/forwarders" /* Saves of the mGBA forwarders */
#define CUSTOM_CAS "sdmc:/config/ssl/cacert.pem" /* the user's CA certificates, as in other homebrew (ADR 0029) */

int english; /* console language: French, otherwise English (views.h) */
static u8 language;   /* CFG_LANGUAGE_*: title of the 3DS games in this language */

typedef struct {
    char key[KEY_MAX]; /* ADR 0007 */
    system_t system;
    char rom[600];  /* ROM (DS icon); empty for the 3DS */
    char sav[600];  /* Save file; empty for the 3DS (Save archive) */
    char sav_rom[600]; /* GB/GBA: its Save without the mGBA forwarder (saves folder) */
    u64 title;      /* 3DS and GB Virtual Console: title ID; 0 for a ROM */
    FS_MediaType media; /* 3DS: SD or game card */
    char name[256];  /* ROM name without extension = name of the .sav */
    char short_name[48]; /* for display only: "Pokemon Diamant" */
    int rom_count;
    int shares_save;    /* Duplicate: ROMs with different Keys that share one Save file (same_save) */
    unsigned long local_size, server_size;
    char local_hash[65], server_hash[65], date[17], client[17];
    long age; /* seconds since the Current version (server clock), -1 if unknown */
    int has_local, has_server;
    int has_name;      /* the server knows the game name (ADR 0018) */
    int blank; /* Blank local SRAM: counts as absent (key.h) */
    time_t modified;   /* DS, GB, GBA: file date (3DS clock); 0 if unknown */
    int has_progress;   /* Pokémon: play time and trainer read (pokemon.h) */
    int mbc2;       /* GB: MBC2 cartridge, byte 0x147 of the ROM (ADR 0012) */
    int unexpected; /* Virtual Console: other files than sav.dat in the archive (vc_root_ok): not synced */
    char why[160];  /* Virtual Console with an empty key: the step where reading its ROM failed */
    progress_t progress;
    progress_t server_progress; /* Pokémon: play time of the server version whose hash is progress_hash */
    char progress_hash[65];     /* server version read for server_progress; "" none */
    int has_server_progress;    /* that version is a known Pokémon game */
    unsigned char vc_cart, vc_ram; /* Virtual Console: cartridge type (0x147) and RAM size (0x149) of its ROM */
    int vc_archive;    /* Virtual Console with no sav.dat: 1 if it has a Save archive anyway (then never written) */
    status_t status;
} game_t;

/* "Pokemon - Version Diamant (France) (Rev 5)" -> "Pokemon Diamant": drops " - Version" and the "(...)" tags. */
static void make_short_name(const char *name, char *out, size_t n)
{
    const char *v = strstr(name, " - Version ");
    char tmp[256];
    char *end;
    if (v) snprintf(tmp, sizeof tmp, "%.*s %s", (int)(v - name), name, v + 11);
    else snprintf(tmp, sizeof tmp, "%s", name);
    if ((end = strstr(tmp, " ("))) *end = '\0';
    snprintf(out, n, "%.*s", (int)n - 1, tmp); /* cut on purpose: display name */
}

static config_t cfg;
static unsigned long long off[MAX_OFF]; /* 3DS games turned off (titres-off.ini, ADR 0028) */
static int off_count;
/* No limit on the number of Games: games and the lists of indexes grow on the heap together (new_game). */
static game_t *games;
static int game_count, game_cap;
static bases_t bases;
static int *visible, visible_count; /* Games with a status (not hidden), sorted: what Sync all goes through */
static int *shown, shown_count;     /* the visible Games of the tab that match the search: the list on screen */
static int *outdated;               /* compute_states */
static tab_t tab;                   /* tab of the list (L, R) */
static int tab_counts[TAB_COUNT];   /* Games of each tab that match the search */
static status_t todo_status = STATUS_IN_SYNC; /* most urgent status of the To do tab: color of its count */
static char query[48];              /* search of the list (Search button), empty: none */
static char message[512];        /* message of the last action: band above the button bar */
static char message_detail[200]; /* its technical line (HTTP, curl, path), in grey under the text */
static u32 message_color = WHITE; /* band mark: red failure, green success, white information */

static void set_message(u32 color, const char *detail, const char *format, ...)
{
    va_list a;
    va_start(a, format);
    vsnprintf(message, sizeof message, format, a);
    va_end(a);
    message_color = color;
    snprintf(message_detail, sizeof message_detail, "%s", detail ? detail : "");
}

static void clear_message(void)
{
    message[0] = message_detail[0] = '\0';
    message_color = WHITE;
}

static int should_quit;          /* update installed (app_update.h): the app closes */
static char alert[300]; /* problem seen during a read or an action: added to the message at the end */
static char alert_detail[200]; /* technical line of the failed server read */

static void add_alert(const char *text)
{
    if (!strstr(alert, text))
        snprintf(alert + strlen(alert), sizeof alert - strlen(alert), "%s%s", alert[0] ? "\n" : "", text);
}

/* One line of journal.log (English, for a bug report): date, then the event. The file stays small (append_log). */
static void log_event(const char *format, ...)
{
    char line[1000];
    time_t t = time(NULL);
    va_list a;
    size_t n = strftime(line, sizeof line, "%Y-%m-%d %H:%M:%S ", localtime(&t));
    va_start(a, format);
    vsnprintf(line + n, sizeof line - n, format, a);
    va_end(a);
    append_log(APP_DIR "/journal.log", line, 128 * 1024);
}

static void show_alert(void)
{
    if (!alert[0]) return;
    snprintf(message + strlen(message), sizeof message - strlen(message), "%s%s", message[0] ? "\n" : "", alert);
    message_color = C_RED;
    if (!message_detail[0]) snprintf(message_detail, sizeof message_detail, "%s", alert_detail);
    alert[0] = alert_detail[0] = '\0';
}

static int list_cursor; /* game selected in the list: the top screen of dialogs and progress */
static int loaded;            /* 0 until the first read is done */
static int server_read;    /* 0 if the last server read failed: the local games stay listed */
static u64 read_time;      /* osGetTime() of the last successful server read */
static void draw_list_top(void);

/* ---------------------------------------------------------------- config and Base */

static void save_config(void)
{
    mkdir(APP_DIR, 0777);
    if (write_config(APP_DIR "/config.ini", &cfg))
        add_alert(T("Réglages non enregistrés. Vérifiez la carte SD.", "Settings not saved. Check the SD card."));
}

/* The only write of the Base (design §1): after In sync, or a successful Upload or Download. */
static void save_base(const char *key, const char *hash)
{
    if (set_base(APP_DIR "/base.ini", &bases, key, hash)) /* a power cut during the write no longer empties base.ini */
        add_alert(T("État de synchronisation non enregistré : des conflits peuvent apparaître. Vérifiez la carte SD.",
                  "Sync state not saved: conflicts may appear. Check the SD card."));
}

/* ---------------------------------------------------------------- observation */

static const char *SYSTEM_NAMES[] = { "DS", "GB", "GBA", "3DS" };

static int has_rom_extension(system_t s, const char *ext)
{
    switch (s) {
    case SYS_DS: return !strcasecmp(ext, ".nds");
    case SYS_GB: return !strcasecmp(ext, ".gb") || !strcasecmp(ext, ".gbc");
    case SYS_GBA: return !strcasecmp(ext, ".gba");
    default: return 0;
    }
}

/* GB/GBC Virtual Console game (scan_vc). */
static int vc(const game_t *j) { return j->system == SYS_GB && j->title; }

/* 3DS game synced as its whole save archive, Key 3DSA- (ADR 0028): every 3DS game but the 8 Pokémon (/main, 3DS-). */
static int whole_archive(const game_t *j) { return j->system == SYS_3DS && !pokemon_3ds(j->title); }

/* Name of the Game's Base in base.ini. A Virtual Console game is a Local save of its own next to a ROM or forwarder
 * with the same Key (ADR 0025): each has its Base, and the server links them like two devices. "VC-" + title ID: never
 * a Key (they start with DS-, GB-, GBA-, 3DS-). */
static const char *base_id(const game_t *j)
{
    static char id[KEY_MAX];
    if (!vc(j)) return j->key;
    snprintf(id, sizeof id, "VC-%016llX", j->title);
    return id;
}

/* Whole content of the Local save: the file (Clock trailer included), /main of a 3DS Pokémon Save archive, the whole
 * archive of another 3DS game (archive.h; -2 if it can't be synced: why), or /sav.dat of the Virtual Console one (VC
 * Clock footer included). 0 if read. */
static int read_save(game_t *j, unsigned char **bytes, size_t *size)
{
    if (whole_archive(j)) return read_title_archive(j->title, &j->media, bytes, size, j->why, sizeof j->why);
    if (j->title) return read_title_save(j->title, vc(j) ? "/sav.dat" : "/main", &j->media, bytes, size);
    return read_file(j->sav, bytes, size);
}

static int forwarder(const game_t *j) { return !strncmp(j->sav, FORWARDERS "/", sizeof FORWARDERS); }

/* SRAM of the local file -> server form (ADR 0012): open_agb_firm EEPROM put back in mGBA order, MBC2 as
 * 512 bytes. Backup copies keep the bytes of the file. */
static void to_server_form(const game_t *j, unsigned char **bytes, size_t *size)
{
    unsigned char *s;
    if (j->system == SYS_GBA && !forwarder(j)) eeprom_swap(*bytes, *size);
    if (!j->mbc2 || !(s = malloc(MBC2_SERVER))) return;
    if (mbc2_normalize(*bytes, *size, s)) { /* unknown size: as is */
        free(s);
        return;
    }
    free(*bytes);
    *bytes = s;
    *size = MBC2_SERVER;
}

/* Server form -> SRAM of the local file: the reverse of to_server_form. MBC2 at the size of the existing file (target),
 * otherwise 256 bytes for the mGBA forwarder and 8 KiB for TWiLight Menu++. 0 if done. */
static int to_file_form(const game_t *j, unsigned char **bytes, size_t *size, size_t target)
{
    unsigned char s[MBC2_SERVER], *f;
    if (j->system == SYS_GBA && !forwarder(j)) eeprom_swap(*bytes, *size);
    if (!j->mbc2 || mbc2_normalize(*bytes, *size, s)) return 0;
    if (target != 256 && target != 512 && target != 8192) target = forwarder(j) ? 256 : 8192;
    if (!(f = malloc(target))) return -1;
    mbc2_to_file(s, f, target);
    free(*bytes);
    *bytes = f;
    *size = target;
    return 0;
}

/* Folders never read: saves, hidden, and those of TWiLight Menu++ and the system (_nds, 3ds, luma, Nintendo 3DS, mGBA). */
static int is_ignored_dir(const char *name)
{
    return name[0] == '.' || name[0] == '_' || !strcasecmp(name, "saves") || !strcasecmp(name, "Nintendo 3DS") ||
           !strcasecmp(name, "3ds") || !strcasecmp(name, "luma") || !strcasecmp(name, "mGBA");
}

static int is_dir(const struct dirent *e, const char *path)
{
    struct stat st;
    if (e->d_type != DT_UNKNOWN) return e->d_type == DT_DIR;
    return !stat(path, &st) && S_ISDIR(st.st_mode);
}

/* One ROM source, subfolders included (ADR 0015): Key read in the header (or Alias), Save named like the
 * ROM (save_path). Two ROMs with the same Key, or the same Save file (mGBA forwarder included): Duplicate. */
#define MAX_DEPTH 4 /* levels of subfolders read */
static aliases_t aliases; /* alias.ini (ADR 0014) */

/* Room for one more Game (capacity doubled), cleared: games[game_count]. NULL if out of memory: reported, the scan
 * goes on without it. */
static game_t *new_game(void)
{
    int **lists[] = { &visible, &shown, &outdated }, cap = game_cap ? game_cap * 2 : 64, i;
    void *p = NULL;
    if (game_count == game_cap) {
        if ((p = realloc(games, cap * sizeof *games))) games = p;
        for (i = 0; p && i < 3; i++)
            if ((p = realloc(*lists[i], cap * sizeof **lists[i]))) *lists[i] = p;
        if (!p) {
            add_alert(T("Mémoire insuffisante : des jeux manquent dans la liste. Redémarrez la 3DS, puis relancez 3dssavesync.",
                        "Out of memory: some games are missing from the list. Restart the 3DS, then start 3dssavesync again."));
            snprintf(alert_detail, sizeof alert_detail, T("realloc impossible après %d jeux", "realloc failed after %d games"), game_count);
            return NULL;
        }
        game_cap = cap;
    }
    memset(&games[game_count], 0, sizeof *games);
    return &games[game_count];
}

static void scan_dir(system_t s, const char *roms, const char *saves, const char *dir, int depth)
{
    DIR *d = opendir(dir);
    struct dirent *e;

    while (d && (e = readdir(d))) {
        char path[600], sav[600], name[256], key[KEY_MAX];
        unsigned char header[0x200];
        const char *ext = strrchr(e->d_name, '.'), *a;
        game_t *j = NULL;
        size_t n;
        FILE *f;
        int i;

        if (snprintf(path, sizeof path, "%s/%s", dir, e->d_name) >= (int)sizeof path) continue;
        if (is_dir(e, path)) {
            if (depth < MAX_DEPTH && !is_ignored_dir(e->d_name)) scan_dir(s, roms, saves, path, depth + 1);
            continue;
        }
        if (!ext || !has_rom_extension(s, ext)) continue;
        snprintf(name, sizeof name, "%.*s", (int)(ext - e->d_name), e->d_name);
        if (save_path(roms, saves, dir, name, sav, sizeof sav)) continue;
        if (!(f = fopen(path, "rb"))) continue;
        n = fread(header, 1, sizeof header, f);
        fclose(f);
        if (n < sizeof header) continue;
        if ((a = alias_of(&aliases, name))) {
            if (alias_key(s, a, key)) {
                char t[200];
                snprintf(t, sizeof t, T("Alias invalide pour « %.40s » : ROM non synchronisée. Préparez de nouveau la carte SD depuis le PC.",
                                        "Invalid alias for \"%.40s\": ROM not synced. Prepare the SD card again from the PC."), name);
                add_alert(t);
                continue;
            }
        } else if (rom_key(s, header, key)) continue; /* homebrew or unreadable header */

        for (i = 0; i < game_count; i++)
            if (!strcmp(games[i].key, key) || same_save(games[i].system, games[i].name, games[i].sav_rom, s, name, sav)) j = &games[i];
        if (j) {
            j->rom_count++;
            j->shares_save |= strcmp(j->key, key) != 0;
            continue;
        }
        if (!(j = new_game())) continue;
        game_count++;
        snprintf(j->key, sizeof j->key, "%s", key);
        j->system = s;
        snprintf(j->name, sizeof j->name, "%s", name);
        snprintf(j->rom, sizeof j->rom, "%s", path);
        snprintf(j->sav_rom, sizeof j->sav_rom, "%s", sav);
        make_short_name(j->name, j->short_name, sizeof j->short_name);
        j->rom_count = 1;
        j->mbc2 = s == SYS_GB && rom_mbc2(header);
        if (game_count % 10 == 0) show_progress(0, 0, T("Lecture des ROM (%d)…", "Reading ROMs (%d)…"), game_count);
    }
    if (d) closedir(d);
}

/* Local save of a Game: its file (mGBA forwarder if it exists, docs/guide-forwarders.md) and its Hash
 * (SRAM only for GB/GBA, ADR 0008; /main for the 3DS). Called at scan, and after an action on this Game only. */
static void read_local(game_t *j)
{
    unsigned char *bytes;
    size_t size;
    u64 mtime;
    int r = 0;
    j->has_local = j->blank = j->has_progress = j->unexpected = 0;
    j->modified = 0;
    j->local_size = 0;
    j->local_hash[0] = '\0';
    if (j->system == SYS_DS) snprintf(j->sav, sizeof j->sav, "%s", j->sav_rom);
    else if (!j->title) {
        char f[600];
        snprintf(f, sizeof f, FORWARDERS "/%s.sav", j->name);
        snprintf(j->sav, sizeof j->sav, "%s", access(f, F_OK) == 0 ? f : j->sav_rom);
    }
    if (j->rom_count != 1 || (r = read_save(j, &bytes, &size))) {
        j->unexpected = whole_archive(j) && r == -2; /* listed Not synced, with why */
        if (vc(j)) { /* no sav.dat: a game never started has no archive at all, and may get its first save */
            char root[128];
            j->vc_archive = title_root(j->title, root, sizeof root) >= 0;
        }
        return;
    }
    if (vc(j)) { /* a savestate (rsm*.dat) could bring an old SRAM back: such a game is not synced */
        char root[128] = "";
        title_root(j->title, root, sizeof root);
        if ((j->unexpected = !vc_root_ok(root))) {
            free(bytes);
            return;
        }
    }
    if (j->system != SYS_3DS) {
        size = sram_size(j->system, size);
        to_server_form(j, &bytes, &size);
        j->blank = is_blank_save(j->system, bytes, size);
        /* stat() does not give the date on the SD. archive_getmtime converts it to UNIX time; a 0 timestamp (Azahar)
         * becomes 1 January 2000: date unknown. */
        if (!j->title && R_SUCCEEDED(archive_getmtime(j->sav, &mtime)) && mtime > 946684800) j->modified = (time_t)mtime;
    }
    j->has_progress = !whole_archive(j) && pokemon_progress(bytes, size, &j->progress) == 0; /* SRAM only: without the Clock trailer */
    hash_hex(bytes, size, j->local_hash);
    j->local_size = size;
    j->has_local = 1;
    free(bytes);
}

static int is_off(u64 id)
{
    int i;
    for (i = 0; i < off_count && off[i] != id; i++);
    return i < off_count;
}

/* titres-off.ini written. 0 if done. */
static int save_off(void)
{
    mkdir(APP_DIR, 0777);
    return write_off_titles(APP_DIR "/titres-off.ini", off, off_count);
}

/* path or its .tmp exists (read_ini reads the .tmp after a power cut). */
static int ini_exists(const char *path)
{
    char tmp[300];
    snprintf(tmp, sizeof tmp, "%s.tmp", path);
    return access(path, F_OK) == 0 || access(tmp, F_OK) == 0;
}

/* A 3DS application that the sync may take (ADR 0028): not a GB/GBC Virtual Console game (scan_vc), not in the skip list. */
static int syncable_title(u64 id, FS_MediaType media) { return !skipped_3ds(id) && !(media == MEDIATYPE_SD && title_vc(id)); }

/* titres.ini (the allow-list before ADR 0028) read once: the games found (with a Save archive) that it does not list
 * are turned off, so that the old choice stays. Then titres-off.ini exists, and titres.ini is no longer read. */
static void migrate_titles(const u64 *ids, const FS_MediaType *medias, int n)
{
    static title_t old[MAX_TITLES];
    char root[16];
    int count, i, before = off_count;
    if (n < 0 || !ini_exists(APP_DIR "/titres.ini") || ini_exists(APP_DIR "/titres-off.ini")) return; /* games not listed: next time */
    count = read_titles(APP_DIR "/titres.ini", old, MAX_TITLES);
    for (i = 0; i < n && off_count < MAX_OFF; i++)
        if (syncable_title(ids[i], medias[i]) && title_index(old, count, ids[i]) < 0 && !is_off(ids[i]) && title_root(ids[i], root, sizeof root) >= 0)
            off[off_count++] = ids[i];
    if (save_off()) /* the games stay off for this session; titres.ini is read again next time */
        add_alert(T("Liste des jeux 3DS désactivés non enregistrée. Vérifiez la carte SD.", "List of turned-off 3DS games not saved. Check the SD card."));
    else log_event("titres.ini read once: %d 3DS games turned off", off_count - before);
}

/* The 3DS games (ADR 0028): every application with a Save archive, installed or on the inserted game card, but the
 * Virtual Console games, the skip list and the games turned off. The 8 Pokémon games: /main, Key 3DS-; every other
 * game: its whole archive, Key 3DSA-. A game never started has no archive: not listed. */
static void scan_3ds(void)
{
    u64 *ids = NULL;
    FS_MediaType *medias = NULL;
    int n = -1, i, k;
    if (R_SUCCEEDED(amInit())) {
        n = installed_titles(&ids, &medias, 0);
        amExit();
    }
    if (n < 0)
        add_alert(T("Jeux 3DS non lus : la liste des jeux installés est illisible. Redémarrez la 3DS, puis relancez 3dssavesync.",
                    "3DS games not read: the installed games can't be listed. Restart the 3DS, then start 3dssavesync again."));
    migrate_titles(ids, medias, n);
    for (i = 0; i < n; i++) {
        char name[256];
        game_t *j;
        if (is_off(ids[i]) || !syncable_title(ids[i], medias[i])) continue;
        for (k = 0; k < game_count && !(games[k].system == SYS_3DS && games[k].title == ids[i]); k++);
        if (k < game_count) continue; /* installed and on the game card: one game (read_save reads the SD card first) */
        if (!(j = new_game())) break;
        if (title_name(ids[i], medias[i], language, name, sizeof name)) snprintf(name, sizeof name, "%016llX", ids[i]);
        show_progress(i + 1, n, T("Lecture des sauvegardes 3DS\n%s", "Reading 3DS saves\n%s"), name);
        j->title = ids[i];
        j->system = SYS_3DS;
        j->rom_count = 1;
        read_local(j); /* Hash, and play time of a Pokémon */
        if (!j->has_local && !j->unexpected) continue; /* never started */
        if (pokemon_3ds(j->title)) key_3ds(j->title, j->key);
        else key_3dsa(j->title, j->key);
        snprintf(j->name, sizeof j->name, "%s", name);
        snprintf(j->short_name, sizeof j->short_name, "%.47s", name);
        game_count++;
    }
    free(ids);
    free(medias);
}

/* GB/GBC Virtual Console games installed on the SD card (title_vc, by product code): a GB Game whose Key is read in the
 * ROM of its RomFS, and whose Save is /sav.dat of its Save archive (read_save). A ROM or forwarder with the same Key is
 * another Local save of the same game, synced on its own (base_id, ADR 0025). ROM not read: listed Not synced with an
 * empty key and the failed step in why, never a guessed Key. */
static void scan_vc(void)
{
    u64 *ids = NULL;
    FS_MediaType *medias = NULL;
    int n = -1, i, k;
    if (R_SUCCEEDED(amInit())) {
        n = installed_titles(&ids, &medias, 0);
        amExit();
    }
    if (n < 0)
        add_alert(T("Jeux Virtual Console non lus : la liste des jeux installés est illisible. Redémarrez la 3DS, puis relancez 3dssavesync.",
                    "Virtual Console games not read: the installed games can't be listed. Restart the 3DS, then start 3dssavesync again."));
    for (i = 0; i < n; i++) {
        unsigned char header[0x200];
        char key[KEY_MAX] = "", name[256], why[160];
        game_t *j = NULL;
        int read;
        if (medias[i] != MEDIATYPE_SD || !title_vc(ids[i])) continue;
        if (title_name(ids[i], MEDIATYPE_SD, language, name, sizeof name)) snprintf(name, sizeof name, "%016llX", ids[i]);
        show_progress(0, 0, T("Lecture des jeux Virtual Console\n%s", "Reading Virtual Console games\n%s"), name);
        read = !title_rom_header(ids[i], header, sizeof header, why, sizeof why);
        if (read && rom_key(SYS_GB, header, key)) {
            key[0] = '\0';
            snprintf(why, sizeof why, "rom_key: no title");
        }
        for (k = 0; key[0] && k < game_count; k++)
            if (vc(&games[k]) && !strcmp(games[k].key, key)) j = &games[k];
        if (j) { /* two VC titles with one Key: Duplicate, as two ROMs */
            j->rom_count++;
            continue;
        }
        if (!(j = new_game())) break;
        game_count++;
        snprintf(j->key, sizeof j->key, "%s", key);
        snprintf(j->why, sizeof j->why, "%s", why);
        j->system = SYS_GB;
        j->title = ids[i];
        j->media = MEDIATYPE_SD;
        snprintf(j->name, sizeof j->name, "%s", name);
        make_short_name(j->name, j->short_name, sizeof j->short_name);
        j->rom_count = 1;
        j->mbc2 = read && rom_mbc2(header);
        j->vc_cart = read ? header[0x147] : 0;
        j->vc_ram = read ? header[0x149] : 0;
    }
    free(ids);
    free(medias);
}

/* Install date of each 3DS and Virtual Console game on the SD card (ADR 0027). A game installed again lost its save
 * with the old install: its Base is removed, so the new save asks before it replaces the server's. */
static void note_installs(void)
{
    int i, r, changed = 0;
    for (i = 0; i < game_count; i++) {
        game_t *j = &games[i];
        if (!j->title || j->media != MEDIATYPE_SD) continue;
        r = note_install(&bases, j->title, title_install_time(j->title), base_id(j));
        changed |= r > 0;
        if (r == 2) {
            char t[400];
            log_event("reinstalled %016llX \"%s\": Base %s removed", j->title, j->name, base_id(j));
            snprintf(t, sizeof t, T("%s a été réinstallé. Si sa sauvegarde diffère de celle du serveur, vous choisirez laquelle garder.",
                                    "%s was installed again. If its save differs from the server's, you'll choose which one to keep."), j->short_name);
            add_alert(t);
        }
    }
    if (changed && write_bases(APP_DIR "/base.ini", &bases))
        add_alert(T("État de synchronisation non enregistré : des conflits peuvent apparaître. Vérifiez la carte SD.",
                  "Sync state not saved: conflicts may appear. Check the SD card."));
}

/* All sources, then the Hash of each Local save (SRAM only for GB/GBA, ADR 0008). */
static void scan(void)
{
    int i, before;

    game_count = visible_count = shown_count = loaded = 0; /* the list shown is no longer valid during the read */
    read_aliases(APP_DIR "/alias.ini", &aliases);
    scan_dir(SYS_DS, cfg.roms, cfg.saves, cfg.roms, 0); /* empty saves = next to the ROM */
    scan_dir(SYS_GB, cfg.roms_gb, cfg.saves_gb, cfg.roms_gb, 0);
    scan_dir(SYS_GBA, cfg.roms_gba, cfg.saves_gba, cfg.roms_gba, 0);
    scan_vc();
    before = game_count;
    for (i = 0; i < before; i++) {
        show_progress(i + 1, before, T("Lecture des sauvegardes\n%s", "Reading saves\n%s"), games[i].short_name);
        read_local(&games[i]);
    }
    scan_3ds();
    note_installs();
}

/* GET /saves: Current version of each Game. 0 if all is well. */
static int read_server(void)
{
    char url[300], *body;
    const char *p;
    entry_t e;
    size_t size;
    long status;
    int i;

    for (i = 0; i < game_count; i++) games[i].has_server = 0;
    server_read = 0;
    snprintf(url, sizeof url, "%s/saves", cfg.url);
    status = net_get(url, cfg.token, &body, &size);
    if (status != 200) {
        char t[200];
        snprintf(t, sizeof t, T("Lecture du serveur impossible. %s", "Cannot read the server. %s"), network_text(status, net_wifi(), english));
        add_alert(t); /* added to the message of the current action (show_alert) */
        network_detail(status, url, alert_detail, sizeof alert_detail);
        if (status > 0) free(body);
        return -1;
    }
    server_read = 1;
    read_time = osGetTime();
    /* [{"code": "IRBF", "version": "...", "hash": "...", "taille": 524288, "date": "...", "client": "pc"}, ...] (list.h) */
    for (p = body; next_server_entry(&p, &e);) {
        if (!e.hash[0] || e.size < 0) continue;
        for (i = 0; i < game_count; i++) { /* every Game of the Key: a ROM or forwarder and a Virtual Console game */
            if (strcmp(games[i].key, e.code)) continue; /* no local ROM: not shown */
            strcpy(games[i].server_hash, e.hash);
            games[i].server_size = e.size;
            strcpy(games[i].date, e.date);
            strcpy(games[i].client, e.client);
            games[i].age = e.age;
            games[i].has_server = 1;
            games[i].has_name = e.has_name;
        }
    }
    free(body);
    /* Name of each game for the History on the PC (ADR 0018), once per game. Stop at the first refusal: an older
     * server does not know this address, and the list stays readable without the names. */
    for (i = 0; i < game_count; i++) {
        if (!games[i].has_server || games[i].has_name) continue;
        show_progress(i + 1, game_count, T("Envoi des noms des jeux\n%s", "Sending game names\n%s"), games[i].short_name);
        snprintf(url, sizeof url, "%s/saves/%.31s/name", cfg.url, games[i].key); /* Key: 31 characters at most (KEY_MAX) */
        if (net_put(url, cfg.token, "3ds", NULL, games[i].name, strlen(games[i].name)) != 204) break;
        games[i].has_name = 1;
    }
    return 0;
}

static row_t row_of(const game_t *j)
{
    row_t r = { j->status, j->system, j->short_name, j->modified };
    return r;
}

static int compare_visible(const void *a, const void *b)
{
    const row_t x = row_of(&games[*(const int *)a]), y = row_of(&games[*(const int *)b]);
    return compare_rows((order_t)cfg.order, &x, &y);
}

/* Index of v in a[0..n), or -1. */
static int index_of(const int *a, int n, int v)
{
    int i;
    for (i = 0; i < n && a[i] != v; i++);
    return i < n ? i : -1;
}

/* shown and the counts of the tabs, from visible: the search filters every tab (full name or display name). */
static void filter_list(void)
{
    int i, t;
    memset(tab_counts, 0, sizeof tab_counts);
    todo_status = STATUS_IN_SYNC;
    shown_count = 0;
    for (i = 0; i < visible_count; i++) {
        const game_t *j = &games[visible[i]];
        const row_t r = row_of(j);
        if (!name_matches(j->short_name, query) && !name_matches(j->name, query)) continue;
        for (t = 0; t < TAB_COUNT; t++) tab_counts[t] += in_tab((tab_t)t, &r);
        if (in_tab(TAB_TODO, &r) && status_priority(r.status) < status_priority(todo_status)) todo_status = r.status;
        if (in_tab(tab, &r)) shown[shown_count++] = visible[i];
    }
}

/* filter_list after a change of tab or search: the cursor stays on the same game if it is still shown, else the first. */
static int refilter(int cursor)
{
    int selected = shown_count ? shown[cursor] : -1;
    filter_list();
    cursor = index_of(shown, shown_count, selected);
    return cursor < 0 ? 0 : cursor;
}

static void sort_list(void)
{
    if (visible_count) qsort(visible, visible_count, sizeof *visible, compare_visible);
    filter_list();
}

/* Status of each Game and the sorted list of the visible ones. No write: returns the number of Games whose Base must
 * become their Local save (In sync), their indexes in outdated. */
static int compute_states(int *outdated)
{
    int i, n = 0;
    visible_count = 0;
    for (i = 0; i < game_count; i++) {
        game_t *j = &games[i];
        const obs_t local = { j->local_hash, j->local_size }, server = { j->server_hash, j->server_size };
        const observation_t o = { j->rom_count, j->has_local ? &local : NULL, j->blank, j->has_server ? &server : NULL };
        j->status = j->unexpected || !j->key[0] ? STATUS_NOT_SYNCED : game_status(&o, base_of(&bases, base_id(j)), server_read);
        if (base_outdated(j->status, base_of(&bases, base_id(j)), j->local_hash)) outdated[n++] = i;
        if (j->status != STATUS_HIDDEN) visible[visible_count++] = i;
    }
    sort_list();
    return n;
}

/* compute_states, then the Bases written (the only write of an In sync Base). */
static void update_states(void)
{
    int i, n = compute_states(outdated);
    for (i = 0; i < n; i++) save_base(base_id(&games[outdated[i]]), games[outdated[i]].local_hash);
}

static void read_server_progresses(void);

/* After an action: the Game it touched is already read again (read_local), only the server changes. */
static void reread_server(void)
{
    show_progress(0, 0, "%s", T("Lecture du serveur…", "Reading the server…"));
    read_server();
    update_states(); /* server not read: the local games stay listed, with no action */
    if (server_read) read_server_progresses();
    loaded = 1;
}

/* Read everything again: ROMs, Saves, server (Y, settings changed). */
static void refresh(void)
{
    scan();
    reread_server();
}

/* ---------------------------------------------------------------- actions */

/* Result with the file of the failure (or NULL). */
static result_t make_result(result_code_t code, const char *path)
{
    result_t r = { code, 0, 1, 0, 0, NULL, "" };
    snprintf(r.path, sizeof r.path, "%s", path ? path : "");
    return r;
}

/* Failed request: its status, whether the 3DS is on a Wi-Fi access point, and its address for the technical line. */
static result_t network_failure(result_code_t code, long status, const char *url)
{
    result_t r = make_result(code, url);
    r.status = status;
    r.wifi = net_wifi();
    return r;
}

/* The message band says the result of an action on the Game j. */
static void show_result(const result_t *r, const game_t *j)
{
    static const u32 COLORS[] = { [KIND_INFO] = WHITE, [KIND_SUCCESS] = C_GREEN, [KIND_FAILURE] = C_RED };
    message_color = COLORS[result_text(r, j->short_name, english, message, sizeof message, message_detail, sizeof message_detail)];
}

/* Conditional Upload: If-Match = Current version seen by this Client (ADR 0001).
 * GB/GBA: only the SRAM goes, the Clock trailer stays on the 3DS (ADR 0008). */
static result_t upload_save(game_t *j)
{
    char url[300], condition[100], hash[65];
    unsigned char *bytes;
    size_t size;
    long status;

    if (read_save(j, &bytes, &size)) return make_result(RESULT_UPLOAD_UNREADABLE, NULL);
    size = sram_size(j->system, size);
    to_server_form(j, &bytes, &size);
    hash_hex(bytes, size, hash);
    if (j->has_server) snprintf(condition, sizeof condition, "If-Match: \"%s\"", j->server_hash);
    else snprintf(condition, sizeof condition, "If-None-Match: *");
    snprintf(url, sizeof url, "%s/saves/%s", cfg.url, j->key);
    status = net_put(url, cfg.token, "3ds", condition, bytes, size);
    free(bytes);
    if (status == 412) return make_result(RESULT_UPLOAD_CONFLICT, NULL);
    if (status != 200) return network_failure(RESULT_UPLOAD_FAILED, status, url);
    save_base(base_id(j), hash);
    return make_result(RESULT_UPLOADED, NULL);
}

/* Creates the folder that holds path if it is missing (saves folder of a first game). */
static void make_parent_dir(const char *path)
{
    char d[600], *end;
    snprintf(d, sizeof d, "%s", path);
    if ((end = strrchr(d, '/'))) {
        *end = '\0';
        mkdir(d, 0777);
    }
}

/* Download (design §4): Backup copy first, then
 *  - file (DS, GB, GBA): received SRAM + local Clock trailer (ADR 0008) in a .tmp, then replace;
 *  - 3DS: rewrite /main with commit and Secure value (ADR 0009); other 3DS games: the whole archive, commit, Secure
 *    value, then read back (write_title_archive, ADR 0028);
 *  - Virtual Console: received SRAM + the VC footer of the old sav.dat, rewritten like /main, then read back
 *    (ADR 0025). A VC game never started (no archive) gets its first save: create_vc_first_save.
 * On failure, the Local save is intact. */
/* SRAM size of a Virtual Console game from its ROM header (byte 0x149), 0 if none or not handled (MBC2: 0x05, 0x06). */
static size_t vc_sram_size(const game_t *j)
{
    static const size_t SIZES[] = { 0, 2048, 8192, 32768, 131072, 65536 };
    if (j->vc_cart == 0x05 || j->vc_cart == 0x06 || j->vc_ram >= sizeof SIZES / sizeof *SIZES) return 0;
    return SIZES[j->vc_ram];
}

/* First save of a Virtual Console game never started (no Save archive): the archive is created
 * (create_vc_save) with /sav.dat = received SRAM + a zero footer for a clock cartridge (MBC3+TIMER 0x0F, 0x10), then
 * read back. Nothing existed, so there is nothing to copy first. */
static result_t create_vc_first_save(game_t *j, char *body, size_t size)
{
    size_t sram = vc_sram_size(j), total = sram + (j->vc_cart == 0x0F || j->vc_cart == 0x10 ? 16 : 0), back_size = 0;
    unsigned char *data, *back = NULL;
    char code[16];
    Result cr;
    result_t r;
    if (size != sram) { /* the server SRAM must fill the cartridge RAM exactly */
        free(body);
        return make_result(RESULT_3DS_WRITE_REFUSED, NULL);
    }
    if (!(data = calloc(1, total))) {
        free(body);
        return make_result(RESULT_OUT_OF_MEMORY, NULL);
    }
    memcpy(data, body, sram);
    free(body);
    cr = create_vc_save(j->title, data, total);
    if (R_FAILED(cr)) {
        free(data);
        snprintf(code, sizeof code, "%08lX", (unsigned long)cr);
        return make_result(RESULT_WRITE_FAILED, code);
    }
    j->media = MEDIATYPE_SD;
    if (read_save(j, &back, &back_size) || back_size != total || memcmp(back, data, total)) {
        free(back);
        free(data);
        r = make_result(RESULT_WRITTEN_DAMAGED, "sav.dat");
        return r;
    }
    free(back);
    free(data);
    save_base(base_id(j), j->server_hash);
    return make_result(RESULT_DOWNLOADED, NULL);
}

/* name for a file of copies/: the characters the SD card refuses (\ / : * ? " < > |) become '_' ("Zelda: ..." from
 * the SMDH of a 3DS game). */
static void file_name(const char *name, char *out, size_t n)
{
    size_t i;
    for (i = 0; name[i] && i < n - 1; i++) out[i] = strchr("\\/:*?\"<>|", name[i]) ? '_' : name[i];
    out[i] = '\0';
}

static result_t download_save(game_t *j)
{
    char tmp[610], backup[700], url[300], hash[65], date[20], name[256];
    char *body;
    unsigned char *old_data = NULL, *new_data;
    size_t size, n = 0, sram;
    long status;
    int failed, exists = j->system == SYS_3DS || (vc(j) && j->has_local) || access(j->sav, F_OK) == 0;
    time_t t = time(NULL);
    result_t r;

    /* The only local write of a Save goes through here (act, Conflict, Sync all, History). A VC game with an archive
     * but no sav.dat (or an unknown RAM size) is left alone; one with no archive at all gets its first save. */
    if (vc(j) && !j->has_local && (j->vc_archive || !vc_sram_size(j)))
        return make_result(RESULT_VC_NEVER_STARTED, NULL);
    snprintf(url, sizeof url, "%s/saves/%s", cfg.url, j->key);
    status = net_get(url, cfg.token, &body, &size);
    if (status != 200) {
        if (status > 0) free(body);
        return network_failure(RESULT_DOWNLOAD_FAILED, status, url);
    }
    hash_hex((unsigned char *)body, size, hash);
    if (strcmp(hash, j->server_hash)) {
        free(body);
        return make_result(RESULT_DOWNLOAD_DAMAGED, NULL);
    }
    if (vc(j) && !j->has_local) return create_vc_first_save(j, body, size);
    /* Copy as soon as the Save exists, even if the scan could not read it: never overwrite without a copy. */
    if (exists) {
        strftime(date, sizeof date, "%Y%m%d-%H%M%S", localtime(&t));
        mkdir(APP_DIR "/copies", 0777);
        /* same ROM name in two subfolders, the same second: -1, -2… (a copy is never overwritten) */
        file_name(j->name, name, sizeof name);
        failed = backup_path(APP_DIR "/copies", name, date, whole_archive(j) ? "tar" : j->system == SYS_3DS ? "main" : "sav", backup, sizeof backup);
        if (!failed) failed = read_save(j, &old_data, &n);
        if (!failed) failed = write_verified(backup, old_data, n); /* read back: a damaged copy protects nothing */
        if (failed) {
            free(old_data);
            free(body);
            return make_result(RESULT_BACKUP_FAILED, backup);
        }
    }

    if (whole_archive(j)) {
        char why[160];
        free(old_data); /* copied: its memory goes to the read back */
        failed = write_title_archive(j->title, j->media, (unsigned char *)body, size, why, sizeof why);
        free(body);
        if (failed) log_event("write %016llX: %s", j->title, why);
        if (failed == -2) {
            r = make_result(RESULT_WRITTEN_DAMAGED, backup);
            r.had_save = 1;
            return r;
        }
        if (failed) return make_result(RESULT_3DS_WRITE_REFUSED, why);
    } else if (j->system == SYS_3DS) {
        failed = write_title_save(j->title, j->media, "/main", body, size);
        free(old_data);
        free(body);
        if (failed) return make_result(RESULT_3DS_WRITE_REFUSED, NULL);
    } else if (vc(j)) {
        /* Received SRAM + the VC footer of the old sav.dat (16 bytes for Gold, Silver, Crystal; none for Gen 1). */
        unsigned char *received = (unsigned char *)body, *back = NULL;
        size_t back_size = 0;
        sram = sram_size(j->system, n);
        new_data = to_file_form(j, &received, &size, sram) || size != sram ? NULL : malloc(n);
        if (!new_data) { /* a server SRAM of another size would change the size of sav.dat: refused like /main */
            free(old_data);
            free(received);
            return make_result(size != sram ? RESULT_3DS_WRITE_REFUSED : RESULT_OUT_OF_MEMORY, NULL);
        }
        memcpy(new_data, received, size);
        memcpy(new_data + size, old_data + sram, n - sram);
        free(old_data);
        free(received);
        failed = write_title_save(j->title, j->media, "/sav.dat", new_data, n);
        /* read back: the commit is done, so a difference means the archive holds something else (copy kept) */
        if (!failed && (read_save(j, &back, &back_size) || back_size != n || memcmp(back, new_data, n))) failed = 2;
        free(back);
        free(new_data);
        if (failed == 2) {
            r = make_result(RESULT_WRITTEN_DAMAGED, backup);
            r.had_save = 1;
            return r;
        }
        if (failed) return make_result(RESULT_3DS_WRITE_REFUSED, NULL);
    } else {
        /* Received SRAM (file form) + local Clock trailer (what lies past the SRAM of the old file). */
        unsigned char *received = (unsigned char *)body;
        sram = exists ? sram_size(j->system, n) : n;
        new_data = to_file_form(j, &received, &size, sram) ? NULL : malloc(size + (n - sram) + 1);
        if (!new_data) {
            free(old_data);
            free(received);
            return make_result(RESULT_OUT_OF_MEMORY, NULL);
        }
        memcpy(new_data, received, size);
        if (n > sram) memcpy(new_data + size, old_data + sram, n - sram);
        free(old_data);
        free(received);
        make_parent_dir(j->sav); /* saves folder absent (first game) */
        /* .tmp written and read back, then the old one (already copied) removed, the .tmp renamed and read back (files.h). */
        failed = replace_file(j->sav, new_data, size + (n - sram));
        free(new_data);
        if (failed == -1) return make_result(RESULT_WRITE_FAILED, j->sav);
        if (failed) {
            snprintf(tmp, sizeof tmp, "%s.tmp", j->sav);
            r = make_result(access(tmp, F_OK) ? RESULT_WRITTEN_DAMAGED : RESULT_RENAME_FAILED, j->sav); /* renamed, but read back different */
            r.had_save = exists;
            return r;
        }
    }
    save_base(base_id(j), j->server_hash);
    return make_result(RESULT_DOWNLOADED, NULL);
}

/* ---------------------------------------------------------------- screen */

/* 32×32 icon of a DS game, read in the ROM, as RGB565 line by line. 0 if read.
 * NDS banner: its offset is at 0x68 in the header; 4-bit tiles at +0x20, BGR555 palette at +0x220. */
#define ICON_BACKGROUND 0x10C3 /* palette color 0 (transparent): background of the bottom screen (0x14181E) */
static int read_ds_icon(const game_t *j, u16 *pixels)
{
    unsigned char o[4], icon[512 + 32];
    u32 offset;
    FILE *f;
    int t, l, k;

    if (j->system != SYS_DS || !(f = fopen(j->rom, "rb"))) return -1; /* only DS ROMs have a banner */
    offset = fseek(f, 0x68, SEEK_SET) == 0 && fread(o, 1, 4, f) == 4 ? o[0] | o[1] << 8 | o[2] << 16 | (u32)o[3] << 24 : 0;
    if (!offset || fseek(f, offset + 0x20, SEEK_SET) || fread(icon, 1, sizeof icon, f) != sizeof icon) {
        fclose(f);
        return -1;
    }
    fclose(f);
    for (t = 0; t < 16; t++) /* 4×4 tiles of 8×8 */
        for (l = 0; l < 8; l++)
            for (k = 0; k < 8; k++) {
                int idx = (icon[t * 32 + l * 4 + k / 2] >> ((k & 1) * 4)) & 0xF;
                u16 c = icon[512 + idx * 2] | icon[512 + idx * 2 + 1] << 8;
                pixels[((t / 4) * 8 + l) * 32 + (t % 4) * 8 + k] =
                    idx ? (c & 0x1F) << 11 | ((c >> 5) & 0x1F) << 6 | ((c >> 10) & 0x1F) : ICON_BACKGROUND;
            }
    return 0;
}

/* ---------------------------------------------------------------- compare two Versions */

/* "152 h 30 · Sacha", or game not started. */
static void progress_text(const progress_t *p, char *out, size_t n)
{
    if (p->trainer[0]) snprintf(out, n, "%d h %02d \xC2\xB7 %s", p->hours, p->minutes, p->trainer);
    else snprintf(out, n, T("%d h %02d \xC2\xB7 partie non commencée", "%d h %02d \xC2\xB7 not started yet"), p->hours, p->minutes);
}

/* Save on the 3DS: play time (Pokémon), otherwise the file date if the clock makes it credible. */
static void local_text(const game_t *j, char *out, size_t n)
{
    time_t now = time(NULL);
    if (!j->has_local) snprintf(out, n, "%s", T("Aucune sauvegarde", "No save"));
    else if (j->blank) snprintf(out, n, "%s", T("Vierge", "Blank"));
    else if (j->has_progress) progress_text(&j->progress, out, n);
    else if (j->modified && now >= j->modified) {
        char age[24];
        format_age((long)(now - j->modified), english, 0, age, sizeof age);
        snprintf(out, n, T("Modifiée %s", "Modified %s"), age);
    } else snprintf(out, n, "%s", T("Présente, date inconnue", "Found, date unknown"));
}

/* Server version: when, then from which device. */
static void server_text(const game_t *j, char *out, size_t n)
{
    char age[24];
    if (!j->has_server) {
        snprintf(out, n, "%s", server_read ? T("Aucune version", "No version") : T("Non lu", "Not read"));
        return;
    }
    format_age(j->age, english, 1, age, sizeof age);
    if (age[0]) snprintf(out, n, T("%s \xC2\xB7 depuis %s", "%s \xC2\xB7 from %s"), age, j->client[0] ? j->client : "?");
    else snprintf(out, n, T("Depuis %s", "From %s"), j->client[0] ? j->client : "?");
}

/* Play time of the server version, only for a Pokémon game (known on the 3DS, or no save there yet). Read once per
 * server version: from another Game of the same Key (a ROM and its Virtual Console game), else downloaded. 1 if read. */
static int server_progress(game_t *j, progress_t *p)
{
    char url[300], *body;
    size_t size;
    long status;
    int i;
    if (!j->has_server || whole_archive(j) || (!j->has_progress && j->has_local)) return 0;
    if (strcmp(j->progress_hash, j->server_hash)) {
        for (i = 0; i < game_count && strcmp(games[i].progress_hash, j->server_hash); i++);
        if (i < game_count) {
            j->server_progress = games[i].server_progress;
            j->has_server_progress = games[i].has_server_progress;
        } else {
            show_progress(0, 0, "%s\n%s", T("Lecture de la version du serveur", "Reading the server version"), j->short_name);
            snprintf(url, sizeof url, "%s/saves/%s", cfg.url, j->key);
            status = net_get(url, cfg.token, &body, &size);
            if (status != 200) { /* not kept: read again next time */
                if (status > 0) free(body);
                return 0;
            }
            j->has_server_progress = pokemon_progress((unsigned char *)body, size, &j->server_progress) == 0;
            free(body);
        }
        strcpy(j->progress_hash, j->server_hash);
    }
    *p = j->server_progress;
    return j->has_server_progress;
}

/* After a server read: the play time of each server version that would replace or be replaced (Pokémon only), so that
 * the bottom screen shows both before any choice. */
static void read_server_progresses(void)
{
    int i;
    progress_t p;
    for (i = 0; i < game_count; i++)
        if (games[i].status == STATUS_TO_UPLOAD || games[i].status == STATUS_TO_DOWNLOAD || games[i].status == STATUS_CONFLICT)
            server_progress(&games[i], &p);
}

/* ---------------------------------------------------------------- main screen (citro2d) */

static u32 state_color(status_t s)
{
    switch (s) {
    case STATUS_TO_UPLOAD: return C_YELLOW;
    case STATUS_TO_DOWNLOAD: return C_CYAN;
    case STATUS_CONFLICT: return C_RED;
    default: return GREY_LIGHT; /* In sync, Duplicate, Unknown */
    }
}

/* Dot and mark of the State: dark green for In sync, which does not catch the eye. */
static u32 dot_color(status_t s) { return s == STATUS_IN_SYNC ? DARK_GREEN : state_color(s); }

/* One word per State, the same in the list, the counters and the details. */
static const char *state_label(status_t s)
{
    switch (s) {
    case STATUS_TO_UPLOAD: return T("À envoyer", "To upload");
    case STATUS_TO_DOWNLOAD: return T("À récupérer", "To download");
    case STATUS_CONFLICT: return T("Conflit", "Conflict");
    case STATUS_DUPLICATE: return T("Doublon", "Duplicate");
    case STATUS_NOT_SYNCED: return T("Non synchro", "Not synced");
    case STATUS_OFFLINE: return T("Inconnu", "Unknown");
    default: return T("À jour", "In sync");
    }
}

/* Choice card of the Conflict screen (152 × 92), touchable: button and action, the version (same form on both
 * sides), what happens. */
static void draw_choice_card(float x, float y, u32 button, const char *action, const char *version, const char *outcome)
{
    screen_rect(x, y, 152, 92, CARD);
    screen_text(x + 6, y + 4, 0.55f, button == KEY_A ? GLYPH_A : GLYPH_X, 0, glyph(button));
    screen_text(x + 24, y + 5, 0.45f, WHITE, 122, action);
    screen_lines(x + 6, y + 26, 0.42f, WHITE, 140, 3, version);
    screen_lines(x + 6, y + 68, 0.38f, GREY_LIGHT, 140, 2, outcome);
    touch_area(x, y, 152, 92, button);
}

/* Game icon at the top right of the bottom screen. */
static void draw_game_icon(const game_t *j)
{
    u16 icon[ICON_SIZE * ICON_SIZE];
    if (j->title && title_icon(j->title, j->media, icon) == 0) screen_icon(icon, ICON_SIZE, ICON_SIZE, ICON_X, 6, 1.0f); /* 3DS, Virtual Console */
    else if (!j->title && read_ds_icon(j, icon) == 0) screen_icon(icon, 32, 32, ICON_X, 6, 1.5f); /* 32 x 1.5 = 48 */
}

static const char *tab_label(tab_t t)
{
    if (t == TAB_ALL) return T("Tous", "All");
    if (t == TAB_TODO) return T("À faire", "To do");
    return SYSTEM_NAMES[t - TAB_DS];
}

/* Tabs right of the title, between the L and R glyphs: "All 312  To do 12  DS 150…", the current one highlighted, the
 * To do count in the color of its most urgent game. A tab with no game is left out, except All and the current one.
 * Smaller text if they do not fit. */
static void draw_tabs(float left)
{
    char t[TAB_COUNT][32];
    tab_t list[TAB_COUNT];
    float w[TAB_COUNT], size = 0.44f, g, total, x;
    int i, n = 0;
    for (i = 0; i < TAB_COUNT; i++)
        if (i == TAB_ALL || i == (int)tab || tab_counts[i]) {
            list[n] = (tab_t)i;
            snprintf(t[n++], sizeof t[0], "%s %d", tab_label((tab_t)i), tab_counts[i]);
        }
    do {
        size -= 0.02f;
        g = screen_text_width(size + 0.08f, "\xEE\x80\x84") + 2; /* L, R */
        total = 2 * g;
        for (i = 0; i < n; i++) total += w[i] = screen_text_width(size, t[i]) * WIDTH_MARGIN + 10;
    } while (size > 0.34f && total > TOP_WIDTH - 4 - left);
    x = TOP_WIDTH - 4 - total;
    screen_text(x, 4, size + 0.08f, GREY_LIGHT, 0, "\xEE\x80\x84");
    x += g;
    for (i = 0; i < n; i++) {
        const int on = list[i] == tab;
        if (on) {
            screen_rect(x, 3, w[i], 18, SEL);
            screen_rect(x, 20, w[i], 2, ACCENT);
        }
        screen_text(x + 5, 6, size, list[i] == TAB_TODO && tab_counts[TAB_TODO] ? state_color(todo_status) : on ? WHITE : GREY_LIGHT, 0, t[i]);
        x += w[i];
    }
    screen_text(x + 2, 4, size + 0.08f, GREY_LIGHT, 0, "\xEE\x80\x85");
}

/* Lines of a list on the top screen: one less under the search band. */
static int list_lines(const char *q) { return q[0] ? TOP_LINES - 1 : TOP_LINES; }

/* Band of an active search, under the title: what is searched, and B clears it. Returns the y of the first line. */
static float draw_search_band(const char *q)
{
    char t[120];
    if (!q[0]) return LIST_Y;
    snprintf(t, sizeof t, T("Recherche : %s \xC2\xB7 \xEE\x80\x81 pour effacer", "Search: %s \xC2\xB7 \xEE\x80\x81 to clear"), q);
    screen_rect(0, 25, TOP_WIDTH, 15, CARD);
    screen_rect(0, 25, 2, 15, ACCENT);
    screen_text(8, 25, 0.44f, WHITE, TOP_WIDTH - 16, t);
    return LIST_Y + 15;
}

/* Top screen of the list: title and tabs, the search, the games, then the footer: the server, when it was read, the
 * position in the list. */
static void draw_list_top(void)
{
    static const status_t all[] = { STATUS_TO_UPLOAD, STATUS_TO_DOWNLOAD, STATUS_CONFLICT, STATUS_DUPLICATE, STATUS_NOT_SYNCED, STATUS_OFFLINE, STATUS_IN_SYNC };
    const int cursor = list_cursor, lines = list_lines(query);
    int i, start = cursor < lines ? 0 : cursor - lines + 1;
    char footer[200], age[32], pos[24], empty[160];
    const char *host = strstr(cfg.url, "://") ? strstr(cfg.url, "://") + 3 : cfg.url;
    float x, xs = 0, xn, top;

    screen_top();
    x = 6 + screen_text(6, 4, 0.55f, WHITE, 0, "3dssavesync") + 12;
    if (loaded) draw_tabs(x);
    screen_rect(0, 23, TOP_WIDTH, 1, GREY_DARK);
    top = draw_search_band(query);

    /* Columns measured on all State words: nothing is cut, and they do not move from one frame to the next. */
    for (i = 0; i < (int)(sizeof all / sizeof *all); i++)
        if (screen_text_width(0.42f, state_label(all[i])) > xs) xs = screen_text_width(0.42f, state_label(all[i]));
    xs = 18 + xs * WIDTH_MARGIN + 6;
    xn = xs + screen_text_width(0.42f, "GB VC") * WIDTH_MARGIN + 8; /* widest system word */
    if (!shown_count) {
        if (!loaded) snprintf(empty, sizeof empty, "%s", T("Lecture en cours…", "Reading…"));
        else if (!visible_count) snprintf(empty, sizeof empty, "%s", server_read ? T("Aucun jeu à afficher.", "No game to show.")
                                                                      : T("Serveur non lu, et aucune sauvegarde sur la 3DS.", "Server not read, and no save on the 3DS."));
        else if (query[0]) snprintf(empty, sizeof empty, T("Aucun jeu ne correspond à « %s ».%s", "No game matches \"%s\".%s"), query,
                                    tab_counts[TAB_ALL] ? T(" Changez d'onglet avec \xEE\x80\x84 et \xEE\x80\x85.", " Switch tabs with \xEE\x80\x84 and \xEE\x80\x85.") : "");
        else snprintf(empty, sizeof empty, "%s", tab == TAB_TODO ? T("Rien à faire pour le moment.", "Nothing to do right now.")
                                                                 : T("Aucun jeu dans cet onglet.", "No game in this tab."));
        screen_paragraph(10, top + 13, 0.5f, GREY_LIGHT, TOP_WIDTH - 20, empty);
    }
    for (i = start; i < shown_count && i < start + lines; i++) {
        game_t *k = &games[shown[i]];
        float y = top + (i - start) * 15;
        if (i == cursor) {
            screen_rect(0, y - 1, TOP_WIDTH, 15, SEL);
            screen_rect(0, y - 1, 2, 15, dot_color(k->status));
        }
        screen_rect(8, y + 5, 5, 5, dot_color(k->status));
        screen_text(18, y + 1, 0.42f, state_color(k->status), 0, state_label(k->status));
        screen_text(xs, y + 1, 0.42f, GREY_LIGHT, 0, vc(k) ? "GB VC" : SYSTEM_NAMES[k->system]);
        screen_text(xn, y, 0.5f, WHITE, TOP_WIDTH - xn - 8, k->short_name);
    }
    if (shown_count > lines) { /* scroll bar */
        float h = 15.0f * lines * lines / shown_count;
        screen_rect(TOP_WIDTH - 4, top - 1 + (15.0f * lines - h) * start / (shown_count - lines), 2, h, GREY_DARK);
    }

    screen_rect(0, FOOTER_Y, TOP_WIDTH, SCREEN_HEIGHT - FOOTER_Y, BAR);
    if (!loaded) snprintf(footer, sizeof footer, T("Serveur %.*s", "Server %.*s"), (int)strcspn(host, "/"), host);
    else if (server_read) {
        format_age((long)((osGetTime() - read_time) / 1000), english, 0, age, sizeof age);
        snprintf(footer, sizeof footer, T("Serveur %.*s \xC2\xB7 lu %s", "Server %.*s \xC2\xB7 read %s"), (int)strcspn(host, "/"), host, age);
    } else {
        snprintf(footer, sizeof footer, T("Serveur %.*s \xC2\xB7 non lu \xC2\xB7 \xEE\x80\x83 Réessayer", "Server %.*s \xC2\xB7 not read \xC2\xB7 \xEE\x80\x83 Try again"),
                 (int)strcspn(host, "/"), host);
    }
    pos[0] = '\0';
    if (shown_count) snprintf(pos, sizeof pos, "%d / %d", cursor + 1, shown_count);
    x = TOP_WIDTH - 6 - screen_text_width(0.38f, pos);
    screen_text(x, FOOTER_Y + 2, 0.38f, GREY_LIGHT, 0, pos);
    screen_text(8, FOOTER_Y + 2, 0.38f, loaded && !server_read ? C_RED : GREY_LIGHT, x - 8 - 12, footer);
}

/* Under the State word: why, and what A does. */
static const char *state_sentence(const game_t *j, char *t, size_t n)
{
    switch (j->status) {
    case STATUS_TO_UPLOAD: return T("La sauvegarde de la 3DS est plus récente. \xEE\x80\x80 l'envoie au serveur.", "The 3DS save is newer. \xEE\x80\x80 uploads it to the server.");
    case STATUS_TO_DOWNLOAD:
        if (vc(j) && !j->has_local) return T("Ce jeu n'a pas encore de sauvegarde. \xEE\x80\x80 la crée avec la version du serveur .",
                                             "This game has no save yet. \xEE\x80\x80 creates it from the server version.");
        return T("Le serveur a une version plus récente. \xEE\x80\x80 la récupère, après une copie de sécurité.",
                               "The server has a newer version. \xEE\x80\x80 downloads it, after a backup.");
    case STATUS_CONFLICT: return T("La sauvegarde a changé sur la 3DS et sur le serveur. \xEE\x80\x80 pour choisir laquelle garder.",
                           "The save changed on the 3DS and on the server. \xEE\x80\x80 to choose which one to keep.");
    case STATUS_DUPLICATE:
        if (j->shares_save) snprintf(t, n, T("%d ROM ont ce nom de fichier et partageraient la même sauvegarde. Renommez-en une.",
                                         "%d ROMs have this file name and would share one save. Rename one."), j->rom_count);
        else snprintf(t, n, T("%d ROM contiennent ce jeu. Gardez-en une, ou ajoutez les autres aux ROM hacks sur le PC, puis préparez la carte SD.",
                              "%d ROMs hold this game. Keep one, or add the others to ROM hacks on the PC, then prepare the SD card."), j->rom_count);
        return t;
    case STATUS_NOT_SYNCED:
        if (whole_archive(j)) return T("La sauvegarde de ce jeu ne peut pas être copiée en entier (détail ci-dessous). Ce jeu n'est pas synchronisé, rien n'y est modifié.",
                                       "This game's save can't be copied whole (details below). This game is not synced, and nothing in it is changed.");
        if (!j->key[0]) return T("La ROM de ce jeu Virtual Console est illisible : sa clé est inconnue. Ce jeu n'est pas synchronisé, rien n'y est modifié.",
                                 "The ROM of this Virtual Console game can't be read: its Key is unknown. This game is not synced, and nothing in it is changed.");
        return T("Ce jeu a des points de restauration : ils pourraient ramener une ancienne partie. Il n'est pas synchronisé, rien n'y est modifié.",
                 "This game has restore points that could bring an old game back. It is not synced, and nothing in it is changed.");
    case STATUS_OFFLINE: return T("Serveur non lu : état inconnu. \xEE\x80\x83 relit le serveur.", "Server not read: state unknown. \xEE\x80\x83 reads the server again.");
    default: return T("La 3DS et le serveur ont la même sauvegarde.", "The 3DS and the server have the same save.");
    }
}

static const char *system_label(const game_t *j)
{
    const char *ext = strrchr(j->rom, '.');
    switch (j->system) {
    case SYS_DS: return "Nintendo DS";
    case SYS_GB: return ext && !strcasecmp(ext, ".gbc") ? "Game Boy Color" : "Game Boy";
    case SYS_GBA: return "Game Boy Advance";
    default: return "Nintendo 3DS";
    }
}

/* GB/GBA: the program that plays the ROM with its Save next to it (the other one is the mGBA forwarder). */
static const char *rom_program(const game_t *j) { return j->system == SYS_GBA ? "open_agb_firm" : "TWiLight Menu++"; }

/* "Last sync" line: which side the Base matches. */
static const char *base_label(const char *b, const game_t *j)
{
    int local = b && !strcmp(b, j->local_hash), server = b && !strcmp(b, j->server_hash);
    if (!b) return T("Jamais", "Never");
    if (local && server) return T("Même version que les deux", "Same as both");
    if (local) return T("Même version que la 3DS", "Same as the 3DS");
    if (server) return T("Même version que le serveur", "Same as the server");
    return T("Version différente des deux", "Differs from both");
}

/* What A does for the selected game. */
static const char *action_label(const game_t *j)
{
    if (!j) return T("Aucun jeu", "No game");
    switch (j->status) {
    case STATUS_TO_UPLOAD: return T("Envoyer au serveur", "Upload to the server");
    case STATUS_TO_DOWNLOAD: return T("Récupérer du serveur", "Download from the server");
    case STATUS_CONFLICT: return T("Résoudre le conflit", "Resolve the conflict");
    default: return T("Rien à faire", "Nothing to do");
    }
}

/* Message of the last action, in its band. */
static void draw_message(float bottom) { draw_band(bottom, message, message_detail, message_color); }

/* Bottom screen of the list: the selected game, the message, and the two-line bar. */
static void draw_bottom(const game_t *j)
{
    char t[600];
    float y = 6;
    int action = 0, gb = 0;

    if (j) {
        const char *b = base_of(&bases, base_id(j));
        u32 c = j->status == STATUS_IN_SYNC ? C_GREEN : state_color(j->status);
        gb = (j->system == SYS_GB || j->system == SYS_GBA) && !vc(j);
        action = j->status == STATUS_TO_UPLOAD || j->status == STATUS_TO_DOWNLOAD || j->status == STATUS_CONFLICT;
        draw_game_icon(j);
        y += screen_lines(8, y, 0.6f, WHITE, ICON_X - 16, 2, j->short_name);
        /* System, and for GB/GBA the program whose save is synced (X changes it); title ID of a Virtual Console game. */
        if (gb) snprintf(t, sizeof t, "%s \xC2\xB7 %s", system_label(j), forwarder(j) ? T("forwarder mGBA", "mGBA forwarder") : rom_program(j));
        else if (vc(j)) snprintf(t, sizeof t, "GB \xC2\xB7 Virtual Console \xC2\xB7 %016llX", j->title);
        else snprintf(t, sizeof t, "%s", system_label(j));
        screen_text(8, y + 1, 0.4f, GREY_LIGHT, ICON_X - 16, t);
        y = y + 18 < UNDER_ICON ? UNDER_ICON : y + 20;
        screen_rect(8, y + 2, 3, 13, c);
        screen_text(15, y, 0.5f, c, 0, state_label(j->status));
        if (j->key[0]) { /* the server Versions of the game (history_screen): a word in a box for the stylus, like Search */
            const char *h = T("Historique", "History");
            float w = screen_text_width(0.42f, h) * WIDTH_MARGIN + 8, x = BOTTOM_WIDTH - 8 - w;
            screen_rect(x, y - 1, w, 18, CARD);
            screen_text(x + 4, y + 1, 0.42f, WHITE, 0, h);
            touch_area(x - 4, y - 4, w + 8, 24, KEY_HISTORY);
        }
        y += 19;
        y += screen_lines(8, y, 0.42f, WHITE, TEXT_WIDTH, j->status == STATUS_DUPLICATE ? 4 : vc(j) || j->unexpected ? 3 : 2, state_sentence(j, t, sizeof t)) + 5;
        if (j->status != STATUS_DUPLICATE && j->status != STATUS_NOT_SYNCED) { /* no save and no version to show */
            local_text(j, t, sizeof t);
            y = detail(y, T("Sur la 3DS", "On the 3DS"), t);
            server_text(j, t, sizeof t);
            y = detail(y, T("Sur le serveur", "On the server"), t);
            if (j->status != STATUS_IN_SYNC && j->has_server_progress && !strcmp(j->progress_hash, j->server_hash)) {
                progress_text(&j->server_progress, t, sizeof t); /* Pokémon: the play time of both, before any choice */
                y = detail(y, "", t);
            }
            y = detail(y, T("Dernière synchro", "Last sync"), base_label(b, j));
        }
        /* the Key, to compare with the one the PC shows under the game name; ROM not read: the failed step */
        if (vc(j) && j->key[0]) y = detail(y, T("Clé", "Key"), j->key);
        else if (vc(j) || (whole_archive(j) && j->unexpected)) screen_lines(8, y, 0.38f, GREY_LIGHT, TEXT_WIDTH, 3, j->why);
    }
    if (message[0]) draw_message(BAR_TOP_2);
    else if (cfg.check_update && update_is_newer(VERSION, update_info.latest)) { /* no message: the update, at the same place */
        snprintf(t, sizeof t, T("Version %s disponible (SELECT)", "Version %s available (SELECT)"), update_info.latest);
        draw_band(BAR_TOP_2, t, NULL, C_CYAN);
    }

    /* Bar: on top, what A (always shown, greyed out when there is nothing to do) and X (GB/GBA) do for this game, and
     * Search on the right; at the bottom, the general buttons. */
    draw_bar_background(2);
    {
        char x[80];
        button_t game_buttons[3] = { { KEY_A, action_label(j), action }, { KEY_X, x, 1 }, { KEY_TOUCH, T("Chercher", "Search"), 1 } };
        const button_t general[4] = { { KEY_Y, T("Rafraîchir", "Refresh"), 1 }, { KEY_START, T("Tout synchro", "Sync all"), 1 },
                                        { KEY_SELECT, T("Réglages", "Settings"), 1 }, { KEY_B, query[0] ? T("Effacer", "Clear") : T("Quitter", "Quit"), 1 } };
        if (gb) snprintf(x, sizeof x, T("Jouer avec %s", "Play with %s"), forwarder(j) ? rom_program(j) : T("le forwarder mGBA", "the mGBA forwarder"));
        else game_buttons[1] = game_buttons[2];
        draw_button_row(game_buttons, gb ? 3 : 2, BAR_ROW_2, 0);
        draw_button_row(general, 4, BAR_ROW_2 + BUTTON_HEIGHT, 0);
    }
}

static void draw(void)
{
    modal_begin(draw_list_top); /* not modal, but the same frame: two-line bar instead of modal_end */
    draw_bottom(shown_count ? &games[shown[list_cursor]] : NULL);
    screen_end();
}

/* Cursor after the arrows: UP and DOWN one line, LEFT and RIGHT one page, within the count lines. */
static int move_cursor(u32 k, int cursor, int count, int lines)
{
    if (k & KEY_DOWN) cursor++;
    if (k & KEY_UP) cursor--;
    if (k & KEY_RIGHT) cursor += lines;
    if (k & KEY_LEFT) cursor -= lines;
    if (cursor >= count) cursor = count - 1;
    return cursor < 0 ? 0 : cursor;
}

static void edit_text(const char *title, char *value, size_t n, int masked)
{
    SwkbdState s;
    char input[256];
    swkbdInit(&s, SWKBD_TYPE_WESTERN, 2, (int)n - 1);
    swkbdSetHintText(&s, title);
    swkbdSetInitialText(&s, value);
    swkbdSetValidation(&s, SWKBD_ANYTHING, 0, 0); /* empty allowed: "next to the ROM" */
    if (masked) swkbdSetPasswordMode(&s, SWKBD_PASSWORD_HIDE_DELAY);
    if (swkbdInputText(&s, input, sizeof input) == SWKBD_BUTTON_CONFIRM) snprintf(value, n, "%s", input);
}

/* Search typed on the system keyboard, trailing spaces dropped; empty: no search. */
static void edit_search(char *q, size_t n)
{
    size_t len;
    edit_text(T("Nom du jeu (vide : tous les jeux)", "Game name (empty: all games)"), q, n, 0);
    for (len = strlen(q); len && q[len - 1] == ' ';) q[--len] = '\0';
}

static int test_state;        /* Connection line of the settings: 0 not tested, 1 OK, -1 failure (test_short) */
static char test_short[80];  /* "Token refused": network_text up to the first ":", "." or "(" */

/* GET /saves: result in the message (text, then the technical line) and the Connection line. 1 if the connection works. */
static int test_connection(void)
{
    char url[300], d[200], *body;
    const char *t;
    size_t size;
    long status;
    show_progress(0, 0, "%s", T("Test de la connexion…", "Testing the connection…"));
    snprintf(url, sizeof url, "%s/saves", cfg.url);
    status = net_get(url, cfg.token, &body, &size);
    if (status > 0) free(body);
    if (status == 200) {
        set_message(C_GREEN, NULL, "%s", T("Connexion réussie.", "Connected."));
        test_state = 1;
        return 1;
    }
    t = network_text(status, net_wifi(), english);
    network_detail(status, url, d, sizeof d);
    set_message(C_RED, d, "%s", t);
    snprintf(test_short, sizeof test_short, "%.*s", (int)strcspn(t, ":.("), t);
    if (strlen(test_short) && test_short[strlen(test_short) - 1] == ' ') test_short[strlen(test_short) - 1] = '\0';
    test_state = -1;
    return 0;
}

/* On/off of the 3DS games (ADR 0028): every installed application and game card game with a Save archive, ticked when
 * it is synced (not in titres-off.ini). A new game is ticked. A turned-off game absent from the console stays off. */
typedef struct {
    u64 id;
    FS_MediaType media;
    char name[48];
    int ticked;
} title_choice_t;

static int compare_choices(const void *a, const void *b)
{
    return compare_names(((const title_choice_t *)a)->name, ((const title_choice_t *)b)->name);
}

static title_choice_t *choices; /* all the games of the choice (heap) */
static int *choice_shown, choice_count, choice_shown_count, choice_cursor; /* choice_shown: those that match the search */
static char choice_query[48];

/* choice_shown from the search; the cursor stays on the same game if it is still shown, else the first. */
static int filter_choices(int cursor)
{
    int selected = choice_shown_count ? choice_shown[cursor] : -1, i;
    choice_shown_count = 0;
    for (i = 0; i < choice_count; i++)
        if (name_matches(choices[i].name, choice_query)) choice_shown[choice_shown_count++] = i;
    cursor = index_of(choice_shown, choice_shown_count, selected);
    return cursor < 0 ? 0 : cursor;
}

/* Top screen of the choice: a check box per game, ticked when synced. */
static void draw_choices_top(void)
{
    const int cursor = choice_cursor, lines = list_lines(choice_query), start = cursor < lines ? 0 : cursor - lines + 1;
    int i;
    float top;
    screen_top();
    screen_text(6, 4, 0.55f, WHITE, 0, T("Jeux 3DS synchronisés", "Synced 3DS games"));
    screen_rect(0, 23, TOP_WIDTH, 1, GREY_DARK);
    top = draw_search_band(choice_query);
    if (!choice_shown_count && choice_query[0]) {
        char t[120];
        snprintf(t, sizeof t, T("Aucun jeu ne correspond à « %s ».", "No game matches \"%s\"."), choice_query);
        screen_paragraph(10, top + 13, 0.5f, GREY_LIGHT, TOP_WIDTH - 20, t);
    }
    for (i = start; i < choice_shown_count && i < start + lines; i++) {
        const title_choice_t *c = &choices[choice_shown[i]];
        float ly = top + (i - start) * 15;
        if (i == cursor) {
            screen_rect(0, ly - 1, TOP_WIDTH, 15, SEL);
            screen_rect(0, ly - 1, 2, 15, ACCENT);
        }
        screen_rect(9, ly + 2, 10, 10, GREY_LIGHT); /* box: border, background, tick */
        screen_rect(10, ly + 3, 8, 8, i == cursor ? SEL : BOX);
        if (c->ticked) screen_rect(11, ly + 4, 6, 6, C_GREEN);
        screen_text(26, ly, 0.5f, c->ticked ? WHITE : GREY_LIGHT, TOP_WIDTH - 40, c->name);
    }
    if (start > 0) screen_text(TOP_WIDTH - 12, top - 1, 0.4f, GREY_LIGHT, 0, "\xE2\x96\xB2");
    if (start + lines < choice_shown_count) screen_text(TOP_WIDTH - 12, FOOTER_Y, 0.4f, GREY_LIGHT, 0, "\xE2\x96\xBC");
}

/* The games of the choice in choices: the syncable applications with a Save archive (a game never started and the
 * forwarders have none), each once. 0 if listed; otherwise the message says why. */
static int list_choices(void)
{
    u64 *ids = NULL;
    FS_MediaType *medias = NULL;
    char root[16];
    int n = -1, i, k, m = 0;
    show_progress(0, 0, "%s", T("Lecture des jeux installés…", "Reading installed games…"));
    if (R_SUCCEEDED(amInit())) {
        n = installed_titles(&ids, &medias, 0);
        amExit();
    }
    choices = n < 0 ? NULL : malloc((n + 1) * sizeof *choices);
    choice_shown = choices ? malloc((n + 1) * sizeof *choice_shown) : NULL;
    if (!choice_shown) {
        if (n < 0) set_message(C_RED, NULL, "%s", T("Impossible de lire la liste des jeux. Redémarrez la 3DS et réessayez.", "Can't read the installed games. Restart the 3DS and try again."));
        else set_message(C_RED, T("malloc impossible", "malloc failed"), "%s", T("Mémoire insuffisante pour lister les jeux. Redémarrez la 3DS et réessayez.", "Not enough memory to list the games. Restart the 3DS and try again."));
        free(ids);
        free(medias);
        free(choices);
        choices = NULL;
        return -1;
    }
    for (i = 0; i < n; i++) {
        show_progress(i + 1, n, "%s", T("Lecture des jeux", "Reading games"));
        for (k = 0; k < m && choices[k].id != ids[i]; k++);
        if (k < m || !syncable_title(ids[i], medias[i]) || title_root(ids[i], root, sizeof root) < 0) continue;
        choices[m].id = ids[i];
        choices[m].media = medias[i];
        choices[m].ticked = !is_off(ids[i]);
        if (title_name(ids[i], medias[i], language, choices[m].name, sizeof choices[m].name)) snprintf(choices[m].name, sizeof choices[m].name, "%016llX", ids[i]);
        m++;
    }
    free(ids);
    free(medias);
    qsort(choices, m, sizeof *choices, compare_choices);
    choice_count = m;
    return 0;
}

/* off from the choice: the unticked games, and the turned-off games the choice does not show (absent from the console). */
static void apply_choices(void)
{
    static unsigned long long kept[MAX_OFF];
    int n = 0, i, k;
    for (i = 0; i < off_count; i++) {
        for (k = 0; k < choice_count && choices[k].id != off[i]; k++);
        if (k == choice_count) kept[n++] = off[i];
    }
    for (i = 0; i < choice_count && n < MAX_OFF; i++)
        if (!choices[i].ticked) kept[n++] = choices[i].id;
    memcpy(off, kept, n * sizeof *off);
    off_count = n;
}

static void choose_titles(void)
{
    int changed = 0;
    if (list_choices()) return;
    clear_message();
    choice_query[0] = '\0';
    choice_shown_count = 0;
    choice_cursor = filter_choices(0);

    while (aptMainLoop()) {
        const int cursor = choice_cursor;
        title_choice_t *ch = choice_shown_count ? &choices[choice_shown[cursor]] : NULL;
        u16 icon[ICON_SIZE * ICON_SIZE];
        float y = 6;
        u32 k;
        button_t t[3] = { { KEY_A, ch && ch->ticked ? T("Désactiver", "Turn off") : T("Activer", "Turn on"), ch != NULL },
                          { KEY_TOUCH, T("Chercher", "Search"), 1 },
                          { KEY_B, choice_query[0] ? T("Effacer la recherche", "Clear the search") : T("Enregistrer et revenir", "Save and go back"), 1 } };

        modal_begin(draw_choices_top);
        if (ch) {
            if (title_icon(ch->id, ch->media, icon) == 0) screen_icon(icon, ICON_SIZE, ICON_SIZE, ICON_X, 6, 1.0f);
            y += screen_lines(8, y, 0.55f, WHITE, ICON_X - 16, 2, ch->name) + 4;
            y = y < UNDER_ICON ? UNDER_ICON + 4 : y + 4;
            if (ch->ticked)
                y += screen_paragraph(8, y, 0.45f, C_GREEN, TEXT_WIDTH, T("Synchronisé : sa sauvegarde est envoyée et récupérée comme celle des autres jeux.",
                                                                         "Synced: its save is uploaded and downloaded like the other games' saves.")) + 6;
            else
                y += screen_paragraph(8, y, 0.45f, GREY_LIGHT, TEXT_WIDTH, T("Désactivé : 3dssavesync ne lit pas sa sauvegarde et ne la modifie pas.",
                                                                            "Turned off: 3dssavesync doesn't read or change its save.")) + 6;
        } else {
            y += screen_paragraph(8, y, 0.5f, GREY_LIGHT, TEXT_WIDTH, choice_query[0] ? T("Aucun jeu ne correspond à la recherche.", "No game matches the search.")
                                                                                     : T("Aucun jeu trouvé.", "No game found.")) + 6;
        }
        screen_paragraph(8, y, 0.42f, GREY_LIGHT, TEXT_WIDTH, T("Un jeu manque ? Lancez-le une fois et sauvegardez.", "Game missing? Start it once and save."));
        draw_message(BAR_TOP_1);
        k = modal_end(t, 3, 18);

        if (k & (KEY_UP | KEY_DOWN | KEY_LEFT | KEY_RIGHT)) clear_message();
        if (k & KEY_B) {
            if (!choice_query[0]) break;
            choice_query[0] = '\0'; /* B clears the search first */
            choice_cursor = filter_choices(cursor);
            continue;
        }
        choice_cursor = move_cursor(k, cursor, choice_shown_count, list_lines(choice_query));
        if (k & KEY_TOUCH) {
            edit_search(choice_query, sizeof choice_query);
            choice_cursor = filter_choices(cursor);
        }
        if (k & KEY_A && ch) {
            clear_message();
            ch->ticked = !ch->ticked;
            changed = 1;
        }
    }
    clear_message();
    if (changed) {
        apply_choices();
        if (save_off()) set_message(C_RED, APP_DIR "/titres-off.ini", "%s", T("Liste des jeux 3DS non enregistrée. Vérifiez la carte SD.", "3DS game list not saved. Check the SD card."));
        else set_message(C_GREEN, NULL, "%s", T("Liste des jeux 3DS enregistrée.", "3DS game list saved."));
    }
    free(choices);
    free(choice_shown);
    choices = NULL;
    choice_shown = NULL;
    choice_count = choice_shown_count = 0;
}

/* Editable setting lines, then 3DS games, List order, Connection and Version (settings screen). */
static const struct {
    const char *name, *name_en, *help, *help_en;
    char *value;
    size_t n;
} SETTINGS[] = {
    { "Serveur", "Server", "Adresse du serveur, par exemple https://saves.exemple.fr, ou le serveur local du PC : http://192.168.1.20:8765",
      "Server address, for example https://saves.example.com, or the PC local server: http://192.168.1.20:8765", cfg.url, sizeof cfg.url },
    { "Jeton", "Token", "Le jeton du serveur, le même que sur le PC.", "The server token, the same as on the PC.", cfg.token, sizeof cfg.token },
    { "ROM DS", "DS ROMs", "Dossier des ROM DS. TWiLight Menu++ : sdmc:/roms/nds", "DS ROM folder. TWiLight Menu++: sdmc:/roms/nds", cfg.roms, sizeof cfg.roms },
    { "Sauvegardes DS", "DS saves", "TWiLight Menu++ : sdmc:/roms/nds/saves. Vide = à côté de la ROM.", "TWiLight Menu++: sdmc:/roms/nds/saves. Empty = next to the ROM.",
      cfg.saves, sizeof cfg.saves },
    { "ROM GB", "GB ROMs", "Dossier des ROM GB et GBC. TWiLight Menu++ : sdmc:/roms/gb", "GB and GBC ROM folder. TWiLight Menu++: sdmc:/roms/gb", cfg.roms_gb,
      sizeof cfg.roms_gb },
    { "Sauvegardes GB", "GB saves", "Vide = à côté de la ROM (TWiLight Menu++). La sauvegarde d'un forwarder mGBA prime si elle existe.",
      "Empty = next to the ROM (TWiLight Menu++). An mGBA forwarder save wins if it exists.", cfg.saves_gb, sizeof cfg.saves_gb },
    { "ROM GBA", "GBA ROMs", "Dossier des ROM GBA. Exemple : sdmc:/roms/gba", "GBA ROM folder. Example: sdmc:/roms/gba", cfg.roms_gba, sizeof cfg.roms_gba },
    { "Sauvegardes GBA", "GBA saves", "open_agb_firm (useSavesFolder=true) : sdmc:/3ds/open_agb_firm/saves. Vide = à côté de la ROM.",
      "open_agb_firm (useSavesFolder=true): sdmc:/3ds/open_agb_firm/saves. Empty = next to the ROM.", cfg.saves_gba, sizeof cfg.saves_gba },
};
#define SETTING_COUNT ((int)(sizeof SETTINGS / sizeof *SETTINGS))
enum { LINE_TITLES = SETTING_COUNT, LINE_ORDER, LINE_TEST, LINE_PHONE, LINE_VERSION, LINE_COUNT }; /* lines after the editable ones */
static int selected_setting; /* selected settings line: also for the top screen during a progress */

/* Server (0) or Token (1) not set yet. */
static int setting_missing(int i)
{
    return !SETTINGS[i].value[0] || !strcmp(SETTINGS[i].value, "https://");
}

/* Phone line: what is missing for the QR code, NULL if the server and the token are set. */
static const char *phone_missing(void)
{
    if (setting_missing(0) && setting_missing(1)) return T("Serveur et jeton à remplir", "Server and token required");
    if (setting_missing(0)) return T("Serveur à remplir", "Server required");
    if (setting_missing(1)) return T("Jeton à remplir", "Token required");
    return NULL;
}

/* Pairing QR code of the Android app; filled only while it is shown (it holds the token). */
static uint8_t qr[qrcodegen_BUFFER_LEN_MAX];

/* The QR code on the top screen, the larger one: whole modules, centered, black on white with a margin of 4 modules.
 * One rectangle per run of black modules in a row (citro2d draws 4096 objects per frame). */
static void draw_qr_top(void)
{
    int size = qrcodegen_getSize(qr), scale = SCREEN_HEIGHT / (size + 8), x, y;
    int left = (TOP_WIDTH - size * scale) / 2, top = (SCREEN_HEIGHT - size * scale) / 2;
    screen_top();
    screen_rect(left - 4 * scale, top - 4 * scale, (size + 8) * scale, (size + 8) * scale, RGB(0xFFFFFF));
    for (y = 0; y < size; y++)
        for (x = 0; x < size; x++) {
            int end = x;
            while (end < size && qrcodegen_getModule(qr, end, y)) end++;
            if (end > x) screen_rect(left + x * scale, top + y * scale, (end - x) * scale, scale, RGB(0x000000));
            x = end; /* a white module or the end of the row */
        }
}

/* A on the Phone line: the QR code until B. Error correction M, like the PC (client/setup_web.go). */
static void show_pairing_qr(void)
{
    static uint8_t temp[qrcodegen_BUFFER_LEN_MAX];
    static char text[1600]; /* 2 × 255 bytes, 3 each once encoded, and the fixed part */
    const button_t b[1] = { { KEY_B, T("Fermer", "Close"), 1 } };
    int ok = pairing_payload(cfg.url, cfg.token, text, sizeof text) &&
             qrcodegen_encodeText(text, temp, qr, qrcodegen_Ecc_MEDIUM, qrcodegen_VERSION_MIN, qrcodegen_VERSION_MAX, qrcodegen_Mask_AUTO, false);
    memset(text, 0, sizeof text);
    memset(temp, 0, sizeof temp);
    while (ok && aptMainLoop()) {
        float y = 6;
        modal_begin(draw_qr_top);
        y += screen_lines(8, y, 0.55f, WHITE, TEXT_WIDTH, 1, T("Téléphone", "Phone")) + 6;
        screen_paragraph(8, y, 0.45f, WHITE, TEXT_WIDTH, T("Dans l'app Android : « Scanner le QR code du PC », puis visez ce code.",
                                                           "In the Android app: \"Scan the PC's QR code\", then point the phone at this code."));
        if (modal_end(b, 1, 0) & KEY_B) break;
    }
    memset(qr, 0, sizeof qr);
}

/* State of the installed version, as on the Version line: "Up to date", "3.7.1 available"… in t (t may be NULL with
 * n = 0: color only). Returns its color. */
static u32 version_state(char *t, size_t n)
{
    if (!strcmp(VERSION, "dev")) {
        snprintf(t, n, "%s", T("Version de développement", "Development build"));
        return GREY_LIGHT;
    }
    if (update_is_newer(VERSION, update_info.latest)) {
        snprintf(t, n, T("%s disponible", "%s available"), update_info.latest);
        return C_CYAN;
    }
    if (update_failed) {
        snprintf(t, n, "%s", T("Vérification impossible", "Check failed"));
        return C_RED;
    }
    if (!update_info.latest[0]) {
        snprintf(t, n, "%s", T("Non vérifiée", "Not checked"));
        return GREY_LIGHT;
    }
    snprintf(t, n, "%s", T("À jour", "Up to date"));
    return C_GREEN;
}

/* "3.7.0 · Up to date" from x: version in white, dot in grey, State in color, then " · auto check off" in grey when
 * verifier_maj=0, cut before end. */
static void draw_version_value(float x, float y, float size, float end)
{
    char e[48];
    u32 c = version_state(e, sizeof e);
    x += screen_text(x, y, size, WHITE, end - x, VERSION);
    x += screen_text(x, y, size, GREY_DARK, 0, " \xC2\xB7 ");
    x += screen_text(x, y, size, c, end - x, e);
    if (!cfg.check_update && end - x > 0) screen_text(x, y, size, GREY_LIGHT, end - x, T(" \xC2\xB7 vérif. auto désactivée", " \xC2\xB7 auto check off"));
}

static const char *setting_label(int i)
{
    if (i < SETTING_COUNT) return T(SETTINGS[i].name, SETTINGS[i].name_en);
    if (i == LINE_TITLES) return T("Jeux 3DS", "3DS games");
    if (i == LINE_ORDER) return T("Tri de la liste", "List order");
    if (i == LINE_TEST) return T("Connexion", "Connection");
    if (i == LINE_PHONE) return T("Téléphone", "Phone");
    return T("Version", "Version");
}

static const char *order_label(int o)
{
    if (o == ORDER_NAME) return T("Nom", "Name");
    if (o == ORDER_RECENT) return T("Dernière partie", "Last played");
    return T("À faire d'abord", "To do first");
}

/* What A does on line i, drawn on the right of the selected line. */
static const char *setting_action(int i)
{
    if (i < SETTING_COUNT) return T("Modifier", "Change");
    if (i == LINE_TITLES) return T("Choisir", "Choose");
    if (i == LINE_ORDER) return T("Changer", "Change");
    if (i == LINE_TEST) return T("Tester", "Test");
    if (i == LINE_PHONE) return T("Afficher le QR code", "Show the QR code");
    return update_is_newer(VERSION, update_info.latest) ? T("Mettre à jour", "Update") : T("Rechercher", "Check");
}

/* Top screen of the settings: label on the left, value in a column, "Ⓐ action" on the right of the selected line. */
static void draw_settings_top(void)
{
    float y = 29;
    int i;
    screen_top();
    screen_text(6, 4, 0.55f, WHITE, 0, T("Réglages", "Settings"));
    screen_rect(0, 23, TOP_WIDTH, 1, GREY_DARK);
    for (i = 0; i < LINE_COUNT; i++) {
        char v[24];
        const char *value = v;
        u32 c = WHITE;
        int required = 0;
        float end = TOP_WIDTH - 8;
        if (i < SETTING_COUNT) {
            value = SETTINGS[i].value;
            if (i <= 1 && setting_missing(i)) {
                value = T("À remplir", "Required");
                c = GREY_LIGHT;
                required = 1;
            } else if (i == 1) {
                value = "\xE2\x80\xA2\xE2\x80\xA2\xE2\x80\xA2\xE2\x80\xA2\xE2\x80\xA2\xE2\x80\xA2\xE2\x80\xA2\xE2\x80\xA2"; /* •••••••• */
            } else if (!value[0]) {
                value = T("À côté de la ROM", "Next to the ROM"); /* default value */
                c = GREY_LIGHT;
            }
        } else if (i == LINE_TITLES) {
            if (off_count) snprintf(v, sizeof v, plural(off_count, english, T("%d désactivé", "%d turned off"), T("%d désactivés", "%d turned off")), off_count);
            else snprintf(v, sizeof v, "%s", T("Tous", "All"));
        } else if (i == LINE_ORDER) {
            value = order_label(cfg.order);
        } else if (i == LINE_TEST) {
            value = test_state > 0 ? T("Réussie", "OK") : test_state < 0 ? test_short : T("Non testée", "Not tested");
            c = test_state > 0 ? C_GREEN : test_state < 0 ? C_RED : GREY_LIGHT;
        } else if (i == LINE_PHONE) {
            value = phone_missing() ? phone_missing() : T("Application Android", "Android app");
            c = GREY_LIGHT;
        }
        if (i == selected_setting) {
            const char *a = setting_action(i);
            float xa = TOP_WIDTH - 8 - screen_text_width(0.44f, a), g = screen_text_width(0.5f, glyph(KEY_A));
            screen_rect(0, y - 1, TOP_WIDTH, 16, SEL);
            screen_rect(0, y - 1, 2, 16, ACCENT);
            if (i != LINE_PHONE || !phone_missing()) { /* Phone without server or token: no action, room for what is missing */
                screen_text(xa - g - 3, y - 1, 0.5f, GLYPH_A, 0, glyph(KEY_A));
                screen_text(xa, y + 1, 0.44f, WHITE, 0, a);
                end = xa - g - 3 - 10;
            }
        }
        if (required) screen_rect(0, y - 1, 2, 16, C_RED);
        screen_text(8, y, 0.46f, i == selected_setting ? WHITE : GREY_LIGHT, 128, setting_label(i));
        if (i == LINE_VERSION) draw_version_value(142, y, 0.46f, end);
        else screen_text(142, y, 0.46f, c, end - 142, value);
        y += 15 + (i == 1 || i == SETTING_COUNT - 1 ? 7 : 0); /* groups: server, folders, the rest (13 lines: 15 pixels each) */
    }
}

/* Settings screen: one line per setting, A for the action of the line, explanation at the bottom. first: no setting
 * yet. 1 if a setting or the list of 3DS games changed (then everything must be read again). */
static int settings_screen(int first)
{
    const int count = SETTING_COUNT;
    int changed, no_read_changed;
    static unsigned long long before_off[MAX_OFF];
    int before_count = off_count;
    config_t before;

    memcpy(&before, &cfg, sizeof cfg);
    memcpy(before_off, off, sizeof off);
    selected_setting = 0;
    draw_current_top = draw_settings_top; /* progress and questions: the settings stay on top */
    clear_message();
    while (aptMainLoop()) {
        const int c = selected_setting;
        u32 k;
        float y = 6;
        /* First start: B tests the connection first; a second B after a failure continues anyway. */
        /* Version line: Y turns the daily check (verifier_maj) on or off, between A and B. */
        button_t t[3] = { { KEY_A, setting_action(c), c != LINE_PHONE || !phone_missing() },
                          { KEY_Y, cfg.check_update ? T("Désactiver la vérif.", "Turn off auto check") : T("Activer la vérif.", "Turn on auto check"), 1 },
                          { KEY_B, !first || test_state > 0 ? T("Enregistrer et revenir", "Save and go back")
                                   : test_state < 0           ? T("Continuer quand même", "Continue anyway")
                                                             : T("Tester et continuer", "Test and continue"), 1 } };

        if (c != LINE_VERSION) t[1] = t[2];

        modal_begin(draw_settings_top);
        if (first) {
            const char *b = T("Bienvenue ! Entrez d'abord le serveur et le jeton.", "Welcome! Enter the server and the token first.");
            float h = screen_paragraph_height(0.45f, BOTTOM_WIDTH - 24, b) + 8;
            screen_rect(0, 0, BOTTOM_WIDTH, h, CARD);
            screen_rect(0, 0, 3, h, ACCENT);
            screen_paragraph(10, 4, 0.45f, WHITE, BOTTOM_WIDTH - 24, b);
            y = h + 6;
        }
        if (c == LINE_VERSION) {
            y += screen_lines(8, y, 0.55f, WHITE, TEXT_WIDTH, 1, T("Mise à jour de 3dssavesync", "3dssavesync update")) + 8;
            screen_text(8, y, 0.42f, GREY_LIGHT, 100, T("Installée", "Installed"));
            draw_version_value(112, y, 0.42f, BOTTOM_WIDTH - 8);
            y += 15 + 6;
            y += screen_paragraph(8, y, 0.42f, GREY_LIGHT, TEXT_WIDTH,
                                  cfg.check_update ? T("Cherche une nouvelle version sur GitHub une fois par jour au démarrage, et vous demande avant de l'installer.",
                                                       "Looks for a new version on GitHub once a day at start, and asks before installing it.")
                                                   : T("Vérification automatique désactivée : 3dssavesync contacte GitHub seulement quand vous appuyez sur \xEE\x80\x80. "
                                                       "Il vous demande avant d'installer.",
                                                       "Automatic check turned off: 3dssavesync contacts GitHub only when you press \xEE\x80\x80. It asks before installing.")) + 6;
        } else {
            y += screen_lines(8, y, 0.55f, WHITE, TEXT_WIDTH, 1, setting_label(c)) + 6;
            y += screen_paragraph(8, y, 0.45f, WHITE, TEXT_WIDTH, c < count ? T(SETTINGS[c].help, SETTINGS[c].help_en)
                                                         : c == LINE_TITLES ? T("Tous les jeux 3DS qui ont une sauvegarde sont synchronisés. Désactivez ici ceux à laisser de côté.",
                                                                                "Every 3DS game with a save is synced. Turn off here the ones to leave out.")
                                                         : c == LINE_PHONE ? T("Le QR code à scanner avec l'application Android : il contient l'adresse et le jeton du serveur.",
                                                                               "The QR code to scan with the Android app: it holds the server address and token.")
                                                         : c == LINE_ORDER ? T("Ordre des jeux dans la liste. À faire d'abord : les conflits, puis ce qu'il faut récupérer "
                                                                               "ou envoyer. Dernière partie : la sauvegarde la plus récente de la 3DS en premier ; "
                                                                               "les jeux sans date (dont les jeux 3DS) vont à la fin.",
                                                                               "Order of the games in the list. To do first: conflicts, then what to download or "
                                                                               "upload. Last played: the newest save on the 3DS first; games without a date "
                                                                               "(3DS games included) go last.")
                                                                   : T("Vérifie l'adresse et le jeton du serveur.", "Checks the server address and token.")) + 6;
        }
        if (message[0]) draw_message(BAR_TOP_1);
        else if (c == LINE_VERSION && update_detail[0]) { /* last check: text, then its technical line (after the last line break) */
            char d[sizeof update_detail];
            char *nl;
            u32 color = version_state(NULL, 0);
            snprintf(d, sizeof d, "%s", update_detail);
            nl = strrchr(d, '\n');
            if (nl) *nl++ = '\0';
            draw_band(BAR_TOP_1, d, nl, color == GREY_LIGHT ? WHITE : color);
        }
        k = modal_end(t, c == LINE_VERSION ? 3 : 2, 18);
        if (k & KEY_Y && c == LINE_VERSION) cfg.check_update = !cfg.check_update; /* saved with the other settings (B) */
        if (k & KEY_B) {
            if (!first || test_state) break;
            clear_message();
            if (test_connection()) break;
            continue;
        }
        if (k & KEY_DOWN && c + 1 < LINE_COUNT) selected_setting++;
        if (k & KEY_UP && c > 0) selected_setting--;
        if (k & KEY_A) {
            clear_message();
            if (c < count) {
                size_t len;
                test_state = 0; /* the address or the token may have changed: the last test no longer counts */
                edit_text(T(SETTINGS[c].name, SETTINGS[c].name_en), SETTINGS[c].value, SETTINGS[c].n, c == 1);
                len = strlen(SETTINGS[c].value);
                while (len > 0 && SETTINGS[c].value[len - 1] == '/') SETTINGS[c].value[--len] = '\0'; /* "…/saves/" -> "…/saves" */
            } else if (c == LINE_TITLES) {
                choose_titles();
            } else if (c == LINE_ORDER) {
                cfg.order = (cfg.order + 1) % 3;
            } else if (c == LINE_TEST) {
                test_connection();
            } else if (c == LINE_PHONE) {
                if (!phone_missing()) show_pairing_qr();
            } else {
                should_quit = update_app();
                if (should_quit) break;
            }
        }
    }
    draw_current_top = draw_list_top;
    if (!first) clear_message(); /* first start: the result of the test stays shown */
    no_read_changed = before.order != cfg.order || before.check_update != cfg.check_update;
    before.order = cfg.order; /* the order only sorts the list again (sort_list): no new read */
    before.check_update = cfg.check_update; /* nor the daily check */
    changed = memcmp(&before, &cfg, sizeof cfg) || before_count != off_count || memcmp(before_off, off, sizeof off);
    if (no_read_changed || memcmp(&before, &cfg, sizeof cfg)) save_config();
    return changed;
}

/* An Upload or Download with sleep refused: closing the lid would cut the Wi-Fi in the middle of the transfer (libctru's
 * APT thread answers the sleep query even while the app is busy). HOME needs no lock: it only acts in aptMainLoop,
 * which no transfer calls. */
static result_t awake_transfer(game_t *j, int upload)
{
    char text[512], detail[200], local[65], server[65];
    result_t r;
    snprintf(local, sizeof local, "%s", j->local_hash);
    snprintf(server, sizeof server, "%s", j->server_hash);
    aptSetSleepAllowed(false);
    r = upload ? upload_save(j) : download_save(j);
    aptSetSleepAllowed(true);
    result_text(&r, j->short_name, 1, text, sizeof text, detail, sizeof detail);
    {
        char *at = strlen(cfg.url) >= 8 ? strstr(detail, cfg.url) : NULL; /* a network failure shows its URL: the address stays out of the file */
        if (at) memmove(at + 8, at + strlen(cfg.url), strlen(at + strlen(cfg.url)) + 1), memcpy(at, "<server>", 8);
    }
    log_event("%s %s \"%s\" (3DS %.12s, server %.12s): %s%s%s", upload ? "upload" : "download", base_id(j), j->name, local[0] ? local : "-",
              server[0] ? server : "-", text, detail[0] ? " | " : "", detail);
    return r;
}

/* An Upload or Download of one game: the screen says it at once (the network, the copy and the read back take seconds). */
static result_t transfer(game_t *j, int upload)
{
    show_progress(0, 0, "%s\n%s", upload ? T("Envoi…", "Uploading…") : T("Récupération…", "Downloading…"), j->short_name);
    return awake_transfer(j, upload);
}

/* A on a Game: its action, with a confirmation before any overwrite. 1 if an action was tried. */
static int act(game_t *j)
{
    char q[500], srv[200], loc[200], t[80];
    progress_t p;
    int done = 0;
    result_t r = make_result(RESULT_NONE, NULL);
    switch (j->status) {
    case STATUS_TO_UPLOAD:
        r = transfer(j, 1);
        done = 1;
        break;
    case STATUS_TO_DOWNLOAD: {
        int has_server_progress;
        has_server_progress = server_progress(j, &p);
        server_text(j, srv, sizeof srv);
        local_text(j, loc, sizeof loc);
        if (has_server_progress) progress_text(&p, t, sizeof t);
        snprintf(q, sizeof q, "%s\t%s%s%s\n%s\t%s", T("Sur le serveur", "On the server"), srv, has_server_progress ? "\n\t" : "", has_server_progress ? t : "", T("Sur la 3DS", "On the 3DS"), loc);
        if ((done = confirm(T("Récupérer la version du serveur ?", "Download the server version?"), j->short_name, q,
                              j->has_local ? T("La sauvegarde de la 3DS sera remplacée, après une copie de sécurité.", "The 3DS save will be replaced, after a backup.") : NULL,
                              T("Récupérer", "Download"))))
            r = transfer(j, 0);
        break;
    }
    case STATUS_CONFLICT: {
        u32 k;
        const button_t b = { KEY_B, T("Ne rien faire", "Do nothing"), 1 };
        /* Both cards have the same form: the version, its play time (Pokémon), what happens. */
        if (server_progress(j, &p)) {
            server_text(j, t, sizeof t);
            snprintf(srv, sizeof srv, "%s\n", t);
            progress_text(&p, srv + strlen(srv), sizeof srv - strlen(srv));
        } else server_text(j, srv, sizeof srv);
        if (j->has_progress && j->modified) { /* local_text then gives the play time: the date first, as on the right */
            char d[24];
            format_age((long)(time(NULL) - j->modified), english, 0, d, sizeof d);
            snprintf(loc, sizeof loc, T("Modifiée %s\n", "Modified %s\n"), d);
            progress_text(&j->progress, loc + strlen(loc), sizeof loc - strlen(loc));
        } else local_text(j, loc, sizeof loc);
        do {
            float y = 6, w;
            modal_begin(NULL);
            draw_game_icon(j);
            screen_rect(8, y + 2, 3, 14, C_RED);
            w = screen_text(15, y, 0.55f, C_RED, 0, T("Conflit", "Conflict"));
            w += screen_text(15 + w, y, 0.55f, GREY_LIGHT, 0, " \xC2\xB7 ");
            screen_text(15 + w, y, 0.55f, WHITE, ICON_X - 16 - 15 - w, j->short_name);
            y += 22;
            y += screen_lines(8, y, 0.45f, WHITE, ICON_X - 16, 2, T("La sauvegarde a changé sur la 3DS et sur le serveur. Quelle version garder ?",
                                                             "The save changed on the 3DS and on the server. Which one to keep?"));
            y = y < UNDER_ICON ? UNDER_ICON + 2 : y + 6;
            draw_choice_card(6, y, KEY_A, T("Version 3DS", "3DS save"), loc, T("Puis envoyée au serveur.", "Then uploaded to the server."));
            draw_choice_card(162, y, KEY_X, T("Version serveur", "Server save"), srv, T("La 3DS est copiée avant.", "The 3DS is backed up first."));
            screen_paragraph(8, y + 98, 0.42f, GREY_LIGHT, TEXT_WIDTH, T("Rien n'est perdu : l'autre version est gardée.", "Nothing is lost: the other one is kept."));
            k = modal_end(&b, 1, 0);
        } while (!(k & (KEY_A | KEY_X | KEY_B)));
        done = (k & (KEY_A | KEY_X)) != 0;
        if (k & KEY_A) r = transfer(j, 1);
        else if (k & KEY_X) r = transfer(j, 0);
        else r = make_result(RESULT_CONFLICT_LEFT, NULL);
        break;
    }
    case STATUS_DUPLICATE:
        r = make_result(j->shares_save ? RESULT_DUPLICATE_SHARED : RESULT_DUPLICATE, NULL);
        r.count = j->rom_count;
        break;
    case STATUS_NOT_SYNCED:
        if (whole_archive(j)) {
            r = make_result(RESULT_3DS_NOT_SYNCED, j->why);
            break;
        }
        if (!j->key[0]) {
            r = make_result(RESULT_VC_NO_KEY, j->why);
            break;
        }
        r = make_result(RESULT_VC_UNEXPECTED, NULL);
        title_root(j->title, r.path, sizeof r.path); /* the technical line: what the archive holds */
        break;
    case STATUS_OFFLINE:
        r = make_result(RESULT_OFFLINE, NULL);
        break;
    default:
        r = make_result(RESULT_ALREADY_IN_SYNC, NULL);
    }
    if (r.code != RESULT_NONE) show_result(&r, j); /* cancelled: the band keeps its previous message */
    if (done) read_local(j);
    return done;
}

/* X on a GB/GBA Game: its Save moves from the mGBA forwarder to the one of the ROM (open_agb_firm, TWiLight), or
 * the reverse. The one of the ROM is sav_rom, the file that read_local reads without a forwarder (saves folder set,
 * or saves/ of the subfolder, ADR 0015). The forwarder file wins as soon as it exists (read_local): removing it
 * is enough to go back to the ROM side. Backup copy of the replaced file (files.h). */
static int switch_save_file(game_t *j)
{
    char fwd[600], backup[700], date[20], q[700], warning_text[200], action[64];
    const char *other = rom_program(j);
    int to_rom;
    time_t t = time(NULL);
    result_t r;

    if ((j->system != SYS_GB && j->system != SYS_GBA) || vc(j)) {
        r = make_result(RESULT_GB_GBA_ONLY, NULL);
        show_result(&r, j);
        return 0;
    }
    snprintf(fwd, sizeof fwd, FORWARDERS "/%s.sav", j->name);
    to_rom = !strcmp(j->sav, fwd);
    snprintf(q, sizeof q,
             T("Sur la 3DS, ce jeu se joue avec %s (sauvegarde à côté de la ROM) ou avec un forwarder mGBA "
               "(icône du menu HOME, sauvegarde dans sdmc:/mGBA). 3dssavesync ne synchronise que l'une des deux.\n\n"
               "Actuellement\t%s\nPasser à\t%s",
               "On the 3DS, this game runs with %s (save next to the ROM) or with an mGBA forwarder "
               "(HOME Menu icon, save in sdmc:/mGBA). 3dssavesync syncs only one of the two.\n\n"
               "Now\t%s\nSwitch to\t%s"),
             other, to_rom ? T("forwarder mGBA", "mGBA forwarder") : other, to_rom ? other : T("forwarder mGBA", "mGBA forwarder"));
    snprintf(warning_text, sizeof warning_text, T("La sauvegarde y sera copiée, après une copie de sécurité.%s", "The save is copied there, after a backup.%s"),
             to_rom ? T(" Le forwarder n'aura plus de sauvegarde.", " The forwarder will have no save left.") : "");
    snprintf(action, sizeof action, T("Jouer avec %s", "Play with %s"), to_rom ? other : T("le forwarder", "the forwarder"));
    if (!confirm(j->short_name, NULL, q, warning_text, action)) return 0;
    strftime(date, sizeof date, "%Y%m%d-%H%M%S", localtime(&t));
    mkdir(APP_DIR "/copies", 0777);
    mkdir("sdmc:/mGBA", 0777);
    mkdir(FORWARDERS, 0777);
    make_parent_dir(j->sav_rom);
    if (backup_path(APP_DIR "/copies", j->name, date, "sav", backup, sizeof backup) ||
        move_save(j->sav, to_rom ? j->sav_rom : fwd, backup, to_rom)) {
        r = make_result(RESULT_SWITCH_FAILED, j->sav);
    } else {
        r = make_result(RESULT_SWITCHED, NULL);
        r.program = to_rom ? other : NULL; /* NULL: the mGBA forwarder */
    }
    show_result(&r, j);
    read_local(j);
    return 1;
}

/* What Sync all downloads: To download, a Virtual Console game never started included (its first save is created), but
 * not one with an archive and no sav.dat, nor one whose RAM size is not known. */
static int to_download(const game_t *j) { return j->status == STATUS_TO_DOWNLOAD && !(vc(j) && !j->has_local && (j->vc_archive || !vc_sram_size(j))); }

/* One Upload or Download of Sync all: progress, then the counts and the first failure. */
static void sync_one(game_t *j, int upload, int done, int total, int *ok_count, int *failures, result_t *first, const char **first_game)
{
    result_t res;
    int i = (int)(j - games);
    if (index_of(shown, shown_count, i) >= 0) list_cursor = index_of(shown, shown_count, i); /* the top screen follows the current game */
    progress_color = upload ? C_YELLOW : C_CYAN;
    show_progress(done, total, "%s\n%s", upload ? T("Envoi", "Upload") : T("Récupération", "Download"), j->short_name);
    res = awake_transfer(j, upload);
    if (result_ok(&res)) ++*ok_count;
    else if (!(*failures)++) {
        *first = res;
        *first_game = j->short_name;
    }
    read_local(j);
}

/* START: Uploads first, then the server read again, then Downloads: a Virtual Console game and the ROM or forwarder of
 * the same Key are two Local saves (ADR 0025), so what one uploads the other downloads in the same Sync all. Skips
 * Conflicts, Duplicates, Not synced and the Virtual Console games never started; one confirmation for the overwrites.
 * Then reads the server again, and the message says what was done. */
static void sync_all(void)
{
    char q[1200], first_text[512], first_detail[200], r[120];
    result_t first = make_result(RESULT_NONE, NULL); /* first failure, of first_game */
    const char *first_game = "";
    int i, overwrites = 0, uploads = 0, downloads = 0, skipped = 0, failures = 0, may_overwrite = 1, total = 0, done = 0;
    int planned_uploads = 0, planned_downloads = 0;
    u64 start = osGetTime();

    if (!server_read) {
        set_message(C_RED, NULL, "%s", T("Serveur non lu : appuyez sur \xEE\x80\x83 pour réessayer.", "Server not read: press \xEE\x80\x83 to try again."));
        return;
    }
    operation = T("Tout synchroniser", "Sync all");
    for (i = 0; i < visible_count; i++) planned_uploads += games[visible[i]].status == STATUS_TO_UPLOAD;
    for (i = 0; i < visible_count && planned_uploads; i++) /* uploads never overwrite a 3DS save: no question */
        if (games[visible[i]].status == STATUS_TO_UPLOAD) sync_one(&games[visible[i]], 1, ++done, planned_uploads, &uploads, &failures, &first, &first_game);
    if (planned_uploads) reread_server(); /* the other Local saves of the uploaded Keys are now To download */
    operation = NULL;

    for (i = 0; i < visible_count; i++) {
        const game_t *j = &games[visible[i]];
        planned_downloads += to_download(j);
        skipped += j->status == STATUS_CONFLICT || j->status == STATUS_DUPLICATE || j->status == STATUS_NOT_SYNCED || (j->status == STATUS_TO_DOWNLOAD && !to_download(j));
    }
    sync_summary(r, sizeof r, uploads, planned_downloads, 0, skipped, english);
    snprintf(q, sizeof q, "%s\n\n%s", r, T("Ces sauvegardes de la 3DS seront remplacées par celles du serveur, après une copie de sécurité :\n",
                                           "These 3DS saves will be replaced by the server's, after a backup:\n"));
    for (i = 0; i < visible_count; i++) {
        game_t *j = &games[visible[i]];
        if (to_download(j) && j->has_local && overwrites++ < 8) {
            char t[160] = "";
            if (j->has_progress) progress_text(&j->progress, t, sizeof t);
            if (j->has_progress && j->has_server_progress && !strcmp(j->progress_hash, j->server_hash)) { /* Pokémon: 3DS -> server */
                snprintf(t + strlen(t), sizeof t - strlen(t), T(" ; serveur ", " ; server "));
                progress_text(&j->server_progress, t + strlen(t), sizeof t - strlen(t));
            }
            snprintf(q + strlen(q), sizeof q - strlen(q), "%.40s%s%s%s%s\n", j->short_name, vc(j) ? " (Virtual Console)" : "", t[0] ? " (" : "", t, t[0] ? ")" : "");
        }
    }
    if (overwrites > 8) snprintf(q + strlen(q), sizeof q - strlen(q), plural(overwrites - 8, english, T("… et %d autre\n", "… and %d more\n"), T("… et %d autres\n", "… and %d more\n")), overwrites - 8);
    if (overwrites) may_overwrite = confirm(T("Tout synchroniser", "Sync all"), NULL, q, NULL, T("Remplacer", "Replace"));
    for (i = 0; i < visible_count; i++) total += to_download(&games[visible[i]]) && (may_overwrite || !games[visible[i]].has_local);
    operation = T("Tout synchroniser", "Sync all");
    for (i = 0, done = 0; i < visible_count; i++) {
        game_t *j = &games[visible[i]];
        if (to_download(j) && (may_overwrite || !j->has_local)) sync_one(j, 0, ++done, total, &downloads, &failures, &first, &first_game);
    }
    operation = NULL;
    progress_color = C_GREEN;
    if (total) reread_server();
    /* Summary, then the first failure: the message of the last action says nothing about the others. */
    sync_summary(r, sizeof r, uploads, downloads, failures, skipped, english);
    if (total || planned_uploads) snprintf(r + strlen(r), sizeof r - strlen(r), " \xC2\xB7 %llu s", (osGetTime() - start) / 1000);
    result_text(&first, first_game, english, first_text, sizeof first_text, first_detail, sizeof first_detail); /* "game: reason", and its technical line */
    set_message(failures ? C_RED : total || planned_uploads ? C_GREEN : WHITE, first_detail, "%s%s%s%.199s", r, failures ? "\n" : "", failures > 1 ? T("Premier échec : ", "First failure: ") : "", first_text);
}

/* ---------------------------------------------------------------- History and Restore (ADR 0016) */

static entry_t *versions; /* History of the open Game, newest first: versions[0] is the Current version */
static int version_count, version_cursor;
static const game_t *history_game;

/* Device of a Version (X-Client), as the user names it. */
static const char *client_label(const char *c)
{
    if (!strcmp(c, "3ds")) return "3DS";
    if (!strcmp(c, "pc")) return "PC";
    if (!strcmp(c, "restauration")) return T("Restauration", "Restore");
    return c[0] ? c : "?";
}

/* "512 Kio", "100 octets". */
static void size_text(long n, char *out, size_t l)
{
    if (n >= 1024) snprintf(out, l, T("%ld Kio", "%ld KiB"), (n + 512) / 1024);
    else snprintf(out, l, T("%ld octets", "%ld bytes"), n);
}

/* Top screen of the History: one line per Version (number, age, device, size), the Current version marked in green. */
static void draw_history_top(void)
{
    const int cursor = version_cursor, start = cursor < TOP_LINES ? 0 : cursor - TOP_LINES + 1;
    char t[48], pos[24];
    int i;
    float x;
    screen_top();
    x = 6 + screen_text(6, 4, 0.55f, WHITE, 0, T("Historique", "History")) + 8;
    screen_text(x, 6, 0.45f, GREY_LIGHT, TOP_WIDTH - x - 8, history_game->short_name);
    screen_rect(0, 23, TOP_WIDTH, 1, GREY_DARK);
    if (!version_count)
        screen_paragraph(10, LIST_Y + 13, 0.5f, GREY_LIGHT, TOP_WIDTH - 20, T("Aucune version de ce jeu sur le serveur.", "No version of this game on the server."));
    for (i = start; i < version_count && i < start + TOP_LINES; i++) {
        const entry_t *v = &versions[i];
        float y = LIST_Y + (i - start) * 15;
        if (i == cursor) {
            screen_rect(0, y - 1, TOP_WIDTH, 15, SEL);
            screen_rect(0, y - 1, 2, 15, i ? ACCENT : C_GREEN);
        }
        snprintf(t, sizeof t, T("n\xC2\xB0 %d", "#%d"), atoi(v->version));
        screen_text(8, y + 1, 0.42f, GREY_LIGHT, 0, t);
        format_age(v->age, english, 1, t, sizeof t);
        screen_text(56, y, 0.5f, WHITE, 0, t);
        screen_text(156, y + 1, 0.45f, GREY_LIGHT, 0, client_label(v->client));
        size_text(v->size, t, sizeof t);
        screen_text(246, y + 1, 0.45f, GREY_LIGHT, 0, t);
        if (!i) screen_text(TOP_WIDTH - 8 - screen_text_width(0.42f, T("Actuelle", "Current")), y + 1, 0.42f, C_GREEN, 0, T("Actuelle", "Current"));
    }
    if (version_count > TOP_LINES) { /* scroll bar */
        float h = 15.0f * TOP_LINES * TOP_LINES / version_count;
        screen_rect(TOP_WIDTH - 4, LIST_Y - 1 + (15.0f * TOP_LINES - h) * start / (version_count - TOP_LINES), 2, h, GREY_DARK);
    }
    screen_rect(0, FOOTER_Y, TOP_WIDTH, SCREEN_HEIGHT - FOOTER_Y, BAR);
    pos[0] = '\0';
    if (version_count) snprintf(pos, sizeof pos, "%d / %d", cursor + 1, version_count);
    screen_text(TOP_WIDTH - 6 - screen_text_width(0.38f, pos), FOOTER_Y + 2, 0.38f, GREY_LIGHT, 0, pos);
}

/* GET /saves/<key>/history into versions, newest first. 0 if read; otherwise the failure is in the message. */
static int read_history(const game_t *j)
{
    char url[340], d[200], *body;
    const char *p;
    size_t size;
    long status;
    int n = 1, i;

    show_progress(0, 0, "%s", T("Lecture de l'historique…", "Reading the history…"));
    snprintf(url, sizeof url, "%s/saves/%s/history", cfg.url, j->key);
    status = net_get(url, cfg.token, &body, &size);
    if (status != 200) {
        network_detail(status, url, d, sizeof d);
        set_message(C_RED, d, T("Historique illisible. %s", "Cannot read the history. %s"), network_text(status, net_wifi(), english));
        if (status > 0) free(body);
        return -1;
    }
    for (p = body; (p = strchr(p, '{')); p++) n++; /* one Version per object at most, + 1 for the last read */
    free(versions);
    version_count = version_cursor = 0;
    if (!(versions = malloc(n * sizeof *versions))) {
        free(body);
        set_message(C_RED, T("malloc impossible", "malloc failed"), "%s", T("Mémoire insuffisante pour l'historique. Relancez 3dssavesync.",
                                                                          "Not enough memory for the history. Restart 3dssavesync."));
        return -1;
    }
    for (p = body; next_server_entry(&p, &versions[version_count]);) version_count += versions[version_count].version[0] != '\0';
    free(body);
    for (i = 0; i < version_count / 2; i++) { /* the server lists the oldest first */
        entry_t e = versions[i];
        versions[i] = versions[version_count - 1 - i];
        versions[version_count - 1 - i] = e;
    }
    return 0;
}

/* A on an old Version: read it, check its Hash, show it (with its play time for a Pokémon game) and ask; then the
 * conditional Upload as "restauration" (ADR 0016): it becomes the Current version, the 3DS save is not touched.
 * 1 if restored; -1 if the Current version changed meanwhile (412: read the History again); 0 otherwise (message). */
static int restore_version(const game_t *j, const entry_t *v)
{
    char url[420], d[200], q[900], hash[65], when[120], loc[200], t[80];
    char *body;
    size_t size;
    long status;
    progress_t p;
    int has_progress, ok;

    show_progress(0, 0, "%s", T("Lecture de la version…", "Reading the version…"));
    snprintf(url, sizeof url, "%s/saves/%s/history/%s", cfg.url, j->key, v->version);
    status = net_get(url, cfg.token, &body, &size);
    if (status != 200) {
        network_detail(status, url, d, sizeof d);
        if (status == 404) set_message(C_RED, d, "%s", T("Cette version n'est plus sur le serveur. Rien n'a changé : choisissez-en une autre.",
                                                         "This version is no longer on the server. Nothing changed: choose another one."));
        else set_message(C_RED, d, T("Lecture de la version impossible. %s Rien n'a changé.", "Cannot read the version. %s Nothing changed."),
                         network_text(status, net_wifi(), english));
        if (status > 0) free(body);
        return 0;
    }
    hash_hex((unsigned char *)body, size, hash);
    if (strcmp(hash, v->hash)) {
        snprintf(d, sizeof d, T("SHA-256 reçu %.12s\xE2\x80\xA6, attendu %.12s\xE2\x80\xA6", "SHA-256 received %.12s\xE2\x80\xA6, expected %.12s\xE2\x80\xA6"), hash, v->hash);
        set_message(C_RED, d, "%s", T("La version reçue est abîmée : rien n'a été restauré. Réessayez.", "The version arrived damaged, so nothing was restored. Try again."));
        free(body);
        return 0;
    }
    format_age(v->age, english, 1, t, sizeof t);
    snprintf(when, sizeof when, "%s \xC2\xB7 %s", t, client_label(v->client));
    has_progress = j->has_progress && pokemon_progress((unsigned char *)body, size, &p) == 0; /* same check as server_progress */
    if (has_progress) progress_text(&p, t, sizeof t);
    local_text(j, loc, sizeof loc);
    snprintf(q, sizeof q, "%s\t%s%s%s\n%s\t%s\n\n%s", T("Cette version", "This version"), when, has_progress ? "\n\t" : "", has_progress ? t : "",
             T("Sur la 3DS", "On the 3DS"), loc,
             T("Elle redevient la version actuelle du serveur. Rien n'est effacé : l'actuelle reste dans l'historique. "
                       "Ensuite, 3dssavesync propose de la récupérer sur la 3DS, après une copie de sécurité de sa sauvegarde.",
                       "It becomes the current version on the server again. Nothing is deleted: the current one stays in the history. "
                       "Then 3dssavesync offers to download it to the 3DS, after a backup of its save."));
    ok = confirm(T("Restaurer cette version ?", "Restore this version?"), j->short_name, q, NULL, T("Restaurer", "Restore"));
    if (ok) {
        char condition[100];
        snprintf(condition, sizeof condition, "If-Match: \"%s\"", versions[0].hash); /* the Current version the user saw */
        snprintf(url, sizeof url, "%s/saves/%s", cfg.url, j->key);
        show_progress(0, 0, "%s", T("Restauration…", "Restoring…"));
        status = net_put(url, cfg.token, "restauration", condition, body, size);
    }
    free(body);
    if (!ok) return 0;
    if (status == 412) {
        set_message(C_RED, "HTTP 412", "%s", T("La sauvegarde vient de changer sur le serveur : un appareil l'a envoyée entre-temps. Rien n'a été restauré. "
                                               "Voici la liste à jour : choisissez de nouveau.",
                                               "The save just changed on the server: a device uploaded it meanwhile. Nothing was restored. "
                                               "Here is the updated list: choose again."));
        return -1;
    }
    log_event("restore %s \"%s\" version %s: HTTP %ld", j->key, j->name, v->version, status);
    if (status != 200) {
        network_detail(status, url, d, sizeof d);
        set_message(C_RED, d, T("Restauration impossible. %s", "Restore failed. %s"), network_text(status, net_wifi(), english));
        return 0;
    }
    return 1;
}

/* History (stylus): the server Versions of the Game, newest first. A restores an old one (restore_version); then the
 * normal Download brings it to the 3DS, after a Backup copy (act, ADR 0016): no other write path. */
static void history_screen(game_t *j)
{
    int restored = 0;
    char t[200], done[120];

    history_game = j;
    if (read_history(j)) return;
    draw_current_top = draw_history_top; /* progress and questions: the History stays on top */
    while (aptMainLoop()) {
        const entry_t *v = version_count ? &versions[version_cursor] : NULL;
        const int same = v && !strcmp(v->hash, versions[0].hash); /* the Current version, or the same content */
        const button_t b[2] = { { KEY_A, T("Restaurer", "Restore"), v && !same }, { KEY_B, T("Revenir", "Back"), 1 } };
        float y = 6;
        u32 k;

        modal_begin(draw_history_top);
        draw_game_icon(j);
        y += screen_lines(8, y, 0.6f, WHITE, ICON_X - 16, 2, j->short_name);
        screen_text(8, y + 1, 0.4f, GREY_LIGHT, ICON_X - 16, T("Versions sur le serveur", "Versions on the server"));
        y = y + 18 < UNDER_ICON ? UNDER_ICON : y + 20;
        if (v) {
            u32 c = version_cursor ? WHITE : C_GREEN;
            if (version_cursor) snprintf(t, sizeof t, T("Version n\xC2\xB0 %d", "Version #%d"), atoi(v->version));
            else snprintf(t, sizeof t, "%s", T("Version actuelle", "Current version"));
            screen_rect(8, y + 2, 3, 13, c);
            screen_text(15, y, 0.5f, c, 0, t);
            y += 21;
            format_age(v->age, english, 1, t, sizeof t);
            y = detail(y, T("Date", "Date"), t[0] ? t : "?");
            y = detail(y, T("Appareil", "Device"), client_label(v->client));
            size_text(v->size, t, sizeof t);
            y = detail(y, T("Taille", "Size"), t) + 5;
            screen_paragraph(8, y, 0.42f, GREY_LIGHT, TEXT_WIDTH,
                             !version_cursor ? T("La version que les appareils récupèrent. Choisissez une version plus ancienne pour la restaurer.",
                                                 "The version the devices download. Choose an older one to restore it.")
                             : same ? T("Même contenu que la version actuelle : rien à restaurer.", "Same content as the current version: nothing to restore.")
                                    : T("\xEE\x80\x80 la remet en version actuelle sur le serveur, puis propose de la récupérer sur la 3DS, après une copie de sécurité.",
                                        "\xEE\x80\x80 makes it the current version on the server again, then offers to download it to the 3DS, after a backup."));
        } else {
            screen_paragraph(8, y, 0.45f, WHITE, TEXT_WIDTH, T("Ce jeu n'a encore aucune version sur le serveur : rien à restaurer. Envoyez d'abord sa sauvegarde avec \xEE\x80\x80 dans la liste.",
                                                               "This game has no version on the server yet, so there is nothing to restore. Upload its save first with \xEE\x80\x80 in the list."));
        }
        draw_message(BAR_TOP_1);
        k = modal_end(b, 2, 18);

        if (k & KEY_B) break;
        if (k & (KEY_UP | KEY_DOWN | KEY_LEFT | KEY_RIGHT)) clear_message();
        version_cursor = move_cursor(k, version_cursor, version_count, TOP_LINES);
        if (k & KEY_A && v && !same) {
            int r;
            clear_message();
            snprintf(done, sizeof done, T("%s : version n\xC2\xB0 %d restaurée sur le serveur.", "%s: version #%d restored on the server."), j->short_name, atoi(v->version));
            r = restore_version(j, v);
            if (r > 0) {
                restored = 1;
                break;
            }
            if (r < 0 && read_history(j)) break; /* 412: the list again; a failed read goes back to the game list, with its message */
        }
    }
    free(versions);
    versions = NULL;
    version_count = 0;
    draw_current_top = draw_list_top;
    if (!restored) return;
    reread_server();
    /* The server changed, not the 3DS: its State says what comes next. To download: the usual question and Download. */
    set_message(C_GREEN, NULL, "%s\n%s", done, to_download(j) ? T("Récupérez-la sur la 3DS avec \xEE\x80\x80, après une copie de sécurité.",
                                                                           "Download it to the 3DS with \xEE\x80\x80, after a backup.")
                                                                         : state_sentence(j, t, sizeof t));
    if (to_download(j) && act(j)) reread_server(); /* Virtual Console: state_sentence says it comes later */
}

/* ---------------------------------------------------------------- loop */

/* Top screen with the app name only (network not started). */
static void draw_title_top(void)
{
    screen_top();
    screen_text(6, 4, 0.55f, WHITE, 0, "3dssavesync");
}

int main(int argc, char **argv)
{
    int cursor = 0;

    draw_current_top = draw_list_top;
    gfxInitDefault();
    osSetSpeedupEnable(true); /* 804 MHz on New 3DS (no effect on Old 3DS): faster TLS and SHA-256 */
    screen_init();
    if (R_SUCCEEDED(cfguInit())) {
        CFGU_GetSystemLanguage(&language);
        cfguExit();
    }
    english = language != CFG_LANGUAGE_FR;
    if (migrate_dir(OLD_DIR, APP_DIR) == 1)
        snprintf(message, sizeof message, T("Réglages repris de %s.", "Settings taken from %s."), OLD_DIR);
    read_config(APP_DIR "/config.ini", &cfg);
    log_event("start " VERSION ", server %s", cfg.url[0] ? "set" : "not set"); /* never the address or the token: the file may go in a bug report */
    if (read_bases(APP_DIR "/base.ini", &bases)) { /* nothing is written to base.ini then: it would lose Bases */
        add_alert(T("Mémoire insuffisante pour lire l'état de synchronisation : il ne sera pas enregistré. Redémarrez la 3DS, puis relancez 3dssavesync.",
                    "Not enough memory to read the sync state: it will not be saved. Restart the 3DS, then start 3dssavesync again."));
        snprintf(alert_detail, sizeof alert_detail, "%s (%d)", APP_DIR "/base.ini", bases.n);
    }
    off_count = read_off_titles(APP_DIR "/titres-off.ini", off, MAX_OFF);
    update_start(argc > 0 ? argv[0] : NULL);

    if (net_init()) {
        const button_t b = { KEY_B, T("Quitter", "Quit"), 1 };
        u32 k;
        loaded = 1;
        do {
            modal_begin(draw_title_top);
            screen_paragraph(16, 70, 0.55f, C_RED, PROGRESS_WIDTH, T("Impossible de démarrer le réseau. Redémarrez la 3DS et relancez 3dssavesync.", "Couldn't start the network. Restart the 3DS and 3dssavesync."));
            k = modal_end(&b, 1, 0);
        } while (!(k & (KEY_B | KEY_START)));
        screen_exit();
        gfxExit();
        return 0;
    }
    net_on_slow(show_slow); /* every Server request comes after a show_progress */
    switch (net_read_cas(CUSTOM_CAS)) {
    case 0: log_event(CUSTOM_CAS " added to the trusted certificates of the server"); break;
    case -1: log_event(CUSTOM_CAS " ignored: unreadable"); break;
    case -2: log_event(CUSTOM_CAS " ignored: larger than 256 KiB"); break;
    }

    if (!cfg.url[0] || !cfg.token[0]) {
        char migration_message[512];
        snprintf(migration_message, sizeof migration_message, "%s", message);
        if (!cfg.url[0]) snprintf(cfg.url, sizeof cfg.url, "https://");
        settings_screen(1); /* message: result of the connection test */
        snprintf(message + strlen(message), sizeof message - strlen(message), "%s%.200s", migration_message[0] && message[0] ? "\n" : "", migration_message);
    }
    if (!should_quit) refresh();
    show_alert();
    if (!should_quit && cfg.check_update) update_check_if_due(); /* verifier_maj=0: GitHub only on A, Version line */
    while (!should_quit && aptMainLoop()) {
        u32 k;
        list_cursor = cursor;
        draw();
        k = read_keys();

        if (k & KEY_B) {
            if (!query[0]) break;
            query[0] = '\0'; /* B clears the search first */
            cursor = refilter(cursor);
            continue;
        }
        if (k & (KEY_UP | KEY_DOWN | KEY_LEFT | KEY_RIGHT | KEY_L | KEY_R)) clear_message(); /* the message is about the game being left */
        cursor = move_cursor(k, cursor, shown_count, list_lines(query));
        if (k & (KEY_L | KEY_R)) {
            tab = next_tab(tab, k & KEY_R ? 1 : -1, tab_counts);
            cursor = refilter(cursor);
        }
        if (k & KEY_TOUCH) {
            edit_search(query, sizeof query);
            cursor = refilter(cursor);
        }
        if (k & (KEY_A | KEY_START | KEY_X | KEY_Y | KEY_SELECT | KEY_HISTORY)) {
            int selected = shown_count ? shown[cursor] : -1;
            clear_message();
            /* Read everything again: Y, or settings changed. After an action: the Game it touched is already read again, the server only. */
            if (k & KEY_SELECT) {
                if (settings_screen(0) && !should_quit) refresh();
                else sort_list(); /* the order may have changed */
            } else if (k & KEY_Y) {
                refresh();
            } else if (k & KEY_A && shown_count) {
                if (act(&games[selected])) reread_server();
            } else if (k & KEY_X && shown_count) {
                if (switch_save_file(&games[selected])) reread_server();
            } else if (k & KEY_START) {
                sync_all();
            } else if (k & KEY_HISTORY && shown_count) {
                history_screen(&games[selected]);
            }
            show_alert();
            if (index_of(shown, shown_count, selected) >= 0) cursor = index_of(shown, shown_count, selected); /* the same game, at its new place */
            else if (cursor >= shown_count) cursor = shown_count ? shown_count - 1 : 0;
            list_cursor = cursor;
        }
    }
    net_exit();
    screen_exit();
    gfxExit();
    return 0;
}
