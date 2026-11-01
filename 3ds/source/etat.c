#include <string.h>
#include "etat.h"

static int egal(const char *a, const char *b)
{
    return a && b && strcmp(a, b) == 0;
}

etat_t etat(const obs_t *local, const obs_t *serveur, const char *base)
{
    if (local && serveur && egal(local->hash, serveur->hash)) return A_JOUR;
    if (!serveur) return A_ENVOYER;
    if (!local) return A_RECUPERER;
    if (local->taille != serveur->taille) return CONFLIT;
    if (egal(local->hash, base)) return A_RECUPERER;
    if (egal(serveur->hash, base)) return A_ENVOYER;
    return CONFLIT;
}
