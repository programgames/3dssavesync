#include <3ds.h>
#include <curl/curl.h>
#include <malloc.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
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
    if (curl_global_init(CURL_GLOBAL_DEFAULT) != CURLE_OK) return -3;
    return (c = curl_easy_init()) ? 0 : -4;
}

void net_fin(void)
{
    curl_easy_cleanup(c);
    curl_global_cleanup();
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

/* Une requête. envoi != NULL => PUT de ce corps. */
static long requete(const char *url, const char *jeton, const char *condition,
                    const void *envoi, size_t taille_envoi, tampon_t *recu)
{
    char auth[320];
    struct curl_slist *entetes;
    struct curl_blob ca = { (void *)ISRG_ROOT_X1, sizeof ISRG_ROOT_X1 - 1, CURL_BLOB_NOCOPY };
    CURLcode r;
    long statut;

    curl_easy_reset(c); /* remet les options à zéro, garde la connexion ouverte */
    snprintf(auth, sizeof auth, "Authorization: Bearer %s", jeton);
    entetes = curl_slist_append(NULL, auth);
    entetes = curl_slist_append(entetes, "X-Client: 3ds");
    if (envoi) {
        entetes = curl_slist_append(entetes, condition);
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
    curl_easy_setopt(c, CURLOPT_TIMEOUT, 60L);
    curl_easy_setopt(c, CURLOPT_USERAGENT, "3dssavesync-3ds");

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

long net_put(const char *url, const char *jeton, const char *condition, const void *octets, size_t taille)
{
    tampon_t b = { NULL, 0 };
    long statut = requete(url, jeton, condition, octets, taille, &b);
    free(b.octets);
    return statut;
}

const char *net_erreur(long statut)
{
    return curl_easy_strerror((CURLcode)-statut);
}
