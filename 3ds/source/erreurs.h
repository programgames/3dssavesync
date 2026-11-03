/* Texte d'un échec de requête pour l'écran de la 3DS : sans code HTTP ni texte anglais de curl. */
#ifndef ERREURS_H
#define ERREURS_H

#include <stddef.h>

/* statut : réponse HTTP (> 0) ou -<code curl> (< 0), comme net_get et net_put. wifi : 0 si la 3DS n'est connectée à
 * aucun point d'accès. Une phrase courte qui dit quoi faire. */
const char *texte_reseau(long statut, int wifi, int anglais);

/* Même chose pour GitHub (vérification et téléchargement d'une mise à jour, ADR 0021). */
const char *texte_github(long statut, int wifi, int anglais);

/* Ligne technique d'un échec, sous le texte, pour le signaler : « HTTP 401 · http://… » ou « curl 7 · http://… »
 * (adresse coupée à 50 caractères). */
void detail_reseau(long statut, const char *url, char *out, size_t n);

#endif
