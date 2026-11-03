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
#include "liste.h"
#include "maj.h"
#include "net.h"
#include "pokemon.h"
#include "reglages.h"
#include "texte.h"
#include "titres3ds.h"

#define DOSSIER "sdmc:/3ds/3dssavesync"
#define ANCIEN "sdmc:/3ds/nds-save-sync" /* nom avant la v3 : réglages et Bases repris au premier lancement */
#define FORWARDERS "sdmc:/mGBA/forwarders" /* Sauvegardes des forwarders mGBA */
#define MAX_JEUX 512 /* ponytail: tableau fixe ; au-delà de 512 ROM, les suivantes sont ignorées (et signalées) */
#define LIGNES_HAUT 13 /* lignes d'une liste sur l'écran du haut (pied en dessous) */
#define MASQUE -1   /* ni Sauvegarde locale ni Version courante : pas affiché */
#define DOUBLON 100 /* plusieurs ROM pour une Clé (ADR 0005) */
#define HORS_LIGNE 101 /* serveur non lu : état inconnu, aucune action proposée */
#define GITHUB "https://api.github.com/repos/programgames/3dssavesync/releases/latest" /* mises à jour (ADR 0021) */

static int anglais; /* langue de la console : français, sinon anglais */
static u8 langue;   /* CFG_LANGUAGE_* : titre des jeux 3DS dans cette langue */
#define T(fr, en) (anglais ? (en) : (fr))

/* Palette : un sens par couleur (le jaune ne dit que « À envoyer »). */
#define BLANC RGB(0xE8EAED)
#define GRIS_C RGB(0x8A929C)  /* libellé */
#define GRIS_F RGB(0x5A626C)  /* désactivé */
#define C_VERT RGB(0x4CC060)  /* À jour, réussite */
#define C_JAUNE RGB(0xF0C040) /* À envoyer */
#define C_CYAN RGB(0x40C8E8)  /* À récupérer, version disponible */
#define C_ROUGE RGB(0xF05050) /* Conflit, échec */
#define BARRE RGB(0x1E242C)   /* barre des touches, pied de l'écran du haut */
#define CARTE RGB(0x2A313B)   /* cartes du conflit, bande du message */
#define SEL RGB(0x263445)     /* ligne choisie */
#define ACCENT RGB(0x4A8FE0)  /* trait de la ligne choisie des réglages */

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
    int a_nom;      /* le serveur connaît le nom du jeu (ADR 0018) */
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

static struct { char url[256], jeton[256], roms[256], saves[256], roms_gb[256], saves_gb[256], roms_gba[256], saves_gba[256]; } cfg;
static titre_t titres[MAX_TITRES]; /* jeux 3DS synchronisés (titres.ini) */
static int nb_titres;
static jeu_t jeux[MAX_JEUX];
static int nb_jeux;
static struct { char cle[CLE_MAX], hash[65]; } bases[MAX_JEUX];
static int nb_bases;
static int visibles[MAX_JEUX], nb_visibles;
static char message[512];        /* message de la dernière action : bande au-dessus de la barre des touches */
static char message_detail[200]; /* sa ligne technique (HTTP, curl, chemin), en gris sous le texte */
static u32 message_couleur = BLANC; /* trait de la bande : rouge échec, vert réussite, blanc information */

static void dire(u32 couleur, const char *detail, const char *format, ...)
{
    va_list a;
    va_start(a, format);
    vsnprintf(message, sizeof message, format, a);
    va_end(a);
    message_couleur = couleur;
    snprintf(message_detail, sizeof message_detail, "%s", detail ? detail : "");
}

static void effacer(void)
{
    message[0] = message_detail[0] = '\0';
    message_couleur = BLANC;
}

/* Mise à jour (ADR 0021). VERSION vient du Makefile : "3.7.0" pour un tag, "dev" sinon (jamais de mise à jour). */
static long maj_jour = -1;   /* jour de la dernière vérification (maj.ini) */
static char maj_version[32]; /* dernière version vue sur GitHub (maj.ini) */
static char maj_installee[32]; /* version qui a tourné en dernier (maj.ini) : lue par le PC avant de copier un .cia */
static char maj_detail[400]; /* résultat de la dernière vérification ou installation : écran des réglages */
static int maj_echec;        /* maj_detail décrit un échec */
static const char *chemin_3dsx = ""; /* argv[0] : le .3dsx lancé depuis le Homebrew Launcher (lance_depuis_sd) */
static int quitter;          /* mise à jour installée : l'application se ferme */
static char alerte[300]; /* problème vu pendant une lecture ou une action : ajouté au message à la fin */
static char alerte_detail[200]; /* ligne technique de la lecture du serveur ratée */

static void alerter(const char *texte)
{
    if (!strstr(alerte, texte))
        snprintf(alerte + strlen(alerte), sizeof alerte - strlen(alerte), "%s%s", alerte[0] ? "\n" : "", texte);
}

static void montrer_alerte(void)
{
    if (!alerte[0]) return;
    snprintf(message + strlen(message), sizeof message - strlen(message), "%s%s", message[0] ? "\n" : "", alerte);
    message_couleur = C_ROUGE;
    if (!message_detail[0]) snprintf(message_detail, sizeof message_detail, "%s", alerte_detail);
    alerte[0] = alerte_detail[0] = '\0';
}

static int curseur_liste; /* jeu choisi dans la liste : l'écran du haut des dialogues et de la progression */
static int lu;            /* 0 tant que la première lecture n'est pas finie */
static int serveur_lu;    /* 0 si la dernière lecture du serveur a échoué : les jeux locaux restent listés */
static u64 lu_quand;      /* osGetTime() de la dernière lecture réussie du serveur */
static void progression(int fait, int total, const char *format, ...);
static void haut_liste(void);
static void (*haut_courant)(void) = haut_liste; /* écran du haut pendant une question ou une progression */
static const char *operation; /* titre de la longue opération en cours (Tout synchroniser), sinon NULL */
static u32 couleur_barre = C_VERT; /* barre de progression : jaune envoi, cyan récupération, vert le reste */

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
                char t[200];
                snprintf(t, sizeof t, T("Alias invalide pour « %.40s » : ROM non synchronisée. Préparez de nouveau la carte SD depuis le PC.",
                                        "Invalid alias for \"%.40s\": ROM not synced. Prepare the SD card again from the PC."), nom);
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
        if (nb_jeux % 10 == 0) progression(0, 0, T("Lecture des ROM (%d)…", "Reading ROMs (%d)…"), nb_jeux);
    }
    if (d) closedir(d);
}

/* Sauvegarde locale d'un Jeu : son fichier (forwarder mGBA s'il existe, docs/guide-forwarders.md) et son Hash
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
    char url[300], *corps;
    const char *p;
    entree_t e;
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
        detail_reseau(statut, url, alerte_detail, sizeof alerte_detail);
        if (statut > 0) free(corps);
        return -1;
    }
    serveur_lu = 1;
    lu_quand = osGetTime();
    /* [{"code": "IRBF", "version": "...", "hash": "...", "taille": 524288, "date": "...", "client": "pc"}, ...] (liste.h) */
    for (p = corps; liste_serveur(&p, &e);) {
        for (i = 0; i < nb_jeux && strcmp(jeux[i].cle, e.code); i++);
        if (i == nb_jeux || !e.hash[0] || e.taille < 0) continue; /* pas de ROM locale : pas affiché */
        strcpy(jeux[i].hash_serveur, e.hash);
        jeux[i].serveur.taille = e.taille;
        strcpy(jeux[i].date, e.date);
        strcpy(jeux[i].client, e.client);
        jeux[i].age = e.age;
        jeux[i].a_serveur = 1;
        jeux[i].a_nom = e.a_nom;
    }
    free(corps);
    /* Nom de chaque jeu pour l'Historique du PC (ADR 0018), une fois par jeu. Arrêt au premier refus : un serveur plus
     * ancien ne connaît pas cette adresse, et la liste reste lisible sans les noms. */
    for (i = 0; i < nb_jeux; i++) {
        if (!jeux[i].a_serveur || jeux[i].a_nom) continue;
        progression(i + 1, nb_jeux, T("Envoi des noms des jeux\n%s", "Sending game names\n%s"), jeux[i].court);
        snprintf(url, sizeof url, "%s/saves/%.31s/name", cfg.url, jeux[i].cle); /* Clé : 31 caractères au plus (CLE_MAX) */
        if (net_put(url, cfg.jeton, NULL, jeux[i].nom, strlen(jeux[i].nom)) != 204) break;
        jeux[i].a_nom = 1;
    }
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
        dire(C_ROUGE, NULL, T("%s : sauvegarde illisible. Vérifiez la carte SD.", "%s: can't read the save. Check the SD card."), j->court);
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
        dire(C_VERT, NULL, T("%s : envoyé au serveur.", "%s: uploaded."), j->court);
    } else if (statut == 412) {
        dire(C_ROUGE, NULL, T("%s : conflit, le serveur a changé entre-temps.", "%s: conflict, the server changed meanwhile."), j->court);
    } else {
        char d[200];
        detail_reseau(statut, url, d, sizeof d);
        dire(C_ROUGE, d, T("%s : envoi impossible. %s", "%s: upload failed. %s"), j->court, texte_reseau(statut, net_wifi(), anglais));
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
        char d[200];
        if (statut > 0) free(corps);
        detail_reseau(statut, url, d, sizeof d);
        dire(C_ROUGE, d, T("%s : récupération impossible. %s La sauvegarde de la 3DS reste intacte.", "%s: download failed. %s The 3DS save is intact."),
             j->court, texte_reseau(statut, net_wifi(), anglais));
        return 0;
    }
    hash_hex((unsigned char *)corps, taille, hash);
    if (strcmp(hash, j->hash_serveur)) {
        free(corps);
        dire(C_ROUGE, NULL, T("%s : version reçue abîmée, rien n'est écrit. Réessayez.", "%s: received version damaged, nothing written. Try again."), j->court);
        return 0;
    }
    /* Copie dès que la Sauvegarde existe, même si le scan n'a pas pu la lire : jamais d'écrasement sans copie. */
    if (existe) {
        strftime(date, sizeof date, "%Y%m%d-%H%M%S", localtime(&t));
        mkdir(DOSSIER "/copies", 0777);
        /* même nom de ROM dans deux sous-dossiers, la même seconde : -1, -2… (jamais d'écrasement d'une copie) */
        echec = chemin_copie(DOSSIER "/copies", j->nom, date, j->systeme == SYS_3DS ? "main" : "sav", copie, sizeof copie);
        if (!echec) echec = lire_sauvegarde(j, &ancien, &n);
        if (!echec) echec = ecrire_relu(copie, ancien, n); /* relue : une copie abîmée ne protège rien */
        if (echec) {
            free(ancien);
            free(corps);
            dire(C_ROUGE, copie, T("%s : copie de sécurité impossible, rien n'est écrit. Vérifiez la carte SD.", "%s: backup copy failed, nothing written. Check the SD card."), j->court);
            return 0;
        }
    }

    if (j->systeme == SYS_3DS) {
        echec = titre_ecrire(j->titre, j->media, corps, taille);
        free(ancien);
        free(corps);
        if (echec) {
            dire(C_ROUGE, NULL, T("%s : écriture 3DS refusée, sauvegarde intacte.", "%s: 3DS write refused, save intact."), j->court);
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
            dire(C_ROUGE, NULL, T("%s : mémoire insuffisante, rien n'est écrit.", "%s: out of memory, nothing written."), j->court);
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
            dire(C_ROUGE, j->sav, T("%s : écriture impossible, sauvegarde de la 3DS intacte.", "%s: write failed, 3DS save intact."), j->court);
            return 0;
        }
        snprintf(tmp, sizeof tmp, "%s.tmp", j->sav);
        if (echec && access(tmp, F_OK)) { /* renommée, mais relue différente : pas de Base sur ce fichier */
            dire(C_ROUGE, j->sav, T("%s : sauvegarde écrite abîmée. Vérifiez la carte SD, puis récupérez de nouveau.%s", "%s: written save damaged. Check the SD card, then download again.%s"),
                 j->court, existe ? T(" Ancienne copiée dans /3ds/3dssavesync/copies.", " Old one copied to /3ds/3dssavesync/copies.") : "");
            return 0;
        }
        if (echec) {
            const char *b = strrchr(j->sav, '/') ? strrchr(j->sav, '/') + 1 : j->sav, *ext = strrchr(b, '.');
            int l = (int)strlen(b), coupe = l > 60; /* écran du bas : nom raccourci, extension gardée */
            dire(C_ROUGE, NULL, T("%s : renommage impossible. Sauvegarde reçue : %.*s%s%s.tmp%s", "%s: rename failed. Downloaded save: %.*s%s%s.tmp%s"),
                     j->court, coupe ? 50 : l, b, coupe ? "…" : "", coupe && ext ? ext : "",
                     existe ? T(". Ancienne copiée dans /3ds/3dssavesync/copies.", ". Old one copied to /3ds/3dssavesync/copies.") : ".");
            return 0;
        }
    }
    fixer_base(j->cle, j->hash_serveur);
    dire(C_VERT, NULL, T("%s : récupéré du serveur.", "%s: downloaded."), j->court);
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

/* « 152 h 30 · Sacha », ou partie non commencée. */
static void texte_partie(const partie_t *p, char *out, size_t n)
{
    if (p->dresseur[0]) snprintf(out, n, "%d h %02d \xC2\xB7 %s", p->heures, p->minutes, p->dresseur);
    else snprintf(out, n, T("%d h %02d \xC2\xB7 partie non commencée", "%d h %02d \xC2\xB7 not started yet"), p->heures, p->minutes);
}

/* Sauvegarde de la 3DS : temps de jeu (Pokémon), sinon date du fichier si l'horloge la rend crédible. */
static void texte_local(const jeu_t *j, char *out, size_t n)
{
    time_t maintenant = time(NULL);
    if (!j->a_local) snprintf(out, n, "%s", T("Aucune sauvegarde", "No save"));
    else if (j->vide) snprintf(out, n, "%s", T("Vierge", "Blank"));
    else if (j->a_partie) texte_partie(&j->partie, out, n);
    else if (j->modif && maintenant >= j->modif) {
        char age[24];
        format_age((long)(maintenant - j->modif), anglais, 0, age, sizeof age);
        snprintf(out, n, T("Modifiée %s", "Modified %s"), age);
    } else snprintf(out, n, "%s", T("Présente, date inconnue", "Found, date unknown"));
}

/* Version du serveur : quand, puis depuis quel appareil. */
static void texte_serveur(const jeu_t *j, char *out, size_t n)
{
    char age[24];
    if (!j->a_serveur) {
        snprintf(out, n, "%s", serveur_lu ? T("Aucune version", "No version") : T("Non lu", "Not read"));
        return;
    }
    format_age(j->age, anglais, 1, age, sizeof age);
    if (age[0]) snprintf(out, n, T("%s \xC2\xB7 depuis %s", "%s \xC2\xB7 from %s"), age, j->client[0] ? j->client : "?");
    else snprintf(out, n, T("Depuis %s", "From %s"), j->client[0] ? j->client : "?");
}

/* Temps de jeu de la version du serveur (téléchargée), seulement si la 3DS a un Pokémon reconnu. 1 si lu. */
static int partie_serveur(const jeu_t *j, partie_t *p)
{
    char url[300], *corps;
    size_t taille;
    long statut;
    int ok;
    if (!j->a_serveur || !j->a_partie) return 0;
    progression(0, 0, "%s", T("Lecture de la version du serveur…", "Reading the server version…"));
    snprintf(url, sizeof url, "%s/saves/%s", cfg.url, j->cle);
    statut = net_get(url, cfg.jeton, &corps, &taille);
    ok = statut == 200 && pokemon_partie((unsigned char *)corps, taille, p) == 0;
    if (statut > 0) free(corps);
    return ok;
}

/* ---------------------------------------------------------------- écran principal (citro2d, issue #21) */

static u32 couleur_etat(int e)
{
    switch (e) {
    case A_ENVOYER: return C_JAUNE;
    case A_RECUPERER: return C_CYAN;
    case CONFLIT: return C_ROUGE;
    }
    return GRIS_C; /* À jour, Doublon, Inconnu */
}

/* Pastille et trait de l'état : vert sombre pour À jour, qui n'attire pas l'œil. */
static u32 couleur_pastille(int e) { return e == A_JOUR ? RGB(0x2F6B3A) : couleur_etat(e); }

/* Un mot par état, le même dans la liste, les compteurs et les détails. */
static const char *etat_court(int e)
{
    switch (e) {
    case A_ENVOYER: return T("À envoyer", "To upload");
    case A_RECUPERER: return T("À récupérer", "To download");
    case CONFLIT: return T("Conflit", "Conflict");
    case DOUBLON: return T("Doublon", "Duplicate");
    case HORS_LIGNE: return T("Inconnu", "Unknown");
    }
    return T("À jour", "In sync");
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
#define FOND_BARRE_1 (240 - HAUTEUR_TOUCHE - 7) /* haut du fond d'une barre d'une ligne de touches */
#define FOND_BARRE_2 (240 - 2 * HAUTEUR_TOUCHE - 7)

/* Glyphe de la police du système (U+E000 A, E001 B, E002 X, E003 Y) à la couleur de la New 3DS, ou START / SELECT. */
static const char *glyphe(u32 t)
{
    return t == KEY_A ? "\xEE\x80\x80" : t == KEY_B ? "\xEE\x80\x81" : t == KEY_X ? "\xEE\x80\x82" : t == KEY_Y ? "\xEE\x80\x83" : t == KEY_START ? "START" : "SELECT";
}

/* START et SELECT : le mot dans une pastille, 6 pixels de plus. */
static float largeur_glyphe(u32 t)
{
    return t & (KEY_START | KEY_SELECT) ? ecran_largeur(0.34f, glyphe(t)) + 6 : ecran_largeur(0.55f, glyphe(t));
}

static float largeur_touche(const touche_t *t, float taille) { return largeur_glyphe(t->touche) + 4 + ecran_largeur(taille, t->texte); }

/* Une ligne de touches à partir de y ; ecart = 0 : touches réparties sur toute la largeur.
 * Texte réduit si la ligne ne tient pas (marge comprise) avec au moins 10 pixels entre deux touches. */
static void ligne_touches(const touche_t *t, int n, float y, float ecart)
{
    float l[6], total, x = 6, taille = 0.44f;
    int i;
    do {
        taille -= 0.02f;
        for (total = 0, i = 0; i < n; i++) total += l[i] = largeur_touche(&t[i], taille);
    } while (taille > 0.34f && total * MARGE_LARGEUR + 10 * (n - 1) > 308);
    if (!ecart) ecart = n > 1 ? (308 - total) / (n - 1) : 0;
    for (i = 0; i < n; i++) {
        u32 c = !t[i].actif ? GRIS_F : t[i].touche == KEY_A ? RGB(0xE04848) : t[i].touche == KEY_B ? RGB(0xF0C030)
              : t[i].touche == KEY_X ? RGB(0x4080F0) : t[i].touche == KEY_Y ? RGB(0x40B060) : GRIS_C;
        float g = largeur_glyphe(t[i].touche);
        if (t[i].touche & (KEY_START | KEY_SELECT)) { /* lu comme une touche, pas comme du texte */
            ecran_rect(x, y + 4, g, 12, GRIS_F);
            ecran_texte(x + 3, y + 4, 0.34f, BLANC, 0, glyphe(t[i].touche));
        } else ecran_texte(x, y + 2, 0.55f, c, 0, glyphe(t[i].touche));
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

/* Carte de choix de l'écran de Conflit (152 × 92), touchable : touche et action, la version (même forme des deux
 * côtés), ce qui se passe. */
static void carte(float x, float y, u32 touche, const char *action, const char *version, const char *suite)
{
    ecran_rect(x, y, 152, 92, CARTE);
    ecran_texte(x + 6, y + 4, 0.55f, touche == KEY_A ? RGB(0xE04848) : RGB(0x4080F0), 0, glyphe(touche));
    ecran_texte(x + 24, y + 5, 0.45f, BLANC, 122, action);
    ecran_lignes(x + 6, y + 26, 0.42f, BLANC, 140, 3, version);
    ecran_lignes(x + 6, y + 68, 0.38f, GRIS_C, 140, 2, suite);
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
    ecran_rect(0, y, 320, 240 - y, BARRE);
}

/* Icône du jeu en haut à droite de l'écran du bas. */
static void icone_jeu(const jeu_t *j)
{
    u16 icone[48 * 48];
    if (j->systeme == SYS_3DS ? titre_icone(j->titre, j->media, icone) == 0 : icone_ds(j, icone) == 0)
        ecran_icone(icone, j->systeme == SYS_3DS ? 48 : 32, j->systeme == SYS_3DS ? 48 : 32, 320 - 6 - 48, 6, j->systeme == SYS_3DS ? 1.0f : 1.5f);
}

/* Écran du haut de la liste : titre et compteurs (ceux qui ne tiennent pas partent, « à jour » d'abord), les jeux, puis
 * le pied : le serveur, quand il a été lu, la position dans la liste. */
static void haut_liste(void)
{
    static const int etats[] = { CONFLIT, A_RECUPERER, A_ENVOYER, DOUBLON, A_JOUR }; /* rang de priorite() */
    static const int tous[] = { A_ENVOYER, A_RECUPERER, CONFLIT, DOUBLON, HORS_LIGNE, A_JOUR };
    const int curseur = curseur_liste;
    int i, nb = 0, e[5], n[5] = { 0 }, debut = curseur < LIGNES_HAUT ? 0 : curseur - LIGNES_HAUT + 1;
    char t[5][40], pied[200], age[32], pos[24];
    const char *hote = strstr(cfg.url, "://") ? strstr(cfg.url, "://") + 3 : cfg.url;
    float l[5], x, xs = 0, xn;

    ecran_haut();
    x = 6 + ecran_texte(6, 4, 0.55f, BLANC, 0, "3dssavesync") + 12;
    if (lu && serveur_lu) { /* sans le serveur, les compteurs seraient faux */
        for (i = 0; i < nb_visibles; i++) n[priorite(jeux[visibles[i]].etat)]++;
        for (i = 0; i < 5; i++) {
            if (!n[i]) continue;
            e[nb] = etats[i];
            snprintf(t[nb], sizeof t[nb], e[nb] == CONFLIT ? pluriel(n[i], anglais, T("%d conflit", "%d conflict"), T("%d conflits", "%d conflicts"))
                                          : e[nb] == A_RECUPERER ? T("%d à récupérer", "%d to download")
                                          : e[nb] == A_ENVOYER ? T("%d à envoyer", "%d to upload")
                                          : e[nb] == DOUBLON ? pluriel(n[i], anglais, T("%d doublon", "%d duplicate"), T("%d doublons", "%d duplicates"))
                                                             : T("%d à jour", "%d in sync"), n[i]);
            l[nb] = ecran_largeur(0.42f, t[nb]);
            nb++;
        }
        nb = textes_tenant(l, nb, 12, 400 - 6 - x);
        for (x = 400 - 6, i = nb - 1; i >= 0; i--) {
            x -= l[i];
            ecran_texte(x, 7, 0.42f, e[i] == A_JOUR ? C_VERT : couleur_etat(e[i]), 0, t[i]);
            x -= 12;
        }
    }
    ecran_rect(0, 23, 400, 1, GRIS_F);

    /* Colonnes mesurées sur tous les mots d'état : rien n'est coupé, et elles ne bougent pas d'une image à l'autre. */
    for (i = 0; i < 6; i++)
        if (ecran_largeur(0.42f, etat_court(tous[i])) > xs) xs = ecran_largeur(0.42f, etat_court(tous[i]));
    xs = 18 + xs * MARGE_LARGEUR + 6;
    xn = xs + ecran_largeur(0.42f, "GBA") * MARGE_LARGEUR + 8;
    if (!nb_visibles) ecran_paragraphe(10, 40, 0.5f, GRIS_C, 380, !lu ? T("Lecture en cours…", "Reading…")
                                                              : serveur_lu ? T("Aucun jeu à afficher.", "No game to show.")
                                                              : T("Serveur non lu, et aucune sauvegarde sur la 3DS.", "Server not read, and no save on the 3DS."));
    for (i = debut; i < nb_visibles && i < debut + LIGNES_HAUT; i++) {
        jeu_t *k = &jeux[visibles[i]];
        float y = 27 + (i - debut) * 15;
        if (i == curseur) {
            ecran_rect(0, y - 1, 400, 15, SEL);
            ecran_rect(0, y - 1, 2, 15, couleur_pastille(k->etat));
        }
        ecran_rect(8, y + 5, 5, 5, couleur_pastille(k->etat));
        ecran_texte(18, y + 1, 0.42f, couleur_etat(k->etat), 0, etat_court(k->etat));
        ecran_texte(xs, y + 1, 0.42f, GRIS_C, 0, NOMS_SYSTEMES[k->systeme]);
        ecran_texte(xn, y, 0.5f, BLANC, 400 - xn - 8, k->court);
    }
    if (nb_visibles > LIGNES_HAUT) { /* barre de défilement */
        float h = 195.0f * LIGNES_HAUT / nb_visibles;
        ecran_rect(396, 26 + (195.0f - h) * debut / (nb_visibles - LIGNES_HAUT), 2, h, GRIS_F);
    }

    ecran_rect(0, 224, 400, 16, BARRE);
    if (!lu) snprintf(pied, sizeof pied, T("Serveur %.*s", "Server %.*s"), (int)strcspn(hote, "/"), hote);
    else if (serveur_lu) {
        format_age((long)((osGetTime() - lu_quand) / 1000), anglais, 0, age, sizeof age);
        snprintf(pied, sizeof pied, T("Serveur %.*s \xC2\xB7 lu %s", "Server %.*s \xC2\xB7 read %s"), (int)strcspn(hote, "/"), hote, age);
    } else {
        snprintf(pied, sizeof pied, T("Serveur %.*s \xC2\xB7 non lu \xC2\xB7 \xEE\x80\x83 Réessayer", "Server %.*s \xC2\xB7 not read \xC2\xB7 \xEE\x80\x83 Try again"),
                 (int)strcspn(hote, "/"), hote);
    }
    pos[0] = '\0';
    if (nb_visibles) snprintf(pos, sizeof pos, "%d / %d", curseur + 1, nb_visibles);
    x = 400 - 6 - ecran_largeur(0.38f, pos);
    ecran_texte(x, 226, 0.38f, GRIS_C, 0, pos);
    ecran_texte(8, 226, 0.38f, lu && !serveur_lu ? C_ROUGE : GRIS_C, x - 8 - 12, pied);
}

/* Une ligne « libellé  valeur » des détails. */
static float detail(float y, const char *libelle, const char *valeur)
{
    ecran_texte(8, y, 0.42f, GRIS_C, 100, libelle);
    ecran_texte(112, y, 0.42f, BLANC, 200, valeur);
    return y + 15;
}

/* Sous le mot d'état : pourquoi, et ce que fait A. */
static const char *phrase_etat(const jeu_t *j, char *t, size_t n)
{
    switch (j->etat) {
    case A_ENVOYER: return T("La sauvegarde de la 3DS est plus récente. \xEE\x80\x80 l'envoie au serveur.", "The 3DS save is newer. \xEE\x80\x80 uploads it to the server.");
    case A_RECUPERER: return T("Le serveur a une version plus récente. \xEE\x80\x80 la récupère, après une copie de sécurité.",
                               "The server has a newer version. \xEE\x80\x80 downloads it, after a backup.");
    case CONFLIT: return T("La sauvegarde a changé sur la 3DS et sur le serveur. \xEE\x80\x80 pour choisir laquelle garder.",
                           "The save changed on the 3DS and on the server. \xEE\x80\x80 to choose which one to keep.");
    case DOUBLON:
        if (j->partage) snprintf(t, n, T("%d ROM ont ce nom de fichier et partageraient la même sauvegarde. Renommez-en une.",
                                         "%d ROMs have this file name and would share one save. Rename one."), j->nb_roms);
        else snprintf(t, n, T("%d ROM contiennent ce jeu. Gardez-en une, ou ajoutez les autres aux ROM hacks sur le PC, puis préparez la carte SD.",
                              "%d ROMs hold this game. Keep one, or add the others to ROM hacks on the PC, then prepare the SD card."), j->nb_roms);
        return t;
    case HORS_LIGNE: return T("Serveur non lu : état inconnu. \xEE\x80\x83 relit le serveur.", "Server not read: state unknown. \xEE\x80\x83 reads the server again.");
    }
    return T("La 3DS et le serveur ont la même sauvegarde.", "The 3DS and the server have the same save.");
}

/* Bande d'un message, collée au-dessus de la barre des touches (bas) : trait de couleur, texte, puis la ligne technique
 * en gris, toutes deux sur plusieurs lignes. Elle monte aussi haut que le texte le demande, par-dessus les détails :
 * le message reste entier (un peu plus petit s'il le faut ; « … » seulement s'il dépasse encore l'écran). */
static void bande(float bas, const char *texte, const char *technique, u32 couleur)
{
    float taille, l1, l2 = technique && technique[0] ? ecran_hauteur(0.38f, 302, technique) : 0, h;
    int lignes, place;
    if (!texte[0]) return;
    for (taille = 0.42f;; taille = 0.36f) {
        l1 = ecran_hauteur(taille, 302, "X");
        lignes = (int)(ecran_hauteur(taille, 302, texte) / l1 + 0.5f);
        place = (int)((bas - 12 - l2) / l1); /* jusqu'à 6 px du haut de l'écran */
        if (lignes <= place || taille < 0.4f) break;
    }
    if (lignes > place) lignes = place < 1 ? 1 : place;
    h = lignes * l1 + l2 + 6;
    ecran_rect(0, bas - h, 320, h, CARTE);
    ecran_rect(0, bas - h, 3, h, couleur);
    ecran_lignes(10, bas - h + 3, taille, BLANC, 302, lignes, texte);
    if (l2) ecran_paragraphe(10, bas - h + 3 + lignes * l1, 0.38f, GRIS_C, 302, technique);
}

/* Message de la dernière action, dans sa bande. */
static void dessiner_message(float bas) { bande(bas, message, message_detail, message_couleur); }

static void dessiner_bas(const jeu_t *j)
{
    char t[600];
    float y = 6;
    int action = 0, gb = 0;

    ecran_bas();
    nb_boutons = 0;
    if (j) {
        const char *b = base_de(j->cle), *ext = strrchr(j->rom, '.');
        u32 c = j->etat == A_JOUR ? C_VERT : couleur_etat(j->etat);
        gb = j->systeme == SYS_GB || j->systeme == SYS_GBA;
        action = j->etat == A_ENVOYER || j->etat == A_RECUPERER || j->etat == CONFLIT;
        icone_jeu(j);
        y += ecran_lignes(8, y, 0.6f, BLANC, 250, 2, j->court);
        /* Système, et pour GB/GBA le programme dont la sauvegarde est synchronisée (X le change). */
        snprintf(t, sizeof t, "%s%s%s", j->systeme == SYS_DS ? "Nintendo DS" : j->systeme == SYS_GB ? (ext && !strcasecmp(ext, ".gbc") ? "Game Boy Color" : "Game Boy")
                                       : j->systeme == SYS_GBA ? "Game Boy Advance" : "Nintendo 3DS",
                 gb ? " \xC2\xB7 " : "", !gb ? "" : forwarder(j) ? T("forwarder mGBA", "mGBA forwarder") : j->systeme == SYS_GBA ? "open_agb_firm" : "TWiLight Menu++");
        ecran_texte(8, y + 1, 0.4f, GRIS_C, 250, t);
        y = y + 18 < 58 ? 58 : y + 20; /* sous l'icône */
        ecran_rect(8, y + 2, 3, 13, c);
        ecran_texte(15, y, 0.5f, c, 0, etat_court(j->etat));
        y += 19;
        y += ecran_lignes(8, y, 0.42f, BLANC, 304, j->etat == DOUBLON ? 4 : 2, phrase_etat(j, t, sizeof t)) + 5;
        if (j->etat != DOUBLON) { /* un Doublon n'a ni sauvegarde ni version à montrer */
            texte_local(j, t, sizeof t);
            y = detail(y, T("Sur la 3DS", "On the 3DS"), t);
            texte_serveur(j, t, sizeof t);
            y = detail(y, T("Sur le serveur", "On the server"), t);
            y = detail(y, T("Dernière synchro", "Last sync"),
                       !b ? T("Jamais", "Never")
                       : !strcmp(b, j->hash_local) && !strcmp(b, j->hash_serveur) ? T("Même version que les deux", "Same as both")
                       : !strcmp(b, j->hash_local) ? T("Même version que la 3DS", "Same as the 3DS")
                       : !strcmp(b, j->hash_serveur) ? T("Même version que le serveur", "Same as the server") : T("Version différente des deux", "Differs from both"));
        }
    }
    if (message[0]) dessiner_message(FOND_BARRE_2);
    else if (maj_plus_recente(VERSION, maj_version)) { /* sans message : la mise à jour, à la même place */
        snprintf(t, sizeof t, T("Version %s disponible (SELECT)", "Version %s available (SELECT)"), maj_version);
        bande(FOND_BARRE_2, t, NULL, C_CYAN);
    }

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
        if (gb) snprintf(x, sizeof x, T("Jouer avec %s", "Play with %s"), forwarder(j) ? (j->systeme == SYS_GBA ? "open_agb_firm" : "TWiLight Menu++")
                                                                                      : T("le forwarder mGBA", "the mGBA forwarder"));
        ligne_touches(jeu, gb ? 2 : 1, HAUT_BARRE, 18);
        ligne_touches(generales, 4, HAUT_BARRE + HAUTEUR_TOUCHE, 0);
    }
}

static void dessiner(void)
{
    ecran_debut();
    haut_liste();
    dessiner_bas(nb_visibles ? &jeux[visibles[curseur_liste]] : NULL);
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

/* Progression des longues boucles : l'écran du haut garde son contexte (haut_courant) ; en bas, l'opération, le texte
 * (« étape\njeu » : l'étape en gris, puis le jeu) et une barre si total > 0.
 * Au plus 20 images par seconde : une image attend l'écran, la lecture ne doit pas en être ralentie. */
static void progression(int fait, int total, const char *format, ...)
{
    static u64 derniere;
    char t[300], *nl;
    float y = 40;
    va_list a;
    u64 maintenant = osGetTime();
    if (total && fait > 1 && fait < total && maintenant - derniere < 50) return;
    derniere = maintenant;
    va_start(a, format);
    vsnprintf(t, sizeof t, format, a);
    va_end(a);
    ecran_debut();
    haut_courant();
    ecran_bas();
    if (operation) {
        ecran_texte(16, y, 0.55f, BLANC, 288, operation);
        y += 22;
    }
    if ((nl = strchr(t, '\n'))) {
        *nl = '\0';
        ecran_texte(16, y, 0.45f, GRIS_C, 288, t);
        ecran_lignes(16, y + 18, 0.55f, BLANC, 288, 2, nl + 1);
    } else ecran_lignes(16, y, 0.55f, BLANC, 288, 3, t);
    if (total > 0) {
        char n[32];
        ecran_rect(16, 140, 288, 10, CARTE);
        ecran_rect(16, 140, 288.0f * fait / total, 10, couleur_barre);
        snprintf(n, sizeof n, "%d / %d", fait, total);
        ecran_texte(16, 156, 0.45f, GRIS_C, 0, n);
        snprintf(n, sizeof n, T("%d %%", "%d%%"), (int)(100LL * fait / total));
        ecran_texte(304 - ecran_largeur(0.45f, n), 156, 0.45f, GRIS_C, 0, n);
    }
    ecran_finir();
}

/* Question sur l'écran du bas (l'écran du haut garde son contexte) : le titre est la question, le sous-titre le jeu ;
 * dans le texte, une ligne « libellé\tvaleur » se range dans la grille des détails ; l'avertissement va dans une bande
 * rouge. 1 si A, l'action nommée ; 0 si B, Annuler. */
static int confirmer(const char *titre, const char *sous_titre, const char *texte, const char *avertissement, const char *action)
{
    const touche_t t[2] = { { KEY_A, action, 1 }, { KEY_B, T("Annuler", "Cancel"), 1 } };
    for (;;) {
        char ligne[300], *tab;
        const char *s = texte;
        float y = 6;
        u32 k;
        ecran_debut();
        haut_courant();
        ecran_bas();
        nb_boutons = 0;
        y += ecran_lignes(8, y, 0.55f, BLANC, 304, 2, titre) + 2;
        if (sous_titre) y += ecran_lignes(8, y, 0.45f, GRIS_C, 304, 1, sous_titre);
        y += 8;
        while (*s) {
            size_t n = strcspn(s, "\n");
            snprintf(ligne, sizeof ligne, "%.*s", (int)n, s);
            if ((tab = strchr(ligne, '\t'))) {
                *tab = '\0';
                y = detail(y, ligne, tab + 1);
            } else y += ligne[0] ? ecran_paragraphe(8, y, 0.45f, BLANC, 304, ligne) : 6;
            s += n + (s[n] == '\n');
        }
        if (avertissement) bande(FOND_BARRE_1, avertissement, NULL, C_ROUGE);
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

static int test_etat;        /* ligne Connexion des réglages : 0 non testée, 1 réussie, -1 échec (test_court) */
static char test_court[80];  /* « Jeton refusé » : texte_reseau jusqu'au premier « : », « . » ou « ( » */

/* GET /saves : résultat dans le message (texte, puis la ligne technique) et la ligne Connexion. 1 si la connexion marche. */
static int tester_connexion(void)
{
    char url[300], d[200], *corps;
    const char *t;
    size_t taille;
    long statut;
    progression(0, 0, "%s", T("Test de la connexion…", "Testing the connection…"));
    snprintf(url, sizeof url, "%s/saves", cfg.url);
    statut = net_get(url, cfg.jeton, &corps, &taille);
    if (statut > 0) free(corps);
    if (statut == 200) {
        dire(C_VERT, NULL, "%s", T("Connexion réussie.", "Connected."));
        test_etat = 1;
        return 1;
    }
    t = texte_reseau(statut, net_wifi(), anglais);
    detail_reseau(statut, url, d, sizeof d);
    dire(C_ROUGE, d, "%s", t);
    snprintf(test_court, sizeof test_court, "%.*s", (int)strcspn(t, ":.("), t);
    if (strlen(test_court) && test_court[strlen(test_court) - 1] == ' ') test_court[strlen(test_court) - 1] = '\0';
    test_etat = -1;
    return 0;
}

/* ---------------------------------------------------------------- mise à jour (ADR 0021) */

static void cle_maj(const char *k, const char *v)
{
    if (!strcmp(k, "jour")) maj_jour = strtol(v, NULL, 10);
    else if (!strcmp(k, "version")) snprintf(maj_version, sizeof maj_version, "%s", v);
    else if (!strcmp(k, "installee")) snprintf(maj_installee, sizeof maj_installee, "%s", v);
}

/* maj.ini : jour de la vérification, dernière version vue, version en marche. Échec : réécrit au prochain lancement. */
static void ecrire_maj_ini(void)
{
    char t[120];
    snprintf(t, sizeof t, "jour=%ld\nversion=%s\ninstallee=%s\n", maj_jour, maj_version, VERSION);
    mkdir(DOSSIER, 0777);
    remplacer_fichier(DOSSIER "/maj.ini", t, strlen(t));
}

/* Échec d'une requête à GitHub : ce qui n'est pas fait, le texte de texte_github, puis le code curl ou HTTP. */
static void echec_github(const char *quoi, long statut)
{
    maj_echec = 1;
    snprintf(maj_detail, sizeof maj_detail, "%s%s%s\n%s %ld", quoi, quoi[0] ? " " : "", texte_github(statut, net_wifi(), anglais),
             statut < 0 ? "curl" : "HTTP", statut < 0 ? -statut : statut);
}

/* GET /releases/latest : r, maj_version, maj_detail et maj.ini à jour. 1 si une version plus récente existe. */
static int verifier_maj(release_t *r)
{
    char *corps;
    size_t taille;
    long statut;
    int ok;

    progression(0, 0, "%s", T("Recherche d'une mise à jour…", "Looking for an update…"));
    statut = net_github(GITHUB, NULL, &corps, &taille);
    ok = statut == 200 && maj_release(corps, r) == 0;
    if (statut > 0) free(corps);
    if (ok) snprintf(maj_version, sizeof maj_version, "%s", r->version);
    maj_jour = (long)(time(NULL) / 86400);
    ecrire_maj_ini(); /* échec : vérifié de nouveau au prochain lancement */
    if (!ok) {
        echec_github("", statut);
        return 0;
    }
    maj_echec = 0;
    if (maj_plus_recente(VERSION, r->version)) {
        snprintf(maj_detail, sizeof maj_detail, T("Version %s disponible.", "Version %s available."), r->version);
        return 1;
    }
    /* Jamais la version de GitHub quand elle n'est pas plus récente : elle ne dit rien d'utile et ressemble à un retard. */
    snprintf(maj_detail, sizeof maj_detail, "%s", !strcmp(VERSION, "dev") ? T("Version de développement : aucune mise à jour proposée.", "Development build: no update offered.")
                                                                           : T("3dssavesync est à jour.", "3dssavesync is up to date."));
    return 0;
}

static char version_dl[32]; /* version en cours de téléchargement ou d'installation */

static void avance_dl(size_t fait, size_t total)
{
    progression((int)(fait / 1024), (int)(total / 1024), T("Téléchargement de la version %s (Ko)", "Downloading version %s (KB)"), version_dl);
}

/* .cia par AM, comme FBI : sur tout échec, l'installation est annulée et l'ancienne version reste. */
static Result installer_cia(const char *octets, size_t taille)
{
    Handle h;
    Result r;
    u32 ecrit = 0;
    size_t fait;

    if (R_FAILED(r = amInit())) return r;
    if (R_SUCCEEDED(r = AM_StartCiaInstall(MEDIATYPE_SD, &h))) {
        for (fait = 0; fait < taille && R_SUCCEEDED(r); fait += ecrit) {
            progression((int)(fait / 1024), (int)(taille / 1024), T("Installation de la version %s (Ko)", "Installing version %s (KB)"), version_dl);
            r = FSFILE_Write(h, &ecrit, fait, octets + fait, taille - fait < 0x20000 ? taille - fait : 0x20000, 0);
            if (R_SUCCEEDED(r) && !ecrit) r = MAKERESULT(RL_PERMANENT, RS_INTERNAL, RM_APPLICATION, RD_NO_DATA);
        }
        if (R_SUCCEEDED(r)) r = AM_FinishCiaInstall(h);
        else AM_CancelCIAInstall(h);
    }
    amExit();
    return r;
}

/* Télécharge r, vérifie son SHA-256 dans SHA256SUMS, puis l'installe : .cia par AM, ou .3dsx remplacé s'il a été lancé
 * depuis le Homebrew Launcher (3dslink, argv vide : refusé, le fichier en marche est inconnu). Échec : maj_detail, et l'ancienne version reste. Réussite : quitter. Les Sauvegardes ne
 * sont jamais touchées. */
static void installer(const release_t *r)
{
    const int dsx = envIsHomebrew();
    const char *nom = dsx ? "3dssavesync.3dsx" : "3dssavesync.cia", *url = dsx ? r->dsx : r->cia;
    char *sommes = NULL, *octets = NULL, attendu[65], obtenu[65];
    size_t taille, n;
    long statut;
    FS_ArchiveResource sd;
    Result res;

    maj_echec = 1;
    snprintf(version_dl, sizeof version_dl, "%s", r->version);
    if (dsx && !lance_depuis_sd(chemin_3dsx)) {
        snprintf(maj_detail, sizeof maj_detail, T("Mise à jour impossible ici : lancez 3dssavesync depuis le Homebrew Launcher, ou installez le .cia.\nargv[0] : %.200s",
                                                  "Update not possible here: start 3dssavesync from the Homebrew Launcher, or install the .cia.\nargv[0]: %.200s"),
                 chemin_3dsx[0] ? chemin_3dsx : "-");
        return;
    }
    if (!url[0] || !r->sommes[0]) {
        snprintf(maj_detail, sizeof maj_detail, T("Version %s incomplète sur GitHub. Installez-la à la main (voir le guide 3DS).\nabsent : %s",
                                                  "Version %s is incomplete on GitHub. Install it by hand (see the 3DS guide).\nmissing: %s"),
                 r->version, url[0] ? "SHA256SUMS" : nom);
        return;
    }
    statut = net_github(r->sommes, avance_dl, &sommes, &n);
    if (statut != 200) {
        echec_github(T("Impossible de télécharger SHA256SUMS.", "Couldn't download SHA256SUMS."), statut); /* « Réessayez » : dans texte_github */
        goto fin;
    }
    if (maj_somme(sommes, nom, attendu)) {
        snprintf(maj_detail, sizeof maj_detail, T("SHA256SUMS ne contient pas %s. Installez la version %s à la main (voir le guide 3DS).\nSHA256SUMS : %u octets",
                                                  "SHA256SUMS has no %s. Install version %s by hand (see the 3DS guide).\nSHA256SUMS: %u bytes"),
                 nom, r->version, (unsigned)n);
        goto fin;
    }
    statut = net_github(url, avance_dl, &octets, &taille);
    if (statut != 200) {
        echec_github(T("Le téléchargement a échoué.", "Download failed."), statut);
        goto fin;
    }
    progression(0, 0, "%s", T("Vérification du fichier…", "Checking the file…"));
    hash_hex((unsigned char *)octets, taille, obtenu);
    if (strcmp(obtenu, attendu)) {
        snprintf(maj_detail, sizeof maj_detail, T("Le fichier téléchargé est abîmé. Réessayez.\nSHA-256 %.12s…, attendu %.12s…",
                                                  "The downloaded file is damaged. Try again.\nSHA-256 %.12s…, expected %.12s…"),
                 obtenu, attendu);
        goto fin;
    }
    /* .cia : place du titre installé ; .3dsx : le .tmp à côté de l'ancien (renommé .bak). Le double de la taille suffit dans les deux cas. */
    if (R_SUCCEEDED(FSUSER_GetSdmcArchiveResource(&sd)) && (u64)sd.freeClusters * sd.clusterSize < 2 * (u64)taille) {
        u64 libre = (u64)sd.freeClusters * sd.clusterSize;
        snprintf(maj_detail, sizeof maj_detail, T("Pas assez de place sur la carte SD. Libérez %llu Mo et réessayez.\nlibre : %llu Ko, requis : %llu Ko",
                                                  "Not enough space on the SD card. Free %llu MB and try again.\nfree: %llu KB, needed: %llu KB"),
                 (2 * (u64)taille - libre) / 1048576 + 1, libre / 1024, 2 * (u64)taille / 1024);
        goto fin;
    }
    if (dsx) {
        int e;
        progression(0, 0, T("Installation de la version %s…", "Installing version %s…"), version_dl);
        if ((e = remplacer_programme(chemin_3dsx, octets, taille))) {
            if (e == -1) snprintf(maj_detail, sizeof maj_detail, T("Impossible d'écrire sur la carte SD, l'ancienne version reste. Vérifiez la carte et réessayez.\n%s",
                                                                   "Can't write to the SD card, the old version stays. Check the card and try again.\n%s"), chemin_3dsx);
            else snprintf(maj_detail, sizeof maj_detail, T("Aucune version en place. Sur un PC, renommez %s.bak en %s pour retrouver l'ancienne.",
                                                           "No version in place. On a PC, rename %s.bak to %s to get the old one back."), chemin_3dsx, strrchr(chemin_3dsx, '/') + 1);
            goto fin;
        }
    } else if (R_FAILED(res = installer_cia(octets, taille))) {
        snprintf(maj_detail, sizeof maj_detail, T("La 3DS a refusé l'installation, l'ancienne version reste. Redémarrez la 3DS et réessayez.\nAM 0x%08lX",
                                                  "The 3DS refused the install, the old version stays. Restart the 3DS and try again.\nAM 0x%08lX"), res);
        goto fin;
    }
    maj_echec = 0;
    quitter = 1;
    {
        const touche_t b = { KEY_A, T("Fermer 3dssavesync", "Close 3dssavesync"), 1 };
        char titre[80];
        snprintf(titre, sizeof titre, T("Version %s installée", "Version %s installed"), r->version);
        do {
            ecran_debut();
            haut_courant();
            ecran_bas();
            nb_boutons = 0;
            ecran_paragraphe(8, 8 + ecran_paragraphe(8, 8, 0.55f, C_VERT, 304, titre) + 6, 0.5f, BLANC, 304,
                             dsx ? T("3dssavesync va se fermer. Relancez-le depuis le Homebrew Launcher.", "3dssavesync will close. Start it again from the Homebrew Launcher.")
                                 : T("3dssavesync va se fermer. Relancez-le depuis le menu HOME.", "3dssavesync will close. Start it again from the HOME Menu."));
            fond_barre(1);
            ligne_touches(&b, 1, 240 - HAUTEUR_TOUCHE - 4, 0);
            ecran_finir();
        } while (!(lire_touches() & (KEY_A | KEY_B)));
    }
fin:
    free(sommes);
    free(octets);
}

/* Réglages, ligne Mise à jour : vérification, puis installation si l'utilisateur la choisit. */
static void mettre_a_jour(void)
{
    release_t r;
    char t[300];
    if (verifier_maj(&r) != 1) return;
    snprintf(t, sizeof t, T("La version %s remplacera la version " VERSION ". Vos sauvegardes et vos réglages ne changent pas.",
                            "Version %s will replace version " VERSION ". Your saves and settings stay the same."),
             r.version);
    if (confirmer(T("Mettre à jour 3dssavesync ?", "Update 3dssavesync?"), NULL, t, NULL, T("Mettre à jour", "Update"))) installer(&r);
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
        dire(C_ROUGE, NULL, "%s", T("Impossible de lire la liste des jeux. Redémarrez la 3DS et réessayez.", "Can't read the installed games. Restart the 3DS and try again."));
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
    effacer();

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
            if (i == curseur) {
                ecran_rect(0, ly - 1, 400, 15, SEL);
                ecran_rect(0, ly - 1, 2, 15, ACCENT);
            }
            ecran_rect(9, ly + 2, 10, 10, c[i].coche || c[i].seul == 1 ? GRIS_C : GRIS_F); /* case : bord, fond, coche */
            ecran_rect(10, ly + 3, 8, 8, i == curseur ? SEL : RGB(0x0C0E12));
            if (c[i].coche) ecran_rect(11, ly + 4, 6, 6, C_VERT);
            ecran_texte(26, ly, 0.5f, c[i].coche || c[i].seul == 1 ? BLANC : GRIS_F, 400 - 26 - 14, c[i].nom);
        }
        if (debut > 0) ecran_texte(388, 26, 0.4f, GRIS_C, 0, "\xE2\x96\xB2");
        if (debut + LIGNES_HAUT < n) ecran_texte(388, 224, 0.4f, GRIS_C, 0, "\xE2\x96\xBC");

        ecran_bas();
        nb_boutons = 0;
        if (ch) {
            if (titre_icone(ch->id, ch->media, icone) == 0) ecran_icone(icone, 48, 48, 320 - 6 - 48, 6, 1.0f);
            y += ecran_lignes(8, y, 0.55f, BLANC, 320 - 8 - 62, 2, ch->nom) + 4;
            y = y < 58 ? 62 : y + 4;
            y += ecran_paragraphe(8, y, 0.45f, ch->seul == 1 ? C_VERT : ch->seul == 0 ? C_ROUGE : GRIS_C, 304,
                                  ch->seul == 1 ? T("Ce jeu peut être synchronisé.", "This game can be synced.")
                                  : ch->seul == 0 ? T("Ce jeu ne peut pas être synchronisé (plusieurs fichiers de sauvegarde).", "This game can't be synced (several save files).")
                                                  : T("Pas encore de sauvegarde : lancez le jeu une fois et sauvegardez.", "No save yet: start the game once and save.")) + 6;
        } else {
            y += ecran_paragraphe(8, y, 0.5f, GRIS_C, 304, T("Aucun jeu trouvé.", "No game found.")) + 6;
        }
        y += ecran_paragraphe(8, y, 0.42f, GRIS_C, 304, T("Un jeu manque ? Lancez-le une fois et sauvegardez.", "Game missing? Start it once and save.")) + 6;
        dessiner_message(FOND_BARRE_1);
        fond_barre(1);
        ligne_touches(t, 2, 240 - HAUTEUR_TOUCHE - 4, 18);
        ecran_finir();

        k = lire_touches();
        if (k & (KEY_UP | KEY_DOWN)) effacer();
        if (k & KEY_B) break;
        if (k & KEY_DOWN && curseur + 1 < n) curseur++;
        if (k & KEY_UP && curseur > 0) curseur--;
        if (k & KEY_A && n) {
            effacer();
            if (cochable) {
                c[curseur].coche = !c[curseur].coche;
                change = 1;
            } else {
                dire(C_ROUGE, NULL, "%s", T("Ce jeu ne peut pas être coché.", "This game cannot be ticked."));
            }
        }
    }
    effacer();
    if (change) {
        nb_titres = 0;
        for (i = 0; i < n && nb_titres < MAX_TITRES; i++)
            if (c[i].coche) {
                titres[nb_titres].id = c[i].id;
                snprintf(titres[nb_titres++].nom, sizeof titres[0].nom, "%s", c[i].nom);
            }
        mkdir(DOSSIER, 0777);
        if (titres_ecrire(DOSSIER "/titres.ini", titres, nb_titres))
            dire(C_ROUGE, DOSSIER "/titres.ini", "%s", T("Liste des jeux 3DS non enregistrée. Vérifiez la carte SD.", "3DS game list not saved. Check the SD card."));
        else dire(C_VERT, NULL, "%s", T("Liste des jeux 3DS enregistrée.", "3DS game list saved."));
    }
}

/* Lignes de réglage modifiables, puis Jeux 3DS, Connexion et Version (écran des réglages). */
static const struct {
    const char *nom, *nom_en, *aide, *aide_en;
    char *valeur;
    size_t n;
} REGLAGES[] = {
    { "Serveur", "Server", "Adresse du serveur, par exemple https://saves.exemple.fr, ou le serveur local du PC : http://192.168.1.20:8765",
      "Server address, for example https://saves.example.com, or the PC local server: http://192.168.1.20:8765", cfg.url, sizeof cfg.url },
    { "Jeton", "Token", "Le jeton du serveur, le même que sur le PC.", "The server token, the same as on the PC.", cfg.jeton, sizeof cfg.jeton },
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
#define NB_REGLAGES ((int)(sizeof REGLAGES / sizeof *REGLAGES))
static int reglage_choisi; /* ligne choisie des réglages : aussi pour l'écran du haut pendant une progression */

/* État de la version installée, comme sur la ligne Version : « À jour », « 3.7.1 disponible »… */
static const char *etat_version(char *t, size_t n, u32 *couleur)
{
    if (!strcmp(VERSION, "dev")) *couleur = GRIS_C, snprintf(t, n, "%s", T("Version de développement", "Development build"));
    else if (maj_plus_recente(VERSION, maj_version)) *couleur = C_CYAN, snprintf(t, n, T("%s disponible", "%s available"), maj_version);
    else if (maj_echec) *couleur = C_ROUGE, snprintf(t, n, "%s", T("Vérification impossible", "Check failed"));
    else if (!maj_version[0]) *couleur = GRIS_C, snprintf(t, n, "%s", T("Non vérifiée", "Not checked"));
    else *couleur = C_VERT, snprintf(t, n, "%s", T("À jour", "Up to date"));
    return t;
}

/* « 3.7.0 · À jour » à partir de x : version en blanc, point en gris, état en couleur, coupé avant fin. */
static void valeur_version(float x, float y, float taille, float fin)
{
    char e[48];
    u32 c;
    etat_version(e, sizeof e, &c);
    x += ecran_texte(x, y, taille, BLANC, fin - x, VERSION);
    x += ecran_texte(x, y, taille, GRIS_F, 0, " \xC2\xB7 ");
    ecran_texte(x, y, taille, c, fin - x, e);
}

static const char *libelle_reglage(int i)
{
    return i < NB_REGLAGES ? T(REGLAGES[i].nom, REGLAGES[i].nom_en) : i == NB_REGLAGES ? T("Jeux 3DS", "3DS games")
         : i == NB_REGLAGES + 1 ? T("Connexion", "Connection") : T("Version", "Version");
}

/* Ce que fait A sur la ligne i, dessiné à droite de la ligne choisie. */
static const char *action_reglage(int i)
{
    return i < NB_REGLAGES ? T("Modifier", "Change") : i == NB_REGLAGES ? T("Choisir", "Choose") : i == NB_REGLAGES + 1 ? T("Tester", "Test")
         : maj_plus_recente(VERSION, maj_version) ? T("Mettre à jour", "Update") : T("Rechercher", "Check");
}

/* Écran du haut des réglages : libellé à gauche, valeur sur une colonne, « Ⓐ action » à droite de la ligne choisie. */
static void haut_reglages(void)
{
    const int total = NB_REGLAGES + 3;
    float y = 29;
    int i;
    ecran_haut();
    ecran_texte(6, 4, 0.55f, BLANC, 0, T("Réglages", "Settings"));
    ecran_rect(0, 23, 400, 1, GRIS_F);
    for (i = 0; i < total; i++) {
        char v[96]; /* test_court : 80 */
        const char *valeur = v;
        u32 c = BLANC;
        int requis = 0;
        float fin = 400 - 8;
        if (i < NB_REGLAGES) {
            valeur = REGLAGES[i].valeur;
            if (i <= 1 && (!valeur[0] || !strcmp(valeur, "https://"))) valeur = T("À remplir", "Required"), c = GRIS_C, requis = 1;
            else if (i == 1) valeur = "\xE2\x80\xA2\xE2\x80\xA2\xE2\x80\xA2\xE2\x80\xA2\xE2\x80\xA2\xE2\x80\xA2\xE2\x80\xA2\xE2\x80\xA2"; /* •••••••• */
            else if (!valeur[0]) valeur = T("À côté de la ROM", "Next to the ROM"), c = GRIS_C; /* valeur par défaut */
        } else if (i == NB_REGLAGES) {
            if (nb_titres) snprintf(v, sizeof v, pluriel(nb_titres, anglais, T("%d jeu", "%d game"), T("%d jeux", "%d games")), nb_titres);
            else snprintf(v, sizeof v, "%s", T("Aucun", "None"));
        } else if (i == NB_REGLAGES + 1) {
            if (test_etat > 0) snprintf(v, sizeof v, "%s", T("Réussie", "OK")), c = C_VERT;
            else if (test_etat < 0) snprintf(v, sizeof v, "%s", test_court), c = C_ROUGE;
            else snprintf(v, sizeof v, "%s", T("Non testée", "Not tested")), c = GRIS_C;
        }
        if (i == reglage_choisi) {
            const char *a = action_reglage(i);
            float xa = 392 - ecran_largeur(0.44f, a), g = ecran_largeur(0.5f, glyphe(KEY_A));
            ecran_rect(0, y - 1, 400, 16, SEL);
            ecran_rect(0, y - 1, 2, 16, ACCENT);
            ecran_texte(xa - g - 3, y - 1, 0.5f, RGB(0xE04848), 0, glyphe(KEY_A));
            ecran_texte(xa, y + 1, 0.44f, BLANC, 0, a);
            fin = xa - g - 3 - 10;
        }
        if (requis) ecran_rect(0, y - 1, 2, 16, C_ROUGE);
        ecran_texte(8, y, 0.46f, i == reglage_choisi ? BLANC : GRIS_C, 128, libelle_reglage(i));
        if (i == NB_REGLAGES + 2) valeur_version(142, y, 0.46f, fin);
        else ecran_texte(142, y, 0.46f, c, fin - 142, valeur);
        y += 17 + (i == 1 || i == NB_REGLAGES - 1 ? 7 : 0); /* groupes : serveur, dossiers, le reste */
    }
}

/* Écran de réglages : une ligne par réglage, A pour l'action de la ligne, explication en bas. premier : aucun réglage
 * encore. 1 si un réglage ou la liste des jeux 3DS a changé (il faut alors tout relire). */
static int reglages(int premier)
{
    const int nb = NB_REGLAGES, total = nb + 3;
    int change;
    static titre_t avant_titres[MAX_TITRES];
    int avant_nb = nb_titres;
    char avant[sizeof cfg];

    memcpy(avant, &cfg, sizeof cfg);
    memcpy(avant_titres, titres, sizeof titres);
    reglage_choisi = 0;
    haut_courant = haut_reglages; /* progression et questions : les réglages restent en haut */
    effacer();
    while (aptMainLoop()) {
        const int c = reglage_choisi;
        u32 k;
        float y = 6;
        /* Premier lancement : B teste d'abord la connexion ; un second B après un échec continue quand même. */
        touche_t t[2] = { { KEY_A, action_reglage(c), 1 },
                          { KEY_B, !premier || test_etat > 0 ? T("Enregistrer et revenir", "Save and go back")
                                   : test_etat < 0           ? T("Continuer quand même", "Continue anyway")
                                                             : T("Tester et continuer", "Test and continue"), 1 } };

        ecran_debut();
        haut_reglages();
        ecran_bas();
        nb_boutons = 0;
        if (premier) {
            const char *b = T("Bienvenue ! Entrez d'abord le serveur et le jeton.", "Welcome! Enter the server and the token first.");
            float h = ecran_hauteur(0.45f, 296, b) + 8;
            ecran_rect(0, 0, 320, h, CARTE);
            ecran_rect(0, 0, 3, h, ACCENT);
            ecran_paragraphe(10, 4, 0.45f, BLANC, 296, b);
            y = h + 6;
        }
        if (c == nb + 2) {
            y += ecran_lignes(8, y, 0.55f, BLANC, 304, 1, T("Mise à jour de 3dssavesync", "3dssavesync update")) + 8;
            ecran_texte(8, y, 0.42f, GRIS_C, 100, T("Installée", "Installed"));
            valeur_version(112, y, 0.42f, 312);
            y += 15 + 6;
            y += ecran_paragraphe(8, y, 0.42f, GRIS_C, 304, T("Cherche une nouvelle version sur GitHub et vous demande avant de l'installer.",
                                                              "Looks for a new version on GitHub and asks before installing it.")) + 6;
        } else {
            y += ecran_lignes(8, y, 0.55f, BLANC, 304, 1, libelle_reglage(c)) + 6;
            y += ecran_paragraphe(8, y, 0.45f, BLANC, 304, c < nb ? T(REGLAGES[c].aide, REGLAGES[c].aide_en)
                                                         : c == nb ? T("Les jeux 3DS à synchroniser. Par défaut, les 8 Pokémon 3DS.", "3DS games to sync. Default: the 8 Pokémon 3DS games.")
                                                                   : T("Vérifie l'adresse et le jeton du serveur.", "Checks the server address and token.")) + 6;
        }
        if (message[0]) dessiner_message(FOND_BARRE_1);
        else if (c == nb + 2 && maj_detail[0]) { /* dernière vérification : texte, puis sa ligne technique (après le dernier saut) */
            char d[sizeof maj_detail];
            char *nl;
            u32 couleur;
            etat_version(d, sizeof d, &couleur);
            snprintf(d, sizeof d, "%s", maj_detail);
            nl = strrchr(d, '\n');
            if (nl) *nl++ = '\0';
            bande(FOND_BARRE_1, d, nl, couleur == GRIS_C ? BLANC : couleur);
        }
        fond_barre(1);
        ligne_touches(t, 2, 240 - HAUTEUR_TOUCHE - 4, 18);
        ecran_finir();

        k = lire_touches();
        if (k & KEY_B) {
            if (!premier || test_etat) break;
            effacer();
            if (tester_connexion()) break;
            continue;
        }
        if (k & KEY_DOWN && c + 1 < total) reglage_choisi++;
        if (k & KEY_UP && c > 0) reglage_choisi--;
        if (k & KEY_A) {
            effacer();
            if (c < nb) {
                size_t len;
                test_etat = 0; /* l'adresse ou le jeton a pu changer : le dernier test ne vaut plus */
                editer(T(REGLAGES[c].nom, REGLAGES[c].nom_en), REGLAGES[c].valeur, REGLAGES[c].n, c == 1);
                len = strlen(REGLAGES[c].valeur);
                while (len > 0 && REGLAGES[c].valeur[len - 1] == '/') REGLAGES[c].valeur[--len] = '\0'; /* "…/saves/" -> "…/saves" */
            } else if (c == nb) {
                choisir_titres();
            } else if (c == nb + 1) {
                tester_connexion();
            } else {
                mettre_a_jour();
                if (quitter) break;
            }
        }
    }
    haut_courant = haut_liste;
    if (!premier) effacer(); /* premier lancement : le résultat du test reste affiché */
    change = memcmp(avant, &cfg, sizeof cfg) || avant_nb != nb_titres || memcmp(avant_titres, titres, sizeof titres);
    if (memcmp(avant, &cfg, sizeof cfg)) ecrire_config();
    return change;
}

/* A sur un Jeu : son action, avec confirmation avant tout écrasement (Q5). 1 si une action a été tentée. */
static int agir(jeu_t *j)
{
    char q[500], srv[200], loc[200], t[80];
    partie_t p;
    int fait = 0;
    switch (j->etat) {
    case A_ENVOYER:
        envoyer(j);
        fait = 1;
        break;
    case A_RECUPERER: {
        int a_p = partie_serveur(j, &p);
        texte_serveur(j, srv, sizeof srv);
        texte_local(j, loc, sizeof loc);
        if (a_p) texte_partie(&p, t, sizeof t);
        snprintf(q, sizeof q, "%s\t%s%s%s\n%s\t%s", T("Sur le serveur", "On the server"), srv, a_p ? "\n\t" : "", a_p ? t : "", T("Sur la 3DS", "On the 3DS"), loc);
        if ((fait = confirmer(T("Récupérer la version du serveur ?", "Download the server version?"), j->court, q,
                              j->a_local ? T("La sauvegarde de la 3DS sera remplacée, après une copie de sécurité.", "The 3DS save will be replaced, after a backup.") : NULL,
                              T("Récupérer", "Download"))))
            recuperer(j);
        break;
    }
    case CONFLIT: {
        u32 k;
        const touche_t b = { KEY_B, T("Ne rien faire", "Do nothing"), 1 };
        /* Les deux cartes ont la même forme : la version, son temps de jeu (Pokémon), ce qui se passe. */
        if (partie_serveur(j, &p)) {
            texte_serveur(j, t, sizeof t);
            snprintf(srv, sizeof srv, "%s\n", t);
            texte_partie(&p, srv + strlen(srv), sizeof srv - strlen(srv));
        } else texte_serveur(j, srv, sizeof srv);
        if (j->a_partie && j->modif) { /* texte_local donne alors le temps de jeu : la date d'abord, comme à droite */
            char d[24];
            format_age((long)(time(NULL) - j->modif), anglais, 0, d, sizeof d);
            snprintf(loc, sizeof loc, T("Modifiée %s\n", "Modified %s\n"), d);
            texte_partie(&j->partie, loc + strlen(loc), sizeof loc - strlen(loc));
        } else texte_local(j, loc, sizeof loc);
        do {
            float y = 6, w;
            ecran_debut();
            haut_courant();
            ecran_bas();
            nb_boutons = 0;
            icone_jeu(j);
            ecran_rect(8, y + 2, 3, 14, C_ROUGE);
            w = ecran_texte(15, y, 0.55f, C_ROUGE, 0, T("Conflit", "Conflict"));
            w += ecran_texte(15 + w, y, 0.55f, GRIS_C, 0, " \xC2\xB7 ");
            ecran_texte(15 + w, y, 0.55f, BLANC, 250 - 15 - w, j->court);
            y += 22;
            y += ecran_lignes(8, y, 0.45f, BLANC, 250, 2, T("La sauvegarde a changé sur la 3DS et sur le serveur. Quelle version garder ?",
                                                             "The save changed on the 3DS and on the server. Which one to keep?"));
            y = y < 58 ? 60 : y + 6;
            carte(6, y, KEY_A, T("Version 3DS", "3DS save"), loc, T("Puis envoyée au serveur.", "Then uploaded to the server."));
            carte(162, y, KEY_X, T("Version serveur", "Server save"), srv, T("La 3DS est copiée avant.", "The 3DS is backed up first."));
            ecran_paragraphe(8, y + 98, 0.42f, GRIS_C, 304, T("Rien n'est perdu : l'autre version est gardée.", "Nothing is lost: the other one is kept."));
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
            dire(BLANC, NULL, T("%s : conflit laissé tel quel.", "%s: conflict left as is."), j->court);
        }
        break;
    }
    case DOUBLON:
        if (j->partage) dire(C_ROUGE, NULL, T("%s : %d ROM ont ce nom de fichier et partageraient une sauvegarde. Renommez-en une.",
                                              "%s: %d ROMs have this file name and would share one save. Rename one."), j->court, j->nb_roms);
        else dire(C_ROUGE, NULL, T("%s : %d ROM contiennent ce jeu. Gardez-en une, ou ajoutez les autres aux ROM hacks sur le PC.",
                                   "%s: %d ROMs hold this game. Keep one, or add the others to ROM hacks on the PC."), j->court, j->nb_roms);
        break;
    case HORS_LIGNE:
        dire(C_ROUGE, NULL, T("%s : serveur non lu. Appuyez sur \xEE\x80\x83 pour réessayer.", "%s: server not read. Press \xEE\x80\x83 to try again."), j->court);
        break;
    default:
        dire(BLANC, NULL, T("%s : déjà à jour.", "%s: already in sync."), j->court);
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
    char fwd[600], copie[700], date[20], q[700], avert[200], action[64];
    const char *autre = j->systeme == SYS_GBA ? "open_agb_firm" : "TWiLight Menu++";
    int vers_rom;
    time_t t = time(NULL);

    if (j->systeme != SYS_GB && j->systeme != SYS_GBA) {
        dire(BLANC, NULL, "%s", T("X : seulement pour les jeux GB et GBA.", "X: GB and GBA games only."));
        return 0;
    }
    snprintf(fwd, sizeof fwd, FORWARDERS "/%s.sav", j->nom);
    vers_rom = !strcmp(j->sav, fwd);
    snprintf(q, sizeof q,
             T("Sur la 3DS, ce jeu se joue avec %s (sauvegarde à côté de la ROM) ou avec un forwarder mGBA "
               "(icône du menu HOME, sauvegarde dans sdmc:/mGBA). 3dssavesync ne synchronise que l'une des deux.\n\n"
               "Actuellement\t%s\nPasser à\t%s",
               "On the 3DS, this game runs with %s (save next to the ROM) or with an mGBA forwarder "
               "(HOME Menu icon, save in sdmc:/mGBA). 3dssavesync syncs only one of the two.\n\n"
               "Now\t%s\nSwitch to\t%s"),
             autre, vers_rom ? T("forwarder mGBA", "mGBA forwarder") : autre, vers_rom ? autre : T("forwarder mGBA", "mGBA forwarder"));
    snprintf(avert, sizeof avert, T("La sauvegarde y sera copiée, après une copie de sécurité.%s", "The save is copied there, after a backup.%s"),
             vers_rom ? T(" Le forwarder n'aura plus de sauvegarde.", " The forwarder will have no save left.") : "");
    snprintf(action, sizeof action, T("Jouer avec %s", "Play with %s"), vers_rom ? autre : T("le forwarder", "the forwarder"));
    if (!confirmer(j->court, NULL, q, avert, action)) return 0;
    strftime(date, sizeof date, "%Y%m%d-%H%M%S", localtime(&t));
    mkdir(DOSSIER "/copies", 0777);
    mkdir("sdmc:/mGBA", 0777);
    mkdir(FORWARDERS, 0777);
    creer_dossier_de(j->sav_rom);
    if (chemin_copie(DOSSIER "/copies", j->nom, date, "sav", copie, sizeof copie) ||
        deplacer_sauvegarde(j->sav, vers_rom ? j->sav_rom : fwd, copie, vers_rom))
        dire(C_ROUGE, j->sav, T("%s : changement impossible, rien n'est perdu.", "%s: switch failed, nothing lost."), j->court);
    else
        dire(C_VERT, NULL, T("%s : se joue maintenant avec %s.", "%s: now played with %s."), j->court, vers_rom ? autre : T("le forwarder mGBA", "the mGBA forwarder"));
    lire_local(j);
    return 1;
}

/* « 2 envois, 1 récupération, 1 ignoré » : seulement ce qui compte. */
static void resume_synchro(char *out, size_t n, int envois, int recups, int echecs, int ignores)
{
    const int v[4] = { envois, recups, echecs, ignores };
    const char *un[4] = { T("%d envoi", "%d upload"), T("%d récupération", "%d download"), T("%d échec", "%d failed"), T("%d ignoré", "%d skipped") };
    const char *plusieurs[4] = { T("%d envois", "%d uploads"), T("%d récupérations", "%d downloads"), T("%d échecs", "%d failed"), T("%d ignorés", "%d skipped") };
    size_t l = 0;
    int i;
    out[0] = '\0';
    for (i = 0; i < 4; i++)
        if (v[i] && l < n) {
            if (l) l += snprintf(out + l, n - l, ", ");
            if (l < n) l += snprintf(out + l, n - l, pluriel(v[i], anglais, un[i], plusieurs[i]), v[i]);
        }
    if (!l) snprintf(out, n, "%s", T("Rien à synchroniser.", "Nothing to sync."));
}

/* START : Envois et Récupérations ; ignore Conflits et Doublons ; une confirmation pour les écrasements. Puis relit le
 * serveur, et le message dit ce qui a été fait. */
static void tout_synchroniser(void)
{
    char q[1200], premier[200] = "", premier_detail[200] = "", r[120];
    int i, ecrase = 0, envois = 0, recups = 0, ignores = 0, echecs = 0, ok_ecraser = 1, total = 0, fait = 0;
    int prevu_envois = 0, prevu_recups = 0;
    u64 debut = osGetTime();

    if (!serveur_lu) {
        dire(C_ROUGE, NULL, "%s", T("Serveur non lu : appuyez sur \xEE\x80\x83 pour réessayer.", "Server not read: press \xEE\x80\x83 to try again."));
        return;
    }
    for (i = 0; i < nb_visibles; i++) {
        int e = jeux[visibles[i]].etat;
        prevu_envois += e == A_ENVOYER;
        prevu_recups += e == A_RECUPERER;
        ignores += e == CONFLIT || e == DOUBLON;
    }
    resume_synchro(r, sizeof r, prevu_envois, prevu_recups, 0, ignores);
    snprintf(q, sizeof q, "%s\n\n%s", r, T("Ces sauvegardes de la 3DS seront remplacées par celles du serveur, après une copie de sécurité :\n",
                                           "These 3DS saves will be replaced by the server's, after a backup:\n"));
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
    if (ecrase > 8) snprintf(q + strlen(q), sizeof q - strlen(q), pluriel(ecrase - 8, anglais, T("… et %d autre\n", "… and %d more\n"), T("… et %d autres\n", "… and %d more\n")), ecrase - 8);
    if (ecrase) ok_ecraser = confirmer(T("Tout synchroniser", "Sync all"), NULL, q, NULL, T("Remplacer", "Replace"));
    for (i = 0; i < nb_visibles; i++) {
        jeu_t *j = &jeux[visibles[i]];
        total += j->etat == A_ENVOYER || (j->etat == A_RECUPERER && (ok_ecraser || !j->a_local));
    }
    operation = T("Tout synchroniser", "Sync all");
    for (i = 0; i < nb_visibles; i++) {
        jeu_t *j = &jeux[visibles[i]];
        int faire_envoi = j->etat == A_ENVOYER;
        int faire_recup = j->etat == A_RECUPERER && (ok_ecraser || !j->a_local);
        if (faire_envoi || faire_recup) {
            int ok;
            curseur_liste = i; /* l'écran du haut suit le jeu en cours */
            couleur_barre = faire_envoi ? C_JAUNE : C_CYAN;
            progression(++fait, total, "%s\n%s", faire_envoi ? T("Envoi", "Upload") : T("Récupération", "Download"), j->court);
            ok = faire_envoi ? envoyer(j) : recuperer(j);
            envois += faire_envoi && ok;
            recups += faire_recup && ok;
            if (!ok && !echecs++) { /* « jeu : raison », et sa ligne technique */
                snprintf(premier, sizeof premier, "%.199s", message);
                snprintf(premier_detail, sizeof premier_detail, "%s", message_detail);
            }
            lire_local(j);
        }
    }
    operation = NULL;
    couleur_barre = C_VERT;
    if (total) relire_serveur();
    /* Résumé, puis le premier échec : le message de la dernière action ne dit rien des autres. */
    resume_synchro(r, sizeof r, envois, recups, echecs, ignores);
    if (total) snprintf(r + strlen(r), sizeof r - strlen(r), " \xC2\xB7 %llu s", (osGetTime() - debut) / 1000);
    dire(echecs ? C_ROUGE : total ? C_VERT : BLANC, premier_detail, "%s%s%s%s", r, echecs ? "\n" : "", echecs > 1 ? T("Premier échec : ", "First failure: ") : "", premier);
}

/* ---------------------------------------------------------------- boucle */

int main(int argc, char **argv)
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
    lire_cles(DOSSIER "/maj.ini", cle_maj);
    if (strcmp(maj_installee, VERSION)) ecrire_maj_ini(); /* le PC ne copie pas un .cia plus ancien (carte3ds.go) */
    if (argc > 0 && argv[0]) chemin_3dsx = argv[0];

    if (net_init()) {
        const touche_t b = { KEY_B, T("Quitter", "Quit"), 1 };
        lu = 1;
        do {
            ecran_debut();
            ecran_haut();
            ecran_texte(6, 4, 0.55f, BLANC, 0, "3dssavesync");
            ecran_bas();
            nb_boutons = 0;
            ecran_paragraphe(16, 70, 0.55f, C_ROUGE, 288, T("Impossible de démarrer le réseau. Redémarrez la 3DS et relancez 3dssavesync.", "Couldn't start the network. Restart the 3DS and 3dssavesync."));
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
    if (!quitter) rafraichir();
    montrer_alerte();
    if (!quitter && strcmp(VERSION, "dev") && net_wifi() && maj_a_verifier(maj_jour, time(NULL))) {
        release_t r;
        verifier_maj(&r); /* échec : détail dans les réglages seulement, la synchronisation marche comme avant */
    }
    while (!quitter && aptMainLoop()) {
        u32 k;
        curseur_liste = curseur;
        dessiner();
        k = lire_touches();

        if (k & KEY_B) break;
        if (k & (KEY_UP | KEY_DOWN)) effacer(); /* le message parle du jeu qu'on quitte */
        if (k & KEY_DOWN && curseur + 1 < nb_visibles) curseur++;
        if (k & KEY_UP && curseur > 0) curseur--;
        if (k & (KEY_A | KEY_START | KEY_X | KEY_Y | KEY_SELECT)) {
            int choisi = nb_visibles ? visibles[curseur] : -1, i;
            effacer();
            /* Tout relire : Y, ou réglages modifiés. Après une action : le Jeu touché est déjà relu, le serveur seul. */
            if (k & KEY_SELECT) {
                if (reglages(0) && !quitter) rafraichir();
            } else if (k & KEY_Y) {
                rafraichir();
            } else if (k & KEY_A && nb_visibles) {
                if (agir(&jeux[choisi])) relire_serveur();
            } else if (k & KEY_X && nb_visibles) {
                if (basculer(&jeux[choisi])) relire_serveur();
            } else if (k & KEY_START) {
                tout_synchroniser();
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
