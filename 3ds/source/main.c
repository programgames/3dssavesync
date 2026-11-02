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
#include "etat.h"
#include "fichiers.h"
#include "net.h"
#include "reglages.h"
#include "titres3ds.h"

#define DOSSIER "sdmc:/3ds/3dssavesync"
#define ANCIEN "sdmc:/3ds/nds-save-sync" /* nom avant la v3 : réglages et Bases repris au premier lancement */
#define FORWARDERS "sdmc:/mGBA/forwarders" /* Sauvegardes des forwarders mGBA */
#define MAX_JEUX 512 /* ponytail: tableau fixe ; au-delà de 512 ROM, les suivantes sont ignorées */
#define LIGNES 27 /* écrans en console : lignes de la liste */
#define LIGNES_HAUT 14 /* écran principal dessiné : lignes de la liste */
#define MASQUE -1   /* ni Sauvegarde locale ni Version courante : pas affiché */
#define DOUBLON 100 /* plusieurs ROM pour une Clé (ADR 0005) */

static int anglais; /* langue de la console : français, sinon anglais (sans accents en console : police 8×8) */
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
    obs_t local, serveur;
    char hash_local[65], hash_serveur[65], date[17], client[17];
    long age; /* secondes depuis la Version courante (horloge du serveur), -1 si inconnu */
    int a_local, a_serveur;
    int vide; /* SRAM locale vierge : comptée comme absente (cle.h) */
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
    else if (s < 60) snprintf(out, n, "%s", T("a l'instant", "just now"));
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
static PrintConsole haut, bas;
static char message[512];

/* Affiche tout de suite une ligne de progression en bas (pendant les longues boucles). */
static void progression(const char *format, ...)
{
    va_list a;
    consoleSelect(&bas);
    printf("\x1b[2J\x1b[1;1H");
    va_start(a, format);
    vprintf(format, a);
    va_end(a);
    gfxFlushBuffers();
    gfxSwapBuffers();
    gspWaitForVBlank();
}

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
    FILE *f;
    mkdir(DOSSIER, 0777);
    if ((f = fopen(DOSSIER "/config.ini", "w"))) {
        fprintf(f, "url=%s\njeton=%s\nroms=%s\nsaves=%s\nroms_gb=%s\nsaves_gb=%s\nroms_gba=%s\nsaves_gba=%s\n", cfg.url,
                cfg.jeton, cfg.roms, cfg.saves, cfg.roms_gb, cfg.saves_gb, cfg.roms_gba, cfg.saves_gba);
        fclose(f);
    }
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
    FILE *f;
    int i;
    for (i = 0; i < nb_bases && strcmp(bases[i].cle, cle); i++);
    if (i == nb_bases) {
        if (nb_bases == MAX_JEUX) return;
        snprintf(bases[nb_bases++].cle, CLE_MAX, "%s", cle);
    }
    snprintf(bases[i].hash, 65, "%s", hash);
    if ((f = fopen(DOSSIER "/base.ini", "w"))) {
        for (i = 0; i < nb_bases; i++) fprintf(f, "%s=%s\n", bases[i].cle, bases[i].hash);
        fclose(f);
    }
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

/* Une source de ROM (racine seulement, Q10) : Clé lue dans l'en-tête, .sav nommé comme la ROM dans le dossier saves. */
static void scanner_dossier(systeme_t s, const char *roms, const char *saves)
{
    DIR *d = opendir(roms);
    struct dirent *e;

    while (d && (e = readdir(d)) && nb_jeux < MAX_JEUX) {
        char chemin[600], cle[CLE_MAX];
        unsigned char entete[0x200];
        const char *ext = strrchr(e->d_name, '.');
        jeu_t *j = NULL;
        size_t n;
        FILE *f;
        int i;

        if (!ext || !extension_de(s, ext)) continue;
        snprintf(chemin, sizeof chemin, "%s/%s", roms, e->d_name);
        if (!(f = fopen(chemin, "rb"))) continue;
        n = fread(entete, 1, sizeof entete, f);
        fclose(f);
        if (n < sizeof entete || cle_rom(s, entete, cle)) continue; /* homebrew ou en-tête illisible */

        for (i = 0; i < nb_jeux; i++)
            if (!strcmp(jeux[i].cle, cle)) j = &jeux[i];
        if (j) {
            j->nb_roms++;
            continue;
        }
        j = &jeux[nb_jeux++];
        memset(j, 0, sizeof *j);
        snprintf(j->cle, sizeof j->cle, "%s", cle);
        j->systeme = s;
        snprintf(j->nom, sizeof j->nom, "%.*s", (int)(ext - e->d_name), e->d_name);
        snprintf(j->rom, sizeof j->rom, "%s", chemin);
        snprintf(j->sav_rom, sizeof j->sav_rom, "%s/%.*s.sav", saves, (int)(ext - e->d_name), e->d_name);
        nom_court(j->nom, j->court, sizeof j->court);
        j->nb_roms = 1;
        if (nb_jeux % 10 == 0) progression(T("Lecture des ROM : %d jeu(x)...", "Reading ROMs: %d game(s)..."), nb_jeux);
    }
    if (d) closedir(d);
}

/* Sauvegarde locale d'un Jeu : son fichier (forwarder mGBA s'il existe, docs/guide-forwarders-mgba.md) et son Hash
 * (SRAM seule pour GB/GBA, ADR 0008 ; /main pour la 3DS). Appelée au scan, et après une action sur ce seul Jeu. */
static void lire_local(jeu_t *j)
{
    unsigned char *octets;
    size_t taille;
    j->a_local = j->vide = 0;
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
        j->vide = sauvegarde_vide(j->systeme, octets, taille);
    }
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
        unsigned char *octets;
        size_t taille;
        progression(T("Lecture des sauvegardes 3DS : %d/%d\n\n%s", "Reading 3DS saves: %d/%d\n\n%s"), i + 1, nb_titres, titres[i].nom);
        memset(j, 0, sizeof *j);
        j->titre = titres[i].id;
        if (titre_lire(j->titre, &j->media, &octets, &taille)) continue; /* jamais lancé ou absent */
        cle_3ds(j->titre, j->cle);
        j->systeme = SYS_3DS;
        snprintf(j->nom, sizeof j->nom, "%s", titres[i].nom);
        snprintf(j->court, sizeof j->court, "%.47s", titres[i].nom);
        j->nb_roms = 1;
        hash_hex(octets, taille, j->hash_local);
        j->local.taille = taille;
        j->a_local = 1;
        free(octets);
        nb_jeux++;
    }
}

/* Toutes les sources, puis le Hash de chaque Sauvegarde locale (SRAM seule pour GB/GBA, ADR 0008). */
static void scanner(void)
{
    int i, avant;

    nb_jeux = 0;
    scanner_dossier(SYS_DS, cfg.roms, cfg.saves[0] ? cfg.saves : cfg.roms); /* sauvegardes vides = à côté de la ROM */
    scanner_dossier(SYS_GB, cfg.roms_gb, cfg.saves_gb[0] ? cfg.saves_gb : cfg.roms_gb);
    scanner_dossier(SYS_GBA, cfg.roms_gba, cfg.saves_gba[0] ? cfg.saves_gba : cfg.roms_gba);
    avant = nb_jeux;
    for (i = 0; i < avant; i++) {
        progression(T("Lecture des sauvegardes : %d/%d\n\n%.38s", "Reading saves: %d/%d\n\n%.38s"), i + 1, avant, jeux[i].court);
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
    snprintf(url, sizeof url, "%s/saves", cfg.url);
    statut = net_get(url, cfg.jeton, &corps, &taille);
    if (statut != 200) {
        if (statut < 0) snprintf(message, sizeof message, T("Serveur injoignable : %s", "Server unreachable: %s"), net_erreur(statut));
        else snprintf(message, sizeof message, T("Serveur : HTTP %ld%s", "Server: HTTP %ld%s"), statut, statut == 401 ? T(" (jeton refuse)", " (token refused)") : "");
        if (statut > 0) free(corps);
        return -1;
    }
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
    progression(T("Lecture du serveur...", "Reading the server..."));
    r = lire_serveur();
    if (r == 0) calculer_etats();
    else nb_visibles = 0;
    return r;
}

/* Tout relire : ROM, Sauvegardes, serveur (Y, réglages modifiés). */
static int rafraichir(void)
{
    int r;
    scanner();
    progression(T("Lecture du serveur...", "Reading the server..."));
    r = lire_serveur();
    if (r == 0) calculer_etats();
    else nb_visibles = 0;
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
        snprintf(message, sizeof message, T("%s : lecture impossible", "%s: cannot read"), j->court);
        return 0;
    }
    taille = sram_taille(j->systeme, taille);
    hash_hex(octets, taille, hash);
    if (j->a_serveur) snprintf(condition, sizeof condition, "If-Match: \"%s\"", j->hash_serveur);
    else snprintf(condition, sizeof condition, "If-None-Match: *");
    snprintf(url, sizeof url, "%s/saves/%s", cfg.url, j->cle);
    statut = net_put(url, cfg.jeton, condition, octets, taille);
    free(octets);
    if (statut == 200) {
        fixer_base(j->cle, hash);
        snprintf(message, sizeof message, T("%s : envoye", "%s: sent"), j->cle);
    } else if (statut == 412) {
        snprintf(message, sizeof message, T("%s : le serveur a change entre-temps (Conflit)", "%s: the server changed meanwhile (Conflict)"), j->cle);
    } else if (statut < 0) {
        snprintf(message, sizeof message, "%s : %s", j->cle, net_erreur(statut));
    } else {
        snprintf(message, sizeof message, T("%s : envoi refuse, HTTP %ld", "%s: upload refused, HTTP %ld"), j->cle, statut);
    }
    return statut == 200;
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
    int existe = j->systeme == SYS_3DS || access(j->sav, F_OK) == 0;
    time_t t = time(NULL);

    snprintf(url, sizeof url, "%s/saves/%s", cfg.url, j->cle);
    statut = net_get(url, cfg.jeton, &corps, &taille);
    if (statut != 200) {
        if (statut > 0) free(corps);
        snprintf(message, sizeof message, T("%s : Recuperation impossible (%ld)", "%s: download failed (%ld)"), j->court, statut);
        return 0;
    }
    hash_hex((unsigned char *)corps, taille, hash);
    if (strcmp(hash, j->hash_serveur)) {
        free(corps);
        snprintf(message, sizeof message, T("%s : Version recue differente, rien n'est ecrit", "%s: received version differs, nothing written"), j->court);
        return 0;
    }
    /* Copie dès que la Sauvegarde existe, même si le scan n'a pas pu la lire : jamais d'écrasement sans copie. */
    if (existe) {
        int echec;
        strftime(date, sizeof date, "%Y%m%d-%H%M%S", localtime(&t));
        mkdir(DOSSIER "/copies", 0777);
        snprintf(copie, sizeof copie, DOSSIER "/copies/%s-%s.%s", j->nom, date, j->systeme == SYS_3DS ? "main" : "sav");
        echec = lire_sauvegarde(j, &ancien, &n);
        if (!echec) echec = ecrire_fichier(copie, ancien, n);
        if (echec) {
            free(ancien);
            free(corps);
            snprintf(message, sizeof message, T("%s : Copie de securite impossible, rien n'est ecrit", "%s: backup copy failed, nothing written"), j->court);
            return 0;
        }
    }

    if (j->systeme == SYS_3DS) {
        int echec = titre_ecrire(j->titre, j->media, corps, taille);
        free(ancien);
        free(corps);
        if (echec) {
            snprintf(message, sizeof message, T("%s : ecriture 3DS refusee, sauvegarde intacte", "%s: 3DS write refused, save intact"), j->court);
            return 0;
        }
    } else {
        /* SRAM reçue + Trailer d'horloge local (ce qui dépasse la SRAM de l'ancien fichier). */
        sram = existe ? sram_taille(j->systeme, n) : n;
        nouveau = malloc(taille + (n - sram) + 1);
        if (!nouveau) {
            free(ancien);
            free(corps);
            snprintf(message, sizeof message, T("%s : memoire insuffisante, rien n'est ecrit", "%s: out of memory, nothing written"), j->court);
            return 0;
        }
        memcpy(nouveau, corps, taille);
        if (n > sram) memcpy(nouveau + taille, ancien + sram, n - sram);
        free(ancien);
        free(corps);
        {   /* dossier des sauvegardes absent (premier jeu) : créé */
            char d[600], *fin;
            snprintf(d, sizeof d, "%s", j->sav);
            if ((fin = strrchr(d, '/'))) {
                *fin = '\0';
                mkdir(d, 0777);
            }
        }
        snprintf(tmp, sizeof tmp, "%s.tmp", j->sav);
        if (ecrire_fichier(tmp, nouveau, taille + (n - sram))) {
            free(nouveau);
            remove(tmp);
            snprintf(message, sizeof message, T("%s : ecriture impossible, Sauvegarde locale intacte", "%s: write failed, local save intact"), j->court);
            return 0;
        }
        free(nouveau);
        /* rename() refuse d'écraser sur la SD : on retire l'ancienne (déjà copiée) juste avant. */
        remove(j->sav);
        if (rename(tmp, j->sav)) {
            snprintf(message, sizeof message, T("%s : renommage impossible, garde %s.tmp !", "%s: rename failed, keep %s.tmp!"), j->court, j->nom);
            return 0;
        }
    }
    fixer_base(j->cle, j->hash_serveur);
    snprintf(message, sizeof message, T("%s : recupere", "%s: downloaded"), j->court);
    return 1;
}

/* ---------------------------------------------------------------- écran */

/* Couleurs de la console libctru (codes ANSI). */
#define NORMAL "\x1b[0m"
#define VERT "\x1b[32m"
#define JAUNE "\x1b[1m\x1b[33m"
#define CYAN "\x1b[1m\x1b[36m"
#define ROUGE "\x1b[1m\x1b[31m"
#define GRIS "\x1b[1m\x1b[30m"
#define SURLIGNE "\x1b[44m"

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

/* Écrans encore en console (Conflit) : icône écrite directement dans l'écran du bas (RGB565, tourné). */
static void dessiner_icone(const jeu_t *j, int x0, int y0)
{
    u16 p[32 * 32], larg, haut_fb, *fb;
    int x, y;
    if (icone_ds(j, p)) return;
    fb = (u16 *)gfxGetFramebuffer(GFX_BOTTOM, GFX_LEFT, &larg, &haut_fb); /* écran tourné : larg = 240 */
    for (y = 0; y < 32; y++)
        for (x = 0; x < 32; x++)
            if (p[y * 32 + x] != FOND_ICONE) fb[(x0 + x) * larg + (larg - 1 - (y0 + y))] = p[y * 32 + x];
    gfxFlushBuffers();
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
    case DOUBLON: return GRIS_C;
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
    }
    return "ok";
}

/* Boutons de l'écran du bas : dessinés, et touchables au stylet. */
static struct { float x, y, l; u32 touche; } boutons[8];
static int nb_boutons;
#define HAUTEUR_BOUTON 26

/* Glyphe de bouton de la police du système (U+E000 A, E001 B, E002 X, E003 Y), à la couleur de la New 3DS. */
static void bouton(float x, float y, float l, u32 touche, const char *texte)
{
    int mot = touche & (KEY_START | KEY_SELECT);
    const char *glyphe = touche == KEY_A ? "\xEE\x80\x80" : touche == KEY_B ? "\xEE\x80\x81" : touche == KEY_X ? "\xEE\x80\x82"
                       : touche == KEY_Y ? "\xEE\x80\x83" : touche == KEY_START ? "START" : "SELECT";
    u32 c = touche == KEY_A ? RGB(0xE04848) : touche == KEY_B ? RGB(0xF0C030) : touche == KEY_X ? RGB(0x4080F0)
          : touche == KEY_Y ? RGB(0x40B060) : GRIS_C;
    float g;
    ecran_rect(x, y, l, HAUTEUR_BOUTON, RGB(0x2A313B));
    g = ecran_texte(x + 6, y + (mot ? 7 : 3), mot ? 0.4f : 0.6f, c, 0, glyphe);
    ecran_texte(x + 12 + g, y + 6, 0.45f, BLANC, l - 18 - g, texte);
    if (nb_boutons < 8) {
        boutons[nb_boutons].x = x;
        boutons[nb_boutons].y = y;
        boutons[nb_boutons].l = l;
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
        if (p.px >= boutons[i].x && p.px < boutons[i].x + boutons[i].l && p.py >= boutons[i].y && p.py < boutons[i].y + HAUTEUR_BOUTON)
            return boutons[i].touche;
    return 0;
}

/* Message des actions, sans les codes couleur de la console. */
static void sans_ansi(const char *s, char *out, size_t n)
{
    size_t j = 0;
    while (*s && j + 1 < n) {
        if (*s == '\x1b') {
            while (*s && *s != 'm' && *s != 'H' && *s != 'J') s++;
            if (*s) s++;
        } else {
            out[j++] = *s++;
        }
    }
    out[j] = '\0';
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
    for (i = 0; i < 5; i++) {
        int e = etats[i], c = n[priorite(e)];
        if (!c && e != A_JOUR) continue;
        snprintf(t, sizeof t, e == A_JOUR ? T("%d à jour", "%d in sync") : e == DOUBLON ? T("%d doublon(s)", "%d duplicate(s)")
                            : e == A_ENVOYER ? T("%d à envoyer", "%d to upload") : e == A_RECUPERER ? T("%d à récupérer", "%d to download")
                                                                                 : T("%d conflit(s)", "%d conflict(s)"), c);
        x -= ecran_largeur(0.45f, t);
        ecran_texte(x, 6, 0.45f, e == A_JOUR ? C_VERT : couleur_etat(e), 0, t);
        x -= 12;
    }
    ecran_rect(0, 23, 400, 1, GRIS_F);

    if (!nb_visibles) ecran_paragraphe(10, 40, 0.5f, GRIS_C, 380, T("Aucun jeu à afficher.", "No game to show."));
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
    case A_ENVOYER: return T("La sauvegarde de la 3DS est plus récente : envoie-la au serveur.", "The 3DS save is newer: upload it to the server.");
    case A_RECUPERER: return T("Le serveur a une version plus récente : récupère-la sur la 3DS.", "The server has a newer version: download it to the 3DS.");
    case CONFLIT: return T("Conflit : la 3DS et le serveur ont changé depuis la dernière synchronisation.", "Conflict: the 3DS and the server both changed since the last sync.");
    case DOUBLON:
        snprintf(t, n, T("%d ROM ont la même clé : garde-en une seule.", "%d ROMs have the same key: keep only one."), j->nb_roms);
        return t;
    }
    return T("À jour : la 3DS et le serveur ont la même sauvegarde.", "In sync: the 3DS and the server have the same save.");
}

static void dessiner_bas(const jeu_t *j)
{
    char t[600], age[24];
    float y = 6, h = 240 - 6 - HAUTEUR_BOUTON;
    int action = 0, gb = 0;
    u16 icone[48 * 48];

    ecran_bas();
    nb_boutons = 0;
    if (j) {
        const char *b = base_de(j->cle);
        int i3ds = j->systeme == SYS_3DS;
        gb = j->systeme == SYS_GB || j->systeme == SYS_GBA;
        action = j->etat == A_ENVOYER || j->etat == A_RECUPERER || j->etat == CONFLIT;
        if (i3ds ? titre_icone(j->titre, j->media, icone) == 0 : icone_ds(j, icone) == 0)
            ecran_icone(icone, i3ds ? 48 : 32, i3ds ? 48 : 32, 320 - 6 - 48, 6, i3ds ? 1.0f : 1.5f);
        y += ecran_paragraphe(8, y, 0.6f, BLANC, 320 - 8 - 62, j->court);
        ecran_texte(8, y, 0.4f, GRIS_C, 320 - 8 - 62, j->systeme == SYS_DS ? "Nintendo DS" : j->systeme == SYS_GB ? "Game Boy / Game Boy Color"
                                                       : j->systeme == SYS_GBA ? "Game Boy Advance" : "Nintendo 3DS");
        y = y + 14 < 58 ? 62 : y + 18; /* sous l'icône */

        y += ecran_paragraphe(8, y, 0.5f, j->etat == A_JOUR ? C_VERT : couleur_etat(j->etat), 304, phrase_etat(j, t, sizeof t)) + 6;
        y = detail(y, "3DS", !j->a_local ? T("aucune sauvegarde", "no save") : j->vide ? T("sauvegarde vierge", "blank save") : T("sauvegarde présente", "save present"));
        format_age(j->age, age, sizeof age);
        if (j->a_serveur) snprintf(t, sizeof t, T("envoyée par %s, %s", "sent by %s, %s"), j->client[0] ? j->client : "?", age);
        y = detail(y, T("Serveur", "Server"), j->a_serveur ? t : T("aucune version", "no version"));
        y = detail(y, T("Dernière synchro", "Last sync"),
                   !b ? T("jamais", "never")
                   : !strcmp(b, j->hash_local) && !strcmp(b, j->hash_serveur) ? T("identique aux deux", "same as both")
                   : !strcmp(b, j->hash_local) ? T("identique à la 3DS", "same as the 3DS")
                   : !strcmp(b, j->hash_serveur) ? T("identique au serveur", "same as the server") : T("autre version", "another version"));
        if (gb) y = detail(y, T("Joué avec", "Played with"), !strncmp(j->sav, FORWARDERS, strlen(FORWARDERS)) ? "forwarder mGBA" : j->systeme == SYS_GBA ? "open_agb_firm" : "TWiLight Menu++");
        y += 6;
    }
    sans_ansi(message, t, sizeof t);
    if (t[0]) ecran_paragraphe(8, y, 0.45f, C_JAUNE, 304, t);

    /* Du bas vers le haut : SELECT et B, START et Y, puis X et A selon le jeu (toute la largeur). */
    bouton(6, h, 152, KEY_SELECT, T("Réglages", "Settings"));
    bouton(162, h, 152, KEY_B, T("Quitter", "Quit"));
    h -= HAUTEUR_BOUTON + 4;
    bouton(6, h, 152, KEY_START, T("Tout synchroniser", "Sync all"));
    bouton(162, h, 152, KEY_Y, T("Rafraîchir", "Refresh"));
    if (gb) {
        int fwd = !strncmp(j->sav, FORWARDERS, strlen(FORWARDERS));
        h -= HAUTEUR_BOUTON + 4;
        snprintf(t, sizeof t, T("Jouer avec %s", "Play with %s"), fwd ? (j->systeme == SYS_GBA ? "open_agb_firm" : "TWiLight") : T("le forwarder mGBA", "the mGBA forwarder"));
        bouton(6, h, 308, KEY_X, t);
    }
    if (action) {
        h -= HAUTEUR_BOUTON + 4;
        bouton(6, h, 308, KEY_A, j->etat == A_ENVOYER ? T("Envoyer au serveur", "Upload to the server")
                                 : j->etat == A_RECUPERER ? T("Récupérer du serveur", "Download from the server") : T("Résoudre le conflit", "Resolve the conflict"));
    }
}

static void dessiner(int curseur)
{
    ecran_debut();
    dessiner_haut(curseur);
    dessiner_bas(nb_visibles ? &jeux[visibles[curseur]] : NULL);
    ecran_finir();
}

/* Attend A, B ou X. */
static u32 attendre_touche(void)
{
    while (aptMainLoop()) {
        u32 k;
        hidScanInput();
        k = hidKeysDown() & (KEY_A | KEY_B | KEY_X);
        if (k) return k;
        gspWaitForVBlank();
    }
    return KEY_B;
}

static int confirmer(const char *question)
{
    consoleSelect(&bas);
    printf("\x1b[2J\x1b[1;1H%s\n\n%s", question, T("A = oui    B = non", "A = yes    B = no"));
    return attendre_touche() == KEY_A;
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

/* Touches appuyées (avec répétition), en attendant la suivante. */
static u32 touche(void)
{
    u32 k = 0;
    while (!k && aptMainLoop()) {
        gspWaitForVBlank();
        hidScanInput();
        k = hidKeysDownRepeat();
    }
    return k ? k : KEY_B;
}

/* GET /saves : résultat du test dans message. */
static void tester_connexion(void)
{
    char url[300], *corps;
    size_t taille;
    long statut;
    progression("%s", T("Test de la connexion...", "Testing the connection..."));
    snprintf(url, sizeof url, "%s/saves", cfg.url);
    statut = net_get(url, cfg.jeton, &corps, &taille);
    if (statut > 0) free(corps);
    if (statut == 200) snprintf(message, sizeof message, "%s", T(VERT "Connexion reussie." NORMAL, VERT "Connection OK." NORMAL));
    else if (statut == 401) snprintf(message, sizeof message, "%s", T(ROUGE "Jeton refuse (401)." NORMAL, ROUGE "Token refused (401)." NORMAL));
    else if (statut < 0) snprintf(message, sizeof message, T(ROUGE "Serveur injoignable :" NORMAL "\n%s", ROUGE "Server unreachable:" NORMAL "\n%s"), net_erreur(statut));
    else snprintf(message, sizeof message, ROUGE "HTTP %ld" NORMAL, statut);
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

    progression("%s", T("Lecture des jeux installes...", "Reading installed games..."));
    if (R_FAILED(amInit())) {
        snprintf(message, sizeof message, "%s", T("Service AM indisponible : liste impossible.", "AM service unavailable: no list."));
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
            progression(T("Lecture des jeux : %d/%d", "Reading games: %d/%d"), i + 1, n);
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
        int debut = curseur < LIGNES ? 0 : curseur - LIGNES + 1;
        u32 k;
        consoleSelect(&haut);
        printf("\x1b[2J\x1b[1;1H%s\n\n", T("Jeux 3DS a synchroniser", "3DS games to sync"));
        for (i = debut; i < n && i < debut + LIGNES; i++)
            printf("%s%s %-44.44s" NORMAL "\n", i == curseur ? SURLIGNE : c[i].coche ? VERT : c[i].seul == 1 ? "" : GRIS,
                   c[i].coche ? "[x]" : "[ ]", c[i].nom);
        consoleSelect(&bas);
        printf("\x1b[2J\x1b[1;1H");
        if (n) {
            printf("%.38s\n%016llX\n\n", c[curseur].nom, c[curseur].id);
            printf("%s\n", c[curseur].seul == 1 ? T("Sauvegarde : un seul fichier /main.\nSynchronisable.", "Save: a single /main file.\nCan be synced.")
                         : c[curseur].seul == 0 ? T(ROUGE "Plusieurs fichiers de sauvegarde :\nnon gere." NORMAL, ROUGE "Several save files:\nnot supported." NORMAL)
                                                : T("Pas encore de sauvegarde :\nlance le jeu une fois et sauvegarde.", "No save yet:\nstart the game once and save."));
        } else {
            printf("%s\n", T("Aucun jeu trouve.", "No game found."));
        }
        printf("\n%.300s\n", message);
        printf("\x1b[24;1H" GRIS "%s" NORMAL, T("Un jeu absent ? Lance-le une fois\net sauvegarde.\nA cocher/decocher\nB enregistrer et revenir",
                                                "A game is missing? Start it once\nand save.\nA tick/untick\nB save and go back"));
        k = touche();
        if (k & KEY_B) break;
        if (k & KEY_DOWN && curseur + 1 < n) curseur++;
        if (k & KEY_UP && curseur > 0) curseur--;
        if (k & KEY_A && n) {
            message[0] = '\0';
            if (c[curseur].coche || c[curseur].seul == 1) {
                c[curseur].coche = !c[curseur].coche;
                change = 1;
            } else {
                snprintf(message, sizeof message, "%s", T("Ce jeu ne peut pas etre coche.", "This game cannot be ticked."));
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
                 titres_ecrire(DOSSIER "/titres.ini", titres, nb_titres) ? T("titres.ini : ecriture impossible", "titres.ini: write failed")
                                                                          : T("Liste des jeux 3DS enregistree.", "3DS game list saved."));
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
        { "Serveur", "Server", "Adresse du serveur, par exemple\nhttps://saves.exemple.fr\nou le serveur local du PC :\nhttp://192.168.1.20:8765",
          "Server address, for example\nhttps://saves.example.com\nor the PC local server:\nhttp://192.168.1.20:8765", cfg.url, sizeof cfg.url },
        { "Jeton", "Token", "Le jeton du serveur (le meme que\ndans la configuration du PC).", "The server token (the same as\nin the PC configuration).",
          cfg.jeton, sizeof cfg.jeton },
        { "ROM DS", "DS ROMs", "Dossier des ROM DS.\nTWiLight : sdmc:/roms/nds", "DS ROM folder.\nTWiLight: sdmc:/roms/nds", cfg.roms, sizeof cfg.roms },
        { "Sauvegardes DS", "DS saves", "TWiLight : sdmc:/roms/nds/saves\nVide = a cote de la ROM.", "TWiLight: sdmc:/roms/nds/saves\nEmpty = next to the ROM.",
          cfg.saves, sizeof cfg.saves },
        { "ROM GB", "GB ROMs", "Dossier des ROM GB et GBC.\nTWiLight : sdmc:/roms/gb", "GB and GBC ROM folder.\nTWiLight: sdmc:/roms/gb", cfg.roms_gb,
          sizeof cfg.roms_gb },
        { "Sauvegardes GB", "GB saves", "Vide = a cote de la ROM (TWiLight).\nLa sauvegarde d'un forwarder mGBA\nprime si elle existe.",
          "Empty = next to the ROM (TWiLight).\nAn mGBA forwarder save wins\nif it exists.", cfg.saves_gb, sizeof cfg.saves_gb },
        { "ROM GBA", "GBA ROMs", "Dossier des ROM GBA.\nExemple : sdmc:/roms/gba", "GBA ROM folder.\nExample: sdmc:/roms/gba", cfg.roms_gba, sizeof cfg.roms_gba },
        { "Sauvegardes GBA", "GBA saves", "open_agb_firm (useSavesFolder=true) :\nsdmc:/3ds/open_agb_firm/saves\nVide = a cote de la ROM.",
          "open_agb_firm (useSavesFolder=true):\nsdmc:/3ds/open_agb_firm/saves\nEmpty = next to the ROM.", cfg.saves_gba, sizeof cfg.saves_gba },
    };
    const int nb = (int)(sizeof l / sizeof *l), total = nb + 2;
    int c = 0, i, change;
    static titre_t avant_titres[MAX_TITRES];
    int avant_nb = nb_titres;
    char avant[sizeof cfg];

    memcpy(avant, &cfg, sizeof cfg);
    memcpy(avant_titres, titres, sizeof titres);

    message[0] = '\0';
    while (aptMainLoop()) {
        u32 k;
        consoleSelect(&haut);
        printf("\x1b[2J\x1b[1;1H3dssavesync - %s\n\n", T("Reglages", "Settings"));
        for (i = 0; i < total; i++) {
            const char *v = i < nb ? l[i].valeur : "";
            char nb3ds[16];
            if (i == 1 && v[0]) v = "********";
            else if (i < nb && !v[0]) v = i == 0 || i == 1 ? T("(vide)", "(empty)") : T("(a cote de la ROM)", "(next to the ROM)");
            if (i == nb) {
                snprintf(nb3ds, sizeof nb3ds, "%d", nb_titres);
                v = nb3ds;
            }
            printf("%s%-16s %-31.31s" NORMAL "\n", i == c ? SURLIGNE : "",
                   i < nb ? T(l[i].nom, l[i].nom_en) : i == nb ? T("Jeux 3DS", "3DS games") : T("Tester la connexion", "Test the connection"), v);
            if (i == nb - 1 || i == 1) printf("\n");
        }
        consoleSelect(&bas);
        printf("\x1b[2J\x1b[1;1H");
        if (premier) printf("%s\n\n", T(JAUNE "Premier lancement : entre au moins\nle serveur et le jeton." NORMAL,
                                         JAUNE "First start: enter at least\nthe server and the token." NORMAL));
        printf("%s\n", c < nb ? T(l[c].aide, l[c].aide_en)
                       : c == nb ? T("Jeux 3DS natifs a synchroniser.\nPar defaut : les 8 Pokemon 3DS.", "Native 3DS games to sync.\nDefault: the 8 3DS Pokemon.")
                                 : T("Verifie l'adresse et le jeton\n(GET /saves).", "Checks the address and the token\n(GET /saves)."));
        printf("\n%.300s\n", message);
        printf("\x1b[26;1H" GRIS "%s" NORMAL, T("A modifier\nB enregistrer et revenir", "A change\nB save and go back"));
        k = touche();
        if (k & KEY_B) break;
        if (k & KEY_DOWN && c + 1 < total) c++;
        if (k & KEY_UP && c > 0) c--;
        if (k & KEY_A) {
            message[0] = '\0';
            if (c < nb) {
                size_t len;
                editer(T(l[c].nom, l[c].nom_en), l[c].valeur, l[c].n, c == 1);
                len = strlen(l[c].valeur);
                while (len > 0 && l[c].valeur[len - 1] == '/') l[c].valeur[--len] = '\0'; /* "…/saves/" -> "…/saves" */
            } else if (c == nb) {
                choisir_titres();
            } else {
                tester_connexion();
            }
        }
    }
    message[0] = '\0';
    change = memcmp(avant, &cfg, sizeof cfg) || avant_nb != nb_titres || memcmp(avant_titres, titres, sizeof titres);
    if (memcmp(avant, &cfg, sizeof cfg)) ecrire_config();
    return change;
}

/* A sur un Jeu : son action, avec confirmation avant tout écrasement (Q5). 1 si une action a été tentée. */
static int agir(jeu_t *j)
{
    char q[400];
    int fait = 0;
    switch (j->etat) {
    case A_ENVOYER:
        envoyer(j);
        fait = 1;
        break;
    case A_RECUPERER:
        snprintf(q, sizeof q, T("Recuperer la Version du serveur pour\n" CYAN "%.38s" NORMAL " ?\n\n%s",
                                "Download the server version for\n" CYAN "%.38s" NORMAL "?\n\n%s"), j->court,
                 j->a_local ? T("La Sauvegarde de la 3DS sera remplacee\n(Copie de securite faite avant).",
                                "The 3DS save will be replaced\n(backup copy made first).") : "");
        if ((fait = confirmer(q))) recuperer(j);
        break;
    case CONFLIT:
        format_age(j->age, q, sizeof q);
        consoleSelect(&bas);
        printf(T("\x1b[2J\x1b[1;1H" ROUGE "CONFLIT" NORMAL " %.30s\n\n"
                 "La 3DS et le serveur ont change\ndepuis la derniere Synchronisation.\n\n"
                 "3DS     : %7lu o  %.8s\nServeur : %7lu o  %.8s\n          %s, %s\n\n"
                 "A garder la 3DS (l'envoyer)\nX prendre le serveur (copie avant)\nB ne rien faire\n\n"
                 GRIS "L'autre version reste dans\nl'Historique du serveur." NORMAL,
                 "\x1b[2J\x1b[1;1H" ROUGE "CONFLICT" NORMAL " %.30s\n\n"
                 "The 3DS and the server both changed\nsince the last sync.\n\n"
                 "3DS     : %7lu B  %.8s\nServer  : %7lu B  %.8s\n          %s, %s\n\n"
                 "A keep the 3DS (upload it)\nX take the server (backup first)\nB do nothing\n\n"
                 GRIS "The other version stays in\nthe server history." NORMAL),
               j->court, j->local.taille, j->hash_local, j->serveur.taille, j->hash_serveur, j->client, q);
        dessiner_icone(j, 320 - 32 - 6, 6);
        switch (attendre_touche()) {
        case KEY_A: fait = 1; envoyer(j); break;
        case KEY_X: fait = 1; recuperer(j); break;
        default: snprintf(message, sizeof message, T("%s : Conflit laisse tel quel", "%s: conflict left as is"), j->court);
        }
        break;
    case DOUBLON:
        snprintf(message, sizeof message, T("%s : %d ROM ont cette Cle. Garde-en une seule.", "%s: %d ROMs have this key. Keep only one."), j->court, j->nb_roms);
        break;
    default:
        snprintf(message, sizeof message, T("%s : rien à faire, déjà à jour.", "%s: nothing to do, already in sync."), j->court);
    }
    if (fait) lire_local(j);
    return fait;
}

/* X sur un Jeu GB/GBA : sa Sauvegarde passe du forwarder mGBA au .sav à côté de la ROM (open_agb_firm,
 * TWiLight), ou l'inverse. Le fichier du forwarder prime dès qu'il existe (scanner_dossier) : le retirer
 * suffit à revenir côté ROM. Copie de sécurité du fichier remplacé (fichiers.h). */
static int basculer(jeu_t *j)
{
    char rom_sav[600], fwd[600], copie[700], date[20], q[600];
    const char *ext = strrchr(j->rom, '.');
    const char *autre = j->systeme == SYS_GBA ? "open_agb_firm" : "TWiLight Menu++";
    int vers_rom;
    time_t t = time(NULL);

    if ((j->systeme != SYS_GB && j->systeme != SYS_GBA) || !ext) {
        snprintf(message, sizeof message, "%s", T("X : seulement pour les jeux GB et GBA.", "X: GB and GBA games only."));
        return 0;
    }
    snprintf(rom_sav, sizeof rom_sav, "%.*s.sav", (int)(ext - j->rom), j->rom);
    snprintf(fwd, sizeof fwd, FORWARDERS "/%s.sav", j->nom);
    vers_rom = !strcmp(j->sav, fwd);
    snprintf(q, sizeof q,
             T(CYAN "%.38s" NORMAL "\n\n"
               "Sur la 3DS, ce jeu peut se jouer avec\nTWiLight Menu++ ou open_agb_firm\n(sauvegarde a cote de la ROM), ou avec\nun forwarder mGBA (icone sur le menu\nHOME, sauvegarde dans sdmc:/mGBA).\n"
               "3dssavesync synchronise une seule de\nces deux sauvegardes.\n\n"
               "Aujourd'hui : " CYAN "%s" NORMAL "\nPasser a : " CYAN "%s" NORMAL " ?\n\n"
               "La sauvegarde y sera copiee (copie de\nsecurite faite avant).%s",
               CYAN "%.38s" NORMAL "\n\n"
               "On the 3DS, this game is played with\nTWiLight Menu++ or open_agb_firm\n(save next to the ROM), or with an mGBA\nforwarder (HOME menu icon, save in\nsdmc:/mGBA).\n"
               "3dssavesync syncs only one of these\ntwo saves.\n\n"
               "Now: " CYAN "%s" NORMAL "\nSwitch to: " CYAN "%s" NORMAL "?\n\n"
               "The save will be copied there (backup\ncopy made first).%s"),
             j->court, vers_rom ? T("forwarder mGBA", "mGBA forwarder") : autre, vers_rom ? autre : T("forwarder mGBA", "mGBA forwarder"),
             vers_rom ? T("\nLe forwarder n'aura plus de sauvegarde.", "\nThe forwarder will have no save left.") : "");
    if (!confirmer(q)) return 0;
    strftime(date, sizeof date, "%Y%m%d-%H%M%S", localtime(&t));
    mkdir(DOSSIER "/copies", 0777);
    snprintf(copie, sizeof copie, DOSSIER "/copies/%s-%s.sav", j->nom, date);
    mkdir("sdmc:/mGBA", 0777);
    mkdir(FORWARDERS, 0777);
    if (deplacer_sauvegarde(j->sav, vers_rom ? rom_sav : fwd, copie, vers_rom))
        snprintf(message, sizeof message, T("%s : bascule impossible, rien n'est perdu", "%s: switch failed, nothing lost"), j->court);
    else
        snprintf(message, sizeof message, T("%s : se joue maintenant avec %s.", "%s: now played with %s."), j->court, vers_rom ? autre : T("le forwarder mGBA", "the mGBA forwarder"));
    lire_local(j);
    return 1;
}

/* START : Envois et Récupérations ; ignore Conflits et Doublons ; une confirmation pour les écrasements. */
static int tout_synchroniser(void)
{
    char q[1200];
    int i, ecrase = 0, envois = 0, recups = 0, ignores = 0, ok_ecraser = 1, total = 0, fait = 0;

    snprintf(q, sizeof q, "%s", T("Ces Sauvegardes de la 3DS seront remplacees\n(Copie de securite faite avant) :\n\n",
                                  "These 3DS saves will be replaced\n(backup copy made first):\n\n"));

    for (i = 0; i < nb_visibles; i++) {
        jeu_t *j = &jeux[visibles[i]];
        if (j->etat == A_RECUPERER && j->a_local && ecrase++ < 12) {
            strncat(q, j->court, 38);
            strcat(q, "\n");
        }
    }
    if (ecrase > 12) snprintf(q + strlen(q), sizeof q - strlen(q), T("... et %d autre(s)\n", "... and %d more\n"), ecrase - 12);
    if (ecrase) ok_ecraser = confirmer(q);
    for (i = 0; i < nb_visibles; i++) {
        jeu_t *j = &jeux[visibles[i]];
        total += j->etat == A_ENVOYER || (j->etat == A_RECUPERER && (ok_ecraser || !j->a_local));
    }
    for (i = 0; i < nb_visibles; i++) {
        jeu_t *j = &jeux[visibles[i]];
        int faire_envoi = j->etat == A_ENVOYER;
        int faire_recup = j->etat == A_RECUPERER && (ok_ecraser || !j->a_local);
        if (faire_envoi || faire_recup) {
            int plein = 30 * ++fait / total;
            progression("%s%d/%d\n\n[" VERT "%.*s" NORMAL "%.*s]\n\n%s%s" NORMAL "\n%.38s", T("Synchronisation ", "Sync "),
                        fait, total, plein, "##############################", 30 - plein, "------------------------------",
                        faire_envoi ? JAUNE : CYAN, faire_envoi ? T("^ Envoi", "^ Upload") : T("v Recuperation", "v Download"), j->court);
        }
        if (faire_envoi) envois += envoyer(j);
        else if (faire_recup) recups += recuperer(j);
        else if (j->etat == CONFLIT || j->etat == DOUBLON) ignores++;
        if (faire_envoi || faire_recup) lire_local(j);
    }
    snprintf(q, sizeof q, "%.200s", message);
    snprintf(message, sizeof message, T("%d envoi(s), %d recuperation(s), %d ignore(s)\nDernier : %s",
                                        "%d upload(s), %d download(s), %d skipped\nLast: %s"),
             envois, recups, ignores, q);
    return total > 0;
}

/* ---------------------------------------------------------------- boucle */

int main(void)
{
    int curseur = 0;

    gfxInitDefault();
    osSetSpeedupEnable(true); /* 804 MHz sur New 3DS (sans effet sur Old 3DS) : TLS et SHA-256 plus rapides */
    consoleInit(GFX_TOP, &haut);
    consoleInit(GFX_BOTTOM, &bas);
    ecran_init(); /* après consoleInit : framebuffers en RGB565 */
    if (R_SUCCEEDED(cfguInit())) {
        CFGU_GetSystemLanguage(&langue);
        cfguExit();
    }
    anglais = langue != CFG_LANGUAGE_FR;
    if (migrer_dossier(ANCIEN, DOSSIER) == 1)
        snprintf(message, sizeof message, T("Reglages et Bases repris de\n%s", "Settings and bases taken from\n%s"), ANCIEN);
    snprintf(cfg.roms, sizeof cfg.roms, "sdmc:/roms/nds");
    snprintf(cfg.saves, sizeof cfg.saves, "sdmc:/roms/nds/saves");
    snprintf(cfg.roms_gb, sizeof cfg.roms_gb, "sdmc:/roms/gb");   /* TWiLight : .sav à côté des ROM */
    snprintf(cfg.roms_gba, sizeof cfg.roms_gba, "sdmc:/roms/gba"); /* open_agb_firm, useSavesFolder=false */
    lire_cles(DOSSIER "/config.ini", cle_config);
    lire_cles(DOSSIER "/base.ini", cle_base);
    nb_titres = titres_lire(DOSSIER "/titres.ini", titres, MAX_TITRES);

    if (net_init()) {
        printf("%s", T("Echec de l'initialisation reseau.\nSTART pour quitter.", "Network setup failed.\nSTART to quit."));
        while (aptMainLoop()) {
            hidScanInput();
            if (hidKeysDown() & KEY_START) break;
            gspWaitForVBlank();
        }
        gfxExit();
        return 0;
    }

    if (!cfg.url[0] || !cfg.jeton[0]) {
        char reprise[512];
        snprintf(reprise, sizeof reprise, "%s", message);
        if (!cfg.url[0]) snprintf(cfg.url, sizeof cfg.url, "https://");
        reglages(1);
        snprintf(message, sizeof message, "%s", reprise);
    }
    consoleSelect(&bas);
    printf("%s", T("Lecture des ROM et du serveur...", "Reading ROMs and the server..."));
    rafraichir();
    while (aptMainLoop()) {
        u32 k;
        dessiner(curseur);
        do {
            gspWaitForVBlank();
            hidScanInput();
            k = hidKeysDownRepeat() & ~KEY_TOUCH;
            if (hidKeysDown() & KEY_TOUCH) k |= bouton_touche();
        } while (!k && aptMainLoop());

        if (k & KEY_B) break;
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
                         T("\nDuree : %llu s", "\nTime: %llu s"), (osGetTime() - debut) / 1000);
            }
            for (i = 0; i < nb_visibles && visibles[i] != choisi; i++);
            curseur = i < nb_visibles ? i : curseur < nb_visibles ? curseur : nb_visibles ? nb_visibles - 1 : 0;
        }
    }
    net_fin();
    ecran_fin();
    gfxExit();
    return 0;
}
