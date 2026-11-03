/* Appels HTTPS vers le Serveur avec curl + mbedTLS (ADR 0003, docs/design.md §4). */
#ifndef NET_H
#define NET_H

#include <stddef.h>

/* socInit + sslcInit (entropie de mbedTLS) + curl. 0 si tout va bien. */
int net_init(void);
void net_exit(void);

/* GET url. Renvoie le statut HTTP (> 0) ou -<code curl> (< 0).
 * Si statut > 0 : *corps est alloué, terminé par '\0', à libérer avec free(). */
long net_get(const char *url, const char *token, char **body, size_t *size);

/* GET d'une adresse de GitHub (mises à jour, ADR 0021) : sans jeton, racines de GitHub en plus d'ISRG. Renvoie et alloue
 * comme net_get. avance != NULL : fichier d'une release, redirections suivies, NET_MAX_FILE octets au plus (sinon
 * -63, CURLE_FILESIZE_EXCEEDED), avance(reçus, total) appelée pendant le transfert. */
#define NET_MAX_FILE (8 * 1024 * 1024)
long net_github(const char *url, void (*on_progress)(size_t done, size_t total), char **body, size_t *size);

/* PUT url avec un corps binaire. condition = "If-Match: \"<hash>\"", "If-None-Match: *" ou NULL (nom d'un jeu).
 * Renvoie le statut HTTP (> 0) ou -<code curl> (< 0). */
long net_put(const char *url, const char *token, const char *condition, const void *bytes, size_t size);

/* 0 si la 3DS n'est connectée à aucun point d'accès Wi-Fi (network_text, errors.h). */
int net_wifi(void);

#endif
