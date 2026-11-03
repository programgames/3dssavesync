/* Même ordre d'opérations que Checkpoint (io.cpp : écriture, commit, secure value), code écrit ici (ADR 0009). */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "titles3ds.h"

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

int titre_seul_main(u64 id)
{
    static const FS_MediaType supports[2] = { MEDIATYPE_SD, MEDIATYPE_GAME_CARD };
    int i;
    for (i = 0; i < 2; i++) {
        FS_Archive a;
        Handle d;
        FS_DirectoryEntry e[2];
        u32 lus = 0;
        int seul;
        if (R_FAILED(ouvrir(id, supports[i], &a))) continue;
        if (R_FAILED(FSUSER_OpenDirectory(&d, a, fsMakePath(PATH_ASCII, "/")))) {
            FSUSER_CloseArchive(a);
            continue;
        }
        FSDIR_Read(d, &lus, 2, e);
        FSDIR_Close(d);
        FSUSER_CloseArchive(a);
        seul = lus == 1 && !(e[0].attributes & FS_ATTRIBUTE_DIRECTORY) && e[0].name[0] == 'm' && e[0].name[1] == 'a' &&
               e[0].name[2] == 'i' && e[0].name[3] == 'n' && e[0].name[4] == 0;
        return seul;
    }
    return -1;
}

/* UTF-16 (plan de base) -> UTF-8, sans couper un caractère ; retour à la ligne -> espace. */
static void utf8(const u16 *src, size_t max, char *nom, size_t n)
{
    size_t i, j = 0;
    for (i = 0; i < max && src[i]; i++) {
        u16 c = src[i] == '\n' ? ' ' : src[i];
        size_t l = c < 0x80 ? 1 : c < 0x800 ? 2 : 3;
        if (j + l + 1 > n) break;
        if (l == 1) nom[j++] = (char)c;
        else if (l == 2) nom[j++] = (char)(0xC0 | c >> 6), nom[j++] = (char)(0x80 | (c & 0x3F));
        else nom[j++] = (char)(0xE0 | c >> 12), nom[j++] = (char)(0x80 | (c >> 6 & 0x3F)), nom[j++] = (char)(0x80 | (c & 0x3F));
    }
    nom[j] = '\0';
}

/* SMDH du titre (0x36C0 octets) : ExeFS:/icon de ARCHIVE_SAVEDATA_AND_CONTENT (méthode de FBI). 0 si lu. */
static int lire_smdh(u64 id, FS_MediaType media, u16 *smdh)
{
    static const u32 fichier[5] = { 0, 0, 2, 0x6E6F6369, 0 }; /* "icon" */
    u32 archive[4] = { (u32)id, (u32)(id >> 32), media, 0 };
    Handle f;
    u32 lu = 0;
    int ok = 0;
    if (R_SUCCEEDED(FSUSER_OpenFileDirectly(&f, ARCHIVE_SAVEDATA_AND_CONTENT, (FS_Path){ PATH_BINARY, sizeof archive, archive },
                                            (FS_Path){ PATH_BINARY, sizeof fichier, fichier }, FS_OPEN_READ, 0))) {
        ok = R_SUCCEEDED(FSFILE_Read(f, &lu, 0, smdh, 0x36C0)) && lu == 0x36C0 && !memcmp(smdh, "SMDH", 4);
        FSFILE_Close(f);
    }
    return ok ? 0 : -1;
}

int titre_nom(u64 id, FS_MediaType media, int langue, char *nom, size_t n)
{
    /* SMDH : 16 titres de 0x200 octets à 0x08, titre court (0x40 caractères UTF-16) en tête de chacun. */
    u16 *smdh = malloc(0x36C0);
    int ok = smdh && lire_smdh(id, media, smdh) == 0;
    if (ok) {
        const u16 *t = smdh + (8 + 0x200 * (langue & 0xF)) / 2;
        if (!t[0]) t = smdh + (8 + 0x200 * 1) / 2; /* titre vide dans cette langue : anglais */
        utf8(t, 0x40, nom, n);
    }
    free(smdh);
    return ok ? 0 : -1;
}

int titre_icone(u64 id, FS_MediaType media, u16 *pixels)
{
    /* Grande icône du SMDH à 0x24C0 : 48×48 RGB565 en tuiles 8×8, ordre de Morton dans chaque tuile. */
    u16 *smdh = malloc(0x36C0);
    int ok = smdh && lire_smdh(id, media, smdh) == 0, i;
    for (i = 0; ok && i < 48 * 48; i++) {
        int tuile = i >> 6, k = i & 63;
        int x = (tuile % 6) * 8 + ((k & 1) | (k >> 1 & 2) | (k >> 2 & 4)), y = (tuile / 6) * 8 + ((k >> 1 & 1) | (k >> 2 & 2) | (k >> 3 & 4));
        pixels[y * 48 + x] = smdh[0x24C0 / 2 + i];
    }
    free(smdh);
    return ok ? 0 : -1;
}

int titres_installes(u64 *ids, FS_MediaType *medias, int max)
{
    static const FS_MediaType supports[2] = { MEDIATYPE_SD, MEDIATYPE_GAME_CARD };
    int i, n = 0;
    for (i = 0; i < 2; i++) {
        u32 total = 0, lus = 0, k;
        u64 *tous;
        if (R_FAILED(AM_GetTitleCount(supports[i], &total)) || !total || !(tous = malloc(total * sizeof *tous))) continue;
        if (R_SUCCEEDED(AM_GetTitleList(&lus, supports[i], total, tous)))
            for (k = 0; k < lus && n < max; k++)
                if ((tous[k] >> 32) == 0x00040000) { /* applications seulement (ni démos, ni mises à jour, ni DLC) */
                    ids[n] = tous[k];
                    medias[n++] = supports[i];
                }
        free(tous);
    }
    return n;
}
