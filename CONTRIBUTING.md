# Contributing to 3dssavesync

Thanks for your interest. Bug reports, emulator profiles and fixes are all welcome. This page explains how the repository is laid out, how to build and test each part, and the few rules every change follows.

Found a bug? Check [troubleshooting](docs/troubleshooting.md) first, then open an issue with the bug report form. For a security problem, see [SECURITY.md](SECURITY.md) instead.

## The parts

| Folder | What | Language |
|---|---|---|
| `client/` | The PC program: sync, settings page, local server, icon near the clock | Go |
| `3ds/` | The 3DS app (homebrew) | C, libctru, citro2d |
| `server/` | The server for your own machine (Docker) | Python, standard library only |
| `tests/cas_etat.csv` | The sync rule's case table, read by the C and Go tests | CSV |
| `pc/forwarder3ds.py` | Builds HOME menu forwarders for GB and GBA games | Python |
| `docs/` | User guides, design, [ADRs](docs/adr/README.md), [glossary](docs/GLOSSARY.md) | Markdown |

[docs/design.md](docs/design.md) explains the sync rule and the server contract. Read it before you change anything that moves a save.

## Build and test

### PC program (`client/`)

You need the Go version named in `client/go.mod`, and Python 3 on your `PATH`: the Go tests run the real `server/server.py`.

```bash
cd client
go build .          # gives 3dssavesync (3dssavesync.exe on Windows)
gofmt -l .          # must print nothing
go vet ./...
go test ./...
```

CI also runs `go vet` for Linux and macOS, and [staticcheck](https://staticcheck.dev/) (the pinned version is in `.github/workflows/ci.yml`).

Tests that touch the user profile (shortcuts, autostart, Recycle Bin, settings folder) go through `fakeHome(t)` and the test adapters in `client/profiles_test.go`. A test must never create a real shortcut or write in your real settings folder.

### 3DS app (`3ds/`)

Install [devkitPro](https://devkitpro.org/wiki/Getting_Started) with devkitARM, then the libraries:

```bash
dkp-pacman -S 3ds-dev 3ds-curl 3ds-mbedtls 3ds-zlib
cd 3ds
make                # 3dssavesync.3dsx; must print no warning
make cia            # also needs makerom and bannertool in PATH
```

`make VERSION=3.7.0` sets the version shown in the app. A `dev` build never offers an update.

The parts of the app that don't depend on libctru (sync rule, keys, file helpers, settings files, Pokémon data, error messages, texts, update check) have tests that run on your PC with plain `gcc`. The exact commands are in the `tests-c` job of `.github/workflows/ci.yml`. They build with `-Wall -Wextra -Werror`. On Windows, use the `gcc` of devkitPro's MSYS2 shell: Git Bash's `gcc` has no C headers.

### Server (`server/`)

```bash
cd server
python -m unittest test_server.py
NDSSYNC_URL=https://saves.example.com NDSSYNC_TOKEN=... python -m unittest test_server.py   # against a deployed server
```

CI also runs [ruff](https://docs.astral.sh/ruff/) on `server/`.

### Before you open a pull request

Run every check of the part you changed. GitHub Actions runs all of them on Windows, Linux and macOS, and builds the `.cia`.

## Rules for changes

- **A save is never lost.** Before a save is replaced, a backup copy is made. Files are written to a temporary file, checked, then renamed; never in place. A conflict is never settled without asking. The server never deletes a version.
- **Errors say what to do.** A message says what happened and what to do, in plain words, then the technical detail (error text, HTTP code, path) on its own line so people can report it. Nothing fails silently.
- **Tests check behaviour.** New logic comes with a test at an existing seam (`3ds/test/*.c`, `client/*_test.go`). Expected values are literals, not recomputed with the code under test. Check that the test fails without your change.
- **Few dependencies.** The server uses the Python standard library. The Go program has one dependency. Ask in an issue before you add one.
- **ADRs are for hard-to-reverse decisions only**: the server's data format, the key of a game, the trust model of updates, things that would break existing saves or installs if changed. A UI choice or a refactor doesn't need one. See [docs/adr/](docs/adr/README.md).
- **Small pull requests.** Say what changed, why, which tests you ran, and what still needs a test on a real 3DS or PC.

## Language

3dssavesync started as a French project. Here's where things stand:

- **Docs, commits, pull requests and issues are in English** from v3.7 on. `README.fr.md` is a short French summary. The notes in `docs/notes/` stay in French: they're the record of past research.
- **Code is moving to English.** Most identifiers and comments are still French. They're being renamed in dedicated pull requests, a part at a time. Write new code in English, and don't mix renames into a fix or a feature. [docs/GLOSSARY.md](docs/GLOSSARY.md) gives the English term for each French name.
- **The PC program (`client/`) is in English**, identifiers and comments. What stays French on purpose, so existing installs keep working: the keys of `config.json`, the JSON fields of the server protocol and of the settings page, and the names of files on disk (`bases/`, `en-attente.json`…). The renames are pure commits listed in [.git-blame-ignore-revs](.git-blame-ignore-revs): run `git config blame.ignoreRevsFile .git-blame-ignore-revs` once, and `git blame` skips them.
- **The apps speak French and English.** Every text shown to users exists in both languages: `tr("fr", "en")` in Go, `T("fr", "en")` in C, `{{t}}` in the settings page. French texts use "vous", never "tu". Keep 3DS texts short: the screen is small.

## AI-assisted development

Most of this code was written with [Claude Code](https://claude.com/claude-code) (Anthropic's Claude models), driven and reviewed by the maintainer. The project doesn't hide it:

- Commits written with the tool end with a `Co-Authored-By: Claude` trailer, and pull requests say so at the bottom.
- The maintainer decides what gets built, reads every change, and tests on a real 3DS and real PCs before a release.
- No change is merged on the tool's word. CI must pass, and new behaviour needs a test that fails without it.
- The safety rules above apply to every change, whoever or whatever wrote it. They're also why the tests use a fake user profile and a throwaway server.

You're welcome to use AI tools for your contributions too. You're responsible for what you submit: understand it, test it, and say in the pull request that you used one.

## License

By contributing, you agree that your work is released under the [MIT License](LICENSE).
