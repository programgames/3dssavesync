/* Texte d'un échec de requête pour l'écran de la 3DS : sans code HTTP ni texte anglais de curl. */
#ifndef ERREURS_H
#define ERREURS_H

/* statut : réponse HTTP (> 0) ou -<code curl> (< 0), comme net_get et net_put. wifi : 0 si la 3DS n'est connectée à
 * aucun point d'accès. Une phrase courte qui dit quoi faire. */
const char *texte_reseau(long statut, int wifi, int anglais);

#endif
