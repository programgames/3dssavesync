/* Mise à jour de l'application depuis les releases GitHub (ADR 0021). Aucune dépendance libctru : testé sur PC
 * (test/test_maj.c). */
#ifndef MAJ_H
#define MAJ_H

typedef struct {
    char version[32];                     /* tag_name sans le « v » : "3.7.0" */
    char cia[512], dsx[512], sommes[512]; /* adresses de 3dssavesync.cia, 3dssavesync.3dsx, SHA256SUMS ; "" si absent */
} release_t;

/* Réponse de GET /repos/<dépôt>/releases/latest. 0 si tag_name est lu (une adresse peut manquer), -1 sinon. */
int maj_release(const char *json, release_t *r);

/* 1 si distante est plus récente que courante : "3.10.0" > "3.9.1", "3.7.0" > "3.7.0-rc.1", « v » initial permis.
 * 0 sinon, et si l'une n'est pas une version X.Y.Z (build "dev" : jamais de mise à jour). */
int maj_plus_recente(const char *courante, const char *distante);

/* Hash de nom dans un fichier SHA256SUMS (format de sha256sum : "<hash>  <nom>" ou "<hash> *<nom>").
 * 0 si trouvé (hash en minuscules), -1 sinon. */
int maj_somme(const char *sommes, const char *nom, char hash[65]);

/* 1 si la vérification automatique est à faire : aucune ce jour (jour = maintenant / 86400, en secondes ; une horloge
 * reculée compte comme un autre jour). */
int maj_a_verifier(long dernier_jour, long long maintenant);

#endif
