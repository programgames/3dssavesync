/* Texte d'un échec de requête pour l'écran de la 3DS : sans code HTTP ni texte anglais de curl. */
#ifndef ERRORS_H
#define ERRORS_H

#include <stddef.h>

/* statut : réponse HTTP (> 0) ou -<code curl> (< 0), comme net_get et net_put. wifi : 0 si la 3DS n'est connectée à
 * aucun point d'accès. Une phrase courte qui dit quoi faire. */
const char *network_text(long status, int wifi, int english);

/* Même chose pour GitHub (vérification et téléchargement d'une mise à jour, ADR 0021). */
const char *github_text(long status, int wifi, int english);

/* Ligne technique d'un échec, sous le texte, pour le signaler : « HTTP 401 · http://… » ou « curl 7 · http://… »
 * (adresse coupée à 50 caractères). */
void network_detail(long status, const char *url, char *out, size_t n);

#endif
