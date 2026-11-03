/* Vérifie texte_reseau() : codes curl et HTTP -> phrase pour l'utilisateur.
 * Compilé sur PC : gcc -std=c99 -Wall -I source source/erreurs.c test/test_erreurs.c -o /tmp/e && /tmp/e */
#include <stdio.h>
#include <string.h>
#include "erreurs.h"

static int echecs, total;

static void cas(long statut, int wifi, int anglais, const char *attendu)
{
    const char *t = texte_reseau(statut, wifi, anglais);
    total++;
    if (strcmp(t, attendu)) {
        printf("ECHEC  statut %ld, wifi %d : attendu « %s », obtenu « %s »\n", statut, wifi, attendu, t);
        echecs++;
    }
}

int main(void)
{
    cas(-6, 0, 0, "Pas de réseau : activez le Wi-Fi de la 3DS."); /* Wi-Fi coupé : avant la cause apparente */
    cas(-7, 0, 1, "No network: turn on the 3DS Wi-Fi.");
    cas(-6, 1, 0, "Serveur injoignable : vérifiez l'adresse."); /* CURLE_COULDNT_RESOLVE_HOST */
    cas(-7, 1, 0, "Serveur injoignable : vérifiez l'adresse."); /* CURLE_COULDNT_CONNECT */
    cas(-28, 1, 0, "Le serveur ne répond pas : réessayez plus tard."); /* CURLE_OPERATION_TIMEDOUT */
    cas(-60, 1, 0, "Certificat du serveur refusé : vérifiez la date et l'heure de la 3DS."); /* CURLE_PEER_FAILED_VERIFICATION */
    cas(-60, 1, 1, "Server certificate refused: check the 3DS date and time.");
    cas(-35, 1, 0, "Connexion sécurisée impossible : vérifiez l'adresse, la date et l'heure de la 3DS.");
    cas(-3, 1, 0, "Adresse du serveur invalide : vérifiez-la (SELECT)."); /* CURLE_URL_MALFORMAT */
    cas(-56, 1, 0, "Connexion coupée : réessayez.");
    cas(-99, 1, 0, "Erreur réseau : réessayez.");
    cas(401, 1, 0, "Jeton refusé : copiez le jeton exact du serveur (SELECT).");
    cas(401, 0, 0, "Jeton refusé : copiez le jeton exact du serveur (SELECT)."); /* réponse reçue : le Wi-Fi marche */
    cas(404, 1, 0, "Cette adresse ne mène pas à un serveur 3dssavesync : vérifiez-la.");
    cas(413, 1, 1, "Save too large for the server.");
    cas(502, 1, 0, "Le serveur a un problème : réessayez plus tard.");
    cas(302, 1, 0, "Réponse inattendue du serveur : vérifiez l'adresse.");
    printf("%d/%d cas OK\n", total - echecs, total);
    return echecs != 0;
}
