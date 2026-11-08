/* Checks the texts of the action results: the expected texts are those the 3DS showed before result.c (v3.6).
 * Built on the PC: gcc -std=c99 -I source source/errors.c source/text.c source/result.c test/test_result.c -o /tmp/s && /tmp/s */
#include <stdio.h>
#include <string.h>
#include "result.h"

static int failures;

static void expect(result_t r, int english, result_kind_t kind, const char *text, const char *detail)
{
    char t[512], d[200];
    result_kind_t k = result_text(&r, "Zelda", english, t, sizeof t, d, sizeof d);
    if (k != kind || strcmp(t, text) || strcmp(d, detail)) {
        printf("FAIL  code %d: got %d \"%s\" / \"%s\"\n      expected %d \"%s\" / \"%s\"\n", r.code, k, t, d, kind, text, detail);
        failures++;
    }
}

static void summary(int uploads, int downloads, int failed, int skipped, int english, const char *expected)
{
    char s[120];
    sync_summary(s, sizeof s, uploads, downloads, failed, skipped, english);
    if (strcmp(s, expected)) {
        printf("FAIL  summary: \"%s\"\n", s);
        failures++;
    }
}

static result_t make(result_code_t code, const char *path)
{
    result_t r = { code, 0, 1, 0, 0, NULL, "" };
    snprintf(r.path, sizeof r.path, "%s", path);
    return r;
}

#define SAV "sdmc:/roms/nds/saves/Zelda.sav"
#define URL "https://saves.example.com/saves/AZEP"

int main(void)
{
    result_t r;

    expect(make(RESULT_NONE, ""), 0, KIND_INFO, "", "");
    expect(make(RESULT_UPLOADED, ""), 0, KIND_SUCCESS, "Zelda : envoyé au serveur.", "");
    expect(make(RESULT_UPLOADED, ""), 1, KIND_SUCCESS, "Zelda: uploaded.", "");
    expect(make(RESULT_UPLOAD_UNREADABLE, ""), 0, KIND_FAILURE, "Zelda : sauvegarde illisible. Vérifiez la carte SD.", "");
    expect(make(RESULT_UPLOAD_CONFLICT, ""), 1, KIND_FAILURE, "Zelda: conflict, the server changed meanwhile.", "");
    r = make(RESULT_UPLOAD_FAILED, URL);
    r.status = 401;
    expect(r, 0, KIND_FAILURE, "Zelda : envoi impossible. Jeton refusé : copiez le jeton exact du serveur (SELECT).",
           "HTTP 401 \xC2\xB7 https://saves.example.com/saves/AZEP");
    expect(make(RESULT_DOWNLOADED, ""), 0, KIND_SUCCESS, "Zelda : récupéré du serveur.", "");
    r = make(RESULT_DOWNLOAD_FAILED, URL);
    r.status = -7;
    r.wifi = 0;
    expect(r, 0, KIND_FAILURE, "Zelda : récupération impossible. Pas de réseau : activez le Wi-Fi de la 3DS. La sauvegarde de la 3DS reste intacte.",
           "curl 7 \xC2\xB7 https://saves.example.com/saves/AZEP");
    r.wifi = 1;
    expect(r, 1, KIND_FAILURE, "Zelda: download failed. Server unreachable: check the address. The 3DS save is intact.",
           "curl 7 \xC2\xB7 https://saves.example.com/saves/AZEP");
    expect(make(RESULT_DOWNLOAD_DAMAGED, ""), 0, KIND_FAILURE, "Zelda : version reçue abîmée, rien n'est écrit. Réessayez.", "");
    expect(make(RESULT_BACKUP_FAILED, "sdmc:/3ds/3dssavesync/copies/Zelda-20260101-120000.sav"), 0, KIND_FAILURE,
           "Zelda : copie de sécurité impossible, rien n'est écrit. Vérifiez la carte SD.", "sdmc:/3ds/3dssavesync/copies/Zelda-20260101-120000.sav");
    expect(make(RESULT_3DS_WRITE_REFUSED, ""), 0, KIND_FAILURE, "Zelda : écriture 3DS refusée, sauvegarde intacte.", "");
    expect(make(RESULT_3DS_WRITE_REFUSED, "write data/a.bin: C86044BE"), 1, KIND_FAILURE, "Zelda: 3DS write refused, save intact.", "write data/a.bin: C86044BE");
    expect(make(RESULT_3DS_NOT_SYNCED, "path longer than 100 bytes: d/x"), 0, KIND_FAILURE,
           "Zelda : la sauvegarde de ce jeu ne peut pas être copiée en entier. Il n'est pas synchronisé, sa sauvegarde n'est pas modifiée.",
           "path longer than 100 bytes: d/x");
    expect(make(RESULT_OUT_OF_MEMORY, ""), 1, KIND_FAILURE, "Zelda: out of memory, nothing written.", "");
    expect(make(RESULT_WRITE_FAILED, SAV), 0, KIND_FAILURE, "Zelda : écriture impossible, sauvegarde de la 3DS intacte.", SAV);
    r = make(RESULT_WRITTEN_DAMAGED, SAV);
    expect(r, 0, KIND_FAILURE, "Zelda : sauvegarde écrite abîmée. Vérifiez la carte SD, puis récupérez de nouveau.", SAV);
    r.had_save = 1;
    expect(r, 1, KIND_FAILURE, "Zelda: written save damaged. Check the SD card, then download again. Old one copied to /3ds/3dssavesync/copies.", SAV);
    r = make(RESULT_RENAME_FAILED, SAV);
    expect(r, 0, KIND_FAILURE, "Zelda : renommage impossible. Sauvegarde reçue : Zelda.sav.tmp.", "");
    r.had_save = 1;
    expect(r, 0, KIND_FAILURE, "Zelda : renommage impossible. Sauvegarde reçue : Zelda.sav.tmp. Ancienne copiée dans /3ds/3dssavesync/copies.", "");
    r = make(RESULT_RENAME_FAILED, "sdmc:/roms/gb/AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA.sav");
    expect(r, 1, KIND_FAILURE, "Zelda: rename failed. Downloaded save: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA\xE2\x80\xA6.sav.tmp.", "");
    r = make(RESULT_SWITCHED, "");
    expect(r, 0, KIND_SUCCESS, "Zelda : se joue maintenant avec le forwarder mGBA.", "");
    r.program = "open_agb_firm";
    expect(r, 1, KIND_SUCCESS, "Zelda: now played with open_agb_firm.", "");
    expect(make(RESULT_SWITCH_FAILED, SAV), 0, KIND_FAILURE, "Zelda : changement impossible, rien n'est perdu.", SAV);
    expect(make(RESULT_GB_GBA_ONLY, ""), 0, KIND_INFO, "X : seulement pour les ROM GB et GBA de la carte SD.", "");
    expect(make(RESULT_VC_NEVER_STARTED, ""), 0, KIND_INFO,
           "Zelda : ce jeu Virtual Console n'a pas encore de sauvegarde. Lancez-le une fois sur la 3DS, puis synchronisez. Rien n'a été modifié.", "");
    expect(make(RESULT_VC_NEVER_STARTED, ""), 1, KIND_INFO,
           "Zelda: this Virtual Console game has no save yet. Start it once on the 3DS, then sync. Nothing was changed.", "");
    expect(make(RESULT_VC_UNEXPECTED, "sav.dat, SecureValue, rsm2.dat"), 1, KIND_FAILURE,
           "Zelda: this game has restore points (or other files than sav.dat) that could bring an old game back. It is not synced, and its save is not changed.",
           "Files: sav.dat, SecureValue, rsm2.dat");
    expect(make(RESULT_VC_UNEXPECTED, ""), 0, KIND_FAILURE, /* archive no longer readable: no empty technical line */
           "Zelda : ce jeu a des points de restauration (ou d'autres fichiers que sav.dat), qui pourraient ramener une ancienne partie. Il n'est pas synchronisé, sa sauvegarde n'est pas modifiée.", "");
    expect(make(RESULT_VC_NO_KEY, "1:D9004676 2:D9004676"), 1, KIND_FAILURE,
           "Zelda: the ROM of this Virtual Console game can't be read, so its Key is unknown. This game is not synced, and its save is not changed. Report the technical line.",
           "ROM : 1:D9004676 2:D9004676");
    expect(make(RESULT_VC_NO_KEY, "1:D9004676"), 0, KIND_FAILURE,
           "Zelda : la ROM de ce jeu Virtual Console est illisible, sa clé est inconnue. Ce jeu n'est pas synchronisé, sa sauvegarde n'est pas modifiée. Signalez la ligne technique.",
           "ROM : 1:D9004676");
    expect(make(RESULT_CONFLICT_LEFT, ""), 0, KIND_INFO, "Zelda : conflit laissé tel quel.", "");
    r = make(RESULT_DUPLICATE_SHARED, "");
    r.count = 2;
    expect(r, 0, KIND_FAILURE, "Zelda : 2 ROM ont ce nom de fichier et partageraient une sauvegarde. Renommez-en une.", "");
    r.code = RESULT_DUPLICATE;
    r.count = 3;
    expect(r, 1, KIND_FAILURE, "Zelda: 3 ROMs hold this game. Keep one, or add the others to ROM hacks on the PC.", "");
    expect(make(RESULT_OFFLINE, ""), 0, KIND_FAILURE, "Zelda : serveur non lu. Appuyez sur \xEE\x80\x83 pour réessayer.", "");
    expect(make(RESULT_ALREADY_IN_SYNC, ""), 1, KIND_INFO, "Zelda: already in sync.", "");

    if (!result_ok(&(result_t){ .code = RESULT_UPLOADED }) || !result_ok(&(result_t){ .code = RESULT_DOWNLOADED }) ||
        !result_ok(&(result_t){ .code = RESULT_SWITCHED }) || result_ok(&(result_t){ .code = RESULT_UPLOAD_CONFLICT }) ||
        result_ok(&(result_t){ .code = RESULT_NONE })) {
        printf("FAIL  result_ok\n");
        failures++;
    }

    summary(2, 1, 1, 3, 0, "2 envois, 1 récupération, 1 échec, 3 ignorés");
    summary(1, 2, 0, 0, 1, "1 upload, 2 downloads");
    summary(0, 0, 0, 0, 0, "Rien à synchroniser.");

    printf("%s\n", failures ? "FAILED" : "results: OK");
    return failures != 0;
}
