/* Même ordre d'opérations que Checkpoint (io.cpp : écriture, commit, secure value), code écrit ici (ADR 0009). */
#include <stdlib.h>
#include "titres3ds.h"

const titre3ds_t TITRES_3DS[NB_TITRES_3DS] = {
    { 0x0004000000055D00ULL, "Pokemon X" },
    { 0x0004000000055E00ULL, "Pokemon Y" },
    { 0x000400000011C400ULL, "Pokemon Rubis Omega" },
    { 0x000400000011C500ULL, "Pokemon Saphir Alpha" },
    { 0x0004000000164800ULL, "Pokemon Soleil" },
    { 0x0004000000175E00ULL, "Pokemon Lune" },
    { 0x00040000001B5000ULL, "Pokemon Ultra-Soleil" },
    { 0x00040000001B5100ULL, "Pokemon Ultra-Lune" },
};

/* Chemin binaire {media, low, high} de ARCHIVE_USER_SAVEDATA (3dbrew, FS:OpenArchive). */
static Result ouvrir(u64 id, FS_MediaType media, FS_Archive *a)
{
    u32 chemin[3] = { media, (u32)id, (u32)(id >> 32) };
    return FSUSER_OpenArchive(a, ARCHIVE_USER_SAVEDATA, (FS_Path){ PATH_BINARY, sizeof chemin, chemin });
}

int titre_lire(u64 id, FS_MediaType *media, unsigned char **octets, size_t *taille)
{
    static const FS_MediaType supports[2] = { MEDIATYPE_SD, MEDIATYPE_GAME_CARD };
    int i;

    for (i = 0; i < 2; i++) {
        FS_Archive a;
        Handle f;
        u64 t = 0;
        u32 lu = 0;
        int ok = 0;

        if (R_FAILED(ouvrir(id, supports[i], &a))) continue;
        if (R_SUCCEEDED(FSUSER_OpenFile(&f, a, fsMakePath(PATH_ASCII, "/main"), FS_OPEN_READ, 0))) {
            if (R_SUCCEEDED(FSFILE_GetSize(f, &t)) && t > 0 && (*octets = malloc(t))) {
                ok = R_SUCCEEDED(FSFILE_Read(f, &lu, 0, *octets, t)) && lu == t;
                if (!ok) free(*octets);
            }
            FSFILE_Close(f);
        }
        FSUSER_CloseArchive(a);
        if (ok) {
            *media = supports[i];
            *taille = t;
            return 0;
        }
    }
    return -1;
}

int titre_ecrire(u64 id, FS_MediaType media, const void *octets, size_t taille)
{
    FS_Archive a;
    Handle f;
    u64 t = 0, valeur;
    u32 ecrit = 0;
    u8 existait;
    Result r;

    if (R_FAILED(ouvrir(id, media, &a))) return -1;
    r = FSUSER_OpenFile(&f, a, fsMakePath(PATH_ASCII, "/main"), FS_OPEN_WRITE, 0);
    if (R_SUCCEEDED(r)) {
        r = FSFILE_GetSize(f, &t);
        if (R_SUCCEEDED(r) && t != taille) r = -2; /* jamais de changement de taille : le jeu ne le prévoit pas */
        if (R_SUCCEEDED(r)) r = FSFILE_Write(f, &ecrit, 0, octets, taille, 0);
        if (R_SUCCEEDED(r) && ecrit != taille) r = -3;
        FSFILE_Close(f);
    }
    if (R_SUCCEEDED(r)) r = FSUSER_ControlArchive(a, ARCHIVE_ACTION_COMMIT_SAVE_DATA, NULL, 0, NULL, 0);
    FSUSER_CloseArchive(a);
    if (R_FAILED(r)) return -1;

    /* Sinon le jeu refuse une sauvegarde restaurée ; il en réécrit une à sa prochaine sauvegarde. */
    valeur = ((u64)SECUREVALUE_SLOT_SD << 32) | ((((u32)id >> 8) & 0xFFFFF) << 8);
    FSUSER_ControlSecureSave(SECURESAVE_ACTION_DELETE, &valeur, sizeof valeur, &existait, 1);
    return 0;
}
