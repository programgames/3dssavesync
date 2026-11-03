/* 3dssavesync 3DS : synchronise les Sauvegardes avec le Serveur (docs/design.md §4) :
 * DS (TWiLight), GB/GBC (TWiLight), GBA (open_agb_firm) et les jeux 3DS de titres.ini (ADR 0007 à 0009).
 * Haut : liste des Jeux et leur État. Bas : détails, messages, touches. */
#include <3ds.h>
#include <dirent.h>
#include <mbedtls/sha256.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <strings.h>
#include <sys/stat.h>
#include <time.h>
#include <unistd.h>
#include "errors.h"
#include "files.h"
#include "key.h"
#include "list.h"
#include "net.h"
#include "pokemon.h"
#include "screen.h"
#include "settings.h"
#include "state.h"
#include "text.h"
#include "titles3ds.h"
#include "update.h"

#define APP_DIR "sdmc:/3ds/3dssavesync"
#define OLD_DIR "sdmc:/3ds/nds-save-sync" /* nom avant la v3 : réglages et Bases repris au premier lancement */
#define FORWARDERS "sdmc:/mGBA/forwarders" /* Sauvegardes des forwarders mGBA */
#define MAX_GAMES 512 /* ponytail: tableau fixe ; au-delà de 512 ROM, les suivantes sont ignorées (et signalées) */
#define TOP_LINES 13 /* lignes d'une liste sur l'écran du haut (pied en dessous) */
#define HIDDEN -1   /* ni Sauvegarde locale ni Version courante : pas affiché */
#define DUPLICATE 100 /* plusieurs ROM pour une Clé (ADR 0005) */
#define OFFLINE 101 /* serveur non lu : état inconnu, aucune action proposée */
#define GITHUB_LATEST "https://api.github.com/repos/programgames/3dssavesync/releases/latest" /* mises à jour (ADR 0021) */

static int english; /* langue de la console : français, sinon anglais */
static u8 language;   /* CFG_LANGUAGE_* : titre des jeux 3DS dans cette langue */
#define T(fr, en) (english ? (en) : (fr))

/* Palette : un sens par couleur (le jaune ne dit que « À envoyer »). */
#define WHITE RGB(0xE8EAED)
#define GREY_LIGHT RGB(0x8A929C)  /* libellé */
#define GREY_DARK RGB(0x5A626C)  /* désactivé */
#define C_GREEN RGB(0x4CC060)  /* À jour, réussite */
#define C_YELLOW RGB(0xF0C040) /* À envoyer */
#define C_CYAN RGB(0x40C8E8)  /* À récupérer, version disponible */
#define C_RED RGB(0xF05050) /* Conflit, échec */
#define BAR RGB(0x1E242C)   /* barre des touches, pied de l'écran du haut */
#define CARD RGB(0x2A313B)   /* cartes du conflit, bande du message */
#define SEL RGB(0x263445)     /* ligne choisie */
#define ACCENT RGB(0x4A8FE0)  /* trait de la ligne choisie des réglages */

typedef struct {
    char key[KEY_MAX]; /* ADR 0007 */
    system_t system;
    char rom[600];  /* ROM (icône DS) ; vide pour la 3DS */
    char sav[600];  /* fichier de Sauvegarde ; vide pour la 3DS (archive) */
    char sav_rom[600]; /* GB/GBA : sa Sauvegarde sans forwarder mGBA (dossier des sauvegardes) */
    u64 title;      /* 3DS : title ID */
    FS_MediaType media; /* 3DS : SD ou cartouche */
    char name[256];  /* nom de la ROM sans extension = nom du .sav */
    char short_name[48]; /* pour l'affichage seulement : "Pokemon Diamant" */
    int rom_count;
    int shares_save;    /* Doublon : ROM de Clés différentes qui partagent un fichier de Sauvegarde (same_save) */
    obs_t local, server;
    char local_hash[65], server_hash[65], date[17], client[17];
    long age; /* secondes depuis la Version courante (horloge du serveur), -1 si inconnu */
    int has_local, has_server;
    int has_name;      /* le serveur connaît le nom du jeu (ADR 0018) */
    int blank; /* SRAM locale vierge : comptée comme absente (key.h) */
    time_t modified;   /* DS, GB, GBA : date du fichier (horloge de la 3DS) ; 0 si inconnue */
    int has_progress;   /* Pokémon : temps de jeu et dresseur lus (pokemon.h) */
    int mbc2;       /* GB : cartouche MBC2, octet 0x147 de la ROM (ADR 0012) */
    progress_t progress;
    int state;
} game_t;

/* "Pokemon - Version Diamant (France) (Rev 5)" -> "Pokemon Diamant" : retire " - Version" et les étiquettes "(...)". */
static void make_short_name(const char *name, char *out, size_t n)
{
    const char *v = strstr(name, " - Version ");
    char tmp[256];
    char *end;
    if (v) snprintf(tmp, sizeof tmp, "%.*s %s", (int)(v - name), name, v + 11);
    else snprintf(tmp, sizeof tmp, "%s", name);
    if ((end = strstr(tmp, " ("))) *end = '\0';
    snprintf(out, n, "%.*s", (int)n - 1, tmp); /* coupé exprès : nom d'affichage */
}

static struct { char url[256], token[256], roms[256], saves[256], roms_gb[256], saves_gb[256], roms_gba[256], saves_gba[256]; } cfg;
static title_t titles[MAX_TITLES]; /* jeux 3DS synchronisés (titres.ini) */
static int title_count;
static game_t games[MAX_GAMES];
static int game_count;
static struct { char key[KEY_MAX], hash[65]; } bases[MAX_GAMES];
static int base_count;
static int visible[MAX_GAMES], visible_count;
static char message[512];        /* message de la dernière action : bande au-dessus de la barre des touches */
static char message_detail[200]; /* sa ligne technique (HTTP, curl, chemin), en gris sous le texte */
static u32 message_color = WHITE; /* trait de la bande : rouge échec, vert réussite, blanc information */

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

/* Mise à jour (ADR 0021). VERSION vient du Makefile : "3.7.0" pour un tag, "dev" sinon (jamais de mise à jour). */
static long last_check_day = -1;   /* jour de la dernière vérification (maj.ini) */
static char latest_version[32]; /* dernière version vue sur GitHub (maj.ini) */
static char installed_version[32]; /* version qui a tourné en dernier (maj.ini) : lue par le PC avant de copier un .cia */
static char update_detail[400]; /* résultat de la dernière vérification ou installation : écran des réglages */
static int update_failed;        /* update_detail décrit un échec */
static const char *path_3dsx = ""; /* argv[0] : le .3dsx lancé depuis le Homebrew Launcher (launched_from_sd) */
static int should_quit;          /* mise à jour installée : l'application se ferme */
static char alert[300]; /* problème vu pendant une lecture ou une action : ajouté au message à la fin */
static char alert_detail[200]; /* ligne technique de la lecture du serveur ratée */

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

static int list_cursor; /* jeu choisi dans la liste : l'écran du haut des dialogues et de la progression */
static int loaded;            /* 0 tant que la première lecture n'est pas finie */
static int server_read;    /* 0 si la dernière lecture du serveur a échoué : les jeux locaux restent listés */
static u64 read_time;      /* osGetTime() de la dernière lecture réussie du serveur */
static void show_progress(int done, int total, const char *format, ...);
static void draw_list_top(void);
static void (*draw_current_top)(void) = draw_list_top; /* écran du haut pendant une question ou une progression */
static const char *operation; /* titre de la longue opération en cours (Tout synchroniser), sinon NULL */
static u32 progress_color = C_GREEN; /* barre de progression : jaune envoi, cyan récupération, vert le reste */

/* ---------------------------------------------------------------- fichiers */

static void hash_hex(const unsigned char *bytes, size_t size, char out[65])
{
    unsigned char h[32];
    int i;
    mbedtls_sha256_ret(bytes, size, h, 0);
    for (i = 0; i < 32; i++) sprintf(out + 2 * i, "%02x", h[i]);
}

/* ---------------------------------------------------------------- config et Base */

static void read_ini(const char *path, void (*key)(const char *, const char *))
{
    FILE *f = fopen(path, "r");
    char line[512];
    if (!f) { /* coupure entre le retrait de l'ancien et le renommage (replace_file) : le .tmp est complet */
        snprintf(line, sizeof line, "%s.tmp", path);
        f = fopen(line, "r");
    }
    if (!f) return;
    while (fgets(line, sizeof line, f)) {
        char *eq;
        line[strcspn(line, "\r\n")] = '\0';
        if ((eq = strchr(line, '='))) {
            *eq = '\0';
            key(line, eq + 1);
        }
    }
    fclose(f);
}

static void config_entry(const char *k, const char *v)
{
    if (!strcmp(k, "url")) snprintf(cfg.url, sizeof cfg.url, "%s", v);
    else if (!strcmp(k, "jeton")) snprintf(cfg.token, sizeof cfg.token, "%s", v);
    else if (!strcmp(k, "roms")) snprintf(cfg.roms, sizeof cfg.roms, "%s", v);
    else if (!strcmp(k, "saves")) snprintf(cfg.saves, sizeof cfg.saves, "%s", v);
    else if (!strcmp(k, "roms_gb")) snprintf(cfg.roms_gb, sizeof cfg.roms_gb, "%s", v);
    else if (!strcmp(k, "roms_gba")) snprintf(cfg.roms_gba, sizeof cfg.roms_gba, "%s", v);
    else if (!strcmp(k, "saves_gb")) snprintf(cfg.saves_gb, sizeof cfg.saves_gb, "%s", v);
    else if (!strcmp(k, "saves_gba")) snprintf(cfg.saves_gba, sizeof cfg.saves_gba, "%s", v);
}

static void write_config(void)
{
    char t[sizeof cfg + 100];
    mkdir(APP_DIR, 0777);
    snprintf(t, sizeof t, "url=%s\njeton=%s\nroms=%s\nsaves=%s\nroms_gb=%s\nsaves_gb=%s\nroms_gba=%s\nsaves_gba=%s\n", cfg.url,
             cfg.token, cfg.roms, cfg.saves, cfg.roms_gb, cfg.saves_gb, cfg.roms_gba, cfg.saves_gba);
    if (replace_file(APP_DIR "/config.ini", t, strlen(t)))
        add_alert(T("Réglages non enregistrés. Vérifiez la carte SD.", "Settings not saved. Check the SD card."));
}

static void base_entry(const char *k, const char *v)
{
    if (base_count < MAX_GAMES && strlen(k) >= 4 && strlen(k) < KEY_MAX && strlen(v) == 64) {
        snprintf(bases[base_count].key, KEY_MAX, "%s", k);
        snprintf(bases[base_count].hash, 65, "%s", v);
        base_count++;
    }
}

static const char *base_of(const char *key)
{
    int i;
    for (i = 0; i < base_count; i++)
        if (!strcmp(bases[i].key, key)) return bases[i].hash;
    return NULL;
}

/* La seule écriture de Base (design §1) : après À jour, Envoi ou Récupération réussis. */
static void set_base(const char *key, const char *hash)
{
    static char t[MAX_GAMES * (KEY_MAX + 66)];
    size_t n = 0;
    int i;
    for (i = 0; i < base_count && strcmp(bases[i].key, key); i++);
    if (i == base_count) {
        if (base_count == MAX_GAMES) return;
        snprintf(bases[base_count++].key, KEY_MAX, "%s", key);
    }
    snprintf(bases[i].hash, 65, "%s", hash);
    for (i = 0; i < base_count; i++) n += snprintf(t + n, sizeof t - n, "%s=%s\n", bases[i].key, bases[i].hash);
    if (replace_file(APP_DIR "/base.ini", t, n)) /* une coupure pendant l'écriture ne vide plus base.ini */
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

/* Contenu complet de la Sauvegarde locale : le fichier (Trailer d'horloge compris), ou /main de l'archive 3DS. */
static int read_save(game_t *j, unsigned char **bytes, size_t *size)
{
    if (j->system == SYS_3DS) return read_title_save(j->title, &j->media, bytes, size);
    return read_file(j->sav, bytes, size);
}

static int forwarder(const game_t *j) { return !strncmp(j->sav, FORWARDERS "/", sizeof FORWARDERS); }

/* SRAM du fichier local -> forme du serveur (ADR 0012) : EEPROM d'open_agb_firm remise dans l'ordre de mGBA, MBC2 en
 * 512 octets. Les copies de sécurité gardent les octets du fichier. */
static void to_server_form(const game_t *j, unsigned char **bytes, size_t *size)
{
    unsigned char *s;
    if (j->system == SYS_GBA && !forwarder(j)) eeprom_swap(*bytes, *size);
    if (!j->mbc2 || !(s = malloc(MBC2_SERVER))) return;
    if (mbc2_normalize(*bytes, *size, s)) { /* taille inconnue : telle quelle */
        free(s);
        return;
    }
    free(*bytes);
    *bytes = s;
    *size = MBC2_SERVER;
}

/* Forme du serveur -> SRAM du fichier local : l'inverse de to_server_form. MBC2 à la taille du fichier existant (cible),
 * sinon 256 octets pour le forwarder mGBA et 8 Kio pour TWiLight Menu++. 0 si fait. */
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

/* Alias (alias.ini, ADR 0014) : "nom de la ROM sans extension=CLÉ", pour qu'un ROM hack ait sa propre Clé. */
#define MAX_ALIAS 64
static struct { char name[256], value[KEY_MAX]; } alias[MAX_ALIAS];
static int alias_count;

static void alias_entry(const char *k, const char *v)
{
    size_t n = strlen(k);
    while (n && k[n - 1] == ' ') n--; /* "Pokemon Radical Red = GBA-0RAD" */
    if (alias_count == MAX_ALIAS || !n || n >= sizeof alias[0].name) return;
    snprintf(alias[alias_count].name, sizeof alias[0].name, "%.*s", (int)n, k);
    snprintf(alias[alias_count].value, sizeof alias[0].value, "%s", v); /* coupé : alias_key le refusera */
    alias_count++;
}

static const char *alias_of(const char *name)
{
    int i;
    for (i = 0; i < alias_count; i++)
        if (!strcmp(alias[i].name, name)) return alias[i].value;
    return NULL;
}

/* Dossiers jamais lus : sauvegardes, cachés, et ceux de TWiLight Menu++ et du système (_nds, 3ds, luma, Nintendo 3DS, mGBA). */
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

/* Une source de ROM, sous-dossiers compris (ADR 0015) : Clé lue dans l'en-tête (ou alias), Sauvegarde nommée comme la
 * ROM (save_path). Deux ROM de même Clé, ou de même fichier de Sauvegarde (forwarder mGBA compris) : Doublon. */
#define MAX_DEPTH 4 /* niveaux de sous-dossiers lus */
static int skipped_roms; /* ROM au-delà de MAX_GAMES */

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
        if ((a = alias_of(name))) {
            if (alias_key(s, a, key)) {
                char t[200];
                snprintf(t, sizeof t, T("Alias invalide pour « %.40s » : ROM non synchronisée. Préparez de nouveau la carte SD depuis le PC.",
                                        "Invalid alias for \"%.40s\": ROM not synced. Prepare the SD card again from the PC."), name);
                add_alert(t);
                continue;
            }
        } else if (rom_key(s, header, key)) continue; /* homebrew ou en-tête illisible */

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

/* Sauvegarde locale d'un Jeu : son fichier (forwarder mGBA s'il existe, docs/guide-forwarders.md) et son Hash
 * (SRAM seule pour GB/GBA, ADR 0008 ; /main pour la 3DS). Appelée au scan, et après une action sur ce seul Jeu. */
static void read_local(game_t *j)
{
    unsigned char *bytes;
    size_t size;
    u64 mtime;
    j->has_local = j->blank = j->has_progress = 0;
    j->modified = 0;
    j->local.size = 0;
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
        /* stat() ne donne pas la date sur la SD. archive_getmtime la convertit en temps UNIX ; un horodatage 0 (Azahar)
         * devient le 1er janvier 2000 : date inconnue. */
        if (R_SUCCEEDED(archive_getmtime(j->sav, &mtime)) && mtime > 946684800) j->modified = (time_t)mtime;
    }
    j->has_progress = pokemon_progress(bytes, size, &j->progress) == 0; /* SRAM seule : sans Trailer d'horloge */
    hash_hex(bytes, size, j->local_hash);
    j->local.size = size;
    j->has_local = 1;
    free(bytes);
}

/* Les jeux 3DS de titres.ini dont l'archive existe (jeu installé ou cartouche insérée, ADR 0009). */
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
        read_local(j); /* Hash, et temps de jeu d'un Pokémon */
        if (!j->has_local) continue; /* jamais lancé ou absent */
        key_3ds(j->title, j->key);
        snprintf(j->name, sizeof j->name, "%s", titles[i].name);
        snprintf(j->short_name, sizeof j->short_name, "%.47s", titles[i].name);
        game_count++;
    }
}

/* Toutes les sources, puis le Hash de chaque Sauvegarde locale (SRAM seule pour GB/GBA, ADR 0008). */
static void scan(void)
{
    int i, before;

    game_count = visible_count = loaded = skipped_roms = alias_count = 0; /* la liste affichée n'est plus valable pendant la lecture */
    read_ini(APP_DIR "/alias.ini", alias_entry);
    scan_dir(SYS_DS, cfg.roms, cfg.saves, cfg.roms, 0); /* sauvegardes vides = à côté de la ROM */
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

/* GET /saves : Version courante de chaque Jeu. 0 si tout va bien. */
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
        add_alert(t); /* ajouté au message de l'action en cours (show_alert) */
        network_detail(status, url, alert_detail, sizeof alert_detail);
        if (status > 0) free(body);
        return -1;
    }
    server_read = 1;
    read_time = osGetTime();
    /* [{"code": "IRBF", "version": "...", "hash": "...", "taille": 524288, "date": "...", "client": "pc"}, ...] (list.h) */
    for (p = body; next_server_entry(&p, &e);) {
        for (i = 0; i < game_count && strcmp(games[i].key, e.code); i++);
        if (i == game_count || !e.hash[0] || e.size < 0) continue; /* pas de ROM locale : pas affiché */
        strcpy(games[i].server_hash, e.hash);
        games[i].server.size = e.size;
        strcpy(games[i].date, e.date);
        strcpy(games[i].client, e.client);
        games[i].age = e.age;
        games[i].has_server = 1;
        games[i].has_name = e.has_name;
    }
    free(body);
    /* Nom de chaque jeu pour l'Historique du PC (ADR 0018), une fois par jeu. Arrêt au premier refus : un serveur plus
     * ancien ne connaît pas cette adresse, et la liste reste lisible sans les noms. */
    for (i = 0; i < game_count; i++) {
        if (!games[i].has_server || games[i].has_name) continue;
        show_progress(i + 1, game_count, T("Envoi des noms des jeux\n%s", "Sending game names\n%s"), games[i].short_name);
        snprintf(url, sizeof url, "%s/saves/%.31s/name", cfg.url, games[i].key); /* Clé : 31 caractères au plus (KEY_MAX) */
        if (net_put(url, cfg.token, NULL, games[i].name, strlen(games[i].name)) != 204) break;
        games[i].has_name = 1;
    }
    return 0;
}

/* Ordre d'affichage : ce qui demande une décision d'abord, puis les actions, puis le reste. */
static int priority(int e)
{
    switch (e) {
    case CONFLICT: return 0;
    case TO_DOWNLOAD: return 1;
    case TO_UPLOAD: return 2;
    case DUPLICATE: return 3;
    }
    return 4;
}

static int compare_visible(const void *a, const void *b)
{
    const game_t *x = &games[*(const int *)a], *y = &games[*(const int *)b];
    int d = priority(x->state) - priority(y->state);
    return d ? d : strcasecmp(x->short_name, y->short_name);
}

static void compute_states(void)
{
    int i;
    visible_count = 0;
    for (i = 0; i < game_count; i++) {
        game_t *j = &games[i];
        /* Sauvegarde vierge = absente, sauf si le serveur a la même ; recuperer() la copie quand même. */
        int seen = j->has_local && (!j->blank || (j->has_server && !strcmp(j->local_hash, j->server_hash)));
        j->local.hash = j->local_hash;
        j->server.hash = j->server_hash;
        if (j->rom_count > 1) j->state = DUPLICATE;
        else if (!seen && !j->has_server) j->state = HIDDEN;
        else if (!server_read) j->state = OFFLINE;
        else j->state = sync_state(seen ? &j->local : NULL, j->has_server ? &j->server : NULL, base_of(j->key));
        if (j->state == IN_SYNC && (!base_of(j->key) || strcmp(base_of(j->key), j->local_hash)))
            set_base(j->key, j->local_hash);
        if (j->state != HIDDEN) visible[visible_count++] = i;
    }
    qsort(visible, visible_count, sizeof *visible, compare_visible);
}

/* Après une action : le Jeu touché est déjà relu (read_local), seul le serveur change. */
static int reread_server(void)
{
    int r;
    show_progress(0, 0, "%s", T("Lecture du serveur…", "Reading the server…"));
    r = read_server();
    compute_states(); /* serveur non lu : les jeux locaux restent listés, sans action */
    loaded = 1;
    return r;
}

/* Tout relire : ROM, Sauvegardes, serveur (Y, réglages modifiés). */
static int refresh(void)
{
    int r;
    scan();
    show_progress(0, 0, "%s", T("Lecture du serveur…", "Reading the server…"));
    r = read_server();
    compute_states();
    loaded = 1;
    return r;
}

/* ---------------------------------------------------------------- actions */

/* Envoi conditionnel : If-Match = Version courante vue par ce Client (ADR 0001). 1 si accepté.
 * GB/GBA : seule la SRAM part, le Trailer d'horloge reste sur la 3DS (ADR 0008). */
static int upload_save(game_t *j)
{
    char url[300], condition[100], hash[65];
    unsigned char *bytes;
    size_t size;
    long status;

    if (read_save(j, &bytes, &size)) {
        set_message(C_RED, NULL, T("%s : sauvegarde illisible. Vérifiez la carte SD.", "%s: can't read the save. Check the SD card."), j->short_name);
        return 0;
    }
    size = sram_size(j->system, size);
    to_server_form(j, &bytes, &size);
    hash_hex(bytes, size, hash);
    if (j->has_server) snprintf(condition, sizeof condition, "If-Match: \"%s\"", j->server_hash);
    else snprintf(condition, sizeof condition, "If-None-Match: *");
    snprintf(url, sizeof url, "%s/saves/%s", cfg.url, j->key);
    status = net_put(url, cfg.token, condition, bytes, size);
    free(bytes);
    if (status == 200) {
        set_base(j->key, hash);
        set_message(C_GREEN, NULL, T("%s : envoyé au serveur.", "%s: uploaded."), j->short_name);
    } else if (status == 412) {
        set_message(C_RED, NULL, T("%s : conflit, le serveur a changé entre-temps.", "%s: conflict, the server changed meanwhile."), j->short_name);
    } else {
        char d[200];
        network_detail(status, url, d, sizeof d);
        set_message(C_RED, d, T("%s : envoi impossible. %s", "%s: upload failed. %s"), j->short_name, network_text(status, net_wifi(), english));
    }
    return status == 200;
}

/* Crée le dossier qui contient chemin s'il manque (dossier des sauvegardes d'un premier jeu). */
static void make_parent_dir(const char *path)
{
    char d[600], *end;
    snprintf(d, sizeof d, "%s", path);
    if ((end = strrchr(d, '/'))) {
        *end = '\0';
        mkdir(d, 0777);
    }
}

/* Récupération (design §4) : Copie de sécurité d'abord, puis
 *  - fichier (DS, GB, GBA) : SRAM reçue + Trailer d'horloge local (ADR 0008) dans un .tmp, puis remplacement ;
 *  - 3DS : réécriture de /main avec commit et secure value (ADR 0009).
 * 1 si faite ; en cas d'échec, la Sauvegarde locale est intacte. */
static int download_save(game_t *j)
{
    char tmp[610], backup[700], url[300], hash[65], date[20];
    char *body;
    unsigned char *old_data = NULL, *new_data;
    size_t size, n = 0, sram;
    long status;
    int failed, exists = j->system == SYS_3DS || access(j->sav, F_OK) == 0;
    time_t t = time(NULL);

    snprintf(url, sizeof url, "%s/saves/%s", cfg.url, j->key);
    status = net_get(url, cfg.token, &body, &size);
    if (status != 200) {
        char d[200];
        if (status > 0) free(body);
        network_detail(status, url, d, sizeof d);
        set_message(C_RED, d, T("%s : récupération impossible. %s La sauvegarde de la 3DS reste intacte.", "%s: download failed. %s The 3DS save is intact."),
             j->short_name, network_text(status, net_wifi(), english));
        return 0;
    }
    hash_hex((unsigned char *)body, size, hash);
    if (strcmp(hash, j->server_hash)) {
        free(body);
        set_message(C_RED, NULL, T("%s : version reçue abîmée, rien n'est écrit. Réessayez.", "%s: received version damaged, nothing written. Try again."), j->short_name);
        return 0;
    }
    /* Copie dès que la Sauvegarde existe, même si le scan n'a pas pu la lire : jamais d'écrasement sans copie. */
    if (exists) {
        strftime(date, sizeof date, "%Y%m%d-%H%M%S", localtime(&t));
        mkdir(APP_DIR "/copies", 0777);
        /* même nom de ROM dans deux sous-dossiers, la même seconde : -1, -2… (jamais d'écrasement d'une copie) */
        failed = backup_path(APP_DIR "/copies", j->name, date, j->system == SYS_3DS ? "main" : "sav", backup, sizeof backup);
        if (!failed) failed = read_save(j, &old_data, &n);
        if (!failed) failed = write_verified(backup, old_data, n); /* relue : une copie abîmée ne protège rien */
        if (failed) {
            free(old_data);
            free(body);
            set_message(C_RED, backup, T("%s : copie de sécurité impossible, rien n'est écrit. Vérifiez la carte SD.", "%s: backup copy failed, nothing written. Check the SD card."), j->short_name);
            return 0;
        }
    }

    if (j->system == SYS_3DS) {
        failed = write_title_save(j->title, j->media, body, size);
        free(old_data);
        free(body);
        if (failed) {
            set_message(C_RED, NULL, T("%s : écriture 3DS refusée, sauvegarde intacte.", "%s: 3DS write refused, save intact."), j->short_name);
            return 0;
        }
    } else {
        /* SRAM reçue (forme du fichier) + Trailer d'horloge local (ce qui dépasse la SRAM de l'ancien fichier). */
        unsigned char *received = (unsigned char *)body;
        sram = exists ? sram_size(j->system, n) : n;
        new_data = to_file_form(j, &received, &size, sram) ? NULL : malloc(size + (n - sram) + 1);
        if (!new_data) {
            free(old_data);
            free(received);
            set_message(C_RED, NULL, T("%s : mémoire insuffisante, rien n'est écrit.", "%s: out of memory, nothing written."), j->short_name);
            return 0;
        }
        memcpy(new_data, received, size);
        if (n > sram) memcpy(new_data + size, old_data + sram, n - sram);
        free(old_data);
        free(received);
        make_parent_dir(j->sav); /* dossier des sauvegardes absent (premier jeu) */
        /* .tmp écrit et relu, puis l'ancienne (déjà copiée) retirée, le .tmp renommé et relu (files.h). */
        failed = replace_file(j->sav, new_data, size + (n - sram));
        free(new_data);
        if (failed == -1) {
            set_message(C_RED, j->sav, T("%s : écriture impossible, sauvegarde de la 3DS intacte.", "%s: write failed, 3DS save intact."), j->short_name);
            return 0;
        }
        snprintf(tmp, sizeof tmp, "%s.tmp", j->sav);
        if (failed && access(tmp, F_OK)) { /* renommée, mais relue différente : pas de Base sur ce fichier */
            set_message(C_RED, j->sav, T("%s : sauvegarde écrite abîmée. Vérifiez la carte SD, puis récupérez de nouveau.%s", "%s: written save damaged. Check the SD card, then download again.%s"),
                 j->short_name, exists ? T(" Ancienne copiée dans /3ds/3dssavesync/copies.", " Old one copied to /3ds/3dssavesync/copies.") : "");
            return 0;
        }
        if (failed) {
            const char *b = strrchr(j->sav, '/') ? strrchr(j->sav, '/') + 1 : j->sav, *ext = strrchr(b, '.');
            int l = (int)strlen(b), cut = l > 60; /* écran du bas : nom raccourci, extension gardée */
            set_message(C_RED, NULL, T("%s : renommage impossible. Sauvegarde reçue : %.*s%s%s.tmp%s", "%s: rename failed. Downloaded save: %.*s%s%s.tmp%s"),
                     j->short_name, cut ? 50 : l, b, cut ? "…" : "", cut && ext ? ext : "",
                     exists ? T(". Ancienne copiée dans /3ds/3dssavesync/copies.", ". Old one copied to /3ds/3dssavesync/copies.") : ".");
            return 0;
        }
    }
    set_base(j->key, j->server_hash);
    set_message(C_GREEN, NULL, T("%s : récupéré du serveur.", "%s: downloaded."), j->short_name);
    return 1;
}

/* ---------------------------------------------------------------- écran */

/* Icône 32×32 d'un jeu DS, lue dans la ROM, en RGB565 ligne par ligne. 0 si lue.
 * Bannière NDS : son offset est à 0x68 de l'en-tête ; tuiles 4 bits à +0x20, palette BGR555 à +0x220. */
#define ICON_BACKGROUND 0x10C3 /* couleur 0 de la palette (transparente) : fond de l'écran du bas (0x14181E) */
static int read_ds_icon(const game_t *j, u16 *pixels)
{
    unsigned char o[4], icon[512 + 32];
    u32 offset;
    FILE *f;
    int t, l, k;

    if (j->system != SYS_DS || !(f = fopen(j->rom, "rb"))) return -1; /* seules les ROM DS ont une bannière */
    offset = fseek(f, 0x68, SEEK_SET) == 0 && fread(o, 1, 4, f) == 4 ? o[0] | o[1] << 8 | o[2] << 16 | (u32)o[3] << 24 : 0;
    if (!offset || fseek(f, offset + 0x20, SEEK_SET) || fread(icon, 1, sizeof icon, f) != sizeof icon) {
        fclose(f);
        return -1;
    }
    fclose(f);
    for (t = 0; t < 16; t++) /* 4×4 tuiles de 8×8 */
        for (l = 0; l < 8; l++)
            for (k = 0; k < 8; k++) {
                int idx = (icon[t * 32 + l * 4 + k / 2] >> ((k & 1) * 4)) & 0xF;
                u16 c = icon[512 + idx * 2] | icon[512 + idx * 2 + 1] << 8;
                pixels[((t / 4) * 8 + l) * 32 + (t % 4) * 8 + k] =
                    idx ? (c & 0x1F) << 11 | ((c >> 5) & 0x1F) << 6 | ((c >> 10) & 0x1F) : ICON_BACKGROUND;
            }
    return 0;
}

/* ---------------------------------------------------------------- comparer deux Versions */

/* « 152 h 30 · Sacha », ou partie non commencée. */
static void progress_text(const progress_t *p, char *out, size_t n)
{
    if (p->trainer[0]) snprintf(out, n, "%d h %02d \xC2\xB7 %s", p->hours, p->minutes, p->trainer);
    else snprintf(out, n, T("%d h %02d \xC2\xB7 partie non commencée", "%d h %02d \xC2\xB7 not started yet"), p->hours, p->minutes);
}

/* Sauvegarde de la 3DS : temps de jeu (Pokémon), sinon date du fichier si l'horloge la rend crédible. */
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

/* Version du serveur : quand, puis depuis quel appareil. */
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

/* Temps de jeu de la version du serveur (téléchargée), seulement si la 3DS a un Pokémon reconnu. 1 si lu. */
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

/* ---------------------------------------------------------------- écran principal (citro2d, issue #21) */

static u32 state_color(int e)
{
    switch (e) {
    case TO_UPLOAD: return C_YELLOW;
    case TO_DOWNLOAD: return C_CYAN;
    case CONFLICT: return C_RED;
    }
    return GREY_LIGHT; /* À jour, Doublon, Inconnu */
}

/* Pastille et trait de l'état : vert sombre pour À jour, qui n'attire pas l'œil. */
static u32 dot_color(int e) { return e == IN_SYNC ? RGB(0x2F6B3A) : state_color(e); }

/* Un mot par état, le même dans la liste, les compteurs et les détails. */
static const char *state_label(int e)
{
    switch (e) {
    case TO_UPLOAD: return T("À envoyer", "To upload");
    case TO_DOWNLOAD: return T("À récupérer", "To download");
    case CONFLICT: return T("Conflit", "Conflict");
    case DUPLICATE: return T("Doublon", "Duplicate");
    case OFFLINE: return T("Inconnu", "Unknown");
    }
    return T("À jour", "In sync");
}

/* Barre des touches en bas de l'écran du bas (maquette 3 de l'issue #21) : chaque touche est aussi touchable au stylet. */
typedef struct {
    u32 button;
    const char *text;
    int actif; /* 0 : grisée (A sans rien à faire) */
} button_t;
static struct { float x, y, l, h; u32 button; } buttons[8];
static int button_count;
#define BUTTON_HEIGHT 22
#define BAR_Y (240 - 2 * BUTTON_HEIGHT - 4)
#define BAR_TOP_1 (240 - BUTTON_HEIGHT - 7) /* haut du fond d'une barre d'une ligne de touches */
#define BAR_TOP_2 (240 - 2 * BUTTON_HEIGHT - 7)

/* Glyphe de la police du système (U+E000 A, E001 B, E002 X, E003 Y) à la couleur de la New 3DS, ou START / SELECT. */
static const char *glyph(u32 t)
{
    return t == KEY_A ? "\xEE\x80\x80" : t == KEY_B ? "\xEE\x80\x81" : t == KEY_X ? "\xEE\x80\x82" : t == KEY_Y ? "\xEE\x80\x83" : t == KEY_START ? "START" : "SELECT";
}

/* START et SELECT : le mot dans une pastille, 6 pixels de plus. */
static float glyph_width(u32 t)
{
    return t & (KEY_START | KEY_SELECT) ? screen_text_width(0.34f, glyph(t)) + 6 : screen_text_width(0.55f, glyph(t));
}

static float button_width(const button_t *t, float size) { return glyph_width(t->button) + 4 + screen_text_width(size, t->text); }

/* Une ligne de touches à partir de y ; ecart = 0 : touches réparties sur toute la largeur.
 * Texte réduit si la ligne ne tient pas (marge comprise) avec au moins 10 pixels entre deux touches. */
static void draw_button_row(const button_t *t, int n, float y, float gap)
{
    float l[6], total, x = 6, size = 0.44f;
    int i;
    do {
        size -= 0.02f;
        for (total = 0, i = 0; i < n; i++) total += l[i] = button_width(&t[i], size);
    } while (size > 0.34f && total * WIDTH_MARGIN + 10 * (n - 1) > 308);
    if (!gap) gap = n > 1 ? (308 - total) / (n - 1) : 0;
    for (i = 0; i < n; i++) {
        u32 c = !t[i].actif ? GREY_DARK : t[i].button == KEY_A ? RGB(0xE04848) : t[i].button == KEY_B ? RGB(0xF0C030)
              : t[i].button == KEY_X ? RGB(0x4080F0) : t[i].button == KEY_Y ? RGB(0x40B060) : GREY_LIGHT;
        float g = glyph_width(t[i].button);
        if (t[i].button & (KEY_START | KEY_SELECT)) { /* lu comme une touche, pas comme du texte */
            screen_rect(x, y + 4, g, 12, GREY_DARK);
            screen_text(x + 3, y + 4, 0.34f, WHITE, 0, glyph(t[i].button));
        } else screen_text(x, y + 2, 0.55f, c, 0, glyph(t[i].button));
        screen_text(x + g + 4, y + 3, size, t[i].actif ? WHITE : GREY_DARK, 0, t[i].text);
        if (button_count < 8) { /* zone touchable : la touche et la moitié de l'écart de chaque côté */
            buttons[button_count].x = x - gap / 2;
            buttons[button_count].y = y - 2;
            buttons[button_count].l = l[i] + gap;
            buttons[button_count].h = BUTTON_HEIGHT;
            buttons[button_count++].button = t[i].button;
        }
        x += l[i] + gap;
    }
}

/* Carte de choix de l'écran de Conflit (152 × 92), touchable : touche et action, la version (même forme des deux
 * côtés), ce qui se passe. */
static void draw_choice_card(float x, float y, u32 button, const char *action, const char *version, const char *outcome)
{
    screen_rect(x, y, 152, 92, CARD);
    screen_text(x + 6, y + 4, 0.55f, button == KEY_A ? RGB(0xE04848) : RGB(0x4080F0), 0, glyph(button));
    screen_text(x + 24, y + 5, 0.45f, WHITE, 122, action);
    screen_lines(x + 6, y + 26, 0.42f, WHITE, 140, 3, version);
    screen_lines(x + 6, y + 68, 0.38f, GREY_LIGHT, 140, 2, outcome);
    if (button_count < 8) {
        buttons[button_count].x = x;
        buttons[button_count].y = y;
        buttons[button_count].l = 152;
        buttons[button_count].h = 92;
        buttons[button_count++].button = button;
    }
}

/* Touche du bouton sous le stylet, 0 si aucun. */
static u32 touched_button(void)
{
    touchPosition p;
    int i;
    hidTouchRead(&p);
    for (i = 0; i < button_count; i++)
        if (p.px >= buttons[i].x && p.px < buttons[i].x + buttons[i].l && p.py >= buttons[i].y && p.py < buttons[i].y + buttons[i].h)
            return buttons[i].button;
    return 0;
}

/* Fond de la barre des touches : 1 ou 2 lignes, en bas de l'écran du bas. */
static void draw_bar_background(int lines)
{
    float y = 240 - lines * BUTTON_HEIGHT - 7;
    screen_rect(0, y, 320, 240 - y, BAR);
}

/* Icône du jeu en haut à droite de l'écran du bas. */
static void draw_game_icon(const game_t *j)
{
    u16 icon[48 * 48];
    if (j->system == SYS_3DS ? title_icon(j->title, j->media, icon) == 0 : read_ds_icon(j, icon) == 0)
        screen_icon(icon, j->system == SYS_3DS ? 48 : 32, j->system == SYS_3DS ? 48 : 32, 320 - 6 - 48, 6, j->system == SYS_3DS ? 1.0f : 1.5f);
}

/* Écran du haut de la liste : titre et compteurs (ceux qui ne tiennent pas partent, « à jour » d'abord), les jeux, puis
 * le pied : le serveur, quand il a été lu, la position dans la liste. */
static void draw_list_top(void)
{
    static const int states[] = { CONFLICT, TO_DOWNLOAD, TO_UPLOAD, DUPLICATE, IN_SYNC }; /* rang de priorite() */
    static const int all[] = { TO_UPLOAD, TO_DOWNLOAD, CONFLICT, DUPLICATE, OFFLINE, IN_SYNC };
    const int cursor = list_cursor;
    int i, count = 0, e[5], n[5] = { 0 }, start = cursor < TOP_LINES ? 0 : cursor - TOP_LINES + 1;
    char t[5][40], footer[200], age[32], pos[24];
    const char *host = strstr(cfg.url, "://") ? strstr(cfg.url, "://") + 3 : cfg.url;
    float l[5], x, xs = 0, xn;

    screen_top();
    x = 6 + screen_text(6, 4, 0.55f, WHITE, 0, "3dssavesync") + 12;
    if (loaded && server_read) { /* sans le serveur, les compteurs seraient faux */
        for (i = 0; i < visible_count; i++) n[priority(games[visible[i]].state)]++;
        for (i = 0; i < 5; i++) {
            if (!n[i]) continue;
            e[count] = states[i];
            snprintf(t[count], sizeof t[count], e[count] == CONFLICT ? plural(n[i], english, T("%d conflit", "%d conflict"), T("%d conflits", "%d conflicts"))
                                          : e[count] == TO_DOWNLOAD ? T("%d à récupérer", "%d to download")
                                          : e[count] == TO_UPLOAD ? T("%d à envoyer", "%d to upload")
                                          : e[count] == DUPLICATE ? plural(n[i], english, T("%d doublon", "%d duplicate"), T("%d doublons", "%d duplicates"))
                                                             : T("%d à jour", "%d in sync"), n[i]);
            l[count] = screen_text_width(0.42f, t[count]);
            count++;
        }
        count = texts_fitting(l, count, 12, 400 - 6 - x);
        for (x = 400 - 6, i = count - 1; i >= 0; i--) {
            x -= l[i];
            screen_text(x, 7, 0.42f, e[i] == IN_SYNC ? C_GREEN : state_color(e[i]), 0, t[i]);
            x -= 12;
        }
    }
    screen_rect(0, 23, 400, 1, GREY_DARK);

    /* Colonnes mesurées sur tous les mots d'état : rien n'est coupé, et elles ne bougent pas d'une image à l'autre. */
    for (i = 0; i < 6; i++)
        if (screen_text_width(0.42f, state_label(all[i])) > xs) xs = screen_text_width(0.42f, state_label(all[i]));
    xs = 18 + xs * WIDTH_MARGIN + 6;
    xn = xs + screen_text_width(0.42f, "GBA") * WIDTH_MARGIN + 8;
    if (!visible_count) screen_paragraph(10, 40, 0.5f, GREY_LIGHT, 380, !loaded ? T("Lecture en cours…", "Reading…")
                                                              : server_read ? T("Aucun jeu à afficher.", "No game to show.")
                                                              : T("Serveur non lu, et aucune sauvegarde sur la 3DS.", "Server not read, and no save on the 3DS."));
    for (i = start; i < visible_count && i < start + TOP_LINES; i++) {
        game_t *k = &games[visible[i]];
        float y = 27 + (i - start) * 15;
        if (i == cursor) {
            screen_rect(0, y - 1, 400, 15, SEL);
            screen_rect(0, y - 1, 2, 15, dot_color(k->state));
        }
        screen_rect(8, y + 5, 5, 5, dot_color(k->state));
        screen_text(18, y + 1, 0.42f, state_color(k->state), 0, state_label(k->state));
        screen_text(xs, y + 1, 0.42f, GREY_LIGHT, 0, SYSTEM_NAMES[k->system]);
        screen_text(xn, y, 0.5f, WHITE, 400 - xn - 8, k->short_name);
    }
    if (visible_count > TOP_LINES) { /* barre de défilement */
        float h = 195.0f * TOP_LINES / visible_count;
        screen_rect(396, 26 + (195.0f - h) * start / (visible_count - TOP_LINES), 2, h, GREY_DARK);
    }

    screen_rect(0, 224, 400, 16, BAR);
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
    x = 400 - 6 - screen_text_width(0.38f, pos);
    screen_text(x, 226, 0.38f, GREY_LIGHT, 0, pos);
    screen_text(8, 226, 0.38f, loaded && !server_read ? C_RED : GREY_LIGHT, x - 8 - 12, footer);
}

/* Une ligne « libellé  valeur » des détails. */
static float detail(float y, const char *label, const char *value)
{
    screen_text(8, y, 0.42f, GREY_LIGHT, 100, label);
    screen_text(112, y, 0.42f, WHITE, 200, value);
    return y + 15;
}

/* Sous le mot d'état : pourquoi, et ce que fait A. */
static const char *state_sentence(const game_t *j, char *t, size_t n)
{
    switch (j->state) {
    case TO_UPLOAD: return T("La sauvegarde de la 3DS est plus récente. \xEE\x80\x80 l'envoie au serveur.", "The 3DS save is newer. \xEE\x80\x80 uploads it to the server.");
    case TO_DOWNLOAD: return T("Le serveur a une version plus récente. \xEE\x80\x80 la récupère, après une copie de sécurité.",
                               "The server has a newer version. \xEE\x80\x80 downloads it, after a backup.");
    case CONFLICT: return T("La sauvegarde a changé sur la 3DS et sur le serveur. \xEE\x80\x80 pour choisir laquelle garder.",
                           "The save changed on the 3DS and on the server. \xEE\x80\x80 to choose which one to keep.");
    case DUPLICATE:
        if (j->shares_save) snprintf(t, n, T("%d ROM ont ce nom de fichier et partageraient la même sauvegarde. Renommez-en une.",
                                         "%d ROMs have this file name and would share one save. Rename one."), j->rom_count);
        else snprintf(t, n, T("%d ROM contiennent ce jeu. Gardez-en une, ou ajoutez les autres aux ROM hacks sur le PC, puis préparez la carte SD.",
                              "%d ROMs hold this game. Keep one, or add the others to ROM hacks on the PC, then prepare the SD card."), j->rom_count);
        return t;
    case OFFLINE: return T("Serveur non lu : état inconnu. \xEE\x80\x83 relit le serveur.", "Server not read: state unknown. \xEE\x80\x83 reads the server again.");
    }
    return T("La 3DS et le serveur ont la même sauvegarde.", "The 3DS and the server have the same save.");
}

/* Bande d'un message, collée au-dessus de la barre des touches (bas) : trait de couleur, texte, puis la ligne technique
 * en gris, toutes deux sur plusieurs lignes. Elle monte aussi haut que le texte le demande, par-dessus les détails :
 * le message reste entier (un peu plus petit s'il le faut ; « … » seulement s'il dépasse encore l'écran). */
static void draw_band(float bottom, const char *text, const char *technical, u32 color)
{
    float size, l1, l2 = technical && technical[0] ? screen_paragraph_height(0.38f, 302, technical) : 0, h;
    int lines, room;
    if (!text[0]) return;
    for (size = 0.42f;; size = 0.36f) {
        l1 = screen_paragraph_height(size, 302, "X");
        lines = (int)(screen_paragraph_height(size, 302, text) / l1 + 0.5f);
        room = (int)((bottom - 12 - l2) / l1); /* jusqu'à 6 px du haut de l'écran */
        if (lines <= room || size < 0.4f) break;
    }
    if (lines > room) lines = room < 1 ? 1 : room;
    h = lines * l1 + l2 + 6;
    screen_rect(0, bottom - h, 320, h, CARD);
    screen_rect(0, bottom - h, 3, h, color);
    screen_lines(10, bottom - h + 3, size, WHITE, 302, lines, text);
    if (l2) screen_paragraph(10, bottom - h + 3 + lines * l1, 0.38f, GREY_LIGHT, 302, technical);
}

/* Message de la dernière action, dans sa bande. */
static void draw_message(float bottom) { draw_band(bottom, message, message_detail, message_color); }

static void draw_bottom(const game_t *j)
{
    char t[600];
    float y = 6;
    int action = 0, gb = 0;

    screen_bottom();
    button_count = 0;
    if (j) {
        const char *b = base_of(j->key), *ext = strrchr(j->rom, '.');
        u32 c = j->state == IN_SYNC ? C_GREEN : state_color(j->state);
        gb = j->system == SYS_GB || j->system == SYS_GBA;
        action = j->state == TO_UPLOAD || j->state == TO_DOWNLOAD || j->state == CONFLICT;
        draw_game_icon(j);
        y += screen_lines(8, y, 0.6f, WHITE, 250, 2, j->short_name);
        /* Système, et pour GB/GBA le programme dont la sauvegarde est synchronisée (X le change). */
        snprintf(t, sizeof t, "%s%s%s", j->system == SYS_DS ? "Nintendo DS" : j->system == SYS_GB ? (ext && !strcasecmp(ext, ".gbc") ? "Game Boy Color" : "Game Boy")
                                       : j->system == SYS_GBA ? "Game Boy Advance" : "Nintendo 3DS",
                 gb ? " \xC2\xB7 " : "", !gb ? "" : forwarder(j) ? T("forwarder mGBA", "mGBA forwarder") : j->system == SYS_GBA ? "open_agb_firm" : "TWiLight Menu++");
        screen_text(8, y + 1, 0.4f, GREY_LIGHT, 250, t);
        y = y + 18 < 58 ? 58 : y + 20; /* sous l'icône */
        screen_rect(8, y + 2, 3, 13, c);
        screen_text(15, y, 0.5f, c, 0, state_label(j->state));
        y += 19;
        y += screen_lines(8, y, 0.42f, WHITE, 304, j->state == DUPLICATE ? 4 : 2, state_sentence(j, t, sizeof t)) + 5;
        if (j->state != DUPLICATE) { /* un Doublon n'a ni sauvegarde ni version à montrer */
            local_text(j, t, sizeof t);
            y = detail(y, T("Sur la 3DS", "On the 3DS"), t);
            server_text(j, t, sizeof t);
            y = detail(y, T("Sur le serveur", "On the server"), t);
            y = detail(y, T("Dernière synchro", "Last sync"),
                       !b ? T("Jamais", "Never")
                       : !strcmp(b, j->local_hash) && !strcmp(b, j->server_hash) ? T("Même version que les deux", "Same as both")
                       : !strcmp(b, j->local_hash) ? T("Même version que la 3DS", "Same as the 3DS")
                       : !strcmp(b, j->server_hash) ? T("Même version que le serveur", "Same as the server") : T("Version différente des deux", "Differs from both"));
        }
    }
    if (message[0]) draw_message(BAR_TOP_2);
    else if (update_is_newer(VERSION, latest_version)) { /* sans message : la mise à jour, à la même place */
        snprintf(t, sizeof t, T("Version %s disponible (SELECT)", "Version %s available (SELECT)"), latest_version);
        draw_band(BAR_TOP_2, t, NULL, C_CYAN);
    }

    /* Barre : en haut, ce que font A (toujours affichée, grisée s'il n'y a rien à faire) et X (GB/GBA) pour ce jeu ;
     * en bas, les touches générales. */
    draw_bar_background(2);
    {
        char x[80];
        button_t game_buttons[2] = { { KEY_A, !j ? T("Aucun jeu", "No game") : j->state == TO_UPLOAD ? T("Envoyer au serveur", "Upload to the server")
                                     : j->state == TO_DOWNLOAD ? T("Récupérer du serveur", "Download from the server")
                                     : j->state == CONFLICT ? T("Résoudre le conflit", "Resolve the conflict") : T("Rien à faire", "Nothing to do"), action },
                            { KEY_X, x, 1 } };
        const button_t general[4] = { { KEY_Y, T("Rafraîchir", "Refresh"), 1 }, { KEY_START, T("Tout synchro", "Sync all"), 1 },
                                        { KEY_SELECT, T("Réglages", "Settings"), 1 }, { KEY_B, T("Quitter", "Quit"), 1 } };
        if (gb) snprintf(x, sizeof x, T("Jouer avec %s", "Play with %s"), forwarder(j) ? (j->system == SYS_GBA ? "open_agb_firm" : "TWiLight Menu++")
                                                                                      : T("le forwarder mGBA", "the mGBA forwarder"));
        draw_button_row(game_buttons, gb ? 2 : 1, BAR_Y, 18);
        draw_button_row(general, 4, BAR_Y + BUTTON_HEIGHT, 0);
    }
}

static void draw(void)
{
    screen_begin();
    draw_list_top();
    draw_bottom(visible_count ? &games[visible[list_cursor]] : NULL);
    screen_end();
}

/* Touches appuyées (avec répétition pour les flèches), ou touche de la barre sous le stylet. B si l'application se ferme. */
static u32 read_keys(void)
{
    u32 k = 0;
    while (!k && aptMainLoop()) {
        gspWaitForVBlank();
        hidScanInput();
        k = hidKeysDownRepeat() & ~KEY_TOUCH;
        if (hidKeysDown() & KEY_TOUCH) k |= touched_button();
    }
    return k ? k : KEY_B;
}

/* Progression des longues boucles : l'écran du haut garde son contexte (draw_current_top) ; en bas, l'opération, le texte
 * (« étape\njeu » : l'étape en gris, puis le jeu) et une barre si total > 0.
 * Au plus 20 images par seconde : une image attend l'écran, la lecture ne doit pas en être ralentie. */
static void show_progress(int done, int total, const char *format, ...)
{
    static u64 last_frame;
    char t[300], *nl;
    float y = 40;
    va_list a;
    u64 now = osGetTime();
    if (total && done > 1 && done < total && now - last_frame < 50) return;
    last_frame = now;
    va_start(a, format);
    vsnprintf(t, sizeof t, format, a);
    va_end(a);
    screen_begin();
    draw_current_top();
    screen_bottom();
    if (operation) {
        screen_text(16, y, 0.55f, WHITE, 288, operation);
        y += 22;
    }
    if ((nl = strchr(t, '\n'))) {
        *nl = '\0';
        screen_text(16, y, 0.45f, GREY_LIGHT, 288, t);
        screen_lines(16, y + 18, 0.55f, WHITE, 288, 2, nl + 1);
    } else screen_lines(16, y, 0.55f, WHITE, 288, 3, t);
    if (total > 0) {
        char n[32];
        screen_rect(16, 140, 288, 10, CARD);
        screen_rect(16, 140, 288.0f * done / total, 10, progress_color);
        snprintf(n, sizeof n, "%d / %d", done, total);
        screen_text(16, 156, 0.45f, GREY_LIGHT, 0, n);
        snprintf(n, sizeof n, T("%d %%", "%d%%"), (int)(100LL * done / total));
        screen_text(304 - screen_text_width(0.45f, n), 156, 0.45f, GREY_LIGHT, 0, n);
    }
    screen_end();
}

/* Question sur l'écran du bas (l'écran du haut garde son contexte) : le titre est la question, le sous-titre le jeu ;
 * dans le texte, une ligne « libellé\tvaleur » se range dans la grille des détails ; l'avertissement va dans une bande
 * rouge. 1 si A, l'action nommée ; 0 si B, Annuler. */
static int confirm(const char *title, const char *subtitle, const char *text, const char *warning, const char *action)
{
    const button_t t[2] = { { KEY_A, action, 1 }, { KEY_B, T("Annuler", "Cancel"), 1 } };
    for (;;) {
        char line[300], *tab;
        const char *s = text;
        float y = 6;
        u32 k;
        screen_begin();
        draw_current_top();
        screen_bottom();
        button_count = 0;
        y += screen_lines(8, y, 0.55f, WHITE, 304, 2, title) + 2;
        if (subtitle) y += screen_lines(8, y, 0.45f, GREY_LIGHT, 304, 1, subtitle);
        y += 8;
        while (*s) {
            size_t n = strcspn(s, "\n");
            snprintf(line, sizeof line, "%.*s", (int)n, s);
            if ((tab = strchr(line, '\t'))) {
                *tab = '\0';
                y = detail(y, line, tab + 1);
            } else y += line[0] ? screen_paragraph(8, y, 0.45f, WHITE, 304, line) : 6;
            s += n + (s[n] == '\n');
        }
        if (warning) draw_band(BAR_TOP_1, warning, NULL, C_RED);
        draw_bar_background(1);
        draw_button_row(t, 2, 240 - BUTTON_HEIGHT - 4, 18);
        screen_end();
        k = read_keys();
        if (k & KEY_A) return 1;
        if (k & KEY_B) return 0;
    }
}

static void edit_text(const char *title, char *value, size_t n, int masked)
{
    SwkbdState s;
    char input[256];
    swkbdInit(&s, SWKBD_TYPE_WESTERN, 2, (int)n - 1);
    swkbdSetHintText(&s, title);
    swkbdSetInitialText(&s, value);
    swkbdSetValidation(&s, SWKBD_ANYTHING, 0, 0); /* vide permis : « à côté de la ROM » */
    if (masked) swkbdSetPasswordMode(&s, SWKBD_PASSWORD_HIDE_DELAY);
    if (swkbdInputText(&s, input, sizeof input) == SWKBD_BUTTON_CONFIRM) snprintf(value, n, "%s", input);
}

static int test_state;        /* ligne Connexion des réglages : 0 non testée, 1 réussie, -1 échec (test_short) */
static char test_short[80];  /* « Jeton refusé » : network_text jusqu'au premier « : », « . » ou « ( » */

/* GET /saves : résultat dans le message (texte, puis la ligne technique) et la ligne Connexion. 1 si la connexion marche. */
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

/* ---------------------------------------------------------------- mise à jour (ADR 0021) */

static void update_entry(const char *k, const char *v)
{
    if (!strcmp(k, "jour")) last_check_day = strtol(v, NULL, 10);
    else if (!strcmp(k, "version")) snprintf(latest_version, sizeof latest_version, "%s", v);
    else if (!strcmp(k, "installee")) snprintf(installed_version, sizeof installed_version, "%s", v);
}

/* maj.ini : jour de la vérification, dernière version vue, version en marche. Échec : réécrit au prochain lancement. */
static void write_update_ini(void)
{
    char t[120];
    snprintf(t, sizeof t, "jour=%ld\nversion=%s\ninstallee=%s\n", last_check_day, latest_version, VERSION);
    mkdir(APP_DIR, 0777);
    replace_file(APP_DIR "/maj.ini", t, strlen(t));
}

/* Échec d'une requête à GitHub : ce qui n'est pas fait, le texte de github_text, puis le code curl ou HTTP. */
static void github_failure(const char *what, long status)
{
    update_failed = 1;
    snprintf(update_detail, sizeof update_detail, "%s%s%s\n%s %ld", what, what[0] ? " " : "", github_text(status, net_wifi(), english),
             status < 0 ? "curl" : "HTTP", status < 0 ? -status : status);
}

/* GET /releases/latest : r, latest_version, update_detail et maj.ini à jour. 1 si une version plus récente existe. */
static int check_update(release_t *r)
{
    char *body;
    size_t size;
    long status;
    int ok;

    show_progress(0, 0, "%s", T("Recherche d'une mise à jour…", "Looking for an update…"));
    status = net_github(GITHUB_LATEST, NULL, &body, &size);
    ok = status == 200 && parse_release(body, r) == 0;
    if (status > 0) free(body);
    if (ok) snprintf(latest_version, sizeof latest_version, "%s", r->version);
    last_check_day = (long)(time(NULL) / 86400);
    write_update_ini(); /* échec : vérifié de nouveau au prochain lancement */
    if (!ok) {
        github_failure("", status);
        return 0;
    }
    update_failed = 0;
    if (update_is_newer(VERSION, r->version)) {
        snprintf(update_detail, sizeof update_detail, T("Version %s disponible.", "Version %s available."), r->version);
        return 1;
    }
    /* Jamais la version de GitHub quand elle n'est pas plus récente : elle ne dit rien d'utile et ressemble à un retard. */
    snprintf(update_detail, sizeof update_detail, "%s", !strcmp(VERSION, "dev") ? T("Version de développement : aucune mise à jour proposée.", "Development build: no update offered.")
                                                                           : T("3dssavesync est à jour.", "3dssavesync is up to date."));
    return 0;
}

static char download_version[32]; /* version en cours de téléchargement ou d'installation */

static void download_progress(size_t done, size_t total)
{
    show_progress((int)(done / 1024), (int)(total / 1024), T("Téléchargement de la version %s (Ko)", "Downloading version %s (KB)"), download_version);
}

/* .cia par AM, comme FBI : sur tout échec, l'installation est annulée et l'ancienne version reste. */
static Result install_cia(const char *bytes, size_t size)
{
    Handle h;
    Result r;
    u32 written = 0;
    size_t done;

    if (R_FAILED(r = amInit())) return r;
    if (R_SUCCEEDED(r = AM_StartCiaInstall(MEDIATYPE_SD, &h))) {
        for (done = 0; done < size && R_SUCCEEDED(r); done += written) {
            show_progress((int)(done / 1024), (int)(size / 1024), T("Installation de la version %s (Ko)", "Installing version %s (KB)"), download_version);
            r = FSFILE_Write(h, &written, done, bytes + done, size - done < 0x20000 ? size - done : 0x20000, 0);
            if (R_SUCCEEDED(r) && !written) r = MAKERESULT(RL_PERMANENT, RS_INTERNAL, RM_APPLICATION, RD_NO_DATA);
        }
        if (R_SUCCEEDED(r)) r = AM_FinishCiaInstall(h);
        else AM_CancelCIAInstall(h);
    }
    amExit();
    return r;
}

/* Télécharge r, vérifie son SHA-256 dans SHA256SUMS, puis l'installe : .cia par AM, ou .3dsx remplacé s'il a été lancé
 * depuis le Homebrew Launcher (3dslink, argv vide : refusé, le fichier en marche est inconnu). Échec : update_detail, et l'ancienne version reste. Réussite : quitter. Les Sauvegardes ne
 * sont jamais touchées. */
static void install_update(const release_t *r)
{
    const int dsx = envIsHomebrew();
    const char *name = dsx ? "3dssavesync.3dsx" : "3dssavesync.cia", *url = dsx ? r->dsx : r->cia;
    char *checksums = NULL, *bytes = NULL, expected[65], actual[65];
    size_t size, n;
    long status;
    FS_ArchiveResource sd;
    Result res;

    update_failed = 1;
    snprintf(download_version, sizeof download_version, "%s", r->version);
    if (dsx && !launched_from_sd(path_3dsx)) {
        snprintf(update_detail, sizeof update_detail, T("Mise à jour impossible ici : lancez 3dssavesync depuis le Homebrew Launcher, ou installez le .cia.\nargv[0] : %.200s",
                                                  "Update not possible here: start 3dssavesync from the Homebrew Launcher, or install the .cia.\nargv[0]: %.200s"),
                 path_3dsx[0] ? path_3dsx : "-");
        return;
    }
    if (!url[0] || !r->checksums[0]) {
        snprintf(update_detail, sizeof update_detail, T("Version %s incomplète sur GitHub. Installez-la à la main (voir le guide 3DS).\nabsent : %s",
                                                  "Version %s is incomplete on GitHub. Install it by hand (see the 3DS guide).\nmissing: %s"),
                 r->version, url[0] ? "SHA256SUMS" : name);
        return;
    }
    status = net_github(r->checksums, download_progress, &checksums, &n);
    if (status != 200) {
        github_failure(T("Impossible de télécharger SHA256SUMS.", "Couldn't download SHA256SUMS."), status); /* « Réessayez » : dans github_text */
        goto end;
    }
    if (find_checksum(checksums, name, expected)) {
        snprintf(update_detail, sizeof update_detail, T("SHA256SUMS ne contient pas %s. Installez la version %s à la main (voir le guide 3DS).\nSHA256SUMS : %u octets",
                                                  "SHA256SUMS has no %s. Install version %s by hand (see the 3DS guide).\nSHA256SUMS: %u bytes"),
                 name, r->version, (unsigned)n);
        goto end;
    }
    status = net_github(url, download_progress, &bytes, &size);
    if (status != 200) {
        github_failure(T("Le téléchargement a échoué.", "Download failed."), status);
        goto end;
    }
    show_progress(0, 0, "%s", T("Vérification du fichier…", "Checking the file…"));
    hash_hex((unsigned char *)bytes, size, actual);
    if (strcmp(actual, expected)) {
        snprintf(update_detail, sizeof update_detail, T("Le fichier téléchargé est abîmé. Réessayez.\nSHA-256 %.12s…, attendu %.12s…",
                                                  "The downloaded file is damaged. Try again.\nSHA-256 %.12s…, expected %.12s…"),
                 actual, expected);
        goto end;
    }
    /* .cia : place du titre installé ; .3dsx : le .tmp à côté de l'ancien (renommé .bak). Le double de la taille suffit dans les deux cas. */
    if (R_SUCCEEDED(FSUSER_GetSdmcArchiveResource(&sd)) && (u64)sd.freeClusters * sd.clusterSize < 2 * (u64)size) {
        u64 free_bytes = (u64)sd.freeClusters * sd.clusterSize;
        snprintf(update_detail, sizeof update_detail, T("Pas assez de place sur la carte SD. Libérez %llu Mo et réessayez.\nlibre : %llu Ko, requis : %llu Ko",
                                                  "Not enough space on the SD card. Free %llu MB and try again.\nfree: %llu KB, needed: %llu KB"),
                 (2 * (u64)size - free_bytes) / 1048576 + 1, free_bytes / 1024, 2 * (u64)size / 1024);
        goto end;
    }
    if (dsx) {
        int e;
        show_progress(0, 0, T("Installation de la version %s…", "Installing version %s…"), download_version);
        if ((e = replace_program(path_3dsx, bytes, size))) {
            if (e == -1) snprintf(update_detail, sizeof update_detail, T("Impossible d'écrire sur la carte SD, l'ancienne version reste. Vérifiez la carte et réessayez.\n%s",
                                                                   "Can't write to the SD card, the old version stays. Check the card and try again.\n%s"), path_3dsx);
            else snprintf(update_detail, sizeof update_detail, T("Aucune version en place. Sur un PC, renommez %s.bak en %s pour retrouver l'ancienne.",
                                                           "No version in place. On a PC, rename %s.bak to %s to get the old one back."), path_3dsx, strrchr(path_3dsx, '/') + 1);
            goto end;
        }
    } else if (R_FAILED(res = install_cia(bytes, size))) {
        snprintf(update_detail, sizeof update_detail, T("La 3DS a refusé l'installation, l'ancienne version reste. Redémarrez la 3DS et réessayez.\nAM 0x%08lX",
                                                  "The 3DS refused the install, the old version stays. Restart the 3DS and try again.\nAM 0x%08lX"), res);
        goto end;
    }
    update_failed = 0;
    should_quit = 1;
    {
        const button_t b = { KEY_A, T("Fermer 3dssavesync", "Close 3dssavesync"), 1 };
        char title[80];
        snprintf(title, sizeof title, T("Version %s installée", "Version %s installed"), r->version);
        do {
            screen_begin();
            draw_current_top();
            screen_bottom();
            button_count = 0;
            screen_paragraph(8, 8 + screen_paragraph(8, 8, 0.55f, C_GREEN, 304, title) + 6, 0.5f, WHITE, 304,
                             dsx ? T("3dssavesync va se fermer. Relancez-le depuis le Homebrew Launcher.", "3dssavesync will close. Start it again from the Homebrew Launcher.")
                                 : T("3dssavesync va se fermer. Relancez-le depuis le menu HOME.", "3dssavesync will close. Start it again from the HOME Menu."));
            draw_bar_background(1);
            draw_button_row(&b, 1, 240 - BUTTON_HEIGHT - 4, 0);
            screen_end();
        } while (!(read_keys() & (KEY_A | KEY_B)));
    }
end:
    free(checksums);
    free(bytes);
}

/* Réglages, ligne Mise à jour : vérification, puis installation si l'utilisateur la choisit. */
static void update_app(void)
{
    release_t r;
    char t[300];
    if (check_update(&r) != 1) return;
    snprintf(t, sizeof t, T("La version %s remplacera la version " VERSION ". Vos sauvegardes et vos réglages ne changent pas.",
                            "Version %s will replace version " VERSION ". Your saves and settings stay the same."),
             r.version);
    if (confirm(T("Mettre à jour 3dssavesync ?", "Update 3dssavesync?"), NULL, t, NULL, T("Mettre à jour", "Update"))) install_update(&r);
}

/* Jeux 3DS synchronisés : applications installées et cartouche, cochées si elles sont dans titres.ini.
 * Seul un jeu dont l'archive ne contient que /main peut être coché (title_main_only) : la synchronisation ne copie
 * que ce fichier. Les jeux de titres.ini absents de la console restent listés (pour les décocher). */
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

static void choose_titles(void)
{
    static title_choice_t c[200];
    static u64 ids[200];
    static FS_MediaType medias[200];
    int n, i, cursor = 0, changed = 0;

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
    {   /* sans archive de sauvegarde et pas coché : caché (forwarders mGBA ou DS, jeu jamais lancé) */
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

    while (aptMainLoop()) {
        int start = cursor < TOP_LINES ? 0 : cursor - TOP_LINES + 1;
        const title_choice_t *ch = n ? &c[cursor] : NULL;
        int tickable = ch && (ch->ticked || ch->main_only == 1);
        u16 icon[48 * 48];
        float y = 6;
        u32 k;
        button_t t[2] = { { KEY_A, ch && ch->ticked ? T("Décocher", "Untick") : T("Cocher", "Tick"), tickable },
                          { KEY_B, T("Enregistrer et revenir", "Save and go back"), 1 } };

        screen_begin();
        screen_top();
        screen_text(6, 4, 0.55f, WHITE, 0, T("Jeux 3DS à synchroniser", "3DS games to sync"));
        screen_rect(0, 23, 400, 1, GREY_DARK);
        for (i = start; i < n && i < start + TOP_LINES; i++) {
            float ly = 27 + (i - start) * 15;
            if (i == cursor) {
                screen_rect(0, ly - 1, 400, 15, SEL);
                screen_rect(0, ly - 1, 2, 15, ACCENT);
            }
            screen_rect(9, ly + 2, 10, 10, c[i].ticked || c[i].main_only == 1 ? GREY_LIGHT : GREY_DARK); /* case : bord, fond, coche */
            screen_rect(10, ly + 3, 8, 8, i == cursor ? SEL : RGB(0x0C0E12));
            if (c[i].ticked) screen_rect(11, ly + 4, 6, 6, C_GREEN);
            screen_text(26, ly, 0.5f, c[i].ticked || c[i].main_only == 1 ? WHITE : GREY_DARK, 400 - 26 - 14, c[i].name);
        }
        if (start > 0) screen_text(388, 26, 0.4f, GREY_LIGHT, 0, "\xE2\x96\xB2");
        if (start + TOP_LINES < n) screen_text(388, 224, 0.4f, GREY_LIGHT, 0, "\xE2\x96\xBC");

        screen_bottom();
        button_count = 0;
        if (ch) {
            if (title_icon(ch->id, ch->media, icon) == 0) screen_icon(icon, 48, 48, 320 - 6 - 48, 6, 1.0f);
            y += screen_lines(8, y, 0.55f, WHITE, 320 - 8 - 62, 2, ch->name) + 4;
            y = y < 58 ? 62 : y + 4;
            y += screen_paragraph(8, y, 0.45f, ch->main_only == 1 ? C_GREEN : ch->main_only == 0 ? C_RED : GREY_LIGHT, 304,
                                  ch->main_only == 1 ? T("Ce jeu peut être synchronisé.", "This game can be synced.")
                                  : ch->main_only == 0 ? T("Ce jeu ne peut pas être synchronisé (plusieurs fichiers de sauvegarde).", "This game can't be synced (several save files).")
                                                  : T("Pas encore de sauvegarde : lancez le jeu une fois et sauvegardez.", "No save yet: start the game once and save.")) + 6;
        } else {
            y += screen_paragraph(8, y, 0.5f, GREY_LIGHT, 304, T("Aucun jeu trouvé.", "No game found.")) + 6;
        }
        y += screen_paragraph(8, y, 0.42f, GREY_LIGHT, 304, T("Un jeu manque ? Lancez-le une fois et sauvegardez.", "Game missing? Start it once and save.")) + 6;
        draw_message(BAR_TOP_1);
        draw_bar_background(1);
        draw_button_row(t, 2, 240 - BUTTON_HEIGHT - 4, 18);
        screen_end();

        k = read_keys();
        if (k & (KEY_UP | KEY_DOWN)) clear_message();
        if (k & KEY_B) break;
        if (k & KEY_DOWN && cursor + 1 < n) cursor++;
        if (k & KEY_UP && cursor > 0) cursor--;
        if (k & KEY_A && n) {
            clear_message();
            if (tickable) {
                c[cursor].ticked = !c[cursor].ticked;
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

/* Lignes de réglage modifiables, puis Jeux 3DS, Connexion et Version (écran des réglages). */
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
static int selected_setting; /* ligne choisie des réglages : aussi pour l'écran du haut pendant une progression */

/* État de la version installée, comme sur la ligne Version : « À jour », « 3.7.1 disponible »… */
static const char *version_state(char *t, size_t n, u32 *color)
{
    if (!strcmp(VERSION, "dev")) *color = GREY_LIGHT, snprintf(t, n, "%s", T("Version de développement", "Development build"));
    else if (update_is_newer(VERSION, latest_version)) *color = C_CYAN, snprintf(t, n, T("%s disponible", "%s available"), latest_version);
    else if (update_failed) *color = C_RED, snprintf(t, n, "%s", T("Vérification impossible", "Check failed"));
    else if (!latest_version[0]) *color = GREY_LIGHT, snprintf(t, n, "%s", T("Non vérifiée", "Not checked"));
    else *color = C_GREEN, snprintf(t, n, "%s", T("À jour", "Up to date"));
    return t;
}

/* « 3.7.0 · À jour » à partir de x : version en blanc, point en gris, état en couleur, coupé avant fin. */
static void draw_version_value(float x, float y, float size, float end)
{
    char e[48];
    u32 c;
    version_state(e, sizeof e, &c);
    x += screen_text(x, y, size, WHITE, end - x, VERSION);
    x += screen_text(x, y, size, GREY_DARK, 0, " \xC2\xB7 ");
    screen_text(x, y, size, c, end - x, e);
}

static const char *setting_label(int i)
{
    return i < SETTING_COUNT ? T(SETTINGS[i].name, SETTINGS[i].name_en) : i == SETTING_COUNT ? T("Jeux 3DS", "3DS games")
         : i == SETTING_COUNT + 1 ? T("Connexion", "Connection") : T("Version", "Version");
}

/* Ce que fait A sur la ligne i, dessiné à droite de la ligne choisie. */
static const char *setting_action(int i)
{
    return i < SETTING_COUNT ? T("Modifier", "Change") : i == SETTING_COUNT ? T("Choisir", "Choose") : i == SETTING_COUNT + 1 ? T("Tester", "Test")
         : update_is_newer(VERSION, latest_version) ? T("Mettre à jour", "Update") : T("Rechercher", "Check");
}

/* Écran du haut des réglages : libellé à gauche, valeur sur une colonne, « Ⓐ action » à droite de la ligne choisie. */
static void draw_settings_top(void)
{
    const int total = SETTING_COUNT + 3;
    float y = 29;
    int i;
    screen_top();
    screen_text(6, 4, 0.55f, WHITE, 0, T("Réglages", "Settings"));
    screen_rect(0, 23, 400, 1, GREY_DARK);
    for (i = 0; i < total; i++) {
        char v[96]; /* test_short : 80 */
        const char *value = v;
        u32 c = WHITE;
        int required = 0;
        float end = 400 - 8;
        if (i < SETTING_COUNT) {
            value = SETTINGS[i].value;
            if (i <= 1 && (!value[0] || !strcmp(value, "https://"))) value = T("À remplir", "Required"), c = GREY_LIGHT, required = 1;
            else if (i == 1) value = "\xE2\x80\xA2\xE2\x80\xA2\xE2\x80\xA2\xE2\x80\xA2\xE2\x80\xA2\xE2\x80\xA2\xE2\x80\xA2\xE2\x80\xA2"; /* •••••••• */
            else if (!value[0]) value = T("À côté de la ROM", "Next to the ROM"), c = GREY_LIGHT; /* valeur par défaut */
        } else if (i == SETTING_COUNT) {
            if (title_count) snprintf(v, sizeof v, plural(title_count, english, T("%d jeu", "%d game"), T("%d jeux", "%d games")), title_count);
            else snprintf(v, sizeof v, "%s", T("Aucun", "None"));
        } else if (i == SETTING_COUNT + 1) {
            if (test_state > 0) snprintf(v, sizeof v, "%s", T("Réussie", "OK")), c = C_GREEN;
            else if (test_state < 0) snprintf(v, sizeof v, "%s", test_short), c = C_RED;
            else snprintf(v, sizeof v, "%s", T("Non testée", "Not tested")), c = GREY_LIGHT;
        }
        if (i == selected_setting) {
            const char *a = setting_action(i);
            float xa = 392 - screen_text_width(0.44f, a), g = screen_text_width(0.5f, glyph(KEY_A));
            screen_rect(0, y - 1, 400, 16, SEL);
            screen_rect(0, y - 1, 2, 16, ACCENT);
            screen_text(xa - g - 3, y - 1, 0.5f, RGB(0xE04848), 0, glyph(KEY_A));
            screen_text(xa, y + 1, 0.44f, WHITE, 0, a);
            end = xa - g - 3 - 10;
        }
        if (required) screen_rect(0, y - 1, 2, 16, C_RED);
        screen_text(8, y, 0.46f, i == selected_setting ? WHITE : GREY_LIGHT, 128, setting_label(i));
        if (i == SETTING_COUNT + 2) draw_version_value(142, y, 0.46f, end);
        else screen_text(142, y, 0.46f, c, end - 142, value);
        y += 17 + (i == 1 || i == SETTING_COUNT - 1 ? 7 : 0); /* groupes : serveur, dossiers, le reste */
    }
}

/* Écran de réglages : une ligne par réglage, A pour l'action de la ligne, explication en bas. premier : aucun réglage
 * encore. 1 si un réglage ou la liste des jeux 3DS a changé (il faut alors tout relire). */
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
    draw_current_top = draw_settings_top; /* progression et questions : les réglages restent en haut */
    clear_message();
    while (aptMainLoop()) {
        const int c = selected_setting;
        u32 k;
        float y = 6;
        /* Premier lancement : B teste d'abord la connexion ; un second B après un échec continue quand même. */
        button_t t[2] = { { KEY_A, setting_action(c), 1 },
                          { KEY_B, !first || test_state > 0 ? T("Enregistrer et revenir", "Save and go back")
                                   : test_state < 0           ? T("Continuer quand même", "Continue anyway")
                                                             : T("Tester et continuer", "Test and continue"), 1 } };

        screen_begin();
        draw_settings_top();
        screen_bottom();
        button_count = 0;
        if (first) {
            const char *b = T("Bienvenue ! Entrez d'abord le serveur et le jeton.", "Welcome! Enter the server and the token first.");
            float h = screen_paragraph_height(0.45f, 296, b) + 8;
            screen_rect(0, 0, 320, h, CARD);
            screen_rect(0, 0, 3, h, ACCENT);
            screen_paragraph(10, 4, 0.45f, WHITE, 296, b);
            y = h + 6;
        }
        if (c == count + 2) {
            y += screen_lines(8, y, 0.55f, WHITE, 304, 1, T("Mise à jour de 3dssavesync", "3dssavesync update")) + 8;
            screen_text(8, y, 0.42f, GREY_LIGHT, 100, T("Installée", "Installed"));
            draw_version_value(112, y, 0.42f, 312);
            y += 15 + 6;
            y += screen_paragraph(8, y, 0.42f, GREY_LIGHT, 304, T("Cherche une nouvelle version sur GitHub et vous demande avant de l'installer.",
                                                              "Looks for a new version on GitHub and asks before installing it.")) + 6;
        } else {
            y += screen_lines(8, y, 0.55f, WHITE, 304, 1, setting_label(c)) + 6;
            y += screen_paragraph(8, y, 0.45f, WHITE, 304, c < count ? T(SETTINGS[c].help, SETTINGS[c].help_en)
                                                         : c == count ? T("Les jeux 3DS à synchroniser. Par défaut, les 8 Pokémon 3DS.", "3DS games to sync. Default: the 8 Pokémon 3DS games.")
                                                                   : T("Vérifie l'adresse et le jeton du serveur.", "Checks the server address and token.")) + 6;
        }
        if (message[0]) draw_message(BAR_TOP_1);
        else if (c == count + 2 && update_detail[0]) { /* dernière vérification : texte, puis sa ligne technique (après le dernier saut) */
            char d[sizeof update_detail];
            char *nl;
            u32 color;
            version_state(d, sizeof d, &color);
            snprintf(d, sizeof d, "%s", update_detail);
            nl = strrchr(d, '\n');
            if (nl) *nl++ = '\0';
            draw_band(BAR_TOP_1, d, nl, color == GREY_LIGHT ? WHITE : color);
        }
        draw_bar_background(1);
        draw_button_row(t, 2, 240 - BUTTON_HEIGHT - 4, 18);
        screen_end();

        k = read_keys();
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
                test_state = 0; /* l'adresse ou le jeton a pu changer : le dernier test ne vaut plus */
                edit_text(T(SETTINGS[c].name, SETTINGS[c].name_en), SETTINGS[c].value, SETTINGS[c].n, c == 1);
                len = strlen(SETTINGS[c].value);
                while (len > 0 && SETTINGS[c].value[len - 1] == '/') SETTINGS[c].value[--len] = '\0'; /* "…/saves/" -> "…/saves" */
            } else if (c == count) {
                choose_titles();
            } else if (c == count + 1) {
                test_connection();
            } else {
                update_app();
                if (should_quit) break;
            }
        }
    }
    draw_current_top = draw_list_top;
    if (!first) clear_message(); /* premier lancement : le résultat du test reste affiché */
    changed = memcmp(before, &cfg, sizeof cfg) || before_count != title_count || memcmp(before_titles, titles, sizeof titles);
    if (memcmp(before, &cfg, sizeof cfg)) write_config();
    return changed;
}

/* A sur un Jeu : son action, avec confirmation avant tout écrasement (Q5). 1 si une action a été tentée. */
static int act(game_t *j)
{
    char q[500], srv[200], loc[200], t[80];
    progress_t p;
    int done = 0;
    switch (j->state) {
    case TO_UPLOAD:
        upload_save(j);
        done = 1;
        break;
    case TO_DOWNLOAD: {
        int has_server_progress = server_progress(j, &p);
        server_text(j, srv, sizeof srv);
        local_text(j, loc, sizeof loc);
        if (has_server_progress) progress_text(&p, t, sizeof t);
        snprintf(q, sizeof q, "%s\t%s%s%s\n%s\t%s", T("Sur le serveur", "On the server"), srv, has_server_progress ? "\n\t" : "", has_server_progress ? t : "", T("Sur la 3DS", "On the 3DS"), loc);
        if ((done = confirm(T("Récupérer la version du serveur ?", "Download the server version?"), j->short_name, q,
                              j->has_local ? T("La sauvegarde de la 3DS sera remplacée, après une copie de sécurité.", "The 3DS save will be replaced, after a backup.") : NULL,
                              T("Récupérer", "Download"))))
            download_save(j);
        break;
    }
    case CONFLICT: {
        u32 k;
        const button_t b = { KEY_B, T("Ne rien faire", "Do nothing"), 1 };
        /* Les deux cartes ont la même forme : la version, son temps de jeu (Pokémon), ce qui se passe. */
        if (server_progress(j, &p)) {
            server_text(j, t, sizeof t);
            snprintf(srv, sizeof srv, "%s\n", t);
            progress_text(&p, srv + strlen(srv), sizeof srv - strlen(srv));
        } else server_text(j, srv, sizeof srv);
        if (j->has_progress && j->modified) { /* local_text donne alors le temps de jeu : la date d'abord, comme à droite */
            char d[24];
            format_age((long)(time(NULL) - j->modified), english, 0, d, sizeof d);
            snprintf(loc, sizeof loc, T("Modifiée %s\n", "Modified %s\n"), d);
            progress_text(&j->progress, loc + strlen(loc), sizeof loc - strlen(loc));
        } else local_text(j, loc, sizeof loc);
        do {
            float y = 6, w;
            screen_begin();
            draw_current_top();
            screen_bottom();
            button_count = 0;
            draw_game_icon(j);
            screen_rect(8, y + 2, 3, 14, C_RED);
            w = screen_text(15, y, 0.55f, C_RED, 0, T("Conflit", "Conflict"));
            w += screen_text(15 + w, y, 0.55f, GREY_LIGHT, 0, " \xC2\xB7 ");
            screen_text(15 + w, y, 0.55f, WHITE, 250 - 15 - w, j->short_name);
            y += 22;
            y += screen_lines(8, y, 0.45f, WHITE, 250, 2, T("La sauvegarde a changé sur la 3DS et sur le serveur. Quelle version garder ?",
                                                             "The save changed on the 3DS and on the server. Which one to keep?"));
            y = y < 58 ? 60 : y + 6;
            draw_choice_card(6, y, KEY_A, T("Version 3DS", "3DS save"), loc, T("Puis envoyée au serveur.", "Then uploaded to the server."));
            draw_choice_card(162, y, KEY_X, T("Version serveur", "Server save"), srv, T("La 3DS est copiée avant.", "The 3DS is backed up first."));
            screen_paragraph(8, y + 98, 0.42f, GREY_LIGHT, 304, T("Rien n'est perdu : l'autre version est gardée.", "Nothing is lost: the other one is kept."));
            draw_bar_background(1);
            draw_button_row(&b, 1, 240 - BUTTON_HEIGHT - 4, 0);
            screen_end();
            k = read_keys();
        } while (!(k & (KEY_A | KEY_X | KEY_B)));
        if (k & KEY_A) {
            done = 1;
            upload_save(j);
        } else if (k & KEY_X) {
            done = 1;
            download_save(j);
        } else {
            set_message(WHITE, NULL, T("%s : conflit laissé tel quel.", "%s: conflict left as is."), j->short_name);
        }
        break;
    }
    case DUPLICATE:
        if (j->shares_save) set_message(C_RED, NULL, T("%s : %d ROM ont ce nom de fichier et partageraient une sauvegarde. Renommez-en une.",
                                              "%s: %d ROMs have this file name and would share one save. Rename one."), j->short_name, j->rom_count);
        else set_message(C_RED, NULL, T("%s : %d ROM contiennent ce jeu. Gardez-en une, ou ajoutez les autres aux ROM hacks sur le PC.",
                                   "%s: %d ROMs hold this game. Keep one, or add the others to ROM hacks on the PC."), j->short_name, j->rom_count);
        break;
    case OFFLINE:
        set_message(C_RED, NULL, T("%s : serveur non lu. Appuyez sur \xEE\x80\x83 pour réessayer.", "%s: server not read. Press \xEE\x80\x83 to try again."), j->short_name);
        break;
    default:
        set_message(WHITE, NULL, T("%s : déjà à jour.", "%s: already in sync."), j->short_name);
    }
    if (done) read_local(j);
    return done;
}

/* X sur un Jeu GB/GBA : sa Sauvegarde passe du forwarder mGBA à celle de la ROM (open_agb_firm, TWiLight), ou
 * l'inverse. Celle de la ROM est sav_rom, le fichier que read_local lit sans forwarder (dossier des sauvegardes réglé,
 * ou saves/ du sous-dossier, ADR 0015). Le fichier du forwarder prime dès qu'il existe (read_local) : le retirer
 * suffit à revenir côté ROM. Copie de sécurité du fichier remplacé (files.h). */
static int switch_save_file(game_t *j)
{
    char fwd[600], backup[700], date[20], q[700], warning_text[200], action[64];
    const char *other = j->system == SYS_GBA ? "open_agb_firm" : "TWiLight Menu++";
    int to_rom;
    time_t t = time(NULL);

    if (j->system != SYS_GB && j->system != SYS_GBA) {
        set_message(WHITE, NULL, "%s", T("X : seulement pour les jeux GB et GBA.", "X: GB and GBA games only."));
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
        move_save(j->sav, to_rom ? j->sav_rom : fwd, backup, to_rom))
        set_message(C_RED, j->sav, T("%s : changement impossible, rien n'est perdu.", "%s: switch failed, nothing lost."), j->short_name);
    else
        set_message(C_GREEN, NULL, T("%s : se joue maintenant avec %s.", "%s: now played with %s."), j->short_name, to_rom ? other : T("le forwarder mGBA", "the mGBA forwarder"));
    read_local(j);
    return 1;
}

/* « 2 envois, 1 récupération, 1 ignoré » : seulement ce qui compte. */
static void sync_summary(char *out, size_t n, int uploads, int downloads, int failures, int skipped)
{
    const int v[4] = { uploads, downloads, failures, skipped };
    const char *one[4] = { T("%d envoi", "%d upload"), T("%d récupération", "%d download"), T("%d échec", "%d failed"), T("%d ignoré", "%d skipped") };
    const char *many[4] = { T("%d envois", "%d uploads"), T("%d récupérations", "%d downloads"), T("%d échecs", "%d failed"), T("%d ignorés", "%d skipped") };
    size_t l = 0;
    int i;
    out[0] = '\0';
    for (i = 0; i < 4; i++)
        if (v[i] && l < n) {
            if (l) l += snprintf(out + l, n - l, ", ");
            if (l < n) l += snprintf(out + l, n - l, plural(v[i], english, one[i], many[i]), v[i]);
        }
    if (!l) snprintf(out, n, "%s", T("Rien à synchroniser.", "Nothing to sync."));
}

/* START : Envois et Récupérations ; ignore Conflits et Doublons ; une confirmation pour les écrasements. Puis relit le
 * serveur, et le message dit ce qui a été fait. */
static void sync_all(void)
{
    char q[1200], first[200] = "", first_detail[200] = "", r[120];
    int i, overwrites = 0, uploads = 0, downloads = 0, skipped = 0, failures = 0, may_overwrite = 1, total = 0, done = 0;
    int planned_uploads = 0, planned_downloads = 0;
    u64 start = osGetTime();

    if (!server_read) {
        set_message(C_RED, NULL, "%s", T("Serveur non lu : appuyez sur \xEE\x80\x83 pour réessayer.", "Server not read: press \xEE\x80\x83 to try again."));
        return;
    }
    for (i = 0; i < visible_count; i++) {
        int e = games[visible[i]].state;
        planned_uploads += e == TO_UPLOAD;
        planned_downloads += e == TO_DOWNLOAD;
        skipped += e == CONFLICT || e == DUPLICATE;
    }
    sync_summary(r, sizeof r, planned_uploads, planned_downloads, 0, skipped);
    snprintf(q, sizeof q, "%s\n\n%s", r, T("Ces sauvegardes de la 3DS seront remplacées par celles du serveur, après une copie de sécurité :\n",
                                           "These 3DS saves will be replaced by the server's, after a backup:\n"));
    for (i = 0; i < visible_count; i++) {
        game_t *j = &games[visible[i]];
        if (j->state == TO_DOWNLOAD && j->has_local && overwrites++ < 8) {
            strncat(q, j->short_name, 40);
            if (j->has_progress) {
                char t[80];
                progress_text(&j->progress, t, sizeof t);
                snprintf(q + strlen(q), sizeof q - strlen(q), " (%s)", t);
            }
            strcat(q, "\n");
        }
    }
    if (overwrites > 8) snprintf(q + strlen(q), sizeof q - strlen(q), plural(overwrites - 8, english, T("… et %d autre\n", "… and %d more\n"), T("… et %d autres\n", "… and %d more\n")), overwrites - 8);
    if (overwrites) may_overwrite = confirm(T("Tout synchroniser", "Sync all"), NULL, q, NULL, T("Remplacer", "Replace"));
    for (i = 0; i < visible_count; i++) {
        game_t *j = &games[visible[i]];
        total += j->state == TO_UPLOAD || (j->state == TO_DOWNLOAD && (may_overwrite || !j->has_local));
    }
    operation = T("Tout synchroniser", "Sync all");
    for (i = 0; i < visible_count; i++) {
        game_t *j = &games[visible[i]];
        int do_upload = j->state == TO_UPLOAD;
        int do_download = j->state == TO_DOWNLOAD && (may_overwrite || !j->has_local);
        if (do_upload || do_download) {
            int ok;
            list_cursor = i; /* l'écran du haut suit le jeu en cours */
            progress_color = do_upload ? C_YELLOW : C_CYAN;
            show_progress(++done, total, "%s\n%s", do_upload ? T("Envoi", "Upload") : T("Récupération", "Download"), j->short_name);
            ok = do_upload ? upload_save(j) : download_save(j);
            uploads += do_upload && ok;
            downloads += do_download && ok;
            if (!ok && !failures++) { /* « jeu : raison », et sa ligne technique */
                snprintf(first, sizeof first, "%.199s", message);
                snprintf(first_detail, sizeof first_detail, "%s", message_detail);
            }
            read_local(j);
        }
    }
    operation = NULL;
    progress_color = C_GREEN;
    if (total) reread_server();
    /* Résumé, puis le premier échec : le message de la dernière action ne dit rien des autres. */
    sync_summary(r, sizeof r, uploads, downloads, failures, skipped);
    if (total) snprintf(r + strlen(r), sizeof r - strlen(r), " \xC2\xB7 %llu s", (osGetTime() - start) / 1000);
    set_message(failures ? C_RED : total ? C_GREEN : WHITE, first_detail, "%s%s%s%s", r, failures ? "\n" : "", failures > 1 ? T("Premier échec : ", "First failure: ") : "", first);
}

/* ---------------------------------------------------------------- boucle */

int main(int argc, char **argv)
{
    int cursor = 0;

    gfxInitDefault();
    osSetSpeedupEnable(true); /* 804 MHz sur New 3DS (sans effet sur Old 3DS) : TLS et SHA-256 plus rapides */
    screen_init();
    if (R_SUCCEEDED(cfguInit())) {
        CFGU_GetSystemLanguage(&language);
        cfguExit();
    }
    english = language != CFG_LANGUAGE_FR;
    if (migrate_dir(OLD_DIR, APP_DIR) == 1)
        snprintf(message, sizeof message, T("Réglages repris de %s.", "Settings taken from %s."), OLD_DIR);
    snprintf(cfg.roms, sizeof cfg.roms, "sdmc:/roms/nds");
    snprintf(cfg.saves, sizeof cfg.saves, "sdmc:/roms/nds/saves");
    snprintf(cfg.roms_gb, sizeof cfg.roms_gb, "sdmc:/roms/gb");   /* TWiLight : .sav à côté des ROM */
    snprintf(cfg.roms_gba, sizeof cfg.roms_gba, "sdmc:/roms/gba"); /* open_agb_firm, useSavesFolder=false */
    read_ini(APP_DIR "/config.ini", config_entry);
    read_ini(APP_DIR "/base.ini", base_entry);
    title_count = read_titles(APP_DIR "/titres.ini", titles, MAX_TITLES);
    read_ini(APP_DIR "/maj.ini", update_entry);
    if (strcmp(installed_version, VERSION)) write_update_ini(); /* le PC ne copie pas un .cia plus ancien (carte3ds.go) */
    if (argc > 0 && argv[0]) path_3dsx = argv[0];

    if (net_init()) {
        const button_t b = { KEY_B, T("Quitter", "Quit"), 1 };
        loaded = 1;
        do {
            screen_begin();
            screen_top();
            screen_text(6, 4, 0.55f, WHITE, 0, "3dssavesync");
            screen_bottom();
            button_count = 0;
            screen_paragraph(16, 70, 0.55f, C_RED, 288, T("Impossible de démarrer le réseau. Redémarrez la 3DS et relancez 3dssavesync.", "Couldn't start the network. Restart the 3DS and 3dssavesync."));
            draw_bar_background(1);
            draw_button_row(&b, 1, 240 - BUTTON_HEIGHT - 4, 0);
            screen_end();
        } while (!(read_keys() & (KEY_B | KEY_START)));
        screen_exit();
        gfxExit();
        return 0;
    }

    if (!cfg.url[0] || !cfg.token[0]) {
        char migration_message[512];
        snprintf(migration_message, sizeof migration_message, "%s", message);
        if (!cfg.url[0]) snprintf(cfg.url, sizeof cfg.url, "https://");
        settings_screen(1); /* message : résultat du test de la connexion */
        snprintf(message + strlen(message), sizeof message - strlen(message), "%s%.200s", migration_message[0] && message[0] ? "\n" : "", migration_message);
    }
    if (!should_quit) refresh();
    show_alert();
    if (!should_quit && strcmp(VERSION, "dev") && net_wifi() && update_check_due(last_check_day, time(NULL))) {
        release_t r;
        check_update(&r); /* échec : détail dans les réglages seulement, la synchronisation marche comme avant */
    }
    while (!should_quit && aptMainLoop()) {
        u32 k;
        list_cursor = cursor;
        draw();
        k = read_keys();

        if (k & KEY_B) break;
        if (k & (KEY_UP | KEY_DOWN)) clear_message(); /* le message parle du jeu qu'on quitte */
        if (k & KEY_DOWN && cursor + 1 < visible_count) cursor++;
        if (k & KEY_UP && cursor > 0) cursor--;
        if (k & (KEY_A | KEY_START | KEY_X | KEY_Y | KEY_SELECT)) {
            int selected = visible_count ? visible[cursor] : -1, i;
            clear_message();
            /* Tout relire : Y, ou réglages modifiés. Après une action : le Jeu touché est déjà relu, le serveur seul. */
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
            cursor = i < visible_count ? i : cursor < visible_count ? cursor : visible_count ? visible_count - 1 : 0;
            list_cursor = cursor;
        }
    }
    net_exit();
    screen_exit();
    gfxExit();
    return 0;
}
