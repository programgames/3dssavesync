/* Server list (GET /saves, docs/design.md §2). No libctru dependency: tested on the PC (test/test_list.c). */
#ifndef LIST_H
#define LIST_H

#include "key.h"

/* One game of the list, or one Version of GET /saves/<key>/history (same objects, oldest first): {"code": "IRBF",
 * "version": "...", "hash": "...", "taille": 524288, "date": "...", "client": "pc", "age": 120, "nom": "..."}.
 * Missing field: "" (text), -1 (size, age), 0 (has_name). hash: 64 lowercase hex digits, else "". version: the file
 * name "<seq>-<date>-<client>-<hash>.sav" ([0-9A-Za-z-] only, ends with this hash), else "": safe in a URL. */
typedef struct {
    char code[KEY_MAX], hash[65], date[17], client[17], version[112];
    long size, age;
    int has_name;
} entry_t;

/* Next game of the JSON list from *p: each field is read within its own object, spaces allowed around
 * ":" and ",". 1 if read (*p moves past the object), 0 at the end of the list or on malformed text. */
int next_server_entry(const char **p, entry_t *e);

#endif
