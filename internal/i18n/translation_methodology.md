# Translation methodology

How this project approaches localizing game content and UI text into a new language. Written
after working through Spanish, so future languages (Portuguese, etc.) can follow this process
directly instead of re-deriving or re-discussing it from scratch. `glossary.md` holds the actual
approved terms for a given language; this file describes *how* those terms get chosen and applied.

## Scope: everything player-facing, UI and game content alike

As of the DeepL-API pivot (2026-09-05), translation is not limited to the UI string catalog.
**Every player-facing string in the app is in scope** — the UI catalog (`internal/i18n/locales/
*.json`, nav labels, buttons, form labels, tooltips, patch notes, credit/footer text, error
messages) *and* every translatable field of every game-content table registered in
`internal/i18n.TranslatableFields` (jutsu, feats, class features, clans, equipment, backgrounds,
subclasses, summon tribes, puppet stat blocks, titan cards, and so on). "Translation complete"
means both halves are fully done — a report that says "the UI is done" or "the game content is
done" while the other half is still partial is not a complete answer to "is it done." There is no
tier of content that's exempt for being small, rarely viewed, or already partially covered (e.g. a
single credit line, a single stat-block label) — if a player can see it, it gets translated, down
to the smallest bit of text.

The only content that stays untranslated is what's listed under "What stays untranslated" below
(proper nouns, language-neutral symbols, source code) and machine-parsed data fields (see
"Distinguish player-facing text from machine-parsed data") — never a table or field skipped merely
because it wasn't gotten to yet.

## Source priority for terminology

When a game term needs a translation, check sources in this order and stop at the first one that
actually applies — don't guess before checking, and don't over-rely on a lower-priority source
when a higher one is available:

1. **Established tabletop-system precedent.** This game is a D&D 5e hack, so mechanical terms
   (saving throw, advantage, bonus action, proficiency bonus, concentration, etc.) should match
   whatever the target language's D&D 5e community/official material already uses — these players
   likely already know these terms from D&D itself. Check the official SRD translation if one
   exists (e.g. Spanish: srd.nosolorol.com hosts the official SRD 5.1 Spanish translation) and
   established community translations of the actual ruleset (e.g. FoundryVTT's `dnd5e-es` system
   translation, a rigorous structured reference). Prefer these over inventing a new rendering of a
   term that already has a settled one — consistency with the wider hobby matters more than any
   single word choice being marginally "more correct."
2. **The source anime/setting's own established terminology**, for anything that's actually a
   named character, role, or concept from the show itself (not invented by this game). Check how
   the official dub or a well-established fan wiki (e.g. Narutopedia) renders it. Example: "Puppet
   Master" → "Titiritero" came from checking how the actual Latin American Spanish dub of Naruto
   Shippuden refers to Sasori and Kankuro, the named characters this class is built around — a far
   stronger source than composing a literal translation ("Maestro Titiritero," rejected) from the
   English words alone.
   - **The dub can be inconsistent with itself** — e.g. "Wood Release" was rendered two different
     ways across two parts of the same show. When that happens, prefer whichever rendering is used
     *consistently* across a fan-wiki reference the target audience is likely to already consult
     (e.g. Narutopedia's "Elemento X" pattern for every nature-transformation kekkei genkai), over
     picking one of the dub's inconsistent variants at random. Document why the dub was rejected as
     a source when this happens, so it isn't re-litigated later.
3. **No precedent exists (game-original content).** Many classes, systems, and mechanics in this
   game are invented by the devs/community, not drawn from D&D or the anime — "Mastery" (the
   stacking bonus system), and several class names (Science-Nin, Cooking-Nin, Intelligence
   Operative, Weapon Specialist, Scout-Nin) have no source to check against anywhere. For these,
   a literal, unambiguous translation is fine and expected — there is nothing to verify it
   against, so don't spend time searching for precedent that doesn't exist. Get a native-speaker
   sanity check that the literal rendering reads clearly and isn't confusable with something else,
   then move on.

## Verify sources for real — don't rely on unverified recall

Don't state a translation as "the standard term" from memory alone; check it against a real,
citable source (official SRD text, a community translation's actual data file, a wiki page quoting
real dub dialogue) before presenting it as verified. If a term is presented without a check and
turns out to be wrong (this happened once with "Maestro Titiritero" and once with "Bono de
Competencia" vs. the official "Bonificador por Competencia"), that's a sign to slow down and
actually verify similar terms, not just the one that got caught.

## What stays untranslated

- **Proper nouns from the setting**: all clan names (Aburame, Uchiha, Hyūga, etc.), rank titles
  that are already Japanese loanwords in the target language's fandom (Genin, Chunin, Jonin,
  Sanin, Kage), and any other in-setting proper noun. Only descriptive/flavor text *about* these
  (a clan's epithet, overview, etc.) gets translated — never the name itself.
- **Language-neutral symbols**: jutsu rank letters (E/D/C/B/A/S) are already language-neutral,
  single characters — don't translate or reinterpret them.
- **Source code.** Go identifiers, function/variable names, code comments, and any other
  developer-facing text stay English permanently, full stop — this is standard, near-universal
  practice across the software industry regardless of the development team's own first language
  (see: every mainstream programming language's syntax is English; the broader ecosystem of docs,
  libraries, and prior art a maintainer will need to search is English). This is categorically
  different from player-facing UI/content text and is never in scope for a translation effort,
  even when a technical detail (like a machine-parsed data field, below) is discovered while
  translating.

## Distinguish player-facing text from machine-parsed data

Not every string stored in a content table is prose meant to be read — some fields are structured
data the application code parses at runtime to drive behavior (filters, calculations, lookups).
Translating a field like that at the data level breaks the feature that parses it, for every row
that uses it. Concrete example found while translating jutsu: `jutsu.components` (e.g.
`"HS, CM, W"`) looks like short prose but is actually parsed by
`cmd/n5e/jutsu_filters.go`'s `componentCodes`/`componentNames` to power the jutsu filter chips —
translating the raw field per-jutsu would silently break filtering across the entire jutsu list.

**Before adding a field to a table's translatable-fields list, check whether any application code
actually parses that column's value** (not just displays it verbatim) — grep for the column name
across `cmd/` and `internal/`. If it's parsed, don't translate the raw data field; instead
translate only the small, fixed set of *display labels* the parsing code maps codes/values to
(e.g. the 6-entry `componentNames` map), through the ordinary UI string catalog mechanism —
exactly like any other button or menu label. This keeps the underlying data machine-readable in
every locale while still translating everything the player actually sees.

**This has now recurred at least twice** (`jutsu.components` → the 6 component-code labels;
`equipment.properties`/`equipment.damage_type` → the property/damage-type labels) and is a
**settled, standing policy, not a per-field judgment call to re-raise**: whenever this
machine-parsed-field pattern is found, the small set of labels it maps to is *always* in scope for
translation — because the standing project rule is that anything the player can potentially see
gets translated, full stop. Apply this automatically the next time the pattern turns up; don't ask
for confirmation again.

## Review workflow — DeepL API run, then a manual spot-check pass

**Superseded 2026-09-05.** This project's translation work used to run through three gated,
conversational stages before any bulk translation happened (glossary review with a native
speaker, a small pilot batch reviewed side-by-side, only then a full bulk run). That process is
retired for both the UI catalog and game content alike, in favor of a DeepL-API-driven pipeline:

1. **Glossary stays, but it's a file, not a conversation.** `glossary.md`'s term tables (built
   from this file's source-priority order and verification discipline, still followed exactly as
   described below) are parsed directly into a DeepL glossary (`internal/i18n.ParseGlossaryTables`)
   and passed to every translate call — no separate live walkthrough gate before translation can
   start. Add or correct a term in `glossary.md` the same way as before; it takes effect on the
   next run.
2. **No pilot batch, no dry run.** Every table/field (game content) or every catalog key (UI)
   translates in one straight pass via the DeepL API — `%s`/`%d` placeholders protected
   (`internal/i18n.ProtectPlaceholders`/`RestorePlaceholders`), glossary-constrained, batched
   through `internal/i18n.DeepLClient`. There is no intermediate approval gate a category has to
   clear before running at volume.
3. **Manual spot-check happens after the run, not before.** DeepL is fast and mechanically
   complete, but not perfectly reliable — it has been observed to drop pinned terms in short
   strings and to translate the same source term two different ways in different catalog entries,
   even with a matching glossary identity/pinned entry already in place (see `glossary.md`'s "Known
   limitation" note). After every run (UI or content, first pass or re-run), spot-check: skim a
   sample of the freshly translated text in a live browser or side-by-side, watching especially for
   (a) a glossary-pinned term being dropped or paraphrased away, (b) the same English term rendered
   inconsistently across two different strings that should match (e.g. a nav label vs. a page
   title), and (c) empty or malformed translations. Fix anything found directly in the locale
   JSON / `content_translations` row, the same way earlier fixes in this file's history were
   applied — don't wait for a report from an actual player to catch it.
4. **Re-running is cheap and expected.** Both the catalog and content pipelines only translate
   keys/fields that are missing or stale (English changed since the last translation) by default —
   adding a new UI string or new game content re-runs cleanly without re-translating everything
   that's already good. A field/key can be forced to retranslate (delete it from the target locale
   file, or `-force` on `catalog-translate`; re-export with `-force` for content) when a spot-check
   finds it needs a redo.

This trades the old process's up-front human-conversation cost for a faster mechanical pass
followed by a lighter, but still real, verification pass — full mechanical coverage on every run,
with human judgment applied to catching and fixing what DeepL gets wrong rather than gating what it's
allowed to attempt.

## Translate from corrected English, not raw broken English

The source content in `rules.db` has real grammatical errors independent of
translation — dangling clauses, comma splices, subject-verb disagreement, and
similar (found first in the Snake summon tribe's description, then confirmed
at scale by a dedicated audit: see `internal/i18n/source_text_grammar_audit.md`,
147 occurrences of one systemic pattern alone). This is a source-content
quality issue, not a translation-methodology one, and fixing the English text
in `dist/rules.db` itself is a separate, human-scheduled task — not something
a translation pass does on its own.

**Before translating any row, check `source_text_grammar_audit.md` for a
known finding on that exact row.** If one exists, translate from the
corrected English the audit suggests, not the raw broken text, and note in
`glossary.md` (at the point of use) which correction was used and that it
traces back to that audit — the same way the Snake pilot translation does.
If a row's English turns out to be broken but isn't yet in the audit report,
add it there rather than silently working around it in the translation alone,
so the finding is tracked in one place instead of being rediscovered later.

## Applying this to a new language

For a language with no existing glossary yet:

1. Repeat the glossary stage from scratch for that language — do not assume terms transfer
   directly from another already-translated language, even when they look like they should
   (e.g. Spanish and Portuguese share a lot of vocabulary, but D&D/anime community terminology in
   each language was independently established by separate translator communities and can diverge
   in surprising, specific ways).
2. Reuse this file's *process* (source priority order, verification discipline, building
   `glossary.md` before running DeepL, the post-run manual spot-check, the machine-parsed-field
   check) unchanged — only the actual term choices are language-specific, not the methodology for
   arriving at them. Add the new locale to `deeplTargetLang`/`glossaryColumn` in
   `cmd/n5e-i18n/main.go` and give `glossary.md`'s term tables a matching language column before
   running any translate command for it.
3. Flag anywhere the new language's own source ecosystem differs in kind — e.g. if no official SRD
   translation exists for that language, or the anime dub for that language is known to be lower
   quality/less consistent than the Spanish dub was — since that changes which source-priority tier
   actually has usable material to check against.
