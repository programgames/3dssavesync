#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <time.h>
#include "app_update.h"
#include "errors.h"
#include "files.h"
#include "net.h"
#include "update.h"
#include "views.h"

#define GITHUB_LATEST "https://api.github.com/repos/programgames/3dssavesync/releases/latest"

update_info_t update_info;
char update_detail[400];
int update_failed;
static const char *path_3dsx = ""; /* argv[0]: the .3dsx started from the Homebrew Launcher (launched_from_sd) */

/* maj.ini: day of the check, last version seen, running version. On failure: written again at the next start. */
static void write_update_ini(void)
{
    mkdir(APP_DIR, 0777);
    write_update_info(APP_DIR "/maj.ini", &update_info, VERSION);
}

/* Failure of a request to GitHub: what is not done, the text of github_text, then the curl or HTTP code. */
static void github_failure(const char *what, long status)
{
    update_failed = 1;
    snprintf(update_detail, sizeof update_detail, "%s%s%s\n%s %ld", what, what[0] ? " " : "", github_text(status, net_wifi(), english),
             status < 0 ? "curl" : "HTTP", status < 0 ? -status : status);
}

/* GET /releases/latest: r, update_info.latest, update_detail and maj.ini updated. 1 if a newer version exists. */
static int check_update(release_t *r)
{
    char *body;
    size_t size;
    long status;
    int ok;

    show_progress(0, 0, "%s", T("Recherche d'une mise à jour…", "Looking for an update…"));
    status = net_github(GITHUB_LATEST, NULL, &body, &size);
    ok = status == 200 && parse_release(body, r) == 0;
    if (status > 0) free(body);
    if (ok) snprintf(update_info.latest, sizeof update_info.latest, "%s", r->version);
    update_info.day = (long)(time(NULL) / 86400);
    write_update_ini(); /* failure: checked again at the next start */
    if (!ok) {
        github_failure("", status);
        return 0;
    }
    update_failed = 0;
    if (update_is_newer(VERSION, r->version)) {
        snprintf(update_detail, sizeof update_detail, T("Version %s disponible.", "Version %s available."), r->version);
        return 1;
    }
    /* Never the GitHub version when it is not newer: it says nothing useful and looks like a delay. */
    snprintf(update_detail, sizeof update_detail, "%s", !strcmp(VERSION, "dev") ? T("Version de développement : aucune mise à jour proposée.", "Development build: no update offered.")
                                                                           : T("3dssavesync est à jour.", "3dssavesync is up to date."));
    return 0;
}

static char download_version[32]; /* version being downloaded or installed */

static void download_progress(size_t done, size_t total)
{
    show_progress((int)(done / 1024), (int)(total / 1024), T("Téléchargement de la version %s (Ko)", "Downloading version %s (KB)"), download_version);
}

/* .cia through AM, like FBI: on any failure, the install is canceled and the old version stays. */
static Result install_cia(const char *bytes, size_t size)
{
    Handle h;
    Result r;
    u32 written = 0;
    size_t done;

    if (R_FAILED(r = amInit())) return r;
    if (R_SUCCEEDED(r = AM_StartCiaInstall(MEDIATYPE_SD, &h))) {
        for (done = 0; done < size && R_SUCCEEDED(r); done += written) {
            show_progress((int)(done / 1024), (int)(size / 1024), T("Installation de la version %s (Ko)", "Installing version %s (KB)"), download_version);
            r = FSFILE_Write(h, &written, done, bytes + done, size - done < 0x20000 ? size - done : 0x20000, 0);
            if (R_SUCCEEDED(r) && !written) r = MAKERESULT(RL_PERMANENT, RS_INTERNAL, RM_APPLICATION, RD_NO_DATA);
        }
        if (R_SUCCEEDED(r)) r = AM_FinishCiaInstall(h);
        else AM_CancelCIAInstall(h);
    }
    amExit();
    return r;
}

/* Downloads r, checks its SHA-256 in SHA256SUMS, then installs it: .cia through AM, or .3dsx replaced if it was started
 * from the Homebrew Launcher (3dslink, empty argv: refused, the running file is unknown). Failure: update_detail, and the
 * old version stays. 1 if installed. The Saves are never touched. */
static int install_update(const release_t *r)
{
    const int dsx = envIsHomebrew();
    const char *name = dsx ? "3dssavesync.3dsx" : "3dssavesync.cia", *url = dsx ? r->dsx : r->cia;
    char *checksums = NULL, *bytes = NULL, expected[65], actual[65];
    size_t size, n;
    long status;
    FS_ArchiveResource sd;
    Result res;

    update_failed = 1;
    snprintf(download_version, sizeof download_version, "%s", r->version);
    if (dsx && !launched_from_sd(path_3dsx)) {
        snprintf(update_detail, sizeof update_detail, T("Mise à jour impossible ici : lancez 3dssavesync depuis le Homebrew Launcher, ou installez le .cia.\nargv[0] : %.200s",
                                                  "Update not possible here: start 3dssavesync from the Homebrew Launcher, or install the .cia.\nargv[0]: %.200s"),
                 path_3dsx[0] ? path_3dsx : "-");
        return 0;
    }
    if (!url[0] || !r->checksums[0]) {
        snprintf(update_detail, sizeof update_detail, T("Version %s incomplète sur GitHub. Installez-la à la main (voir le guide 3DS).\nabsent : %s",
                                                  "Version %s is incomplete on GitHub. Install it by hand (see the 3DS guide).\nmissing: %s"),
                 r->version, url[0] ? "SHA256SUMS" : name);
        return 0;
    }
    status = net_github(r->checksums, download_progress, &checksums, &n);
    if (status != 200) {
        github_failure(T("Impossible de télécharger SHA256SUMS.", "Couldn't download SHA256SUMS."), status); /* "Try again": in github_text */
        goto end;
    }
    if (find_checksum(checksums, name, expected)) {
        snprintf(update_detail, sizeof update_detail, T("SHA256SUMS ne contient pas %s. Installez la version %s à la main (voir le guide 3DS).\nSHA256SUMS : %u octets",
                                                  "SHA256SUMS has no %s. Install version %s by hand (see the 3DS guide).\nSHA256SUMS: %u bytes"),
                 name, r->version, (unsigned)n);
        goto end;
    }
    status = net_github(url, download_progress, &bytes, &size);
    if (status != 200) {
        github_failure(T("Le téléchargement a échoué.", "Download failed."), status);
        goto end;
    }
    show_progress(0, 0, "%s", T("Vérification du fichier…", "Checking the file…"));
    hash_hex((unsigned char *)bytes, size, actual);
    if (strcmp(actual, expected)) {
        snprintf(update_detail, sizeof update_detail, T("Le fichier téléchargé est abîmé. Réessayez.\nSHA-256 %.12s…, attendu %.12s…",
                                                  "The downloaded file is damaged. Try again.\nSHA-256 %.12s…, expected %.12s…"),
                 actual, expected);
        goto end;
    }
    /* .cia: room for the installed title; .3dsx: the .tmp next to the old one (renamed .bak). Twice the size is enough in both cases. */
    if (R_SUCCEEDED(FSUSER_GetSdmcArchiveResource(&sd)) && (u64)sd.freeClusters * sd.clusterSize < 2 * (u64)size) {
        u64 free_bytes = (u64)sd.freeClusters * sd.clusterSize;
        snprintf(update_detail, sizeof update_detail, T("Pas assez de place sur la carte SD. Libérez %llu Mo et réessayez.\nlibre : %llu Ko, requis : %llu Ko",
                                                  "Not enough space on the SD card. Free %llu MB and try again.\nfree: %llu KB, needed: %llu KB"),
                 (2 * (u64)size - free_bytes) / 1048576 + 1, free_bytes / 1024, 2 * (u64)size / 1024);
        goto end;
    }
    if (dsx) {
        int e;
        show_progress(0, 0, T("Installation de la version %s…", "Installing version %s…"), download_version);
        if ((e = replace_program(path_3dsx, bytes, size))) {
            if (e == -1) snprintf(update_detail, sizeof update_detail, T("Impossible d'écrire sur la carte SD, l'ancienne version reste. Vérifiez la carte et réessayez.\n%s",
                                                                   "Can't write to the SD card, the old version stays. Check the card and try again.\n%s"), path_3dsx);
            else snprintf(update_detail, sizeof update_detail, T("Aucune version en place. Sur un PC, renommez %s.bak en %s pour retrouver l'ancienne.",
                                                           "No version in place. On a PC, rename %s.bak to %s to get the old one back."), path_3dsx, strrchr(path_3dsx, '/') + 1);
            goto end;
        }
    } else if (R_FAILED(res = install_cia(bytes, size))) {
        snprintf(update_detail, sizeof update_detail, T("La 3DS a refusé l'installation, l'ancienne version reste. Redémarrez la 3DS et réessayez.\nAM 0x%08lX",
                                                  "The 3DS refused the install, the old version stays. Restart the 3DS and try again.\nAM 0x%08lX"), res);
        goto end;
    }
    update_failed = 0;
    {
        const button_t b = { KEY_A, T("Fermer 3dssavesync", "Close 3dssavesync"), 1 };
        const char *restart = dsx ? T("3dssavesync va se fermer. Relancez-le depuis le Homebrew Launcher.", "3dssavesync will close. Start it again from the Homebrew Launcher.")
                                  : T("3dssavesync va se fermer. Relancez-le depuis le menu HOME.", "3dssavesync will close. Start it again from the HOME Menu.");
        char title[80];
        u32 k;
        snprintf(title, sizeof title, T("Version %s installée", "Version %s installed"), r->version);
        do {
            float y;
            modal_begin(NULL);
            y = 8 + screen_paragraph(8, 8, 0.55f, C_GREEN, TEXT_WIDTH, title) + 6;
            screen_paragraph(8, y, 0.5f, WHITE, TEXT_WIDTH, restart);
            k = modal_end(&b, 1, 0);
        } while (!(k & (KEY_A | KEY_B)));
    }
end:
    free(checksums);
    free(bytes);
    return !update_failed;
}

int update_app(void)
{
    release_t r;
    char t[300];
    int installed;
    if (check_update(&r) != 1) return 0;
    snprintf(t, sizeof t, T("La version %s remplacera la version " VERSION ". Vos sauvegardes et vos réglages ne changent pas.",
                            "Version %s will replace version " VERSION ". Your saves and settings stay the same."),
             r.version);
    if (!confirm(T("Mettre à jour 3dssavesync ?", "Update 3dssavesync?"), NULL, t, NULL, T("Mettre à jour", "Update"))) return 0;
    aptSetSleepAllowed(false); /* closing the lid would cut the download or stop the install midway */
    installed = install_update(&r);
    aptSetSleepAllowed(true);
    return installed;
}

void update_start(const char *argv0)
{
    read_update_info(APP_DIR "/maj.ini", &update_info);
    if (strcmp(update_info.installed, VERSION)) write_update_ini(); /* the PC does not copy an older .cia (sdcard.go) */
    if (argv0) path_3dsx = argv0;
}

void update_check_if_due(void)
{
    release_t r;
    if (strcmp(VERSION, "dev") && net_wifi() && update_check_due(update_info.day, time(NULL)))
        check_update(&r); /* failure: detail in the settings only, the sync works as before */
}
