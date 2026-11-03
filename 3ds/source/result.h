/* Result of an action on a Game (Upload, Download, switch of the Save file, A on a Game) and the one text that says it.
 * No libctru dependency: tested on the PC (test/test_result.c). */
#ifndef RESULT_H
#define RESULT_H

#include <stddef.h>

typedef enum {
    RESULT_NONE,           /* nothing done, nothing to say (question canceled) */
    RESULT_UPLOADED,
    RESULT_UPLOAD_UNREADABLE,
    RESULT_UPLOAD_CONFLICT, /* 412: the Current version changed meanwhile */
    RESULT_UPLOAD_FAILED,   /* status, url */
    RESULT_DOWNLOADED,
    RESULT_DOWNLOAD_FAILED,  /* status, url */
    RESULT_DOWNLOAD_DAMAGED, /* received Hash differs */
    RESULT_BACKUP_FAILED,    /* path: the Backup copy */
    RESULT_3DS_WRITE_REFUSED,
    RESULT_OUT_OF_MEMORY,
    RESULT_WRITE_FAILED,     /* path: the Save file, intact */
    RESULT_WRITTEN_DAMAGED,  /* path: the Save file; had_save */
    RESULT_RENAME_FAILED,    /* path: the Save file (the .tmp stays); had_save */
    RESULT_SWITCHED,         /* program */
    RESULT_SWITCH_FAILED,    /* path: the Save file */
    RESULT_GB_GBA_ONLY,
    RESULT_VC_READ_ONLY,     /* Download of a Virtual Console game: not yet, nothing written */
    RESULT_VC_UNEXPECTED,    /* path: the files of its archive (title_root) */
    RESULT_CONFLICT_LEFT,
    RESULT_DUPLICATE_SHARED, /* count */
    RESULT_DUPLICATE,        /* count */
    RESULT_OFFLINE,
    RESULT_ALREADY_IN_SYNC,
} result_code_t;

typedef struct {
    result_code_t code;
    long status;         /* network failure: HTTP status (> 0) or -<curl code> */
    int wifi;            /* network failure: 0 if the 3DS was connected to no access point */
    int count;           /* Duplicate: number of ROMs */
    int had_save;        /* Download: a Local save existed, so it is in copies/ */
    const char *program; /* switch: program that now plays the Game; NULL: the mGBA forwarder */
    char path[700];      /* url (network failure) or file of the failure */
} result_t;

typedef enum { KIND_INFO, KIND_SUCCESS, KIND_FAILURE } result_kind_t;

/* Text of r for the Game game ("Pokemon Diamant : envoyé au serveur."), and its technical line in detail ("" if none).
 * Returns the kind of the message (its color). */
result_kind_t result_text(const result_t *r, const char *game, int english, char *text, size_t n, char *detail, size_t dn);

/* 1 if r is a successful Upload, Download or switch. */
int result_ok(const result_t *r);

/* Sync all: "2 envois, 1 récupération, 1 ignoré", only what counts; "Rien à synchroniser." if nothing. */
void sync_summary(char *out, size_t n, int uploads, int downloads, int failures, int skipped, int english);

#endif
