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

/* GET d'une adresse de GitHub (mises à jour, ADR 0021) : sans jeton, racines de GitHub en plus d'ISRG. Renvoie et alloue
 * comme net_get. avance != NULL : fichier d'une release, redirections suivies, NET_MAX_FICHIER octets au plus (sinon
 * -63, CURLE_FILESIZE_EXCEEDED), avance(reçus, total) appelée pendant le transfert. */
#define NET_MAX_FICHIER (8 * 1024 * 1024)
long net_github(const char *url, void (*avance)(size_t fait, size_t total), char **corps, size_t *taille);

/* PUT url avec un corps binaire. condition = "If-Match: \"<hash>\"", "If-None-Match: *" ou NULL (nom d'un jeu).
 * Renvoie le statut HTTP (> 0) ou -<code curl> (< 0). */
long net_put(const char *url, const char *jeton, const char *condition, const void *octets, size_t taille);

/* 0 si la 3DS n'est connectée à aucun point d'accès Wi-Fi (texte_reseau, erreurs.h). */
int net_wifi(void);

#endif
