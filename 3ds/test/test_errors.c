/* Vérifie network_text() et github_text() : codes curl et HTTP -> phrase pour l'utilisateur.
 * Compilé sur PC : gcc -std=c99 -Wall -I source source/errors.c test/test_errors.c -o /tmp/e && /tmp/e */
#include <stdio.h>
#include <string.h>
#include "errors.h"

static int failures, total;

static void check_text(const char *t, long status, int wifi, const char *expected)
{
    total++;
    if (strcmp(t, expected)) {
        printf("ECHEC  statut %ld, wifi %d : attendu « %s », obtenu « %s »\n", status, wifi, expected, t);
        failures++;
    }
}

static void check(long status, int wifi, int english, const char *expected) { check_text(network_text(status, wifi, english), status, wifi, expected); }
static void check_github(long status, int wifi, int english, const char *expected) { check_text(github_text(status, wifi, english), status, wifi, expected); }

int main(void)
{
    char d[100];

    check(-6, 0, 0, "Pas de réseau : activez le Wi-Fi de la 3DS."); /* Wi-Fi coupé : avant la cause apparente */
    check(-7, 0, 1, "No network: turn on the 3DS Wi-Fi.");
    check(-6, 1, 0, "Serveur injoignable : vérifiez l'adresse."); /* CURLE_COULDNT_RESOLVE_HOST */
    check(-7, 1, 0, "Serveur injoignable : vérifiez l'adresse."); /* CURLE_COULDNT_CONNECT */
    check(-28, 1, 0, "Le serveur ne répond pas : réessayez plus tard."); /* CURLE_OPERATION_TIMEDOUT */
    check(-60, 1, 0, "Certificat du serveur refusé : vérifiez la date et l'heure de la 3DS."); /* CURLE_PEER_FAILED_VERIFICATION */
    check(-60, 1, 1, "Server certificate refused: check the 3DS date and time.");
    check(-35, 1, 0, "Connexion sécurisée impossible : vérifiez l'adresse, la date et l'heure de la 3DS.");
    check(-3, 1, 0, "Vérifiez l'adresse du serveur (SELECT)."); /* CURLE_URL_MALFORMAT */
    check(-56, 1, 0, "Connexion coupée : réessayez.");
    check(-99, 1, 0, "Erreur réseau : réessayez.");
    check(401, 1, 0, "Jeton refusé : copiez le jeton exact du serveur (SELECT).");
    check(401, 0, 0, "Jeton refusé : copiez le jeton exact du serveur (SELECT)."); /* réponse reçue : le Wi-Fi marche */
    check(404, 1, 0, "Cette adresse n'est pas un serveur 3dssavesync. Vérifiez-la.");
    check(-28, 1, 1, "Server not responding. Try again later.");
    check(413, 1, 1, "Save too large for the server.");
    check(502, 1, 0, "Le serveur a un problème : réessayez plus tard.");
    check(302, 1, 0, "Réponse inattendue du serveur : vérifiez l'adresse.");
    check_github(-6, 0, 0, "Pas de réseau : activez le Wi-Fi de la 3DS, puis réessayez.");
    check_github(-6, 1, 0, "GitHub injoignable : vérifiez l'accès à Internet, puis réessayez.");
    check_github(-60, 1, 1, "Secure connection to GitHub failed: check the 3DS date and time.");
    check_github(-18, 1, 0, "Connexion à GitHub coupée : rapprochez-vous du point d'accès, puis réessayez."); /* CURLE_PARTIAL_FILE */
    check_github(-56, 1, 1, "Connection to GitHub cut: move closer to the access point, then try again.");
    check_github(-63, 1, 0, "Fichier de mise à jour trop gros, téléchargement annulé."); /* CURLE_FILESIZE_EXCEEDED */
    check_github(403, 1, 0, "GitHub limite les vérifications : réessayez dans une heure."); /* 60 requêtes par heure sans compte */
    check_github(429, 1, 1, "GitHub limits the checks: try again in an hour.");
    check_github(404, 1, 0, "Réponse inattendue de GitHub : réessayez plus tard.");
    check_github(200, 1, 1, "Unexpected answer from GitHub: try again later."); /* 200 sans tag_name */
    network_detail(401, "http://127.0.0.1:8799/saves", d, sizeof d);
    check_text(d, 401, 1, "HTTP 401 \xC2\xB7 http://127.0.0.1:8799/saves");
    network_detail(-7, "http://127.0.0.1:8799/saves", d, sizeof d);
    check_text(d, -7, 1, "curl 7 \xC2\xB7 http://127.0.0.1:8799/saves");
    network_detail(-6, "https://saves.example.com/saves/GB-PM_CRYSTAL-BE2C/name", d, sizeof d); /* coupée à 50 */
    check_text(d, -6, 1, "curl 6 \xC2\xB7 https://saves.example.com/saves/GB-PM_CRYSTAL-BE2C\xE2\x80\xA6");
    printf("%d/%d cas OK\n", total - failures, total);
    return failures != 0;
}
