/* Vérifie texte_reseau() et texte_github() : codes curl et HTTP -> phrase pour l'utilisateur.
 * Compilé sur PC : gcc -std=c99 -Wall -I source source/erreurs.c test/test_erreurs.c -o /tmp/e && /tmp/e */
#include <stdio.h>
#include <string.h>
#include "errors.h"

static int echecs, total;

static void cas_texte(const char *t, long statut, int wifi, const char *attendu)
{
    total++;
    if (strcmp(t, attendu)) {
        printf("ECHEC  statut %ld, wifi %d : attendu « %s », obtenu « %s »\n", statut, wifi, attendu, t);
        echecs++;
    }
}

static void cas(long statut, int wifi, int anglais, const char *attendu) { cas_texte(texte_reseau(statut, wifi, anglais), statut, wifi, attendu); }
static void cas_github(long statut, int wifi, int anglais, const char *attendu) { cas_texte(texte_github(statut, wifi, anglais), statut, wifi, attendu); }

int main(void)
{
    char d[100];

    cas(-6, 0, 0, "Pas de réseau : activez le Wi-Fi de la 3DS."); /* Wi-Fi coupé : avant la cause apparente */
    cas(-7, 0, 1, "No network: turn on the 3DS Wi-Fi.");
    cas(-6, 1, 0, "Serveur injoignable : vérifiez l'adresse."); /* CURLE_COULDNT_RESOLVE_HOST */
    cas(-7, 1, 0, "Serveur injoignable : vérifiez l'adresse."); /* CURLE_COULDNT_CONNECT */
    cas(-28, 1, 0, "Le serveur ne répond pas : réessayez plus tard."); /* CURLE_OPERATION_TIMEDOUT */
    cas(-60, 1, 0, "Certificat du serveur refusé : vérifiez la date et l'heure de la 3DS."); /* CURLE_PEER_FAILED_VERIFICATION */
    cas(-60, 1, 1, "Server certificate refused: check the 3DS date and time.");
    cas(-35, 1, 0, "Connexion sécurisée impossible : vérifiez l'adresse, la date et l'heure de la 3DS.");
    cas(-3, 1, 0, "Vérifiez l'adresse du serveur (SELECT)."); /* CURLE_URL_MALFORMAT */
    cas(-56, 1, 0, "Connexion coupée : réessayez.");
    cas(-99, 1, 0, "Erreur réseau : réessayez.");
    cas(401, 1, 0, "Jeton refusé : copiez le jeton exact du serveur (SELECT).");
    cas(401, 0, 0, "Jeton refusé : copiez le jeton exact du serveur (SELECT)."); /* réponse reçue : le Wi-Fi marche */
    cas(404, 1, 0, "Cette adresse n'est pas un serveur 3dssavesync. Vérifiez-la.");
    cas(-28, 1, 1, "Server not responding. Try again later.");
    cas(413, 1, 1, "Save too large for the server.");
    cas(502, 1, 0, "Le serveur a un problème : réessayez plus tard.");
    cas(302, 1, 0, "Réponse inattendue du serveur : vérifiez l'adresse.");
    cas_github(-6, 0, 0, "Pas de réseau : activez le Wi-Fi de la 3DS, puis réessayez.");
    cas_github(-6, 1, 0, "GitHub injoignable : vérifiez l'accès à Internet, puis réessayez.");
    cas_github(-60, 1, 1, "Secure connection to GitHub failed: check the 3DS date and time.");
    cas_github(-18, 1, 0, "Connexion à GitHub coupée : rapprochez-vous du point d'accès, puis réessayez."); /* CURLE_PARTIAL_FILE */
    cas_github(-56, 1, 1, "Connection to GitHub cut: move closer to the access point, then try again.");
    cas_github(-63, 1, 0, "Fichier de mise à jour trop gros, téléchargement annulé."); /* CURLE_FILESIZE_EXCEEDED */
    cas_github(403, 1, 0, "GitHub limite les vérifications : réessayez dans une heure."); /* 60 requêtes par heure sans compte */
    cas_github(429, 1, 1, "GitHub limits the checks: try again in an hour.");
    cas_github(404, 1, 0, "Réponse inattendue de GitHub : réessayez plus tard.");
    cas_github(200, 1, 1, "Unexpected answer from GitHub: try again later."); /* 200 sans tag_name */
    detail_reseau(401, "http://127.0.0.1:8799/saves", d, sizeof d);
    cas_texte(d, 401, 1, "HTTP 401 \xC2\xB7 http://127.0.0.1:8799/saves");
    detail_reseau(-7, "http://127.0.0.1:8799/saves", d, sizeof d);
    cas_texte(d, -7, 1, "curl 7 \xC2\xB7 http://127.0.0.1:8799/saves");
    detail_reseau(-6, "https://saves.example.com/saves/GB-PM_CRYSTAL-BE2C/name", d, sizeof d); /* coupée à 50 */
    cas_texte(d, -6, 1, "curl 6 \xC2\xB7 https://saves.example.com/saves/GB-PM_CRYSTAL-BE2C\xE2\x80\xA6");
    printf("%d/%d cas OK\n", total - echecs, total);
    return echecs != 0;
}
