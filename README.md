# Download Disclaimer

Your antivirus software may show a message about unrecognized apps or files when you attempt to download it. 
This is expected behavior for small, free, indie software without a paid code-signing certificate. Click More info, then Run anyway to launch it.

# N5Etools

A portable, offline desktop app for the **Naruto 5e** homebrew TTRPG: character
creation, a persistent interactive character sheet, and built-in dice rolling.
Ships as a single executable with the rules database embedded — no installs,
no config files, no separate database file to manage. Written in Golang, with
some HTML, CSS and JavaScript sprinkled in to make the web server ✨ pop ✨

## Download

Prebuilt binaries for Windows, Linux, and macOS are
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
| `testdata/`          | Golden fixtures cut from sourcebook pages.                             |

## The two databases

- **Rules DB** — read-only game content (clans, classes, jutsu, feats, gear),
  built by `n5e-ingest` from the sourcebook PDFs and embedded into the app
  executable at build time.
- **`characters.db`** — created next to the executable on first run; holds
  player characters. Copy the folder and everything travels with it.

## Security

The server binds to `127.0.0.1` only and every request must carry a per-launch secret 
token (set as a cookie on first load, (the same pattern Jupyter notebooks use) plus a
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
