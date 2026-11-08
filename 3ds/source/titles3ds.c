/* Same order of operations as Checkpoint (io.cpp: write, commit, secure value), code written here (ADR 0009). */
#include <dirent.h>
#include <malloc.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <strings.h>
#include "archive.h"
#include "titles3ds.h"
#include "vc.h"

/* Binary path {media, low, high} of ARCHIVE_USER_SAVEDATA (3dbrew, FS:OpenArchive). */
static Result open_save_archive(u64 id, FS_MediaType media, FS_Archive *a)
{
    u32 path[3] = { media, (u32)id, (u32)(id >> 32) };
    return FSUSER_OpenArchive(a, ARCHIVE_USER_SAVEDATA, (FS_Path){ PATH_BINARY, sizeof path, path });
}

int read_title_save(u64 id, const char *file, FS_MediaType *media, unsigned char **bytes, size_t *size)
{
    static const FS_MediaType media_types[2] = { MEDIATYPE_SD, MEDIATYPE_GAME_CARD };
    int i;

    for (i = 0; i < 2; i++) {
        FS_Archive a;
        Handle f;
        u64 t = 0;
        u32 bytes_read = 0;
        int ok = 0;

        if (R_FAILED(open_save_archive(id, media_types[i], &a))) continue;
        if (R_SUCCEEDED(FSUSER_OpenFile(&f, a, fsMakePath(PATH_ASCII, file), FS_OPEN_READ, 0))) {
            if (R_SUCCEEDED(FSFILE_GetSize(f, &t)) && t > 0 && (*bytes = malloc(t))) {
                ok = R_SUCCEEDED(FSFILE_Read(f, &bytes_read, 0, *bytes, t)) && bytes_read == t;
                if (!ok) free(*bytes);
            }
            FSFILE_Close(f);
        }
        FSUSER_CloseArchive(a);
        if (ok) {
            *media = media_types[i];
            *size = t;
            return 0;
        }
    }
    return -1;
}

/* Otherwise the game refuses a restored save; it writes a new one at its next save. */
static void remove_secure_value(u64 id)
{
    u64 value = ((u64)SECUREVALUE_SLOT_SD << 32) | ((((u32)id >> 8) & 0xFFFFF) << 8);
    u8 existed;
    FSUSER_ControlSecureSave(SECURESAVE_ACTION_DELETE, &value, sizeof value, &existed, 1);
}

int write_title_save(u64 id, FS_MediaType media, const char *file, const void *bytes, size_t size)
{
    FS_Archive a;
    Handle f;
    u64 t = 0;
    u32 written = 0;
    Result r;

    if (R_FAILED(open_save_archive(id, media, &a))) return -1;
    r = FSUSER_OpenFile(&f, a, fsMakePath(PATH_ASCII, file), FS_OPEN_WRITE, 0);
    if (R_SUCCEEDED(r)) {
        r = FSFILE_GetSize(f, &t);
        if (R_SUCCEEDED(r) && t != size) r = -2; /* never a size change: the game does not expect one */
        if (R_SUCCEEDED(r)) r = FSFILE_Write(f, &written, 0, bytes, size, 0);
        if (R_SUCCEEDED(r) && written != size) r = -3;
        FSFILE_Close(f);
    }
    if (R_SUCCEEDED(r)) r = FSUSER_ControlArchive(a, ARCHIVE_ACTION_COMMIT_SAVE_DATA, NULL, 0, NULL, 0);
    FSUSER_CloseArchive(a);
    if (R_FAILED(r)) return -1;
    remove_secure_value(id);
    return 0;
}

/* ---------------------------------------------------------------- whole save archive (ADR 0028) */

/* Files of a save archive: path and size (data NULL), on the heap. */
typedef struct {
    archive_file_t *f;
    int n, cap;
} file_list_t;

static int add_file(file_list_t *l, const char *path, u64 size)
{
    if (l->n == l->cap) {
        archive_file_t *p = realloc(l->f, (l->cap = l->cap ? l->cap * 2 : 16) * sizeof *p);
        if (!p) return -1;
        l->f = p;
    }
    memset(&l->f[l->n], 0, sizeof *l->f);
    snprintf(l->f[l->n].path, sizeof l->f[0].path, "%s", path);
    l->f[l->n++].size = (size_t)size;
    return 0;
}

/* Name of a directory entry, printable ASCII only (ponytail: other names refused; UTF-16 <-> UTF-8 if a game needs it).
 * 0 if converted. */
static int ascii_name(const u16 *src, char *name, size_t n)
{
    size_t i;
    for (i = 0; i < n - 1 && src[i]; i++) {
        if (src[i] < 0x20 || src[i] > 0x7E || src[i] == '\\') return -1;
        name[i] = (char)src[i];
    }
    name[i] = '\0';
    return src[i] ? -1 : 0;
}

/* Files of dir ("/" or "/a/b/") and its folders in l, as "a/b/file"; *total counts their bytes. why on failure. */
static int list_dir(FS_Archive a, const char *dir, int depth, file_list_t *l, unsigned long long *total, char *why, size_t wn)
{
    FS_DirectoryEntry e;
    Handle d;
    u32 got = 0;
    int failed = 0;
    Result r;
    if (depth > 8) {
        snprintf(why, wn, "folders deeper than 8: %.60s", dir);
        return -1;
    }
    if (R_FAILED(r = FSUSER_OpenDirectory(&d, a, fsMakePath(PATH_ASCII, dir)))) {
        snprintf(why, wn, "OpenDirectory %.60s: %08lX", dir, (unsigned long)r);
        return -1;
    }
    while (!failed && R_SUCCEEDED(r = FSDIR_Read(d, &got, 1, &e)) && got == 1) {
        char name[ARCHIVE_PATH_MAX], path[ARCHIVE_PATH_MAX];
        const int folder = (e.attributes & FS_ATTRIBUTE_DIRECTORY) != 0;
        failed = -1;
        if (ascii_name(e.name, name, sizeof name)) snprintf(why, wn, "file name not supported in %.60s", dir);
        else if (snprintf(path, sizeof path, "%s%s%s", dir, name, folder ? "/" : "") >= (int)sizeof path) snprintf(why, wn, "path too long: %.60s", path);
        else if (folder) failed = list_dir(a, path, depth + 1, l, total, why, wn);
        else if ((*total += e.fileSize) > ARCHIVE_MAX) snprintf(why, wn, "save larger than %d MiB", ARCHIVE_MAX >> 20);
        else if (add_file(l, path + 1, e.fileSize)) snprintf(why, wn, "out of memory (%d files)", l->n);
        else failed = 0;
    }
    FSDIR_Close(d);
    if (!failed && R_FAILED(r)) {
        snprintf(why, wn, "FSDIR_Read %.60s: %08lX", dir, (unsigned long)r);
        failed = -1;
    }
    return failed;
}

/* The f->size bytes of file f into out. why on failure. */
static int read_whole(FS_Archive a, const archive_file_t *f, unsigned char *out, char *why, size_t wn)
{
    char path[ARCHIVE_PATH_MAX + 1];
    Handle h;
    u64 t = 0;
    u32 got = 0;
    Result r;
    snprintf(path, sizeof path, "/%s", f->path);
    if (R_SUCCEEDED(r = FSUSER_OpenFile(&h, a, fsMakePath(PATH_ASCII, path), FS_OPEN_READ, 0))) {
        r = FSFILE_GetSize(h, &t);
        if (R_SUCCEEDED(r) && t != f->size) r = -2; /* changed since the listing */
        if (R_SUCCEEDED(r) && t) r = FSFILE_Read(h, &got, 0, out, (u32)t);
        if (R_SUCCEEDED(r) && got != t) r = -3;
        FSFILE_Close(h);
    }
    if (R_SUCCEEDED(r)) return 0;
    snprintf(why, wn, "read %.60s: %08lX", f->path, (unsigned long)r);
    return -1;
}

/* Every file of the opened archive a, packed (archive.h): each file read straight into its place. */
static int read_open_archive(FS_Archive a, unsigned char **bytes, size_t *size, char *why, size_t wn)
{
    file_list_t l = { NULL, 0, 0 };
    unsigned long long total = 0;
    int bad = 0, i, r = list_dir(a, "/", 0, &l, &total, why, wn);
    if (!r && (r = pack_archive(l.f, l.n, bytes, size, &bad))) {
        if (r == -1) snprintf(why, wn, "path longer than 100 bytes: %.60s", l.f[bad].path);
        else snprintf(why, wn, "out of memory (%llu bytes)", total);
    }
    for (i = 0; !r && i < l.n; i++)
        if ((r = read_whole(a, &l.f[i], *bytes + l.f[i].offset, why, wn))) free(*bytes);
    free(l.f);
    return r ? -1 : 0;
}

int read_title_archive(u64 id, FS_MediaType *media, unsigned char **bytes, size_t *size, char *why, size_t wn)
{
    static const FS_MediaType media_types[2] = { MEDIATYPE_SD, MEDIATYPE_GAME_CARD };
    int i, r;
    why[0] = '\0';
    for (i = 0; i < 2; i++) {
        FS_Archive a;
        if (R_FAILED(open_save_archive(id, media_types[i], &a))) continue;
        r = read_open_archive(a, bytes, size, why, wn);
        FSUSER_CloseArchive(a);
        if (r) return -2;
        *media = media_types[i];
        return 0;
    }
    return -1;
}

/* Index of path in files[0..n), -1 if absent. */
static int find_file(const archive_file_t *files, int n, const char *path)
{
    int i;
    for (i = 0; i < n; i++)
        if (!strcmp(files[i].path, path)) return i;
    return -1;
}

/* Folders of path ("a/b/file": "/a", "/a/b"), created if missing (an error is seen when the file is created). */
static void make_folders(FS_Archive a, const char *path)
{
    char dir[ARCHIVE_PATH_MAX + 1];
    const char *s;
    for (s = strchr(path, '/'); s; s = strchr(s + 1, '/')) {
        snprintf(dir, sizeof dir, "/%.*s", (int)(s - path), path);
        FSUSER_CreateDirectory(a, fsMakePath(PATH_ASCII, dir), 0);
    }
}

/* Writes file f of the version: created at its size, or resized if it exists (old: 1), then its bytes. */
/* old_size: size of the file already in the archive, -1 if none. A save archive file keeps the size it was created with,
 * so a file of another size is deleted and created again at the new size, as Checkpoint restores (no SetSize). */
static Result write_one(FS_Archive a, const archive_file_t *f, long long old_size)
{
    char path[ARCHIVE_PATH_MAX + 1];
    Handle h;
    u32 written = 0;
    Result r = 0;
    snprintf(path, sizeof path, "/%s", f->path);
    if (old_size >= 0 && old_size != (long long)f->size) {
        r = FSUSER_DeleteFile(a, fsMakePath(PATH_ASCII, path));
        old_size = -1;
    }
    if (R_SUCCEEDED(r) && old_size < 0) {
        make_folders(a, f->path);
        r = FSUSER_CreateFile(a, fsMakePath(PATH_ASCII, path), 0, f->size);
    }
    if (R_SUCCEEDED(r)) r = FSUSER_OpenFile(&h, a, fsMakePath(PATH_ASCII, path), FS_OPEN_WRITE, 0);
    if (R_FAILED(r)) return r;
    if (f->size) r = FSFILE_Write(h, &written, 0, f->data, (u32)f->size, 0);
    if (R_SUCCEEDED(r) && written != f->size) r = -3;
    FSFILE_Close(h);
    return r;
}

/* The version files in the opened archive a, then the commit: files absent from it deleted, the others created or
 * resized and written. Nothing is committed on a failure: the archive keeps its old content. */
static Result write_open_archive(FS_Archive a, const archive_file_t *files, int n, char *why, size_t wn)
{
    file_list_t old = { NULL, 0, 0 };
    unsigned long long total = 0;
    char path[ARCHIVE_PATH_MAX + 1];
    Result r = 0;
    int i;
    if (list_dir(a, "/", 0, &old, &total, why, wn)) return -4;
    for (i = 0; R_SUCCEEDED(r) && i < old.n; i++)
        if (find_file(files, n, old.f[i].path) < 0) {
            snprintf(path, sizeof path, "/%s", old.f[i].path);
            if (R_FAILED(r = FSUSER_DeleteFile(a, fsMakePath(PATH_ASCII, path)))) snprintf(why, wn, "delete %.60s: %08lX", old.f[i].path, (unsigned long)r);
        }
    for (i = 0; R_SUCCEEDED(r) && i < n; i++) {
        int k = find_file(old.f, old.n, files[i].path);
        if (R_FAILED(r = write_one(a, &files[i], k >= 0 ? (long long)old.f[k].size : -1)))
            snprintf(why, wn, "write %.60s: %08lX", files[i].path, (unsigned long)r);
    }
    free(old.f);
    if (R_SUCCEEDED(r) && R_FAILED(r = FSUSER_ControlArchive(a, ARCHIVE_ACTION_COMMIT_SAVE_DATA, NULL, 0, NULL, 0)))
        snprintf(why, wn, "commit: %08lX", (unsigned long)r);
    return r;
}

int write_title_archive(u64 id, FS_MediaType media, unsigned char *tar, size_t size, char *why, size_t wn)
{
    archive_file_t *files = NULL;
    unsigned char *back = NULL;
    unsigned long long total = 0;
    size_t back_size = 0;
    FS_Archive a;
    Result r;
    int n = 0, i, same;
    why[0] = '\0';
    if (unpack_archive(tar, size, &files, &n)) {
        snprintf(why, wn, "server version is not a 3dssavesync archive");
        return -1;
    }
    for (i = 0; i < n; i++) {
        const char *p;
        for (p = files[i].path; *p > 0x1F && *p < 0x7F; p++);
        if (*p) snprintf(why, wn, "file name not supported: %.60s", files[i].path); /* as list_dir: printable ASCII only */
        total += files[i].size;
    }
    if (!why[0] && total > ARCHIVE_MAX) snprintf(why, wn, "save larger than %d MiB", ARCHIVE_MAX >> 20);
    if (why[0] || R_FAILED(r = open_save_archive(id, media, &a))) { /* never created: a game never started is skipped */
        if (!why[0]) snprintf(why, wn, "OpenArchive: %08lX", (unsigned long)r);
        free(files);
        return -1;
    }
    r = write_open_archive(a, files, n, why, wn);
    FSUSER_CloseArchive(a);
    free(files);
    if (R_FAILED(r)) return -1;
    remove_secure_value(id);

    /* read back: the commit is done, so a difference means the archive holds something else (the copy is kept) */
    if (R_FAILED(open_save_archive(id, media, &a))) {
        snprintf(why, wn, "read back: OpenArchive");
        return -2;
    }
    i = read_open_archive(a, &back, &back_size, why, wn);
    FSUSER_CloseArchive(a);
    same = !i && back_size == size && !memcmp(back, tar, size);
    if (!i) free(back);
    if (!same && !why[0]) snprintf(why, wn, "read back differs (%lu bytes, expected %lu)", (unsigned long)back_size, (unsigned long)size);
    return same ? 0 : -2;
}

/* UTF-16 (Basic Multilingual Plane) -> UTF-8, without cutting a character; line break -> space. */
static void utf8(const u16 *src, size_t max, char *name, size_t n)
{
    size_t i, j = 0;
    for (i = 0; i < max && src[i]; i++) {
        u16 c = src[i] == '\n' ? ' ' : src[i];
        size_t l = c < 0x80 ? 1 : c < 0x800 ? 2 : 3;
        if (j + l + 1 > n) break;
        if (l == 1) name[j++] = (char)c;
        else if (l == 2) name[j++] = (char)(0xC0 | c >> 6), name[j++] = (char)(0x80 | (c & 0x3F));
        else name[j++] = (char)(0xE0 | c >> 12), name[j++] = (char)(0x80 | (c >> 6 & 0x3F)), name[j++] = (char)(0x80 | (c & 0x3F));
    }
    name[j] = '\0';
}

int title_root(u64 id, char *names, size_t n)
{
    static const FS_MediaType media_types[2] = { MEDIATYPE_SD, MEDIATYPE_GAME_CARD };
    int i;
    for (i = 0; i < 2; i++) {
        FS_Archive a;
        Handle d;
        FS_DirectoryEntry e;
        u32 count_read = 0;
        int count = 0;
        if (R_FAILED(open_save_archive(id, media_types[i], &a))) continue;
        if (R_FAILED(FSUSER_OpenDirectory(&d, a, fsMakePath(PATH_ASCII, "/")))) {
            FSUSER_CloseArchive(a);
            continue;
        }
        names[0] = '\0';
        while (R_SUCCEEDED(FSDIR_Read(d, &count_read, 1, &e)) && count_read == 1) {
            char name[128];
            size_t l = strlen(names);
            utf8(e.name, sizeof e.name / sizeof *e.name, name, sizeof name);
            /* cut when full: a cut list is never one of the short lists that callers accept */
            snprintf(names + l, n - l, "%s%s%s", count++ ? ", " : "", name, e.attributes & FS_ATTRIBUTE_DIRECTORY ? "/" : "");
        }
        FSDIR_Close(d);
        FSUSER_CloseArchive(a);
        return count;
    }
    return -1;
}

/* fs:USER never gives the RomFS of an SD application: 0xD9004676 on every path (v3.10 rc2 on a real 3DS; libctru issue
 * #516; the gate is in fs itself, Luma already grants every FS access bit). FSPXI, the fs <-> Process9 interface, has no
 * such gate, and Process9 decrypts the content. Luma3DS lets a process use it: svcControlService (0xB0, Luma's csvc.h)
 * with SERVICEOP_STEAL_CLIENT_SESSION (0) gives a session to PxiFS0, as Checkpoint does for raw saves (archive.cpp). */
static Result __attribute__((naked)) svc_control_service(u32 op, Handle *out, const char *name)
{
    (void)op; (void)out; (void)name;
    __asm__ volatile("svc 0xB0\n\tbx lr");
}

static int luma(void)
{
    s64 version;
    return R_SUCCEEDED(svcGetSystemInfo(&version, 0x10000, 0)); /* Luma3DS's own SystemInfo type; error on stock */
}

/* Result and byte count of the last read, for the diagnostic line. */
static Result pxi_result;
static u32 pxi_got;
static unsigned long long pxi_size;

/* FSPXI reads go to Process9 through a PXI buffer: the bytes are read in whole 0x1000-byte blocks into a page-aligned
 * buffer, then copied (rc4: a 0x60-byte read at 0 into the stack failed). */
static Result pxi_read(Handle h, FSPXI_File f, u64 offset, void *buffer, u32 size)
{
    u64 start = offset & ~0xFFFULL;
    u32 length = (u32)((offset + size - start + 0xFFF) & ~0xFFFULL);
    unsigned char *bounce = memalign(0x1000, length);
    pxi_got = 0;
    pxi_result = bounce ? FSPXI_ReadFile(h, f, &pxi_got, start, bounce, length) : (Result)-2;
    if (R_SUCCEEDED(pxi_result) && pxi_got >= offset + size - start) memcpy(buffer, bounce + (offset - start), size);
    else if (R_SUCCEEDED(pxi_result)) pxi_result = -1;
    free(bounce);
    return pxi_result;
}

/* "step(result,bytes read,file size)" of the last read, for the diagnostic line. */
static const char *read_failed(const char *step)
{
    static char text[64];
    snprintf(text, sizeof text, "%s(%08lX,%lu,%llu)", step, (unsigned long)pxi_result, (unsigned long)pxi_got, pxi_size);
    return text;
}

static u32 le32(const unsigned char *p)
{
    return p[0] | p[1] << 8 | p[2] << 16 | (u32)p[3] << 24;
}

/* First n bytes of the first file of rom/ in a RomFS read by FSPXI. FSPXI gives the whole RomFS section from its IVFC
 * header; level 3 follows the master hash, aligned on its block size (3dbrew "RomFS"). Level 3: header of 10 u32
 * (directory and file metadata tables, file data); a directory entry = parent, sibling, first child directory, first
 * file, hash sibling, name length, UTF-16 name at 0x18; a file entry = parent, sibling, u64 data offset at 0x08, u64 size
 * at 0x10, hash sibling, name length. 0xFFFFFFFF: none. Returns a step name on failure, NULL if read. */
static const char *rom_from_romfs(Handle h, FSPXI_File f, unsigned char *header, size_t n)
{
    unsigned char b[0x60];
    u64 level3 = 0;
    u32 dir_meta, file_meta, file_data, d;
    u64 size = 0;
    FSPXI_GetFileSize(h, f, &size);
    pxi_size = size;
    if (R_FAILED(pxi_read(h, f, 0, b, 0x60))) return read_failed("ivfc");
    if (!memcmp(b, "IVFC", 4)) {
        u64 block = 1ULL << (le32(b + 0x4C) & 31);
        level3 = (0x60 + le32(b + 0x08) + block - 1) / block * block;
    }
    if (R_FAILED(pxi_read(h, f, level3, b, 0x28))) return read_failed("level3");
    dir_meta = le32(b + 0x0C), file_meta = le32(b + 0x1C), file_data = le32(b + 0x24);
    if (R_FAILED(pxi_read(h, f, level3 + dir_meta, b, 0x18))) return read_failed("root");
    for (d = le32(b + 0x08); d != 0xFFFFFFFF; d = le32(b + 0x04)) { /* root's child directories */
        if (R_FAILED(pxi_read(h, f, level3 + dir_meta + d, b, 0x20))) return read_failed("dir");
        if (le32(b + 0x14) == 6 && !memcmp(b + 0x18, "r\0o\0m\0", 6)) break;
    }
    if (d == 0xFFFFFFFF) return "no rom/";
    if ((d = le32(b + 0x0C)) == 0xFFFFFFFF) return "no ROM";
    if (R_FAILED(pxi_read(h, f, level3 + file_meta + d, b, 0x20))) return read_failed("file");
    if (!le32(b + 0x14) && le32(b + 0x10) < n) return "small ROM";
    if (R_FAILED(pxi_read(h, f, level3 + file_data + (le32(b + 0x08) | (u64)le32(b + 0x0C) << 32), header, n))) return read_failed("data");
    return NULL;
}

/* NCCH seed of a title (16 bytes): fs:USER GetSeed, 0x087B0080 (3dbrew "Filesystem services"; no libctru wrapper), FS
 * access bit 0x200000, which Luma3DS gives every process. 0xC8804464: no seed in SEEDDB. */
static Result title_seed(u64 id, u32 *seed)
{
    u32 *cmd = getThreadCommandBuffer();
    Result r;
    cmd[0] = 0x087B0080;
    cmd[1] = (u32)id;
    cmd[2] = (u32)(id >> 32);
    if (R_FAILED(r = svcSendSyncRequest(*fsGetSessionHandle()))) return r;
    if (R_FAILED(r = (Result)cmd[1])) return r;
    memcpy(seed, cmd + 2, 16);
    return 0;
}

int title_rom_header(u64 id, unsigned char *header, size_t n, char *why, size_t wn)
{
    /* Archive path {low, high, media, 0}. File path = 3dbrew Title Access path, 0x24 bytes as fs itself sends it
     * (decompiled fs, handler 0x10e798): title access 0, content 0, NCCH section 0 = RomFS, u64 mask of allowed content
     * types (all six; a zero mask allows nothing), then the title's NCCH seed (zero when it has none). Without the seed
     * of a seeded title (the Pokémon VC titles, 3dstool ext_key.txt) the first read fails: 0xD9004587, NCCH hash check.
     * Checked on a real 3DS (v3.10 rc6): Pokémon Cristal FR, GB-PM_CRYSTAL-F2E2, the key of the cartridge ROM. */
    u32 archive[4] = { (u32)id, (u32)(id >> 32), MEDIATYPE_SD, 0 }, file[9] = { 0, 0, 0, 0x3F };
    const char *step = NULL;
    FSPXI_Archive a;
    FSPXI_File f;
    Handle h;
    Result r;
    why[0] = '\0';
    if (!luma()) {
        snprintf(why, wn, "no Luma3DS");
        return -1;
    }
    if (R_FAILED(r = title_seed(id, file + 5)) && r != (Result)0xC8804464) { /* 0xC8804464: no seed, title not seeded */
        snprintf(why, wn, "GetSeed:%08lX", (unsigned long)r);
        return -1;
    }
    if (R_FAILED(r = svc_control_service(0, &h, "PxiFS0"))) {
        snprintf(why, wn, "PxiFS0:%08lX", (unsigned long)r);
        return -1;
    }
    if (R_FAILED(r = FSPXI_OpenArchive(h, &a, ARCHIVE_SAVEDATA_AND_CONTENT, (FS_Path){ PATH_BINARY, sizeof archive, archive })))
        snprintf(why, wn, "OpenArchive:%08lX", (unsigned long)r);
    else {
        if (R_FAILED(r = FSPXI_OpenFile(h, &f, a, (FS_Path){ PATH_BINARY, sizeof file, file }, FS_OPEN_READ, 0)))
            snprintf(why, wn, "OpenFile:%08lX", (unsigned long)r);
        else {
            if ((step = rom_from_romfs(h, f, header, n))) snprintf(why, wn, "%s", step);
            FSPXI_CloseFile(h, f);
        }
        FSPXI_CloseArchive(h, a);
    }
    svcCloseHandle(h);
    return R_FAILED(r) || step ? -1 : 0;
}

/* Hash table size for count entries, as save-data-copy-tool (ihaveamac, main.c get_buckets) gives FormatSaveData. */
static u32 buckets(u32 count)
{
    u32 i;
    if (count < 20) return count < 4 ? 3 : count | 1;
    for (i = 0; i < 100; i++) {
        u32 n = count + i;
        if (n & 1 && n % 3 && n % 5 && n % 7 && n % 11 && n % 13 && n % 17) return n;
    }
    return count | 1;
}

Result create_vc_save(u64 id, const void *bytes, size_t size)
{
    u32 path[3] = { MEDIATYPE_SD, (u32)id, (u32)(id >> 32) }, written = 0;
    const FS_Path p = { PATH_BINARY, sizeof path, path };
    FS_Archive a;
    Handle f;
    Result r;
    if (R_SUCCEEDED(FSUSER_OpenArchive(&a, ARCHIVE_USER_SAVEDATA, p))) { /* never format an archive that exists: it would erase its save */
        FSUSER_CloseArchive(a);
        return (Result)0xFFFFFFFE;
    }
    /* The format of the VC Pokémon archives read on a real 3DS (FSUSER_GetFormatInfo, Crystal and Red): 1 directory, 4 files,
     * duplicate data; 0x200 blocks and the bucket counts as save-data-copy-tool, which formats the save of a game never
     * started. */
    if (R_FAILED(r = FSUSER_FormatSaveData(ARCHIVE_USER_SAVEDATA, p, 0x200, 1, 4, buckets(1), buckets(4), true))) return r;
    if (R_FAILED(r = FSUSER_OpenArchive(&a, ARCHIVE_USER_SAVEDATA, p))) return r;
    r = FSUSER_CreateFile(a, fsMakePath(PATH_ASCII, "/sav.dat"), 0, size);
    if (R_SUCCEEDED(r)) r = FSUSER_OpenFile(&f, a, fsMakePath(PATH_ASCII, "/sav.dat"), FS_OPEN_WRITE, 0);
    if (R_SUCCEEDED(r)) {
        r = FSFILE_Write(f, &written, 0, bytes, size, 0);
        if (R_SUCCEEDED(r) && written != size) r = (Result)0xFFFFFFFD;
        FSFILE_Close(f);
    }
    if (R_SUCCEEDED(r)) r = FSUSER_ControlArchive(a, ARCHIVE_ACTION_COMMIT_SAVE_DATA, NULL, 0, NULL, 0);
    FSUSER_CloseArchive(a);
    remove_secure_value(id);
    return r;
}

/* Product code of the title (AM, "CTR-N-QBXA"): GB/GBC Virtual Console or not (vc_product). The last answer is kept:
 * the choices screen asks for the same title at every frame. */
static Result title_product_code(u64 id, char *code)
{
    Result r;
    code[0] = '\0';
    if (R_FAILED(r = amInit())) return r;
    if (R_FAILED(r = AM_GetTitleProductCode(MEDIATYPE_SD, id, code))) code[0] = '\0';
    amExit();
    code[15] = '\0';
    return r;
}

int title_vc(u64 id)
{
    static u64 last;
    static int last_vc;
    char code[16];
    if (id == last) return last_vc;
    title_product_code(id, code);
    last = id;
    return last_vc = vc_product(code);
}

/* SMDH of the title (0x36C0 bytes): ExeFS:/icon of ARCHIVE_SAVEDATA_AND_CONTENT (FBI's method). 0 if read. */
static int read_smdh(u64 id, FS_MediaType media, u16 *smdh)
{
    static const u32 file[5] = { 0, 0, 2, 0x6E6F6369, 0 }; /* "icon" */
    u32 archive[4] = { (u32)id, (u32)(id >> 32), media, 0 };
    Handle f;
    u32 bytes_read = 0;
    int ok = 0;
    if (R_SUCCEEDED(FSUSER_OpenFileDirectly(&f, ARCHIVE_SAVEDATA_AND_CONTENT, (FS_Path){ PATH_BINARY, sizeof archive, archive },
                                            (FS_Path){ PATH_BINARY, sizeof file, file }, FS_OPEN_READ, 0))) {
        ok = R_SUCCEEDED(FSFILE_Read(f, &bytes_read, 0, smdh, 0x36C0)) && bytes_read == 0x36C0 && !memcmp(smdh, "SMDH", 4);
        FSFILE_Close(f);
    }
    return ok ? 0 : -1;
}

int title_name(u64 id, FS_MediaType media, int language, char *name, size_t n)
{
    /* SMDH: 16 titles of 0x200 bytes from 0x08, short title (0x40 UTF-16 characters) at the start of each. */
    u16 *smdh = malloc(0x36C0);
    int ok = smdh && read_smdh(id, media, smdh) == 0;
    if (ok) {
        const u16 *t = smdh + (8 + 0x200 * (language & 0xF)) / 2;
        if (!t[0]) t = smdh + (8 + 0x200 * 1) / 2; /* empty title in this language: English */
        utf8(t, 0x40, name, n);
    }
    free(smdh);
    return ok ? 0 : -1;
}

int title_icon(u64 id, FS_MediaType media, u16 *pixels)
{
    /* Large SMDH icon at 0x24C0: 48×48 RGB565 in 8×8 tiles, Morton order within each tile. */
    u16 *smdh = malloc(0x36C0);
    int ok = smdh && read_smdh(id, media, smdh) == 0, i;
    for (i = 0; ok && i < 48 * 48; i++) {
        int tile = i >> 6, k = i & 63;
        int x = (tile % 6) * 8 + ((k & 1) | (k >> 1 & 2) | (k >> 2 & 4)), y = (tile / 6) * 8 + ((k >> 1 & 1) | (k >> 2 & 2) | (k >> 3 & 4));
        pixels[y * 48 + x] = smdh[0x24C0 / 2 + i];
    }
    free(smdh);
    return ok ? 0 : -1;
}

int installed_titles(u64 **ids, FS_MediaType **medias, int extra)
{
    static const FS_MediaType media_types[2] = { MEDIATYPE_SD, MEDIATYPE_GAME_CARD };
    u32 totals[2] = { 0, 0 };
    int i, n = 0;
    for (i = 0; i < 2; i++)
        if (R_FAILED(AM_GetTitleCount(media_types[i], &totals[i]))) totals[i] = 0;
    *ids = malloc((totals[0] + totals[1] + extra + 1) * sizeof **ids);
    *medias = malloc((totals[0] + totals[1] + extra + 1) * sizeof **medias);
    for (i = 0; i < 2 && *ids && *medias; i++) {
        u32 count_read = 0, k;
        u64 *all;
        if (!totals[i]) continue;
        if (!(all = malloc(totals[i] * sizeof *all))) break;
        if (R_SUCCEEDED(AM_GetTitleList(&count_read, media_types[i], totals[i], all)))
            for (k = 0; k < count_read; k++)
                if ((all[k] >> 32) == 0x00040000) { /* applications only (no demos, updates or DLC) */
                    (*ids)[n] = all[k];
                    (*medias)[n++] = media_types[i];
                }
        free(all);
    }
    if (i == 2) return n;
    free(*ids);
    free(*medias);
    *ids = NULL;
    *medias = NULL;
    return -1;
}

unsigned long long title_install_time(u64 id)
{
    u16 root[0x40] = { 0 };
    char dir[160], path[300];
    unsigned long long newest = 0;
    struct dirent *e;
    DIR *d;
    int i, n;
    if (R_FAILED(FSUSER_GetSdmcCtrRootPath((u8 *)root, sizeof root - 2))) return 0;
    n = snprintf(dir, sizeof dir, "sdmc:");
    for (i = 0; root[i] && n < (int)sizeof dir - 1; i++) dir[n++] = root[i] < 0x80 ? (char)root[i] : '?'; /* "/Nintendo 3DS/<id0>/<id1>" */
    dir[n] = '\0';
    snprintf(dir + n, sizeof dir - n, "/title/%08lx/%08lx/content", (unsigned long)(id >> 32), (unsigned long)(u32)id);
    if (!(d = opendir(dir))) return 0;
    while ((e = readdir(d))) {
        size_t l = strlen(e->d_name);
        u64 t = 0;
        if (l < 4 || strcasecmp(e->d_name + l - 4, ".tmd")) continue;
        snprintf(path, sizeof path, "%s/%s", dir, e->d_name);
        if (R_SUCCEEDED(archive_getmtime(path, &t)) && t > newest) newest = t;
    }
    closedir(d);
    return newest;
}
