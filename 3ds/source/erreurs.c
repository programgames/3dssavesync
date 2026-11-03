/* Textes des échecs de requête (erreurs.h). Sans curl.h : testé sur l'hôte (test/test_erreurs.c). */
#include "erreurs.h"

#define T(fr, en) (anglais ? (en) : (fr))

/* Valeurs de CURLcode (curl/curl.h), fixes dans l'ABI de libcurl ; net.c vérifie qu'elles correspondent. */
enum {
    PROTOCOLE = 1, URL_MAL = 3, RESOLUTION = 6, CONNEXION = 7, MEMOIRE = 27, DELAI = 28, TLS = 35, REPONSE_VIDE = 52,
    ENVOI = 55, RECEPTION = 56, CERT_CLIENT = 58, CERTIFICAT = 60, CA = 77, CLE_PUBLIQUE = 90, OCSP = 91
};

const char *texte_reseau(long statut, int wifi, int anglais)
{
    if (statut < 0 && !wifi) return T("Pas de réseau : activez le Wi-Fi de la 3DS.", "No network: turn on the 3DS Wi-Fi.");
    switch (statut) {
    case -PROTOCOLE:
    case -URL_MAL: return T("Adresse du serveur invalide : vérifiez-la (SELECT).", "Invalid server address: check it (SELECT).");
    case -RESOLUTION:
    case -CONNEXION: return T("Serveur injoignable : vérifiez l'adresse.", "Server unreachable: check the address.");
    case -DELAI: return T("Le serveur ne répond pas : réessayez plus tard.", "The server does not answer: try again later.");
    case -TLS: return T("Connexion sécurisée impossible : vérifiez l'adresse, la date et l'heure de la 3DS.",
                        "Secure connection failed: check the address, and the 3DS date and time.");
    case -CERT_CLIENT:
    case -CERTIFICAT:
    case -CA:
    case -CLE_PUBLIQUE:
    case -OCSP: return T("Certificat du serveur refusé : vérifiez la date et l'heure de la 3DS.", "Server certificate refused: check the 3DS date and time.");
    case -REPONSE_VIDE:
    case -ENVOI:
    case -RECEPTION: return T("Connexion coupée : réessayez.", "Connection cut: try again.");
    case -MEMOIRE: return T("Mémoire insuffisante : relancez 3dssavesync.", "Out of memory: restart 3dssavesync.");
    case 400: return T("Le serveur refuse cette sauvegarde : réessayez.", "The server refuses this save: try again.");
    case 401: return T("Jeton refusé : copiez le jeton exact du serveur (SELECT).", "Token refused: copy the exact server token (SELECT).");
    case 404: return T("Cette adresse ne mène pas à un serveur 3dssavesync : vérifiez-la.", "This address does not lead to a 3dssavesync server: check it.");
    case 413: return T("Sauvegarde trop grande pour le serveur.", "Save too large for the server.");
    }
    if (statut >= 500) return T("Le serveur a un problème : réessayez plus tard.", "The server has a problem: try again later.");
    if (statut > 0) return T("Réponse inattendue du serveur : vérifiez l'adresse.", "Unexpected server answer: check the address.");
    return T("Erreur réseau : réessayez.", "Network error: try again.");
}
