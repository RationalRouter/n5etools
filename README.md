# N5E Toolkit

A portable, offline desktop app for the **Naruto 5e** homebrew TTRPG: character
creation, a persistent interactive character sheet, and built-in dice rolling.
Ships as a single executable with the rules database embedded — no installs,
no config files, no separate database file to manage. Written in Golang, with
some HTML, CSS and JavaScript sprinkled in to make the web server ✨ pop ✨

## Download

Prebuilt binaries for Windows, Linux, and macOS (Apple Silicon and Intel) are
attached to each [GitHub Release](https://github.com/RationalRouter/n5etools/releases).
Download the zip for your platform, unzip it, and run the executable — no
build step required. `characters.db` is created next to the executable on
first run and holds your characters; copying the folder moves everything.
Binaries for other OS's that are compatible with Golang compiling are available
upon request.

## Staying up to date

On every launch, the app checks each official sourcebook on Google Drive in
the background (`internal/autoupdate`) and re-ingests anything newer — no
manual step required, and it never blocks startup. If no connection is
available, the check is skipped and the app continues with whatever's
already in the local database.

## Layout

| Path                 | What lives here                                                        |
| -------------------- | ---------------------------------------------------------------------- |
| `cmd/n5e`            | The player-facing desktop app.                                         |
| `cmd/n5e-ingest`     | Maintainer CLI: extract → parse → load → validate → report.            |
| `internal/schema`    | Versioned SQL migrations for both databases + migration runner.        |
| `internal/extract`   | PDF text/image extraction and two-column reading-order reconstruction. |
| `internal/parse`     | Per-sourcebook parsers (jutsu, clans, classes, core book).             |
| `internal/ocr`       | OCR of the class progression-table images + reviewable cache.          |
| `internal/store`     | SQLite access, slug keying, override-preserving upserts.               |
| `internal/validate`  | Post-ingestion validation report ("these N things need your eyes").    |
| `internal/rules`     | All derived-value math (modifiers, AC, max HP/chakra, save DCs).       |
| `seed/`              | Hand-checked seed data (rule tables, reviewed OCR output).             |
| `testdata/`          | Golden fixtures cut from real sourcebook pages.                        |

## The two databases

- **Rules DB** — read-only game content (clans, classes, jutsu, feats, gear),
  built by `n5e-ingest` from the sourcebook PDFs and embedded into the app
  executable at build time.
- **`characters.db`** — created next to the executable on first run; holds
  player characters. Copy the folder and everything travels with it.

## Building from source

Building from source is only needed to develop the app or cut a new release
— see [Download](#download) above for prebuilt binaries. `cmd/n5e` is a
local web server: on launch it picks a free port on `127.0.0.1`, opens the
default browser to it, and keeps running in the background even if that tab
is closed — quit it from the in-page **Quit** button (or Ctrl+C in a
terminal). It has no other CLI surface.

```sh
make build   # embeds out/rules.db, builds dist/n5e and dist/n5e-ingest
make run     # build + launch
```

`out/rules.db` must already exist (built via `n5e-ingest`, see below) before
`make build`/`make embed-rules` will succeed. `make build-windows` cross-builds
a Windows binary with the console window suppressed (`-H=windowsgui`), so
players never see a terminal — a native message box (see
`cmd/n5e/errdialog_windows.go`) covers the one case that would otherwise be
invisible without it: a fatal startup failure. `make build-windows` embeds an
app icon from `assets/n5e.ico` if present (not committed — supply your own;
the build just skips the icon step with a warning if it's missing).
`make release-windows` builds and zips the player-facing exe into
`dist-release/n5e-windows.zip` — a "download, unzip, double-click" package,
no installer, using `tools/zipdist` (stdlib `archive/zip`) so it doesn't
depend on a system `zip` binary being installed.

A note on security: the server binds to `127.0.0.1` only — never reachable from
the network — and every request must carry a per-launch secret token (set as
a cookie on first load, the same pattern Jupyter notebook uses) plus a
matching `Origin` header on any cross-origin-capable request. This blocks the
one realistic residual threat for a long-lived loopback server: another
browser tab trying to script requests against it.

## Ingesting the sourcebooks

Shipped builds keep themselves current automatically — see
[Staying up to date](#staying-up-to-date) above. `n5e-ingest` is the
maintainer CLI behind that same pipeline, used directly for local testing,
bootstrapping a fresh `out/rules.db`, or backfilling a book that has no
parser yet. Each subcommand is a dry run (parse summary + anomaly list)
until `-db` is given.
Run `go run ./cmd/n5e-ingest sources` for the official, creator-maintained
Google Drive link to every current book (see `internal/sources`) — those
links are kept immutable as the game updates, so they're hardcoded rather
than looked up.

```sh
# Inspect extraction quality on any page range
go run ./cmd/n5e-ingest dump  <book.pdf> <page> [endpage]

# Jutsu compendium → jutsu + keywords + summon tribe stat blocks
go run ./cmd/n5e-ingest jutsu -db out/rules.db -version 3.1  Jiraiyas_Jutsu_Compendium.pdf

# Clan compendium → clans, traits, features, clan jutsu, clan feats,
# bloodline latents
go run ./cmd/n5e-ingest clans -db out/rules.db -version 3.11 Tsunades_Studies_Compendium.pdf

# Class compendium → classes, proficiencies, casting, features, subclasses,
# subclass features, option lists (maneuvers/upgrades/mirages/...), class feats
go run ./cmd/n5e-ingest classes -db out/rules.db -version 3.12 Orochimarus_Observation_Compendium.pdf

# Core book → fighting stances, feats, backgrounds, enhancement seals,
# multiclassing rules
go run ./cmd/n5e-ingest core -db out/rules.db -version 3.11 "Naruto 5e - Full Document.pdf"

# Community Mastersheet → the tables that print as images in the PDFs:
# class progression charts, armor/weapon stats. Bootstrap only — see the
# note in internal/sources/books.go about replacing this with real OCR
# against the PDF images after v1 ships.
go run ./cmd/n5e-ingest sheet -db out/rules.db -version 3.1 "Mastersheet - N5E v3.1.xlsx"
```

## Design rules

1. Every content entity gets a **stable slug primary key** at ingestion
   (`jutsu/chakra-blow`). Nothing is ever re-typed somewhere else to be matched.
2. **Overrides are first-class**: parsed columns sit beside `*_override`
   columns and a `detection_status` flag; effective values come from views.
   Re-running ingestion updates parsed values and never touches overrides.
3. Ingestion never imports silently — it always ends with a **validation
   report** listing everything that needs to be looked over.
4. Derived values (ability modifiers, AC, max HP/chakra) are **computed in
   `internal/rules`**, not stored. Stored state is inputs + mutable play state.
