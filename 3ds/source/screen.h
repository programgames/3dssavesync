/* Drawing of both screens with citro2d: text in the system font (accents, button icons),
 * rectangles, game icon. */
#ifndef SCREEN_H
#define SCREEN_H

#include <3ds.h>

#define RGB(c) (0xFF000000u | ((c) & 0xFF) << 16 | ((c) & 0xFF00) | ((c) >> 16 & 0xFF)) /* 0xRRGGBB -> citro2d */

void screen_init(void);
void screen_exit(void);

/* One frame: screen_begin, then screen_top or screen_bottom before drawing on that screen, then screen_end. */
void screen_begin(void);
void screen_top(void);
void screen_bottom(void);
void screen_end(void);

void screen_rect(float x, float y, float l, float h, u32 color);

/* Text on one line, cut with "…" beyond width (0 = no limit). Returns the drawn width. */
float screen_text(float x, float y, float size, u32 color, float width, const char *s);

/* Width of the text on one line. */
float screen_text_width(float size, const char *s);

/* Text on several lines, wrapped before width. Returns the drawn height. */
float screen_paragraph(float x, float y, float size, u32 color, float width, const char *s);

/* The same on max lines at most: the last one ends with "…" if text remains. Returns the drawn height. */
float screen_lines(float x, float y, float size, u32 color, float width, int max, const char *s);

/* Height this paragraph would take, without drawing it. */
float screen_paragraph_height(float size, float width, const char *s);

/* RGB565 icon, row by row (l, h <= 64), scaled by scale. */
void screen_icon(const u16 *pixels, int l, int h, float x, float y, float scale);

#endif
