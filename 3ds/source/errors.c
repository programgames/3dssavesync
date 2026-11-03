/* Texts of failed requests (errors.h). No curl.h: tested on the host (test/test_errors.c). */
#include <stdio.h>
#include <string.h>
#include "errors.h"

#define T(fr, en) (english ? (en) : (fr))

/* CURLcode values (curl/curl.h), fixed by the libcurl ABI; net.c checks that they match. */
enum {
    UNSUPPORTED_PROTOCOL = 1, URL_MALFORMAT = 3, COULDNT_RESOLVE = 6, COULDNT_CONNECT = 7, PARTIAL_FILE = 18, OUT_OF_MEMORY = 27, TIMED_OUT = 28, TLS = 35, GOT_NOTHING = 52,
    SEND_ERROR = 55, RECV_ERROR = 56, CERT_CLIENT = 58, PEER_FAILED_VERIFICATION = 60, FILESIZE_EXCEEDED = 63, CA = 77, PINNED_PUBKEY = 90,
    OCSP = 91
};

const char *network_text(long status, int wifi, int english)
{
    if (status < 0 && !wifi) return T("Pas de réseau : activez le Wi-Fi de la 3DS.", "No network: turn on the 3DS Wi-Fi.");
    switch (status) {
    case -UNSUPPORTED_PROTOCOL:
    case -URL_MALFORMAT: return T("Vérifiez l'adresse du serveur (SELECT).", "Check the server address (SELECT).");
    case -COULDNT_RESOLVE:
    case -COULDNT_CONNECT: return T("Serveur injoignable : vérifiez l'adresse.", "Server unreachable: check the address.");
    case -TIMED_OUT: return T("Le serveur ne répond pas : réessayez plus tard.", "Server not responding. Try again later.");
    case -TLS: return T("Connexion sécurisée impossible : vérifiez l'adresse, la date et l'heure de la 3DS.",
                        "Secure connection failed: check the address, and the 3DS date and time.");
    case -CERT_CLIENT:
    case -PEER_FAILED_VERIFICATION:
    case -CA:
    case -PINNED_PUBKEY:
    case -OCSP: return T("Certificat du serveur refusé : vérifiez la date et l'heure de la 3DS.", "Server certificate refused: check the 3DS date and time.");
    case -GOT_NOTHING:
    case -SEND_ERROR:
    case -RECV_ERROR: return T("Connexion coupée : réessayez.", "Connection cut: try again.");
    case -OUT_OF_MEMORY: return T("Mémoire insuffisante : relancez 3dssavesync.", "Out of memory: restart 3dssavesync.");
    case 400: return T("Le serveur refuse cette sauvegarde : réessayez.", "The server refuses this save: try again.");
    case 401: return T("Jeton refusé : copiez le jeton exact du serveur (SELECT).", "Token refused: copy the exact server token (SELECT).");
    case 404: return T("Cette adresse n'est pas un serveur 3dssavesync. Vérifiez-la.", "This address isn't a 3dssavesync server. Check it.");
    case 413: return T("Sauvegarde trop grande pour le serveur.", "Save too large for the server.");
    }
    if (status >= 500) return T("Le serveur a un problème : réessayez plus tard.", "The server has a problem: try again later.");
    if (status > 0) return T("Réponse inattendue du serveur : vérifiez l'adresse.", "Unexpected server answer: check the address.");
    return T("Erreur réseau : réessayez.", "Network error: try again.");
}

const char *github_text(long status, int wifi, int english)
{
    if (status < 0 && !wifi) return T("Pas de réseau : activez le Wi-Fi de la 3DS, puis réessayez.", "No network: turn on the 3DS Wi-Fi, then try again.");
    switch (status) {
    case -TLS:
    case -CERT_CLIENT:
    case -PEER_FAILED_VERIFICATION:
    case -CA:
    case -PINNED_PUBKEY:
    case -OCSP: return T("Connexion sécurisée à GitHub impossible : vérifiez la date et l'heure de la 3DS.",
                         "Secure connection to GitHub failed: check the 3DS date and time.");
    case -PARTIAL_FILE:
    case -TIMED_OUT:
    case -GOT_NOTHING:
    case -SEND_ERROR:
    case -RECV_ERROR: return T("Connexion à GitHub coupée : rapprochez-vous du point d'accès, puis réessayez.",
                              "Connection to GitHub cut: move closer to the access point, then try again.");
    case -FILESIZE_EXCEEDED: return T("Fichier de mise à jour trop gros, téléchargement annulé.", "Update file too large. Download canceled.");
    case -OUT_OF_MEMORY: return T("Mémoire insuffisante : relancez 3dssavesync.", "Out of memory: restart 3dssavesync.");
    case 403:
    case 429: return T("GitHub limite les vérifications : réessayez dans une heure.", "GitHub limits the checks: try again in an hour.");
    }
    if (status > 0) return T("Réponse inattendue de GitHub : réessayez plus tard.", "Unexpected answer from GitHub: try again later.");
    return T("GitHub injoignable : vérifiez l'accès à Internet, puis réessayez.", "GitHub unreachable: check the Internet access, then try again.");
}

void network_detail(long status, const char *url, char *out, size_t n)
{
    snprintf(out, n, "%s %ld \xC2\xB7 %.50s%s", status < 0 ? "curl" : "HTTP", status < 0 ? -status : status, url, strlen(url) > 50 ? "\xE2\x80\xA6" : "");
}
