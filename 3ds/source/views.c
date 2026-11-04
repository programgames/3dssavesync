#include <stdarg.h>
#include <stdio.h>
#include <string.h>
#include "text.h"
#include "views.h"

void (*draw_current_top)(void);
const char *operation;
u32 progress_color = C_GREEN;

static struct { float x, y, l, h; u32 button; } buttons[8]; /* touch areas of the frame */
static int button_count;

const char *glyph(u32 button)
{
    switch (button) {
    case KEY_A: return "\xEE\x80\x80"; /* U+E000 to U+E003 */
    case KEY_B: return "\xEE\x80\x81";
    case KEY_X: return "\xEE\x80\x82";
    case KEY_Y: return "\xEE\x80\x83";
    case KEY_START: return "START";
    case KEY_TOUCH: return ""; /* touch only (Search): no button */
    default: return "SELECT";
    }
}

static u32 glyph_color(u32 button)
{
    switch (button) {
    case KEY_A: return GLYPH_A;
    case KEY_B: return GLYPH_B;
    case KEY_X: return GLYPH_X;
    case KEY_Y: return GLYPH_Y;
    default: return GREY_LIGHT;
    }
}

/* START and SELECT: the word in a pill, 6 pixels wider. */
static float glyph_width(u32 t)
{
    return t & (KEY_START | KEY_SELECT) ? screen_text_width(0.34f, glyph(t)) + 6 : screen_text_width(0.55f, glyph(t));
}

static float button_width(const button_t *t, float size) { return glyph_width(t->button) + 4 + screen_text_width(size, t->text); }

void touch_area(float x, float y, float l, float h, u32 button)
{
    if (button_count == 8) return;
    buttons[button_count].x = x;
    buttons[button_count].y = y;
    buttons[button_count].l = l;
    buttons[button_count].h = h;
    buttons[button_count++].button = button;
}

/* Smaller text if the row does not fit (margin included) with at least 10 pixels between two buttons. */
void draw_button_row(const button_t *t, int n, float y, float gap)
{
    float l[6], total, x = 6, size = 0.44f;
    int i;
    do {
        size -= 0.02f;
        total = 0;
        for (i = 0; i < n; i++) {
            l[i] = button_width(&t[i], size);
            total += l[i];
        }
    } while (size > 0.34f && total * WIDTH_MARGIN + 10 * (n - 1) > ROW_WIDTH);
    if (!gap) gap = n > 1 ? (ROW_WIDTH - total) / (n - 1) : 0;
    for (i = 0; i < n; i++) {
        float g = glyph_width(t[i].button);
        if (t[i].button & (KEY_START | KEY_SELECT)) { /* read as a button, not as text */
            screen_rect(x, y + 4, g, 12, GREY_DARK);
            screen_text(x + 3, y + 4, 0.34f, WHITE, 0, glyph(t[i].button));
        } else if (t[i].button == KEY_TOUCH) {
            screen_rect(x, y + 1, l[i] + 4, 18, CARD); /* touch only: the word in a box, read as a button */
        } else screen_text(x, y + 2, 0.55f, t[i].active ? glyph_color(t[i].button) : GREY_DARK, 0, glyph(t[i].button));
        screen_text(x + g + 4, y + 3, size, t[i].active ? WHITE : GREY_DARK, 0, t[i].text);
        touch_area(x - gap / 2, y - 2, l[i] + gap, BUTTON_HEIGHT, t[i].button); /* the button and half of the gap on each side */
        x += l[i] + gap;
    }
}

/* Button under the stylus, 0 if none. */
static u32 touched_button(void)
{
    touchPosition p;
    int i;
    hidTouchRead(&p);
    for (i = 0; i < button_count; i++)
        if (p.px >= buttons[i].x && p.px < buttons[i].x + buttons[i].l && p.py >= buttons[i].y && p.py < buttons[i].y + buttons[i].h)
            return buttons[i].button;
    return 0;
}

void draw_bar_background(int lines)
{
    float y = SCREEN_HEIGHT - lines * BUTTON_HEIGHT - 7;
    screen_rect(0, y, BOTTOM_WIDTH, SCREEN_HEIGHT - y, BAR);
}

/* The band grows as high as the text needs, over the details: the message stays whole (a bit smaller if needed;
 * "…" only if it still overflows the screen). */
void draw_band(float bottom, const char *text, const char *technical, u32 color)
{
    float size, l1, l2 = technical && technical[0] ? screen_paragraph_height(0.38f, BAND_WIDTH, technical) : 0, h;
    int lines, room;
    if (!text[0]) return;
    for (size = 0.42f;; size = 0.36f) {
        l1 = screen_paragraph_height(size, BAND_WIDTH, "X");
        lines = (int)(screen_paragraph_height(size, BAND_WIDTH, text) / l1 + 0.5f);
        room = (int)((bottom - 12 - l2) / l1); /* up to 6 px from the top of the screen */
        if (lines <= room || size < 0.4f) break;
    }
    if (lines > room) lines = room < 1 ? 1 : room;
    h = lines * l1 + l2 + 6;
    screen_rect(0, bottom - h, BOTTOM_WIDTH, h, CARD);
    screen_rect(0, bottom - h, 3, h, color);
    screen_lines(10, bottom - h + 3, size, WHITE, BAND_WIDTH, lines, text);
    if (l2) screen_paragraph(10, bottom - h + 3 + lines * l1, 0.38f, GREY_LIGHT, BAND_WIDTH, technical);
}

float detail(float y, const char *label, const char *value)
{
    screen_text(8, y, 0.42f, GREY_LIGHT, 100, label);
    screen_text(112, y, 0.42f, WHITE, 200, value);
    return y + 15;
}

u32 read_keys(void)
{
    u32 k = 0;
    while (!k && aptMainLoop()) {
        gspWaitForVBlank();
        hidScanInput();
        k = hidKeysDownRepeat() & ~KEY_TOUCH;
        if (hidKeysDown() & KEY_TOUCH) k |= touched_button();
    }
    return k ? k : KEY_B;
}

/* The top screen keeps its context (draw_current_top). At most 20 frames per second: a frame waits for the screen, and
 * must not slow down the read. */
void show_progress(int done, int total, const char *format, ...)
{
    static u64 last_frame;
    char t[300], *nl;
    float y = 40;
    va_list a;
    u64 now = osGetTime();
    if (total && done > 1 && done < total && now - last_frame < 50) return;
    last_frame = now;
    va_start(a, format);
    vsnprintf(t, sizeof t, format, a);
    va_end(a);
    screen_begin();
    draw_current_top();
    screen_bottom();
    if (operation) {
        screen_text(16, y, 0.55f, WHITE, PROGRESS_WIDTH, operation);
        y += 22;
    }
    if ((nl = strchr(t, '\n'))) { /* the step in grey, then the game */
        *nl = '\0';
        screen_text(16, y, 0.45f, GREY_LIGHT, PROGRESS_WIDTH, t);
        screen_lines(16, y + 18, 0.55f, WHITE, PROGRESS_WIDTH, 2, nl + 1);
    } else screen_lines(16, y, 0.55f, WHITE, PROGRESS_WIDTH, 3, t);
    if (total > 0) {
        char n[32];
        screen_rect(16, 140, PROGRESS_WIDTH, 10, CARD);
        screen_rect(16, 140, (float)PROGRESS_WIDTH * done / total, 10, progress_color);
        snprintf(n, sizeof n, "%d / %d", done, total);
        screen_text(16, 156, 0.45f, GREY_LIGHT, 0, n);
        snprintf(n, sizeof n, T("%d %%", "%d%%"), (int)(100LL * done / total));
        screen_text(16 + PROGRESS_WIDTH - screen_text_width(0.45f, n), 156, 0.45f, GREY_LIGHT, 0, n);
    }
    screen_end();
}

void modal_begin(void (*top)(void))
{
    screen_begin();
    (top ? top : draw_current_top)();
    screen_bottom();
    button_count = 0;
}

u32 modal_end(const button_t *b, int n, float gap)
{
    draw_bar_background(1);
    draw_button_row(b, n, BAR_ROW_1, gap);
    screen_end();
    return read_keys();
}

int confirm(const char *title, const char *subtitle, const char *text, const char *warning, const char *action)
{
    const button_t t[2] = { { KEY_A, action, 1 }, { KEY_B, T("Annuler", "Cancel"), 1 } };
    for (;;) {
        char line[300], *tab;
        const char *s = text;
        float y = 6;
        u32 k;
        modal_begin(NULL);
        y += screen_lines(8, y, 0.55f, WHITE, TEXT_WIDTH, 2, title) + 2;
        if (subtitle) y += screen_lines(8, y, 0.45f, GREY_LIGHT, TEXT_WIDTH, 1, subtitle);
        y += 8;
        while (*s) {
            size_t n = strcspn(s, "\n");
            snprintf(line, sizeof line, "%.*s", (int)n, s);
            if ((tab = strchr(line, '\t'))) {
                *tab = '\0';
                y = detail(y, line, tab + 1);
            } else y += line[0] ? screen_paragraph(8, y, 0.45f, WHITE, TEXT_WIDTH, line) : 6;
            s += n + (s[n] == '\n');
        }
        if (warning) draw_band(BAR_TOP_1, warning, NULL, C_RED);
        k = modal_end(t, 2, 18);
        if (k & KEY_A) return 1;
        if (k & KEY_B) return 0;
    }
}
