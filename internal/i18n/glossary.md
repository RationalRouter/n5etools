# Spanish (es) translation glossary

Approved terms for consistent use across all translated content (UI strings and
`content_translations` rows alike). Do not deviate from these without updating this file
and re-approving the change — the whole point of a glossary is that a term is translated
the same way everywhere.

Verified against the official Spanish D&D 5e SRD (srd.nosolorol.com), the FoundryVTT
`dnd5e-es` community translation, and (where noted) the Latin American Spanish dub of
Naruto itself, since several class names are drawn directly from the source anime.

## Core mechanical terms (verified against D&D 5e Spanish sources)

| English | Spanish | Source |
|---|---|---|
| N5etools | N5etools | unchanged, brand/app name |
| Chakra | Chakra | unchanged, already a loanword |
| Jutsu | Jutsu | unchanged, proper term of art |
| Hand seals | Sellos de manos | |
| Saving throw | Tirada de Salvación | FoundryVTT dnd5e-es |
| Ability check | Prueba de Característica | FoundryVTT dnd5e-es |
| Proficiency bonus | Bonificador por Competencia | official SRD 5.1 (srd.nosolorol.com) |
| Advantage | Ventaja | FoundryVTT dnd5e-es |
| Disadvantage | Desventaja | FoundryVTT dnd5e-es |
| Bonus action | Acción Adicional | FoundryVTT dnd5e-es |
| Reaction | Reacción | FoundryVTT dnd5e-es |
| Concentration | Concentración | FoundryVTT dnd5e-es |
| Casting at higher rank ("upcasting") | Lanzado/a a un nivel superior | the official SRD's own "lanzado a un nivel superior", unchanged — see the Rank row below |
| Rank | Nivel | covers jutsu rank (E–S), Shinobi rank, adversary rank and condition rank. **changed 2026-10-08 by direction** from "rango" (jutsu/adversary ranks) and "grado" (condition ranks): "Rango D" → "Nivel D", "1 grado de Sangrado" → "1 nivel de Sangrado". Matches Portuguese ("Nível D") and the official SRD's "nivel de agotamiento". Character/class level is also "nivel"; the rank letter or the number keeps the two apart. Exceptions that stay as-is: "rango de amenaza (crítica)" = critical threat **range**, "rango Bronce" etc. in the UI catalog = upgrade **tier**, and angles ("360 grados"). Applied to all existing es content_translations rows and es.json the same day |
| Ranks | Niveles | plural of the row above |
| Mastery (this game's own stacking rank system) | Maestría | homebrew system, no D&D precedent to check against — approved as our own call |
| Class Feature | Rasgo de Clase | confirmed via the official Spanish SRD mirror's own class pages (e.g. Guerrero) and nivel20.com — NOT "Característica de Clase" (initially guessed wrong): "Característica" is already reserved for ability scores (see "Ability check → Prueba de Característica" above), so reusing it here would collide with that meaning. "Rasgo" (trait) is the correct word |

## Core mechanics canonical renderings (pinned 2026-10-08)

One mechanic, one rendering, in `es.json` and in every es `content_translations` row. Verified
against the official Spanish SRD 5.1 (srd.nosolorol.com) and the FoundryVTT `dnd5e-es`
translation (`lang/es.json`: "Tirada de Salvación", "CD Salvación", "Dados de Golpe", "Puntos
de Golpe" / "PG"). "Saving throw → Tirada de Salvación" is already pinned in the core table
above; the short form "salvación" (column headers, "Wisdom save") follows it.

| English | Spanish | Source |
|---|---|---|
| Save DC | CD de salvación | FoundryVTT dnd5e-es "CD Salvación"; official SRD "CD de la tirada de salvación". Replaces "Salvad a DC", "Salvación: DC =" and "CD de tirada de salvación". Bare "DC" is always "CD" (never "DC" or "CC"), and CD is feminine ("la CD", "tu CD") |
| DC | CD | official SRD 5.1 |
| Death saving throw | tirada de salvación contra la muerte | official SRD 5.1 |
| Hit Die | Dado de Golpe | official SRD 5.1 and FoundryVTT dnd5e-es. Replaces "dado de vida" (79 content rows), which is the pt/2024 wording, not the Spanish 5e term; "Dado de Daño" is a damage die and is never used for this |
| Hit Dice | Dados de Golpe | plural of the row above |
| Hit points | puntos de golpe | official SRD 5.1 and FoundryVTT dnd5e-es; pairs with Dado de Golpe. Replaces "puntos de vida" (~1,320 content rows) |
| HP | PG | abbreviation of the row above; replaces "PV" |
| Chakra Die | Dado de Chakra | homebrew, built on "Dado de Golpe" (same construction); previously left as untranslated "Chakra Die" in `classes.chakra_die` |
| Chakra Dice | Dados de Chakra | plural of the row above |

The Flurry die stays "Dado de Ráfaga" (pinned in the Subclass Features section below).

Applied with `n5e-i18n normalize-terms` (see the Portuguese section's matching heading for
the tool), plus hand fixes for UI labels it does not cover.

### Jutsu cost lines (`jutsu.cost_text`)

`n5e-i18n translate` renders a jutsu's cost line itself instead of sending it to DeepL
(`cmd/n5e-i18n/jutsu_cost.go`), because DeepL mangled the bare lines without context ("Los 5
Chakras", "Chakra de 13", "4. Chakra"). The rendering, for es and pt alike, is "N de Chakra",
"Especial (N de Chakra)", "N de Chakra Estelar", and for es / pt: "o" / "ou", "N Calorías" /
"N Calorias", "N Dado(s) de Golpe" / "N Dado(s) de Vida" (plural by the count, so the book's
"2 Hit Die" becomes plural), "X de Furia" / "X de Fúria". The few sentence-like costs are
listed verbatim in that file. Applied to every existing es/pt row on 2026-10-08.

## UI navigation categories (must match between nav bar and any other reference to the same category)

Pinned as glossary identity/consistency entries because DeepL translated a second,
independent mention of each of these three on the home page differently from the nav bar's
own mention of the same category ("Classes" as "cursos" instead of "Clases", "Feats" as
"hazañas" instead of "Dotes", "Backgrounds" as "fondos" instead of "Trasfondos") — same
English word, two different Spanish outputs depending only on which catalog string it
appeared in.

**Known limitation, confirmed on a later run**: even with these glossary entries in place,
a second catalog-translate run still produced `feats.title` -> "Hazañas" and
`backgrounds.title` -> "Antecedentes" instead of the pinned "Dotes"/"Trasfondos" — DeepL's
glossary matching is not 100% reliable even for a standalone single-word source string
sent as its own catalog entry. Manually spot-check every "Classes"/"Feats"/"Backgrounds"-
equivalent string (and any other single-word nav/category term) against the nav bar's own
translation after every full or partial catalog-translate run — do not assume the glossary
entry alone guarantees consistency.

| English | Spanish | Source |
|---|---|---|
| Classes | Clases | matches nav.classes |
| Feats | Dotes | matches nav.feats |
| Backgrounds | Trasfondos | matches nav.backgrounds |

## Ranks / titles (unchanged — already Japanese loanwords in Spanish fandom usage)

Genin, Chunin, Jonin, Sanin, Kage

## Jutsu rank letters

E, D, C, B, A, S — kept as bare letters, language-neutral, no translation needed.

## Class names

| English | Spanish | Source |
|---|---|---|
| Puppet Master | Titiritero | confirmed dub term for Sasori/Kankuro ("hasta el mejor titiritero tiene su punto débil") — do NOT prefix with "Maestro", the word already means master-of-puppets |
| Medical-Nin | Ninja Médico | confirmed, consistently used for Tsunade/Sakura's medical ninjutsu |
| Hunter-Nin | Ninja Cazador | confirmed via Spanish Narutopedia translation of the Haku/Zabuza arc role |
| Genjutsu Specialist | Especialista en Genjutsu | root term unchanged (spoken as-is in dub), wrapper is game-invented |
| Ninjutsu Specialist | Especialista en Ninjutsu | root term unchanged, wrapper is game-invented |
| Taijutsu Specialist | Especialista en Taijutsu | root term unchanged, wrapper is game-invented |
| Weapon Specialist | Especialista en Armas | no anime precedent — class is original to this game, approved as literal translation |
| Intelligence Operative | Operativo de Inteligencia | no anime precedent — original to this game, approved as literal translation |
| Scout-Nin | Ninja Explorador | no anime precedent — original to this game, approved as literal translation |
| Science-Nin | Ninja Científico | no anime precedent — original to this game, approved as literal translation |
| Cooking-Nin | Ninja Cocinero | no anime precedent — original to this game, approved as literal translation |

## Clan names

All 45 clan names (Aburame, Uchiha, Hyūga, Nara, Uzumaki, etc.) stay untranslated — Japanese
proper nouns, same treatment as rank titles like "Kage." Only a clan's epithet/overview flavor
text gets translated normally; the clan name itself never does.

## Nature-transformation ("X Release") jutsu naming

**READ THIS BEFORE TRANSLATING ANY "X Release" TERM, IN ANY LANGUAGE.** "Release" here has two
completely different, correctly-differentiated translations depending on WHICH kind of "X" it is —
conflating them is a confirmed, twice-occurring bug class (see the two pinned "fixed" sections
below: "'Nature Release' terminology" and "'Elemento X' bug"). Check which category applies BEFORE
picking a term:

| Category | Members | Canonical translation | Do NOT use |
|---|---|---|---|
| The 5 **base elements** + Yin/Yang | Earth, Fire, Water, Wind, Lightning, Yin, Yang Release | **"Descarga de X"** (Spanish)/**"Descarga de/do/da X"** (Portuguese) — see the full per-element table in the "'Elemento X' bug" section below for exact articles/gender | "Elemento X" — this is the specific bug fixed 2026-09-12, do not reintroduce it |
| Kekkei-genkai **combo** types | Crystal, Explosion, Wood, Boil, Steel, Magnet, Plasma, Lava, Storm, Scorch, Ice, etc. — i.e. every OTHER nature-transformation name, always a combination of two base elements | **"Elemento X"** (e.g. Crystal Release → Elemento Cristal, Explosion Release → Elemento Explosivo) | "Descarga de X" — that pattern is reserved for the base elements only |

**Why these look confusable but must not be merged**: both are "X Release" in the English source,
and a base element and a combo type can appear in the very same sentence (e.g. "close affinity to
both Earth and Lightning Release" describing a combo clan's manifestation) — see
`clan/bakuton/feature/explosion-release`, which correctly mixes both forms: "afinidad con la
**descarga de Tierra** y la **descarga del Rayo**... manifestar el **Elemento Explosivo**." The
"Elemento X" bug happened because a translator over-generalized the correct combo-type rule to the
base elements too (see e.g. the now-corrected glossary row for "Yin Release" below, which used to
say "follows the already-approved 'Elemento X' pattern" — that reasoning was the bug itself).

Rationale for **why combo types use "Elemento X"**: matches the Spanish Narutopedia/fan-wiki
convention, used consistently across every kekkei genkai type — notably NOT matching the actual
anime dub, which was inconsistent with itself (Wood Release was rendered "jutsu arbóreo" in Part I
and "jutsu lignario" in Part II of the same show). The fan-wiki term is what Spanish-speaking
players are actually likely to already know from looking up jutsu online, so it's the better
standardized choice despite not being official-dub sourced. Applies to jutsu names across most
kekkei genkai clans (Bakuton, Futton, Shoton, Ranton, Yoton, Jiton, Keton, etc.) — this is a naming
*pattern*, not a one-off term, so apply it consistently every time a genuine combo "X Release"
jutsu name is translated.

Rationale for **why base elements use "Descarga de X," not "Elemento X" or "Liberación"**: see the
two pinned sections lower in this file — "'Nature Release' terminology — 'Liberación'/'Liberação'
replaced by 'Descarga'" (the connotation bug: "Liberación" means freedom/parole, not
unleash-power) and "'Elemento X' bug for the 5 base elements + Yin/Yang" (the noun bug: "Elemento"
is the wrong word class entirely, borrowed from the unrelated combo-type convention above). Between
them, NEITHER "Liberación X" NOR "Elemento X" is ever correct for a base element in any locale —
only "Descarga de X" (or that language's own equivalent verified against raw English source, if
translating into a brand-new language) is.

## Technical note: `components` field is NOT translatable content

The `jutsu.components` column (e.g. `"HS, CM, W"`) is machine-parsed at runtime by
`cmd/n5e/jutsu_filters.go` (`componentCodes`/`componentNames`) to power the jutsu filter chips —
it is not display prose. Translating it per-row in `content_translations` would break filter
parsing across all jutsu. **Do not include `components` in `jutsu`'s translatable-fields list.**
Instead, the 6 fixed abbreviation labels get translated once, through the UI string catalog, not
per-jutsu content translation:

| Code | English | Spanish |
|---|---|---|
| HS | Hand Seals | Sellos de Manos |
| CM | Chakra Molding | Moldeado de Chakra |
| CS | Chakra Seals | Sellos de Chakra |
| M | Mobility | Movilidad |
| W | Weapon | Arma |
| NT | Ninja Tools | Herramientas Ninja |

Decision: translate these labels (not left English-only) — anything a player can potentially
see gets translated, per standing project policy (confirmed 2026-09-04). Only the Go code itself
(identifiers, map keys `HS`/`CM`/etc., comments) stays English forever; that's ordinary source
code, not player-facing content, and is out of scope for translation entirely — see the "why
source code stays English" note in `translation_methodology.md`.

## Puppet Master "Technique" naming (subclass groups)

"Technique" as a capitalized, formal in-game term — referring to the named Puppet Master
subclass groups (e.g. Purple Technique, Perfect Technique) — translates as **"Técnica"**,
capitalized the same way whenever it refers to one of these named disciplines (e.g. "Técnica
Púrpura," "tu Técnica de Titiritero elegida"). Confirmed while reviewing the "Puppeteer
Enthusiast" feat pilot translation (2026-09-04). Lowercase "technique" used generically
(not naming a specific subclass) does not need this capitalization.

## Puppet Master upgrade tiers

Direct material-word translations, no ambiguity:

| English | Spanish |
|---|---|
| Wood (tier) | Madera |
| Bronze (tier) | Bronce |
| Silver (tier) | Plata |
| Gold (tier) | Oro |
| Platinum (tier) | Platino |

## Clan/lineage terms surfaced by the Clans pilot batch (Hyūga, Nara, Inuzuka)

| English | Spanish | Source |
|---|---|---|
| Byakugan | Byakugan | unchanged — proper noun/kekkei genkai name, same treatment as clan names |
| Tenketsu | Tenketsu | unchanged — Japanese term used as-is in the show itself, same treatment as chakra/jutsu |
| Ōtsutsuki | Ōtsutsuki | unchanged — proper noun (clan/lineage name) |
| Yin Release | descarga Yin | **corrected 2026-09-12** — this row previously said "Elemento Yin," justified as "follows the already-approved 'Elemento X' nature-transformation pattern"; that reasoning was itself the bug (Yin is a base element, not a kekkei-genkai combo type — see the "READ THIS BEFORE TRANSLATING..." box in the "Nature-transformation" section above). Direct apposition, no preposition — Portuguese's own precedent ("Sua Descarga Yin atingiu...") confirmed this shape before applying it to Spanish. Kept in sync with (same value as) the full base-element+Yin/Yang canonical table in the "'Elemento X' bug" section — that table is the source of truth, this row exists for DeepL glossary coverage in the Spanish-only tables scanned earlier in the file |
| Yang Release | descarga Yang | same correction and rationale as Yin Release above |
| Main House / Branch House (Hyūga) | **Rama Principal / Rama Secundaria** | confirmed via Spanish Narutopedia (Sōke/Bunke) — "Rama" (branch) preferred over "Casa" (house): more literal, and also the natural word for a genealogical branch in Spanish, so it reads correctly in both the physical clan-structure sense and the family-tree sense at once |
| Ino–Shika–Chō | Ino–Shika–Chō | unchanged — canonical trio name, kept untranslated in Spanish fandom same as clan names |

## Mechanical terms surfaced by the Class Features pilot batch (Taijutsu/Ninjutsu Specialist)

| English | Spanish | Source |
|---|---|---|
| Short Rest | Descanso Corto | confirmed official D&D 5e Spanish SRD term |
| Long Rest | Descanso Largo | confirmed official D&D 5e Spanish SRD term |
| Full Rest (this game's own superset of Long Rest) | Descanso Completo | homebrew, no D&D precedent — natural extension of the Corto/Largo pattern |
| Armor Seal (minor/refined/greater/superior tiers) | Sello de Armadura (menor/refinado/mayor/superior) | literal, no ambiguity |
| Martial Dice / Martial Die | Dado(s) Marcial(es) | literal |
| Martial Technique (Taijutsu Specialist's maneuver list) | **Técnica Marcial** | confirmed — kept distinct in *concept* from Puppet Master's "Técnica" (a subclass identity) despite the shared word; the two systems live on different classes and won't be confused by a player reading either sheet |
| Efficient Molding | Moldeado Eficiente | consistent with existing glossary term "Chakra Molding" → "Moldeado de Chakra" |
| Combination (jutsu keyword) | Combinación | |

## Policy: source-content wording inconsistencies are not a translation problem to fix

Found while translating Ninjutsu Specialist's class features: the subclass-group is officially
named "Ninjutsu Focus" in the data (`subclass_groups` table), but the class's own flavor text
calls the same concept a "Tradition" ("you begin to Specialize in a Tradition") — an inconsistency
in the original English sourcebook text itself, not something translation should try to reconcile.
**Confirmed policy: translate whatever the player-facing English text actually says, as written,
even when it's internally inconsistent with itself elsewhere.** Don't silently "fix" or normalize
wording to match a different field's terminology — that's a content-editing decision for the
sourcebook/ingest side of the project, out of scope here. "Tradition" in this spot translates
generically as **"Tradición."**

## Technical note: `armor_properties` / `weapon_properties` ARE normal translatable content

Unlike `jutsu.components` (a parsed-code field, see above), `armor_properties` and
`weapon_properties` (10 and 27 rows) are genuine structured reference-catalog tables —
`cmd/n5e/properties.go` and `internal/features/choices.go` join against them by row (a feature
"grants an Armor Property" via a slug/junction-table reference), not by regex-matching English
words out of free prose. **Safe to translate normally** as ordinary `content_translations` rows,
same treatment as clan/feat text.

## Armor properties (10, all straightforward)

| English | Spanish |
|---|---|
| Bulky | Voluminoso |
| Bulwark | Baluarte |
| Camouflage | Camuflaje |
| Fashionable | Elegante |
| Fortified | Fortificado |
| Heavyweight | Pesado |
| High Quality | Alta Calidad |
| Lightweight | Ligero |
| Reinforced | Reforzado |
| Threatening | Amenazante |

## Weapon properties (27)

| English | Spanish | Source |
|---|---|---|
| Finesse | Finura | official SRD uses "Sutileza," but rejected here — this property is about choosing STR or DEX for weapon rolls, and "Finura" reads correctly as finesse/refinement/flexibility, whereas "Sutileza" reads as "subtlety/stealth," the wrong sense entirely |
| Reach | Alcance | confirmed official D&D 5e Spanish SRD term |
| Thrown | Arrojadiza | confirmed official D&D 5e Spanish SRD term |
| Versatile | Versátil | confirmed official D&D 5e Spanish SRD term |
| Two-Handed | A Dos Manos | confirmed official D&D 5e Spanish SRD term |
| Light | Ligera | confirmed official D&D 5e Spanish SRD term |
| Heavy | Pesada | confirmed official D&D 5e Spanish SRD term |
| Ammunition | Munición | straightforward |
| Critical | Crítico | straightforward |
| Deadly | Mortal | straightforward |
| Grapple | Agarre | straightforward |
| Loading | Carga | straightforward |
| Special | Especial | straightforward |
| Unarmed | Desarmado | straightforward |
| Volatile | Volátil | straightforward |
| **Range** | **Distancia** | this game's own addition (not standard D&D) — deliberately NOT "Alcance" to avoid colliding with "Reach," which already owns that word officially |
| Tactical | Táctica | homebrew (not standard D&D), approved as our own call |
| Trip | Tropezar | homebrew (not standard D&D), approved as our own call |
| Multiattack | Ataque Múltiple | homebrew (not standard D&D), approved as our own call |
| Blocking | Bloqueo | homebrew, straightforward |
| Disarm | Desarmar | homebrew, straightforward |
| Evocation | Evocación | homebrew, straightforward |
| Flexible | Flexible | homebrew, straightforward |
| Hidden | Oculta | homebrew, straightforward |
| Lethal | Letal | homebrew, straightforward |
| Returning | Que Regresa | homebrew, straightforward |
| **Winding** | **Giro** | rules text: "can be wound up to store energy before unleashing it in one blow... the next weapon attack you make with this weapon has advantage." Confirmed by the weapon list it applies to (Kusarigama, Nunchaku, Sansetsukon, Urumi, Whip) — all flexible/chain-type weapons, so this is about coiling tension into the weapon, not a reload/crank mechanism |

## Enhancement seal tiers

Adds a 5th tier beyond the four already covered under "Puppet Master upgrade tiers"/"Armor Seal":

| English | Spanish | Note |
|---|---|---|
| Mastercraft | Obra Maestra | deliberately NOT "Maestría" — avoids colliding with the already-reserved Mastery-system term |

(Minor/Refined/Greater/Superior already covered under the Armor Seal entry above.)

## Backgrounds

| English | Spanish | Source |
|---|---|---|
| Hermit | Ermitaño | confirmed against the official Spanish PHB background list |
| Entertainer | Artista | confirmed against the official Spanish PHB background list |
| Noble | Noble | confirmed, unchanged |
| **Urchin** | **Golfillo** | *unconfirmed* — no official Spanish precedent found (not one of the 13 core PHB backgrounds); best guess, flagged |
| Genius | Genio | original to this game, no D&D precedent |
| Hard Worker | Trabajador | original to this game, no D&D precedent |
| Leader | Líder | original to this game, no D&D precedent |
| Student | Estudiante | original to this game, no D&D precedent |
| Traveler | Viajero | original to this game, no D&D precedent |
| Trouble Maker | Alborotador | original to this game, no D&D precedent |

## Equipment terms surfaced by the Equipment pilot batch

| English | Spanish | Source |
|---|---|---|
| Bulk (this game's own carrying-capacity unit) | Volumen | homebrew, no D&D precedent — this game replaces weight-in-pounds with its own Bulk system |

Proper nouns from specific class features (e.g. "Storm Rider," "Air Trecks") stay untranslated,
same treatment as clan/class names — pending their own glossary entry once Subclass Features gets
piloted.

## Technical note: `equipment.properties` and `damage_type` are NOT translatable content

Same shape as `jutsu.components` above. `equipment.properties` (e.g. `"Light, Finesse, Thrown
(30/60), Multiattack, Ammunition"`) is comma-separated free text copied verbatim from the source
sheet, but `internal/parse/weaponprops.go`'s `ParseEquipmentProperties` parses it at ingest into
normalized `equipment_properties`/`equipment_armor_properties` junction rows — that's what actually
drives the Items page's property tooltips, not the raw string. **Do not include `properties` in
`equipment`'s translatable-fields list**; the display already resolves through
`weapon_properties`/`armor_properties`, both already translated above.

`damage_type` (Piercing, Slashing, Bludgeoning, Fire, Cold, Lightning, Slashing/Piercing, Variable —
8 distinct values total, shared across every weapon row) is the same shape for a different reason:
a small shared enum, not per-row prose. Translate it once through the UI string catalog, not as a
per-equipment-row `content_translations` entry:

| English | Spanish | Portuguese |
|---|---|---|
| Bludgeoning | Contundente | Contundente |
| Piercing | Perforante | Perfurante |
| Slashing | Cortante | Cortante |
| Fire | Fuego | Fogo |
| Cold | Frío | Frio |
| Lightning | Rayo | Relâmpago |
| Variable | Variable | Variável |

("Slashing/Piercing" is a display-time join of the two rows above, not its own label.)

**The Portuguese column was missing entirely until 2026-09-13, and the three physical types had
drifted because of it** — nothing pinned them, so `damage_type.bludgeoning` was "Espancamento"
("a beating," the act of assaulting someone, not a damage class), `damage_type.piercing` had never
been translated at all and rendered the English "Piercing", and `damage_type.slashing` was the
noun "Corte" ("a cut") where the enum wants an adjective. Corrected to the official PT-BR D&D 5e
terms — which is also what this project's own 22,528 translated rows already overwhelmingly use in
prose: `dano contundente` 141, `dano perfurante` 113, `dano cortante` 60, against **zero** uses of
"espancamento" or "corte" as a damage type. The corpus had been right and the enum wrong.

Fixed alongside them: `sheet.slashing_placeholder` and the three
`full_metal_shinobi_passive.{bludgeoning,piercing,slashing}` labels, which are the other keys
carrying these same three words. Safe to change without touching behavior — `items.go`'s
`damageTypeCatalogKey` maps raw **English** to the catalog key, so these values are display-only
and are never matched against.

**General lesson**: a locale column missing from a glossary table is invisible in every report —
`verify` can only check terms it can parse a target rendering for, so an absent column reads as
"nothing to check" rather than "unpinned." When adding a locale, diff the column set across every
table in this file, don't assume the tables are uniform.

### Closed 2026-09-13: the 23 Portuguese rows using noun forms of the physical damage types

Adding the Portuguese column above made `verify` check these three terms for the first time, which
took Portuguese from 49 findings to 99 — newly-visible pre-existing drift, not new breakage. The
~31 findings across **23 rows** were prose using the noun form where the enum uses the adjective
("danos por contusão, perfuração e corte"), bullet labels ("• Contusão:", "• Corte:"), damage
expressions ("1d6 de Contusão"), and rows already mixing both forms in one sentence
("(Contusão, Perfurante, Cortante)").

**Fixed by hand, one occurrence at a time — and `normalize-glossary` still must not be extended to
cover this.** The reason the hand-pass was necessary is the whole point: "Contusão" is ambiguous
in this corpus. In "ela ganha 1 grau de Contusão" it is the **Bruised condition**, and the correct
fix there is "1 grau de Contundido" (the condition's own name), not "Contundente" (the damage
type). A blanket noun→adjective rule would have turned every one of those into nonsense. Same
hazard class as the "Raio" tail: which rendering is right depends on which mechanic the sentence
is about, which is exactly what a token-level rule cannot see.

Representative corrections, all verified against the English row:

| Portuguese as written | Corrected to | Why |
|---|---|---|
| "Se o alvo estiver Contusão" | "Se o alvo estiver Contundido" | ungrammatical; EN is "If the target is Bruised" — the condition |
| "ela ganha 1 grau de Contusão" | "...de Contundido" | the condition, not the damage type |
| "fica Contusada" | "fica Contundida" | "Contusada" is not a word; condition name, feminine agreement |
| "(Contusão/Terra = Contusão, Corte = Sangramento, Perfuração = Enfraquecimento)" | "(Contundente/Terra = Contundido, Cortante = Sangramento, Perfurante = Enfraquecido)" | one side of each pair is a damage type, the other a condition — the original conflated them |
| "danos por contusão, perfuração e corte" | "dano contundente, perfurante e cortante" | noun forms where the enum uses adjectives |
| "danos contusos" | "dano contundente" | "contuso" means bruised, not the damage type |
| "1d4+1 de dano de Corte" | "...de dano cortante" | same |

After this pass and the Lightning normalization below, **Portuguese and Spanish both report zero
`missing-glossary-term` findings.**

### Closed 2026-09-13: the Lightning "Raio"/"elétrico" family

Previously left alone as an accepted exception. Reopened and fixed under the "correct, not
understandable" standard (see translation_methodology.md), because it was not one alternative
rendering but **three families across ~75 rows** — "dano de Raio" / "dano de raio" / "Dano de
Raio" / "dano por Raio" / "danos de Raio" for the damage type, "descarga de raios" / "descarga de
raio" / "Descarga do Raio" / "Descarga de Raios" for the element, and a separate "dano elétrico"
family in 22 more rows.

There is also a concrete readability defect, not just inconsistency: **"raio" is also the
Portuguese word for *radius***. "dano de Raio em um raio de 15 pés" is genuinely ambiguous on the
page, which is why the glossary pinned "Relâmpago" for the mechanic in the first place.

Normalized to `dano de Relâmpago` and `Descarga do Relâmpago`. The rewrite was restricted to
occurrences carrying a lightning-marking head word (`dano`/`danos`/`descarga`), so bare "raio"
meaning radius was never touched; the "dano elétrico" rows were each confirmed against their
English saying "Lightning damage" (as opposed to the Science-Nin "Electric"/"Electro" upgrades,
which legitimately keep "elétrico"). The country name keeps "Raio" — see "Country names built on
an element word".

**Standing rule, confirmed 2026-09-04 as a hard policy (not re-confirmed per instance going
forward): anything the player can potentially see gets translated.** This is the same policy
already applied to the `jutsu.components` labels — every time a new machine-parsed field turns up
(a fixed, small set of codes/enums mapped to display labels), the labels themselves are always
in scope for translation via the UI catalog; only the raw parsed data column itself stays
untranslated. Apply this by default from now on without asking again.

## Adversary / Encounter Generator terms (Bingo Book, Pack 1, Adversary Builder sheet)

Added 2026-10-08, before the first DeepL pass over the Bingo Book NPC templates, the Pack 1
enrichment tables, and the community Adversary Builder sheet. The earlier `adversary_roles` pass ran
without a Portuguese column and rendered the roles as sports positions: es "Delantero"/"Defensa"/
"Simpatizante", pt "Zagueiro"/"Apoiante". Those rows were corrected to the values below in the same
pass. Rank names match the UI catalog's `npc_templates.class_*` keys.

| English | Spanish | Portuguese | Note |
|---|---|---|---|
| Striker | Atacante | Atacante | adversary role; D&D 4e Devir Spanish |
| Defender | Defensor | Defensor | adversary role; D&D 4e Devir Spanish |
| Controller | Controlador | Controlador | adversary role |
| Lurker | Acechador | Espreitador | adversary role; homebrew call, no 4e citation |
| Supporter | Apoyo | Apoiador | adversary role; homebrew call (pt matches the existing role description's own usage) |
| Minion | Esbirro | Lacaio | adversary rank |
| Elite | Élite | Elite | adversary rank |
| Solo | Solitario | Solo | adversary rank; pt keeps the loanword, as the UI catalog already does |
| Tenacity | Tenacidad | Tenacidade | adversary resource |
| Optional Classification | Clasificación Opcional | Classificação Opcional | |
| Iconic | Icónico | Icônico | classification tier |
| Epic | Épico | Épico | classification tier |
| Battlefield Domain | Dominio del Campo de Batalla | Domínio do Campo de Batalha | |
| Elite Action | Acción de Élite | Ação de Elite | |
| Specialist Jutsu | Jutsu de Especialista | Jutsu de Especialista | |
| General Traits | Rasgos Generales | Traços Gerais | Bingo Book section |
| Adversary | Adversario | Adversário | |
| GM | DJ | Mestre | matches existing UI-catalog usage |
| Melee Weapon Attack | Ataque con arma cuerpo a cuerpo | Ataque Corpo a Corpo com Arma | stat-block attack line |
| Ranged Weapon Attack | Ataque con arma a distancia | Ataque à Distância com Arma | stat-block attack line |
| +N to hit | +N a impactar | +N para atingir | stat-block attack line |
| Hit: (damage) | Impacto: | Acerto: | stat-block attack line |

The stat-block attack lines follow the published 5e (2014) stat-block wording. The Spanish
"Impacto" is confirmed by the official Spanish SRD's monster-reading guide
(srd.nosolorol.com/DD5/aventuras/monstruos.html). "a impactar" and the Portuguese
(Galápagos-style) phrasing were not re-verified online on 2026-10-08. The SRD site's stat-block
pages render client-side and couldn't be fetched.

DeepL's raw output varied across the first pass ("Si acierta", "al golpear", "Ataque com arma
branca", "de longo alcance", "Acertando"). It was normalized to the table above after
translation. Expect the same drift on future runs and re-check these lines.

## Combat actions (verified against official Spanish SRD)

| English | Spanish | Source |
|---|---|---|
| Dash | Esprintar | official Spanish SRD (srd.nosolorol.com) — not the guessable literal "Correr" |
| Disengage | Retirarse | official Spanish SRD (srd.nosolorol.com) |

## Terms surfaced by the Subclass Features pilot batch (Storm Rider, Samurai Form, Interrogationist)

| English | Spanish | Source |
|---|---|---|
| Weapon Focus | Enfoque de Arma | homebrew, literal |
| Flurry die | Dado de Ráfaga | homebrew, literal |
| Keyword (Genjutsu tag, e.g. Visual/Auditory) | Palabra Clave | homebrew, literal |
| Visual (keyword) | Visual | unchanged |
| Auditory (keyword) | Auditivo | homebrew, literal |
| Brave Order (Intelligence Operative resource) | Orden Valiente | confirmed — this is the actual in-game resource name |
| Power (weapon group) | Poder | homebrew — only one weapon-group name surfaced this batch; full weapon-group list needs its own pass once Weapon Specialist's core class is piloted |
| Bukijutsu | Bukijutsu | unchanged, proper term of art, same treatment as Taijutsu/Ninjutsu/Genjutsu |
| Forensics Kit(s) | Herramientas Forenses | homebrew tool proficiency, no real D&D equivalent — matches the official SRD's "Herramientas de X" pattern for real tool proficiencies (e.g. Herramientas de Ladrón) |
| Weapon Forms (subclass group, Weapon Specialist) | Formas de Arma | literal |
| Master Strategies (subclass group, Intelligence Operative) | Estrategias Maestras | literal |
| CCD ("Chakra Containment Device," Science-Nin) | DCC ("Dispositivo de Contención de Chakra") | translated, not left as the English acronym — CCD appears repeatedly across multiple Science-Nin subclasses (Scientific Ninja Beast, Full-Metal Shinobi, In His Image, Ghost in the Shell, Cruel Angel's Thesis) as a real named mechanic (a chakra-pool resource), not incidental flavor text, so it gets the same treatment as any other spelled-out game term. Apply consistently: "5 CCD Chakra" → "5 Chakra de DCC", "your CCD" → "tu DCC", "CCD chakra cost" → "costo de Chakra del DCC" |
| Scientific Ninja Tool(s) | Herramienta(s) Ninja Científica(s) | homebrew, literal |

### Worked example: Chakra Containment Device (Science-Nin base class feature, Level 2)

Approved translation of the actual feature that defines CCD, confirming the terms above apply
correctly in real running text (not just in isolation):

> **EN**: Starting at Level 2 You learn how to create a Chakra Containment Device. This device
> allows you to store chakra to power your Scientific Ninja Tools. This Device can either be
> attached to a weapon or your armor and can be switched to either as a Bonus Action. When you
> take a short rest and spend Chakra Dice, you can choose to seal the rolled amount into the CCD.
> The CCD can hold a number of chakras equal to your Science-Nin Level x 15. This chakra can only
> be used to power a Scientific Ninja Tool or Science-Nin Class feature. This device charges to
> half-full during a Long rest and to full on a Full Rest.
>
> **ES**: A partir del nivel 2, aprendes a crear un Dispositivo de Contención de Chakra (DCC). Este
> dispositivo te permite almacenar chakra para alimentar tus Herramientas Ninja Científicas. El
> Dispositivo puede estar conectado a un arma o a tu armadura, y puedes cambiarlo de una a otra
> como Acción Adicional. Cuando tomas un Descanso Corto y gastas Dados de Chakra, puedes optar por
> sellar la cantidad obtenida dentro del DCC. El DCC puede almacenar una cantidad de chakra igual a
> tu nivel de Ninja Científico x 15. Este chakra solo puede usarse para alimentar una Herramienta
> Ninja Científica o un rasgo de clase de Ninja Científico. Este dispositivo se carga hasta la
> mitad durante un Descanso Largo y se carga por completo en un Descanso Completo.

Feature name: "Chakra Containment Device" → **"Dispositivo de Contención de Chakra"**. Note the
inline class-name reference ("Science-Nin Level"/"Science-Nin Class feature") is translated using
the already-approved class-name entry (Science-Nin → Ninja Científico) rather than left English —
apply this same substitution whenever a class's own name is referenced generically in running text.
| "Genjutsu Break" (jutsu name referenced in prose) | Ruptura de Genjutsu | placeholder — this is an actual jutsu name that gets its own official translation once jutsu bulk-translation runs; any prose reference to it must be kept in sync with that name at that time |

## Conditions

| English | Spanish | Note |
|---|---|---|
| Fear (condition) | Asustado | standard D&D 5e Spanish condition name; approved on the strength of how standard/well-known it is, not independently re-verified this session |
| Charmed | Hechizado | confirmed official Spanish SRD conditions page (srd.nosolorol.com/DD5/reglas/estados.html) — NOT "Encantado," which would have been a wrong guess |
| Unconscious | Inconsciente | confirmed official Spanish SRD conditions page |
| Poisoned (standard D&D condition) | Envenenado | confirmed official Spanish SRD conditions page — this ruleset uses both this AND its own homebrew "Envenomed" condition (see below); do not conflate the two |
| Envenomed (this game's own stacking-rank condition, distinct from Poisoned) | **Intoxicado** | homebrew, our own call — deliberately a different word from "Envenenado" specifically to avoid colliding with the real Poisoned condition above, since the two conditions coexist in this ruleset's own text (`cmd/n5e/passive_traits.go`'s "saving throws against being Envenomed or Poisoned") |

## Terms surfaced by the Traps & Poisons pilot batch

| English | Spanish | Source |
|---|---|---|
| Poison Kit | Kit de Envenenador | confirmed — matches the real D&D "Poisoner's Kit" official term, same tool |
| Trapper's Kit | Kit de Trampero | homebrew, no D&D equivalent — literal, mirrors the "Kit de Envenenador" pattern |
| Moulding Mushroom (poison ingredient/name) | Champiñón Mohoso | homebrew ingredient name |
| Devil's Kiss (poison name) | Beso del Diablo | homebrew, literal |
| Assassin's Blood (poison name) | Sangre del Asesino | homebrew, literal |

### Worked examples (approved)

> **Poisonous Trap**: Las Etiquetas de Veneno preparadas detonan cuando una criatura entra en el
> espacio de 5 pies de la trampa o se activa manualmente. Las criaturas dentro de 10 pies quedan
> Intoxicadas si fallan la salvación, con la mitad del efecto si tienen éxito.
>
> **Poisonous Lock**: Una cerradura de resorte con una púa Intoxicante cerca del ojo de la
> cerradura; rompiendo la cerradura no la desactiva. Intentando abrirla o forzarla sin la llave
> correcta activa un pinchazo, obligando a una salvación para evitar quedar Intoxicado durante 1
> hora (3 niveles de Intoxicado si se falla por 5 o más).
>
> **Hidden Pit Trap**: Una trampilla sobre un pozo cuadrado de 10 pies, de 30 pies de profundidad,
> con 5 pies de agua en el fondo. Una criatura que camine sobre ella cae y recibe daño por caída
> (salvación para evitarlo). Puede reiniciarse volviendo a encajar la trampilla.
>
> **Ether**: Se usa en espionaje y manipulación política más que en combate. Una criatura sometida
> a este veneno debe superar una salvación de Constitución CD 16 o queda Hechizada por la primera
> criatura que vea que le dé una orden, durante 24 horas.
>
> **Assassin's Blood**: Una criatura sometida a este veneno debe superar una salvación de
> Constitución CD 12; si falla, recibe 2d6 de daño de veneno y queda Intoxicada durante 24 horas, o
> recibe la mitad del daño y no sufre ningún efecto si tiene éxito.
>
> **Angel's Breath**: Una mezcla de Beso del Diablo, Champiñón Mohoso y Sangre del Asesino. Una
> criatura sometida a este veneno debe superar una salvación de Constitución CD 20 o queda
> Inconsciente durante 96 horas (4 días) si falla.

Grammar note for future gerund use in descriptive present-tense clauses (confirmed 2026-09-04):
Spanish gerunds follow -ar → -ando, -er/-ir → -iendo (romper → rompiendo, intentar → intentando) —
apply this whenever translating an English "-ing" clause describing an ongoing action causing an
effect, not just a static description.

## Terms surfaced by the Summon Tribes / Puppet Stat Blocks / Adversaries / Class Mechanics / Clan Mechanics pilot batch

Sourcing note: `adversary_roles`/`adversary_role_traits` come from `book/bingo-book` ("Bingo Book
Pack 1 — Adversaries," page 19) — a separate sourcebook from the core rulebook, which is why these
terms hadn't surfaced in any earlier pilot batch covering core-book content.

**Known source-text quality issue, confirmed 2026-09-04**: the English source prose itself
contains real grammatical errors (dangling clauses, comma splices, missing verbs) independent of
translation — found live in the Snake summon tribe's description (a trailing "...with its sharp
fangs to pierce..." clause has no verb). This is NOT a translation problem to silently fix (same
policy as the "Tradition"/"Ninjutsu Focus" inconsistency, above) — it's flagged for the sourcebook/
ingest side to actually correct in English, and the translation should follow whatever the
corrected English ends up being. A background audit for this class of issue is in progress (see
below); results land in a separate report before this becomes a live editing task.

| English | Spanish | Source/note |
|---|---|---|
| Striker (adversary role) | Atacante | D&D 4e official Devir Spanish translation |
| Defender (adversary role) | Defensor | D&D 4e official Devir Spanish translation |
| Controller (adversary role) | Controlador | D&D 4e official Devir Spanish translation |
| Lurker (adversary role) | Acechador | no exact 4e-monster-role citation found; homebrew judgment call |
| Supporter (adversary role) | Apoyo | homebrew, no precedent — 4e's equivalent role was "Líder," a different name |
| Powerful Build | Complexión Poderosa | attested D&D 5e Spanish trait name (Goliath/Firbolg) |
| Greataxe | **Hacha de Guerra** | corrected — NOT "Hacha Grande," which was wrong |
| Puppet (noun, the object) | Marioneta | distinct from "Titiritero" (the puppeteer/class name) |
| Upgrade (Puppet Master resource) | Mejora | homebrew, literal |
| Exhaustion | Agotamiento | confirmed official Spanish SRD conditions page |
| Clone(s) | Clon(es) | literal |
| Burned (homebrew stacking condition) | Quemado | homebrew, our own call — no collision with existing terms |
| Creation Points / Program (Intelligence Operative resources) | Puntos de Creación / Programa | homebrew, literal |
| Whelp (Science-Nin's companion) | Cría | literal |
| Draconic Gauntlet (item name) | Guantelete Dracónico | literal |
| Summon / Summoner | Invocación / Invocador | straightforward |
| Aether (Science-Nin ability prefix: Aether Fireball/Breath/Meteor/Connector) | **Éter** | translated after all, per direction — same word as the poison "Ether" → "Éter." The naming overlap (Ether the poison vs. Éter the Science-Nin tech prefix) already exists in the English source itself (Ether/Aether are the same word, differently spelled); translating both to "Éter" doesn't introduce a new ambiguity, it just carries the existing one over |
| Ninshou (Ninjutsu Specialist multiclass proficiency) | *deferred* | confirmed homebrew, but no translation decided yet — left as-is in Spanish text for now, revisit later rather than guess |
| "Fire Storm" (jutsu name referenced in the Whelp's Aether Meteor attack) | Tormenta de Fuego | placeholder — actual jutsu name, needs to match its own translation once jutsu bulk-translation runs, same caveat as "Genjutsu Break" |

### Worked examples (approved, with corrections applied)

> **Snake** (summon tribe, corrected source text): Hay pocas criaturas terrestres más peligrosas,
> astutas y poderosas que una serpiente. La serpiente es una bestia cuyos vínculos e historias se
> remontan a antes de que se llevara la cuenta del tiempo. La mayor arma de la serpiente es su
> astucia y su poderoso cuerpo. Sus afilados colmillos están usados para perforar la carne de sus
> enemigos.
>
> **Tag-Team** (summon tribe feature, Monkey/Primate): Trabajo en Equipo (característica de mono).
> Dos veces por descanso, cuando esta invocación consigue un impacto con un ataque cuerpo a cuerpo,
> su invocador puede gastar su Reacción para realizar la Acción de Ataque, o lanzar un Taijutsu
> dirigido a la misma criatura.
>
> **Inu Distraction** (summon tribe feature, Dog/Wolf): Distracción Inu (característica de
> perro/lobo). Como acción, mientras esté dentro de 5 pies de una criatura hostil, esta invocación
> puede distraer activamente a la criatura objetivo, otorgando a su invocador ventaja en el próximo
> ataque que realice contra el objetivo.
>
> **Bola de Fuego de Éter** (Costo: 5 de Chakra). Ataque de Arma a Distancia: Alcance 90 pies, un
> objetivo. Impacto: 4d10+4 de daño de fuego. Con una tirada de 17 a 20, el objetivo obtiene 1 nivel
> de Quemado.
>
> **Aliento de Éter** (Costo: 10 de Chakra). La Cría exhala un gran cono de fuego alquímico. Ataque
> de Arma de Área: tirada de salvación de Destreza, alcance de cono de 15 pies, todos los objetivos.
> Éxito: mitad de daño, sin efectos. Fallo: 5d6+5 de daño de fuego, Quemado. La Cría puede pagar
> incrementos de 10 de chakra para aumentar el daño en 2d6+2 y el área del cono en +10 pies, hasta
> un total de 40 de chakra.
>
> **Meteoro de Éter** (Costo: 25 de Chakra) [Recarga 5-6]. La Cría exhala un sigilo dracónico de
> llamas sobre el suelo, en un espacio dentro de 120 pies. Grandes bolas de fuego alquímico
> comienzan a manifestarse en el cielo, tratando el área como si estuviera siendo el objetivo del
> Ninjutsu de Fuego Tormenta de Fuego de Nivel A.

# Portuguese (pt) translation glossary

Started 2026-09-07, after the first-pass DeepL Pro pipeline run for Portuguese (see
[[project_i18n_deepl_pivot_status]]). Unlike the Spanish glossary above, this section is
**not** a mirror of every Spanish table — it starts empty and grows the same way Spanish's
did: from real mistranslations found by diffing translated output against the English
source, not speculative upfront entries. `glossaryColumn["pt"] = "Portuguese"` in
`cmd/n5e-i18n/main.go` already picks up any table below with an "English"/"Portuguese"
column pair — no code change needed to add more.

**Important finding from the first audit pass**: of the ~74 jutsu `name` fields that came
back from DeepL identical to their English source, comparing each one against Spanish's own
translation of the *same* field showed most of them are **not** Portuguese-specific bugs —
Spanish left the same short, stylized move titles untranslated too (e.g. "Shatter", "Flash",
"Bane", "Eclipse", "Juggernaut", "Grand Slam", "Buzzsaw Blitz", "Dancing Fool Twister").
That's consistent behavior across both locales for this category of name, not a Portuguese
regression — treat single/short stylized English jutsu titles as intentionally untranslated
unless a specific one is confirmed wrong. Only the entries below, where Spanish *did*
produce a real translation for the same field (proving DeepL was capable of it) while
Portuguese returned the raw English, are treated as genuine gaps.

## Core mechanical terms

| English | Portuguese | Source |
|---|---|---|
| Class | Classe | RPG character-class sense. Confirmed bug: DeepL non-deterministically mistranslated this as "aula"/"turma" (school-class sense) in ~17 UI catalog keys — hand-patched in `internal/i18n/locales/pt.json`, see [[project_i18n_deepl_pivot_status]] |
| Classes | Classes | RPG character-class sense, plural. Same bug/fix as above |
| Action | Ação | universal D&D-style action-economy term; ES already pins this under its own "Core mechanical terms" — PT had never pinned the bare (unprefixed) form, only ever seeing it as part of a full `"N Action"` casting-time string. Corroborated by the fan's independently-reviewed PT-BR spreadsheet (2026-09-28) |
| Reaction | Reação | same gap/reasoning as Action above; ES's own "Core mechanical terms" table already has this pinned |
| Bonus action | Ação Bônus | same gap/reasoning; matches the casing/word order already correctly produced for the prefixed `"N Bonus Action"` casting-time strings in this project's own PT content |
| Touch | Toque | jutsu/weapon range value; not previously pinned for PT |
| Permanent | Permanente | jutsu duration value; not previously pinned for PT |
| Instant | Imediato | jutsu duration value, distinct from "Instantaneous" below — both appear as separate English words in source content and the fan's spreadsheet renders them distinctly, so both are pinned separately rather than merged |
| Instantaneous | Instantâneo | jutsu duration value, see note on "Instant" above |

## Core mechanics canonical renderings (pinned 2026-10-08)

One mechanic, one rendering, in `pt.json` and in every pt `content_translations` row. Before
this pass, "saving throw" appeared as "jogada de resistência", "jogada de salvamento", "jogo de
defesa", "teste de salvação", "teste de defesa" and "salvaguarda", and "Save DC" as "CD de
Defesa", "CD de Salvação", "CD de Jogo de Defesa", "DC de Defesa" and "Salve DC".

**Source**: the 2014 official PT-BR D&D 5e (Galápagos Jogos) Livro do Jogador and its character
sheet, which use "teste de resistência", "CD de resistência", "Dados de Vida" and "pontos de
vida" / "PV". This game is a 2014-5e hack and already used "teste de resistência" in ~2,050
content rows, and `translation_methodology.md` uses it too.

**Considered and rejected**: "salvaguarda" from the 2024 PT-BR Livro do Jogador (Asmodee, May
2026), which the community Foundry `pt-BR` module also follows ("salvaguarda", "CD de Magia").
It is now official, but it is the 2024 rules' vocabulary, while every other term this game
borrows (Hit Dice, proficiency, the 2014 class/condition set) follows the 2014 books. Mixing
both editions' terms is worse than consistently using the 2014 one. Revisit this if the game
ever rebases on the 2024 rules.

| English | Portuguese | Source |
|---|---|---|
| Saving throw | teste de resistência | Galápagos 2014 PT-BR. Plural "testes de resistência". The short English "save" ("Wisdom save", "on a failed save") also maps here, never to bare "resistência", which is damage resistance in this ruleset. "teste" is masculine: "o teste", "bem-sucedido", "um teste" |
| Save DC | CD de resistência | Galápagos 2014 PT-BR "CD de resistência (de magia)". CD (classe de dificuldade) is **feminine**: "a CD", "sua CD", "na CD". UI labels use "CD de Resistência" |
| DC | CD | Galápagos 2014 PT-BR; never "DC" |
| Death saving throw | teste de resistência contra a morte | Galápagos 2014 PT-BR; replaces "teste de morte" |
| Hit Die | Dado de Vida | Galápagos 2014 PT-BR. Replaces "Dado de Dano" (`classes.hit_die`), which means damage die |
| Hit Dice | Dados de Vida | plural of the row above |
| Hit points | pontos de vida | Galápagos 2014 PT-BR (unchanged, already consistent) |
| Chakra Die | Dado de Chakra | homebrew, built on "Dado de Vida"; previously left as untranslated "Chakra Die" in `classes.chakra_die` |
| Chakra Dice | Dados de Chakra | plural of the row above |
| Flurry Die | Dado de Rajada | homebrew; the majority pt rendering. Replaces "Matriz Flurry", "dado de Flurry", "dado de série" and "dado de enxurrada" |
| Flurry Technique | Técnica de Rajada | homebrew, matches Dado de Rajada. Named techniques are quoted after it: Técnica de Rajada “Desvio Aprimorado”. Replaces "técnica de série de ataques", "técnica de ataque em sequência" and "Técnica Flurry" |
| Weapon Flurry | Rajada com Armas | Weapon Specialist class feature; replaces "Série de Golpes com Armas" and "Flurry com Arma" |
| Superior Weapon Flurry | Rajada com Armas Superior | Weapon Specialist class feature; replaces "Série de Ataques com Armas Superiores", where "Superior" wrongly modified the weapons |
| Enhanced Flurry | Rajada Aprimorada | Taijutsu Specialist (Passionate Flame) feature; replaces the untranslated "Flurry Aprimorado". es "Ráfaga mejorada" uses the same family |
| Disturbance Flurry | Rajada de Perturbação | Taijutsu Specialist (Disturbance) feature; replaces "Oleada de perturbações", where "Oleada" is Spanish |
| Cooking Die | Dado de Cozinha | homebrew; always capitalized as a resource name. Replaces "Dado de Culinária" and "dado de culinária". "Kit de Culinária" / "Ferramenta de Culinária" (the Cooking Kit / Tool) are a different thing and stay |
| Cooking Dice | Dados de Cozinha | plural of the row above |
| Attack roll | jogada de ataque | Galápagos 2014 PT-BR. Replaces "teste de ataque" and "lançamento de ataque". "jogada" is feminine: "a jogada", "sua próxima jogada de ataque bem-sucedida", "nas jogadas de ataque" |
| Attack rolls | jogadas de ataque | plural of the row above |

Out of scope for this pin: the Taijutsu Specialist's "Flurry of Blows" (its own Martial
Technique, "Enxurrada de golpes"), and "Gama Flurry" (a toad summon's proper name, kept as-is
in es too).

### `n5e-i18n normalize-terms`

`cmd/n5e-i18n/normalize_terms.go` rewrites the drifted variants above to the pinned
renderings, in a catalog (`-catalog internal/i18n/locales/pt.json`) or in a rules DB's
`content_translations` (`-db out/rules.db`). It is a dry run unless `-apply` is given. For pt it
also fixes grammatical agreement around the rewritten noun: "a jogada de resistência
bem-sucedida" becomes "o teste de resistência bem-sucedido", and "o seu CD" becomes "a sua CD".
Each rule is gated on the English source containing the matching term, so an unrelated
"defesa" (defense) is never touched. It was run on 2026-10-08 against both catalogs and against
`out/rules.db` and `dist/rules.db`: 787 pt rows and 854 es rows, plus 83 pt and 25 es catalog
keys. The Cooking Die and attack-roll rules were added the same day and rewrote 497 more pt rows
and 5 catalog keys; the two remaining hand fixes were "é necessário uma jogada de ataque" →
"necessária".

## Class names (pinned 2026-10-09)

Before this pin the class table left every "-Nin" class in English, and the translated text
mixed three styles for the same class: the English name ("Science-Nin"), an "X-Nin" hybrid
("Ciência-Nin", "Batedor-Nin") and "Ninja X" ("Ninja Científico"). Puppet Master alone had
eleven renderings ("Mestre de Marionetes", "Marionetista", "Mestre de Bonecos", "Mestre dos
Fantoches", ...). The renderings below were chosen by native PT-BR speakers in the community;
"-Nin" classes use the "Ninja X" form, which also matches the Spanish class names.

| English | Portuguese | Source |
|---|---|---|
| Science-Nin | Ninja Cientista | community PT-BR translators. Replaces "Science-Nin", "Ciência-Nin", "Ninja Científico" |
| Scout-Nin | Ninja Batedor | community PT-BR translators. Replaces "Scout-Nin", "Batedor-Nin", "Escoteiro-Nin" |
| Hunter-Nin | Ninja Caçador | community PT-BR translators. Replaces "Hunter-Nin", "Caçador-Nin" |
| Medical-Nin | Ninja Médico | community PT-BR translators (accent restored: "Médico"). Replaces "Medical-Nin", "Médico-Nin", "Medicina-Nin" |
| Cooking-Nin | Ninja Cozinheiro | community PT-BR translators. Replaces "Cooking-Nin", "Cozinheiros-Nin" |
| Puppet Master | Mestre das Marionetes | community PT-BR translators; already the class table's name. Replaces "Mestre de Marionetes", "Marionetista", "Mestre de Bonecos", "Mestre dos Fantoches", "Manipulador de Marionetes", "Puppet Master" |
| Intelligence Operative | Agente de Inteligência | community PT-BR translators (accent restored: "Inteligência"). Capital I; replaces "Agente de inteligência", "Operativo de Inteligência" |
| Weapon Specialist | Especialista em Armas | community PT-BR translators; already consistent |
| Genjutsu Specialist | Especialista em Genjutsu | already consistent |
| Ninjutsu Specialist | Especialista em Ninjutsu | already consistent |
| Taijutsu Specialist | Especialista em Taijutsu | already consistent |

## Jutsu names — confirmed gaps (Spanish translated the same field, Portuguese didn't)

| English | Portuguese | Source |
|---|---|---|
| Mirage | Miragem | jutsu name; direct cognate; Spanish translated as "Espejismo" |
| Noble Impact | Impacto Nobre | Spanish translated as "Impacto noble" |
| Phantom Mobility | Mobilidade Fantasma | Spanish translated (oddly) as "Phantom Movilidad" — corrected here rather than copied, since Spanish's own version left "Phantom" untranslated |
| Shockwave Slash | Corte de Onda de Choque | Spanish translated (grammatically odd) as "Shockwave Cortante" — corrected here rather than copied |
| Web Bind | Amarração da Teia | Tsuchigumo-clan jutsu name; Spanish mistranslated this one as "Encuadernación en rústica" (paperback binding, a false-cognate error on "bind") — do NOT copy Spanish's version, it's wrong there too |
| Breath of Earth: Smash | Sopro da Terra: Impacto | matches the established "Breath of X" → "Sopro de/do/da X" pattern already correctly applied to ~30 sibling jutsu names in this family (Breath of Ash/Beasts/Dust/Echo/Enoki/Flame/Moon/Rain/Skies/Sound/Water/Wind); this was the one straggler left fully in English |
| Breath of Echo: Reverb | Sopro do Eco: Reverberação | same "Breath of X" pattern; sibling "Breath of Echo: Crescendo/Roar" both already translate the prefix as "Sopro do Eco" |
| Falling Heaven: Cross | Falling Heaven: Cruz | "Falling Heaven" is a Fuma-clan technique-line title consistently left untranslated across every sibling (Calamity/Execution/Focus/Penetrate/Protect/Rain/Storm all keep "Falling Heaven:" in English) — only the suffix translates |
| Falling Heaven: Divide | Falling Heaven: Divisão | same pattern as above |
| Falling Heaven: Split | Falling Heaven: Fragmentação | same pattern as above |

## Jutsu names — left untranslated on purpose (identity, locks the decision)

Confirmed against the equivalent Spanish field (also untranslated) or against genre/franchise
convention. Listed as identity pairs so a future `-force` retranslation doesn't churn them.

| English | Portuguese | Source |
|---|---|---|
| Shatter | Shatter | stylized jutsu title, untranslated in Spanish too |
| Flash | Flash | stylized jutsu title, untranslated in Spanish too |
| Bane | Bane | stylized jutsu title, untranslated in Spanish too |
| Eclipse | Eclipse | stylized jutsu title, untranslated in Spanish too |
| Juggernaut | Juggernaut | stylized jutsu title, untranslated in Spanish too |
| Grand Slam | Grand Slam | stylized jutsu title, untranslated in Spanish too |
| Heavy Metal | Heavy Metal | music-genre loanword, commonly left untranslated in Portuguese; Spanish's "Pesada Metal" is grammatically wrong (should be "Metal Pesado" if translated at all) — left as the loanword instead of copying Spanish's error |

**Resolved 2026-09-11**: `jutsu/o-encode-thoughts`'s stray leading "O" was a source-book
data-entry typo, confirmed and fixed — see `internal/correct.CuratedJutsuNameFixes`
("O Encode Thoughts" → "Encode Thoughts", "One Man Front Lotud" → "One Man Front Lotus").
The fix is applied directly after parsing in `internal/ingest/ingest.go`'s `Jutsu()`, so it
survives a fresh PDF re-ingest — jutsu.Name is deliberately excluded from `correct.Sweep`'s
prose fields (see `internal/correct/registry.go`), so this couldn't go through the normal
misspell/curated-fix pipeline. Translate the corrected name normally going forward.

## Crossover anime/pop-culture proper nouns — left untranslated by design

N5e deliberately borrows named techniques/features from anime outside the Naruto-verse (Science-Nin's
"Spyware" Inquiry cluster in particular reads as a cyberpunk/mecha-anime homage — see
Cruel Angel's Thesis, In His Image, Full-Metal Shinobi elsewhere in this file, all already
translated as ordinary phrases since they're not the property's own title). These two are the
actual title of the referenced work itself, not just a themed phrase, so they're pinned here as
identity pairs — this is the correct, intentional outcome, not a translation gap:

| English | Spanish | Portuguese | Source |
|---|---|---|---|
| Ghost in the Shell | Ghost in the Shell | Ghost in the Shell | film/manga title kept in English in both Spanish- and Portuguese-language distribution — Science-Nin "Spyware" subclass feature name (`subclass_features/class/science-nin/group/scientific-inquiry/spyware/feature/ghost-in-the-shell`) |
| Unlimited Blade Works | Unlimited Blade Works | Unlimited Blade Works | Fate/stay night Reality Marble name — stated in English in the original Japanese source material itself, so never translated in any market; appears twice in this game as a Puppet Master platinum-tier upgrade name (`class_option_entries/class/puppet-master/option/upgrades-of-war/platinum-tier/entry/unlimited-blade-works`) and a Hunter-Nin "Arsenalist" subclass feature name (`subclass_features/class/hunter-nin/group/hunters-creeds/arsenalist/feature/unlimited-blade-works`) |

## Weapons Range (jutsu.range field value — pinned 2026-09-11)

**Correction, 2026-09-11**: this table's own header/values were copy-pasted from the Spanish
glossary by mistake — the cells below were Spanish text ("Distancia de las armas") sitting under
a "Portuguese" heading, so `ParseGlossaryTables(path, "Portuguese")` never picked them up and the
underlying bug (DeepL mistranslating "Weapons Range" as "Campo de Tiro," a shooting-range-facility
sense) was still live in Portuguese content across 73 `jutsu.range` rows at audit time. Fixed here
with real Portuguese and hand-patched directly in `content_translations` for all 73 affected rows
(the 2 "Twice Weapons Range" rows were already correct, matching the pattern below).

| English | Portuguese | Source |
|---|---|---|
| Weapons Range | Alcance das armas | homebrew `jutsu.range` value meaning "this jutsu's range equals your weapon's range"; matches the already-correct singular/apostrophe'd sibling forms ("Weapon's Range" → "Alcance da arma") |
| Twice Weapons Range | Alcance duplo das armas | same convention, doubled-range variant; already correctly rendered before this fix |

## Summon Tribe feature names — animal-name prefix stays untranslated, descriptor translates

Follows the established "Full-Metal Shinobi" → "Shinobi de Metal Puro" partial-translation
convention: the Japanese summon-animal name (Kitsune/Nezumi/Gama/Kame/Itachi) stays untranslated,
only the English descriptor translates. Pinned 2026-09-11 after DeepL left these 5 fully
untranslated (identical to English) despite correctly handling every other sibling in the same
family (e.g. "Nezumi Assault" → "Asalto de Nezumi", "Itachi Speed" → "Velocidad de Itachi").

**Correction, 2026-09-11**: this table's header/values were also copy-pasted from Spanish by
mistake (same bug as "Weapons Range" above) — "Nezumi Tutelage" had already been separately fixed
to real Portuguese ("Tutela de Nezumi") and that fix stuck, but the other 4 rows were still
Spanish text under a "Portuguese" heading, so the underlying gap stayed live for all 4 in actual
Portuguese content. Fixed here with real Portuguese and hand-patched directly in
`content_translations` for the 4 affected rows.

| English | Portuguese | Source |
|---|---|---|
| Kitsune Starball | Bola Estelar de Kitsune | `summon_tribe_features/summon/fox/feature/kitsune-starball` |
| Nezumi Tutelage | Tutela de Nezumi | `summon_tribe_features/summon/rat/feature/nezumi-tutelage`; already correct before this fix |
| Gama Oily Strike | Golpe Oleoso de Gama | `summon_tribe_features/summon/toad/feature/gama-oily-strike` |
| Kame Tough Shell | Carapaça Resistente de Kame | `summon_tribe_features/summon/turtle/feature/kame-tough-shell` |
| Itachi Cycling Blade | Lâmina Cíclica de Itachi | `summon_tribe_features/summon/weasel/feature/itachi-cycling-blade` |

**Left untranslated on purpose**: `Kitsune Sessho-Seki` — "Sessho-Seki" ("killing stone") is
itself a genuine Japanese proper noun from kitsune folklore, not an English descriptor, matching
the sibling untranslated names in the same fox family (Kitsune Tenko/Yako/Zenko/Ninko/Tsume).
The verify pass's "suspiciously identical" flag on this one is a false positive, not a bug.

## Descriptive names surfaced by the 405-warning glossary triage (pinned 2026-09-11)

**Correction, 2026-09-11**: this table's header/values were copy-pasted from the Spanish
glossary by mistake — every cell below was Spanish text sitting under a "Portuguese" heading
(same bug as "Weapons Range"/"Summon Tribe feature names" above), so none of these terms ever
reached the pt DeepL glossary and the underlying gaps were still live in Portuguese content at
audit time. Fixed here with real Portuguese, reusing this project's own already-established
canonical renderings where one already existed elsewhere (`internal/i18n/locales/pt.json`'s
`ns_molding_name:`/`cr_name:`/`foundation_name:` catalogs), and hand-patched directly in
`content_translations` for every affected row.

| English | Portuguese | Source |
|---|---|---|
| Frenzy: Magenta | Frenesi: Magenta | `jutsu/frenzy-magenta`; "Frenzy" → "Frenesi" matches the already-translated `cr_name:Freedom of Frenzy` → "Liberdade do Frenesi" |
| Psycho Mind Flux | Fluxo Psicomental | `feat/yamanaka/psycho-mind-flux` |
| Shinobi Sentinel | Shinobi Sentinela | `feat/shinobi-sentinel`; "Shinobi" stays untranslated per the established convention (see "Full-Metal Shinobi" → "Shinobi de Metal Puro") |
| Arsenal Builder | Construtor de Arsenal | `subclass_features/class/hunter-nin/group/hunters-creeds/arsenalist/feature/arsenal-builder` |
| Sapphire Insights | Percepções de Safira | `subclass_features/class/intelligence-operative/group/master-strategies/azure-analyst/feature/sapphire-insights`; matches the already-translated `cr_name:Sapphire Insights` catalog entry |
| Gigawatt Molding | Moldagem de Gigawatts | `subclass_features/class/ninjutsu-specialist/group/ninjutsu-focus/lightning-breaker/feature/gigawatt-molding`; matches the already-translated `ns_molding_name:Gigawatt Molding` catalog entry |
| Lifeforce Casting | Lançamento de Força Vital | `subclass_features/class/ninjutsu-specialist/group/ninjutsu-focus/sanguine-master/feature/lifeforce-casting`; "Casting" in the jutsu-casting sense ("Lançamento"), not the puppet-upgrade "Fundição" sense below |
| Vibrosteel Edge | Fio de Vibroaço | `class_option_entries/class/science-nin/option/arsenal-modifications/minor/entry/vibrosteel-edge` |
| Peerless Casting | Fundição Inigualável | `class_option_entries/class/puppet-master/option/magus-upgrades/wood-tier/entry/peerless-casting`; "Casting" in the puppet-construction sense here, matching sibling entries already translated in the same magus-upgrades tier group ("Successive Casting" → "Fundição sucessiva", "Resizable Casting" → "Fundição redimensionável") |
| Bestial Framework | Estrutura Bestial | `class_options/class/puppet-master/option/puppet-frameworks/bestial-framework`; matches the already-translated `foundation_name:Bestial Framework` catalog entry — this one was already correctly translated in the database, added here only to lock the decision and match the catalog |
| Kujaku Flourish | Floreio Kujaku | `feat/hoshi/kujaku-flourish`; "Kujaku" (Japanese for peacock) stays untranslated as the summon/technique-name prefix, matching the word order already used for `cr_name:Kujaku Mode` → "Modo Kujaku" |
| "12 Chakra, X Fury" (jutsu.cost_text) | "12 Chakra, X Fúria" | `jutsu/biding-fury-velocity-untold`; "Fury" per its own sibling row's established rendering (`jutsu/binding-fury-meteor`'s "todas as suas acumulações atuais de Fúria") |

**Left untranslated on purpose**: `Triple Iron Maiden` (`class_option_entries/class/puppet-master/option/black-iron-upgrades/gold-tier/entry/triple-iron-maiden`)
— its own prerequisite/base entry "Iron Maiden" is already left untranslated by design, so
translating only the "Triple" variant would break consistency with the term it's built on.
`Full Metal Jacket` (`class_option_entries/class/puppet-master/option/armorers-upgrades/gold-tier/entry/full-metal-jacket`)
— a real-world firearms/armor term (and the Kubrick film title), kept in English matching the
same "real-world proper noun stays in English" pattern as Iron Maiden rather than translated
literally.

## Clan Hijutsu naming (pinned 2026-09-11)

Every clan's own signature Hijutsu trait is named `"[Clan] Hijutsu"` in English (e.g. "Uzumaki
Hijutsu") — matches Spanish's noun-first reorder, applied here the same way since Portuguese
shares that head-noun-first structure. Found untranslated (identical to English) for all 6 clans
that have this trait during this audit; hand-patched directly in `content_translations`.

| English | Portuguese | Source |
|---|---|---|
| Hyūga Hijutsu | Hijutsu Hyūga | `clan_traits` (clan/hyuga) |
| Inuzuka Hijutsu | Hijutsu Inuzuka | `clan_traits` (clan/inuzuka) |
| Jugo Hijutsu | Hijutsu Jugo | `clan_traits` (clan/jugo) |
| Kashu Hijutsu | Hijutsu Kashu | `clan_traits` (clan/kashu) |
| Uzumaki Hijutsu | Hijutsu Uzumaki | `clan_traits` (clan/uzumaki) |
| Yamanaka Hijutsu | Hijutsu Yamanaka | `clan_traits` (clan/yamanaka) |

## Equipment seal names — Superior/Mastercraft tier gap (pinned 2026-09-11)

Found untranslated (identical to English) at the Superior or Mastercraft tier specifically, while
sibling Minor/Refined/Greater tiers of the same base seal were already correctly translated —
hand-patched directly in `content_translations`. "(Mastercraft)" → "(Obra-prima)" matches the
convention already used on every other Mastercraft-tier item.

| English | Portuguese | Source |
|---|---|---|
| Hunter Seal | Selo de Caçador | `equipment` (seal/weapon/superior/hunter-seal) |
| Speed Seal | Selo de Velocidade | `equipment` (seal/weapon/superior/speed-seal) |
| Hot Seal | Selo Quente | `equipment` (seal/weapon/mastercraft/hot-seal, seal/armor/mastercraft/hot-seal); matches already-translated Minor/Greater tiers |
| Cold Steel Seal | Selo de Aço Frio | `equipment` (seal/armor/mastercraft/cold-steel-seal); matches already-translated Minor/Greater tiers |
| Star Metal Seal | Selo Estelar de Metal | `equipment` (seal/armor/mastercraft/star-metal-seal, seal/armor/greater/star-metal-seal, seal/armor/minor/star-metal-seal); the Minor tier had a separate, inconsistent "Selo Metálico Estelar" — normalized to match its Greater/Mastercraft siblings |

## Jutsu category_group — elemental Ninjutsu subheadings (pinned 2026-09-12)

Found during a follow-up audit specifically checking that Bukijutsu (and its siblings Taijutsu/
Ninjutsu/Genjutsu) weren't replicating the earlier Spanish glossary typo bug (glossary.md's
Bukijutsu row briefly containing the literal word "*unchanged*" instead of "Bukijutsu" — see the
"Nature-transformation" section's neighboring history). Bukijutsu itself is fine in Portuguese
(and Spanish) — confirmed unchanged in both `jutsu.category_group` and ordinary prose. But the
same audit surfaced a different, previously-uncaught bug in the *same field*, same root cause as
the already-fixed "Weapons Range" bug: `jutsu.category_group` is a clean, byte-identical-per-row
enum column (e.g. exactly `"Ninjutsu / Fire Release"` repeated across every Fire jutsu), yet DeepL
translated that identical input inconsistently across different import batches, splitting what
should be one book-section subheading on the jutsu browse page into two or three separate ones.
574 total elemental/non-elemental rows checked; Earth (`Liberação da Terra`, 75/75) and Wind
(`Liberação do Vento`, 74/74) were already fully consistent — Fire, Water, Lightning, and
Non-Elemental were not. Canonical form for each picked to match how the *same* element already
reads correctly and consistently in `jutsu.name` for "X Release: ..." jutsu titles (verified by
querying those rows directly, not guessed) — hand-patched directly in `content_translations`
rather than re-running translation, so no risk of a fresh DeepL pass reintroducing different drift.

**Superseded 2026-09-12 by the "Descarga" pass below — the table's word choices in the
Portuguese column are stale.** Kept for its historical record of what was normalized and why
(the consistency methodology below is still valid); wherever this table says "Liberação de X,"
the row now actually reads **"Descarga de/do/da X"** in `content_translations` — do not copy
"Liberação" from this table into a new language or a restored/rolled-back DB.

| English | Portuguese (as fixed at the time — now superseded, see note above) | Source |
|---|---|---|
| Ninjutsu / Fire Release | Ninjutsu / Liberação de Fogo → now Descarga de Fogo | matches `jutsu.name`'s own "Liberação de Fogo: X" pattern (e.g. `jutsu/fire-release-dawn`); normalized 4 stray "Liberação **do** Fogo" rows |
| Ninjutsu / Water Release | Ninjutsu / Liberação de Água → now Descarga de Água | matches `jutsu.name`'s own "Liberação de Água: X" pattern; normalized 19 stray "Liberação **da** Água" rows |
| Ninjutsu / Lightning Release | Ninjutsu / Liberação do Relâmpago → now Descarga do Relâmpago | matches `jutsu.name`'s own "Liberação do Relâmpago: X" pattern (word choice "Relâmpago," not "Raio"); normalized 8 stray "Liberação do **Raio**" rows |
| Ninjutsu / Non-Elemental | Ninjutsu / Não-elemental (unaffected by the Descarga pass — "Non-Elemental" is not a "Release" term) | matches the hyphenated-lowercase form already used consistently elsewhere in Portuguese content (e.g. "condição Elemental" prose, `feat`/`subclass_feature` descriptions); normalized 5 "Não Elemental" + 8 "Não elementar" stray rows |

**Spanish fixed 2026-09-12.** Unlike Portuguese, `jutsu.name` itself had ZERO rows using the
correct "Liberación de Tierra:" prefix to copy from — Earth Release was mistranslated three
different ways across the *name* field too (`Técnica de la Tierra:` 71 rows, `Elemento Tierra:` 3
rows, `Libertad de la Tierra:` 1 row), so the canonical form couldn't be picked by majority vote
the way it was for Portuguese. Picked "Liberación de Tierra" instead by matching the existing
correct minority form (2 `category_group` rows already read this way) and the "no article for
feminine elements" convention already established by the correctly-translated `Fuego`/`Agua`
siblings (`Liberación de Fuego`, `Liberación de Agua` — no article — vs. `Liberación del Viento`/
`Liberación del Rayo`, which take `del` because Viento/Rayo are masculine). Hand-patched (case-
preserving `REPLACE`) across `jutsu.name`, `jutsu.category_group`, `jutsu.description` (6 rows,
including mid-sentence "Chakra de liberación de tierra" phrasing), and `jutsu.keywords` (dead
field, fixed for consistency only — see [[project_i18n_deepl_pivot_status]] on why keywords is
excluded from `TranslatableFields` and never actually overlaid at read time). `Ninjutsu /
Liberación de Rayo` (2 stray rows) also normalized to the majority-correct `Liberación del Rayo`
(72 rows), matching `jutsu.name`'s own consistent "Liberación del Rayo: X" pattern.

**Superseded 2026-09-12 by the "Descarga" pass below — same caveat as the Portuguese table
above**: wherever this table says "Liberación de/del X," the row now actually reads "Descarga de
Tierra"/"Descarga del Rayo" in `content_translations`. Do not copy "Liberación" from this table.

| English | Spanish (as fixed at the time — now superseded, see note above) | Source |
|---|---|---|
| Ninjutsu / Earth Release | Ninjutsu / Liberación de Tierra → now Descarga de Tierra | matches the pre-existing correct minority `category_group` rows and the no-article Fuego/Agua convention; normalized 73 stray `category_group` rows (71 "Técnica de la Tierra" + 2 "Elemento Tierra") and separately all 75 `jutsu.name` rows, which had zero correct rows to begin with (71 "Técnica de la Tierra:", 3 "Elemento Tierra:", 1 "Libertad de la Tierra:") |
| Ninjutsu / Lightning Release | Ninjutsu / Liberación del Rayo → now Descarga del Rayo | matches `jutsu.name`'s own consistent "Liberación del Rayo: X" pattern; normalized 2 stray "Liberación **de** Rayo" rows |

**Found but deliberately NOT fixed in this pass — a much larger cross-table version of the same
"Elemento Tierra" mistranslation**: the identical wrong term (and its Rayo/Viento/Fuego/Agua
siblings) also appears as a proper-noun stand-in for "Earth/Lightning/Wind/Fire/Water Release"
outside the `jutsu` table — confirmed via English-source cross-checking (Portuguese's own already-
correct rendering of the same row, e.g. `clan/bakuton/feature/explosion-release`'s pt text reads
"Liberação de Terra"/"Liberação do Relâmpago" where the es text reads "Elemento Tierra"/"Elemento
Rayo") in `clan_features.name`/`.description`, `clan_traits.name`/`.description`, `clans.overview`,
`feats.name`, and `subclass_features.name`/`.description` — roughly 150+ rows across all five
elements, not just Earth. **Not the same as the lowercase generic "elemento tierra/agua" (small
`e`) found in `class_option_entries`/`class_options` prose** (e.g. "si has elegido el elemento
tierra o agua"), which is a correct, unrelated translation of a genuinely generic phrase and must
NOT be touched by any future blind find/replace here. Worth its own dedicated pass, scoped and
verified table-by-table the same way this one was, rather than a blanket replace.

## "Nature Release" terminology — "Liberación"/"Liberação" replaced by "Descarga" (pinned 2026-09-12)

**Supersedes the canonical forms given in "Jutsu category_group — elemental Ninjutsu
subheadings" above** (`Liberación de Tierra`, `Liberação de Fogo`, etc.) — those rows are now
`Descarga de Tierra`, `Descarga de Fogo`, and so on. This was not a bug-fix pass, it was a
deeper correction to the whole project's established glossary convention for the Naruto-style
"X Release"/"Nature Release" chakra-technique system, applied across *every* table in both
locales in one sweep, not just `jutsu.category_group`.

**The problem**: "Release" in "Fire Release," "Nature Release," "Limiter Release," etc. means
unleashing/discharging chakra power — the sense in "Release your limiters!" or an archer's
"Release!" command to loose arrows, not the sense of setting someone free. DeepL (and the
project's own prior hand-picked term) rendered it as **"Liberación"/"Liberação"/"Libertação"** —
grammatically fluent Spanish/Portuguese, but carrying the *freedom/parole* connotation (releasing
a prisoner), the wrong sense entirely. Confirmed directly against raw English source (not a
translated proxy) — e.g. `clan/bakuton/feature/explosion-release`'s English text: "...their close
affinity to both Earth and Lightning Release... casting a ... Jutsu with the Earth or Lightning
Release Keywords..." — plainly the power/unleash sense throughout.

**The fix**: replaced with **"Descarga"** ("Discharge") for both locales, everywhere the
elemental/power sense of "Release" was intended. Applied via a whitelist-driven script (continuation
word after "Liberación"/"Liberação"/"Libertação" must match a confirmed-safe elemental-context
phrase, e.g. "de Tierra," "do Fogo," "de la Naturaleza," plus a short list of exact-phrase literal
overrides for irregular phrasing) rather than a blind regex — anything not matching stays
untouched by default. Result: **1282/1287 Spanish rows and 1425/1431 Portuguese rows changed,
3986 total substitutions**, across `jutsu` (category_group, keywords, description, name,
cost_text, at_higher_ranks, casting_time), `subclass_features`, `feats`, `class_options`,
`class_option_entries`, `equipment`, `summon_tribes`, `clan_features`, `clan_traits`,
`summon_tribe_features`, `clans`, `clans` overview, `subclasses`, `adversary_role_traits`, and
`class_features`. Live-verified in a fresh browser session: the jutsu browse page renders
"Descarga de Tierra/Agua/Fuego/Viento/Rayo/..." with zero stray "Liberación"/"Liberação" left in
that list, in both locales.

**Genuine exceptions, deliberately left as "Liberación"/"Liberação"/"Libertação" — this is the
other, correct sense of "Release" (freeing/dispelling), not a miss**: `jutsu/release`'s own
Genjutsu-dispel technique ("isolate and forcefully remove the effect of a single Genjutsu... free
yourself or your target from the Genjutsu") and its direct cross-references — the Puppet Master
"Countermeasures" upgrade ("«Liberación»/«Técnica de Libertação» o «Escape»/«Fuga»"), the
Genjutsu Specialist "Tentative Escape" option (bare "«Liberación»"/"“Libertação”"), and the
"Hanami" feat's "Mass Release"/"Enhanced Inhalant" subclass feature (both reference "Release
Genjutsu" = "Liberación de Genjutsu"/"Liberação de Genjutsu"). 7 rows in each locale, all
confirmed by reading the actual English source text row-by-row before excluding them, and
confirmed still rendering correctly (unchanged) in a live browser check.

**Not folded into this pass**: the previously-documented cross-table "Elemento Tierra"/"Elemento
X" bug (see the category_group section above) is a different root cause — a wrong noun entirely
("Element" instead of "Release"), not a wrong connotation on the right noun — even though fixing
it would also plausibly land on "Descarga de X." Fixed separately immediately after, see below.

## "Elemento X" bug for the 5 base elements + Yin/Yang — fixed to "Descarga de X" (pinned 2026-09-12)

**Different root cause from the "Liberación"/"Descarga" pass above, even though both land on
"Descarga."** That pass fixed the wrong *connotation* on the right noun ("Liberación" = freedom
instead of unleash-power). This bug used the wrong noun entirely — "Elemento"/"Elemento" ("Element")
in place of any release-word — for the 5 base elements (Earth/Fire/Water/Wind/Lightning) and
Yin/Yang, in `clan_features`, `clan_traits`, `clans` overview, `feats`, and `subclass_features`.
The regex that drove the Liberación pass never touched these rows, since they never contained
"Liberación"/"Liberação" at all.

**Canonical form for the 5 base elements + Yin/Yang, both current locales — check this table
before touching any "Earth/Fire/Water/Wind/Lightning/Yin/Yang Release" row, and use it as the
template when translating into any future language**:

| English | Spanish | Portuguese |
|---|---|---|
| Earth Release | descarga de Tierra | descarga da Terra |
| Fire Release | descarga de Fuego | descarga de Fogo |
| Water Release | descarga de Agua | descarga de Água |
| Wind Release | descarga del Viento | descarga do Vento |
| Lightning Release | descarga del Rayo | descarga do Relâmpago |
| Yin Release | descarga Yin | descarga Yin |
| Yang Release | descarga Yang | descarga Yang |

### Country names built on an element word

These are setting place names, not the mechanic, and they keep the element's
everyday word rather than the pinned mechanic rendering — the country is
"Terra do Raio," never "Terra do Relâmpago." They live in this table so
longest-match-wins (`dropSubsumedTerms`) suppresses the bare `Lightning` entry
on rows that only mention the country; without an entry here the checker flags
a correct translation, since the lowercase "of" in "Land of Lightning" puts it
outside `inTitleCaseRun`'s reach.

| English | Spanish | Portuguese |
|---|---|---|
| Land of Lightning | Tierra del Rayo | Terra do Raio |

The wider convention these follow, confirmed by a corpus sweep 2026-09-13
across ~55 rows: every `Land of X` country is `Terra do/da X` / `Tierra del/de
la X`, capitalized as the proper noun it is. That sweep also turned up the two
ways this goes wrong — a lowercased country name ("tierra del rayo"), and an
English word left in place inside an otherwise-translated name ("tierra de
Cackles", "Tierra de Graves"). Both are fixed; check new country names for
both.

**Corrected 2026-09-12**: Portuguese Fire/Water Release were originally entered here as "descarga
do Fogo"/"descarga da Água" — the grammatically-expected contracted forms — without checking
whether that was actually the established convention. Cross-referencing `jutsu.category_group`
(the most-audited, already-normalized PT data in the whole project) and a raw substring count
across the entire PT locale (`de Água` 190 rows vs `da Água` 63; `de Fogo` 230 vs `do Fogo` 50)
showed the real established convention is uncontracted "de Fogo"/"de Água" for these two elements
specifically — Portuguese does NOT uniformly contract the preposition with the element's gender.
Lesson generalized: **before assuming a "grammatically expected" form is the established
convention, check the most-audited existing data for that exact phrase** — don't derive it from
grammar rules alone, even inside this same glossary file's own prior notes.

Notes on this table: capitalize the first word when it starts the sentence/phrase ("Descarga de
Tierra"), never mid-phrase. Spanish takes no article for feminine-gender element nouns (Fuego,
Agua) but "del" for masculine ones (Viento, Rayo) when preceded by "la descarga del Viento" etc.
Portuguese contracts the preposition with the element's gender for Earth/Wind/Lightning ("da
Terra," "do Vento," "do Relâmpago") but NOT for Fire/Water ("de Fogo," "de Água" — uncontracted,
confirmed established usage, not a typo) — check this table's actual values per element rather
than assuming uniform contraction. Yin/Yang take no preposition in either language (direct
apposition) — this was the one form with no pre-existing correct precedent in Spanish, derived
from Portuguese's own already-correct usage.
For a brand-new language: do NOT default to whatever pattern that language already uses for the
kekkei-genkai combo types (see the table right below) — verify the intended sense against raw
English source first (unleash/discharge power, not "Element" and not "freedom/release-from-
captivity"), the same two-step check documented in
[[feedback_established_translation_can_still_be_wrong_sense]].

**Must not be confused with the intentional, correct "Elemento X" convention for kekkei-genkai
COMBO release types** (Crystal/Explosion/Wood/Boil/Steel/Magnet/Plasma/Lava/Storm/Scorch/Ice,
etc. — see "Nature-transformation ('X Release') jutsu naming" above) — those remain "Elemento
Cristal," "Elemento Explosivo," etc., and were explicitly preserved by scoping the fix's element
list to only the base elements + Yin/Yang, never combo names.

**The fix**: article-aware regex (`(el|del|o|do)? elemento <name>` → the fem. "descarga" phrase,
converting a leading masculine article to feminine — "el Elemento Tierra" → "la descarga de
Tierra," "del Elemento Rayo" → "de la descarga del Rayo") plus one exact-phrase literal override
for a compound case (`clan/shakuton` overview's single "Elemento" governing two conjuncts,
"Elemento Fuego y Viento" → "descarga de Fuego y del Viento"). Yin/Yang extended using
Portuguese's own already-correct precedent phrasing ("Descarga Yin"/"Descarga Yang", direct
apposition, no preposition) confirmed against `feat/konjiki/superior-steel` and
`feat/kuru/yin-chakra-mastery` before applying the same shape to Spanish. **Result: 99 Spanish
rows + 5 Portuguese rows, 177 total substitutions.** Live-verified in a fresh scratch-probe
session directly against `rules.db` rows (browser UI not required for this table set):
`clan/bakuton/feature/explosion-release` now reads "...afinidad con la descarga de Tierra y la
descarga del Rayo..." while its own name/trait rows correctly keep "Elemento Explosivo" (the
combo name); Portuguese rows show "Descarga de Terra," "Descarga do Relâmpago," etc.

**Portuguese was checked as thoroughly as Spanish** (not just spot-checked) per explicit
instruction, since Portuguese could plausibly have had the identical bug. Found the same bug in
5 Portuguese rows, plus one confirmed **false positive that was excluded, not fixed**:
`clans/clan/fushin` overview's "usuários do elemento vento" — its English source ("some of the
most powerful wind users in the shinobi world") contains no "Release" at all, so this is a
correct, unrelated, generic translation. Verified against raw English source before excluding,
per [[feedback_established_translation_can_still_be_wrong_sense]]'s standing lesson — confirmed
still reading "elemento vento" unchanged after the fix ran.

**RESOLVED 2026-09-12 (item 1's base-element scope only — `jutsu.name`'s own chaos is still
open, see below)**: fixed the 19 ES + 82 PT `missing-glossary-term` warnings this same sweep
surfaced. Three distinct causes, found in this order:
- A real checker bug in `internal/i18n/verify.go`: `wholeWordRe`/`stemPrefixRe` used Go regexp's
  `\b`, which only recognizes ASCII word characters (`[0-9A-Za-z_]`) — a target-language stem or
  source term starting with an accented letter (Portuguese "Água" being the main offender here)
  never registers a real `\b` boundary against a preceding space, since RE2 treats "Á" itself as
  non-word. This alone accounted for the large majority of the 82 PT warnings (confirmed live:
  `\bÁgu` failed to match "e Água" while `\bAgu` matched "e Agua" on otherwise-identical text).
  Fixed by replacing `\b` with a Unicode-aware boundary (`[^\p{L}\p{N}_]` or start/end of string)
  in both helpers — re-running verify after just this fix dropped the count from 19+82 to 12+8.
- Two of this file's own Phase-2 canonical values were themselves wrong: Portuguese Fire/Water
  Release were entered as the grammatically-expected contracted forms ("descarga do Fogo"/"descarga
  da Água") without checking the actual established convention — corrected to uncontracted "de
  Fogo"/"de Água" above, per the discovery method noted in that section.
- The remaining ~20 genuine hits (after the checker fix and glossary correction) were real
  per-row translation drift, all fixed by hand after checking each against raw English source:
  `subclass_features.name` rows for the Ninjutsu Specialist's own "X Release" feature names (e.g.
  Stone Crusher's `earth-release` = "Técnica de la Tierra"/"Técnica da Terra" instead of "Descarga
  de Tierra"/"Descarga da Terra" — same shape repeated across Fire/Lightning/Wind/Water's own
  base and "-master" feature names in both locales), `feats.name` (`lightning-release-expert`,
  `wind-release-expert` — "Experto en la técnica del Rayo"/"Experto en el Estilo del Viento"),
  a dropped-half-of-compound bug (`feat/kuru/yang-chakra-adept` ES rendered "Yin/Yang Release" as
  just "descarga Yin," losing "Yang" entirely), `clans` overview flavor text using generic "estilo
  de"/"técnica da" instead of the glossary term (`shoton`, `hozuki`), and the elided-list gotcha
  documented below under `fix_release_articles.py`'s known limitation (`weapon-specialist`'s
  `primal-weapon-techniques-changed` PT description had a bare "(Descarga da Terra, do Vento, do
  Fogo, da Água ou do Raio)" list where only the first item carried "Descarga" and the last used
  the wrong word "Raio" instead of "Relâmpago" — hand-fixed since the regex only matches a literal
  preceding "descarga "). Two remaining flags in `feats/feat/medical-release` ("Reach") and
  `clan_features/clan/bakuton/feature/explosion-release` ("Range") are confirmed false positives —
  ordinary English words, not this game's homebrew "Range"/"Reach" mechanics keywords.
- `out/rules.db` also had the separate `fix_release_articles.py` normalization script applied
  (45 ES + 77 PT rows, 215 substitutions) to bring the broader "de X" vs "da/do X" preposition
  inconsistency across `clan_features`/`clan_traits`/`clans`/`feats`/`subclass_features` in line
  with the corrected canonical table, before the hand-fixes above.
- **Reusable lesson**: before assuming a "grammatically expected" form is the established
  convention for a language, check the most-audited existing data for that exact phrase (here,
  `jutsu.category_group`, plus a raw substring count across the whole locale) — don't derive it
  from grammar rules alone, even against this same glossary file's own prior notes.

**Related findings surfaced during this investigation, deliberately NOT fixed here — flagged for
a future dedicated pass**:
1. **`jutsu.name`'s "X Release:" prefix chaos** — a much bigger, unrelated mess than
   `category_group` ever was. Spanish Water Release jutsu names alone are split across at least 7
   different renderings ("Técnica del Agua," "Técnica del Elemento Agua," "Técnica de Agua,"
   "Descarga de agua," "Lanzamiento de Agua," and two raw-English-leaking forms). Needs its own
   normalization pass analogous to the `category_group` one documented above, not a quick
   find-replace. **Confirmed 2026-09-12 to extend well beyond `jutsu.name`**: after the base-element
   glossary entries below were added, a full `n5e-i18n verify` sweep (all 5 affected tables, both
   locales) surfaced the identical "wrong word entirely, neither Liberación/Descarga nor Elemento"
   shape in `feats.name`/`.prerequisites`/`.description` (e.g. `feat/lightning-release-expert.name`
   = "Experto en la técnica del Rayo," `.prerequisites` = "Descarga de Rayos" — plural, missing
   article — vs the same feat's own `.description` correctly using "Descarga de Rayo"), and in
   `subclass_features.name` (e.g. the Ninjutsu Specialist Stone Crusher group's `earth-release`
   feature name is still "Técnica de la Tierra," never touched by either fix since it contains
   neither "Liberación" nor "Elemento"). 19 Spanish + 82 Portuguese `missing-glossary-term` warnings
   for base-element Release terms remained across `clan_features`/`clan_traits`/`clans`/`feats`/
   `subclass_features` after both fixes — a mix of genuine misses like the ones above and expected
   false positives (short name/prerequisite fields, and generic flavor text like `clan/shoton`
   overview's "afinidad por el estilo de la Tierra," which may not be meant to restate the literal
   keyword at all). Needs the same careful, per-row-verified normalization pass as `jutsu.name`,
   not a blanket find-replace — run `n5e-i18n verify -db out/rules.db -locale <es|pt> -table
   <table> -backtranslate=false` per table to regenerate the current list before starting.
2. **Bare "Elemento" (no element name following) used generically to mean "Nature Release"** —
   structurally different from the fixed pattern (no name token for the regex to anchor on), so
   not caught by this fix. Found in `clan/jiton` overview ("Todos los Kazekage han tenido este
   Elemento"), `clan/ryu` overview ("manifiesten un Elemento extremadamente temprano"), and
   `subclass_features/class/taijutsu-specialist/.../nature-enhanced-combat` ("Elemento de la
   Naturaleza"). Deliberately left untouched — confirming correct wording here needs a
   per-instance rewrite, not a pattern match.
3. **`feat/kuru/yang-chakra-adept` content-completeness gap** — English source says "Yin/Yang
   Release," but the Spanish description only ever says "descarga Yin" (post-fix), never
   mentioning Yang at all — a missing-content gap in the original translation, not a
   terminology/connotation issue, so out of this fix's scope. The description text itself
   ("Has comenzado a dominar la otra mitad...") implies the other half should be Yang.
4. **Portuguese kekkei-genkai combo-name mistranslations, found incidentally while spot-checking
   the preserved "Elemento Explosivo" pattern** — `clan/bakuton/feature/explosion-release`'s PT
   `name` field reads "Comunicado sobre a explosão" (nonsensical — "Communiqué about the
   explosion") where Spanish correctly has "Elemento Explosivo," and
   `clan/bakuton::Explosion Release Affinity`'s PT name is "Afinidade de Descarga por Explosão"
   (inconsistent with its own sibling terminology). Neither touched by this fix ("explosão" isn't
   in this fix's base-element list) — flagged as a separate, apparently PT-specific
   kekkei-genkai-combo-naming quality issue worth its own audit.

## Pipeline-level safeguard against this bug class recurring (added 2026-09-12)

Everything above catches this bug class through **audits** — reading translated text after the
fact. That is necessarily reactive. The actual DeepL translation pipeline (`n5e-i18n
translate`/`catalog-translate`, and the `n5e-i18n verify` QA pass) already has the machinery to
prevent this class of bug automatically, it just didn't have the right glossary *entries* wired
into it — the fix here is closing that gap, not building new machinery:

**Root cause of why the pipeline didn't catch this the first time**: `setUpGlossary`
(`cmd/n5e-i18n/main.go`) parses every English→target term pair out of `glossary.md`'s tables
(`ParseGlossaryTables`) and feeds them to DeepL as a real glossary constraint, and `n5e-i18n
verify`'s `GlossaryChecker` (`internal/i18n/verify.go`) independently re-checks every translated
row against those same terms after the fact. But **no standalone entry for "Earth Release," "Fire
Release," etc. existed anywhere in the file** — only compound, field-specific phrases like
"Ninjutsu / Fire Release" (scoped to `jutsu.category_group`'s exact enum string) and the since-
corrected "Yin Release" row. A compound-phrase entry only matches that exact phrase; it does
nothing to constrain "Fire Release" appearing on its own in `clan_features`/`feats`/
`subclass_features` prose, which is exactly where DeepL was free to invent a different wrong term
per table.

**The fix**: added the canonical base-element table earlier in this file (see the "'Elemento X'
bug" section's per-element table) as a real, standalone `| English | Spanish | Portuguese |`
glossary entry for all 7 terms (5 base elements + Yin/Yang). This now does two things
automatically, with no audit required:
1. **At translate time**: any future `n5e-i18n translate`/`catalog-translate -force` run
   constrains DeepL to render "Earth Release" as "descarga de Tierra"/"descarga da Terra"
   wherever it appears, in any table, not just the ones already audited.
2. **At verify time**: `n5e-i18n verify` now flags any row where the English text contains one of
   these 7 terms but the translated text doesn't contain the matching glossary rendering — this
   already works today, not just for future translations. Confirmed live: `go run ./cmd/n5e-i18n
   verify -db out/rules.db -locale es -table clan_features -backtranslate=false` reports zero
   `missing-glossary-term` findings for these 7 terms against the now-fixed data.

**Regression-guarded in code, not just documentation**: `internal/i18n/glossary_parse_test.go`
(`TestElementReleaseGlossaryCoverage`, `TestElementReleaseGlossaryRejectsKnownWrongTerms`) asserts
via `go test ./internal/i18n/...` that all 7 terms parse out of `glossary.md` with the exact
expected canonical value for both locales, and that none of them contain "Elemento"/"Liberaci"/
"Liberaç"/"Libertaç" — so an accidental future edit to `glossary.md` that breaks or reverts one of
these entries (a bad merge, a careless table reformat, a rollback to an old commit) fails `go
test`/CI immediately, rather than silently reopening the gap until the next manual audit.

**Bonus finding from running `verify` for the first time across all 5 affected tables**: it
immediately, automatically caught the already-known `feat/kuru/yang-chakra-adept` content gap
(item 3 above) with zero manual reading required, confirming the check has real signal — and
surfaced that the broader "wrong word entirely" naming chaos (item 1 above) extends past
`jutsu.name` into `feats`/`subclass_features` name/prerequisite fields (19 Spanish + 82 Portuguese
`missing-glossary-term` warnings remain across the 5 tables, not yet triaged row-by-row — see item
1's updated note above). Standalone glossary entries prevent a *future* translation from choosing
a *new* wrong word; they cannot retroactively fix content that was already mistranslated with a
third, different wrong word before the entry existed. That backlog is real but out of scope for
this pipeline fix — it's the same deferred normalization pass item 1 already flags.

**That deferred normalization pass is now built and run (2026-09-13)**: `n5e-i18n
normalize-glossary -db <db> -locale <code> [-apply]` (`cmd/n5e-i18n/normalize.go`). It rewrites the
already-mistranslated backlog that a glossary entry alone can never reach, and is re-runnable after
any future `translate -force` re-introduces the same drift. Portuguese `missing-glossary-term`
findings went 384 → 35 (349 rows rewritten); Spanish 421 → ~16 element findings (338 rows
rewritten), with the remaining Spanish total dominated by the common-word noise `MissingTerms`
documents as inherent (Light 98, Fire 64, Range 48, Thrown 46, Reach 18 — ordinary prose the
token-level check cannot distinguish from the pinned game sense).

Three properties make it safe to run unattended, and are worth preserving if it's ever extended:
- **It only touches rows `verify` already flags**, via the same `GlossaryChecker.MissingTerms`
  call, and only for the terms that row is flagged for. A row already carrying the approved
  rendering is never rewritten, so a second run is a verified no-op.
- **It only handles the 5 base element Releases** (`elementReleaseTerms`). These are multi-word
  mechanics names with no ordinary-prose homograph, which is exactly the property the noisy terms
  above lack — a mechanical rewrite of "Light" or "Range" would corrupt correct prose.
- **It re-agrees grammar rather than doing a flat string swap.** The approved renderings are
  spelled lowercase and mid-sentence in the tables above ("descarga da Terra"), but the
  overwhelmingly common site is a jutsu name's own leading position, so the replacement inherits
  the capitalization of what it replaced; and a consumed governing article is re-agreed with the
  feminine "descarga" (pt "da", es "de la"), so "chakra **do** Elemento Fogo" becomes "chakra
  **da** descarga de Fogo" rather than a broken "chakra do descarga de Fogo".

**Portuguese's residual 35** are one documented word-choice family, left deliberately for a human:
rows rendering Lightning Release's head noun as "Raio" rather than the pinned "Relâmpago" (see the
`Ninjutsu / Lightning Release` row above), in the bare form "Chakra do Raio" with no wrapper word.
Those cannot be rewritten mechanically without also corrupting legitimate *lightning damage-type*
prose ("dano de raio") in the same rows, which is the exact ambiguity a human read resolves and a
token-level rule cannot.

**For a future locale or `-force` retranslation**: this pattern (a term appearing in prose across
multiple content tables having no standalone glossary entry, only field-specific compound
entries) is the general lesson, not specific to "Release." Before trusting DeepL's default output
for any established multi-word game term, check whether it has a real standalone glossary row —
a category_group-style compound entry alone does not protect the same term elsewhere.

## Jutsu range "Self" — mistranslated as a bare pronoun (pinned 2026-09-12)

Found in the same follow-up audit. `jutsu.range = "Self"` (D&D 5e's standard range term meaning
"this jutsu only affects you") was translated to **"Eu"** ("I"/"Me," a bare first-person pronoun)
across all 363 Portuguese rows that use it — grammatically odd as a range label and not the actual
D&D term. Confirmed via web search against real Brazilian Portuguese D&D 5e spell text (multiple
independent PT-BR spell compendiums, e.g. spells with "Alcance: Pessoal") that the correct,
official term is **"Pessoal"** ("Personal") — hand-patched directly in `content_translations` for
all 363 rows rather than left as "Eu." Confirmed safe to change: `parseJutsuRange` (cmd/n5e/jutsu.go)
always parses the *raw English* `jutsu.range` value fetched before the i18n overlay is applied, so
this is a display-only fix with no effect on range-based filtering/matching.

**Spanish fixed 2026-09-12**: `jutsu.range = "Self"` was "Uno mismo" ("oneself") for all 363
Spanish rows — also confirmed via web search that the official Spanish D&D 5e term is likewise
"Personal" (alcance personal), not "Uno mismo." Hand-patched all 363 rows the same way as
Portuguese; same `parseJutsuRange`-uses-raw-English confirmation applies, so this is display-only.

## "Chakra" pluralized as "Chakras" in jutsu names — inconsistency found via a fan's independent PT-BR spreadsheet (found and patched 2026-09-28)

A fan's separate, independently-reviewed PT-BR translation spreadsheet (built for their own
adversary-generator tool, ~4,683 rows marked as human-reviewed/sourced rather than raw machine
translation) was cross-referenced against every jutsu name this project already translates, as a
sanity check on our own PT-BR quality. Of 912 jutsu names the two sources both cover, 910 already
match exactly — strong independent confirmation that the existing PT jutsu-name translations are
solid. Two disagreed:

- **"Light" → ours "Luz", fan's spreadsheet "Leve."** Checked against the actual jutsu text (a
  D&D-style "object sheds bright light" effect): "Luz" (illumination) is correct; "Leve"
  (lightweight) is a mistranslation on the fan's side. **Not changed** — this is the exact kind of
  case a "trusted reviewed source" still needs sanity-checking against actual game text before
  merging, not blind adoption.
- **"Chakra Hands" → ours "Mãos dos Chakras" (plural, "Hands of the Chakras"), fan's spreadsheet
  "Mãos de Chakra" (singular, treating Chakra as the unchanged mass-noun loanword this glossary
  already pins it as — see "Core mechanical terms" above). The fan's version is correct, and
  checking every other Chakra-prefixed jutsu name already in this project's own PT translations
  confirms it: 10 of 17 already use the correct singular form ("Golpe do Chakra," "Pulso do
  Chakra," "Pele de Chakra," etc.) — "Chakra Hands" and 6 siblings are the outliers, not the norm.

**Full list of the 7 outliers found (cross-checked 2026-09-28, confirmed against the majority
pattern already established across this project's own 17 Chakra-prefixed jutsu names — not
solely the fan's spreadsheet, which only independently corroborates the "Chakra Hands" row)**:

| Jutsu (English) | Current (wrong) | Corrected |
|---|---|---|
| Chakra Distortion Field | Campo de Distorção dos Chakras | Campo de Distorção do Chakra |
| Chakra Hands | Mãos dos Chakras | Mãos de Chakra |
| Chakra Movement | Movimento dos Chakras | Movimento do Chakra |
| Chakra-Seeking Rounds | Rodadas de Busca dos Chakras | Rodadas de Busca do Chakra |
| Chakra Shield | Escudo dos Chakras | Escudo do Chakra |
| Chakra Sphere | Esfera dos Chakras | Esfera do Chakra |
| Sealing Art: Chakra Dispel | Arte do Selamento: Dissipação dos Chakras | Arte do Selamento: Dissipação do Chakra |

**Status: hand-patched 2026-09-28**, same mechanism as the "Self"/"Weapons Range" fixes above —
direct `UPDATE` on `content_translations` (table_name='jutsu', field='name', locale='pt') across
all three `rules.db` copies (`out/`, `internal/embedded/`, `dist/`), matching the corrected values
in the table above exactly.

## Confirmed false positives — left untranslated by design (pinned 2026-09-11)

Verified during this audit's triage, each against the actual English source and (where relevant)
the Spanish glossary/database for corroboration — listed as identity pairs so a future `-force`
retranslation doesn't churn them, same treatment as "Kitsune Sessho-Seki" above.

| English | Portuguese | Source |
|---|---|---|
| Genjutsu: Sharingan! | Genjutsu: Sharingan! | `jutsu/uchiha/genjutsu-sharingan`; "Sharingan" is a kekkei genkai proper noun, same treatment as "Byakugan" |
| Genjutsu: Ketsuryūgan! | Genjutsu: Ketsuryūgan! | `jutsu/chinoike/genjutsu-ketsuryugan`; same treatment as Sharingan above |
| Tajū Kage Bunshin | Tajū Kage Bunshin | `subclass_features/class/scout-nin/group/scouting-technique/cloning-scout/feature/taju-kage-bunshin`; the actual Japanese name of the real Naruto "Shadow Clone Jutsu," not an invented game term |
| 1 Fuma Shuriken | 1 Fuma Shuriken | `class_equipment_options` (class/intelligence-operative, group 2 choice 1); "Fuma Shuriken" is a real Naruto weapon name, same treatment as "Kunai" |

**"Touch" glossary-verb false positive (found 2026-09-28, pinning "Touch"/"Toque" above)**: the
`clan/shakuton` epithet "Too Hot to Touch" → "Quente demais para tocar" trips `verify`'s
`missing-glossary-term` check, since "Touch" here is an ordinary verb, not the jutsu/weapon
range value the glossary term pins. The pin itself is correct and worth keeping — confirmed 158
`jutsu.range='Touch'` rows across the database, already consistently and correctly rendered as
"Toque" — this epithet is the one narrow collision, not a sign the pin is wrong.

**Known pre-existing inconsistency, not fixed here (out of scope for this pass)**: the
`class_options/class/scout-nin/option/trickster-maneuvers/ramp-up-*` trio ("Ramp Up: Rakukaja/
Tarukaja/Sukukaja," Shin Megami Tensei/Persona buff-spell names) is left fully untranslated in
*both* Spanish and Portuguese, consistent with this project's "stylized ability title" convention
(Shatter/Flash/Bane/etc.) — likely correct as-is, not re-verified further. Separately, the
Science-Nin "B.I.M." (bomb) entry family (`class_option_entries/class/science-nin/option/
explosive-modifications/*`) is inconsistently translated across ~19 sibling entries in *Spanish
itself* (some fully translated, some left in English) — "Blazing Gas B.I.M"/"Razor Shard B.I.M"
are two of the untranslated ones in Portuguese, matching that same pre-existing Spanish pattern
rather than a new Portuguese-only regression. Worth its own dedicated normalization pass across
both locales, not a quick glossary pin.

## One-off content fixes (not glossary terms — full phrases, not reusable)

`clan/futton` epithet: English "A Corrosive Breath" came back untranslated in Portuguese
(Spanish: "Un Aliento Corrosivo"). Hand-fixed to "Um Sopro Corrosivo" directly in the
database, matching the "Sopro" convention established across the "Breath of X" jutsu family
above. Not a table entry since it's a one-off phrase, not a reusable term.

`subclass_features/class/scout-nin/group/scouting-technique/tactical-scout/feature/unmatched-tactics`
description: a table-header phrase "TABLA DE TÁCTICAS SUPERIORES Scout-Nin" left the class name
"Scout-Nin" untranslated mid-sentence while every other Spanish occurrence of this class name
translates it to "Explorador-Nin". Hand-fixed to match. Not a table entry — a one-off stray
instance, not a systemic term-mapping gap.

`class_option_entries/genjutsu-inception` description (pt): a mid-sentence cross-reference to
the RPG-sense "Class" glossary term rendered as "aula" (school-lesson sense) instead of "Classe" —
the 18th confirmed instance of DeepL's known non-deterministic mistranslation of this pinned term
(see "Class"/"Classe" above; 17 prior instances were already patched in the pt.json UI catalog).
Hand-fixed to "classe" directly in the database. Not a table entry — the glossary term itself is
already pinned above; this is just DeepL failing to apply it in this one spot.

`class_option_entries/momentum` and `class_option_entries/bronze-tier` descriptions (pt): both
contain a mid-sentence cross-reference to the jutsu name "Shockwave Slash," left as the raw
English name instead of its established Portuguese rendering. Hand-fixed both to "Corte de Onda
de Choque" directly in the database, matching the jutsu's own translated `name` field. DeepL's
glossary application is documented as unreliable for mid-sentence proper-name cross-references
(see the technical note at the top of this file); not a new/reusable term.

`class_option_entries/falling-heaven-cross` description (pt): a mid-sentence cross-reference to
the jutsu name "Falling Heaven: Divide" left the ": Divide" half untranslated. Hand-fixed to
"Divisão" directly in the database, matching the jutsu's own translated `name` field.

`class_option_entries/spider-web-area` description (pt): a mid-sentence cross-reference to the
jutsu name "Web Bind" left untranslated in two separate phrasings within the same description.
Hand-fixed both occurrences to "Amarração da Teia" directly in the database, matching the jutsu's
own translated `name` field.

`jutsu/demonic-culling-eye-basket`, `jutsu/whirlwind-catapult`, `jutsu/wrecking-ball` `range`
fields (pt): 3 more instances of the "Weapons Range" glossary term missed by the 91-row batch fix
above — the first two used a singular "Alcance da arma" (weapon's-range sense) instead of the
plural "Alcance das armas," and `wrecking-ball` still had the original mistranslated "Campo de
Tiro" (shooting-range-facility sense). All 3 hand-fixed directly in the database to match the
pinned term.

`class_option_entries/.../magus-upgrades/bronze-tier/entry/clashing-adept`,
`class_options/.../magus-upgrades/wood-tier`, `class_options/.../magus-upgrades/bronze-tier`
descriptions (pt): mid-sentence cross-references to "Peerless Casting" rendered as "Lançamento
Incomparável" (jutsu-casting sense) instead of the entry's own pinned name "Fundição Inigualável"
(matching the sibling "Casting" family's established "Fundição" convention — see the 405-warning
triage table above). Hand-fixed all 3 directly in the database.

`class_options/.../arsenal-modifications/minor` and `.../arsenal-modifications/refined`
descriptions (pt): 2 more mid-sentence cross-references to "Vibrosteel Edge," each rendered as a
different, inconsistent guess ("Lama de Vibroaço," a likely OCR/DeepL typo, and "Lâmina de
Vibroaço") instead of the entry's own pinned name "Fio de Vibroaço." Hand-fixed both directly in
the database.

`feat/yamanaka/psycho-mind-flux` description (pt): its own self-reference to the jutsu it creates,
"Psycho Mind Flux," rendered as "Fluxo Psíquico da Mente" instead of the feat's own pinned name
"Fluxo Psicomental." Hand-fixed directly in the database.

`subclass_features/.../lightning-breaker/feature/gigawatt-molding` description (pt): its own
self-reference to "Gigawatt Molding," rendered as "Modelagem Gigawatt" instead of the feature's
own pinned name "Moldagem de Gigawatts." Hand-fixed directly in the database.

**Checker false positive, not fixed**: `class_options/.../malleable-mirages/one-with-shadows`
description (pt) trips the verify pass's "missing-glossary-term" check for "Mirage," but the
English source uses "mirage" there as an ordinary common noun ("You lose the benefit of this
mirage..."), not a reference to the pinned jutsu-name glossary term — rendered as "essa ilusão"
("this illusion"), a legitimate contextual translation of the sentence, not a missed term.
