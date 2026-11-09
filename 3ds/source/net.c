#include <3ds.h>
#include <curl/curl.h>
#include <malloc.h>
#include <mbedtls/sha256.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include "files.h"
#include "github_roots.h"
#include "isrg_root_x1.h"
#include "net.h"

void hash_hex(const unsigned char *bytes, size_t size, char out[65])
{
    unsigned char h[32];
    int i;
    mbedtls_sha256_ret(bytes, size, h, 0);
    for (i = 0; i < 32; i++) sprintf(out + 2 * i, "%02x", h[i]);
}

#define SOC_SIZE 0x100000

static u32 *soc_buffer;
static CURL *c; /* a single handle: curl keeps the TLS connection open between requests */

int net_init(void)
{
    soc_buffer = memalign(0x1000, SOC_SIZE);
    if (!soc_buffer || R_FAILED(socInit(soc_buffer, SOC_SIZE))) return -1;
    if (R_FAILED(sslcInit(0))) return -2;
    acInit(); /* Wi-Fi state (net_wifi); without it, Wi-Fi is assumed present */
    if (curl_global_init(CURL_GLOBAL_DEFAULT) != CURLE_OK) return -3;
    return (c = curl_easy_init()) ? 0 : -4;
}

void net_exit(void)
{
    curl_easy_cleanup(c);
    SOCU_CloseSockets(); /* what curl left open: closed now, not after the app (see no_linger) */
    curl_global_cleanup();
    acExit();
    sslcExit();
    socExit();
    free(soc_buffer);
}

typedef struct {
    char *bytes;
    size_t size, cap;
} buffer_t;

/* The whole body is allocated once, at its Content-Length: growing it chunk by chunk may copy it (old and new block at
 * once) and split the heap, which an Old 3DS can't afford for a 16 MiB archive. Without one, as the chunks come. */
static size_t receive(char *chunk, size_t t, size_t n, void *u)
{
    buffer_t *b = u;
    if (b->size + t * n + 1 > b->cap) {
        size_t cap = b->size + t * n + 1;
        curl_off_t length = -1;
        char *new_data;
        if (!b->size && curl_easy_getinfo(c, CURLINFO_CONTENT_LENGTH_DOWNLOAD_T, &length) == CURLE_OK && length >= (curl_off_t)cap &&
            (unsigned long long)length < (size_t)-1)
            cap = (size_t)length + 1;
        if (!(new_data = realloc(b->bytes, cap))) return 0; /* curl stops: CURLE_WRITE_ERROR, *body freed by net_get */
        b->bytes = new_data;
        b->cap = cap;
    }
    memcpy(b->bytes + b->size, chunk, t * n);
    b->size += t * n;
    b->bytes[b->size] = '\0';
    return t * n;
}

static void (*on_progress)(size_t, size_t); /* net_github: download of a release file in progress */
static void (*on_slow)(void);
static u64 slow_start; /* osGetTime() at the start of a Server request; 0: GitHub, or on_slow already called */

void net_on_slow(void (*f)(void)) { on_slow = f; }

static int xferinfo(void *u, curl_off_t total, curl_off_t done, curl_off_t ul_total, curl_off_t ul_now)
{
    if (on_progress) on_progress((size_t)done, (size_t)total);
    if (slow_start && osGetTime() - slow_start > 5000) {
        slow_start = 0;
        if (on_slow) on_slow();
    }
    return 0;
}

/* Linger 0: a closed connection is reset at once, not kept waiting (TIME_WAIT). Seen on a New 3DS XL: after the app
 * used the network, the next DS game (TWiLight Menu++ forwarder) started 10 s later, unless the app ended 60 s before. */
static int no_linger(void *u, curl_socket_t fd, curlsocktype purpose)
{
    struct linger l = { 1, 0 };
    setsockopt(fd, SOL_SOCKET, SO_LINGER, &l, sizeof l);
    return CURL_SOCKOPT_OK;
}

static char *server_cas; /* ISRG + sdmc:/config/ssl/cacert.pem (ADR 0029); NULL: ISRG only */
static size_t server_cas_size;

int net_read_cas(const char *path)
{
    int r = read_cas(path, ISRG_ROOT_X1, NET_MAX_CAS, &server_cas, &server_cas_size);
    if (r) server_cas = NULL;
    return r;
}

/* One request. upload != NULL => PUT of this body. token == NULL => GitHub (net_github). client: X-Client of the Server. */
static long request(const char *url, const char *token, const char *client, const char *condition,
                    const void *upload, size_t upload_size, buffer_t *received)
{
    char auth[320], x_client[40];
    struct curl_slist *headers = NULL;
    static char github_cas[sizeof ISRG_ROOT_X1 + sizeof GITHUB_ROOTS]; /* built on the first call */
    struct curl_blob ca = { (void *)ISRG_ROOT_X1, sizeof ISRG_ROOT_X1 - 1, CURL_BLOB_NOCOPY };
    CURLcode r;
    long status;

    curl_easy_reset(c); /* resets the options, keeps the connection open */
    if (token) { /* Server: ISRG (ADR 0003), and the user's certificates if any (ADR 0029) */
        if (server_cas) {
            ca.data = server_cas;
            ca.len = server_cas_size;
        }
        snprintf(auth, sizeof auth, "Authorization: Bearer %s", token);
        headers = curl_slist_append(NULL, auth);
        snprintf(x_client, sizeof x_client, "X-Client: %s", client);
        headers = curl_slist_append(headers, x_client);
    } else { /* GitHub: no token, ISRG + Sectigo (ADR 0021) */
        if (!github_cas[0]) snprintf(github_cas, sizeof github_cas, "%s%s", ISRG_ROOT_X1, GITHUB_ROOTS);
        ca.data = github_cas;
        ca.len = strlen(github_cas);
    }
    if (upload) {
        if (condition) headers = curl_slist_append(headers, condition);
        headers = curl_slist_append(headers, "Content-Type: application/octet-stream");
        headers = curl_slist_append(headers, "Expect:");
        curl_easy_setopt(c, CURLOPT_CUSTOMREQUEST, "PUT");
        curl_easy_setopt(c, CURLOPT_POSTFIELDSIZE_LARGE, (curl_off_t)upload_size);
        curl_easy_setopt(c, CURLOPT_POSTFIELDS, upload);
    }

    curl_easy_setopt(c, CURLOPT_URL, url);
    curl_easy_setopt(c, CURLOPT_HTTPHEADER, headers);
    curl_easy_setopt(c, CURLOPT_CAINFO_BLOB, &ca);
    curl_easy_setopt(c, CURLOPT_WRITEFUNCTION, receive);
    curl_easy_setopt(c, CURLOPT_WRITEDATA, received);
    curl_easy_setopt(c, CURLOPT_CONNECTTIMEOUT, 10L);
    curl_easy_setopt(c, CURLOPT_TIMEOUT, on_progress ? 300L : 60L);
    curl_easy_setopt(c, CURLOPT_USERAGENT, "3dssavesync-3ds"); /* required by the GitHub API */
    curl_easy_setopt(c, CURLOPT_SOCKOPTFUNCTION, no_linger);
    curl_easy_setopt(c, CURLOPT_XFERINFOFUNCTION, xferinfo); /* also during the connection, about once a second */
    curl_easy_setopt(c, CURLOPT_NOPROGRESS, 0L);
    slow_start = token ? osGetTime() : 0;
    if (on_progress) { /* github.com redirects to release-assets.githubusercontent.com */
        curl_easy_setopt(c, CURLOPT_FOLLOWLOCATION, 1L);
        curl_easy_setopt(c, CURLOPT_MAXREDIRS, 5L);
        curl_easy_setopt(c, CURLOPT_MAXFILESIZE_LARGE, (curl_off_t)NET_MAX_FILE);
    }

    r = curl_easy_perform(c);
    status = -(long)r;
    if (r == CURLE_OK) curl_easy_getinfo(c, CURLINFO_RESPONSE_CODE, &status);
    curl_slist_free_all(headers);
    return status;
}

long net_get(const char *url, const char *token, char **body, size_t *size)
{
    buffer_t b = { NULL, 0, 0 };
    long status = request(url, token, "3ds", NULL, NULL, 0, &b);

    if (status <= 0) {
        free(b.bytes);
        b.bytes = NULL;
        b.size = 0;
    } else if (!b.bytes) {
        b.bytes = calloc(1, 1);
    }
    *body = b.bytes;
    *size = b.size;
    return status;
}

long net_github(const char *url, void (*file_progress)(size_t, size_t), char **body, size_t *size)
{
    long status;
    on_progress = file_progress;
    status = net_get(url, NULL, body, size);
    on_progress = NULL;
    return status;
}

long net_put(const char *url, const char *token, const char *client, const char *condition, const void *bytes, size_t size)
{
    buffer_t b = { NULL, 0, 0 };
    long status = request(url, token, client, condition, bytes, size, &b);
    free(b.bytes);
    return status;
}

/* errors.c reads curl codes without curl.h: same values. */
_Static_assert(CURLE_COULDNT_RESOLVE_HOST == 6 && CURLE_COULDNT_CONNECT == 7 && CURLE_OPERATION_TIMEDOUT == 28 &&
               CURLE_SSL_CONNECT_ERROR == 35 && CURLE_PEER_FAILED_VERIFICATION == 60 && CURLE_RECV_ERROR == 56 &&
               CURLE_PARTIAL_FILE == 18 && CURLE_FILESIZE_EXCEEDED == 63,
               "codes curl de errors.c");

int net_wifi(void)
{
    u32 status;
    return R_FAILED(ACU_GetStatus(&status)) || status != 1; /* 1: no access point */
}
