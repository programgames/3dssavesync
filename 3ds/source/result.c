#include <stdio.h>
#include <string.h>
#include "errors.h"
#include "result.h"
#include "text.h"

#define T(fr, en) (english ? (en) : (fr))

result_kind_t result_text(const result_t *r, const char *game, int english, char *text, size_t n, char *detail, size_t dn)
{
    const char *old_copied = r->had_save ? T(" Ancienne copiée dans /3ds/3dssavesync/copies.", " Old one copied to /3ds/3dssavesync/copies.") : "";
    text[0] = detail[0] = '\0';
    switch (r->code) {
    case RESULT_NONE: return KIND_INFO;
    case RESULT_UPLOADED:
        snprintf(text, n, T("%s : envoyé au serveur.", "%s: uploaded."), game);
        return KIND_SUCCESS;
    case RESULT_UPLOAD_UNREADABLE:
        snprintf(text, n, T("%s : sauvegarde illisible. Vérifiez la carte SD.", "%s: can't read the save. Check the SD card."), game);
        return KIND_FAILURE;
    case RESULT_UPLOAD_CONFLICT:
        snprintf(text, n, T("%s : conflit, le serveur a changé entre-temps.", "%s: conflict, the server changed meanwhile."), game);
        return KIND_FAILURE;
    case RESULT_UPLOAD_FAILED:
        network_detail(r->status, r->path, detail, dn);
        snprintf(text, n, T("%s : envoi impossible. %s", "%s: upload failed. %s"), game, network_text(r->status, r->wifi, english));
        return KIND_FAILURE;
    case RESULT_DOWNLOADED:
        snprintf(text, n, T("%s : récupéré du serveur.", "%s: downloaded."), game);
        return KIND_SUCCESS;
    case RESULT_DOWNLOAD_FAILED:
        network_detail(r->status, r->path, detail, dn);
        snprintf(text, n, T("%s : récupération impossible. %s La sauvegarde de la 3DS reste intacte.", "%s: download failed. %s The 3DS save is intact."), game,
                 network_text(r->status, r->wifi, english));
        return KIND_FAILURE;
    case RESULT_DOWNLOAD_DAMAGED:
        snprintf(text, n, T("%s : version reçue abîmée, rien n'est écrit. Réessayez.", "%s: received version damaged, nothing written. Try again."), game);
        return KIND_FAILURE;
    case RESULT_BACKUP_FAILED:
        snprintf(detail, dn, "%s", r->path);
        snprintf(text, n, T("%s : copie de sécurité impossible, rien n'est écrit. Vérifiez la carte SD.", "%s: backup copy failed, nothing written. Check the SD card."), game);
        return KIND_FAILURE;
    case RESULT_3DS_WRITE_REFUSED:
        snprintf(text, n, T("%s : écriture 3DS refusée, sauvegarde intacte.", "%s: 3DS write refused, save intact."), game);
        return KIND_FAILURE;
    case RESULT_OUT_OF_MEMORY:
        snprintf(text, n, T("%s : mémoire insuffisante, rien n'est écrit.", "%s: out of memory, nothing written."), game);
        return KIND_FAILURE;
    case RESULT_WRITE_FAILED:
        snprintf(detail, dn, "%s", r->path);
        snprintf(text, n, T("%s : écriture impossible, sauvegarde de la 3DS intacte.", "%s: write failed, 3DS save intact."), game);
        return KIND_FAILURE;
    case RESULT_WRITTEN_DAMAGED: /* renamed, but read back different: no Base on this file */
        snprintf(detail, dn, "%s", r->path);
        snprintf(text, n, T("%s : sauvegarde écrite abîmée. Vérifiez la carte SD, puis récupérez de nouveau.%s", "%s: written save damaged. Check the SD card, then download again.%s"),
                 game, old_copied);
        return KIND_FAILURE;
    case RESULT_RENAME_FAILED: { /* bottom screen: file name shortened, extension kept */
        const char *base = strrchr(r->path, '/') ? strrchr(r->path, '/') + 1 : r->path, *ext = strrchr(base, '.');
        int l = (int)strlen(base);
        if (l > 60)
            snprintf(text, n, T("%s : renommage impossible. Sauvegarde reçue : %.50s\xE2\x80\xA6%s.tmp.%s", "%s: rename failed. Downloaded save: %.50s\xE2\x80\xA6%s.tmp.%s"),
                     game, base, ext ? ext : "", old_copied);
        else
            snprintf(text, n, T("%s : renommage impossible. Sauvegarde reçue : %s.tmp.%s", "%s: rename failed. Downloaded save: %s.tmp.%s"), game, base, old_copied);
        return KIND_FAILURE;
    }
    case RESULT_SWITCHED:
        snprintf(text, n, T("%s : se joue maintenant avec %s.", "%s: now played with %s."), game,
                 r->program ? r->program : T("le forwarder mGBA", "the mGBA forwarder"));
        return KIND_SUCCESS;
    case RESULT_SWITCH_FAILED:
        snprintf(detail, dn, "%s", r->path);
        snprintf(text, n, T("%s : changement impossible, rien n'est perdu.", "%s: switch failed, nothing lost."), game);
        return KIND_FAILURE;
    case RESULT_GB_GBA_ONLY:
        snprintf(text, n, "%s", T("X : seulement pour les ROM GB et GBA de la carte SD.", "X: GB and GBA ROMs on the SD card only."));
        return KIND_INFO;
    case RESULT_VC_READ_ONLY:
        snprintf(text, n, T("%s : la récupération des jeux Virtual Console arrive dans une prochaine version. Votre sauvegarde n'a pas été modifiée.",
                            "%s: downloading Virtual Console games comes in a later version. Your save was not changed."), game);
        return KIND_INFO;
    case RESULT_VC_UNEXPECTED:
        snprintf(detail, dn, T("Fichiers : %s", "Files: %s"), r->path);
        snprintf(text, n, T("%s : fichiers inattendus dans la sauvegarde Virtual Console. Ce jeu n'est pas synchronisé, sa sauvegarde n'est pas modifiée. Signalez la ligne technique.",
                            "%s: unexpected files in the Virtual Console save. This game is not synced, and its save is not changed. Report the technical line."), game);
        return KIND_FAILURE;
    case RESULT_CONFLICT_LEFT:
        snprintf(text, n, T("%s : conflit laissé tel quel.", "%s: conflict left as is."), game);
        return KIND_INFO;
    case RESULT_DUPLICATE_SHARED:
        snprintf(text, n, T("%s : %d ROM ont ce nom de fichier et partageraient une sauvegarde. Renommez-en une.",
                            "%s: %d ROMs have this file name and would share one save. Rename one."), game, r->count);
        return KIND_FAILURE;
    case RESULT_DUPLICATE:
        snprintf(text, n, T("%s : %d ROM contiennent ce jeu. Gardez-en une, ou ajoutez les autres aux ROM hacks sur le PC.",
                            "%s: %d ROMs hold this game. Keep one, or add the others to ROM hacks on the PC."), game, r->count);
        return KIND_FAILURE;
    case RESULT_OFFLINE:
        snprintf(text, n, T("%s : serveur non lu. Appuyez sur \xEE\x80\x83 pour réessayer.", "%s: server not read. Press \xEE\x80\x83 to try again."), game);
        return KIND_FAILURE;
    case RESULT_ALREADY_IN_SYNC:
        snprintf(text, n, T("%s : déjà à jour.", "%s: already in sync."), game);
        return KIND_INFO;
    }
    return KIND_INFO;
}

int result_ok(const result_t *r)
{
    return r->code == RESULT_UPLOADED || r->code == RESULT_DOWNLOADED || r->code == RESULT_SWITCHED;
}

void sync_summary(char *out, size_t n, int uploads, int downloads, int failures, int skipped, int english)
{
    const int v[4] = { uploads, downloads, failures, skipped };
    const char *one[4] = { T("%d envoi", "%d upload"), T("%d récupération", "%d download"), T("%d échec", "%d failed"), T("%d ignoré", "%d skipped") };
    const char *many[4] = { T("%d envois", "%d uploads"), T("%d récupérations", "%d downloads"), T("%d échecs", "%d failed"), T("%d ignorés", "%d skipped") };
    size_t l = 0;
    int i;
    out[0] = '\0';
    for (i = 0; i < 4; i++)
        if (v[i] && l < n) {
            if (l) l += snprintf(out + l, n - l, ", ");
            if (l < n) l += snprintf(out + l, n - l, plural(v[i], english, one[i], many[i]), v[i]);
        }
    if (!l) snprintf(out, n, "%s", T("Rien à synchroniser.", "Nothing to sync."));
}
