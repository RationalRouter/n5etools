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
| Casting at higher rank ("upcasting") | Lanzado/a a un rango superior | adapted from official SRD's "lanzado a un nivel superior" — this game uses jutsu ranks (E-S), not spell levels, so "nivel" becomes "rango" |
| Mastery (this game's own stacking rank system) | Maestría | homebrew system, no D&D precedent to check against — approved as our own call |
| Class Feature | Rasgo de Clase | confirmed via the official Spanish SRD mirror's own class pages (e.g. Guerrero) and nivel20.com — NOT "Característica de Clase" (initially guessed wrong): "Característica" is already reserved for ability scores (see "Ability check → Prueba de Característica" above), so reusing it here would collide with that meaning. "Rasgo" (trait) is the correct word |

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

"X Release" (Crystal Release, Explosion Release, Wood Release, etc.) → **"Elemento X"**
(e.g. Crystal Release → Elemento Cristal, Explosion Release → Elemento Explosivo). Matches the
Spanish Narutopedia/fan-wiki convention, used consistently across every kekkei genkai type —
notably NOT matching the actual anime dub, which was inconsistent with itself (Wood Release was
rendered "jutsu arbóreo" in Part I and "jutsu lignario" in Part II of the same show). The fan-wiki
term is what Spanish-speaking players are actually likely to already know from looking up jutsu
online, so it's the better standardized choice despite not being official-dub sourced. Applies to
jutsu names across most kekkei genkai clans (Bakuton, Futton, Shoton, Ranton, Yoton, Jiton, Keton,
etc.) — this is a naming *pattern*, not a one-off term, so apply it consistently every time an
"X Release" jutsu name is translated.

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
| Yin Release | Elemento Yin | follows the already-approved "Elemento X" nature-transformation pattern |
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

| English | Spanish |
|---|---|
| Bludgeoning | Contundente |
| Piercing | Perforante |
| Slashing | Cortante |
| Fire | Fuego |
| Cold | Frío |
| Lightning | Rayo |
| Variable | Variable |

("Slashing/Piercing" is a display-time join of the two rows above, not its own label.)

**Standing rule, confirmed 2026-09-04 as a hard policy (not re-confirmed per instance going
forward): anything the player can potentially see gets translated.** This is the same policy
already applied to the `jutsu.components` labels — every time a new machine-parsed field turns up
(a fixed, small set of codes/enums mapped to display labels), the labels themselves are always
in scope for translation via the UI catalog; only the raw parsed data column itself stays
untranslated. Apply this by default from now on without asking again.

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
| Bukijutsu | *unchanged* | proper term of art, same treatment as Taijutsu/Ninjutsu/Genjutsu |
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
> hora (3 rangos de Intoxicado si se falla por 5 o más).
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
> objetivo. Impacto: 4d10+4 de daño de fuego. Con una tirada de 17 a 20, el objetivo obtiene 1 rango
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
> Ninjutsu de Fuego Tormenta de Fuego de Rango A.

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

**Not yet resolved**: `jutsu/o-encode-thoughts` ("O Encode Thoughts") — the English source
name itself looks malformed/ambiguous (stray leading "O"?), so no Portuguese translation was
guessed here; check with Sergio whether the English name is correct before translating it.

## One-off content fixes (not glossary terms — full phrases, not reusable)

`clan/futton` epithet: English "A Corrosive Breath" came back untranslated in Portuguese
(Spanish: "Un Aliento Corrosivo"). Hand-fixed to "Um Sopro Corrosivo" directly in the
database, matching the "Sopro" convention established across the "Breath of X" jutsu family
above. Not a table entry since it's a one-off phrase, not a reusable term.
