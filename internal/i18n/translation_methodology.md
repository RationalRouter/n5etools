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

- **`scout_nin.go`** (Scout-Nin's superiority-die maneuver/pick system, ~1900 lines): three fixes.
  (1) `loadScoutNinManeuverCatalog` (one subclass's own Maneuvers `class_options` rows) had NO
  `i18n.Batch` overlay at all — same missing-loader-overlay category as `genjutsu.go`/
  `science_nin_subclasses.go`'s prior findings — but fixing it was NOT a simple "add the call and
  return" fix: `loadScoutNinTabData`, its one call site, reuses the returned catalog's raw
  `Description` text AFTER the loader returns, for `filterScoutNinManeuversByKeyword(catalog,
  "Keyword: Tactical")` (the Signature Maneuver/Tactical Scout branch) and a raw slug-membership
  scan (Supreme Clones/Cloning Scout). Translating inside the loader itself — the first pass here —
  would have broken both in every non-English locale, an ordering bug of the identical shape just
  caught in `science_nin_subclasses.go`'s `handleScienceNinSubclassPickDetail` earlier the same
  session (see above): translated prose can't be relied on to still contain the English "Keyword: X"
  marker. This is now the **second time** this exact bug shape recurred in one session, worth
  calling out as a standing risk, not a one-off — see the recurring-bug-shape paragraph below. Fixed
  by reverting the loader to return the catalog RAW (documented in its own doc comment) and instead
  translating each of the 6 display-bound output lists (`KnownManeuvers`/`AvailableManeuvers`,
  `KnownSignatureManeuver`/`AvailableSignatureManeuver`, `KnownSupremeClones`/
  `AvailableSupremeClones`) individually via `translateScoutNinPickOptions`/a new
  `translateKnownScoutNinPicks` twin (for the `knownScoutNinPick`-shaped Known half
  `splitScoutNinPicks` returns), applied immediately after each split — safe because
  `splitScoutNinPicks` copies `scoutNinPickOption` by value, so translating a split-off copy never
  mutates the shared raw `catalog` slice still needed for keyword matching further down the same
  function. (2) `class_options.list_name` (e.g. "Arbiter Maneuvers", the Maneuvers section's own
  heading) is itself in `internal/i18n/fields.go`'s `class_options` TranslatableFields entry but was
  returned completely raw — added `translateScoutNinManeuverListName`, which resolves it via a
  single `i18n.Row` keyed on the catalog's own first slug (`list_name` is identical across every row
  of one subclass's catalog, so any known slug resolves it; returns the raw name unchanged if the
  catalog is empty). (3) Two hand-curated `featureChoiceOption` catalogs — Change of Heart's
  Messiah/Izanagi/Satanael (Trickster Scout, 17th level) and Paragon's Presence's
  Berserk/Charmed/Frightened (Arbiter Scout, 14th level) — had no translation at all; both already
  match only on `Value` (never Label/Description) in `setChangeOfHeart`/`setParagonsPresence`, so
  wired through the existing `translateFeatureChoiceOptions` (`feature_choices.go`) at their two
  `changeOfHeartView`/`paragonsPresenceView` construction sites rather than writing a new per-file
  wrapper. Separately, `scoutNinSubclassSlug`'s own raw `subclasses.name` (used for
  `data.TechniqueName`, the Technique tab's own heading) had no `i18n.Row` overlay despite
  `subclasses` already being a registered `TranslatableFields` table — fixed at the one call site in
  `loadScoutNinTabData`, which already has `locale` in scope. **Found but NOT fixed, flagged as a
  cross-cutting follow-up outside this file's own scope**: `scoutNinMobileSavantGrantedJutsu`'s
  `labels[slug] = "Mobile Savant"` and `scoutNinElementalResistanceEntry`'s `Sources:
  []string{"Elemental Resistance"}` are both hardcoded English display badges, but they're instances
  of a much wider pre-existing pattern living in shared infra files that aren't among the 19
  siblings (`jutsu_grants.go`'s `loadGrantedJutsuLabels` hardcodes "Class Feature"/"Subclass
  Feature"/"Clan"/"Feat" the same way; `passive_traits.go`'s `PassiveTraitEntry.Sources`/
  `AdvantageEntry.Sources` carry the same shape across every class file that produces one) — fixing
  scout_nin.go's two instances alone wouldn't close the gap and touching the shared files
  mid-sibling-sweep risks scope creep across files not yet audited; needs its own dedicated pass
  across `jutsu_grants.go`, `passive_traits.go`, and every one of their call sites once the 19-file
  sweep is done.

- **`puppets.go`** (Puppet Master's Upgrade/Foundation/companion system, ~4600 lines, by far the
  largest and most heavily cross-referenced file audited so far): five fixes, three of them the
  recurring translate-before-match ordering bug — this file alone accounts for as many recurrences
  of that shape as every other file combined, because `loadPuppetUpgradeTiers` builds one large,
  deeply cross-referenced catalog where several independent downstream checks all read the SAME
  translated `Description`/`name` fields the initial `i18n.Batch` overlay had already overwritten.
  (1) `puppetUpgradeAvailableToColor` parses a description's own literal `"Techniques: Black, Blue,
  ..."` prefix to gate an option to the character's subclass color — but by the time it ran (in the
  option-filtering loop), the `Description` it read had already been translated for two DISTINCT
  reasons: the untiered-row branch (`SELECT description FROM class_options WHERE slug = ?`,
  previously raw and correctly matched, but never translated for DISPLAY either — a second, opposite
  bug in the same spot) and the tier-escalation injection branch (entries re-shown in their own
  higher tiers, reading the already-translated `entryBySlug` map). Fixed by adding
  `rawDescriptionBySlug` (captured at scan time, before the translation overlay runs) and matching
  against that everywhere `puppetUpgradeAvailableToColor` is called from this function, while
  translating the untiered branch's description separately for its own display-only copy. (2) The
  SAME tier-escalation branch also compared `o.name` (a tier's own name, e.g. "Silver Tier" —
  already translated by that point) against `puppetUpgradeTierEscalation`'s `HigherTiers`, a
  hardcoded Go-literal list of raw English tier names (`puppet_upgrade_prereq.go`) — added
  `rawTierNameBySlug` (same "capture before translate" shape) and matched against that instead.
  (3) `loadPuppetWeaponTypeOptions` (Transformer's second-Weapon-Type pick, `class_options` read
  with `list_name = 'Puppet Weapon Types'`) had no `i18n.Batch` overlay at all — plain missing-loader
  category. (4) `loadAlwaysPreparedUpgradeOptions` (Always Prepared's own temporary-Upgrade pick,
  18th level) had NO locale param and read `class_option_entries`/`class_options.name` completely
  raw for display, while correctly (already) matching `puppetUpgradeAvailableToColor` against that
  same raw text — fixed by adding `locale`, translating the OUTPUT struct's Name/Tier/Description
  fields only, after the raw-text color/prereq checks already ran. (5) `loadGreenTechniqueJutsuOptions`
  (Master of the Green Technique's long-rest jutsu pick, `v_jutsu`) had no locale param either — the
  eligibility check (`ctx.eligible`, keyword-parsing) already correctly ran against raw
  `keywords`/`rank`, so only the OUTPUT Name/Description needed the translation overlay added, after
  eligibility filtering. Separately, three hand-curated `puppetUpgradeSubChoiceOption` catalogs used
  as `FixedOptions` in the package-level `puppetUpgradeSubChoiceSpecs` map (`elementReactorOptions`,
  `integratedWeaponOptions`, `masteredArmamentOptions`) were untranslated Go literals with no way to
  take a `locale` at all, since the map is built once at package init — translation had to move to
  the point of USE instead (`loadPuppetUpgradeTiers`'s `FixedOptions` branch), via a new
  `translatePuppetUpgradeFixedOptions` dotted-key helper (`puppet_upgrade.<catalog>.<slug>.name`/
  `.description`) keyed by which entry slug is being resolved (`puppetUpgradeFixedOptionKeyPrefix`).
  `elementReactorOptions`' own `Slug`/`Name` (both currently the raw English element name, e.g.
  "Fire") is a placeholder until `elemental_affinity.go` (one of the remaining sibling files)
  establishes a real canonical element-name translation mechanism — when that happens, revisit this
  helper to reuse it rather than keeping its own parallel `puppet_upgrade.element_reactor.*` keys.
  **Found but NOT fixed, cross-cutting, same follow-up bucket as scout_nin.go's**: `loadGreenTechniqueJutsuOptions`
  and `loadAlwaysPreparedUpgradeOptions` are read-vs-write consistent within themselves, but this
  file's `loadPuppetUpgradeTiers` ALSO drives `parsePuppetUpgradePrereq`'s own prerequisite-clause
  parsing against ALREADY-TRANSLATED description/entry-name vocabulary (self-consistent — translated
  text matched against a translated vocabulary built the same way, unlike the two ordering bugs fixed
  above) — this only stays correct if the DeepL-translated `prerequisites`/`description` text for one
  `class_options`/`class_option_entries` row consistently reuses the SAME translated rendering of
  another row's `name` it references (e.g. a Spanish "Requiere: Arma Integrada" line must match the
  Spanish translation of the "Integrated Weapon" entry's own `name` exactly) — a DB CONTENT
  consistency question the DeepL translation pass would need to get right, not a code-level wiring
  gap this pass can fix. `puppetUpgradeEntryPrereqMet` (the server-side POST-time revalidation of the
  same prerequisite, `handlePuppetUpgradeAdd`'s own gate) sidesteps this entirely by matching 100%
  RAW English throughout (including forcing `loadPuppetArmorChassisOptions("")` with an empty
  locale) — deliberately, correctly, and consistent with itself, but worth knowing the display-side
  and the revalidation-side use two entirely different consistency strategies for what is
  conceptually the same check.

**Recurring bug shape, now confirmed FIVE times across two files in one session — treat as a
standing risk, not a rare edge case**: whenever a hand-curated/DB-backed field's own English text
doubles as a keyword/marker matched by substring, exact-value, or membership elsewhere downstream
(`strings.Contains(description, "Keyword: Tactical")`, an ALL-CAPS name-extraction regex, a raw
slug/tier-name membership scan built from that same text), that matching MUST run against the RAW
text BEFORE translation, never after. All five recurrences so far
(`science_nin_subclasses.go`'s `handleScienceNinSubclassPickDetail` childless-tier branch,
`scout_nin.go`'s `loadScoutNinManeuverCatalog`, and puppets.go's `puppetUpgradeAvailableToColor`
description check, its tier-escalation `HigherTiers` name check, and its untiered-row description
which had the OPPOSITE half of the bug — matched correctly raw but never translated for display)
were caught by reading every downstream consumer of a loader's return value BEFORE running
build/test, not by a test failure — neither bug produces a compile error or an English-locale test
failure, since the English catalog's raw and "translated" text are byte-identical; it only breaks
non-English locales, silently, with a
downstream candidate list simply coming back empty. **Before wiring `i18n.Batch`/`i18n.Row`/any
translate-wrapper into an existing DB-backed loader, grep every call site of that loader for
`strings.Contains`, an ALL-CAPS/regex name-extraction helper, or a raw-value membership scan applied
to its return value — if any exist, translate at the point of display (each output list, right
before it's assigned into the response struct), never inside the loader itself.**

- **The 21 canonical skill names (`internal/charsheet.SkillAbility`) have no translation catalog
  anywhere in the app** (new finding, 2026-09-07, found while auditing `hunter_pattern_choices.go`).
  Skill names are used as literal map keys throughout the codebase — `charsheet.SkillAbility`
  itself, `character_proficiencies` rows, `feature_choices.go`'s skill picker, `mastery.go`,
  `puppet_skills.go`, `hunter_pattern_choices.go`'s Habitual Researcher field, and (very likely) the
  Core sheet's own main Skills table — and are shown to the player raw and untranslated in every
  locale, including "fully translated" Spanish/Portuguese. This is bigger than the badge-label gap
  above: it's not one shared merge point but a name used as a matching key in dozens of call sites
  across the whole app, so any fix needs a display-only translate-at-the-point-of-render overlay
  (never touching the canonical English key any of those sites match against), the same shape
  `translateEnumDisplay`/`damageTypeCatalogKey` (`items.go`) already established for a different
  enum. **Deliberately NOT fixed in this pass** — `hunter_pattern_choices.go`'s own
  `sortedSkillChoiceOptions` was left with untranslated Labels rather than inventing a one-off,
  file-local translation key namespace for skill names that a proper sitewide fix would later have
  to reconcile with. Needs its own dedicated pass, scoped to cover every one of the sites listed
  above at once (the project's own "translate everything in one shot" rule), not just whichever
  sibling file happens to touch skill names next.
- **`hunter_pattern_choices.go`** (2026-09-07): `buildPendingHunterPatternChoiceRows` had no
  `locale` parameter — its two hardcoded `Label: "Kleptomaniac"`/`Label: "Habitual Researcher"`
  banner strings were duplicating text that already lives in `class_options` (both are real Hunters
  Patterns catalog rows), so fixed by looking up each pattern's own translated `name` via
  `i18n.Row(s.rulesDB, locale, "class_options", slug)` (new `hunterPatternChoiceLabel` helper,
  falling back to the raw English name if the row can't be read, since a pending-choice banner
  isn't worth failing the whole sheet render over) rather than duplicating a second, parallel
  hardcoded string. `kleptomaniacChoiceOptions`' own two options (Sleight of Hand / Security Kits)
  ARE this file's own hand-curated text — wired through the existing
  `translateFeatureChoiceOptions` (`fco_label:`/`fco_desc:` convention, `feature_choices.go`), 4 new
  locale keys added (2 labels + 2 descriptions, each with an English identity entry).
  `sortedSkillChoiceOptions` left untranslated — see the skill-names finding above. Updated all 3
  call sites (`characters.go`'s two full-sheet-render sites plus this file's own
  `handleSheetHunterPatternChoice`) to pass `localeFromContext(r.Context())`.
- **`wow_whelp.go`, audited 2026-09-07, no fix applied** (new cross-cutting finding, not a
  file-local bug): the whole file is Science-Nin's Draconic Gauntlet Whelp companion — a
  `kind="custom"` companion `ensureWhelpCompanion` auto-adds ONCE (name "Whelp (Draconic
  Gauntlet)", three `charstore.CompanionAttack` rows with hardcoded English `Name`/`Description`
  text lifted straight from the book), matching the exact same shape
  `ensureAngelEIPSpectreCompanion` (`science_nin_subclasses.go`, already a **closed** file in this
  sweep) already uses for its own Spectre companion. Neither auto-add takes a `locale` parameter or
  translates anything — the rulebook-sourced attack/description text is written once, in raw
  English, into an ordinary per-character `custom` companion row that (like any other
  player-authored custom companion) is never re-read through an `i18n` overlay afterward. This is
  a genuine gap (a Spanish/Portuguese player's auto-granted Whelp/Spectre shows English attack text
  forever, unlike every DB-catalog-backed companion kind), but it's NOT specific to `wow_whelp.go`
  — fixing it properly means threading `locale` through both this file's `ensureWhelpCompanion` (5
  call sites across `science_nin_elemental_innovationist_popup.go` and Core-sheet add/delete
  routes, per `syncDraconicGauntletWhelp`'s own doc) AND reopening the already-closed
  `science_nin_subclasses.go` for its own identically-shaped Spectre gap, translating the
  write-once text at CREATION time (there's no live re-read to overlay onto, unlike every other
  gap fixed so far in this sweep). Left as a flagged, deferred cross-cutting item rather than a
  partial one-file fix that would leave the sibling `science_nin_subclasses.go` instance
  inconsistent. Everything else in `wow_whelp.go` — `loadWhelpReference`'s AC/HP recompute, the
  exact-name companion matching — is numeric/matching-key only, nothing else to translate.
- **`medical_nin.go`** (2026-09-07): `loadKnownExpertCombatantJutsu` (Combat Medic's Expert
  Combatant picker, sourced from the character's own known Taijutsu/Bukijutsu rather than a rules
  catalog) had no `locale` parameter at all and returned raw `v_jutsu`/`custom_jutsu` Name/
  Description untranslated — the one genuine missing-overlay bug in this file. Fixed by adding a
  `locale` parameter and overlaying `i18n.Batch(s.rulesDB, locale, "jutsu", ...)` onto the
  published-jutsu entries only (a parallel `publishedSlugs` slice tracks which output rows came
  from a published jutsu vs. player-authored `custom_jutsu`, which has no translation catalog at
  all) AFTER `HasCombination`'s own raw-`keywords`-substring match — `keywords` itself is never
  overwritten by the overlay, so this needed no ordering fix, just the missing overlay. No new
  locale keys needed (reuses the existing `jutsu` table catalog). Everything else in the file was
  already correct: `loadMedicalDoctrineCatalog` already took `locale` and overlaid
  `class_features` correctly; `medicalDoctrinePickedRows`' `SourceLabel: "Medical Doctrine"` looked
  like the same hardcoded-badge-label gap as `scoutNinMobileSavantGrantedJutsu` but is actually
  dead text — `computeCustomResources` (custom_resources.go) only ever reads `.Slug` off these
  synthetic rows, confirmed by reading the function; `medicalNinJutsuChartGrantedJutsu`'s
  `labels[...] = "Subclass Feature"` IS a real instance of the already-flagged cross-cutting
  badge-label gap (feeds `characters.go`'s `grantLabels` map into a real player-facing
  `.jutsu-source-badge`, alongside `puppetUpgradeTableGrantedJutsu` and
  `scoutNinMobileSavantGrantedJutsu`'s identical-shaped hardcoded labels) — left deferred,
  consistent with the `scout_nin.go` finding, since fixing it means threading `locale` through
  three call sites feeding one shared merge point in `characters.go`, not a `medical_nin.go`-local
  change.
- Found but NOT fixed, cross-cutting (adds to the `jutsu_grants.go`/`passive_traits.go` note
  above): `medicalNinJutsuChartGrantedJutsu`'s `"Subclass Feature"` label is a third confirmed
  instance of the shared jutsu-source-badge hardcoded-label gap (alongside
  `puppetUpgradeTableGrantedJutsu` and `scoutNinMobileSavantGrantedJutsu`), all three merged
  together in `characters.go`'s jutsu-sheet-loading code around `grantLabels`. None of the three
  granting functions take a `locale` parameter today. This confirms the gap is a single shared
  mechanism spanning `puppet_upgrade_jutsu_grants.go`, `scout_nin.go`, and `medical_nin.go` (plus
  `jutsu_grants.go`/`passive_traits.go` from the earlier finding) — still deferred to its own pass
  once the 19-file sweep is done, not fixed piecemeal per sibling file.
- **`taijutsu.go`** (2026-09-07): three fixes. (1) `loadMartialTechniquesTabData`'s raw Martial
  Techniques `class_options` query had no `i18n.Batch` overlay at all — genuine missing-overlay
  bug, returning fully untranslated Name/Description; fixed the same way as every other bare
  `class_options` read in this sweep, no matching-key risk since `Slug` is what's matched, not
  `Name`. (2) `handWrapsOfPassionOptions` (Passionate Flame's 3-option hand-curated catalog) wired
  through the existing `translateFeatureChoiceOptions` helper — safe, matched only on `.Value`. (3)
  `ruinAntiChakraView.Options` converted from `[]string` to `[]featureChoiceOption` via a new
  `translateAntiChakraWavelengthOptions` function — this was a genuine persisted-matching-key risk:
  the 2-of-10 keyword pick is stored via `SetFeatureChoice` and re-matched by exact string later, so
  translating the stored value itself would have broken re-matching after a locale switch; `Value`
  stays the raw canonical English keyword, `Label` carries the translation.
  `character_taijutsu_ruin.html`'s two `<select>` ranges updated to use `.Value`/`.Label` instead of
  the bare `.`. ~19 new locale keys added (`fco_label:`/`fco_desc:` for Hand Wraps of Passion,
  `acw_keyword:` for the 10 Anti-Chakra Wavelength keywords).
- Found but NOT fixed, cross-cutting (new, surfaced while auditing `taijutsu.go`): every
  `subclassTrackerPopupData{Title: "...", ...}` construction site hardcodes its `Title` and
  `EmptyHint` fields in English, and `Title` renders as a real visible `<h1>` on the popup page
  (`partials/subclass_tracker_popup.html`'s `subclass_tracker_popup_header` block), not just a
  browser-tab title. `grep -rn "subclassTrackerPopupData{Title:" cmd/n5e/*.go` finds **22 call
  sites** across `cooking_nin_blend_enhancements_popup.go`, `cooking_nin_expert_combatant_popup.go`,
  `cooking_nin_pipe_popup.go`, `cooking_nin_fast_and_furious_popup.go`,
  `genjutsu_twisted_casting_popup.go`, `genjutsu_psyche_breaker_popup.go`,
  `medical_nin_combat_medic_popup.go`, `hunter_creed_popup.go`, `science_nin_grenadier_popup.go`,
  `science_nin_shinobi_ware_popup.go`, `science_nin_spyware_popup.go`,
  `ninjutsu_specialist_awakened_scroll_popup.go`, `scout_nin_scouting_technique_popup.go`,
  `science_nin_storm_rider_popup.go`, `intelligence_operative_operative_traps_popup.go`,
  `science_nin_technobi_popup.go`, `science_nin_ninjaneer_popup.go`, `taijutsu_ruin_popup.go`,
  `weapon_form_popup.go`, `science_nin_mad_scientist_popup.go`, `taijutsu_stancer_popup.go`,
  `taijutsu_passionate_flame_popup.go`. This spans many files already marked "closed" in this sweep
  (`hunter_nin.go`'s `hunter_creed_popup.go`, `genjutsu.go`'s two popups, `scout_nin.go`'s popup,
  `medical_nin.go`'s popup, `cooking_nin.go`'s four popups) — those files are not actually 100%
  complete for this reason. Given the scale (22 sites across 15+ files, several already closed),
  needs its own single dedicated pass (a translated Title/EmptyHint lookup, or a small map threaded
  through `subclassTrackerPopupData`) rather than being fixed piecemeal — do not fix only a few and
  call it done; fix all 22 in one pass or defer all 22 together. Same "defer a shared/cross-cutting
  mechanism to its own pass" precedent as the badge-label gap and the skill-names gap above.
- **This popup-Title gap is WIDER than first scoped, confirmed 2026-09-07 while auditing
  `weapon_specialist.go`**: `subclass_tracker_popup.go`'s sibling struct `subclassTrackerSection`
  (`Title`, `Hint`, `AddLabel` fields — the per-section heading/hint/button-label inside a popup,
  not the page-level `subclassTrackerPopupData.Title` counted above) has the exact same hardcoded-
  English shape, at every one of that struct's construction sites across the same set of popup
  files (spot-checked `hunter_creed_popup.go`'s four `subclassTrackerSection{Title: "Warden
  Weapon (...)" ...}`-shaped sites, all hardcoded, in an already-"closed" file). Not counted in the
  22-site number above, not separately grepped/counted yet — same shared mechanism
  (`subclass_tracker_popup.go`), same deferred-to-its-own-pass treatment, but the real total scope
  of "fix every subclass-tracker-popup hardcoded string in one pass" is `subclassTrackerPopupData`
  AND `subclassTrackerSection` sites together, not just the 22 `Title`/`EmptyHint` ones. Whoever
  picks up this deferred pass should grep `subclassTrackerSection{` fresh rather than trusting the
  22-site count above to be the whole job.
- **`weapon_specialist.go`** (2026-09-07): `loadWeaponFormTabData` had no `locale` parameter at
  all — its Weapon Form Styles `class_options` query returned fully untranslated Name/Description,
  the same bare-missing-overlay shape fixed repeatedly elsewhere in this sweep (confirmed the
  translations themselves already exist in `content_translations` for both `es`/`pt`, just never
  read); fixed by adding `locale`, overlaying `i18n.Batch(..., "class_options", styleSlugs)`, and
  threading the new parameter through every call site (`loadSuperiorWeaponFlurryTabData`,
  `addWeaponFormStyle`, `addSuperiorWeaponFlurryBenefit`, both `characters.go` full-sheet-render
  sites, and all three `weapon_form_popup.go` routes). `stalkingPredatorOptions` (Slayer Form's
  Stalking Predator, already a `[]featureChoiceOption`) wired through the existing
  `translateFeatureChoiceOptions` helper — safe, matched only on `.Value`
  (`"vipers-tongue"`/`"apex-glare"`), never on translated text.
  `superiorWeaponFlurryCatalog` (base class's own hand-curated 4-benefit catalog, NOT a
  `featureChoiceOption` — its own `superiorWeaponFlurryOption{Slug, Name, Description}` shape) got
  a new `translateSuperiorWeaponFlurryCatalog` following `translateAntiChakraWavelengthOptions`'s
  precedent (`taijutsu.go`) for a hand-curated catalog with no `class_options` row of its own —
  `Slug` stays the raw canonical value (matched by `AddSuperiorWeaponFlurryBenefit`/
  `RemoveSuperiorWeaponFlurryBenefit`), `Name`/`Description` translated via new `"swf_name:"`/
  `"swf_desc:"` locale keys. `weaponFormTechniqueAutoGrants` (the map of granting-feature-slug to
  2-3 bespoke Flurry Technique names, e.g. `{"Disastrous Strike", "Forced Regression"}`) is left
  untranslated — see the new cross-cutting finding below, this is NOT specific to this file.
- Found but NOT fixed, cross-cutting (new, surfaced while auditing `weapon_specialist.go`): every
  "feature slug → hardcoded English name(s) it auto-grants for free" map in this codebase has the
  identical untranslated shape — confirmed by name across files: `weaponFormTechniqueAutoGrants`
  (this file), `martialTechniqueAutoGrants` (`taijutsu.go`, already a **closed** file — meaning it
  is not actually 100% complete for this reason either), `ninjutsuMoldingAutoGrants`
  (`ninjutsu_specialist.go`, not yet audited in this sweep), `huntersExploitAutoGrants`
  (`hunter_nin.go`, already **closed**), `puppetUpgradeAutoGrants` (`puppets.go`, already
  **closed**), and `genjutsuInceptionMirageAutoGrants` (`genjutsu.go`, already **closed**). All six
  render their hardcoded Name(s) as real player-facing "Granted" entries on the sheet (the auto-
  known, no-cap, no-delete-button treatment these auto-grant maps share, per
  `weaponFormTechniqueAutoGrants`'s own doc comment). None of these values are ever persisted or
  matched against — they're pure display text, keyed by the (already-translatable) granting
  feature's own slug — so fixing this is purely a translation-lookup problem, not a matching-key
  risk one. Given the scale (6+ maps across 6 files, most already closed) and the shared display
  shape, this is the same "defer a cross-cutting mechanism to its own pass" call as the badge-label,
  skill-names, and subclass-tracker-popup gaps above — do not fix only `weaponFormTechniqueAutoGrants`
  and call it done.
- **`titan.go`** (2026-09-07): two genuine hardcoded-Name gaps, both fixed. `titanBashAttack`'s
  rollable "Bash" row `Name` was a bare Go string literal (never translated, unlike every sibling
  attack-row builder in this file which already reads its own Name via `i18n.Translator` — e.g.
  `titanXo16GatlingAttack`) — fixed via a new `"titan.bash_attack_row_name"` key. Kept as the
  English loanword "Bash" in every locale, matching `companion_fields.bash_heading`'s own existing
  identity translation for the same word (a companion-field label, unrelated file, but the same
  established "Bash stays Bash" precedent). `loadTitanUpgradeCatalog`'s Mastercraft-tier special
  case (`titanUpgradeOption{..., Name: "Bijuu Slayer", ...}` — the one Titan Upgrade item that was
  never split into its own `class_option_entries` row, per this file's own header doc, so it has no
  DB-backed translation to overlay) was a second bare literal; fixed via a new
  `"titan.bijuu_slayer.name"` key, translated ("Asesino de Bijuu"/"Assassino de Bijuu") — "Bijuu"
  itself stays untranslated (Naruto-specific term, per this project's own "translate everything
  except genuinely Japanese-origin/Naruto terms" rule), "Slayer" translated the same way the
  existing `"cr_name:Master Slayer"` entry already does ("Asesino"/"Assassino"). Every other
  `Name:`/`Description:` assignment in the file was already correctly wired through
  `i18n.Translator`/`i18n.Batch`/`i18n.Row` — this file was already unusually thorough about i18n
  (see its own extensive header-doc citations of prior translation work) before this pass; only
  these two literal proper-noun names had slipped through.
- Found but NOT fixed, cross-cutting (two NEW findings, both surfaced while auditing `titan.go`):
  (1) every "tier/category name" field sourced from a raw `class_options.name`/`class_option_entries`
  parent-row value (not the entry's own translatable `name`) is rendered player-facing untranslated
  — confirmed by name: `titanUpgradeOption.Tier` (`titan.go`, e.g. "Minor"/"Refined"/"Mastercraft",
  rendered raw in `character_sheet.html` at `{{.Tier}}`/`{{T "sheet.tier_cost" .Tier .Cost}}`),
  `puppetUpgradeTier.TierName` (`puppets.go`, already **closed** — "Wood Tier"/"Bronze Tier"/etc.,
  same raw-`class_options.name` source), and (not yet individually confirmed, but sharing the exact
  same `Tier string` struct-field shape) `science_nin.go`'s and `science_nin_subclasses.go`'s
  (already **closed**) own tool/upgrade catalogs. (2) `companionAttackRow.GrantedHint` — the
  "Granted by a [X] — remove the [X] to remove this attack" hint text shown on every auto-granted
  rollable attack row — is a bare hardcoded English string at every construction site, confirmed at
  `titanGrantedUpgradeHint` (this file) and `puppets.go`'s identically-shaped
  `"Granted by a Puppet Upgrade — remove the upgrade to remove this attack"` (`puppets.go`, already
  **closed**). Both are the same shape of gap already established in this sweep (a shared struct
  field populated with raw English at many call sites across many files, several already marked
  closed) — deferred to their own dedicated pass(es) rather than fixed piecemeal here, consistent
  with every other cross-cutting finding above. Whoever picks these up should grep
  `Tier:\s*o\.(name|Name)\b`/`TierName:\s*o\.name\b` and `GrantedHint:\s*"` respectively across
  `cmd/n5e/*.go` fresh, rather than trusting this note's file list to be exhaustive.
- **`elemental_affinity.go`** (2026-09-07): surfaced a genuine FUNCTIONAL CORRECTNESS bug, not just
  a display gap — `jutsu.keywords` was listed as a translatable field
  (`internal/i18n/fields.go`'s `TranslatableFields["jutsu"]`) and DeepL-translated, but this file's
  own `jutsuElement`/`jutsuElements`/`jutsuNeedsAnyAffinity`/`elementFromReleaseKeyword` (and
  `jutsu_eligibility.go`'s callers) match FIXED ENGLISH SUBSTRINGS against a jutsu's own keywords
  column at runtime (`strings.Contains(keywords, "Fire Release")`, etc.) to decide which jutsu a
  character is eligible to learn. Confirmed live against `out/rules.db`: `"Fire Release, Ninjutsu"`
  had been translated to `"Técnica del Fuego, Ninjutsu"` (es) — a string `elementReleaseKeyword`
  would never match. Net effect: in every non-English locale, elemental jutsu eligibility was
  silently broken wherever `loadJutsuList`/`loadJutsuDetail` overlaid the translated `keywords`
  value onto `j.Keywords` before it reached the matching code (`characters.go`'s own eligibility
  loop at `jutsuEligible(jutsuOrigins[j.Slug], j.Keywords, ...)`) — likely making most/all
  Nature-Release-gated jutsu read as ineligible for es/pt players (or, depending on which locale
  string happened to substring-collide, silently WRONG eligibility), with no error, no build
  failure, and no obvious symptom short of manually comparing eligible-jutsu counts across locales.
  Fixed at the root: removed `"keywords"` from `jutsu`'s `TranslatableFields` (same "machine-parsed,
  must stay in the source language" exclusion category as the pre-existing `jutsu.components` and
  `equipment.properties`/`damage_type` exclusions, now documented the same way) and removed the two
  runtime overlay sites in `jutsu.go` (`loadJutsuList`, `loadJutsuDetail`) that copied
  `tr["keywords"]` onto the struct field — `content_translations` rows for `jutsu`/`keywords` are
  now simply never read, regardless of whether stale rows still exist in a given `rules.db` (not
  cleaned up as part of this fix; harmless dead data, not a correctness risk, since nothing reads
  them anymore). One real trade-off: `partials/jutsu_detail.html`'s own `{{.Keywords}}` display (the
  jutsu detail popup's "Keywords" row) now always shows the raw English column value in every
  locale — the same already-accepted trade-off `equipment.properties` makes for the identical
  reason (a machine-parsed field doubling as a raw-text display field, where reversible per-token
  display translation would need its own dedicated parser, deferred rather than attempted here).
  Separately, this file's own genuine per-file gap (once the correctness bug above was ruled out as
  something bigger to defer): `elementalAffinity.Source`, `elementalAffinitySlot.Label`, each
  `allAffinityFeatureSlots` entry's `FeatureLabel` (e.g. "Versatile Release", "Blood of the
  Dragon"), and `elementalAffinitySlot.Options`/`elementalAffinity.Element` (the raw "Fire"/
  "Water"/etc. element names) were all rendered directly to the player
  (`character_sheet.html`'s `sheet_elemental_affinities` block) with zero translation, while also
  being the exact same values `resolveElementalAffinities`/`elementalAffinitySlots` return for
  server-side matching/persistence (`handleElementalAffinityAdd`'s `slices.Contains`,
  `charstore.SetElementalAffinity`) — another genuine persisted-matching-key risk, same shape as
  taijutsu.go's Anti-Chakra Wavelength fix. Resolved by leaving `resolveElementalAffinities`/
  `elementalAffinitySlots` themselves untouched (still canonical English, still safe for
  `jutsu_eligibility.go`'s own separate `characterElementalAffinities` matching path and for
  `handleElementalAffinityAdd`'s validation) and adding a display-only translation layer instead:
  `translateElementName`/`translateAffinityLabel` (new `"element.*"` and `"ea_label:"` locale
  namespaces) plus `translateResolvedAffinities`/`translateAffinitySlotsForDisplay`, called in
  `characters.go` only AFTER the raw canonical values have already fed `affinitySet`/
  `jutsuEligible` — a new `elementalAffinitySlotView`/`elementalAffinityOption` pair of display-only
  types (`Value`/`Label` split, mirroring `featureChoiceOption`) replaces the picker `<select>`'s
  bare `[]string` Options so the submitted `value` stays the canonical English element name while
  the visible `<option>` text is translated; `character_sheet.html`'s `sheet_elemental_affinities`
  block updated to read `.Value`/`.Label` instead of the bare `.`.
- **`snb.go`** (2026-09-07): this file was already unusually i18n-thorough (every displayed string
  already routed through `i18n.Translator`/`i18n.Row` before this pass — the S.N.B stat block was
  clearly built with i18n in mind from the start, unlike most files in this sweep). Two real
  findings anyway: (1) another instance of the SAME class of bug `elemental_affinity.go`'s
  `jutsu.keywords` fix documents above — `loadSNBReference`'s own `knownUpgradeNames` map was keyed
  by `o.Name` from `loadScienceNinEntryCatalog`'s translated S.N.B Upgrades catalog, then compared
  against 5 hardcoded English literals ("Armored Exterior", "Sturdy", "Improved Accuracy", "Piston
  Legs", "Enhanced Size") to decide whether to fold each upgrade's own flat numeric bonus (AC floor,
  HP bonus, speed override, to-hit bonus, size override) into the computed stat block — silently
  never matching in es/pt, since `o.Name` there is the DeepL-translated name, not the English
  literal. Confirmed via `sqlite3` that all 5 have stable slugs
  (`class/science-nin/option/s-n-b-upgrades/.../entry/<slug>`). Fixed by switching the map to
  `knownUpgradeSlugs` (keyed by `o.Slug`, immune to locale) and adding 5 named slug constants
  (`snbArmoredExteriorSlug` etc.) instead of comparing against `o.Name` at all — the correct general
  lesson from this and the `jutsu.keywords` bug: **any comparison against a `class_option_entries`/
  `class_options`/`jutsu`/etc. row's own `Name` (or any other `TranslatableFields`-listed column) is
  a latent locale bug — always match by `Slug` instead, and treat a `.Name`-keyed map/comparison
  found anywhere in this codebase as worth checking even outside this per-file sweep.** (2) the
  rollable Bite attack row's own `Name: "Bite"` (in `snbBiteAttack`) was a bare literal, never
  translated — same shape as `nindog.go`'s own already-fixed Bite and `titan.go`'s own already-fixed
  Bash; fixed via a new `"snb.bite_name"` key (translated "Mordisco"/"Mordida", matching
  `nindog.bite.name`'s own precedent for the same word).

**Standing check for any future "is this language done?" audit**: grep the whole `cmd/n5e`
package for `(Name|SourceLabel|Label|Title|Error|Restriction):\s*"[A-Z]` to find this category of
gap by SOURCE, not by database/catalog completeness percentage — completeness reports only cover
what was in scope to begin with, and this whole class of string structurally wasn't. As of
2026-09-07, `custom_resources.go`, `hunter_nin.go`, `genjutsu.go`, `cooking_nin.go`,
`wow_weapons.go`, `nindog.go`, `science_nin_subclasses.go`, `scout_nin.go`, `puppets.go`,
`medical_nin.go`, `hunter_pattern_choices.go`, `taijutsu.go`, `weapon_specialist.go`,
`titan.go`, `elemental_affinity.go`, and `snb.go` are closed; `wow_whelp.go` was audited
(2026-09-07) and found clean OF THIS PARTICULAR
gap — its own translation gap is real but cross-cutting (see its own bullet above) and
deliberately deferred rather than fixed in isolation, so don't re-audit it from scratch, but also
don't count it "closed" until the cross-cutting companion-auto-add pass lands; `taijutsu.go` and
`weapon_specialist.go` also surfaced their own cross-cutting deferred items (the 22-site-and-growing
`subclassTrackerPopupData`/`subclassTrackerSection` Title/Hint/EmptyHint/AddLabel gap, and the
"auto-grant map with hardcoded Name(s)" gap spanning `weaponFormTechniqueAutoGrants`/
`martialTechniqueAutoGrants`/`ninjutsuMoldingAutoGrants`/`huntersExploitAutoGrants`/
`puppetUpgradeAutoGrants`/`genjutsuInceptionMirageAutoGrants`, see their own bullets above), and
`titan.go` surfaced two more (the `Tier`-field gap spanning `titan.go`/`puppets.go`/
`science_nin.go`/`science_nin_subclasses.go`, and the `GrantedHint` gap spanning `titan.go`/
`puppets.go`, see their own bullet above) — same
"don't count fully closed until the relevant cross-cutting pass lands" caveat applies to
`taijutsu.go`, `weapon_specialist.go`, `titan.go`, and every other closed file that turns out to
share any of these mechanisms (already confirmed true for `hunter_nin.go`, `puppets.go`, and
`genjutsu.go` via the auto-grant-map gap; for `hunter_nin.go`/`genjutsu.go`/`scout_nin.go`/
`medical_nin.go`/`cooking_nin.go` via the popup gap; and for `puppets.go` via both the Tier-field
gap and the GrantedHint gap); ~3 sibling files (`science_nin.go`, `puppet_tactics.go`,
`full_metal_shinobi.go`) remain fully unaudited — check
each one for its own hidden `Restriction`/`Description`-shaped field too (the grep above catches
`Restriction` now, but a new field name in a new file won't match until it's added here); check for
outlying lookup maps outside the file's main catalog(s) the way `hunterNinPatternPassiveRows` was
hiding one; AND — per the `genjutsu.go` finding above — check every DB-backed loader function in
the file (`SELECT ... FROM class_options`/`class_features`/`equipment`/etc.) actually takes a
`locale` parameter and calls `i18n.Batch`/`i18n.Row` before returning, since a loader silently
missing that call produces zero hardcoded-English-literal grep hits while still leaking
untranslated rules-DB content onto the sheet in every non-English locale — AND, per the
`scout_nin.go` finding above, before adding that call, check whether the loader's return value ever
feeds a downstream keyword/marker match, per the recurring-bug-shape paragraph above. Also check
whether the file's hand-curated catalogs already use `featureChoiceOption`
(`cmd/n5e/feature_choices.go`) — if so, reuse `translateFeatureChoiceOptions` (added for
`cooking_nin.go`) rather than writing a new per-file wrapper. AND — per the `wow_weapons.go` finding
above — before translating any hardcoded field that gets persisted onto a per-character row rather
than recomputed fresh each request, check
whether it doubles as a matching key or a machine-parsed keyword field elsewhere in the codebase
before assuming it's safe to translate.

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
