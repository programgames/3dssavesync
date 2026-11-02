/* Appels HTTPS vers le Serveur avec curl + mbedTLS (ADR 0003, docs/design.md §4). */
#ifndef NET_H
#define NET_H

#include <stddef.h>

/* socInit + sslcInit (entropie de mbedTLS) + curl. 0 si tout va bien. */
int net_init(void);
void net_fin(void);

/* GET url. Renvoie le statut HTTP (> 0) ou -<code curl> (< 0).
 * Si statut > 0 : *corps est alloué, terminé par '\0', à libérer avec free(). */
long net_get(const char *url, const char *jeton, char **corps, size_t *taille);

/* PUT url avec un corps binaire. condition = "If-Match: \"<hash>\"" ou "If-None-Match: *".
 * Renvoie le statut HTTP (> 0) ou -<code curl> (< 0). */
long net_put(const char *url, const char *jeton, const char *condition, const void *octets, size_t taille);

/* 0 si la 3DS n'est connectée à aucun point d'accès Wi-Fi (texte_reseau, erreurs.h). */
int net_wifi(void);

#endif
