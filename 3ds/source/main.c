/* 3dssavesync 3DS: syncs the Saves with the Server (docs/design.md §4):
 * DS (TWiLight), GB/GBC (TWiLight), GBA (open_agb_firm) and the 3DS games of titres.ini (ADR 0007 to 0009).
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
#include "games.h"
#include "key.h"
#include "list.h"
#include "net.h"
#include "pokemon.h"
#include "result.h"
#include "screen.h"
#include "settings.h"
#include "state.h"
#include "text.h"
#include "titles3ds.h"
#include "update.h"
#include "views.h"

#define OLD_DIR "sdmc:/3ds/nds-save-sync" /* name before v3: settings and Bases taken over at first start */
#define FORWARDERS "sdmc:/mGBA/forwarders" /* Saves of the mGBA forwarders */
#define MAX_GAMES 512 /* ponytail: fixed array; beyond 512 ROMs, the next ones are skipped (and reported) */

int english; /* console language: French, otherwise English (views.h) */
static u8 language;   /* CFG_LANGUAGE_*: title of the 3DS games in this language */

typedef struct {
    char key[KEY_MAX]; /* ADR 0007 */
    system_t system;
    char rom[600];  /* ROM (DS icon); empty for the 3DS */
    char sav[600];  /* Save file; empty for the 3DS (Save archive) */
    char sav_rom[600]; /* GB/GBA: its Save without the mGBA forwarder (saves folder) */
    u64 title;      /* 3DS: title ID */
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
    progress_t progress;
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
static title_t titles[MAX_TITLES]; /* synced 3DS games (titres.ini) */
static int title_count;
static game_t games[MAX_GAMES];
static int game_count;
static bases_t bases;
static int visible[MAX_GAMES], visible_count;
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

/* Whole content of the Local save: the file (Clock trailer included), or /main of the 3DS Save archive. */
static int read_save(game_t *j, unsigned char **bytes, size_t *size)
{
    if (j->system == SYS_3DS) return read_title_save(j->title, &j->media, bytes, size);
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
static int skipped_roms; /* ROMs beyond MAX_GAMES */
static aliases_t aliases; /* alias.ini (ADR 0014) */

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
        if (game_count == MAX_GAMES) {
            skipped_roms++;
            continue;
        }
        j = &games[game_count++];
        memset(j, 0, sizeof *j);
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
    j->has_local = j->blank = j->has_progress = 0;
    j->modified = 0;
    j->local_size = 0;
    j->local_hash[0] = '\0';
    if (j->system == SYS_DS) snprintf(j->sav, sizeof j->sav, "%s", j->sav_rom);
    else if (j->system != SYS_3DS) {
        char f[600];
        snprintf(f, sizeof f, FORWARDERS "/%s.sav", j->name);
        snprintf(j->sav, sizeof j->sav, "%s", access(f, F_OK) == 0 ? f : j->sav_rom);
    }
    if (j->rom_count != 1 || read_save(j, &bytes, &size)) return;
    if (j->system != SYS_3DS) {
        size = sram_size(j->system, size);
        to_server_form(j, &bytes, &size);
        j->blank = is_blank_save(j->system, bytes, size);
        /* stat() does not give the date on the SD. archive_getmtime converts it to UNIX time; a 0 timestamp (Azahar)
         * becomes 1 January 2000: date unknown. */
        if (R_SUCCEEDED(archive_getmtime(j->sav, &mtime)) && mtime > 946684800) j->modified = (time_t)mtime;
    }
    j->has_progress = pokemon_progress(bytes, size, &j->progress) == 0; /* SRAM only: without the Clock trailer */
    hash_hex(bytes, size, j->local_hash);
    j->local_size = size;
    j->has_local = 1;
    free(bytes);
}

/* The 3DS games of titres.ini whose Save archive exists (game installed or card inserted, ADR 0009). */
static void scan_3ds(void)
{
    int i;
    for (i = 0; i < title_count && game_count < MAX_GAMES; i++) {
        game_t *j = &games[game_count];
        show_progress(i + 1, title_count, T("Lecture des sauvegardes 3DS\n%s", "Reading 3DS saves\n%s"), titles[i].name);
        memset(j, 0, sizeof *j);
        j->title = titles[i].id;
        j->system = SYS_3DS;
        j->rom_count = 1;
        read_local(j); /* Hash, and play time of a Pokémon */
        if (!j->has_local) continue; /* never started or absent */
        key_3ds(j->title, j->key);
        snprintf(j->name, sizeof j->name, "%s", titles[i].name);
        snprintf(j->short_name, sizeof j->short_name, "%.47s", titles[i].name);
        game_count++;
    }
}

/* All sources, then the Hash of each Local save (SRAM only for GB/GBA, ADR 0008). */
static void scan(void)
{
    int i, before;

    game_count = visible_count = loaded = skipped_roms = 0; /* the list shown is no longer valid during the read */
    read_aliases(APP_DIR "/alias.ini", &aliases);
    scan_dir(SYS_DS, cfg.roms, cfg.saves, cfg.roms, 0); /* empty saves = next to the ROM */
    scan_dir(SYS_GB, cfg.roms_gb, cfg.saves_gb, cfg.roms_gb, 0);
    scan_dir(SYS_GBA, cfg.roms_gba, cfg.saves_gba, cfg.roms_gba, 0);
    if (skipped_roms) {
        char t[80];
        snprintf(t, sizeof t, T("%d ROM ignorées : plus de %d jeux.", "%d ROMs skipped: more than %d games."), skipped_roms, MAX_GAMES);
        add_alert(t);
    }
    before = game_count;
    for (i = 0; i < before; i++) {
        show_progress(i + 1, before, T("Lecture des sauvegardes\n%s", "Reading saves\n%s"), games[i].short_name);
        read_local(&games[i]);
    }
    scan_3ds();
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
        for (i = 0; i < game_count && strcmp(games[i].key, e.code); i++);
        if (i == game_count || !e.hash[0] || e.size < 0) continue; /* no local ROM: not shown */
        strcpy(games[i].server_hash, e.hash);
        games[i].server_size = e.size;
        strcpy(games[i].date, e.date);
        strcpy(games[i].client, e.client);
        games[i].age = e.age;
        games[i].has_server = 1;
        games[i].has_name = e.has_name;
    }
    free(body);
    /* Name of each game for the History on the PC (ADR 0018), once per game. Stop at the first refusal: an older
     * server does not know this address, and the list stays readable without the names. */
    for (i = 0; i < game_count; i++) {
        if (!games[i].has_server || games[i].has_name) continue;
        show_progress(i + 1, game_count, T("Envoi des noms des jeux\n%s", "Sending game names\n%s"), games[i].short_name);
        snprintf(url, sizeof url, "%s/saves/%.31s/name", cfg.url, games[i].key); /* Key: 31 characters at most (KEY_MAX) */
        if (net_put(url, cfg.token, NULL, games[i].name, strlen(games[i].name)) != 204) break;
        games[i].has_name = 1;
    }
    return 0;
}

static int compare_visible(const void *a, const void *b)
{
    const game_t *x = &games[*(const int *)a], *y = &games[*(const int *)b];
    int d = status_priority(x->status) - status_priority(y->status);
    return d ? d : strcasecmp(x->short_name, y->short_name);
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
        j->status = game_status(&o, base_of(&bases, j->key), server_read);
        if (base_outdated(j->status, base_of(&bases, j->key), j->local_hash)) outdated[n++] = i;
        if (j->status != STATUS_HIDDEN) visible[visible_count++] = i;
    }
    qsort(visible, visible_count, sizeof *visible, compare_visible);
    return n;
}

/* compute_states, then the Bases written (the only write of an In sync Base). */
static void update_states(void)
{
    static int outdated[MAX_GAMES];
    int i, n = compute_states(outdated);
    for (i = 0; i < n; i++) save_base(games[outdated[i]].key, games[outdated[i]].local_hash);
}

/* After an action: the Game it touched is already read again (read_local), only the server changes. */
static void reread_server(void)
{
    show_progress(0, 0, "%s", T("Lecture du serveur…", "Reading the server…"));
    read_server();
    update_states(); /* server not read: the local games stay listed, with no action */
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
    status = net_put(url, cfg.token, condition, bytes, size);
    free(bytes);
    if (status == 412) return make_result(RESULT_UPLOAD_CONFLICT, NULL);
    if (status != 200) return network_failure(RESULT_UPLOAD_FAILED, status, url);
    save_base(j->key, hash);
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
 *  - 3DS: rewrite /main with commit and Secure value (ADR 0009).
 * On failure, the Local save is intact. */
static result_t download_save(game_t *j)
{
    char tmp[610], backup[700], url[300], hash[65], date[20];
    char *body;
    unsigned char *old_data = NULL, *new_data;
    size_t size, n = 0, sram;
    long status;
    int failed, exists = j->system == SYS_3DS || access(j->sav, F_OK) == 0;
    time_t t = time(NULL);
    result_t r;

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
    /* Copy as soon as the Save exists, even if the scan could not read it: never overwrite without a copy. */
    if (exists) {
        strftime(date, sizeof date, "%Y%m%d-%H%M%S", localtime(&t));
        mkdir(APP_DIR "/copies", 0777);
        /* same ROM name in two subfolders, the same second: -1, -2… (a copy is never overwritten) */
        failed = backup_path(APP_DIR "/copies", j->name, date, j->system == SYS_3DS ? "main" : "sav", backup, sizeof backup);
        if (!failed) failed = read_save(j, &old_data, &n);
        if (!failed) failed = write_verified(backup, old_data, n); /* read back: a damaged copy protects nothing */
        if (failed) {
            free(old_data);
            free(body);
            return make_result(RESULT_BACKUP_FAILED, backup);
        }
    }

    if (j->system == SYS_3DS) {
        failed = write_title_save(j->title, j->media, body, size);
        free(old_data);
        free(body);
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
    save_base(j->key, j->server_hash);
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

/* Play time of the server version (downloaded), only if the 3DS has a known Pokémon game. 1 if read. */
static int server_progress(const game_t *j, progress_t *p)
{
    char url[300], *body;
    size_t size;
    long status;
    int ok;
    if (!j->has_server || !j->has_progress) return 0;
    show_progress(0, 0, "%s", T("Lecture de la version du serveur…", "Reading the server version…"));
    snprintf(url, sizeof url, "%s/saves/%s", cfg.url, j->key);
    status = net_get(url, cfg.token, &body, &size);
    ok = status == 200 && pokemon_progress((unsigned char *)body, size, p) == 0;
    if (status > 0) free(body);
    return ok;
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
    if (j->system == SYS_3DS && title_icon(j->title, j->media, icon) == 0) screen_icon(icon, ICON_SIZE, ICON_SIZE, ICON_X, 6, 1.0f);
    else if (j->system != SYS_3DS && read_ds_icon(j, icon) == 0) screen_icon(icon, 32, 32, ICON_X, 6, 1.5f); /* 32 x 1.5 = 48 */
}

/* Counter of the title bar: "%d conflicts"… */
static const char *counter_format(status_t s, int n)
{
    switch (s) {
    case STATUS_CONFLICT: return plural(n, english, T("%d conflit", "%d conflict"), T("%d conflits", "%d conflicts"));
    case STATUS_TO_DOWNLOAD: return T("%d à récupérer", "%d to download");
    case STATUS_TO_UPLOAD: return T("%d à envoyer", "%d to upload");
    case STATUS_DUPLICATE: return plural(n, english, T("%d doublon", "%d duplicate"), T("%d doublons", "%d duplicates"));
    default: return T("%d à jour", "%d in sync");
    }
}

/* Top screen of the list: title and counters (those that do not fit are dropped, "in sync" first), the games, then
 * the footer: the server, when it was read, the position in the list. */
static void draw_list_top(void)
{
    static const status_t states[] = { STATUS_CONFLICT, STATUS_TO_DOWNLOAD, STATUS_TO_UPLOAD, STATUS_DUPLICATE, STATUS_IN_SYNC }; /* order of status_priority() */
    static const status_t all[] = { STATUS_TO_UPLOAD, STATUS_TO_DOWNLOAD, STATUS_CONFLICT, STATUS_DUPLICATE, STATUS_OFFLINE, STATUS_IN_SYNC };
    const int cursor = list_cursor;
    int i, count = 0, n[5] = { 0 }, start = cursor < TOP_LINES ? 0 : cursor - TOP_LINES + 1;
    status_t e[5];
    char t[5][40], footer[200], age[32], pos[24];
    const char *host = strstr(cfg.url, "://") ? strstr(cfg.url, "://") + 3 : cfg.url;
    float l[5], x, xs = 0, xn;

    screen_top();
    x = 6 + screen_text(6, 4, 0.55f, WHITE, 0, "3dssavesync") + 12;
    if (loaded && server_read) { /* without the server, the counters would be wrong */
        for (i = 0; i < visible_count; i++) n[status_priority(games[visible[i]].status)]++;
        for (i = 0; i < 5; i++) {
            if (!n[i]) continue;
            e[count] = states[i];
            snprintf(t[count], sizeof t[count], counter_format(e[count], n[i]), n[i]);
            l[count] = screen_text_width(0.42f, t[count]);
            count++;
        }
        count = texts_fitting(l, count, 12, TOP_WIDTH - 6 - x);
        x = TOP_WIDTH - 6;
        for (i = count - 1; i >= 0; i--) {
            x -= l[i];
            screen_text(x, 7, 0.42f, e[i] == STATUS_IN_SYNC ? C_GREEN : state_color(e[i]), 0, t[i]);
            x -= 12;
        }
    }
    screen_rect(0, 23, TOP_WIDTH, 1, GREY_DARK);

    /* Columns measured on all State words: nothing is cut, and they do not move from one frame to the next. */
    for (i = 0; i < 6; i++)
        if (screen_text_width(0.42f, state_label(all[i])) > xs) xs = screen_text_width(0.42f, state_label(all[i]));
    xs = 18 + xs * WIDTH_MARGIN + 6;
    xn = xs + screen_text_width(0.42f, "GBA") * WIDTH_MARGIN + 8;
    if (!visible_count) screen_paragraph(10, 40, 0.5f, GREY_LIGHT, TOP_WIDTH - 20, !loaded ? T("Lecture en cours…", "Reading…")
                                                              : server_read ? T("Aucun jeu à afficher.", "No game to show.")
                                                              : T("Serveur non lu, et aucune sauvegarde sur la 3DS.", "Server not read, and no save on the 3DS."));
    for (i = start; i < visible_count && i < start + TOP_LINES; i++) {
        game_t *k = &games[visible[i]];
        float y = LIST_Y + (i - start) * 15;
        if (i == cursor) {
            screen_rect(0, y - 1, TOP_WIDTH, 15, SEL);
            screen_rect(0, y - 1, 2, 15, dot_color(k->status));
        }
        screen_rect(8, y + 5, 5, 5, dot_color(k->status));
        screen_text(18, y + 1, 0.42f, state_color(k->status), 0, state_label(k->status));
        screen_text(xs, y + 1, 0.42f, GREY_LIGHT, 0, SYSTEM_NAMES[k->system]);
        screen_text(xn, y, 0.5f, WHITE, TOP_WIDTH - xn - 8, k->short_name);
    }
    if (visible_count > TOP_LINES) { /* scroll bar */
        float h = 195.0f * TOP_LINES / visible_count;
        screen_rect(TOP_WIDTH - 4, 26 + (195.0f - h) * start / (visible_count - TOP_LINES), 2, h, GREY_DARK);
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
    if (visible_count) snprintf(pos, sizeof pos, "%d / %d", cursor + 1, visible_count);
    x = TOP_WIDTH - 6 - screen_text_width(0.38f, pos);
    screen_text(x, FOOTER_Y + 2, 0.38f, GREY_LIGHT, 0, pos);
    screen_text(8, FOOTER_Y + 2, 0.38f, loaded && !server_read ? C_RED : GREY_LIGHT, x - 8 - 12, footer);
}

/* Under the State word: why, and what A does. */
static const char *state_sentence(const game_t *j, char *t, size_t n)
{
    switch (j->status) {
    case STATUS_TO_UPLOAD: return T("La sauvegarde de la 3DS est plus récente. \xEE\x80\x80 l'envoie au serveur.", "The 3DS save is newer. \xEE\x80\x80 uploads it to the server.");
    case STATUS_TO_DOWNLOAD: return T("Le serveur a une version plus récente. \xEE\x80\x80 la récupère, après une copie de sécurité.",
                               "The server has a newer version. \xEE\x80\x80 downloads it, after a backup.");
    case STATUS_CONFLICT: return T("La sauvegarde a changé sur la 3DS et sur le serveur. \xEE\x80\x80 pour choisir laquelle garder.",
                           "The save changed on the 3DS and on the server. \xEE\x80\x80 to choose which one to keep.");
    case STATUS_DUPLICATE:
        if (j->shares_save) snprintf(t, n, T("%d ROM ont ce nom de fichier et partageraient la même sauvegarde. Renommez-en une.",
                                         "%d ROMs have this file name and would share one save. Rename one."), j->rom_count);
        else snprintf(t, n, T("%d ROM contiennent ce jeu. Gardez-en une, ou ajoutez les autres aux ROM hacks sur le PC, puis préparez la carte SD.",
                              "%d ROMs hold this game. Keep one, or add the others to ROM hacks on the PC, then prepare the SD card."), j->rom_count);
        return t;
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
        const char *b = base_of(&bases, j->key);
        u32 c = j->status == STATUS_IN_SYNC ? C_GREEN : state_color(j->status);
        gb = j->system == SYS_GB || j->system == SYS_GBA;
        action = j->status == STATUS_TO_UPLOAD || j->status == STATUS_TO_DOWNLOAD || j->status == STATUS_CONFLICT;
        draw_game_icon(j);
        y += screen_lines(8, y, 0.6f, WHITE, ICON_X - 16, 2, j->short_name);
        /* System, and for GB/GBA the program whose save is synced (X changes it). */
        if (gb) snprintf(t, sizeof t, "%s \xC2\xB7 %s", system_label(j), forwarder(j) ? T("forwarder mGBA", "mGBA forwarder") : rom_program(j));
        else snprintf(t, sizeof t, "%s", system_label(j));
        screen_text(8, y + 1, 0.4f, GREY_LIGHT, ICON_X - 16, t);
        y = y + 18 < UNDER_ICON ? UNDER_ICON : y + 20;
        screen_rect(8, y + 2, 3, 13, c);
        screen_text(15, y, 0.5f, c, 0, state_label(j->status));
        y += 19;
        y += screen_lines(8, y, 0.42f, WHITE, TEXT_WIDTH, j->status == STATUS_DUPLICATE ? 4 : 2, state_sentence(j, t, sizeof t)) + 5;
        if (j->status != STATUS_DUPLICATE) { /* a Duplicate has no save and no version to show */
            local_text(j, t, sizeof t);
            y = detail(y, T("Sur la 3DS", "On the 3DS"), t);
            server_text(j, t, sizeof t);
            y = detail(y, T("Sur le serveur", "On the server"), t);
            detail(y, T("Dernière synchro", "Last sync"), base_label(b, j));
        }
    }
    if (message[0]) draw_message(BAR_TOP_2);
    else if (update_is_newer(VERSION, update_info.latest)) { /* no message: the update, at the same place */
        snprintf(t, sizeof t, T("Version %s disponible (SELECT)", "Version %s available (SELECT)"), update_info.latest);
        draw_band(BAR_TOP_2, t, NULL, C_CYAN);
    }

    /* Bar: on top, what A (always shown, greyed out when there is nothing to do) and X (GB/GBA) do for this game;
     * at the bottom, the general buttons. */
    draw_bar_background(2);
    {
        char x[80];
        button_t game_buttons[2] = { { KEY_A, action_label(j), action }, { KEY_X, x, 1 } };
        const button_t general[4] = { { KEY_Y, T("Rafraîchir", "Refresh"), 1 }, { KEY_START, T("Tout synchro", "Sync all"), 1 },
                                        { KEY_SELECT, T("Réglages", "Settings"), 1 }, { KEY_B, T("Quitter", "Quit"), 1 } };
        if (gb) snprintf(x, sizeof x, T("Jouer avec %s", "Play with %s"), forwarder(j) ? rom_program(j) : T("le forwarder mGBA", "the mGBA forwarder"));
        draw_button_row(game_buttons, gb ? 2 : 1, BAR_ROW_2, 18);
        draw_button_row(general, 4, BAR_ROW_2 + BUTTON_HEIGHT, 0);
    }
}

static void draw(void)
{
    modal_begin(draw_list_top); /* not modal, but the same frame: two-line bar instead of modal_end */
    draw_bottom(visible_count ? &games[visible[list_cursor]] : NULL);
    screen_end();
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

/* Synced 3DS games: installed applications and game card, ticked if they are in titres.ini.
 * Only a game whose Save archive holds only /main can be ticked (title_main_only): the sync copies
 * only this file. The games of titres.ini absent from the console stay listed (to untick them). */
typedef struct {
    u64 id;
    FS_MediaType media;
    char name[48];
    int main_only, ticked;
} title_choice_t;

static int compare_choices(const void *a, const void *b)
{
    return strcasecmp(((const title_choice_t *)a)->name, ((const title_choice_t *)b)->name);
}

static title_choice_t choices[200];
static int choice_count, choice_cursor;

/* Top screen of the choice: a check box per game. */
static void draw_choices_top(void)
{
    const int cursor = choice_cursor, start = cursor < TOP_LINES ? 0 : cursor - TOP_LINES + 1;
    int i;
    screen_top();
    screen_text(6, 4, 0.55f, WHITE, 0, T("Jeux 3DS à synchroniser", "3DS games to sync"));
    screen_rect(0, 23, TOP_WIDTH, 1, GREY_DARK);
    for (i = start; i < choice_count && i < start + TOP_LINES; i++) {
        const title_choice_t *c = &choices[i];
        float ly = LIST_Y + (i - start) * 15;
        if (i == cursor) {
            screen_rect(0, ly - 1, TOP_WIDTH, 15, SEL);
            screen_rect(0, ly - 1, 2, 15, ACCENT);
        }
        screen_rect(9, ly + 2, 10, 10, c->ticked || c->main_only == 1 ? GREY_LIGHT : GREY_DARK); /* box: border, background, tick */
        screen_rect(10, ly + 3, 8, 8, i == cursor ? SEL : BOX);
        if (c->ticked) screen_rect(11, ly + 4, 6, 6, C_GREEN);
        screen_text(26, ly, 0.5f, c->ticked || c->main_only == 1 ? WHITE : GREY_DARK, TOP_WIDTH - 40, c->name);
    }
    if (start > 0) screen_text(TOP_WIDTH - 12, 26, 0.4f, GREY_LIGHT, 0, "\xE2\x96\xB2");
    if (start + TOP_LINES < choice_count) screen_text(TOP_WIDTH - 12, FOOTER_Y, 0.4f, GREY_LIGHT, 0, "\xE2\x96\xBC");
}

static void choose_titles(void)
{
    title_choice_t *c = choices;
    static u64 ids[200];
    static FS_MediaType medias[200];
    int n, i, changed = 0;

    show_progress(0, 0, "%s", T("Lecture des jeux installés…", "Reading installed games…"));
    if (R_FAILED(amInit())) {
        set_message(C_RED, NULL, "%s", T("Impossible de lire la liste des jeux. Redémarrez la 3DS et réessayez.", "Can't read the installed games. Restart the 3DS and try again."));
        return;
    }
    n = installed_titles(ids, medias, 200);
    amExit();
    for (i = 0; i < title_count && n < 200; i++) {
        int k;
        for (k = 0; k < n && ids[k] != titles[i].id; k++);
        if (k == n) {
            ids[n] = titles[i].id;
            medias[n++] = MEDIATYPE_SD;
        }
    }
    {   /* no Save archive and not ticked: hidden (mGBA or DS forwarders, game never started) */
        int m = 0;
        for (i = 0; i < n; i++) {
            int t = title_index(titles, title_count, ids[i]), main_only;
            show_progress(i + 1, n, "%s", T("Lecture des jeux", "Reading games"));
            main_only = title_main_only(ids[i]);
            if (main_only < 0 && t < 0) continue;
            c[m].id = ids[i];
            c[m].media = medias[i];
            c[m].ticked = t >= 0;
            c[m].main_only = main_only;
            if (title_name(ids[i], medias[i], language, c[m].name, sizeof c[m].name)) {
                if (t >= 0) snprintf(c[m].name, sizeof c[m].name, "%s", titles[t].name);
                else snprintf(c[m].name, sizeof c[m].name, "%016llX", ids[i]);
            }
            m++;
        }
        n = m;
    }
    qsort(c, n, sizeof *c, compare_choices);
    clear_message();
    choice_count = n;
    choice_cursor = 0;

    while (aptMainLoop()) {
        const int cursor = choice_cursor;
        const title_choice_t *ch = n ? &c[cursor] : NULL;
        int tickable = ch && (ch->ticked || ch->main_only == 1);
        u16 icon[ICON_SIZE * ICON_SIZE];
        float y = 6;
        u32 k;
        button_t t[2] = { { KEY_A, ch && ch->ticked ? T("Décocher", "Untick") : T("Cocher", "Tick"), tickable },
                          { KEY_B, T("Enregistrer et revenir", "Save and go back"), 1 } };

        modal_begin(draw_choices_top);
        if (ch) {
            if (title_icon(ch->id, ch->media, icon) == 0) screen_icon(icon, ICON_SIZE, ICON_SIZE, ICON_X, 6, 1.0f);
            y += screen_lines(8, y, 0.55f, WHITE, ICON_X - 16, 2, ch->name) + 4;
            y = y < UNDER_ICON ? UNDER_ICON + 4 : y + 4;
            if (ch->main_only == 1) y += screen_paragraph(8, y, 0.45f, C_GREEN, TEXT_WIDTH, T("Ce jeu peut être synchronisé.", "This game can be synced.")) + 6;
            else if (ch->main_only == 0)
                y += screen_paragraph(8, y, 0.45f, C_RED, TEXT_WIDTH, T("Ce jeu ne peut pas être synchronisé (plusieurs fichiers de sauvegarde).",
                                                                       "This game can't be synced (several save files).")) + 6;
            else
                y += screen_paragraph(8, y, 0.45f, GREY_LIGHT, TEXT_WIDTH, T("Pas encore de sauvegarde : lancez le jeu une fois et sauvegardez.",
                                                                            "No save yet: start the game once and save.")) + 6;
        } else {
            y += screen_paragraph(8, y, 0.5f, GREY_LIGHT, TEXT_WIDTH, T("Aucun jeu trouvé.", "No game found.")) + 6;
        }
        screen_paragraph(8, y, 0.42f, GREY_LIGHT, TEXT_WIDTH, T("Un jeu manque ? Lancez-le une fois et sauvegardez.", "Game missing? Start it once and save."));
        draw_message(BAR_TOP_1);
        k = modal_end(t, 2, 18);

        if (k & (KEY_UP | KEY_DOWN)) clear_message();
        if (k & KEY_B) break;
        if (k & KEY_DOWN && choice_cursor + 1 < n) choice_cursor++;
        if (k & KEY_UP && choice_cursor > 0) choice_cursor--;
        if (k & KEY_A && n) {
            clear_message();
            if (tickable) {
                c[choice_cursor].ticked = !c[choice_cursor].ticked;
                changed = 1;
            } else {
                set_message(C_RED, NULL, "%s", T("Ce jeu ne peut pas être coché.", "This game cannot be ticked."));
            }
        }
    }
    clear_message();
    if (changed) {
        title_count = 0;
        for (i = 0; i < n && title_count < MAX_TITLES; i++)
            if (c[i].ticked) {
                titles[title_count].id = c[i].id;
                snprintf(titles[title_count++].name, sizeof titles[0].name, "%s", c[i].name);
            }
        mkdir(APP_DIR, 0777);
        if (write_titles(APP_DIR "/titres.ini", titles, title_count))
            set_message(C_RED, APP_DIR "/titres.ini", "%s", T("Liste des jeux 3DS non enregistrée. Vérifiez la carte SD.", "3DS game list not saved. Check the SD card."));
        else set_message(C_GREEN, NULL, "%s", T("Liste des jeux 3DS enregistrée.", "3DS game list saved."));
    }
}

/* Editable setting lines, then 3DS games, Connection and Version (settings screen). */
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
static int selected_setting; /* selected settings line: also for the top screen during a progress */

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

/* "3.7.0 · Up to date" from x: version in white, dot in grey, State in color, cut before end. */
static void draw_version_value(float x, float y, float size, float end)
{
    char e[48];
    u32 c = version_state(e, sizeof e);
    x += screen_text(x, y, size, WHITE, end - x, VERSION);
    x += screen_text(x, y, size, GREY_DARK, 0, " \xC2\xB7 ");
    screen_text(x, y, size, c, end - x, e);
}

static const char *setting_label(int i)
{
    if (i < SETTING_COUNT) return T(SETTINGS[i].name, SETTINGS[i].name_en);
    if (i == SETTING_COUNT) return T("Jeux 3DS", "3DS games");
    if (i == SETTING_COUNT + 1) return T("Connexion", "Connection");
    return T("Version", "Version");
}

/* What A does on line i, drawn on the right of the selected line. */
static const char *setting_action(int i)
{
    if (i < SETTING_COUNT) return T("Modifier", "Change");
    if (i == SETTING_COUNT) return T("Choisir", "Choose");
    if (i == SETTING_COUNT + 1) return T("Tester", "Test");
    return update_is_newer(VERSION, update_info.latest) ? T("Mettre à jour", "Update") : T("Rechercher", "Check");
}

/* Top screen of the settings: label on the left, value in a column, "Ⓐ action" on the right of the selected line. */
static void draw_settings_top(void)
{
    const int total = SETTING_COUNT + 3;
    float y = 29;
    int i;
    screen_top();
    screen_text(6, 4, 0.55f, WHITE, 0, T("Réglages", "Settings"));
    screen_rect(0, 23, TOP_WIDTH, 1, GREY_DARK);
    for (i = 0; i < total; i++) {
        char v[24];
        const char *value = v;
        u32 c = WHITE;
        int required = 0;
        float end = TOP_WIDTH - 8;
        if (i < SETTING_COUNT) {
            value = SETTINGS[i].value;
            if (i <= 1 && (!value[0] || !strcmp(value, "https://"))) {
                value = T("À remplir", "Required");
                c = GREY_LIGHT;
                required = 1;
            } else if (i == 1) {
                value = "\xE2\x80\xA2\xE2\x80\xA2\xE2\x80\xA2\xE2\x80\xA2\xE2\x80\xA2\xE2\x80\xA2\xE2\x80\xA2\xE2\x80\xA2"; /* •••••••• */
            } else if (!value[0]) {
                value = T("À côté de la ROM", "Next to the ROM"); /* default value */
                c = GREY_LIGHT;
            }
        } else if (i == SETTING_COUNT) {
            if (title_count) snprintf(v, sizeof v, plural(title_count, english, T("%d jeu", "%d game"), T("%d jeux", "%d games")), title_count);
            else snprintf(v, sizeof v, "%s", T("Aucun", "None"));
        } else if (i == SETTING_COUNT + 1) {
            value = test_state > 0 ? T("Réussie", "OK") : test_state < 0 ? test_short : T("Non testée", "Not tested");
            c = test_state > 0 ? C_GREEN : test_state < 0 ? C_RED : GREY_LIGHT;
        }
        if (i == selected_setting) {
            const char *a = setting_action(i);
            float xa = TOP_WIDTH - 8 - screen_text_width(0.44f, a), g = screen_text_width(0.5f, glyph(KEY_A));
            screen_rect(0, y - 1, TOP_WIDTH, 16, SEL);
            screen_rect(0, y - 1, 2, 16, ACCENT);
            screen_text(xa - g - 3, y - 1, 0.5f, GLYPH_A, 0, glyph(KEY_A));
            screen_text(xa, y + 1, 0.44f, WHITE, 0, a);
            end = xa - g - 3 - 10;
        }
        if (required) screen_rect(0, y - 1, 2, 16, C_RED);
        screen_text(8, y, 0.46f, i == selected_setting ? WHITE : GREY_LIGHT, 128, setting_label(i));
        if (i == SETTING_COUNT + 2) draw_version_value(142, y, 0.46f, end);
        else screen_text(142, y, 0.46f, c, end - 142, value);
        y += 17 + (i == 1 || i == SETTING_COUNT - 1 ? 7 : 0); /* groups: server, folders, the rest */
    }
}

/* Settings screen: one line per setting, A for the action of the line, explanation at the bottom. first: no setting
 * yet. 1 if a setting or the list of 3DS games changed (then everything must be read again). */
static int settings_screen(int first)
{
    const int count = SETTING_COUNT, total = count + 3;
    int changed;
    static title_t before_titles[MAX_TITLES];
    int before_count = title_count;
    char before[sizeof cfg];

    memcpy(before, &cfg, sizeof cfg);
    memcpy(before_titles, titles, sizeof titles);
    selected_setting = 0;
    draw_current_top = draw_settings_top; /* progress and questions: the settings stay on top */
    clear_message();
    while (aptMainLoop()) {
        const int c = selected_setting;
        u32 k;
        float y = 6;
        /* First start: B tests the connection first; a second B after a failure continues anyway. */
        button_t t[2] = { { KEY_A, setting_action(c), 1 },
                          { KEY_B, !first || test_state > 0 ? T("Enregistrer et revenir", "Save and go back")
                                   : test_state < 0           ? T("Continuer quand même", "Continue anyway")
                                                             : T("Tester et continuer", "Test and continue"), 1 } };

        modal_begin(draw_settings_top);
        if (first) {
            const char *b = T("Bienvenue ! Entrez d'abord le serveur et le jeton.", "Welcome! Enter the server and the token first.");
            float h = screen_paragraph_height(0.45f, BOTTOM_WIDTH - 24, b) + 8;
            screen_rect(0, 0, BOTTOM_WIDTH, h, CARD);
            screen_rect(0, 0, 3, h, ACCENT);
            screen_paragraph(10, 4, 0.45f, WHITE, BOTTOM_WIDTH - 24, b);
            y = h + 6;
        }
        if (c == count + 2) {
            y += screen_lines(8, y, 0.55f, WHITE, TEXT_WIDTH, 1, T("Mise à jour de 3dssavesync", "3dssavesync update")) + 8;
            screen_text(8, y, 0.42f, GREY_LIGHT, 100, T("Installée", "Installed"));
            draw_version_value(112, y, 0.42f, BOTTOM_WIDTH - 8);
            y += 15 + 6;
            y += screen_paragraph(8, y, 0.42f, GREY_LIGHT, TEXT_WIDTH, T("Cherche une nouvelle version sur GitHub et vous demande avant de l'installer.",
                                                              "Looks for a new version on GitHub and asks before installing it.")) + 6;
        } else {
            y += screen_lines(8, y, 0.55f, WHITE, TEXT_WIDTH, 1, setting_label(c)) + 6;
            y += screen_paragraph(8, y, 0.45f, WHITE, TEXT_WIDTH, c < count ? T(SETTINGS[c].help, SETTINGS[c].help_en)
                                                         : c == count ? T("Les jeux 3DS à synchroniser. Par défaut, les 8 Pokémon 3DS.", "3DS games to sync. Default: the 8 Pokémon 3DS games.")
                                                                   : T("Vérifie l'adresse et le jeton du serveur.", "Checks the server address and token.")) + 6;
        }
        if (message[0]) draw_message(BAR_TOP_1);
        else if (c == count + 2 && update_detail[0]) { /* last check: text, then its technical line (after the last line break) */
            char d[sizeof update_detail];
            char *nl;
            u32 color = version_state(NULL, 0);
            snprintf(d, sizeof d, "%s", update_detail);
            nl = strrchr(d, '\n');
            if (nl) *nl++ = '\0';
            draw_band(BAR_TOP_1, d, nl, color == GREY_LIGHT ? WHITE : color);
        }
        k = modal_end(t, 2, 18);
        if (k & KEY_B) {
            if (!first || test_state) break;
            clear_message();
            if (test_connection()) break;
            continue;
        }
        if (k & KEY_DOWN && c + 1 < total) selected_setting++;
        if (k & KEY_UP && c > 0) selected_setting--;
        if (k & KEY_A) {
            clear_message();
            if (c < count) {
                size_t len;
                test_state = 0; /* the address or the token may have changed: the last test no longer counts */
                edit_text(T(SETTINGS[c].name, SETTINGS[c].name_en), SETTINGS[c].value, SETTINGS[c].n, c == 1);
                len = strlen(SETTINGS[c].value);
                while (len > 0 && SETTINGS[c].value[len - 1] == '/') SETTINGS[c].value[--len] = '\0'; /* "…/saves/" -> "…/saves" */
            } else if (c == count) {
                choose_titles();
            } else if (c == count + 1) {
                test_connection();
            } else {
                should_quit = update_app();
                if (should_quit) break;
            }
        }
    }
    draw_current_top = draw_list_top;
    if (!first) clear_message(); /* first start: the result of the test stays shown */
    changed = memcmp(before, &cfg, sizeof cfg) || before_count != title_count || memcmp(before_titles, titles, sizeof titles);
    if (memcmp(before, &cfg, sizeof cfg)) save_config();
    return changed;
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
        r = upload_save(j);
        done = 1;
        break;
    case STATUS_TO_DOWNLOAD: {
        int has_server_progress = server_progress(j, &p);
        server_text(j, srv, sizeof srv);
        local_text(j, loc, sizeof loc);
        if (has_server_progress) progress_text(&p, t, sizeof t);
        snprintf(q, sizeof q, "%s\t%s%s%s\n%s\t%s", T("Sur le serveur", "On the server"), srv, has_server_progress ? "\n\t" : "", has_server_progress ? t : "", T("Sur la 3DS", "On the 3DS"), loc);
        if ((done = confirm(T("Récupérer la version du serveur ?", "Download the server version?"), j->short_name, q,
                              j->has_local ? T("La sauvegarde de la 3DS sera remplacée, après une copie de sécurité.", "The 3DS save will be replaced, after a backup.") : NULL,
                              T("Récupérer", "Download"))))
            r = download_save(j);
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
        if (k & KEY_A) r = upload_save(j);
        else if (k & KEY_X) r = download_save(j);
        else r = make_result(RESULT_CONFLICT_LEFT, NULL);
        break;
    }
    case STATUS_DUPLICATE:
        r = make_result(j->shares_save ? RESULT_DUPLICATE_SHARED : RESULT_DUPLICATE, NULL);
        r.count = j->rom_count;
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

    if (j->system != SYS_GB && j->system != SYS_GBA) {
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

/* START: Uploads and Downloads; skips Conflicts and Duplicates; one confirmation for the overwrites. Then reads the
 * server again, and the message says what was done. */
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
    for (i = 0; i < visible_count; i++) {
        status_t e = games[visible[i]].status;
        planned_uploads += e == STATUS_TO_UPLOAD;
        planned_downloads += e == STATUS_TO_DOWNLOAD;
        skipped += e == STATUS_CONFLICT || e == STATUS_DUPLICATE;
    }
    sync_summary(r, sizeof r, planned_uploads, planned_downloads, 0, skipped, english);
    snprintf(q, sizeof q, "%s\n\n%s", r, T("Ces sauvegardes de la 3DS seront remplacées par celles du serveur, après une copie de sécurité :\n",
                                           "These 3DS saves will be replaced by the server's, after a backup:\n"));
    for (i = 0; i < visible_count; i++) {
        game_t *j = &games[visible[i]];
        if (j->status == STATUS_TO_DOWNLOAD && j->has_local && overwrites++ < 8) {
            char t[80] = "";
            if (j->has_progress) progress_text(&j->progress, t, sizeof t);
            snprintf(q + strlen(q), sizeof q - strlen(q), "%.40s%s%s%s\n", j->short_name, t[0] ? " (" : "", t, t[0] ? ")" : "");
        }
    }
    if (overwrites > 8) snprintf(q + strlen(q), sizeof q - strlen(q), plural(overwrites - 8, english, T("… et %d autre\n", "… and %d more\n"), T("… et %d autres\n", "… and %d more\n")), overwrites - 8);
    if (overwrites) may_overwrite = confirm(T("Tout synchroniser", "Sync all"), NULL, q, NULL, T("Remplacer", "Replace"));
    for (i = 0; i < visible_count; i++) {
        game_t *j = &games[visible[i]];
        total += j->status == STATUS_TO_UPLOAD || (j->status == STATUS_TO_DOWNLOAD && (may_overwrite || !j->has_local));
    }
    operation = T("Tout synchroniser", "Sync all");
    for (i = 0; i < visible_count; i++) {
        game_t *j = &games[visible[i]];
        int do_upload = j->status == STATUS_TO_UPLOAD;
        int do_download = j->status == STATUS_TO_DOWNLOAD && (may_overwrite || !j->has_local);
        if (do_upload || do_download) {
            result_t res;
            int ok;
            list_cursor = i; /* the top screen follows the current game */
            progress_color = do_upload ? C_YELLOW : C_CYAN;
            show_progress(++done, total, "%s\n%s", do_upload ? T("Envoi", "Upload") : T("Récupération", "Download"), j->short_name);
            res = do_upload ? upload_save(j) : download_save(j);
            ok = result_ok(&res);
            uploads += do_upload && ok;
            downloads += do_download && ok;
            if (!ok && !failures++) {
                first = res;
                first_game = j->short_name;
            }
            read_local(j);
        }
    }
    operation = NULL;
    progress_color = C_GREEN;
    if (total) reread_server();
    /* Summary, then the first failure: the message of the last action says nothing about the others. */
    sync_summary(r, sizeof r, uploads, downloads, failures, skipped, english);
    if (total) snprintf(r + strlen(r), sizeof r - strlen(r), " \xC2\xB7 %llu s", (osGetTime() - start) / 1000);
    result_text(&first, first_game, english, first_text, sizeof first_text, first_detail, sizeof first_detail); /* "game: reason", and its technical line */
    set_message(failures ? C_RED : total ? C_GREEN : WHITE, first_detail, "%s%s%s%.199s", r, failures ? "\n" : "", failures > 1 ? T("Premier échec : ", "First failure: ") : "", first_text);
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
    read_bases(APP_DIR "/base.ini", &bases);
    title_count = read_titles(APP_DIR "/titres.ini", titles, MAX_TITLES);
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

    if (!cfg.url[0] || !cfg.token[0]) {
        char migration_message[512];
        snprintf(migration_message, sizeof migration_message, "%s", message);
        if (!cfg.url[0]) snprintf(cfg.url, sizeof cfg.url, "https://");
        settings_screen(1); /* message: result of the connection test */
        snprintf(message + strlen(message), sizeof message - strlen(message), "%s%.200s", migration_message[0] && message[0] ? "\n" : "", migration_message);
    }
    if (!should_quit) refresh();
    show_alert();
    if (!should_quit) update_check_if_due();
    while (!should_quit && aptMainLoop()) {
        u32 k;
        list_cursor = cursor;
        draw();
        k = read_keys();

        if (k & KEY_B) break;
        if (k & (KEY_UP | KEY_DOWN)) clear_message(); /* the message is about the game being left */
        if (k & KEY_DOWN && cursor + 1 < visible_count) cursor++;
        if (k & KEY_UP && cursor > 0) cursor--;
        if (k & (KEY_A | KEY_START | KEY_X | KEY_Y | KEY_SELECT)) {
            int selected = visible_count ? visible[cursor] : -1, i;
            clear_message();
            /* Read everything again: Y, or settings changed. After an action: the Game it touched is already read again, the server only. */
            if (k & KEY_SELECT) {
                if (settings_screen(0) && !should_quit) refresh();
            } else if (k & KEY_Y) {
                refresh();
            } else if (k & KEY_A && visible_count) {
                if (act(&games[selected])) reread_server();
            } else if (k & KEY_X && visible_count) {
                if (switch_save_file(&games[selected])) reread_server();
            } else if (k & KEY_START) {
                sync_all();
            }
            show_alert();
            for (i = 0; i < visible_count && visible[i] != selected; i++);
            if (i < visible_count) cursor = i; /* the same game, at its new place */
            else if (cursor >= visible_count) cursor = visible_count ? visible_count - 1 : 0;
            list_cursor = cursor;
        }
    }
    net_exit();
    screen_exit();
    gfxExit();
    return 0;
}
