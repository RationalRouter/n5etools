# Source text grammar audit

This is a scan of the game's own English source content in `dist/rules.db` for
grammatical errors that are independent of translation — dangling clauses, comma
splices, subject-verb disagreement, missing/duplicated words, and similar. It was
prompted by a real error found while translating the Snake summon tribe's flavor
text into Spanish (see `internal/i18n/translation_methodology.md`): the English
source itself ends on a dangling clause, unrelated to any translation choice.

Nothing in `dist/rules.db` or `out/rules.db` has been changed. This is a list for
a human to review and decide on — some items are clear-cut, some are suggestions
only, and a few are flagged explicitly as low-confidence guesses that may turn out
to be intentional phrasing or acceptable informal style.

35 tables with prose-bearing columns were scanned (every `description`/`overview`/
`feature_text`/`raw_text`/`effect_text`-shaped TEXT column across the schema, i.e.
adversary roles/traits, armor and weapon properties, backgrounds, bloodline
latents, clans, clan features/traits, class descriptions/quick-builds, class
features, class multiclass rules, class options and option entries, feats,
fighting stances, jutsu (description and at-higher-ranks), poisons, puppet armor
chassis (+ properties), puppet swarm/tool stat blocks, subclasses, subclass groups,
subclass features, summon tribes (+ attacks, features, roles), titan unit cards,
trap templates). A few tables turned out to hold no real prose worth auditing
(`ability_point_costs`, `elemental_advantage`, `jutsu_ranks`, `xp_levels`,
`source_books`, and similar lookup-only tables) and were skipped.

Method: every prose column was dumped to a flat text file (one row per line,
`slug~column text`), then scanned with grep/regex passes for known tell-tale
shapes (the "with its/their ... to ..." dangling-clause shape that started this
audit, duplicated words, "a/an" article mismatches, "than/then" confusion,
subject-verb disagreement patterns, unmatched punctuation) followed by manual
read-through of every match plus a full manual read of the smaller tables
(poisons, traps, weapon/armor properties, puppet armor chassis, fighting stances,
summon tribe attacks/roles, clan traits, backgrounds, clan ability-increase text,
class quick-builds, class descriptions, subclass descriptions).

**Total: 1 systemic finding covering ~150 individual occurrences across 11
tables, plus 26 individual confirmed/high-confidence issues, plus 3 low-confidence
items**, broken out below by table.

---

## Systemic finding: "a +N bonuses" (singular article + plural noun)

By far the most common pattern found: text reads "gain **a +2 bonuses**" (or
+1/+3/+4/+5/+10/+15/+20), mixing the singular article "a" with the plural noun
"bonuses," where a single bonus is being granted. Should almost certainly read
"a +2 bonus" throughout. This is not a one-off typo — it recurs **147 times**
across 11 different tables, strongly suggesting it was typed this way as a
matter of habit by whoever authored these sections, rather than 147 unrelated
mistakes.

Per-table occurrence counts (grep count of `\ba \+?[0-9]+ bonuses\b`):

| Table | Count |
|---|---|
| jutsu.description | 49 |
| clan_features.description | 21 |
| feats.description | 15 |
| class_options.description | 14 |
| bloodline_latents.description | 13 |
| class_option_entries.description | 9 |
| subclass_features.description | 9 |
| summon_tribe_features.description | 6 |
| class_features.description | 5 |
| jutsu.at_higher_ranks | 3 |
| puppet_swarm_stat_block.raw_text | 2 |
| fighting_stances.description | 1 |

A representative sample of affected rows (not exhaustive — a fresh
`grep -E '\ba \+?[0-9]+ bonuses\b'` over each column will find the rest):

- `jutsu.description` / `jutsu/enhanced-resistance`, `jutsu/chakra-mark`, `jutsu/sealing-art-block`
- `clan_features.description` / `clan/chinoike/feature/ketsuryugan`, `clan/futton/feature/boil-over`, `clan/futton/feature/boiling-chakra`
- `feats.description` / `feat/namikaze/superior-speed`, `feat/uchiha/madaras-hatred`, `feat/yuki/chilling-realization`
- `class_options.description` / `class/genjutsu-specialist/option/malleable-mirages/supreme-illusionary-weapon`, `class/taijutsu-specialist/option/martial-techniques/raw-taijutsu`
- `bloodline_latents.description` / `latent/akimichi/latent-food-pill-i`, `latent/fuma/latent-immaculate-precision-iiii`
- `class_option_entries.description` / `class/puppet-master/option/armorers-upgrades/platinum-tier/entry/mech-pilot`
- `subclass_features.description` / `class/hunter-nin/group/hunters-creeds/arsenalist/feature/arsenals-proficiency`
- `summon_tribe_features.description` / `summon/dog-wolf/feature/alpha-predator`, `summon/lizard/feature/tokages-ferocity`
- `class_features.description` / `class/intelligence-operative/feature/sabaki`, `class/medical-nin/feature/chakra-scalpel`
- `jutsu.at_higher_ranks` / `jutsu/toads-whipping-tongue`
- `puppet_swarm_stat_block.raw_text` / `class/puppet-master` (two occurrences in the same block: "gains a +2 bonuses to AC" and "giving the attack a +1 to hit and a +3 bonuses to the damage roll")
- `fighting_stances.description` / `stance/dragon-fist-stance` ("You gain a +2 bonuses to your AC until the start of your next turn")

Given the count, a targeted find/mostly-safe-replace pass (`a +N bonuses` →
`a +N bonus`) across all 11 columns is likely more efficient for a human to run
than fixing each row individually — but flagged here as a suggestion, not applied.

---

## Original bug that prompted this audit

**`summon_tribes.description` / `summon/snake`**: "There are few land creatures
more dangerous, cunning, and powerful than a snake. The snake is a beast which
has ties and stories that date back to before time was tracked. **The snake's
greatest weapon is its cunning and powerful body with its sharp fangs to pierce
the flesh of its enemies.**"

The final sentence is a dangling clause — "with its sharp fangs to pierce the
flesh of its enemies" has no verb attaching it to the sentence. Suggested fix
(not applied to `dist/rules.db`, which still holds the original broken text):
split into two sentences, e.g. "...its cunning and powerful body. Its sharp
fangs are used to pierce the flesh of its enemies."

**Status: confirmed, and the corrected English is already what got translated**
into Spanish in `internal/i18n/glossary.md`'s Summon Tribes pilot batch — see
that file's "Terms surfaced by the Summon Tribes..." section for the approved
Spanish translation built from this corrected sentence, not the original
broken one. This is the reference case for the process rule in
`translation_methodology.md`: translate from a confirmed correction here, not
from the raw source text, whenever one exists for the row being translated.

## Status tracking

**2026-09-05: implemented in code.** Every finding in this report that was
confirmed (not flagged low-confidence) now has a real, persistent fix in
`internal/correct`, which survives re-ingestion the same way every other
correction in that package does:

- The systemic "a +N bonuses" pattern (147 occurrences, 11 tables) has a new
  dedicated heuristic, `internal/correct/bonusplural.go` (regex
  `\ba (\+?\d+) bonuses\b` -> `a $1 bonus`), wired into `Sweep` as a new pass
  and a new `Report.BonusPluralFixes`/`Record.Tool == "bonusplural"`.
- The 8-row class-intro comma splice and every other individually-confirmed
  one-off issue (jutsu, bloodline_latents, clan_traits, clans, backgrounds,
  feats, fighting_stances, summon_tribe_attacks, classes, subclasses,
  subclass_features, class_options/class_option_entries) were added to the
  existing `internal/correct/curated.go` list — every `Original` string was
  verified against the live `dist/rules.db` text before being added (each
  matched exactly once, or twice for the class_options/class_option_entries
  duplicate-table pairs, as expected).
- Items the audit itself flagged as low-confidence (needing a larger,
  not-safely-automatable rewrite) were deliberately left alone rather than
  guessed at — e.g. `class/ninjutsu-specialist/.../sanguine-master`'s
  dangling "look upon" with no object, and the `shinjutsu/sukunahikona`
  sentence beyond its confirmed article fix. These still need a human content
  edit, not a mechanical correction.

**Not yet done**: these fixes exist in code but have not yet been applied to
`dist/rules.db` — that requires re-running `cmd/n5e-ingest`, which was
deliberately not run yet (a heavy-write operation on this project's data,
confirmed with the user before running each time).

Whenever a pilot-batch translation touches a row with a confirmed finding
here, the translation is still built from the corrected English, and that's
noted at the point of use in `glossary.md` (e.g. the Snake entry above). If a
future translation pass needs to know "is this row's English already
known-broken," check this file first — that's the reason it's kept as a
living reference alongside the glossary rather than a one-time report to be
filed away.

## Second pass (2026-09-05): proper `spelling-grammar-checker` subagent sweep

A follow-up sweep by the project's actual specialized `spelling-grammar-checker`
subagent (not the general-purpose agent that produced the report above) was run
specifically to cover `equipment.description` and `class_options`/
`class_option_entries`, which the first pass only pattern-matched due to their
size (466KB/288KB), plus a re-check of every other table. All of its confirmed
findings are now also implemented in `internal/correct`:

- **New systemic pattern**: "a -N penalties" (the mirror image of "a +N
  bonuses"), 63 occurrences across 10 tables — new heuristic
  `internal/correct/penaltyplural.go`.
- **~20 new one-off fixes** added to `internal/correct/curated.go`: several
  jutsu (`wind-release-vacuum-barrier`, `earth-release-tectonic-wave`,
  `flying-thunder-god`, `night-bitter-feelings`, both Lightning-release
  "its covering"/"it's covering" jutsu, `shikigami/origami-servitor`),
  `feat/shoton/crystal-soul`, three `class_features` rows (`weapon-form`,
  `gifted-healer`, `martial-adept`), one `subclass_features` row
  (`natures-avatar`), several Science-Nin `class_options`/`class_option_entries`
  upgrade rows (`body-mass-increase`, `advanced-biotic-amplifier`,
  `floating-shoes`, `ricocheting-weapon`, `sinister-skull`), and 5 `equipment`
  enhancement-seal rows missing the word "seal" ("imbued with **seal**"/
  "imbued with **this** grants" instead of "imbued with this **seal**").
- **A LanguageTool auto-apply regression was caught and corrected as a side
  effect**: `text_corrections` shows LanguageTool's `SUBJECT_VERB_AGREEMENT_PLURAL`
  rule flipped a correct "explodes" into an incorrect "explode" in
  `jutsu/wind-release-vacuum-barrier` on a prior ingest. Since curated fixes run
  after LanguageTool in `Sweep`, the new curated entry for that row wins
  deterministically on every future ingest even if LanguageTool keeps making
  the same mistake.
- **Left alone, flagged low-confidence, needs a human content call**:
  `jutsu/corruption-temerity`'s "class and class features" (could be a typo for
  "clan and class features," or could be intentional — needs a rules check, not
  a grammar call) and the "an herbal spice" a/an-before-h question (a genuine
  regional-dialect split, not a clear error).

Same status as above applies: implemented in code, not yet applied to
`dist/rules.db` (pending an `n5e-ingest` re-run).

---

## jutsu

- **`jutsu/psionics-fortress`**: "This armor grants you a +1 bonus to your AC.
  **If you was the target of a Genjutsu Attack**, you instead treat this bonus
  as a +2." Subject-verb disagreement — "you was" should be "you were."
- **`jutsu/hidden-weapon`**: "...you make such an attack at advantage, **with an
  +1 bonus** to the attacks critical threat ra[nge]..." — "an" before "+1"
  (pronounced "plus one," a consonant sound) should be "a +1 bonus." Also note
  "the attacks critical threat range" is missing an apostrophe ("attack's"),
  flagged here as low-confidence since it's punctuation rather than a sentence
  structure error.
- **`jutsu/dark-lotus-field`**: "...you must have fertile soil which can
  facilitate **a flowers growth**." Missing apostrophe/wrong word — should read
  "a flower's growth." Also later in the same row: "...must make **a
  Intelligence Saving Throw**..." — should be "an Intelligence Saving Throw."
- **`jutsu/poison-forest`**: "Creatures in the target area must make **a
  Intelligence Saving Throw**..." Same "a/an" error as above, independent
  occurrence.
- **`jutsu/earth-release-jade-rain`** (and several other jutsu rows, see the
  systemic finding above): representative of the "a +N bonuses" pattern.

## bloodline_latents

- **`latent/uzumaki/latent-chakra-reserves-i`**: "...which can only be spent
  **as a Action, action** on your turn, to regain 4 chakra points per cell
  spent." Two issues in one clause: "a Action" should be "an Action," and the
  word "action" appears to be duplicated immediately after (reads like an
  edit that left both the old and corrected word in place).
- 13 additional rows in this table carry the "a +N bonuses" systemic issue
  (see above); representative slugs: `latent/akimichi/latent-food-pill-i`,
  `latent/fuma/latent-immaculate-precision-iiii`, `latent/fuma/latent-immaculate-precision-v`.

## clan_traits

- **`clan/inuzuka` / "Inuzuka Hijutsu"**: "**You Nin-Dog knows** 1 Inuzuka Clan
  D-Rank Jutsu, that you do not need to add to your known jutsu list." "You
  Nin-Dog" doesn't parse as a subject — almost certainly should be "Your
  Nin-Dog knows..."

## clans (overview / description)

- **`clan/genwa`**: "...they can manifest something greater. **A powerful
  nature release transformation known as Data Release, which possesses a
  unique affinity for technology.**" This is a standalone sentence fragment —
  a noun phrase with a relative clause but no main verb. Likely meant to be
  joined to the previous sentence with a comma instead of a period, or to have
  its own subject and verb added (e.g. "This manifests as a powerful nature
  release transformation known as Data Release, which possesses...").
- **`clan/ranton`**: "The Ranton Clan is a clan that hails proudly from the
  Land of Lightning. **Known to work closely with the Raikage, but not limited
  to that one country.**" Sentence fragment — missing subject ("They are
  known to work closely...").
- **`clan/senju`**: "**Even three of its Hokage has been Senju by birth** and
  each one brought about a new era of prosperity for the leaf." Subject-verb
  disagreement — "three of its Hokage" (plural) + "has" should be "have."
- **`clan/shoton`** and **`clan/yoton`** (identical text, apparently
  copy-pasted between the two clan entries): "A Clan without a clear family
  name, but one which has produced **many of prominent shinobi**." Two issues:
  the sentence has no main subject/verb attaching "A Clan..." to anything (a
  fragment, same shape as `clan/genwa` above), and "many of prominent shinobi"
  has an extra "of" — should read "many prominent shinobi."

## backgrounds

- **`background/hard-worker`** (description): "...so you can keep up **you're
  training** on your own time..." — "you're" (contraction of "you are") should
  be "your" (possessive).
- **`background/student`** (description): "You were taken from your home to be
  trained **either by the Master who found you**." "Either" is left dangling
  with no matching "or" clause — reads like a second option was dropped during
  editing (e.g. "...either by the Master who found you, or by an assigned
  instructor").
- **`background/genius`** (equipment_text): "1 Blank Jutsu Scrolls" — number
  disagreement, should be "1 Blank Jutsu Scroll" (singular).

## feats

- **`feat/class/knights-expert-new`**: two occurrences of "than" written as
  "then": "If you roll higher **then** their attack roll AND hit..." and "If
  they roll higher **then** you, you gain the Defeated condition."
- 15 rows in this table carry the "a +N bonuses" systemic issue (see above);
  representative slugs: `feat/namikaze/superior-speed`,
  `feat/uchiha/madaras-hatred`, `feat/yuki/chilling-realization`.

## fighting_stances

- **`stance/polearm-stance`**: "You master the use of weapons with a long
  reach, **capable to striking** enemies at a slight distance from you."
  "Capable to" should be "capable of."
- **`stance/dragon-fist-stance`**: carries the "a +N bonuses" systemic issue
  ("You gain a +2 bonuses to your AC").

## summon_tribe_attacks

- **`summon/spider` / "Webbing"**: "Ranged Weapon Attack: Reach 30ft., On a hit
  target must make a Strength Saving Throw or be" — the row's text ends
  mid-sentence, cut off after "or be." This looks like a genuine truncation
  (the row is presumably meant to say "...or be Restrained," tying into the
  separate `summon/spider` / "Restrained" row that follows it), not a stylistic
  fragment.
- **`summon/dog-wolf` / "Bite"**: "...the target is knocked Prone or drug 5
  feet in any direction **to summon wants**." Reads as "the summon wants" with
  "the" mistyped as "to."

## classes (description / quick_build)

- Systemic comma splice, found in 8 of the 10 class flavor-text intros: each
  ends its "what connects these shinobi" sentence with "...connected by one
  common factor**,** they are masters of..." — a comma splicing two
  independent clauses with no conjunction. The other two classes
  (`class/puppet-master`, `class/cooking-nin`) use a colon in the identical
  construction ("...connected by one common factor**:** they are masters..."),
  which is the version that reads correctly. Affected slugs: `class/genjutsu-specialist`,
  `class/hunter-nin`, `class/intelligence-operative`, `class/medical-nin`,
  `class/ninjutsu-specialist`, `class/scout-nin`, `class/taijutsu-specialist`,
  `class/weapon-specialist`. Suggested fix: change the comma to a colon (or a
  period) in all 8, matching the two that already do this correctly.
- **`class/hunter-nin`** (description): "**With a focused eye on the wanted
  criminal who fled the village.** A masked Hyūga activates his Byakugan..." —
  a prepositional-phrase sentence fragment with no verb, punctuated as its own
  sentence.
- **`class/hunter-nin`** (description), earlier in the same row: "**Slyly and
  gracefully entering unseen, and she raises** from the floor counting the
  seconds in her head..." — the dangling participle "entering unseen" is
  joined to the main clause with "and," which doesn't work grammatically;
  reads correctly only if "and" is dropped ("entering unseen, she raises...").
- **`class/weapon-specialist`** (description): "...allowing them to shift the
  tides of battle with the edge of a blade **of the knock** of a bow, the song
  of a flying shuriken or the lethality of a flying kunai." This is a
  four-item list where the second item is joined with "of" instead of a comma
  — reads like "with [the edge of a blade] of [the knock of a bow]" was meant
  to be two separate list items ("...with the edge of a blade, the knock of a
  bow, the song of a flying shuriken, or the lethality of a flying kunai").
- **`class/scout-nin`** (quick_build): "Second, choose any clan that you find
  the most appealing**,** this class is flexible enough to support them." Comma
  splice — two independent clauses with no conjunction; suggest a semicolon or
  period in place of the comma.

## subclasses

- **`class/ninjutsu-specialist/group/ninjutsu-focus/sanguine-master`**: "The
  Ninjutsu Specialist who chose to specialize in the Hemomantic Traditions,
  **learn** to use the chakra inherent in a creatures blood, even their own,
  and often **look upon** with distrusting eyes." Two issues: subject-verb
  disagreement ("The Ninjutsu Specialist... learn" should be "learns"), and
  "look upon" is left without an object — "look upon" what? Reads like a
  clause was dropped (e.g. "look upon others with distrusting eyes"). Also
  "a creatures blood" is missing an apostrophe ("creature's"), flagged as
  low-confidence since it's punctuation rather than sentence structure.
- **Genjutsu Pledges group** — four rows drop the leading article and use a
  plural verb against the singular noun "Specialist": `class/genjutsu-specialist/group/genjutsu-pledges/layered-reality`
  ("Genjutsu Specialist who pledge themselves..."), `.../misty-distortionist`
  ("Genjutsu Specialist who choose to become Misty Distortionists specialize
  in..."), `.../time-slipper` ("Genjutsu Specialist who choose to become a Time
  Slipper, dedicate themselves..."), `.../siren` ("Genjutsu Specialist who
  choose to become a Siren dedicate themselves..."). Contrast with the other
  three rows in the same group (`beguiler`, `corrupt-thoughts`, `illusionist`),
  which correctly read "**The** Genjutsu Specialist who **chooses**..." This
  looks like the same sentence template was typed by hand each time and lost
  its article/verb agreement on four of the seven rows.

## subclass_features

- **`class/taijutsu-specialist/group/taijutsu-style/stancer`**: "Taijutsu
  Specialists who **take specialize** in multiple stances, are known as
  'Stances'..." "Take specialize" doesn't parse — either "take" or
  "specialize" is an extraneous/leftover word; likely meant to be just "who
  specialize in multiple stances."
- 9 rows in this table carry the "a +N bonuses" systemic issue (see above);
  representative slugs: `class/hunter-nin/group/hunters-creeds/arsenalist/feature/arsenals-proficiency`,
  `class/hunter-nin/group/hunters-creeds/vice-agent/feature/arrogances-influence`.

## class_options / class_option_entries

(These two tables carry duplicate copies of the same option text — one row per
option, one row per option's "entry" — so the same wording error appears twice,
once per table, for each item below.)

- **`class/puppet-master/option/armorers-upgrades/gold-tier`** (class_options)
  and **`.../gold-tier/entry/armored-adventures`** (class_option_entries):
  "You craft **a another** set of Juggernaut Armor for an ally during a full
  rest." Should be "another set," dropping the stray "a."
- **`class/science-nin/option/shinobi-ware-upgrades/shinjutsu`**
  (class_options) and **`.../shinjutsu/entry/sukunahikona`**
  (class_option_entries): "When you are **a allied creature** you can see
  makes a Ranged attack roll." Should be "an allied creature." (This sentence
  also reads awkwardly beyond the article — "When you are a[n] allied creature
  you can see makes a Ranged attack roll" doesn't parse cleanly even after
  fixing the article; flagged as low-confidence beyond the article fix, since
  it may need a larger rewrite the audit can't confidently suggest.)
- 14 rows in `class_options.description` and 9 in `class_option_entries.description`
  carry the "a +N bonuses" systemic issue (see above).

---

## Tables checked with no issues found

`ability_point_costs`, `adversary_freeform_slots`, `adversary_ranks`,
`adversary_role_traits`, `adversary_roles`, `ambition_prompts`,
`armor_properties`, `background_proficiencies`, `class_casting`,
`class_equipment_options`, `class_multiclass_rules`, `class_proficiencies`,
`clan_ability_increases`, `clan_jutsu`, `clan_proficiencies`, `elemental_advantage`,
`equipment` (spot-checked, not exhaustively read), `equipment_armor_properties`,
`equipment_properties`, `jutsu_creation_options` (empty `effect_text` column in
the current database), `jutsu_keywords`, `jutsu_ranks`, `jutsu_upcast_rules`,
`poisons`, `puppet_armor_chassis`, `puppet_armor_chassis_property`,
`puppet_tool_stat_block`, `source_books`, `subclass_groups`,
`summon_tribe_progression`, `summon_tribe_roles`, `text_corrections`,
`titan_unit_card` (aside from the one issue above), `trap_templates`,
`weapon_properties`, `xp_levels`.

`equipment.description` and `class_options`/`class_option_entries` beyond the
items listed above were sampled via automated pattern matches rather than read
in full line by line, given their size (466KB and 288KB of raw prose
respectively) — a further manual read-through of these two tables in
particular would likely turn up more issues than the automated passes caught,
since most of the errors found elsewhere in this audit (comma splices,
dropped words, dangling clauses) don't have a reliable regex signature and
were only caught by eye.
