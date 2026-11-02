# Design: translation system

Status: proposal. Nothing here is implemented yet. The migration plan (section 12) lists the steps.

## 1. Goal and constraints

A contributor adds a language (Spanish, German, Italian…) by writing **one text file per side**. The contributor does not touch the code. The contributor tests the file without recompiling: they copy it to the SD card or to the PC config folder. A release then ships the file inside the 3DS app and the PC program.

Hard constraints:

- The existing calls stay as they are: `T("fr", "en")` in `3ds/source/main.c` (136 calls), `tr("fr", "en")` in `client/*.go` (159 calls), `{{t "fr" "en"}}` in `client/assistant.html` (52 calls). That is 347 pairs and 317 distinct English strings (about 10 KB of English text).
- 3DS: C, libctru, no gettext, no JSON library in the link line today. Limited memory. Files come from `sdmc:/3ds/3dssavesync/`. Text is drawn with citro2d and the console's system font.
- PC: one static Go binary. `embed.FS` is available. No new dependency.
- Format strings (`%s`, `%d`, `%ld`) keep the argument order. 101 of the 347 pairs contain a verb.
- A bad translation file must never crash the app or block a sync. The worst outcome is English text.
- The journal (log) stays in French, as today.

Non-goals (YAGNI): right-to-left scripts, a plural engine, per-string context IDs, translating the server (`server/server.py` has no user-facing text), translating the docs.

## 2. What similar projects do

| Project | Format | Key | Where the files live | Language choice |
|---|---|---|---|---|
| TWiLight Menu++ (DS/3DS) | INI, one `language.ini` per language folder | ID (`DONE=¡Hecho!`) | NitroFS inside the app | Setting, default = system |
| Universal-Updater (3DS) | flat JSON, `romfs:/lang/<code>/app.json` | ID (`"ACTION_FAILED": "¡%s falló!"`) | RomFS | Setting, default = `CFGU_GetSystemLanguage` mapped to codes |
| Checkpoint (3DS/Switch) | one JSON for all languages: `key → {lang → text}` | dotted ID (`settings.title`) | RomFS | System language; missing → English → key |
| Syncthing (web GUI, Go) | flat JSON per language, `lang-de.json` | **the English source string** | embedded in the binary | Browser language |
| go-i18n (Go library) | JSON/TOML/YAML per language | ID + optional default text | embedded or files | caller decides |
| gettext (C, many Linux tools) | `.po` → `.mo` | **the English source string** (`msgid`) | system dir | `LANGUAGE`, `LC_*`, `LANG` |

What we keep from them:

- **RomFS for embedded defaults on 3DS** (Universal-Updater, Checkpoint). The app opens `romfs:/lang/<code>.json` like any file. No code change per language.
- **The CFG_LANGUAGE → code table** of Universal-Updater (`jp, en, fr, de, it, es, zh-CN, ko, nl, pt, ru, zh-TW`). We use `ja` (ISO 639-1) instead of `jp`.
- **"Missing entry → show the English text"** (all of them). Never an empty label.
- **English source string as key, flat JSON per language** (Syncthing, gettext). Our call sites already contain the English text. Syncthing proves the format works with Weblate.
- **Never crash on a malformed file** (Checkpoint parses non-throwing and falls back).

What we do not take: IDs (TWiLight, Universal-Updater, Checkpoint, go-i18n). IDs would mean rewriting 347 call sites and keeping an ID ↔ English table in sync. Checkpoint's single file for all languages makes every contributor edit the same file, so pull requests conflict.

## 3. Decision: English text as key, French stays in the code

```
T("Lecture des ROM : %d jeu(x)…", "Reading ROMs: %d game(s)…")
                                    └── key in every catalog
```

- French and English stay in the source code. They are the two built-in languages. Today's behavior for French and English users does not change, even if every catalog file is missing.
- Any other language is a **catalog**: a flat JSON object that maps the English string to the translation.
- Only the definitions of `T` and `tr` change. The call sites do not change.

Cost of this choice: two different French strings with the same English text share one key. Today there are 3 such cases: `Upload` (Envoi / Envoyer), `Download` (Récupérer / Récupération), `Sync all` (Tout synchroniser / Tout synchro). The noun and the verb can need two words in another language. The extractor reports these collisions. The fix is to make the English text distinct (for example "Downloading" for the title). We add no context mechanism.

A second cost: when a developer changes an English string, the old translation no longer matches. The string shows in English until a translator updates it. That is the gettext behavior and it is safe.

## 4. File format

One UTF-8 JSON object per language and per side. Keys and values are strings. No nesting.

```json
{
  "Reading ROMs: %d game(s)…": "Leyendo ROM: %d juego(s)…",
  "Reading 3DS saves\n%s": "Leyendo partidas de 3DS\n%s",
  "Server unreachable: %s": "",
  "Settings": "Ajustes"
}
```

Rules:

- `""` (empty) means "not translated yet". The app shows the English key.
- Keys appear in source order (file, then line), not in alphabetical order. Strings of the same screen stay together, which gives the translator context.
- `en.json` is the **template**: generated, value = key. A translator copies it to `es.json` and changes the values. Weblate's "JSON file" format can use `en.json` as the monolingual base file if we ever connect Weblate (not part of this plan).

Why JSON and not INI or `.po`:

- INI (TWiLight) needs IDs as keys: an English sentence with `=`, `:` or `\n` is not a valid INI key.
- `.po` is good (source references, `c-format` checks in `msgfmt`). But it needs gettext tools to check, and the PC side would need a `.po` parser too. JSON is read by `encoding/json` on PC. On 3DS a flat-object parser is about 80 lines of C (section 6).
- Every translator's editor and every translation platform reads JSON.

### Two catalogs per language

| Side | Template | Translations | Content |
|---|---|---|---|
| 3DS | `3ds/romfs/lang/en.json` | `3ds/romfs/lang/<code>.json` | the `T(...)` strings |
| PC | `client/lang/en.json` | `client/lang/<code>.json` | the `tr(...)` strings and the web page `{{t}}` strings |

Two files instead of one: each side embeds only what it shows, and the translator knows which strings go on the small 3DS screen. A new language = two new files, `3ds/romfs/lang/es.json` and `client/lang/es.json`.

### Language codes

File name = language code: lowercase ISO 639-1, plus a region only when the language needs it (`pt-BR`, `zh-CN`, `zh-TW`). The same names on both sides.

## 5. Placeholders

A translation must contain **the same conversion specifications, in the same order**, as its key. Example: key `"%s: upload refused, HTTP %ld."` → the translation needs exactly `%s` then `%ld`.

- Spec pattern: `%[flags][width][.precision][length]conversion`, with `%%` ignored. Compare the full spec text, so `%ld` ≠ `%d`.
- No positional arguments (`%1$s`, `%[1]s`). newlib's printf on devkitARM does not promise them, and the same rule on both sides is simpler. If a language needs another word order, the translator rephrases ("Error (%s): %s" style).
- The web page has named tokens: `{rom}` and `{sauvegardes}` (2 strings). The translation must keep the same set of `{name}` tokens.

This rule is checked **twice**:

1. In CI, for the catalogs in the repo (section 9).
2. **At load time, in the app**, for every entry. A user can copy an unchecked file to the SD card. A wrong verb in a C `snprintf` (`%s` with an `int` argument) can crash the 3DS. So the loader drops any entry whose placeholders differ from its key. That string then shows in English. This check is a trust-boundary check, not an optimization: it stays.

## 6. 3DS side

### Lookup

```c
static int francais;  /* replaces "anglais" */
#define T(fr, en) (francais ? (fr) : traduire(en))
```

`traduire(en)` returns the catalog value for `en`, or `en` itself. With no catalog loaded it returns `en`, so English users see exactly what they see today. Call sites that pass struct fields (`T(l[i].nom, l[i].nom_en)` in the settings screen) work unchanged: the lookup uses the English text at run time, not at compile time.

### New module `3ds/source/langue.c` (no libctru, tested on the host)

```c
/* Charge un catalogue JSON plat (clé anglaise → traduction) depuis un tampon modifiable.
 * Le tampon reste la propriété du module ; les chaînes pointent dedans. 0 si chargé. */
int catalogue_charger(char *json, size_t n);
const char *traduire(const char *en);   /* traduction, ou en */
```

- Read the whole file into one `malloc` buffer. Refuse files bigger than 256 KB. A full catalog is about 30 KB.
- Parse in place: unescaping never makes a string longer, so keys and values stay in the buffer. Supported escapes: `\" \\ \/ \b \f \n \r \t \uXXXX` (with surrogate pairs → UTF-8).
- Any syntax error → free the buffer, no catalog, English everywhere.
- Store `{key, value}` pointer pairs in one array (about 320 × 8 bytes). Drop entries with an empty value or a placeholder mismatch. `qsort` by key once; `bsearch` with `strcmp` for each `T()`.
- Cost per frame: a screen calls `T` about 20 times; each lookup is about 9 `strcmp`. That is negligible next to citro2d text layout.

The test `3ds/test/test_langue.c` covers: a valid file, each escape, `é` and a surrogate pair, a truncated file (rejected), an entry with `%d` instead of `%s` (dropped), an empty value (English), a missing key (English). It runs in the `tests-c` CI job like the other host tests.

### Embedded catalogs: RomFS

- `3ds/Makefile`: uncomment `ROMFS := romfs` (the template line is already there).
- `3ds/app.rsf`: add `RomFs: RootPath: romfs` so the `.cia` gets the same files as the `.3dsx`.
- `main.c`: `romfsInit()` at start, `romfsExit()` at exit.

Rejected alternative: generate a C file with all catalogs as string constants. It needs no RomFS, but it adds a generated source file to every diff and grows the binary with every language. RomFS is the native mechanism and the one Universal-Updater and Checkpoint use.

### Language detection and fallback

```
CFG_LANGUAGE_JP=0 → ja    EN=1 → en    FR=2 → fr    DE=3 → de    IT=4 → it     ES=5 → es
ZH=6 → zh-CN              KO=7 → ko    NL=8 → nl    PT=9 → pt    RU=10 → ru    TW=11 → zh-TW
```

1. Code = `langue=` in `config.ini` if set, else the console language from the table.
2. `fr` → `francais = 1`, no catalog. `en` → no catalog.
3. Else load the first file that exists: `sdmc:/3ds/3dssavesync/lang/<code>.json` (user override), then `romfs:/lang/<code>.json`.
4. No file, or a file that does not parse → English.
5. Per string: catalog value if present, non-empty and placeholder-compatible; else English.

The override file **replaces** the embedded catalog; it does not merge with it. The behavior is predictable for a translator. After a release ships the translation, the translator deletes the override file.

The `langue=` key lets a translator test a language without changing the console language. `ecrire_config()` writes all keys back, so it must also write `langue=` (one more `%s` in its `fprintf`). Value empty = console language.

### Fonts

The app draws with the system font (`fontEnsureMapped`). The 3DS lets a user pick Chinese or Korean only on a console of that region, and that console has the matching system font. So the console language always has its glyphs. Only a forced `langue=ko` on a European console can show empty boxes. We accept that; we do not load other fonts.

### Screen width

`ecran.c` already shrinks or wraps long text. Translations are often 20–30 % longer than English. The validator warns (does not fail) when a 3DS translation is more than 1.5 times the length of its key, so the translator can check that screen on the console.

## 7. PC side (Go)

### Lookup

```go
//go:embed lang/*.json
var langues embed.FS

var anglais = ...             // kept: "not French" (tests set it)
var catalogue map[string]string

func tr(fr, en string) string {
	if !anglais {
		return fr
	}
	if v := catalogue[en]; v != "" {
		return v
	}
	return en
}
```

- `catalogue` is loaded in the same package-level initialization that sets `anglais`. Go orders package initialization by dependency, so a package-level `var x = tr(...)` still sees the catalog.
- Loading: `json.Unmarshal` into `map[string]string`, then delete entries with a placeholder mismatch (same function as the validator, section 9). A parse error → `catalogue = nil` and one line in the journal (in French, like every journal line).
- Existing tests set `anglais = true/false` and read French or English text. They keep working because `catalogue` stays `nil` in tests.

### Language detection

Today `langueSysteme()` returns `"fr"` or `"en"` on Windows, and the raw locale string elsewhere. New contract: return a language tag, then normalize it.

- Windows: `GetUserDefaultUILanguage` (already called), then `LCIDToLocaleName` (kernel32, Vista+) → `es-ES`. The current `id & 0x3FF == 0x0C` test goes away.
- Linux: `LANGUAGE` (colon list, take each entry in order), `LC_ALL`, `LC_MESSAGES`, `LANG` (already the case).
- macOS: `defaults read -g AppleLocale` (already the case) → `es_ES`.
- Translator override, all systems: environment variable `LANGUAGE=es`. On Windows only `LANGUAGE` is read, not `LANG`, because Git Bash and MSYS set `LANG` for their child processes.

Normalization: lowercase, `_` → `-`, cut at `.` or `@` (`es_ES.UTF-8` → `es-es`). Candidates: the full tag, then the base language (`pt-br`, then `pt`). File names compare case-insensitively.

### Fallback

1. Base language `fr` → French (as today, including `fr-CA`, `fr-BE`).
2. Base language `en`, or no tag → English.
3. Else the first file that exists, for each candidate tag: `<os.UserConfigDir()>/3dssavesync/lang/<code>.json` (next to `config.json`; user override), then `lang/<code>.json` in `embed.FS`.
4. No file → English (today's behavior for every non-French user).
5. Per string, as on 3DS.

### Web setup page

`{{t "fr" "en"}}` calls `tr` through `template.FuncMap{"t": tr}`. It gets the catalogs with no change. The JavaScript `T` object in the page is filled by the same template, so it also follows.

Two small fixes:

- `<html lang="{{.Langue}}">` receives `tr("fr", "en")` today. It must receive the active language code.
- The browser language could differ from the OS language. We ignore `Accept-Language`: the page and the program windows use the same language.

### Strings with behavior

`Questions.oui` prints `tr("[O/n]", "[Y/n]")` and accepts only `o`, `oui`, `y`, `yes`. A Spanish translation `[S/n]` would make the answer `s` mean "no". Fix: also accept the first letter of the translated hint, in lowercase. Until then, the validator flags any translation of `[Y/n]` / `[y/N]` that changes the letters.

Dialog buttons: `demander` returns the clicked label and the code compares it with the same `tr(...)` value. That stays correct in every language.

## 8. Extraction (`go generate`)

One stdlib-only tool for both sides: `client/outils/traductions/main.go`. `client/langue.go` gets:

```go
//go:generate go run ./outils/traductions
```

What it reads:

- **Go**: `go/parser` on `client/*.go` (not `_test.go`). Every call `tr(a, b)` where `a` and `b` are string literals, or `+` concatenations of literals. A call with a non-literal argument is an error with `file:line` (none exist today except the definition of `tr`).
- **HTML**: `client/assistant.html`, pattern `{{t "…" "…"}}`. The arguments are Go string literals, so `strconv.Unquote` decodes them.
- **C**: `3ds/source/*.c`, a small scanner for `T(` followed by string literals (adjacent literals are joined), a comma, string literals, `)`. C escapes in our strings (`\n`, `\"`, `\\`) decode with `strconv.Unquote` too. Calls with non-literal arguments are skipped.
- **C tables**: the settings table in `reglages()` keeps French/English pairs in struct fields. Mark each pair with a no-op macro so the scanner finds it:

  ```c
  #define PAIRE(fr, en) fr, en   /* paire de textes du tableau : extraite par outils/traductions */
  { PAIRE("Serveur", "Server"), PAIRE("Adresse du serveur…", "Server address…"), cfg.url, sizeof cfg.url },
  ```

What it writes:

- `client/lang/en.json` and `3ds/romfs/lang/en.json`: every English string, value = key, source order.
- Each existing `<code>.json` is rewritten in template order: existing translations kept, new keys added with `""`, keys no longer in the source removed. Git history keeps the removed text.
- It prints the English key collisions (section 3).

## 9. Validation (CI)

`go run ./outils/traductions -verifier` (same tool) in the `client-go` job, on `ubuntu-latest` only:

| Check | Result |
|---|---|
| File is valid UTF-8 JSON, an object of strings | **error** |
| Same `%` specs in the same order as the key | **error** |
| Same `{name}` tokens as the key | **error** |
| `[Y/n]` / `[y/N]` letters changed | **error** (until `oui` accepts them) |
| Key not in the template (stale) | warning |
| Key missing or empty (untranslated) | info: `es: 212/317 translated` |
| 3DS translation longer than 1.5 × key | warning |
| English key with two French texts | warning |

Plus a freshness check of the templates:

```sh
go generate ./... && git diff --exit-code -- lang ../3ds/romfs/lang/en.json
```

It fails when a developer adds a `tr`/`T` string without regenerating. Translations (`<code>.json`) are not part of the diff check. A translator without Go can open a pull request; missing or stale keys never fail CI.

The placeholder function is the same code at load time (Go) and in the validator. The C loader has its own 15-line copy, tested by `test_langue.c` with the same cases.

## 10. Plurals

No plural engine. Today 7 strings use the neutral form `jeu(x)` / `game(s)`. Rules for translators and developers:

- Use a form that reads correctly for every count: `"%d game(s)"`, or put the number after a label: `"Games: %d"`.
- Languages with more than two forms (Polish, Russian, Arabic; see the CLDR plural rules) use the label form.

If a translator reports a string that cannot be written this way, we add an explicit pair of calls in the code for that string (`n == 1 ? T(...) : T(...)`), not a general mechanism.

## 11. Contributor workflow

1. Copy `client/lang/en.json` to `client/lang/es.json` and `3ds/romfs/lang/en.json` to `3ds/romfs/lang/es.json`.
2. Translate the values. Keep `%s`, `%d`, `%ld`, `{rom}`, `\n` as they are and in the same order.
3. Test on PC: copy `es.json` to the `lang` folder next to `config.json`. Start the program with `LANGUAGE=es` if the OS is not in Spanish.
4. Test on 3DS: copy the 3DS `es.json` to `sdmc:/3ds/3dssavesync/lang/`. Set the console language, or add `langue=es` to `config.ini`.
5. Open a pull request. CI checks the placeholders.

A short `docs/translating.md` holds these steps; the README links it in one line.

## 12. Migration plan

Each step is one small pull request and keeps the app working. Effort: S = under half a day, M = one day, L = more.

| # | Step | Effort |
|---|---|---|
| 1 | ADR "English text as translation key; French and English built in". Go: `catalogue`, new `tr`, tag normalization, override + `embed.FS` loading, placeholder filter, active code for `<html lang>`. Tests in `langue_test.go` (normalization table, fallback order, dropped bad entry). No catalog shipped yet: no visible change. | S |
| 2 | Windows `LCIDToLocaleName`; `LANGUAGE` override on all systems. | S |
| 3 | `outils/traductions`: extract + generate `en.json` templates + `-verifier`; CI step; fix the 3 key collisions; `PAIRE()` in the settings table. | M |
| 4 | 3DS: `langue.c` + `test_langue.c` + CI line; `T` → `traduire`; CFG table; RomFS in Makefile, `app.rsf` and `main.c`; SD override; `langue=` in `config.ini`. Test on a real console in French, English and with an override file. | M |
| 5 | `Questions.oui` accepts the translated letter; remove the validator rule for it. | S |
| 6 | `docs/translating.md`, one README line. | S |
| 7 | First real language from a contributor (or a machine draft reviewed by a native speaker). | — |

Not planned: Weblate or Crowdin hosting (add when two or more regular translators appear; the file format already fits), merge of override and embedded catalogs, plural forms, context IDs.

## 13. Sources

- TWiLight Menu++, Spanish `language.ini` (ID keys, INI): https://github.com/DS-Homebrew/TWiLightMenu/blob/master/settings/nitrofiles/languages/es/language.ini
- Universal-Updater, loader (`romfs:/lang/<code>/app.json`, missing key → key, missing file → `en`): https://github.com/Universal-Team/Universal-Updater/blob/master/source/utils/lang.cpp
- Universal-Updater, `CFGU_GetSystemLanguage` → code table: https://github.com/Universal-Team/Universal-Updater/blob/master/source/utils/config.cpp
- Checkpoint, loader (non-throwing parse, fallback to English, then to the key): https://github.com/BernardoGiordano/Checkpoint/blob/master/common/i18n.cpp
- Checkpoint, single multi-language catalog: https://github.com/BernardoGiordano/Checkpoint/blob/master/3ds/assets/romfs/i18n.json
- Syncthing, English-keyed flat JSON (`lang-de.json`), generated from Weblate: https://github.com/syncthing/syncthing/blob/main/gui/default/assets/lang/lang-de.json
- libctru `CFG_Language` enum: https://github.com/devkitPro/libctru/blob/master/libctru/include/3ds/services/cfgu.h
- libctru RomFS: https://github.com/devkitPro/libctru/blob/master/libctru/include/3ds/romfs.h
- Go `embed`: https://pkg.go.dev/embed — `os.UserConfigDir`: https://pkg.go.dev/os#UserConfigDir
- go-i18n (ID-based Go catalogs, for comparison): https://github.com/nicksnyder/go-i18n
- `golang.org/x/text/message` (gotext extraction, for comparison): https://pkg.go.dev/golang.org/x/text/message
- GNU gettext manual (`msgid` = source text, `c-format` checks, plural forms): https://www.gnu.org/software/gettext/manual/gettext.html
- Weblate JSON formats: https://docs.weblate.org/en/latest/formats/json.html
- CLDR plural rules: https://www.unicode.org/cldr/charts/latest/supplemental/language_plural_rules.html
- Windows `GetUserDefaultUILanguage`: https://learn.microsoft.com/en-us/windows/win32/api/winnls/nf-winnls-getuserdefaultuilanguage — `LCIDToLocaleName`: https://learn.microsoft.com/en-us/windows/win32/api/winnls/nf-winnls-lcidtolocalename
