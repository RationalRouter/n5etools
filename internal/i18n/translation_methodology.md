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

## Known DeepL/pipeline bugs (fixed in code, but explain scars if seen again)

1. **DeepL's glossary-creation endpoint rejects regional `target_lang` variants** (e.g. `PT-BR`)
   even though the translate endpoint requires one. `cmd/n5e-i18n/main.go`'s
   `glossaryTargetLang()` strips the region suffix only for the `CreateGlossary` call.
2. **`cmdCatalogTranslate`'s `existing` map stayed `nil`** (not empty-initialized) when the
   locale's catalog file didn't exist yet — panicked on first write for a genuinely new locale.
   Fixed with a nil-check + initialize.

## Known DeepL translation-quality failure modes — check for these on every run

- **Non-deterministic mistranslation of ambiguous English homonyms.** "class(es)" (RPG
  character-class sense) came back as Portuguese "aula(s)"/"turma(s)" (school-class sense) in
  ~17 keys, inconsistently even across two runs of identical source text. Spot-check
  homonym-prone terms (class, level, rank, school, ...) after every translate/retranslate run.
- **A raw backtick used as a quotation mark** in place of a proper target-language quotation
  mark — a Portuguese-specific artifact found in `creation_hub.title`, `backups.snapshot_notice`,
  `sheet.unpack_named`. Grep the locale file for `` ` `` after every run.
- **DeepL leaves short, stylized compound titles untranslated** (jutsu/move names like "Shatter",
  "Flash", "Grand Slam"). Cross-locale agreement (both Spanish and Portuguese left the same name
  untranslated) is NOT evidence it's intentional — it's evidence of a shared pipeline limitation.
  Translate it unless the term is genuinely Japanese/Naruto-verse in origin, a true cognate
  spelled identically in the target language, or a coined/invented word with no dictionary
  meaning anywhere (flag that last category explicitly instead of silently leaving it — see
  [[feedback_translate_everything_except_japanese_naruto_terms]] for the full standing rule and
  the incident that established it).

## Methodology that worked, worth reusing directly

- **Diff exported vs. translated `.jsonl` per table** to find no-op translations (English text
  unchanged) across an entire table without eyeballing every page. Filter to length > 3 and
  containing a letter to cut noise.
- **Compare against the OTHER already-translated locale's rendering of the same field, in BOTH
  directions**, when starting a new locale — a name can be untranslated in the new locale while
  the older one already got it right (a real gap, worth adapting), or the reverse. Never copy an
  older locale's translation blindly, though: verify it first (Spanish's own "Encuadernación en
  rústica" for "Web Bind" was a false-cognate error that would have shipped twice if copied
  as-is to Portuguese).
- **Querying `content_translations` directly via sqlite3** (`SELECT`/`UPDATE` against
  `out/rules.db` and `internal/embedded/rules.db`) is a fast, no-API-key way to hand-patch a
  handful of confirmed field translations without a full pipeline re-run. Re-run the real
  pipeline instead once confirmed fixes number in the hundreds.

## Sitewide code-level gaps found — NOT DeepL's fault, structural blind spots in the app itself

The DeepL pipeline only ever sees two things: the UI catalog (`internal/i18n/locales/*.json`)
and database content registered in `internal/i18n.TranslatableFields`. Any player-facing string
that lives somewhere else in the Go source is invisible to it — a 100%-complete pipeline report
says nothing about this category of gap. Found so far:

- **Page `<title>` values were hardcoded English literals** across ~30 Go handler call sites
  (`"Title": "Fighting Stances"`), never routed through the catalog even though the page's own
  body content was correctly wired via `i18n.Row`/`Batch`. Fixed 2026-09-07 via a
  `func (s *server) t(r *http.Request, key string, args ...any) string` helper in
  `cmd/n5e/server.go`. Applies to any new page/handler added later too — check `"Title":` (and
  any other literal handed straight into `s.render`'s data map) goes through the catalog.
- **Class-mechanics Go files build hundreds of feature/resource display names as hardcoded
  English string literals** — `cmd/n5e/custom_resources.go` and sibling class files
  (`hunter_nin.go`, `genjutsu.go`, `cooking_nin.go`, and others): `grantedFeatureRow`/custom
  resource `Name`/`SourceLabel`/`Label`/`Restriction` fields. These never went through the catalog
  in ANY locale, including "fully translated" Spanish, because they don't live in the rules
  database at all — there was never a `.jsonl` export for the pipeline to see them in the first
  place. **`custom_resources.go` fixed 2026-09-07** (243 `Name` values, 164 `Restriction` values):
  see `translateCustomResourceEntries` in that file for the pattern used, reusable for the
  remaining sibling files —
  - Since these ~400 curated strings have no natural short key to hand-name one at a time, the
    English text itself doubles as the catalog key, namespaced per field
    (`"cr_name:" + rawName`, `"cr_restriction:" + rawRestriction`) so it can't collide with an
    unrelated catalog key elsewhere. `locales/en.json` carries an identity entry for every one of
    these keys (value == the raw English text) so a missing translation falls back to the
    original text instead of leaking a raw `cr_name:...` key onto the sheet.
  - Translation happens in exactly one place per file — at the point the display struct
    (`CustomResourceEntry` here) is actually constructed — not inside the big curated data map
    itself. That kept every existing `computeCustomResources` test call site (50+) untouched: the
    map and the pure compute function still produce plain English, and only the one real
    request-facing entry point (`loadCustomResources`) takes a `locale` parameter and overlays the
    translation as a final post-process step before returning.
  - `locale` only needs to reach the few functions whose OUTPUT is actually read for display
    (`Name`/`Restriction`, or anything copied from them, like `jutsuFreeCastGrant.MirageName` in
    `characters.go`). A caller that only reads `Max`/`Current`/regen kind from the same function
    (`applyCustomResourceRest`, pure rest-math) can safely pass `"en"` and discard the unused
    translated text — no benefit to threading the real request locale through a call site that
    never displays the result.
  - The 164 `Restriction` strings were translated by hand for this file (DeepL wasn't reachable
    from the session that did this work — no `DEEPL_API_KEY` in that tool environment) — run them
    through `cmd/n5e-i18n catalog-translate` (or a manual spot-check against the glossary) next
    time DeepL access is available, the same "verify by hand once, then trust the pipeline for the
    rest" precedent as everything else in this doc.
  - **`hunter_nin.go` fixed 2026-09-07** (72 `Name` values, 63 `Description` values, plus a
    `SourceLabel` and 2 more names hidden in a `hunterNinPatternPassiveRows` lookup map outside the
    file's main catalogs): this file's shape differs from `custom_resources.go` — instead of one
    big map, the gap was 11 separate hand-curated `[]hunterPickOption{...}` package-level catalogs
    (`lethalPrecisionOptions`, `defensiveTacticsOptions`, `wardenWeaponPropertyOptions`,
    `medicalAssassinationTechniques`, `shadowAssassinationTechniques`, `toxicAssassinationTechniques`,
    `viceAssassinationTechniques`, `voidAssassinationTechniques`, `prostheticAttachmentOptions`,
    `wolfTechniqueOptions`, `arsenalItems`), each read from two different call sites
    (`splitHunterPicks` in `loadHunterTechniquesTabData`, and `lookupHunterPickOption` in
    `handleHunterPickDetail`). Two sibling loader functions in the same file
    (`loadHunterOptionCatalog`, `loadWardenWeaponCatalog`) were already correctly translated via
    `i18n.Batch`/`i18n.Row` because they're DB-backed (`class_options`/`equipment` tables) — those,
    and their own `splitHunterPicks`/`lookupHunterPickOption` call sites, were left untouched.
    New helpers `translateHunterPickOption`/`translateHunterPickOptions` wrap a
    `hunterPickOption`'s `Name`/`Description` at the read site, keyed `"hn_pick_name:" + rawName`/
    `"hn_pick_desc:" + rawDescription` — same raw-text-as-key convention as `cr_name:`/
    `cr_restriction:`. The outlying `hunterNinPatternPassiveRows` map (2 names + a `SourceLabel`,
    used for a Core-tab Sources tooltip, not a `hunterPickOption`) got its own `locale` parameter
    and reuses `hn_pick_name:` for its two names (they're Hunters Patterns' own `class_options`
    display names, so the key just needs to match what a player would see, not a strict "which
    struct produced this" rule) plus a new `hn_source_label:` namespace for the one `SourceLabel`
    literal. All Spanish/Portuguese translation done by hand (still no `DEEPL_API_KEY` in this
    session's tool environment either).

  - **`genjutsu.go` fixed 2026-09-07** (4 `Name`/`Description` pairs in `masterOfIllusionOptions`,
    plus 4 `KeywordDCBonusLabel` literals) — this file turned out to hide a WORSE bug than the
    "hardcoded Go literal" category the other entries in this section describe: two of its
    loader functions, `loadGenjutsuOptionCatalog` (backs Malleable Mirages and Genjutsu Inception)
    and `loadGenjutsuConversionCatalog` (backs Real World Conversion), read straight from
    `class_options`/`class_features` with **no `i18n.Batch` call at all** — unlike every sibling
    loader in `hunter_nin.go`, these two never translated their DB-backed content in ANY locale.
    Same root cause for `handleGenjutsuPickDetail`: 3 of its 4 `switch category` branches
    (`mirage`, `inception`, `conversion`) queried `rulesDB` directly with no `i18n.Row` overlay;
    only `illusion-mastery` (the one hand-curated `masterOfIllusionOptions` catalog) needed the
    usual raw-text-as-key treatment. This is NOT a "string never existed in the pipeline" gap like
    `custom_resources.go`/`hunter_nin.go` — the translated rows already exist in `class_options`/
    `class_features`, they just were never read. **Any future file audit must check not only for
    hardcoded literals but for a DB-backed loader silently missing its `i18n.Batch`/`i18n.Row`
    call** — grepping for hardcoded strings alone would never have caught this one, since the Go
    source contains no English literals for Mirages/Inception/Conversion at all. Found by
    inspecting `loadGenjutsuOptionCatalog`'s signature (no `locale` parameter, unlike
    `loadHunterOptionCatalog`) while porting the sibling-file fix pattern across — a useful signal
    to check for on every remaining file: does every rules-database-backed loader function take
    a `locale` parameter and end with an `i18n.Batch`/`i18n.Row` call, not just the hand-curated
    ones? Also found 3 more hardcoded display names living outside `masterOfIllusionOptions`
    entirely (`genjutsuActualizedAlterationPickedRows`'s "Actualized Alteration",
    `genjutsuMiragePickedRows`'s "Deceitful Duplicate" + its `genjutsuMirageRestLimitedSlugs` map
    of ~32 Malleable Mirage names, `genjutsuMirageDemonSightPassiveRow`'s "Demon Sight") — all of
    them duplicate real `class_options` Mirage names already present in the catalog under
    `custom_resources.go`'s own `cr_name:` keys (since `customResourceGrants` already tracks these
    same Mirages' rest-pools under those names), so these were translated by REUSING `cr_name:`
    rather than inventing a new namespace, except "Demon Sight" which had no prior `cr_name:`
    entry and got one added. All Spanish/Portuguese translation done by hand (still no
    `DEEPL_API_KEY` in this session's tool environment).

  - **`cooking_nin.go` fixed 2026-09-07** — a cleaner case than `hunter_nin.go`/`genjutsu.go`:
    every one of this file's DB-backed loaders (`loadCookingToolImplementCatalog`,
    `loadCookingToolPipeImplementCatalog`, `loadExpertCombatantWeaponCatalog`,
    `loadCookingNinTabData` and everything under it) already took `locale` and correctly called
    `i18n.Batch` — no missing-loader-overlay bug here. The whole gap was 9 hand-curated
    `[]featureChoiceOption{...}` package-level catalogs (`fastAndFuriousOptions`,
    `gastrochemistEnhancementOptions`, `cookingToolDamageTypeOptions`,
    `cookingToolPropertyL1Options`/`L6Options`/`L11Options`,
    `cookingToolPipePropertyL3Options`/`L6Options`/`L11Options` — 31 entries, 62 Label+Description
    strings total), plus two value-to-label resolver functions
    (`gastrochemistEnhancementLabel`, `cookingToolOptionLabel`) that read those same raw catalogs
    back for a Known-list display label. Unlike `hunterPickOption`/`genjutsuPickOption` (each
    scoped to one file), `featureChoiceOption` (`cmd/n5e/feature_choices.go`) is a struct shared
    across ~10 sibling files still open in this list — so instead of a per-file wrapper, one
    shared `translateFeatureChoiceOptions(options []featureChoiceOption, locale string)
    []featureChoiceOption` helper was added directly to `feature_choices.go`, keyed under a
    shared `"fco_label:" + rawLabel`/`"fco_desc:" + rawDescription` namespace (not a per-file
    prefix like `hn_`/`gj_`, since the struct itself is shared — a hand-curated Label with
    identical English text in two different files' catalogs means identical displayed text, so
    sharing the key is correct, not a collision risk). Both resolver functions were changed to
    take `locale` and translate the catalog before scanning it, rather than duplicating the
    lookup logic. Applied at 9 read sites across `loadCookingToolInfusionView`,
    `loadCookingToolInfusionPipeView`, and `loadCookingNinTabData` — every popup handler
    (`cooking_nin_*_popup.go`) already threaded `localeFromContext(r.Context())` into
    `loadCookingNinTabData` correctly, so no popup-level changes were needed. All
    Spanish/Portuguese translation done by hand (still no `DEEPL_API_KEY` in this session's tool
    environment). **Any sibling file reached later that also uses `featureChoiceOption` for a
    hand-curated catalog should reuse `translateFeatureChoiceOptions` directly rather than writing
    a new per-file wrapper** — check `feature_choices.go` first before assuming a new helper is
    needed. **Caught and fixed in the same pass**: `cookingToolDamageTypeOptions`' 3 Bludgeoning/
    Piercing/Slashing Labels were initially routed through the generic `fco_label:` namespace like
    every other catalog here, duplicating `items.go`'s own pre-existing `damageTypeCatalogKey`/
    `translateEnumDisplay` mechanism (the canonical `damage_type.bludgeoning`/`.piercing`/
    `.slashing` catalog keys every other weapon's damage-type display already uses) under a second,
    independent key. Fixed via a dedicated `cookingToolDamageTypeOptionsFor(locale)` that calls
    `translateEnumDisplay(tr, damageTypeCatalogKey, ...)` for Label (reusing the canonical key) and
    `fco_desc:` only for Description (no existing catalog entry for the Cooking-Tool-specific
    sentence). **Standing check going forward**: before adding ANY new catalog key for a Label that
    is a plain damage-type/weapon-category/armor-category word (Bludgeoning, Piercing, Slashing,
    Fire, Cold, Lightning, Variable, Simple/Martial/Exotic, Light/Medium/Heavy armor), grep
    `items.go`'s `weaponCategoryCatalogKey`/`armorCategoryCatalogKey`/`damageTypeCatalogKey` maps
    first and reuse the canonical key via `translateEnumDisplay` — the same "reuse, don't duplicate"
    discipline `genjutsu.go`'s `cr_name:` reuse already established for resource names, just for
    this different, UI-catalog-native family of enum labels instead. (Separately confirmed: `loadKnownBlendJutsu` reads known-jutsu `Name`/`Rank`/
    `Description` straight from `v_jutsu`/`custom_jutsu` with no translation overlay at all — but
    this mirrors `ninjutsu_specialist.go`'s own `loadKnownNinjutsu`, a pre-existing, wider
    "known-jutsu-picker" pattern that predates this file and isn't part of the current 19-file
    hardcoded-literal list; flagged here for the eventual en/pt audit rather than fixed inline,
    since fixing it means touching `ninjutsu_specialist.go` too, outside this pass's scope.)

  - **`wow_weapons.go` fixed 2026-09-07** — a genuinely different shape of gap from every other
    entry in this section, and the first one where the "obvious" fix (translate the hardcoded
    `Name`/`Properties` strings the same way as everywhere else) would have been actively wrong,
    not just incomplete. `wowWeaponSpecs`' 10 curated entries (`Name`, `DamageDice`, `DamageType`,
    `Properties`) get baked into a real, per-character `custom_items` row once, at the moment a
    W.o.W is picked (`grantWoWWeapon`) — unlike every catalog fixed so far, which is re-read and
    re-translated on every request, this text is written once and never re-synced. Two of the four
    fields turned out to be load-bearing beyond display:
    - **`Name` is buildAttacks' only matching key** back to `wowWeaponSpecsByName` (for
      `AbilityOverride`/`NoDamageAbilityMod`) and to `wowAscendedOverride`'s own designated-W.o.W
      comparison, and it's also what `revokeWoWWeapon`'s `SELECT ... WHERE ci.name = ?` matches on
      to find the granted row again when the pick is forgotten. Translating it would silently break
      all three the moment a character's locale differs from whatever locale was active at grant
      time (there is no re-sync mechanism) — so it stays English forever, by design, not oversight.
    - **`Properties` is a machine-parsed keyword field**, not prose — `characters.go`'s own
      `buildAttacks` lowercases it and greps for `"finesse"`/`"thrown"`/`"ammunition"`/`"range"` to
      pick the attack's ability score. This is the exact same category `internal/i18n/fields.go`
      already excludes `equipment.properties`/`equipment.damage_type` for — confirmed here for a
      second, independent field (a Go-literal one, not a DB column) rather than assumed.
    - **`DamageType`, by contrast, IS safe** — read back only for display, never matched against
      anything — so it's the one field actually translated, via `translateEnumDisplay`+
      `damageTypeCatalogKey` (`items.go`, the same canonical mechanism `cookingToolDamageTypeOptionsFor`
      above reuses), extended with 3 new keys (`damage_type.elemental`/`.force`/`.wind`) for the
      3 invented damage types (confirmed via `SELECT DISTINCT damage_type FROM equipment` that no
      real rules.db weapon ever uses these — they're bespoke to these 10 W.o.W).
    - Separately, `scienceNinOptionName`/`scienceNinOptionNameAndDescription` (this file's own
      `class_options` lookups, used to resolve a W.o.W pick's printed name/description) were reading
      `class_options` directly with **no `i18n.Row` overlay at all**, despite `class_options` already
      being a registered `TranslatableFields` table — a smaller instance of the `genjutsu.go`
      missing-loader-overlay bug. Fixed for `description` only (translated via `i18n.Row`, since it
      only ever lands in a `custom_items.Description` free-text field, never matched); `name` stays
      untranslated for the identical matching-key reason `Name` does above, since `removeWoWPick`
      matches `revokeWoWWeapon` against this exact same raw value.
    - **Lesson for the remaining files**: before translating ANY hand-curated field that ends up
      persisted onto a `custom_items`/`character_inventory`/similar row (rather than recomputed
      fresh from `rules.db` on every request), check whether ANYTHING — in the same file or a
      different one — matches equipped items back against that field's value, or parses it for
      keywords, before assuming it's pure display text. Grep the field's own values (or the
      variable/column name) across `cmd/n5e` for a `==`/`strings.Contains`/map-key use, not just
      for where it's read into a template.

- **`nindog.go`** (Inuzuka Beast Master Nin-Dog stat block): `ninDogBreeds`' hand-transcribed
  Name/Blurb/Traits (3 breeds, 5 traits each — no natural DB row, same shape as
  `custom_resources.go`), `ninDogFeaturePickSlots`' 6 hardcoded pick-slot `Label` strings, and
  `ninDogBiteAttack`'s built-in Name/Description/DamageType were all untranslated. Fixed via
  dotted fixed-key namespacing (`nindog.breed.<slug>.name`/`.blurb`/`.trait_N`, `nindog.slot.*`,
  `nindog.bite.*`) — matching `titan.go`'s existing dotted-key convention for its own hand-curated
  content, rather than the raw-English-as-key convention `custom_resources.go`/`cooking_nin.go`
  use, since `titan.go` already established that alternative for this same "no natural DB row"
  shape and either convention works as long as it's applied consistently within a file. `Slug` (the
  only field `ninDogBreedBySlug`/`companion.NinDogBreed` match against) is untouched — only
  Name/Blurb/Traits, which are pure display, are translated. DamageType routes through the existing
  `damageTypeCatalogKey`/`translateEnumDisplay` mechanism (`"Piercing"`), not a new key, since the
  Bite attack is computed fresh every render (never persisted) and Piercing is one of the 7 real
  rules.db damage types already covered there.

- **`science_nin_subclasses.go`** (Science-Nin's 8-subclass cap+catalog picks, ~2650 lines): two
  gaps, both narrow — most of the file already correctly threads `locale` through
  `loadScienceNinEntryCatalog`/`loadScienceNinChildlessTierOptions`. (1) `loadScienceNinFlatCatalog`
  (W.O.W's 9 named Weapons of Wonder — the DISPLAY catalog for the Elemental Innovationist picker,
  a different data path from `wow_weapons.go`'s own hardcoded `wowWeaponSpecs` map and
  `scienceNinOptionNameAndDescription`) had NO `i18n.Batch` call at all — a DB-backed loader
  silently missing the overlay, same category as `genjutsu.go`'s and `wow_weapons.go`'s prior
  findings. Fixed by adding the same `class_options` batch-overlay pattern
  `loadScienceNinChildlessTierOptions` already uses. (2) `handleScienceNinSubclassPickDetail`'s own
  three raw `QueryRow` calls (the click-to-open popup shared by all thirteen non-hand-curated
  categories) also had no overlay — fixed with `i18n.Row` on each of the three query shapes
  (`class_options` flat, `class_option_entries`, and the childless-tier `class_options` fallback).
  For the childless-tier fallback specifically: `splitScienceNinSoloTierItem` must run BEFORE
  translating, not after — it extracts the item's ALL-CAPS name by pattern-matching the RAW
  English description (`findEntries`), and a translated body won't match that pattern in Spanish/
  Portuguese; got this backwards on the first pass here and caught it by comparing against
  `loadScienceNinChildlessTierOptions`'s own (correct) ordering before testing, not after. Also
  translated `scienceNinRegaliaOptions` (King's Road's own 8 hand-curated Regalia types, no DB row)
  via a `translateScienceNinRegaliaOptions` helper, same dotted-key shape as `nindog.go`'s breeds —
  `Slug` stays untouched since `charstore.ScienceNinPickRegalia` and the detail-popup's own
  "regalia" branch both match against it. `scienceNinAngelEIPCompanionName` ("Spectre (Angel
  E.I.P)") was checked and correctly left alone — it's a real matching key
  (`charstore.ListCompanions` exact-name comparison in both `ensureAngelEIPSpectreCompanion` and
  `removeAngelEIPSpectreCompanion`), the same "don't translate a matching key" restraint
  `wow_weapons.go` established.

**Standing check for any future "is this language done?" audit**: grep the whole `cmd/n5e`
package for `(Name|SourceLabel|Label|Title|Error|Restriction):\s*"[A-Z]` to find this category of
gap by SOURCE, not by database/catalog completeness percentage — completeness reports only cover
what was in scope to begin with, and this whole class of string structurally wasn't. As of
2026-09-07, `custom_resources.go`, `hunter_nin.go`, `genjutsu.go`, `cooking_nin.go`,
`wow_weapons.go`, `nindog.go`, and `science_nin_subclasses.go` are closed; ~13 sibling files
(`scout_nin.go`, `puppets.go`, `medical_nin.go`, `hunter_pattern_choices.go`, `wow_whelp.go`,
`taijutsu.go`, `weapon_specialist.go`, `titan.go`, `elemental_affinity.go`, `snb.go`,
`science_nin.go`, `puppet_tactics.go`, `full_metal_shinobi.go`) still have this gap open — check
each one for its own hidden `Restriction`/`Description`-shaped field too (the grep above catches
`Restriction` now, but a new field name in a new file won't match until it's added here); check for
outlying lookup maps outside the file's main catalog(s) the way `hunterNinPatternPassiveRows` was
hiding one; AND — per the `genjutsu.go` finding above — check every DB-backed loader function in
the file (`SELECT ... FROM class_options`/`class_features`/`equipment`/etc.) actually takes a
`locale` parameter and calls `i18n.Batch`/`i18n.Row` before returning, since a loader silently
missing that call produces zero hardcoded-English-literal grep hits while still leaking
untranslated rules-DB content onto the sheet in every non-English locale. Also check whether the
file's hand-curated catalogs already use `featureChoiceOption` (`cmd/n5e/feature_choices.go`) — if
so, reuse `translateFeatureChoiceOptions` (added for `cooking_nin.go`) rather than writing a new
per-file wrapper. AND — per the `wow_weapons.go` finding above — before translating any hardcoded
field that gets persisted onto a per-character row rather than recomputed fresh each request, check
whether it doubles as a matching key or a machine-parsed keyword field elsewhere in the codebase
before assuming it's safe to translate.

Also note `scout_nin.go`'s own confirmed-but-not-yet-fixed instance of the missing-loader-overlay
bug: `loadScoutNinManeuverCatalog(subclassSlug string)` (its single-subclass variant) queries
`class_options` with no `i18n.Batch` call at all, unlike its sibling
`loadScoutNinManeuverCatalogForSubclasses(locale string, ...)` which is correctly wired — fix this
alongside `scout_nin.go`'s own hardcoded-literal pass when that file is reached.

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
