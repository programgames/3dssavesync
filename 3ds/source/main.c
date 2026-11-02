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
#include "cle.h"
#include "ecran.h"
#include "erreurs.h"
#include "etat.h"
#include "fichiers.h"
#include "net.h"
#include "pokemon.h"
#include "reglages.h"
#include "titres3ds.h"

#define DOSSIER "sdmc:/3ds/3dssavesync"
#define ANCIEN "sdmc:/3ds/nds-save-sync" /* nom avant la v3 : réglages et Bases repris au premier lancement */
#define FORWARDERS "sdmc:/mGBA/forwarders" /* Sauvegardes des forwarders mGBA */
#define MAX_JEUX 512 /* ponytail: tableau fixe ; au-delà de 512 ROM, les suivantes sont ignorées (et signalées) */
#define LIGNES_HAUT 14 /* lignes d'une liste sur l'écran du haut */
#define MASQUE -1   /* ni Sauvegarde locale ni Version courante : pas affiché */
#define DOUBLON 100 /* plusieurs ROM pour une Clé (ADR 0005) */
#define HORS_LIGNE 101 /* serveur non lu : état inconnu, aucune action proposée */

static int anglais; /* langue de la console : français, sinon anglais */
static u8 langue;   /* CFG_LANGUAGE_* : titre des jeux 3DS dans cette langue */
#define T(fr, en) (anglais ? (en) : (fr))

typedef struct {
    char cle[CLE_MAX]; /* ADR 0007 */
    systeme_t systeme;
    char rom[600];  /* ROM (icône DS) ; vide pour la 3DS */
    char sav[600];  /* fichier de Sauvegarde ; vide pour la 3DS (archive) */
    char sav_rom[600]; /* GB/GBA : sa Sauvegarde sans forwarder mGBA (dossier des sauvegardes) */
    u64 titre;      /* 3DS : title ID */
    FS_MediaType media; /* 3DS : SD ou cartouche */
    char nom[256];  /* nom de la ROM sans extension = nom du .sav */
    char court[48]; /* pour l'affichage seulement : "Pokemon Diamant" */
    int nb_roms;
    int partage;    /* Doublon : ROM de Clés différentes qui partagent un fichier de Sauvegarde (meme_sauvegarde) */
    obs_t local, serveur;
    char hash_local[65], hash_serveur[65], date[17], client[17];
    long age; /* secondes depuis la Version courante (horloge du serveur), -1 si inconnu */
    int a_local, a_serveur;
    int vide; /* SRAM locale vierge : comptée comme absente (cle.h) */
    time_t modif;   /* DS, GB, GBA : date du fichier (horloge de la 3DS) ; 0 si inconnue */
    int a_partie;   /* Pokémon : temps de jeu et dresseur lus (pokemon.h) */
    int mbc2;       /* GB : cartouche MBC2, octet 0x147 de la ROM (ADR 0012) */
    partie_t partie;
    int etat;
} jeu_t;

/* "Pokemon - Version Diamant (France) (Rev 5)" -> "Pokemon Diamant" : retire " - Version" et les étiquettes "(...)". */
static void nom_court(const char *nom, char *out, size_t n)
{
    const char *v = strstr(nom, " - Version ");
    char tmp[256];
    char *fin;
    if (v) snprintf(tmp, sizeof tmp, "%.*s %s", (int)(v - nom), nom, v + 11);
    else snprintf(tmp, sizeof tmp, "%s", nom);
    if ((fin = strstr(tmp, " ("))) *fin = '\0';
    snprintf(out, n, "%.*s", (int)n - 1, tmp); /* coupé exprès : nom d'affichage */
}

static void format_age(long s, char *out, size_t n)
{
    if (s < 0) snprintf(out, n, "%s", "");
    else if (s < 60) snprintf(out, n, "%s", T("à l'instant", "just now"));
    else if (s < 3600) snprintf(out, n, T("il y a %ld min", "%ld min ago"), s / 60);
    else if (s < 86400) snprintf(out, n, T("il y a %ld h", "%ld h ago"), s / 3600);
    else snprintf(out, n, T("il y a %ld j", "%ld days ago"), s / 86400);
}

static struct { char url[256], jeton[256], roms[256], saves[256], roms_gb[256], saves_gb[256], roms_gba[256], saves_gba[256]; } cfg;
static titre_t titres[MAX_TITRES]; /* jeux 3DS synchronisés (titres.ini) */
static int nb_titres;
static jeu_t jeux[MAX_JEUX];
static int nb_jeux;
static struct { char cle[CLE_MAX], hash[65]; } bases[MAX_JEUX];
static int nb_bases;
static int visibles[MAX_JEUX], nb_visibles;
static char message[512];
static char alerte[300]; /* problème vu pendant une lecture ou une action : ajouté au message à la fin */

static void alerter(const char *texte)
{
    if (!strstr(alerte, texte))
        snprintf(alerte + strlen(alerte), sizeof alerte - strlen(alerte), "%s%s", alerte[0] ? "\n" : "", texte);
}

static void montrer_alerte(void)
{
    if (!alerte[0]) return;
    snprintf(message + strlen(message), sizeof message - strlen(message), "%s%s", message[0] ? "\n" : "", alerte);
    alerte[0] = '\0';
}

static int curseur_liste; /* jeu choisi dans la liste : l'écran du haut des dialogues et de la progression */
static int lu;            /* 0 tant que la première lecture n'est pas finie */
static int serveur_lu;    /* 0 si la dernière lecture du serveur a échoué : les jeux locaux restent listés */
static void progression(int fait, int total, const char *format, ...);

/* ---------------------------------------------------------------- fichiers */

static void hash_hex(const unsigned char *octets, size_t taille, char out[65])
{
    unsigned char h[32];
    int i;
    mbedtls_sha256_ret(octets, taille, h, 0);
    for (i = 0; i < 32; i++) sprintf(out + 2 * i, "%02x", h[i]);
}

/* ---------------------------------------------------------------- config et Base */

static void lire_cles(const char *chemin, void (*cle)(const char *, const char *))
{
    FILE *f = fopen(chemin, "r");
    char ligne[512];
    if (!f) { /* coupure entre le retrait de l'ancien et le renommage (remplacer_fichier) : le .tmp est complet */
        snprintf(ligne, sizeof ligne, "%s.tmp", chemin);
        f = fopen(ligne, "r");
    }
    if (!f) return;
    while (fgets(ligne, sizeof ligne, f)) {
        char *egal;
        ligne[strcspn(ligne, "\r\n")] = '\0';
        if ((egal = strchr(ligne, '='))) {
            *egal = '\0';
            cle(ligne, egal + 1);
        }
    }
    fclose(f);
}

static void cle_config(const char *k, const char *v)
{
    if (!strcmp(k, "url")) snprintf(cfg.url, sizeof cfg.url, "%s", v);
    else if (!strcmp(k, "jeton")) snprintf(cfg.jeton, sizeof cfg.jeton, "%s", v);
    else if (!strcmp(k, "roms")) snprintf(cfg.roms, sizeof cfg.roms, "%s", v);
    else if (!strcmp(k, "saves")) snprintf(cfg.saves, sizeof cfg.saves, "%s", v);
    else if (!strcmp(k, "roms_gb")) snprintf(cfg.roms_gb, sizeof cfg.roms_gb, "%s", v);
    else if (!strcmp(k, "roms_gba")) snprintf(cfg.roms_gba, sizeof cfg.roms_gba, "%s", v);
    else if (!strcmp(k, "saves_gb")) snprintf(cfg.saves_gb, sizeof cfg.saves_gb, "%s", v);
    else if (!strcmp(k, "saves_gba")) snprintf(cfg.saves_gba, sizeof cfg.saves_gba, "%s", v);
}

static void ecrire_config(void)
{
    char t[sizeof cfg + 100];
    mkdir(DOSSIER, 0777);
    snprintf(t, sizeof t, "url=%s\njeton=%s\nroms=%s\nsaves=%s\nroms_gb=%s\nsaves_gb=%s\nroms_gba=%s\nsaves_gba=%s\n", cfg.url,
             cfg.jeton, cfg.roms, cfg.saves, cfg.roms_gb, cfg.saves_gb, cfg.roms_gba, cfg.saves_gba);
    if (remplacer_fichier(DOSSIER "/config.ini", t, strlen(t)))
        alerter(T("Réglages non enregistrés. Vérifiez la carte SD.", "Settings not saved. Check the SD card."));
}

static void cle_base(const char *k, const char *v)
{
    if (nb_bases < MAX_JEUX && strlen(k) >= 4 && strlen(k) < CLE_MAX && strlen(v) == 64) {
        snprintf(bases[nb_bases].cle, CLE_MAX, "%s", k);
        snprintf(bases[nb_bases].hash, 65, "%s", v);
        nb_bases++;
    }
}

static const char *base_de(const char *cle)
{
    int i;
    for (i = 0; i < nb_bases; i++)
        if (!strcmp(bases[i].cle, cle)) return bases[i].hash;
    return NULL;
}

/* La seule écriture de Base (design §1) : après À jour, Envoi ou Récupération réussis. */
static void fixer_base(const char *cle, const char *hash)
{
    static char t[MAX_JEUX * (CLE_MAX + 66)];
    size_t n = 0;
    int i;
    for (i = 0; i < nb_bases && strcmp(bases[i].cle, cle); i++);
    if (i == nb_bases) {
        if (nb_bases == MAX_JEUX) return;
        snprintf(bases[nb_bases++].cle, CLE_MAX, "%s", cle);
    }
    snprintf(bases[i].hash, 65, "%s", hash);
    for (i = 0; i < nb_bases; i++) n += snprintf(t + n, sizeof t - n, "%s=%s\n", bases[i].cle, bases[i].hash);
    if (remplacer_fichier(DOSSIER "/base.ini", t, n)) /* une coupure pendant l'écriture ne vide plus base.ini */
        alerter(T("État de synchronisation non enregistré : des conflits peuvent apparaître. Vérifiez la carte SD.",
                  "Sync state not saved: conflicts may appear. Check the SD card."));
}

/* ---------------------------------------------------------------- observation */

static const char *NOMS_SYSTEMES[] = { "DS", "GB", "GBA", "3DS" };

static int extension_de(systeme_t s, const char *ext)
{
    switch (s) {
    case SYS_DS: return !strcasecmp(ext, ".nds");
    case SYS_GB: return !strcasecmp(ext, ".gb") || !strcasecmp(ext, ".gbc");
    case SYS_GBA: return !strcasecmp(ext, ".gba");
    default: return 0;
    }
}

/* Contenu complet de la Sauvegarde locale : le fichier (Trailer d'horloge compris), ou /main de l'archive 3DS. */
static int lire_sauvegarde(jeu_t *j, unsigned char **octets, size_t *taille)
{
    if (j->systeme == SYS_3DS) return titre_lire(j->titre, &j->media, octets, taille);
    return lire_fichier(j->sav, octets, taille);
}

static int forwarder(const jeu_t *j) { return !strncmp(j->sav, FORWARDERS "/", sizeof FORWARDERS); }

/* SRAM du fichier local -> forme du serveur (ADR 0012) : EEPROM d'open_agb_firm remise dans l'ordre de mGBA, MBC2 en
 * 512 octets. Les copies de sécurité gardent les octets du fichier. */
static void vers_serveur(const jeu_t *j, unsigned char **octets, size_t *taille)
{
    unsigned char *s;
    if (j->systeme == SYS_GBA && !forwarder(j)) eeprom_inverser(*octets, *taille);
    if (!j->mbc2 || !(s = malloc(MBC2_SERVEUR))) return;
    if (mbc2_normaliser(*octets, *taille, s)) { /* taille inconnue : telle quelle */
        free(s);
        return;
    }
    free(*octets);
    *octets = s;
    *taille = MBC2_SERVEUR;
}

/* Forme du serveur -> SRAM du fichier local : l'inverse de vers_serveur. MBC2 à la taille du fichier existant (cible),
 * sinon 256 octets pour le forwarder mGBA et 8 Kio pour TWiLight Menu++. 0 si fait. */
static int vers_fichier(const jeu_t *j, unsigned char **octets, size_t *taille, size_t cible)
{
    unsigned char s[MBC2_SERVEUR], *f;
    if (j->systeme == SYS_GBA && !forwarder(j)) eeprom_inverser(*octets, *taille);
    if (!j->mbc2 || mbc2_normaliser(*octets, *taille, s)) return 0;
    if (cible != 256 && cible != 512 && cible != 8192) cible = forwarder(j) ? 256 : 8192;
    if (!(f = malloc(cible))) return -1;
    mbc2_fichier(s, f, cible);
    free(*octets);
    *octets = f;
    *taille = cible;
    return 0;
}

/* Alias (alias.ini, ADR 0014) : "nom de la ROM sans extension=CLÉ", pour qu'un ROM hack ait sa propre Clé. */
#define MAX_ALIAS 64
static struct { char nom[256], valeur[CLE_MAX]; } alias[MAX_ALIAS];
static int nb_alias;

static void cle_alias_ini(const char *k, const char *v)
{
    size_t n = strlen(k);
    while (n && k[n - 1] == ' ') n--; /* "Pokemon Radical Red = GBA-0RAD" */
    if (nb_alias == MAX_ALIAS || !n || n >= sizeof alias[0].nom) return;
    snprintf(alias[nb_alias].nom, sizeof alias[0].nom, "%.*s", (int)n, k);
    snprintf(alias[nb_alias].valeur, sizeof alias[0].valeur, "%s", v); /* coupé : cle_alias le refusera */
    nb_alias++;
}

static const char *alias_de(const char *nom)
{
    int i;
    for (i = 0; i < nb_alias; i++)
        if (!strcmp(alias[i].nom, nom)) return alias[i].valeur;
    return NULL;
}

/* Dossiers jamais lus : sauvegardes, cachés, et ceux de TWiLight Menu++ et du système (_nds, 3ds, luma, Nintendo 3DS, mGBA). */
static int dossier_ignore(const char *nom)
{
    return nom[0] == '.' || nom[0] == '_' || !strcasecmp(nom, "saves") || !strcasecmp(nom, "Nintendo 3DS") ||
           !strcasecmp(nom, "3ds") || !strcasecmp(nom, "luma") || !strcasecmp(nom, "mGBA");
}

static int est_dossier(const struct dirent *e, const char *chemin)
{
    struct stat st;
    if (e->d_type != DT_UNKNOWN) return e->d_type == DT_DIR;
    return !stat(chemin, &st) && S_ISDIR(st.st_mode);
}

/* Une source de ROM, sous-dossiers compris (ADR 0015) : Clé lue dans l'en-tête (ou alias), Sauvegarde nommée comme la
 * ROM (chemin_sauvegarde). Deux ROM de même Clé, ou de même fichier de Sauvegarde (forwarder mGBA compris) : Doublon. */
#define PROFONDEUR 4 /* niveaux de sous-dossiers lus */
static int ignorees; /* ROM au-delà de MAX_JEUX */

static void scanner_dossier(systeme_t s, const char *roms, const char *saves, const char *dossier, int profondeur)
{
    DIR *d = opendir(dossier);
    struct dirent *e;

    while (d && (e = readdir(d))) {
        char chemin[600], sav[600], nom[256], cle[CLE_MAX];
        unsigned char entete[0x200];
        const char *ext = strrchr(e->d_name, '.'), *a;
        jeu_t *j = NULL;
        size_t n;
        FILE *f;
        int i;

        if (snprintf(chemin, sizeof chemin, "%s/%s", dossier, e->d_name) >= (int)sizeof chemin) continue;
        if (est_dossier(e, chemin)) {
            if (profondeur < PROFONDEUR && !dossier_ignore(e->d_name)) scanner_dossier(s, roms, saves, chemin, profondeur + 1);
            continue;
        }
        if (!ext || !extension_de(s, ext)) continue;
        snprintf(nom, sizeof nom, "%.*s", (int)(ext - e->d_name), e->d_name);
        if (chemin_sauvegarde(roms, saves, dossier, nom, sav, sizeof sav)) continue;
        if (!(f = fopen(chemin, "rb"))) continue;
        n = fread(entete, 1, sizeof entete, f);
        fclose(f);
        if (n < sizeof entete) continue;
        if ((a = alias_de(nom))) {
            if (cle_alias(s, a, cle)) {
                char t[120];
                snprintf(t, sizeof t, T("Alias invalide pour « %.40s » (alias.ini) : ROM non synchronisée.",
                                        "Invalid alias for \"%.40s\" (alias.ini): ROM not synced."), nom);
                alerter(t);
                continue;
            }
        } else if (cle_rom(s, entete, cle)) continue; /* homebrew ou en-tête illisible */

        for (i = 0; i < nb_jeux; i++)
            if (!strcmp(jeux[i].cle, cle) || meme_sauvegarde(jeux[i].systeme, jeux[i].nom, jeux[i].sav_rom, s, nom, sav)) j = &jeux[i];
        if (j) {
            j->nb_roms++;
            j->partage |= strcmp(j->cle, cle) != 0;
            continue;
        }
        if (nb_jeux == MAX_JEUX) {
            ignorees++;
            continue;
        }
        j = &jeux[nb_jeux++];
        memset(j, 0, sizeof *j);
        snprintf(j->cle, sizeof j->cle, "%s", cle);
        j->systeme = s;
        snprintf(j->nom, sizeof j->nom, "%s", nom);
        snprintf(j->rom, sizeof j->rom, "%s", chemin);
        snprintf(j->sav_rom, sizeof j->sav_rom, "%s", sav);
        nom_court(j->nom, j->court, sizeof j->court);
        j->nb_roms = 1;
        j->mbc2 = s == SYS_GB && rom_mbc2(entete);
        if (nb_jeux % 10 == 0) progression(0, 0, T("Lecture des ROM : %d jeu(x)…", "Reading ROMs: %d game(s)…"), nb_jeux);
    }
    if (d) closedir(d);
}

/* Sauvegarde locale d'un Jeu : son fichier (forwarder mGBA s'il existe, docs/guide-forwarders-mgba.md) et son Hash
 * (SRAM seule pour GB/GBA, ADR 0008 ; /main pour la 3DS). Appelée au scan, et après une action sur ce seul Jeu. */
static void lire_local(jeu_t *j)
{
    unsigned char *octets;
    size_t taille;
    u64 mtime;
    j->a_local = j->vide = j->a_partie = 0;
    j->modif = 0;
    j->local.taille = 0;
    j->hash_local[0] = '\0';
    if (j->systeme == SYS_DS) snprintf(j->sav, sizeof j->sav, "%s", j->sav_rom);
    else if (j->systeme != SYS_3DS) {
        char f[600];
        snprintf(f, sizeof f, FORWARDERS "/%s.sav", j->nom);
        snprintf(j->sav, sizeof j->sav, "%s", access(f, F_OK) == 0 ? f : j->sav_rom);
    }
    if (j->nb_roms != 1 || lire_sauvegarde(j, &octets, &taille)) return;
    if (j->systeme != SYS_3DS) {
        taille = sram_taille(j->systeme, taille);
        vers_serveur(j, &octets, &taille);
        j->vide = sauvegarde_vide(j->systeme, octets, taille);
        /* stat() ne donne pas la date sur la SD. archive_getmtime la convertit en temps UNIX ; un horodatage 0 (Azahar)
         * devient le 1er janvier 2000 : date inconnue. */
        if (R_SUCCEEDED(archive_getmtime(j->sav, &mtime)) && mtime > 946684800) j->modif = (time_t)mtime;
    }
    j->a_partie = pokemon_partie(octets, taille, &j->partie) == 0; /* SRAM seule : sans Trailer d'horloge */
    hash_hex(octets, taille, j->hash_local);
    j->local.taille = taille;
    j->a_local = 1;
    free(octets);
}

/* Les jeux 3DS de titres.ini dont l'archive existe (jeu installé ou cartouche insérée, ADR 0009). */
static void scanner_3ds(void)
{
    int i;
    for (i = 0; i < nb_titres && nb_jeux < MAX_JEUX; i++) {
        jeu_t *j = &jeux[nb_jeux];
        progression(i + 1, nb_titres, T("Lecture des sauvegardes 3DS\n%s", "Reading 3DS saves\n%s"), titres[i].nom);
        memset(j, 0, sizeof *j);
        j->titre = titres[i].id;
        j->systeme = SYS_3DS;
        j->nb_roms = 1;
        lire_local(j); /* Hash, et temps de jeu d'un Pokémon */
        if (!j->a_local) continue; /* jamais lancé ou absent */
        cle_3ds(j->titre, j->cle);
        snprintf(j->nom, sizeof j->nom, "%s", titres[i].nom);
        snprintf(j->court, sizeof j->court, "%.47s", titres[i].nom);
        nb_jeux++;
    }
}

/* Toutes les sources, puis le Hash de chaque Sauvegarde locale (SRAM seule pour GB/GBA, ADR 0008). */
static void scanner(void)
{
    int i, avant;

    nb_jeux = nb_visibles = lu = ignorees = nb_alias = 0; /* la liste affichée n'est plus valable pendant la lecture */
    lire_cles(DOSSIER "/alias.ini", cle_alias_ini);
    scanner_dossier(SYS_DS, cfg.roms, cfg.saves, cfg.roms, 0); /* sauvegardes vides = à côté de la ROM */
    scanner_dossier(SYS_GB, cfg.roms_gb, cfg.saves_gb, cfg.roms_gb, 0);
    scanner_dossier(SYS_GBA, cfg.roms_gba, cfg.saves_gba, cfg.roms_gba, 0);
    if (ignorees) {
        char t[80];
        snprintf(t, sizeof t, T("%d ROM ignorées : plus de %d jeux.", "%d ROMs skipped: more than %d games."), ignorees, MAX_JEUX);
        alerter(t);
    }
    avant = nb_jeux;
    for (i = 0; i < avant; i++) {
        progression(i + 1, avant, T("Lecture des sauvegardes\n%s", "Reading saves\n%s"), jeux[i].court);
        lire_local(&jeux[i]);
    }
    scanner_3ds();
}

/* GET /saves : Version courante de chaque Jeu. 0 si tout va bien. */
static int lire_serveur(void)
{
    char url[300], *corps, *p;
    size_t taille;
    long statut;
    int i;

    for (i = 0; i < nb_jeux; i++) jeux[i].a_serveur = 0;
    serveur_lu = 0;
    snprintf(url, sizeof url, "%s/saves", cfg.url);
    statut = net_get(url, cfg.jeton, &corps, &taille);
    if (statut != 200) {
        char t[200];
        snprintf(t, sizeof t, T("Lecture du serveur impossible. %s", "Cannot read the server. %s"), texte_reseau(statut, net_wifi(), anglais));
        alerter(t); /* ajouté au message de l'action en cours (montrer_alerte) */
        if (statut > 0) free(corps);
        return -1;
    }
    serveur_lu = 1;
    /* [{"code": "IRBF", "version": "...", "hash": "...", "taille": 524288, "date": "...", "client": "pc"}, ...]
     * ponytail: lecture à la main, liée au format de json.dumps (séparateur ": ") ; un vrai parseur (jsmn) si le serveur change. */
    for (p = strstr(corps, "\"code\": \""); p; p = strstr(p + 1, "\"code\": \"")) {
        const char *h = strstr(p, "\"hash\": \""), *t = strstr(p, "\"taille\": ");
        const char *d = strstr(p, "\"date\": \""), *c = strstr(p, "\"client\": \""), *a = strstr(p, "\"age\": ");
        char cle[CLE_MAX] = "";
        sscanf(p + 9, "%31[^\"]", cle);
        for (i = 0; i < nb_jeux && strcmp(jeux[i].cle, cle); i++);
        if (i == nb_jeux || !h || !t) continue; /* pas de ROM locale : pas affiché */
        sscanf(h + 9, "%64[0-9a-f]", jeux[i].hash_serveur);
        jeux[i].serveur.taille = strtoul(t + 10, NULL, 10);
        if (d) sscanf(d + 9, "%16[^\"]", jeux[i].date);
        if (c) sscanf(c + 11, "%16[^\"]", jeux[i].client);
        jeux[i].age = a ? strtol(a + 7, NULL, 10) : -1;
        jeux[i].a_serveur = 1;
    }
    free(corps);
    return 0;
}

/* Ordre d'affichage : ce qui demande une décision d'abord, puis les actions, puis le reste. */
static int priorite(int e)
{
    switch (e) {
    case CONFLIT: return 0;
    case A_RECUPERER: return 1;
    case A_ENVOYER: return 2;
    case DOUBLON: return 3;
    }
    return 4;
}

static int comparer(const void *a, const void *b)
{
    const jeu_t *x = &jeux[*(const int *)a], *y = &jeux[*(const int *)b];
    int d = priorite(x->etat) - priorite(y->etat);
    return d ? d : strcasecmp(x->court, y->court);
}

static void calculer_etats(void)
{
    int i;
    nb_visibles = 0;
    for (i = 0; i < nb_jeux; i++) {
        jeu_t *j = &jeux[i];
        /* Sauvegarde vierge = absente, sauf si le serveur a la même ; recuperer() la copie quand même. */
        int vu = j->a_local && (!j->vide || (j->a_serveur && !strcmp(j->hash_local, j->hash_serveur)));
        j->local.hash = j->hash_local;
        j->serveur.hash = j->hash_serveur;
        if (j->nb_roms > 1) j->etat = DOUBLON;
        else if (!vu && !j->a_serveur) j->etat = MASQUE;
        else if (!serveur_lu) j->etat = HORS_LIGNE;
        else j->etat = etat(vu ? &j->local : NULL, j->a_serveur ? &j->serveur : NULL, base_de(j->cle));
        if (j->etat == A_JOUR && (!base_de(j->cle) || strcmp(base_de(j->cle), j->hash_local)))
            fixer_base(j->cle, j->hash_local);
        if (j->etat != MASQUE) visibles[nb_visibles++] = i;
    }
    qsort(visibles, nb_visibles, sizeof *visibles, comparer);
}

/* Après une action : le Jeu touché est déjà relu (lire_local), seul le serveur change. */
static int relire_serveur(void)
{
    int r;
    progression(0, 0, "%s", T("Lecture du serveur…", "Reading the server…"));
    r = lire_serveur();
    calculer_etats(); /* serveur non lu : les jeux locaux restent listés, sans action */
    lu = 1;
    return r;
}

/* Tout relire : ROM, Sauvegardes, serveur (Y, réglages modifiés). */
static int rafraichir(void)
{
    int r;
    scanner();
    progression(0, 0, "%s", T("Lecture du serveur…", "Reading the server…"));
    r = lire_serveur();
    calculer_etats();
    lu = 1;
    return r;
}

/* ---------------------------------------------------------------- actions */

/* Envoi conditionnel : If-Match = Version courante vue par ce Client (ADR 0001). 1 si accepté.
 * GB/GBA : seule la SRAM part, le Trailer d'horloge reste sur la 3DS (ADR 0008). */
static int envoyer(jeu_t *j)
{
    char url[300], condition[100], hash[65];
    unsigned char *octets;
    size_t taille;
    long statut;

    if (lire_sauvegarde(j, &octets, &taille)) {
        snprintf(message, sizeof message, T("%s : lecture de la sauvegarde impossible. Vérifiez la carte SD.", "%s: cannot read the save. Check the SD card."), j->court);
        return 0;
    }
    taille = sram_taille(j->systeme, taille);
    vers_serveur(j, &octets, &taille);
    hash_hex(octets, taille, hash);
    if (j->a_serveur) snprintf(condition, sizeof condition, "If-Match: \"%s\"", j->hash_serveur);
    else snprintf(condition, sizeof condition, "If-None-Match: *");
    snprintf(url, sizeof url, "%s/saves/%s", cfg.url, j->cle);
    statut = net_put(url, cfg.jeton, condition, octets, taille);
    free(octets);
    if (statut == 200) {
        fixer_base(j->cle, hash);
        snprintf(message, sizeof message, T("%s : envoyé au serveur.", "%s: uploaded."), j->court);
    } else if (statut == 412) {
        snprintf(message, sizeof message, T("%s : le serveur a changé entre-temps (conflit).", "%s: the server changed meanwhile (conflict)."), j->court);
    } else {
        snprintf(message, sizeof message, T("%s : envoi impossible. %s", "%s: upload failed. %s"), j->court, texte_reseau(statut, net_wifi(), anglais));
    }
    return statut == 200;
}

/* Crée le dossier qui contient chemin s'il manque (dossier des sauvegardes d'un premier jeu). */
static void creer_dossier_de(const char *chemin)
{
    char d[600], *fin;
    snprintf(d, sizeof d, "%s", chemin);
    if ((fin = strrchr(d, '/'))) {
        *fin = '\0';
        mkdir(d, 0777);
    }
}

/* Récupération (design §4) : Copie de sécurité d'abord, puis
 *  - fichier (DS, GB, GBA) : SRAM reçue + Trailer d'horloge local (ADR 0008) dans un .tmp, puis remplacement ;
 *  - 3DS : réécriture de /main avec commit et secure value (ADR 0009).
 * 1 si faite ; en cas d'échec, la Sauvegarde locale est intacte. */
static int recuperer(jeu_t *j)
{
    char tmp[610], copie[700], url[300], hash[65], date[20];
    char *corps;
    unsigned char *ancien = NULL, *nouveau;
    size_t taille, n = 0, sram;
    long statut;
    int echec, existe = j->systeme == SYS_3DS || access(j->sav, F_OK) == 0;
    time_t t = time(NULL);

    snprintf(url, sizeof url, "%s/saves/%s", cfg.url, j->cle);
    statut = net_get(url, cfg.jeton, &corps, &taille);
    if (statut != 200) {
        if (statut > 0) free(corps);
        snprintf(message, sizeof message, T("%s : récupération impossible. %s La sauvegarde de la 3DS reste intacte.", "%s: download failed. %s The 3DS save is intact."),
                 j->court, texte_reseau(statut, net_wifi(), anglais));
        return 0;
    }
    hash_hex((unsigned char *)corps, taille, hash);
    if (strcmp(hash, j->hash_serveur)) {
        free(corps);
        snprintf(message, sizeof message, T("%s : version reçue abîmée, rien n'est écrit. Réessayez.", "%s: received version damaged, nothing written. Try again."), j->court);
        return 0;
    }
    /* Copie dès que la Sauvegarde existe, même si le scan n'a pas pu la lire : jamais d'écrasement sans copie. */
    if (existe) {
        strftime(date, sizeof date, "%Y%m%d-%H%M%S", localtime(&t));
        mkdir(DOSSIER "/copies", 0777);
        snprintf(copie, sizeof copie, DOSSIER "/copies/%s-%s.%s", j->nom, date, j->systeme == SYS_3DS ? "main" : "sav");
        if (access(copie, F_OK) == 0) /* même nom de ROM dans deux sous-dossiers, la même seconde : la Clé les sépare */
            snprintf(copie, sizeof copie, DOSSIER "/copies/%s-%s-%s.%s", j->nom, date, j->cle, j->systeme == SYS_3DS ? "main" : "sav");
        echec = lire_sauvegarde(j, &ancien, &n);
        if (!echec) echec = ecrire_relu(copie, ancien, n); /* relue : une copie abîmée ne protège rien */
        if (echec) {
            free(ancien);
            free(corps);
            snprintf(message, sizeof message, T("%s : copie de sécurité impossible, rien n'est écrit. Vérifiez la carte SD.", "%s: backup copy failed, nothing written. Check the SD card."), j->court);
            return 0;
        }
    }

    if (j->systeme == SYS_3DS) {
        echec = titre_ecrire(j->titre, j->media, corps, taille);
        free(ancien);
        free(corps);
        if (echec) {
            snprintf(message, sizeof message, T("%s : écriture 3DS refusée, sauvegarde intacte.", "%s: 3DS write refused, save intact."), j->court);
            return 0;
        }
    } else {
        /* SRAM reçue (forme du fichier) + Trailer d'horloge local (ce qui dépasse la SRAM de l'ancien fichier). */
        unsigned char *recu = (unsigned char *)corps;
        sram = existe ? sram_taille(j->systeme, n) : n;
        nouveau = vers_fichier(j, &recu, &taille, sram) ? NULL : malloc(taille + (n - sram) + 1);
        if (!nouveau) {
            free(ancien);
            free(recu);
            snprintf(message, sizeof message, T("%s : mémoire insuffisante, rien n'est écrit.", "%s: out of memory, nothing written."), j->court);
            return 0;
        }
        memcpy(nouveau, recu, taille);
        if (n > sram) memcpy(nouveau + taille, ancien + sram, n - sram);
        free(ancien);
        free(recu);
        creer_dossier_de(j->sav); /* dossier des sauvegardes absent (premier jeu) */
        /* .tmp écrit et relu, puis l'ancienne (déjà copiée) retirée, le .tmp renommé et relu (fichiers.h). */
        echec = remplacer_fichier(j->sav, nouveau, taille + (n - sram));
        free(nouveau);
        if (echec == -1) {
            snprintf(message, sizeof message, T("%s : écriture impossible, sauvegarde de la 3DS intacte.", "%s: write failed, 3DS save intact."), j->court);
            return 0;
        }
        snprintf(tmp, sizeof tmp, "%s.tmp", j->sav);
        if (echec && access(tmp, F_OK)) { /* renommée, mais relue différente : pas de Base sur ce fichier */
            snprintf(message, sizeof message, T("%s : sauvegarde écrite abîmée. Vérifiez la carte SD, puis récupérez de nouveau.%s", "%s: written save damaged. Check the SD card, then download again.%s"),
                     j->court, existe ? T(" Ancienne copiée dans /3ds/3dssavesync/copies.", " Old one copied to /3ds/3dssavesync/copies.") : "");
            return 0;
        }
        if (echec) {
            const char *b = strrchr(j->sav, '/') ? strrchr(j->sav, '/') + 1 : j->sav, *ext = strrchr(b, '.');
            int l = (int)strlen(b), coupe = l > 60; /* écran du bas : nom raccourci, extension gardée */
            snprintf(message, sizeof message, T("%s : renommage impossible. Sauvegarde reçue : %.*s%s%s.tmp%s", "%s: rename failed. Downloaded save: %.*s%s%s.tmp%s"),
                     j->court, coupe ? 50 : l, b, coupe ? "…" : "", coupe && ext ? ext : "",
                     existe ? T(". Ancienne copiée dans /3ds/3dssavesync/copies.", ". Old one copied to /3ds/3dssavesync/copies.") : ".");
            return 0;
        }
    }
    fixer_base(j->cle, j->hash_serveur);
    snprintf(message, sizeof message, T("%s : récupéré du serveur.", "%s: downloaded."), j->court);
    return 1;
}

/* ---------------------------------------------------------------- écran */

/* Icône 32×32 d'un jeu DS, lue dans la ROM, en RGB565 ligne par ligne. 0 si lue.
 * Bannière NDS : son offset est à 0x68 de l'en-tête ; tuiles 4 bits à +0x20, palette BGR555 à +0x220. */
#define FOND_ICONE 0x10C3 /* couleur 0 de la palette (transparente) : fond de l'écran du bas (0x14181E) */
static int icone_ds(const jeu_t *j, u16 *pixels)
{
    unsigned char o[4], icone[512 + 32];
    u32 offset;
    FILE *f;
    int t, l, k;

    if (j->systeme != SYS_DS || !(f = fopen(j->rom, "rb"))) return -1; /* seules les ROM DS ont une bannière */
    offset = fseek(f, 0x68, SEEK_SET) == 0 && fread(o, 1, 4, f) == 4 ? o[0] | o[1] << 8 | o[2] << 16 | (u32)o[3] << 24 : 0;
    if (!offset || fseek(f, offset + 0x20, SEEK_SET) || fread(icone, 1, sizeof icone, f) != sizeof icone) {
        fclose(f);
        return -1;
    }
    fclose(f);
    for (t = 0; t < 16; t++) /* 4×4 tuiles de 8×8 */
        for (l = 0; l < 8; l++)
            for (k = 0; k < 8; k++) {
                int idx = (icone[t * 32 + l * 4 + k / 2] >> ((k & 1) * 4)) & 0xF;
                u16 c = icone[512 + idx * 2] | icone[512 + idx * 2 + 1] << 8;
                pixels[((t / 4) * 8 + l) * 32 + (t % 4) * 8 + k] =
                    idx ? (c & 0x1F) << 11 | ((c >> 5) & 0x1F) << 6 | ((c >> 10) & 0x1F) : FOND_ICONE;
            }
    return 0;
}

/* ---------------------------------------------------------------- comparer deux Versions */

/* « 152 h 30 · Julien », ou partie jamais commencée. */
static void texte_partie(const partie_t *p, char *out, size_t n)
{
    if (p->dresseur[0]) snprintf(out, n, "%d h %02d \xC2\xB7 %s", p->heures, p->minutes, p->dresseur);
    else snprintf(out, n, T("%d h %02d \xC2\xB7 partie jamais commencée", "%d h %02d \xC2\xB7 game never started"), p->heures, p->minutes);
}

/* Sauvegarde de la 3DS : temps de jeu (Pokémon), sinon date du fichier si l'horloge la rend crédible. */
static void texte_local(const jeu_t *j, char *out, size_t n)
{
    time_t maintenant = time(NULL);
    if (!j->a_local) snprintf(out, n, "%s", T("aucune sauvegarde", "no save"));
    else if (j->vide) snprintf(out, n, "%s", T("sauvegarde vierge", "blank save"));
    else if (j->a_partie) texte_partie(&j->partie, out, n);
    else if (j->modif && maintenant >= j->modif) {
        char age[24];
        format_age((long)(maintenant - j->modif), age, sizeof age);
        snprintf(out, n, T("modifiée %s", "modified %s"), age);
    } else snprintf(out, n, "%s", T("sauvegarde présente", "save present"));
}

/* Version du serveur : qui l'a envoyée et quand ; avec lire, pour un Pokémon, son temps de jeu (téléchargée). */
static void texte_serveur(const jeu_t *j, int lire, char *out, size_t n)
{
    char age[24], url[300], *corps;
    size_t taille;
    long statut;
    partie_t p;
    if (!j->a_serveur) {
        snprintf(out, n, "%s", serveur_lu ? T("aucune version", "no version") : T("non lu", "not read"));
        return;
    }
    format_age(j->age, age, sizeof age);
    snprintf(out, n, T("envoyée par %s, %s", "sent by %s, %s"), j->client[0] ? j->client : "?", age);
    if (!lire || !j->a_partie) return; /* seulement si la 3DS a un Pokémon reconnu */
    progression(0, 0, "%s", T("Lecture de la version du serveur…", "Reading the server version…"));
    snprintf(url, sizeof url, "%s/saves/%s", cfg.url, j->cle);
    statut = net_get(url, cfg.jeton, &corps, &taille);
    if (statut == 200 && pokemon_partie((unsigned char *)corps, taille, &p) == 0) {
        char t[80];
        texte_partie(&p, t, sizeof t);
        snprintf(out + strlen(out), n - strlen(out), "\n%s", t);
    }
    if (statut > 0) free(corps);
}

/* ---------------------------------------------------------------- écran principal (citro2d, issue #21) */

#define BLANC RGB(0xE8EAED)
#define GRIS_C RGB(0x8A929C)
#define GRIS_F RGB(0x5A626C)
#define C_VERT RGB(0x4CC060)
#define C_JAUNE RGB(0xF0C040)
#define C_CYAN RGB(0x40C8E8)
#define C_ROUGE RGB(0xF05050)

static u32 couleur_etat(int e)
{
    switch (e) {
    case A_ENVOYER: return C_JAUNE;
    case A_RECUPERER: return C_CYAN;
    case CONFLIT: return C_ROUGE;
    case DOUBLON:
    case HORS_LIGNE: return GRIS_C;
    }
    return GRIS_F;
}

static const char *etat_court(int e)
{
    switch (e) {
    case A_ENVOYER: return T("Envoyer", "Upload");
    case A_RECUPERER: return T("Récupérer", "Download");
    case CONFLIT: return T("Conflit", "Conflict");
    case DOUBLON: return T("Doublon", "Duplicate");
    case HORS_LIGNE: return T("Inconnu", "Unknown");
    }
    return "ok";
}

/* Barre des touches en bas de l'écran du bas (maquette 3 de l'issue #21) : chaque touche est aussi touchable au stylet. */
typedef struct {
    u32 touche;
    const char *texte;
    int actif; /* 0 : grisée (A sans rien à faire) */
} touche_t;
static struct { float x, y, l, h; u32 touche; } boutons[8];
static int nb_boutons;
#define HAUTEUR_TOUCHE 22
#define HAUT_BARRE (240 - 2 * HAUTEUR_TOUCHE - 4)

/* Glyphe de la police du système (U+E000 A, E001 B, E002 X, E003 Y) à la couleur de la New 3DS, ou START / SELECT. */
static const char *glyphe(u32 t)
{
    return t == KEY_A ? "\xEE\x80\x80" : t == KEY_B ? "\xEE\x80\x81" : t == KEY_X ? "\xEE\x80\x82" : t == KEY_Y ? "\xEE\x80\x83" : t == KEY_START ? "START" : "SELECT";
}

static float largeur_touche(const touche_t *t, float taille)
{
    int mot = t->touche & (KEY_START | KEY_SELECT);
    return ecran_largeur(mot ? 0.36f : 0.55f, glyphe(t->touche)) + 4 + ecran_largeur(taille, t->texte);
}

/* Une ligne de touches à partir de y ; ecart = 0 : touches réparties sur toute la largeur.
 * Texte réduit si la ligne ne tient pas avec au moins 10 pixels entre deux touches. */
static void ligne_touches(const touche_t *t, int n, float y, float ecart)
{
    float l[6], total, x = 6, taille = 0.44f;
    int i;
    do {
        taille -= 0.02f;
        for (total = 0, i = 0; i < n; i++) total += l[i] = largeur_touche(&t[i], taille);
    } while (taille > 0.34f && total + 10 * (n - 1) > 308);
    if (!ecart) ecart = n > 1 ? (308 - total) / (n - 1) : 0;
    for (i = 0; i < n; i++) {
        int mot = t[i].touche & (KEY_START | KEY_SELECT);
        u32 c = !t[i].actif ? GRIS_F : t[i].touche == KEY_A ? RGB(0xE04848) : t[i].touche == KEY_B ? RGB(0xF0C030)
              : t[i].touche == KEY_X ? RGB(0x4080F0) : t[i].touche == KEY_Y ? RGB(0x40B060) : GRIS_C;
        float g = ecran_texte(x, y + (mot ? 5 : 2), mot ? 0.36f : 0.55f, c, 0, glyphe(t[i].touche));
        ecran_texte(x + g + 4, y + 3, taille, t[i].actif ? BLANC : GRIS_F, 0, t[i].texte);
        if (nb_boutons < 8) { /* zone touchable : la touche et la moitié de l'écart de chaque côté */
            boutons[nb_boutons].x = x - ecart / 2;
            boutons[nb_boutons].y = y - 2;
            boutons[nb_boutons].l = l[i] + ecart;
            boutons[nb_boutons].h = HAUTEUR_TOUCHE;
            boutons[nb_boutons++].touche = t[i].touche;
        }
        x += l[i] + ecart;
    }
}

/* Carte de choix de l'écran de Conflit (152 × 92), touchable : touche, action, explication. */
static void carte(float x, float y, u32 touche, const char *action, const char *texte)
{
    ecran_rect(x, y, 152, 92, RGB(0x2A313B));
    ecran_texte(x + 6, y + 4, 0.55f, touche == KEY_A ? RGB(0xE04848) : RGB(0x4080F0), 0, glyphe(touche));
    ecran_texte(x + 24, y + 5, 0.45f, BLANC, 122, action);
    ecran_paragraphe(x + 6, y + 24, 0.4f, GRIS_C, 140, texte);
    if (nb_boutons < 8) {
        boutons[nb_boutons].x = x;
        boutons[nb_boutons].y = y;
        boutons[nb_boutons].l = 152;
        boutons[nb_boutons].h = 92;
        boutons[nb_boutons++].touche = touche;
    }
}

/* Touche du bouton sous le stylet, 0 si aucun. */
static u32 bouton_touche(void)
{
    touchPosition p;
    int i;
    hidTouchRead(&p);
    for (i = 0; i < nb_boutons; i++)
        if (p.px >= boutons[i].x && p.px < boutons[i].x + boutons[i].l && p.py >= boutons[i].y && p.py < boutons[i].y + boutons[i].h)
            return boutons[i].touche;
    return 0;
}

/* Fond de la barre des touches : 1 ou 2 lignes, en bas de l'écran du bas. */
static void fond_barre(int lignes)
{
    float y = 240 - lignes * HAUTEUR_TOUCHE - 7;
    ecran_rect(0, y, 320, 240 - y, RGB(0x1E242C));
}

/* Icône du jeu en haut à droite de l'écran du bas. */
static void icone_jeu(const jeu_t *j)
{
    u16 icone[48 * 48];
    if (j->systeme == SYS_3DS ? titre_icone(j->titre, j->media, icone) == 0 : icone_ds(j, icone) == 0)
        ecran_icone(icone, j->systeme == SYS_3DS ? 48 : 32, j->systeme == SYS_3DS ? 48 : 32, 320 - 6 - 48, 6, j->systeme == SYS_3DS ? 1.0f : 1.5f);
}

static void dessiner_haut(int curseur)
{
    static const int etats[] = { A_JOUR, DOUBLON, A_ENVOYER, A_RECUPERER, CONFLIT }; /* compteurs, de droite à gauche */
    int i, n[5] = { 0 }, debut = curseur < LIGNES_HAUT ? 0 : curseur - LIGNES_HAUT + 1;
    char t[48];
    float x = 400 - 6;

    for (i = 0; i < nb_visibles; i++) n[priorite(jeux[visibles[i]].etat)]++;
    ecran_haut();
    ecran_texte(6, 4, 0.55f, BLANC, 0, "3dssavesync");
    for (i = 0; i < 5 && (serveur_lu || !lu); i++) {
        int e = etats[i], c = n[priorite(e)];
        if (!c && e != A_JOUR) continue;
        snprintf(t, sizeof t, e == A_JOUR ? T("%d à jour", "%d in sync") : e == DOUBLON ? T("%d doublon(s)", "%d duplicate(s)")
                            : e == A_ENVOYER ? T("%d à envoyer", "%d to upload") : e == A_RECUPERER ? T("%d à récupérer", "%d to download")
                                                                                 : T("%d conflit(s)", "%d conflict(s)"), c);
        x -= ecran_largeur(0.45f, t);
        ecran_texte(x, 6, 0.45f, e == A_JOUR ? C_VERT : couleur_etat(e), 0, t);
        x -= 12;
    }
    if (lu && !serveur_lu) { /* compteurs sans le serveur : faux */
        x = 400 - 6 - ecran_largeur(0.45f, T("serveur non lu", "server not read"));
        ecran_texte(x, 6, 0.45f, C_ROUGE, 0, T("serveur non lu", "server not read"));
    }
    ecran_rect(0, 23, 400, 1, GRIS_F);

    if (!nb_visibles) ecran_paragraphe(10, 40, 0.5f, GRIS_C, 380, !lu ? T("Lecture en cours…", "Reading…")
                                                              : serveur_lu ? T("Aucun jeu à afficher.", "No game to show.")
                                                              : T("Serveur non lu, et aucune sauvegarde sur la 3DS.", "Server not read, and no save on the 3DS."));
    for (i = debut; i < nb_visibles && i < debut + LIGNES_HAUT; i++) {
        jeu_t *k = &jeux[visibles[i]];
        float y = 27 + (i - debut) * 15;
        if (i == curseur) ecran_rect(0, y - 1, 400, 15, RGB(0x24508C));
        ecran_texte(8, y, 0.45f, i == curseur && k->etat == A_JOUR ? GRIS_C : couleur_etat(k->etat), 62, etat_court(k->etat));
        ecran_texte(76, y, 0.45f, GRIS_C, 0, NOMS_SYSTEMES[k->systeme]);
        ecran_texte(108, y, 0.5f, BLANC, 400 - 108 - 14, k->court);
    }
    if (debut > 0) ecran_texte(388, 26, 0.4f, GRIS_C, 0, "\xE2\x96\xB2");                         /* ▲ */
    if (debut + LIGNES_HAUT < nb_visibles) ecran_texte(388, 224, 0.4f, GRIS_C, 0, "\xE2\x96\xBC"); /* ▼ */
}

/* Une ligne « libellé  valeur » des détails. */
static float detail(float y, const char *libelle, const char *valeur)
{
    ecran_texte(8, y, 0.45f, GRIS_C, 0, libelle);
    ecran_texte(108, y, 0.45f, BLANC, 204, valeur);
    return y + 15;
}

static const char *phrase_etat(const jeu_t *j, char *t, size_t n)
{
    switch (j->etat) {
    case A_ENVOYER: return T("La sauvegarde de la 3DS est plus récente. \xEE\x80\x80 l'envoie au serveur.", "The 3DS save is newer. \xEE\x80\x80 uploads it to the server.");
    case A_RECUPERER: return T("Le serveur a une version plus récente. \xEE\x80\x80 la copie sur la 3DS (copie de sécurité faite avant).",
                               "The server has a newer version. \xEE\x80\x80 copies it to the 3DS (backup copy made first).");
    case CONFLIT: return T("Conflit : la 3DS et le serveur ont changé depuis la dernière synchronisation. \xEE\x80\x80 permet de choisir la version à garder.",
                           "Conflict: the 3DS and the server both changed since the last sync. \xEE\x80\x80 lets you choose the version to keep.");
    case DOUBLON:
        if (j->partage) snprintf(t, n, T("%d ROM ont ce nom de fichier : elles partageraient une sauvegarde (forwarder mGBA ou dossier des sauvegardes). Renommez-en une.",
                                         "%d ROMs have this file name: they would share one save (mGBA forwarder or saves folder). Rename one."), j->nb_roms);
        else snprintf(t, n, T("%d ROM contiennent ce jeu : gardez-en une seule, ou donnez un alias aux autres (alias.ini).", "%d ROMs hold this game: keep only one, or give the others an alias (alias.ini)."), j->nb_roms);
        return t;
    case HORS_LIGNE: return T("Serveur non lu : état inconnu. \xEE\x80\x83 relit le serveur.", "Server not read: state unknown. \xEE\x80\x83 reads the server again.");
    }
    return T("À jour : la 3DS et le serveur ont la même sauvegarde. Rien à faire.", "In sync: the 3DS and the server have the same save. Nothing to do.");
}

/* Message (jaune) à partir de y, jamais sous la barre des touches (bas) : s'il ne tient pas, il remonte sur un fond,
 * par-dessus la fin des détails. */
static void dessiner_message(float y, float bas)
{
    float h;
    if (!message[0]) return;
    h = ecran_hauteur(0.45f, 304, message);
    if (y + h > bas) {
        y = bas - h < 6 ? 6 : bas - h;
        ecran_rect(0, y - 4, 320, bas - y + 4, RGB(0x1E242C));
    }
    ecran_paragraphe(8, y, 0.45f, C_JAUNE, 304, message);
}

static void dessiner_bas(const jeu_t *j)
{
    char t[600];
    float y = 6;
    int action = 0, gb = 0;

    ecran_bas();
    nb_boutons = 0;
    if (j) {
        const char *b = base_de(j->cle);
        gb = j->systeme == SYS_GB || j->systeme == SYS_GBA;
        action = j->etat == A_ENVOYER || j->etat == A_RECUPERER || j->etat == CONFLIT;
        icone_jeu(j);
        y += ecran_paragraphe(8, y, 0.6f, BLANC, 320 - 8 - 62, j->court);
        ecran_texte(8, y, 0.4f, GRIS_C, 320 - 8 - 62, j->systeme == SYS_DS ? "Nintendo DS" : j->systeme == SYS_GB ? "Game Boy / Game Boy Color"
                                                       : j->systeme == SYS_GBA ? "Game Boy Advance" : "Nintendo 3DS");
        y = y + 14 < 58 ? 62 : y + 18; /* sous l'icône */

        y += ecran_paragraphe(8, y, 0.5f, j->etat == A_JOUR ? C_VERT : couleur_etat(j->etat), 304, phrase_etat(j, t, sizeof t)) + 6;
        texte_local(j, t, sizeof t);
        y = detail(y, "3DS", t);
        texte_serveur(j, 0, t, sizeof t);
        y = detail(y, T("Serveur", "Server"), t);
        y = detail(y, T("Dernière synchro", "Last sync"),
                   !b ? T("jamais", "never")
                   : !strcmp(b, j->hash_local) && !strcmp(b, j->hash_serveur) ? T("identique aux deux", "same as both")
                   : !strcmp(b, j->hash_local) ? T("identique à la 3DS", "same as the 3DS")
                   : !strcmp(b, j->hash_serveur) ? T("identique au serveur", "same as the server") : T("autre version", "another version"));
        if (gb) y = detail(y, T("Joué avec", "Played with"), !strncmp(j->sav, FORWARDERS, strlen(FORWARDERS)) ? "forwarder mGBA" : j->systeme == SYS_GBA ? "open_agb_firm" : "TWiLight Menu++");
        y += 6;
    }
    dessiner_message(y, HAUT_BARRE - 5);

    /* Barre : en haut, ce que font A (toujours affichée, grisée s'il n'y a rien à faire) et X (GB/GBA) pour ce jeu ;
     * en bas, les touches générales. */
    fond_barre(2);
    {
        char x[80];
        touche_t jeu[2] = { { KEY_A, !j ? T("Aucun jeu", "No game") : j->etat == A_ENVOYER ? T("Envoyer au serveur", "Upload to the server")
                                     : j->etat == A_RECUPERER ? T("Récupérer du serveur", "Download from the server")
                                     : j->etat == CONFLIT ? T("Résoudre le conflit", "Resolve the conflict") : T("Rien à faire", "Nothing to do"), action },
                            { KEY_X, x, 1 } };
        const touche_t generales[4] = { { KEY_Y, T("Rafraîchir", "Refresh"), 1 }, { KEY_START, T("Tout synchro", "Sync all"), 1 },
                                        { KEY_SELECT, T("Réglages", "Settings"), 1 }, { KEY_B, T("Quitter", "Quit"), 1 } };
        if (gb) snprintf(x, sizeof x, T("Jouer avec %s", "Play with %s"), !strncmp(j->sav, FORWARDERS, strlen(FORWARDERS))
                                                    ? (j->systeme == SYS_GBA ? "open_agb_firm" : "TWiLight Menu++") : T("le forwarder mGBA", "the mGBA forwarder"));
        ligne_touches(jeu, gb ? 2 : 1, HAUT_BARRE, 18);
        ligne_touches(generales, 4, HAUT_BARRE + HAUTEUR_TOUCHE, 0);
    }
}

static void dessiner(int curseur)
{
    ecran_debut();
    dessiner_haut(curseur);
    dessiner_bas(nb_visibles ? &jeux[visibles[curseur]] : NULL);
    ecran_finir();
}

/* Touches appuyées (avec répétition pour les flèches), ou touche de la barre sous le stylet. B si l'application se ferme. */
static u32 lire_touches(void)
{
    u32 k = 0;
    while (!k && aptMainLoop()) {
        gspWaitForVBlank();
        hidScanInput();
        k = hidKeysDownRepeat() & ~KEY_TOUCH;
        if (hidKeysDown() & KEY_TOUCH) k |= bouton_touche();
    }
    return k ? k : KEY_B;
}

/* Progression des longues boucles : la liste reste en haut ; en bas, le texte et une barre si total > 0.
 * Au plus 20 images par seconde : une image attend l'écran, la lecture ne doit pas en être ralentie. */
static void progression(int fait, int total, const char *format, ...)
{
    static u64 derniere;
    char t[300];
    va_list a;
    u64 maintenant = osGetTime();
    if (total && fait > 1 && fait < total && maintenant - derniere < 50) return;
    derniere = maintenant;
    va_start(a, format);
    vsnprintf(t, sizeof t, format, a);
    va_end(a);
    ecran_debut();
    dessiner_haut(curseur_liste);
    ecran_bas();
    ecran_paragraphe(16, 70, 0.55f, BLANC, 288, t);
    if (total > 0) {
        char n[32];
        ecran_rect(16, 140, 288, 12, RGB(0x2A313B));
        ecran_rect(16, 140, 288.0f * fait / total, 12, C_VERT);
        snprintf(n, sizeof n, "%d / %d", fait, total);
        ecran_texte(16, 158, 0.45f, GRIS_C, 0, n);
    }
    ecran_finir();
}

/* Question sur l'écran du bas (la liste reste en haut) : 1 si A, l'action nommée ; 0 si B, Annuler. */
static int confirmer(const char *titre, const char *texte, const char *action)
{
    const touche_t t[2] = { { KEY_A, action, 1 }, { KEY_B, T("Annuler", "Cancel"), 1 } };
    for (;;) {
        u32 k;
        ecran_debut();
        dessiner_haut(curseur_liste);
        ecran_bas();
        nb_boutons = 0;
        ecran_paragraphe(8, 8 + ecran_paragraphe(8, 8, 0.55f, C_CYAN, 304, titre) + 6, 0.45f, BLANC, 304, texte);
        fond_barre(1);
        ligne_touches(t, 2, 240 - HAUTEUR_TOUCHE - 4, 18);
        ecran_finir();
        k = lire_touches();
        if (k & KEY_A) return 1;
        if (k & KEY_B) return 0;
    }
}

static void editer(const char *titre, char *valeur, size_t n, int cache)
{
    SwkbdState s;
    char saisie[256];
    swkbdInit(&s, SWKBD_TYPE_WESTERN, 2, (int)n - 1);
    swkbdSetHintText(&s, titre);
    swkbdSetInitialText(&s, valeur);
    swkbdSetValidation(&s, SWKBD_ANYTHING, 0, 0); /* vide permis : « à côté de la ROM » */
    if (cache) swkbdSetPasswordMode(&s, SWKBD_PASSWORD_HIDE_DELAY);
    if (swkbdInputText(&s, saisie, sizeof saisie) == SWKBD_BUTTON_CONFIRM) snprintf(valeur, n, "%s", saisie);
}

/* GET /saves : résultat du test dans out. 1 si la connexion marche. */
static int tester_connexion(char *out, size_t n)
{
    char url[300], *corps;
    size_t taille;
    long statut;
    progression(0, 0, "%s", T("Test de la connexion…", "Testing the connection…"));
    snprintf(url, sizeof url, "%s/saves", cfg.url);
    statut = net_get(url, cfg.jeton, &corps, &taille);
    if (statut > 0) free(corps);
    snprintf(out, n, "%s", statut == 200 ? T("Connexion réussie.", "Connection OK.") : texte_reseau(statut, net_wifi(), anglais));
    return statut == 200;
}

/* Jeux 3DS synchronisés : applications installées et cartouche, cochées si elles sont dans titres.ini.
 * Seul un jeu dont l'archive ne contient que /main peut être coché (titre_seul_main) : la synchronisation ne copie
 * que ce fichier. Les jeux de titres.ini absents de la console restent listés (pour les décocher). */
typedef struct {
    u64 id;
    FS_MediaType media;
    char nom[48];
    int seul, coche;
} choix_t;

static int comparer_choix(const void *a, const void *b)
{
    return strcasecmp(((const choix_t *)a)->nom, ((const choix_t *)b)->nom);
}

static void choisir_titres(void)
{
    static choix_t c[200];
    static u64 ids[200];
    static FS_MediaType medias[200];
    int n, i, curseur = 0, change = 0;

    progression(0, 0, "%s", T("Lecture des jeux installés…", "Reading installed games…"));
    if (R_FAILED(amInit())) {
        snprintf(message, sizeof message, "%s", T("Liste des jeux installés illisible. Redémarrez la 3DS, puis réessayez.",
                                                  "Cannot read the installed games. Restart the 3DS, then try again."));
        return;
    }
    n = titres_installes(ids, medias, 200);
    amExit();
    for (i = 0; i < nb_titres && n < 200; i++) {
        int k;
        for (k = 0; k < n && ids[k] != titres[i].id; k++);
        if (k == n) {
            ids[n] = titres[i].id;
            medias[n++] = MEDIATYPE_SD;
        }
    }
    {   /* sans archive de sauvegarde et pas coché : caché (forwarders mGBA ou DS, jeu jamais lancé) */
        int m = 0;
        for (i = 0; i < n; i++) {
            int t = titres_index(titres, nb_titres, ids[i]), seul;
            progression(i + 1, n, "%s", T("Lecture des jeux", "Reading games"));
            seul = titre_seul_main(ids[i]);
            if (seul < 0 && t < 0) continue;
            c[m].id = ids[i];
            c[m].media = medias[i];
            c[m].coche = t >= 0;
            c[m].seul = seul;
            if (titre_nom(ids[i], medias[i], langue, c[m].nom, sizeof c[m].nom)) {
                if (t >= 0) snprintf(c[m].nom, sizeof c[m].nom, "%s", titres[t].nom);
                else snprintf(c[m].nom, sizeof c[m].nom, "%016llX", ids[i]);
            }
            m++;
        }
        n = m;
    }
    qsort(c, n, sizeof *c, comparer_choix);
    message[0] = '\0';

    while (aptMainLoop()) {
        int debut = curseur < LIGNES_HAUT ? 0 : curseur - LIGNES_HAUT + 1;
        const choix_t *ch = n ? &c[curseur] : NULL;
        int cochable = ch && (ch->coche || ch->seul == 1);
        u16 icone[48 * 48];
        float y = 6;
        u32 k;
        touche_t t[2] = { { KEY_A, ch && ch->coche ? T("Décocher", "Untick") : T("Cocher", "Tick"), cochable },
                          { KEY_B, T("Enregistrer et revenir", "Save and go back"), 1 } };

        ecran_debut();
        ecran_haut();
        ecran_texte(6, 4, 0.55f, BLANC, 0, T("Jeux 3DS à synchroniser", "3DS games to sync"));
        ecran_rect(0, 23, 400, 1, GRIS_F);
        for (i = debut; i < n && i < debut + LIGNES_HAUT; i++) {
            float ly = 27 + (i - debut) * 15;
            if (i == curseur) ecran_rect(0, ly - 1, 400, 15, RGB(0x24508C));
            ecran_rect(9, ly + 2, 10, 10, c[i].coche || c[i].seul == 1 ? GRIS_C : GRIS_F); /* case : bord */
            ecran_rect(10, ly + 3, 8, 8, c[i].coche ? C_VERT : i == curseur ? RGB(0x24508C) : RGB(0x0C0E12));
            ecran_texte(26, ly, 0.5f, c[i].coche || c[i].seul == 1 ? BLANC : GRIS_F, 400 - 26 - 14, c[i].nom);
        }
        if (debut > 0) ecran_texte(388, 26, 0.4f, GRIS_C, 0, "\xE2\x96\xB2");
        if (debut + LIGNES_HAUT < n) ecran_texte(388, 224, 0.4f, GRIS_C, 0, "\xE2\x96\xBC");

        ecran_bas();
        nb_boutons = 0;
        if (ch) {
            if (titre_icone(ch->id, ch->media, icone) == 0) ecran_icone(icone, 48, 48, 320 - 6 - 48, 6, 1.0f);
            y += ecran_paragraphe(8, y, 0.55f, BLANC, 320 - 8 - 62, ch->nom) + 4;
            y = y < 58 ? 62 : y + 4;
            y += ecran_paragraphe(8, y, 0.45f, ch->seul == 1 ? C_VERT : ch->seul == 0 ? C_ROUGE : C_JAUNE, 304,
                                  ch->seul == 1 ? T("Sauvegarde : un seul fichier. Ce jeu peut être synchronisé.", "Save: a single file. This game can be synced.")
                                  : ch->seul == 0 ? T("Plusieurs fichiers de sauvegarde : ce jeu ne peut pas être synchronisé.", "Several save files: this game cannot be synced.")
                                                  : T("Pas encore de sauvegarde : lancez le jeu une fois et sauvegardez.", "No save yet: start the game once and save.")) + 6;
        } else {
            y += ecran_paragraphe(8, y, 0.5f, GRIS_C, 304, T("Aucun jeu trouvé.", "No game found.")) + 6;
        }
        y += ecran_paragraphe(8, y, 0.42f, GRIS_C, 304, T("Un jeu absent de la liste ? Lancez-le une fois et sauvegardez.", "A game is missing? Start it once and save.")) + 6;
        dessiner_message(y, 240 - HAUTEUR_TOUCHE - 9);
        fond_barre(1);
        ligne_touches(t, 2, 240 - HAUTEUR_TOUCHE - 4, 18);
        ecran_finir();

        k = lire_touches();
        if (k & (KEY_UP | KEY_DOWN)) message[0] = '\0';
        if (k & KEY_B) break;
        if (k & KEY_DOWN && curseur + 1 < n) curseur++;
        if (k & KEY_UP && curseur > 0) curseur--;
        if (k & KEY_A && n) {
            message[0] = '\0';
            if (cochable) {
                c[curseur].coche = !c[curseur].coche;
                change = 1;
            } else {
                snprintf(message, sizeof message, "%s", T("Ce jeu ne peut pas être coché.", "This game cannot be ticked."));
            }
        }
    }
    if (change) {
        nb_titres = 0;
        for (i = 0; i < n && nb_titres < MAX_TITRES; i++)
            if (c[i].coche) {
                titres[nb_titres].id = c[i].id;
                snprintf(titres[nb_titres++].nom, sizeof titres[0].nom, "%s", c[i].nom);
            }
        mkdir(DOSSIER, 0777);
        snprintf(message, sizeof message, "%s",
                 titres_ecrire(DOSSIER "/titres.ini", titres, nb_titres) ? T("Liste des jeux 3DS non enregistrée. Vérifiez la carte SD.", "3DS game list not saved. Check the SD card.")
                                                                          : T("Liste des jeux 3DS enregistrée.", "3DS game list saved."));
    } else {
        message[0] = '\0';
    }
}

/* Écran de réglages : une ligne par réglage, A pour modifier, aide en bas. premier : aucun réglage encore.
 * 1 si un réglage ou la liste des jeux 3DS a changé (il faut alors tout relire). */
static int reglages(int premier)
{
    struct {
        const char *nom, *nom_en, *aide, *aide_en;
        char *valeur;
        size_t n;
    } l[] = {
        { "Serveur", "Server", "Adresse du serveur, par exemple https://saves.exemple.fr, ou le serveur local du PC : http://192.168.1.20:8765",
          "Server address, for example https://saves.example.com, or the PC local server: http://192.168.1.20:8765", cfg.url, sizeof cfg.url },
        { "Jeton", "Token", "Le jeton du serveur (le même que dans la configuration du PC).", "The server token (the same as in the PC configuration).",
          cfg.jeton, sizeof cfg.jeton },
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
    const int nb = (int)(sizeof l / sizeof *l), total = nb + 2;
    int c = 0, i, change, test_ok = 0;
    char test[200] = "";
    static titre_t avant_titres[MAX_TITRES];
    int avant_nb = nb_titres;
    char avant[sizeof cfg];

    memcpy(avant, &cfg, sizeof cfg);
    memcpy(avant_titres, titres, sizeof titres);

    message[0] = '\0';
    while (aptMainLoop()) {
        u32 k;
        float y = 30;
        /* Premier lancement : B teste d'abord la connexion ; un second B après un échec revient quand même. */
        touche_t t[2] = { { KEY_A, c < nb ? T("Modifier", "Change") : c == nb ? T("Choisir", "Choose") : T("Tester", "Test"), 1 },
                          { KEY_B, !premier || test_ok ? T("Enregistrer et revenir", "Save and go back")
                                   : test[0]           ? T("Revenir quand même", "Go back anyway")
                                                       : T("Tester et revenir", "Test and go back"), 1 } };

        ecran_debut();
        ecran_haut();
        ecran_texte(6, 4, 0.55f, BLANC, 0, T("Réglages", "Settings"));
        ecran_rect(0, 23, 400, 1, GRIS_F);
        for (i = 0; i < total; i++) {
            const char *v = i < nb ? l[i].valeur : "";
            char nb3ds[16];
            if (i == 1 && v[0]) v = "••••••••";
            else if (i < nb && !v[0]) v = i == 0 || i == 1 ? T("(vide)", "(empty)") : T("(à côté de la ROM)", "(next to the ROM)");
            if (i == nb) {
                snprintf(nb3ds, sizeof nb3ds, T("%d jeu(x)", "%d game(s)"), nb_titres);
                v = nb3ds;
            }
            if (i == c) ecran_rect(0, y - 1, 400, 16, RGB(0x24508C));
            ecran_texte(8, y, 0.48f, i == c ? BLANC : GRIS_C, 130, i < nb ? T(l[i].nom, l[i].nom_en) : i == nb ? T("Jeux 3DS", "3DS games") : T("Tester la connexion", "Test the connection"));
            ecran_texte(142, y, 0.48f, BLANC, 400 - 142 - 8, v);
            y += i == nb - 1 || i == 1 ? 24 : 17;
        }

        ecran_bas();
        nb_boutons = 0;
        y = 8;
        if (premier) y += ecran_paragraphe(8, y, 0.5f, C_JAUNE, 304, T("Premier lancement : entrez au moins le serveur et le jeton.", "First start: enter at least the server and the token.")) + 8;
        y += ecran_paragraphe(8, y, 0.5f, BLANC, 304, c < nb ? T(l[c].aide, l[c].aide_en)
                                                     : c == nb ? T("Jeux 3DS natifs à synchroniser. Par défaut : les 8 Pokémon 3DS.", "Native 3DS games to sync. Default: the 8 3DS Pokémon.")
                                                               : T("Test de l'adresse et du jeton du serveur.", "Test of the server address and token.")) + 8;
        if (test[0]) y += ecran_paragraphe(8, y, 0.5f, test_ok ? C_VERT : C_ROUGE, 304, test) + 6;
        dessiner_message(y, 240 - HAUTEUR_TOUCHE - 9);
        fond_barre(1);
        ligne_touches(t, 2, 240 - HAUTEUR_TOUCHE - 4, 18);
        ecran_finir();

        k = lire_touches();
        if (k & KEY_B) {
            if (!premier || test_ok || test[0]) break;
            message[0] = '\0';
            if ((test_ok = tester_connexion(test, sizeof test))) break;
            continue;
        }
        if (k & KEY_DOWN && c + 1 < total) c++;
        if (k & KEY_UP && c > 0) c--;
        if (k & KEY_A) {
            message[0] = test[0] = '\0';
            test_ok = 0;
            if (c < nb) {
                size_t len;
                editer(T(l[c].nom, l[c].nom_en), l[c].valeur, l[c].n, c == 1);
                len = strlen(l[c].valeur);
                while (len > 0 && l[c].valeur[len - 1] == '/') l[c].valeur[--len] = '\0'; /* "…/saves/" -> "…/saves" */
            } else if (c == nb) {
                choisir_titres();
            } else {
                test_ok = tester_connexion(test, sizeof test);
            }
        }
    }
    snprintf(message, sizeof message, "%s", premier ? test : ""); /* premier lancement : le résultat du test reste affiché */
    change = memcmp(avant, &cfg, sizeof cfg) || avant_nb != nb_titres || memcmp(avant_titres, titres, sizeof titres);
    if (memcmp(avant, &cfg, sizeof cfg)) ecrire_config();
    return change;
}

/* A sur un Jeu : son action, avec confirmation avant tout écrasement (Q5). 1 si une action a été tentée. */
static int agir(jeu_t *j)
{
    char q[500], srv[200], loc[120];
    int fait = 0;
    switch (j->etat) {
    case A_ENVOYER:
        envoyer(j);
        fait = 1;
        break;
    case A_RECUPERER:
        texte_serveur(j, 1, srv, sizeof srv);
        texte_local(j, loc, sizeof loc);
        snprintf(q, sizeof q, T("Récupérer la version du serveur ?\n\nServeur : %s\n3DS : %s%s", "Download the server version?\n\nServer: %s\n3DS: %s%s"), srv, loc,
                 j->a_local ? T("\n\nLa sauvegarde de la 3DS sera remplacée. Une copie de sécurité est faite avant.",
                                "\n\nThe 3DS save will be replaced. A backup copy is made first.") : "");
        if ((fait = confirmer(j->court, q, T("Récupérer", "Download")))) recuperer(j);
        break;
    case CONFLIT: {
        u32 k;
        const touche_t b = { KEY_B, T("Ne rien faire", "Do nothing"), 1 };
        texte_serveur(j, 1, srv, sizeof srv);
        texte_local(j, loc, sizeof loc);
        snprintf(q, sizeof q, T("%s\nCopie de la 3DS faite avant.", "%s\nBackup of the 3DS made first."), srv);
        snprintf(srv, sizeof srv, T("Cette console : %s\nEnvoyée ensuite au serveur.", "This console: %s\nThen uploaded to the server."), loc);
        do {
            float y = 6;
            ecran_debut();
            dessiner_haut(curseur_liste);
            ecran_bas();
            nb_boutons = 0;
            icone_jeu(j);
            ecran_texte(8, y, 0.6f, C_ROUGE, 250, T("Conflit", "Conflict"));
            y += 20;
            y += ecran_paragraphe(8, y, 0.5f, BLANC, 250, j->court);
            y = y < 58 ? 60 : y + 4;
            y += ecran_paragraphe(8, y, 0.45f, BLANC, 304, T("La 3DS et le serveur ont changé depuis la dernière synchronisation. Choisissez la version à garder :",
                                                            "The 3DS and the server both changed since the last sync. Choose the version to keep:")) + 6;
            carte(6, y, KEY_A, T("Garder la 3DS", "Keep the 3DS"), srv);
            carte(162, y, KEY_X, T("Prendre le serveur", "Take the server"), q);
            ecran_paragraphe(8, y + 96, 0.4f, GRIS_C, 304, T("L'autre version reste dans l'historique du serveur.", "The other version stays in the server history."));
            fond_barre(1);
            ligne_touches(&b, 1, 240 - HAUTEUR_TOUCHE - 4, 0);
            ecran_finir();
            k = lire_touches();
        } while (!(k & (KEY_A | KEY_X | KEY_B)));
        if (k & KEY_A) {
            fait = 1;
            envoyer(j);
        } else if (k & KEY_X) {
            fait = 1;
            recuperer(j);
        } else {
            snprintf(message, sizeof message, T("%s : conflit laissé tel quel.", "%s: conflict left as is."), j->court);
        }
        break;
    }
    case DOUBLON:
        if (j->partage) snprintf(message, sizeof message, T("%s : %d ROM ont ce nom de fichier et partageraient une sauvegarde. Renommez-en une.",
                                                            "%s: %d ROMs have this file name and would share one save. Rename one."), j->court, j->nb_roms);
        else snprintf(message, sizeof message, T("%s : %d ROM contiennent ce jeu. Gardez-en une seule, ou donnez un alias aux autres (alias.ini).", "%s: %d ROMs hold this game. Keep only one, or give the others an alias (alias.ini)."),
                      j->court, j->nb_roms);
        break;
    case HORS_LIGNE:
        snprintf(message, sizeof message, T("%s : serveur non lu. Appuyez sur \xEE\x80\x83 pour réessayer.", "%s: server not read. Press \xEE\x80\x83 to try again."), j->court);
        break;
    default:
        snprintf(message, sizeof message, T("%s : rien à faire, déjà à jour.", "%s: nothing to do, already in sync."), j->court);
    }
    if (fait) lire_local(j);
    return fait;
}

/* X sur un Jeu GB/GBA : sa Sauvegarde passe du forwarder mGBA à celle de la ROM (open_agb_firm, TWiLight), ou
 * l'inverse. Celle de la ROM est sav_rom, le fichier que lire_local lit sans forwarder (dossier des sauvegardes réglé,
 * ou saves/ du sous-dossier, ADR 0015). Le fichier du forwarder prime dès qu'il existe (lire_local) : le retirer
 * suffit à revenir côté ROM. Copie de sécurité du fichier remplacé (fichiers.h). */
static int basculer(jeu_t *j)
{
    char fwd[600], copie[700], date[20], q[700], action[64];
    const char *autre = j->systeme == SYS_GBA ? "open_agb_firm" : "TWiLight Menu++";
    int vers_rom;
    time_t t = time(NULL);

    if (j->systeme != SYS_GB && j->systeme != SYS_GBA) {
        snprintf(message, sizeof message, "%s", T("X : seulement pour les jeux GB et GBA.", "X: GB and GBA games only."));
        return 0;
    }
    snprintf(fwd, sizeof fwd, FORWARDERS "/%s.sav", j->nom);
    vers_rom = !strcmp(j->sav, fwd);
    snprintf(q, sizeof q,
             T("Sur la 3DS, ce jeu peut se jouer avec %s (sauvegarde à côté de la ROM), ou avec un forwarder mGBA "
               "(icône sur le menu HOME, sauvegarde dans sdmc:/mGBA). 3dssavesync synchronise une seule de ces deux sauvegardes.\n\n"
               "Aujourd'hui : %s\nPasser à : %s\n\nLa sauvegarde y sera copiée (copie de sécurité faite avant).%s",
               "On the 3DS, this game is played with %s (save next to the ROM), or with an mGBA forwarder "
               "(HOME menu icon, save in sdmc:/mGBA). 3dssavesync syncs only one of these two saves.\n\n"
               "Now: %s\nSwitch to: %s\n\nThe save will be copied there (backup copy made first).%s"),
             autre, vers_rom ? T("forwarder mGBA", "mGBA forwarder") : autre, vers_rom ? autre : T("forwarder mGBA", "mGBA forwarder"),
             vers_rom ? T(" Le forwarder n'aura plus de sauvegarde.", " The forwarder will have no save left.") : "");
    snprintf(action, sizeof action, T("Jouer avec %s", "Play with %s"), vers_rom ? autre : T("le forwarder", "the forwarder"));
    if (!confirmer(j->court, q, action)) return 0;
    strftime(date, sizeof date, "%Y%m%d-%H%M%S", localtime(&t));
    mkdir(DOSSIER "/copies", 0777);
    snprintf(copie, sizeof copie, DOSSIER "/copies/%s-%s.sav", j->nom, date);
    mkdir("sdmc:/mGBA", 0777);
    mkdir(FORWARDERS, 0777);
    creer_dossier_de(j->sav_rom);
    if (deplacer_sauvegarde(j->sav, vers_rom ? j->sav_rom : fwd, copie, vers_rom))
        snprintf(message, sizeof message, T("%s : changement impossible, rien n'est perdu.", "%s: switch failed, nothing lost."), j->court);
    else
        snprintf(message, sizeof message, T("%s : se joue maintenant avec %s.", "%s: now played with %s."), j->court, vers_rom ? autre : T("le forwarder mGBA", "the mGBA forwarder"));
    lire_local(j);
    return 1;
}

/* START : Envois et Récupérations ; ignore Conflits et Doublons ; une confirmation pour les écrasements. */
static int tout_synchroniser(void)
{
    char q[1200], premier[200] = "";
    int i, ecrase = 0, envois = 0, recups = 0, ignores = 0, echecs = 0, ok_ecraser = 1, total = 0, fait = 0;

    if (!serveur_lu) {
        snprintf(message, sizeof message, "%s", T("Serveur non lu : appuyez sur \xEE\x80\x83 pour réessayer.", "Server not read: press \xEE\x80\x83 to try again."));
        return 0;
    }

    snprintf(q, sizeof q, "%s", T("Ces sauvegardes de la 3DS seront remplacées par la version du serveur (copie de sécurité faite avant) :\n\n",
                                  "These 3DS saves will be replaced by the server version (backup copy made first):\n\n"));

    for (i = 0; i < nb_visibles; i++) {
        jeu_t *j = &jeux[visibles[i]];
        if (j->etat == A_RECUPERER && j->a_local && ecrase++ < 8) {
            strncat(q, j->court, 40);
            if (j->a_partie) {
                char t[80];
                texte_partie(&j->partie, t, sizeof t);
                snprintf(q + strlen(q), sizeof q - strlen(q), " (%s)", t);
            }
            strcat(q, "\n");
        }
    }
    if (ecrase > 8) snprintf(q + strlen(q), sizeof q - strlen(q), T("… et %d autre(s)\n", "… and %d more\n"), ecrase - 8);
    if (ecrase) ok_ecraser = confirmer(T("Tout synchroniser", "Sync all"), q, T("Remplacer", "Replace"));
    for (i = 0; i < nb_visibles; i++) {
        jeu_t *j = &jeux[visibles[i]];
        total += j->etat == A_ENVOYER || (j->etat == A_RECUPERER && (ok_ecraser || !j->a_local));
    }
    for (i = 0; i < nb_visibles; i++) {
        jeu_t *j = &jeux[visibles[i]];
        int faire_envoi = j->etat == A_ENVOYER;
        int faire_recup = j->etat == A_RECUPERER && (ok_ecraser || !j->a_local);
        if (faire_envoi || faire_recup)
            progression(++fait, total, "%s\n%s", faire_envoi ? T("Envoi", "Upload") : T("Récupération", "Download"), j->court);
        if (faire_envoi || faire_recup) {
            int ok = faire_envoi ? envoyer(j) : recuperer(j);
            envois += faire_envoi && ok;
            recups += faire_recup && ok;
            if (!ok && !echecs++) snprintf(premier, sizeof premier, "%.199s", message); /* « jeu : raison » */
            lire_local(j);
        } else if (j->etat == CONFLIT || j->etat == DOUBLON) ignores++;
    }
    /* Résumé, puis le premier échec : le message de la dernière action ne dit rien des autres. */
    snprintf(message, sizeof message, T("%d envoi(s), %d récupération(s)", "%d upload(s), %d download(s)"), envois, recups);
    if (echecs) snprintf(message + strlen(message), sizeof message - strlen(message), T(", %d échec(s)", ", %d failed"), echecs);
    if (ignores) snprintf(message + strlen(message), sizeof message - strlen(message), T(", %d ignoré(s)", ", %d skipped"), ignores);
    if (echecs) snprintf(message + strlen(message), sizeof message - strlen(message), "\n%s%s", echecs > 1 ? T("Premier échec : ", "First failure: ") : "", premier);
    return total > 0;
}

/* ---------------------------------------------------------------- boucle */

int main(void)
{
    int curseur = 0;

    gfxInitDefault();
    osSetSpeedupEnable(true); /* 804 MHz sur New 3DS (sans effet sur Old 3DS) : TLS et SHA-256 plus rapides */
    ecran_init();
    if (R_SUCCEEDED(cfguInit())) {
        CFGU_GetSystemLanguage(&langue);
        cfguExit();
    }
    anglais = langue != CFG_LANGUAGE_FR;
    if (migrer_dossier(ANCIEN, DOSSIER) == 1)
        snprintf(message, sizeof message, T("Réglages repris de %s.", "Settings taken from %s."), ANCIEN);
    snprintf(cfg.roms, sizeof cfg.roms, "sdmc:/roms/nds");
    snprintf(cfg.saves, sizeof cfg.saves, "sdmc:/roms/nds/saves");
    snprintf(cfg.roms_gb, sizeof cfg.roms_gb, "sdmc:/roms/gb");   /* TWiLight : .sav à côté des ROM */
    snprintf(cfg.roms_gba, sizeof cfg.roms_gba, "sdmc:/roms/gba"); /* open_agb_firm, useSavesFolder=false */
    lire_cles(DOSSIER "/config.ini", cle_config);
    lire_cles(DOSSIER "/base.ini", cle_base);
    nb_titres = titres_lire(DOSSIER "/titres.ini", titres, MAX_TITRES);

    if (net_init()) {
        const touche_t b = { KEY_B, T("Quitter", "Quit"), 1 };
        lu = 1;
        do {
            ecran_debut();
            ecran_haut();
            ecran_texte(6, 4, 0.55f, BLANC, 0, "3dssavesync");
            ecran_bas();
            nb_boutons = 0;
            ecran_paragraphe(16, 70, 0.55f, C_ROUGE, 288, T("Démarrage du réseau impossible. Redémarrez la 3DS, puis relancez 3dssavesync.", "Network start failed. Restart the 3DS, then start 3dssavesync again."));
            fond_barre(1);
            ligne_touches(&b, 1, 240 - HAUTEUR_TOUCHE - 4, 0);
            ecran_finir();
        } while (!(lire_touches() & (KEY_B | KEY_START)));
        ecran_fin();
        gfxExit();
        return 0;
    }

    if (!cfg.url[0] || !cfg.jeton[0]) {
        char reprise[512];
        snprintf(reprise, sizeof reprise, "%s", message);
        if (!cfg.url[0]) snprintf(cfg.url, sizeof cfg.url, "https://");
        reglages(1); /* message : résultat du test de la connexion */
        snprintf(message + strlen(message), sizeof message - strlen(message), "%s%.200s", reprise[0] && message[0] ? "\n" : "", reprise);
    }
    rafraichir();
    montrer_alerte();
    while (aptMainLoop()) {
        u32 k;
        curseur_liste = curseur;
        dessiner(curseur);
        k = lire_touches();

        if (k & KEY_B) break;
        if (k & (KEY_UP | KEY_DOWN)) message[0] = '\0'; /* le message parle du jeu qu'on quitte */
        if (k & KEY_DOWN && curseur + 1 < nb_visibles) curseur++;
        if (k & KEY_UP && curseur > 0) curseur--;
        if (k & (KEY_A | KEY_START | KEY_X | KEY_Y | KEY_SELECT)) {
            u64 debut = osGetTime();
            int choisi = nb_visibles ? visibles[curseur] : -1, i;
            message[0] = '\0';
            /* Tout relire : Y, ou réglages modifiés. Après une action : le Jeu touché est déjà relu, le serveur seul. */
            if (k & KEY_SELECT) {
                if (reglages(0)) rafraichir();
            } else if (k & KEY_Y) {
                rafraichir();
            } else if (k & KEY_A && nb_visibles) {
                if (agir(&jeux[choisi])) relire_serveur();
            } else if (k & KEY_X && nb_visibles) {
                if (basculer(&jeux[choisi])) relire_serveur();
            } else if (k & KEY_START) {
                if (tout_synchroniser()) relire_serveur();
                snprintf(message + strlen(message), sizeof message - strlen(message),
                         T(" (%llu s)", " (%llu s)"), (osGetTime() - debut) / 1000);
            }
            montrer_alerte();
            for (i = 0; i < nb_visibles && visibles[i] != choisi; i++);
            curseur = i < nb_visibles ? i : curseur < nb_visibles ? curseur : nb_visibles ? nb_visibles - 1 : 0;
            curseur_liste = curseur;
        }
    }
    net_fin();
    ecran_fin();
    gfxExit();
    return 0;
}
