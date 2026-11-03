#include <3ds.h>
#include <curl/curl.h>
#include <malloc.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "github_roots.h"
#include "isrg_root_x1.h"
#include "net.h"

#define SOC_TAILLE 0x100000

static u32 *soc_tampon;
static CURL *c; /* un seul handle : curl garde la connexion TLS ouverte entre les requêtes */

int net_init(void)
{
    soc_tampon = memalign(0x1000, SOC_TAILLE);
    if (!soc_tampon || R_FAILED(socInit(soc_tampon, SOC_TAILLE))) return -1;
    if (R_FAILED(sslcInit(0))) return -2;
    acInit(); /* état du Wi-Fi (net_wifi) ; sans lui, le Wi-Fi est supposé présent */
    if (curl_global_init(CURL_GLOBAL_DEFAULT) != CURLE_OK) return -3;
    return (c = curl_easy_init()) ? 0 : -4;
}

void net_fin(void)
{
    curl_easy_cleanup(c);
    curl_global_cleanup();
    acExit();
    sslcExit();
    socExit();
    free(soc_tampon);
}

typedef struct {
    char *octets;
    size_t taille;
} tampon_t;

static size_t recevoir(char *morceau, size_t t, size_t n, void *u)
{
    tampon_t *b = u;
    char *nouveau = realloc(b->octets, b->taille + t * n + 1);
    if (!nouveau) return 0;
    memcpy(nouveau + b->taille, morceau, t * n);
    b->octets = nouveau;
    b->taille += t * n;
    b->octets[b->taille] = '\0';
    return t * n;
}

static void (*avance)(size_t, size_t); /* net_github : téléchargement d'un fichier de release en cours */

static int avancer(void *u, curl_off_t total, curl_off_t fait, curl_off_t total_envoi, curl_off_t envoye)
{
    avance((size_t)fait, (size_t)total);
    return 0;
}

/* Une requête. envoi != NULL => PUT de ce corps. jeton == NULL => GitHub (net_github). */
static long requete(const char *url, const char *jeton, const char *condition,
                    const void *envoi, size_t taille_envoi, tampon_t *recu)
{
    char auth[320];
    struct curl_slist *entetes = NULL;
    static char github[sizeof ISRG_ROOT_X1 + sizeof RACINES_GITHUB]; /* assemblées au premier appel */
    struct curl_blob ca = { (void *)ISRG_ROOT_X1, sizeof ISRG_ROOT_X1 - 1, CURL_BLOB_NOCOPY };
    CURLcode r;
    long statut;

    curl_easy_reset(c); /* remet les options à zéro, garde la connexion ouverte */
    if (jeton) { /* Serveur : ISRG seule (ADR 0003) */
        snprintf(auth, sizeof auth, "Authorization: Bearer %s", jeton);
        entetes = curl_slist_append(NULL, auth);
        entetes = curl_slist_append(entetes, "X-Client: 3ds");
    } else { /* GitHub : sans jeton, ISRG + Sectigo (ADR 0021) */
        if (!github[0]) snprintf(github, sizeof github, "%s%s", ISRG_ROOT_X1, RACINES_GITHUB);
        ca.data = github;
        ca.len = strlen(github);
    }
    if (envoi) {
        if (condition) entetes = curl_slist_append(entetes, condition);
        entetes = curl_slist_append(entetes, "Content-Type: application/octet-stream");
        entetes = curl_slist_append(entetes, "Expect:");
        curl_easy_setopt(c, CURLOPT_CUSTOMREQUEST, "PUT");
        curl_easy_setopt(c, CURLOPT_POSTFIELDSIZE_LARGE, (curl_off_t)taille_envoi);
        curl_easy_setopt(c, CURLOPT_POSTFIELDS, envoi);
    }

    curl_easy_setopt(c, CURLOPT_URL, url);
    curl_easy_setopt(c, CURLOPT_HTTPHEADER, entetes);
    curl_easy_setopt(c, CURLOPT_CAINFO_BLOB, &ca);
    curl_easy_setopt(c, CURLOPT_WRITEFUNCTION, recevoir);
    curl_easy_setopt(c, CURLOPT_WRITEDATA, recu);
    curl_easy_setopt(c, CURLOPT_CONNECTTIMEOUT, 10L);
    curl_easy_setopt(c, CURLOPT_TIMEOUT, avance ? 300L : 60L);
    curl_easy_setopt(c, CURLOPT_USERAGENT, "3dssavesync-3ds"); /* exigé par l'API de GitHub */
    if (avance) { /* github.com renvoie vers release-assets.githubusercontent.com */
        curl_easy_setopt(c, CURLOPT_FOLLOWLOCATION, 1L);
        curl_easy_setopt(c, CURLOPT_MAXREDIRS, 5L);
        curl_easy_setopt(c, CURLOPT_MAXFILESIZE_LARGE, (curl_off_t)NET_MAX_FICHIER);
        curl_easy_setopt(c, CURLOPT_XFERINFOFUNCTION, avancer);
        curl_easy_setopt(c, CURLOPT_NOPROGRESS, 0L);
    }

    r = curl_easy_perform(c);
    statut = -(long)r;
    if (r == CURLE_OK) curl_easy_getinfo(c, CURLINFO_RESPONSE_CODE, &statut);
    curl_slist_free_all(entetes);
    return statut;
}

long net_get(const char *url, const char *jeton, char **corps, size_t *taille)
{
    tampon_t b = { NULL, 0 };
    long statut = requete(url, jeton, NULL, NULL, 0, &b);

    if (statut <= 0) {
        free(b.octets);
        b.octets = NULL;
        b.taille = 0;
    } else if (!b.octets) {
        b.octets = calloc(1, 1);
    }
    *corps = b.octets;
    *taille = b.taille;
    return statut;
}

long net_github(const char *url, void (*avance_fichier)(size_t, size_t), char **corps, size_t *taille)
{
    long statut;
    avance = avance_fichier;
    statut = net_get(url, NULL, corps, taille);
    avance = NULL;
    return statut;
}

long net_put(const char *url, const char *jeton, const char *condition, const void *octets, size_t taille)
{
    tampon_t b = { NULL, 0 };
    long statut = requete(url, jeton, condition, octets, taille, &b);
    free(b.octets);
    return statut;
}

/* erreurs.c lit les codes curl sans curl.h : mêmes valeurs. */
_Static_assert(CURLE_COULDNT_RESOLVE_HOST == 6 && CURLE_COULDNT_CONNECT == 7 && CURLE_OPERATION_TIMEDOUT == 28 &&
               CURLE_SSL_CONNECT_ERROR == 35 && CURLE_PEER_FAILED_VERIFICATION == 60 && CURLE_RECV_ERROR == 56 &&
               CURLE_PARTIAL_FILE == 18 && CURLE_FILESIZE_EXCEEDED == 63,
               "codes curl de erreurs.c");

int net_wifi(void)
{
    u32 statut;
    return R_FAILED(ACU_GetStatus(&statut)) || statut != 1; /* 1 : aucun point d'accès */
}
