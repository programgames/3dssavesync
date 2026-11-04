/* Same order of operations as Checkpoint (io.cpp: write, commit, secure value), code written here (ADR 0009). */
#include <malloc.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
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

int write_title_save(u64 id, FS_MediaType media, const void *bytes, size_t size)
{
    FS_Archive a;
    Handle f;
    u64 t = 0, value;
    u32 written = 0;
    u8 existed;
    Result r;

    if (R_FAILED(open_save_archive(id, media, &a))) return -1;
    r = FSUSER_OpenFile(&f, a, fsMakePath(PATH_ASCII, "/main"), FS_OPEN_WRITE, 0);
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

    /* Otherwise the game refuses a restored save; it writes a new one at its next save. */
    value = ((u64)SECUREVALUE_SLOT_SD << 32) | ((((u32)id >> 8) & 0xFFFFF) << 8);
    FSUSER_ControlSecureSave(SECURESAVE_ACTION_DELETE, &value, sizeof value, &existed, 1);
    return 0;
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

int title_main_only(u64 id)
{
    char names[64];
    return title_root(id, names, sizeof names) < 0 ? -1 : !strcmp(names, "main");
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

/* Product code of the title (AM, "CTR-N-QBXA"): GB/GBC Virtual Console or not (vc_product). The last answer is kept:
 * the choices screen asks for the same title at every frame. */
Result title_product_code(u64 id, char *code)
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
