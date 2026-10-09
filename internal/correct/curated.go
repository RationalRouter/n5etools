package correct

import "strings"

// curatedCorrection is one hand-verified, context-dependent prose fix that
// can't be expressed as a safe general rule — unlike the systematic
// heuristics elsewhere in this package (misspell, apostrophe, hyphenspace,
// possessive, wordconfusion), each of these is a single specific sentence
// in a single specific entry. Every Original string below was pulled
// directly from dist/rules.db (full-corpus audit, 2026-08-05) and is long
// enough to be a practical non-collision with unrelated prose elsewhere in
// the corpus. Keys are written against the text AFTER the other Sweep
// passes have already run (see the pass order in Sweep) — where a
// correction's original phrase overlaps ground the hyphenspace/possessive/
// wordconfusion passes already cover (e.g. "creatures Hit Points" ->
// "creature's Hit Points"), the Original string here already reflects that
// post-fix form, since curatedFix runs last.
type curatedCorrection struct {
	Original, Corrected string
}

var curatedCorrections = []curatedCorrection{
	// --- classes ---
	{"different as they might be been connected by one common factor",
		"different as they might be, are connected by one common factor"},
	{"Shisui Uchiha and Minato Namikaze When creating a Hunter-Nin consider",
		"Shisui Uchiha and Minato Namikaze. When creating a Hunter-Nin consider"},
	{"Kabuto Yakushi, Orochimaru & Shizune When creating a Medical-Nin consider",
		"Kabuto Yakushi, Orochimaru & Shizune. When creating a Medical-Nin consider"},
	{"Hinata Hyūga, and Killer B When creating a Taijutsu Specialist consider",
		"Hinata Hyūga, and Killer B. When creating a Taijutsu Specialist consider"},
	{"Darui, and Sasuke Uchiha  When creating a Weapon Specialist consider",
		"Darui, and Sasuke Uchiha. When creating a Weapon Specialist consider"},
	{"Kabuto Yakushi & Orochimaru. INTELLIGENCE OPERATIVE When creating an Intelligence Operative consider",
		"Kabuto Yakushi & Orochimaru. When creating an Intelligence Operative consider"},
	{"your attack and damage rolls. creatures that are not proficient with a cooking tool",
		"your attack and damage rolls. Creatures that are not proficient with a cooking tool"},

	// --- subclasses / class options (bronze-tier upgrades etc.) ---
	{"The jutsu gains gains the benefits of being Overcharged.",
		"The jutsu gains the benefits of being Overcharged."},
	{"this also also shrinks and enlarges the creature",
		"this also shrinks and enlarges the creature"},
	{"the Reinforced Property, with a amount equal to your Intelligence Modifier",
		"the Reinforced Property, with an amount equal to your Intelligence Modifier"},
	{"As an Bonus action you spend the CCD Drain of this enhancement to spin",
		"As a Bonus action you spend the CCD Drain of this enhancement to spin"},
	{"automatically explode when a creature enters with 10 feet of the B.I.M",
		"automatically explode when a creature enters within 10 feet of the B.I.M"},
	{"When denotated creatures within a 5-foot-radius sphere of the B.I.M",
		"When detonated, creatures within a 5-foot-radius sphere of the B.I.M"},
	{"the range you can throw B.I. Ms.", "the range you can throw B.I.Ms."},
	{"this B.I.M when it explodes must a Dexterity saving throw",
		"this B.I.M when it explodes must make a Dexterity saving throw"},
	{"remove the B.I.M as a action by making a Athletics check",
		"remove the B.I.M as an action by making an Athletics check"},
	{"You can roll a number of your hit dice, up to half. and give that amount of hit points",
		"You can roll a number of your hit dice, up to half, and give that amount of hit points"},
	{"succeed a Investigation or Perception check", "succeed an Investigation or Perception check"},
	{"All melee attacks this Corpse makes have a extended reach of 5 feet",
		"All melee attacks this Corpse makes have an extended reach of 5 feet"},
	{"would hit the target with a unarmed", "would hit the target with an unarmed"},
	{"your Cooking Tool gains a additional weapon property",
		"your Cooking Tool gains an additional weapon property"},
	{"your cooking tool gains a additional weapon property",
		"your cooking tool gains an additional weapon property"},
	{"turned into a Icon for your team", "turned into an Icon for your team"},
	{"spend 5 charges of a Armorsmith Kit", "spend 5 charges of an Armorsmith Kit"},
	{"When you use a Enhancement with the Trick", "When you use an Enhancement with the Trick"},
	{"how many of each B.I.M type you hold a on long rest",
		"how many of each B.I.M type you hold on a long rest"},
	{"your inhalants can affect even affect those who believe themselves to be immune",
		"your inhalants can affect even those who believe themselves to be immune"},
	{"now apply to any number creatures of your choice", "now apply to any number of creatures of your choice"},

	// --- clans ---
	{"Demons Second Coming", "Demon’s Second Coming"},
	{"The Villages Strongest", "The Village’s Strongest"},
	{"it has brough about an era of prosperity", "it has brought about an era of prosperity"},
	{"each one brough about a new era of prosperity", "each one brought about a new era of prosperity"},
	{"contracts with unknown evils from different realms to preform malicious jutsu",
		"contracts with unknown evils from different realms to perform malicious jutsu"},
	{"At 3rd Level, the gain access to a special form of military food pills",
		"At 3rd Level, they gain access to a special form of military food pills"},
	{"you can select a another Genjutsu of D-Rank or lower", "you can select another Genjutsu of D-Rank or lower"},
	{"You gain a Burrowing Speed equal of 15 feet", "You gain a Burrowing Speed equal to 15 feet"},
	{"Starting, at 11rd level, you tap into your innate bloodline limit",
		"Starting, at 11th level, you tap into your innate bloodline limit"},
	{"Beginning at 11thh level, you may select a second Nindo",
		"Beginning at 11th level, you may select a second Nindo"},
	{"further increased to+4 at 11th", "further increased to +4 at 11th"},
	{"place your palm on an object no larger than10 feet in any dimension",
		"place your palm on an object no larger than 10 feet in any dimension"},
	{"the object sheds bright light in a20-foot radius", "the object sheds bright light in a 20-foot radius"},
	{"increase the duration of this feature on an object effected from 10 minutes to1 hour",
		"increase the duration of this feature on an object effected from 10 minutes to 1 hour"},
	{"Beginning at level 11th level, constructs created with the Earth Release keyword",
		"Beginning at 11th level, constructs created with the Earth Release keyword"},
	{"due to you to your natural gift for speed", "due to your natural gift for speed"},
	{"taking 4d6 fire damage or half as much on a successful. If used with a Jutsu that affects an area, you instead increase the Jutsu's range by 10 feet",
		"taking 4d6 fire damage or half as much on a successful one. If used with a Jutsu that affects an area, you instead increase the Jutsu's range by 10 feet"},
	{"Weapons with the Ammunition property that you creature do not need to roll ammunition die",
		"Weapons with the Ammunition property that you create do not need to roll ammunition die"},
	{"Beginning at 18th level, you ignore treat immunity to Fire damage as resistance",
		"Beginning at 18th level, you treat immunity to Fire damage as resistance"},
	{"you gain a bonus equal to half of your Wisdom modifier as you can foresee a creature’ intentions",
		"you gain a bonus equal to half of your Wisdom modifier as you can foresee a creature’s intentions"},

	// --- feats ---
	{"Increase your Intelligence score by 1, to a maximum of • Constructs and structures you summon",
		"Increase your Intelligence score by 1, to a maximum of 20. • Constructs and structures you summon"},
	{"Increase your Intelligence score by 1, to a maximum of • When you are near a sufficient source of water",
		"Increase your Intelligence score by 1, to a maximum of 20. • When you are near a sufficient source of water"},
	{"Increase your Intelligence score by 1, to a maximum of • When you would trigger a Swirl effect",
		"Increase your Intelligence score by 1, to a maximum of 20. • When you would trigger a Swirl effect"},
	{"When you would choose to deal damage to a creature hit points, you increase your",
		"When you would choose to deal damage to a creature's hit points, you increase your"},
	{"you gain a number of temporaries hit points equal to the number of healing dice rolled",
		"you gain a number of temporary hit points equal to the number of healing dice rolled"},
	{"as a hive, it's queen, and its army", "as a hive, its queen, and its army"},
	{"Increase your Wisdom score by 1, to a maximum 20.", "Increase your Wisdom score by 1, to a maximum of 20."},
	{"select a number of Genjutsu you know equal to your Wisdom or Charisma modifier (Max. 4) and make an Wisdom or Charisma (Illusions) check",
		"select a number of Genjutsu you know equal to your Wisdom or Charisma modifier (Max. 4) and make a Wisdom or Charisma (Illusions) check"},

	// --- backgrounds ---
	{"damaging to the people who or consigned you to exile", "damaging to the people who consigned you to exile"},
	{"• Select Gain a feat from Chapter 13: Customization options", "• Gain a feat from Chapter 13: Customization options"},
	{"some things its people take for granted will be to you knew wonders",
		"some things its people take for granted will be to you new wonders"},
	{"a poorly written map from your homeland that depict where you are in the world",
		"a poorly written map from your homeland that depicts where you are in the world"},

	// --- jutsu ---
	{"they appear as close as possible to to intrude and attack it",
		"they appear as close as possible to intrude and attack it"},
	{"must make an Intelligence Saving Throw throw at disadvantage",
		"must make an Intelligence Saving Throw at disadvantage"},
	{"creating a shockwave of force that that expands outward", "creating a shockwave of force that expands outward"},
	{"All creature in in range must make a Dexterity Saving Throw",
		"All creatures in range must make a Dexterity Saving Throw"},
	{"once per turn, you can force them to make Strength Saving Throw",
		"once per turn, you can force them to make a Strength Saving Throw"},
	{"After, all creatures within 5 feet of you must make Constitution Saving Throw",
		"After, all creatures within 5 feet of you must make a Constitution Saving Throw"},
	{"When a or a creature you can see within range would take damage",
		"When you or a creature you can see within range would take damage"},
	{"targeting another creature other then you", "targeting another creature other than you"},
	{"before refusing back together with you, you do not regain", "before fusing back together with you, you do not regain"},
	{"recast this jutsu while touching it, you refuse with it as normal",
		"recast this jutsu while touching it, you fuse with it as normal"},
	{"the creatures are thrown 10 feet away into a pace that can hold them",
		"the creatures are thrown 10 feet away into a place that can hold them"},
	{"locking them in to a deadly waltz with you", "locking them into a deadly waltz with you"},
	{"against all creature who are experiencing bouts of fear",
		"against all creatures who are experiencing bouts of fear"},
	{"All creature in a 45-foot cone originating from you", "All creatures in a 45-foot cone originating from you"},
	{"it’s damage type becomes the corresponding nature releases damage type (Water = Cold)",
		"its damage type becomes the corresponding nature release’s damage type (Water = Cold)"},
	{"enhance its ability to fly far and it’s damage power", "enhance its ability to fly far and its damage power"},
	{"as an action, make a Acrobatics or Sleight of Hand Check",
		"as an action, make an Acrobatics or Sleight of Hand Check"},
	{"Each of the selected creatures must make a Intelligence Saving Throw",
		"Each of the selected creatures must make an Intelligence Saving Throw"},
	{"with an Quake Shard that’s 10 feet tall", "with a Quake Shard that’s 10 feet tall"},
	{"Spend a number of chakras die up to a max of an amount equal to twice your proficiency bonus",
		"Spend a number of chakra die up to a max of an amount equal to twice your proficiency bonus"},
	{"The target must spend a equal number of chakras die",
		"The target must spend an equal number of chakra die"},
	{"within 10-fee of the original target", "within 10-feet of the original target"},
	{"you can move the weapon 30 feet an make a Melee Ninjutsu Attack",
		"you can move the weapon 30 feet and make a Melee Ninjutsu Attack"},
	{"This jutsu’s effects end immediately if the take damage or are knocked prone",
		"This jutsu’s effects end immediately if they take damage or are knocked prone"},
	{"Critical Failure: Same effects a failure, but the target is also Incapacitated",
		"Critical Failure: Same effects as a failure, but the target is also Incapacitated"},
	{"this jutsu gains a single nature release keyword that you can cast, and additional effects base on the Nature release",
		"this jutsu gains a single nature release keyword that you can cast, and additional effects based on the Nature release"},
	{"Make a Melee Taijutsu Attack. On a hit deal your Weapon’s Damage. You then can choose to keep it there",
		"Make a Melee Taijutsu Attack. On a hit, you deal your Weapon’s Damage. You then can choose to keep it there"},
	{"Make a Melee Taijutsu Attack against a creature in range. On a hit deal your Weapon’s Damage, and they become Restrained",
		"Make a Melee Taijutsu Attack against a creature in range. On a hit, you deal your Weapon’s Damage, and they become Restrained"},
	{"Make two Melee Taijutsu Attack. On a hit, you your Weapon’s Damage + 2d6",
		"Make two Melee Taijutsu Attacks. On a hit, you deal your Weapon’s Damage + 2d6"},
	{"you impost an effect based on the damage type of the weapon used",
		"you impose an effect based on the damage type of the weapon used"},
	{"Make a two Ranged Taijutsu Attacks against one creature you can see within range",
		"Make two Ranged Taijutsu Attacks against one creature you can see within range"},
	{"You make a Melee Ninjutsu Attack on a target creature or object. On a hit creature take 3d6 Wind Damage",
		"You make a Melee Ninjutsu Attack on a target creature or object. On a hit, the creature takes 3d6 Wind Damage"},
	{"On a failed save creature take 11d8+11 Fire Damage and gaining 3 ranks of Burned",
		"On a failed save, the creature takes 11d8+11 Fire Damage and gains 3 ranks of Burned"},
	{"Critical Failure: Affected creatures takes 3d12 + Genjutsu Ability modifier psychic damage and loses concentration",
		"Critical Failure: Affected creatures take 3d12 + Genjutsu Ability modifier psychic damage and lose concentration"},
	{"On a failed save select creatures takes Xd4+Taijutsu ability modifier",
		"On a failed save, the selected creatures take Xd4+Taijutsu ability modifier"},

	// --- equipment (enhancement seals) ---
	{"for the purposed of overcoming resistance and capitalizing on Vulnerabilities",
		"for the purpose of overcoming resistance and capitalizing on Vulnerabilities"},

	// --- fighting stances ---
	{"you can use a bonus action to make a one melee weapon attack",
		"you can use a bonus action to make one melee weapon attack"},
	{"These attacks are still three different instances of damage, but The damage reduction applied to the total value, once.",
		"These attacks are still three different instances of damage, but the damage reduction is applied to the total value, once."},
	{"These attacks are still two different instances of damage, but The damage reduction applied to the total value, once.",
		"These attacks are still two different instances of damage, but the damage reduction is applied to the total value, once."},
	{"as your stab into an opponent’s pressure points", "as you stab into an opponent’s pressure points"},
	{"or other special senses that doesn't rely on sight", "or other special senses that don't rely on sight"},
	{"Each strike aiming for a creature sensitive organs attempting to cause bludgeoning damage",
		"Each strike aiming for a creature's sensitive organs attempting to cause bludgeoning damage"},

	// --- summon tribes ---
	{"Dear are timid, docile creatures", "Deer are timid, docile creatures"},
	{"Deer’s have access to any Jutsu", "Deer have access to any Jutsu"},
	{"Hare/ have access to any Jutsu", "Hare/Rabbit have access to any Jutsu"},
	{"Turtle have access to any Jutsu", "Turtles have access to any Jutsu"},
	{"Weasel have access to any Jutsu", "Weasels have access to any Jutsu"},
	{"Ninjutsu with without any nature release keyword", "Ninjutsu without any nature release keyword"},
	{"They serve summoner’s that appreciate methodic planning", "They serve summoners that appreciate methodic planning"},
	{"For the duration of this stance, to summon cannot be attacked with advantage",
		"For the duration of this stance, this summon cannot be attacked with advantage"},
	{"For the duration of this stance, to summon ignores Resistance",
		"For the duration of this stance, this summon ignores Resistance"},
	{"this summons slams it’s hooves into the ground", "this summon slams its hooves into the ground"},
	{"Creatures who’s space it passes through must make a Strength Saving Throw",
		"Creatures whose space it passes through must make a Strength Saving Throw"},
	{"as a result of a bite attack swallows them whole, fully consuming them",
		"as a result of a bite attack, it swallows them whole, fully consuming them"},
	{"to summon 4 Broodling’s as found in the Tsuchigumo clan description",
		"to summon 4 Broodlings as found in the Tsuchigumo clan description"},
	{"These Broodling’s can be commanded by this summons, summoner at no action cost",
		"These Broodlings can be commanded by this summon’s summoner at no action cost"},
	{"When this summons, summoner casts a jutsu. This summon can spend a Jutsu slot",
		"When this summon’s summoner casts a jutsu, this summon can spend a Jutsu slot"},
	{"This summons, summoner can spend 1 chakra die.", "This summon’s summoner can spend 1 chakra die."},
	{"When this summons, summoner makes a Saving Throw, it adds the summons Strength modifier to the save",
		"When this summon’s summoner makes a Saving Throw, it adds the summon’s Strength modifier to the save"},
	{"When this summons, summoner deals damage with a melee attack, it adds the summons Strength modifier to the damage dealt",
		"When this summon’s summoner deals damage with a melee attack, it adds the summon’s Strength modifier to the damage dealt"},
	{"When this summons, summoner casts a Taijutsu or Bukijutsu of B-Rank or higher",
		"When this summon’s summoner casts a Taijutsu or Bukijutsu of B-Rank or higher"},
	{"This summons, summoner can use this summons Saving throw bonuses, in place of their own",
		"This summon’s summoner can use this summon’s Saving throw bonuses, in place of their own"},
	{"before the beginning of this summons, summoner’s turn", "before the beginning of this summon’s summoner’s turn"},

	// --- adversaries ---
	{"When this Adversary cast a Jutsu that requires an attack roll",
		"When this Adversary casts a Jutsu that requires an attack roll"},
	{"When this Adversary cast a Ninjutsu or Genjutsu while affected by an Elemental condition",
		"When this Adversary casts a Ninjutsu or Genjutsu while affected by an Elemental condition"},
	{"If this Adversary is takes damage equal to its level, this trait is suppressed",
		"If this Adversary takes damage equal to its level, this trait is suppressed"},
	{"it instead remains standing. But is unable to Attack, cast jutsu, consume items",
		"it instead remains standing, but is unable to Attack, cast jutsu, consume items"},

	// --- bloodline latents ---
	{"Beginning at 11th level, The range increases to 1 mile.", "Beginning at 11th level, the range increases to 1 mile."},
	{"you can spend convert 6 calories to end either the Shocked or Chilled conditions",
		"you can convert 6 calories to end either the Shocked or Chilled conditions"},
	{"You must have Latent Food Insolation I", "You must have Latent Fat Insulation I"},
	{"Beginning at 11st level, ranged attacks with shuriken", "Beginning at 11th level, ranged attacks with shuriken"},
	{"Beginning at 18st level, ranged attacks with shuriken", "Beginning at 18th level, ranged attacks with shuriken"},
	{"Beginning at 11rd level, you can now remake the attack roll", "Beginning at 11th level, you can now remake the attack roll"},
	{"You must have Latent Typhoon Go with the Flow I", "You must have Latent Go with the Flow I"},
	{"You must have Latent Typhoon Go with the Flow II", "You must have Latent Go with the Flow II"},
	{"if you would cast a jutsu with the Earth Release or Wind Release keywords, you can, you deal a bonus damage die to creatures who’s movement speed is currently 0",
		"if you would cast a jutsu with the Earth Release or Wind Release keywords, you can deal a bonus damage die to creatures whose movement speed is currently 0"},
	{"You must have Latent Chakra Adept II", "You must have Latent Yin Chakra Adept II"},
	{"You must have Master of Shadows I1", "You must have Latent Master of Shadows I"},
	{"you gain the unique benefits associated with your damage type as if you wereth level.",
		"you gain the unique benefits associated with your damage type as if you were 7th level."},
	{"Beginning at 7rd level, you can use Constitution as your Ninjutsu ability modifier",
		"Beginning at 7th level, you can use Constitution as your Ninjutsu ability modifier"},
	{"Beginning at 7rd level, you gain 3 Scorch die (d4)", "Beginning at 7th level, you gain 3 Scorch die (d4)"},
	{"Beginning at 15rd level, you gain 2 additional Scorch die.", "Beginning at 15th level, you gain 2 additional Scorch die."},
	{"Beginning at 15rd level, the reduction transforms and becomes resistance",
		"Beginning at 15th level, the reduction transforms and becomes resistance"},
	{"Beginning at 15rd level, ranged weapon or taijutsu attacks ignore resistance",
		"Beginning at 15th level, ranged weapon or taijutsu attacks ignore resistance"},

	// --- from internal/i18n/source_text_grammar_audit.md, 2026-09-04/05 ---

	// jutsu
	{"If you was the target of a Genjutsu Attack", "If you were the target of a Genjutsu Attack"},
	{"with an +1 bonus to the attacks critical threat range", "with a +1 bonus to the attacks critical threat range"},
	{"can facilitate a flowers growth", "can facilitate a flower's growth"},
	{"Creatures in the target area must make a Intelligence Saving Throw, being under the effects",
		"Creatures in the target area must make an Intelligence Saving Throw, being under the effects"},
	{"All creatures within 60 feet of the flower of your choice must make a Intelligence Saving Throw",
		"All creatures within 60 feet of the flower of your choice must make an Intelligence Saving Throw"},

	// bloodline_latents
	{"which can only be spent as a Action, action on your turn",
		"which can only be spent as an Action on your turn"},

	// clan_traits
	{"You Nin-Dog knows 1 Inuzuka Clan D-Rank Jutsu", "Your Nin-Dog knows 1 Inuzuka Clan D-Rank Jutsu"},

	// clans (overview)
	{"they can manifest something greater. A powerful nature release transformation known as Data Release, which possesses a unique affinity for technology.",
		"they can manifest something greater, a powerful nature release transformation known as Data Release, which possesses a unique affinity for technology."},
	{"The Ranton Clan is a clan that hails proudly from the Land of Lightning. Known to work closely with the Raikage, but not limited to that one country.",
		"The Ranton Clan is a clan that hails proudly from the Land of Lightning. They are known to work closely with the Raikage, but are not limited to that one country."},
	{"Even three of its Hokage has been Senju by birth", "Even three of its Hokage have been Senju by birth"},
	{"A Clan without a clear family name, but one which has produced many of prominent shinobi.",
		"A Clan without a clear family name, but one which has produced many prominent shinobi."},

	// backgrounds
	{"so you can keep up you’re training on your own time", "so you can keep up your training on your own time"},
	{"You were taken from your home to be trained either by the Master who found you.",
		"You were taken from your home to be trained by the Master who found you."},
	{"1 Blank Jutsu Scrolls, A Wallet Containing 100 Ryo", "1 Blank Jutsu Scroll, A Wallet Containing 100 Ryo"},

	// feats
	{"If you roll higher then their attack roll AND hit", "If you roll higher than their attack roll AND hit"},
	{"If they roll higher then you, you gain the Defeated condition", "If they roll higher than you, you gain the Defeated condition"},

	// fighting_stances
	{"capable to striking enemies at a slight distance from you", "capable of striking enemies at a slight distance from you"},

	// summon_tribe_attacks
	{"Ranged Weapon Attack: Reach 30ft., On a hit target must make a Strength Saving Throw or be",
		"Ranged Weapon Attack: Reach 30ft., On a hit target must make a Strength Saving Throw or be Restrained."},
	{"the target is knocked Prone or drug 5 feet in any direction to summon wants",
		"the target is knocked Prone or drug 5 feet in any direction the summon wants"},

	// classes — systemic comma splice in the "connected by one common
	// factor" sentence template, 8 of 10 class intros (the other 2 already
	// use a colon, which is the correct version).
	{"one common factor, they", "one common factor: they"},
	{"With a focused eye on the wanted criminal who fled the village. A masked Hyūga activates his Byakugan",
		"With a focused eye on the wanted criminal who fled the village, a masked Hyūga activates his Byakugan"},
	{"Slyly and gracefully entering unseen, and she raises from the floor",
		"Slyly and gracefully entering unseen, she raises from the floor"},
	{"with the edge of a blade of the knock of a bow, the song of a flying shuriken or the lethality of a flying kunai",
		"with the edge of a blade, the knock of a bow, the song of a flying shuriken, or the lethality of a flying kunai"},
	{"Second, choose any clan that you find the most appealing, this class is flexible enough to support them.",
		"Second, choose any clan that you find the most appealing; this class is flexible enough to support them."},

	// subclasses
	{"The Ninjutsu Specialist who chose to specialize in the Hemomantic Traditions, learn to use the chakra inherent in a creatures blood",
		"The Ninjutsu Specialist who chose to specialize in the Hemomantic Traditions, learns to use the chakra inherent in a creature's blood"},
	{"Genjutsu Specialist who pledge themselves to the art of Layered Reality",
		"The Genjutsu Specialist who pledges themselves to the art of Layered Reality"},
	{"Genjutsu Specialist who choose to become Misty Distortionists specialize in inhaled toxins",
		"The Genjutsu Specialist who chooses to become a Misty Distortionist specializes in inhaled toxins"},
	{"Genjutsu Specialist who choose to become a Time Slipper, dedicate themselves",
		"The Genjutsu Specialist who chooses to become a Time Slipper dedicates themselves"},
	{"Genjutsu Specialist who choose to become a Siren dedicate themselves",
		"The Genjutsu Specialist who chooses to become a Siren dedicates themselves"},

	// subclass_features
	{"Taijutsu Specialists who take specialize in multiple stances, are known as",
		"Taijutsu Specialists who specialize in multiple stances are known as"},

	// class_options / class_option_entries
	{"You craft a another set of Juggernaut Armor for an ally during a full rest",
		"You craft another set of Juggernaut Armor for an ally during a full rest"},
	// Article fix only — the audit flagged the rest of this sentence as
	// possibly needing a larger rewrite it wasn't confident enough to
	// suggest, so only the confirmed "a"->"an" error is corrected here.
	{"When you are a allied creature you can see makes a Ranged attack roll",
		"When you are an allied creature you can see makes a Ranged attack roll"},

	// --- second pass, from the spelling-grammar-checker subagent, 2026-09-05 ---

	// jutsu — also fixes a LanguageTool auto-apply regression recorded in
	// text_corrections (SUBJECT_VERB_AGREEMENT_PLURAL flipped a correct
	// "explodes" into "explode"): this curated fix runs after LanguageTool,
	// so it wins deterministically every ingest regardless of what LT does.
	{"right before take damage. The compressed air compresses then explode pushing",
		"right before it takes damage. The compressed air compresses then explodes, pushing"},
	{"as your release a surge of Earth Release chakra", "as you release a surge of Earth Release chakra"},
	{"that then explode. At random. All creatures within range",
		"that then explode at random. All creatures within range"},
	{"On a failed save a creature roll 1d4+2 Recording the result.",
		"On a failed save a creature rolls 1d4+2, recording the result."},
	{"If you make a 11th mark then one of the previous seals vanish.",
		"If you make an 11th mark, then one of the previous seals vanishes."},
	{"so long as they know the location of the target seal's destination, and you have opened your next work to them",
		"so long as they know the location of the target seal's destination, and you have opened your network to them"},
	{"1 Reaction, which you take when an hostile creature gains", "1 Reaction, which you take when a hostile creature gains"},
	{"You manifest your Lightning release until its covering your body",
		"You manifest your Lightning release until it's covering your body"},
	{"You manifest your Lightning release until its covering the body of an allied creature",
		"You manifest your Lightning release until it's covering the body of an allied creature"},
	{"your body is incapacitated and unaware of its surrounding.",
		"your body is incapacitated and unaware of its surroundings."},

	// feats
	{"that creates an quake shard you gain +1 concentration slots",
		"that creates a quake shard you gain +1 concentration slots"},

	// class_features
	{"help compliment the form you’ve chosen", "help complement the form you’ve chosen"},
	{"Starting at 10th Level, you're Healing Jutsu are more effective",
		"Starting at 10th Level, your Healing Jutsu are more effective"},
	{"you're training with unarmed techniques have granted you a technique",
		"your training with unarmed techniques has granted you a technique"},

	// subclass_features
	{"You may select a second target for this jutsu to effect, in addition to yourself",
		"You may select a second target for this jutsu to affect, in addition to yourself"},

	// class_options / class_option_entries (duplicate text, same fix needed in both tables)
	{"and you're carrying capacity and the weight you can push, drag, or lift doubles",
		"and your carrying capacity and the weight you can push, drag, or lift doubles"},
	{"you can spend additional CCD Chakra.  they instead gain four times as many",
		"you can spend additional CCD Chakra; they instead gain four times as many"},
	{"For the same duration You can walk or run across any liquid as if it were solid stone",
		"For the same duration, you can walk or run across any liquid as if it were solid stone"},
	{"engineered to ricochet of its targets You have proficiency in this weapon",
		"engineered to ricochet off its targets. You have proficiency in this weapon"},
	{"imputing it with the psychological and psychological properties of your element",
		"imbuing it with the physical and psychological properties of your element"},
	{"it begins to gently glow while and produce an numbing psychedelic mist",
		"it begins to gently glow and produce a numbing psychedelic mist"},

	// equipment (enhancement seals) — every sibling row of the same tier
	// says "imbued with this seal", confirming the word "seal"/"this seal"
	// was dropped from just these rows.
	{"Armor imbued with seal has a 1d6 bonus to saving throws made to resist being moved, pushed or pulled",
		"Armor imbued with this seal has a 1d6 bonus to saving throws made to resist being moved, pushed or pulled"},
	{"Armor imbued with seal has a 1d6 bonus to saving throws made to resist the Fear condition",
		"Armor imbued with this seal has a 1d6 bonus to saving throws made to resist the Fear condition"},
	{"Armor imbued with seal has a 1d6 bonus to saving throws made to resist the Burned condition",
		"Armor imbued with this seal has a 1d6 bonus to saving throws made to resist the Burned condition"},
	{"Armor imbued with this grants its user a +2 bonuses to Chakra Control Checks",
		"Armor imbued with this seal grants its user a +2 bonuses to Chakra Control Checks"},
	{"Armor imbued with this grants its user a +4 bonuses to Chakra Control Checks",
		"Armor imbued with this seal grants its user a +4 bonuses to Chakra Control Checks"},

	// summon_tribes — the original finding that prompted this whole audit
	// (see internal/i18n/translation_methodology.md and
	// internal/i18n/source_text_grammar_audit.md's "Original bug" section):
	// a dangling clause with no verb attaching it to the sentence.
	{"The snake’s greatest weapon is its cunning and powerful body with its sharp fangs to pierce the flesh of its enemies.",
		"The snake’s greatest weapon is its cunning and powerful body. Its sharp fangs are used to pierce the flesh of its enemies."},

	// jutsu — same "a +N bonuses" shape as the systemic bonusPluralRe
	// heuristic, but missing the leading "a" entirely, so the regex (which
	// requires "a " before the number) doesn't match these two.
	{"you gain +10 bonuses to your movement speed until the end of the turn you cast it",
		"you gain a +10 bonus to your movement speed until the end of the turn you cast it"},
	{"You gain +3 bonuses to AC and resistance to Bludgeoning, piercing and slashing damage",
		"You gain a +3 bonus to AC and resistance to Bludgeoning, piercing and slashing damage"},
	{"you can make a third attack against a creature in range, this time with an additional -2 penalties.",
		"you can make a third attack against a creature in range, this time with an additional -2 penalty."},

	// --- third pass, from the pt/es glossary drift investigation, 2026-09-13 ---
	// Keys here are written against the text AFTER straycaps has run (pass
	// 9), which is why "Fires" already appears lowercased below.
	{"creating a shockwave the fires out in all directions overwhelming all enemies",
		"creating a shockwave that fires out in all directions, overwhelming all enemies"},
	{"to Succeed a Constitution saving throw", "to succeed on a Constitution saving throw"},
	{"they must Succeed a Constitution saving throw", "they must succeed on a Constitution saving throw"},
	{"and must Succeed a Constitution saving throw", "and must succeed on a Constitution saving throw"},
	// Dust Spear deals three separate damage types (piercing, earth,
	// bludgeoning — see the jutsu's own description); the missing serial
	// commas made "Piercing earth" read as one compound, which is exactly
	// how the Portuguese translated it ("dano de perfuração na terra").
	{"increase the Piercing earth and bludgeoning damage by 1d8 each",
		"increase the piercing, earth, and bludgeoning damage by 1d8 each"},
	{"the bonus to damage from second instance of bruised is halved",
		"the bonus to damage from the second instance of Bruised is halved"},
	// The ordinary noun, not the Light weapon property — the capital made
	// the glossary check read it as the mechanic.
	{"you emit a painfully bright Light.", "you emit a painfully bright light."},

	// --- fourth pass, the stray-capitalization audit, 2026-09-13 ---
	//
	// Sentences the sourcebook runs together with no terminal punctuation at
	// all, which is why the following word's capital is the only thing marking
	// the break. The general commacaps/midcaps rules cannot help here: there is
	// no comma to anchor on, and lowercasing the capital would turn a missing
	// period into a run-on rather than fixing anything. Each needs a human to
	// decide whether the break is a period (two independent clauses) or a comma
	// (a level/timing header the book writes as a fragment).
	//
	// Keys for the five entries the midcaps pass reaches first are written
	// against its output, not the raw corpus text — curatedFix runs last.
	{"pushed back 10 feet If used near a sufficient source of water",
		"pushed back 10 feet. If used near a sufficient source of water"},
	{"visual limitations upon them select one hostile creature",
		"visual limitations upon them. Select one hostile creature"},
	{"spend your action to make a melee or Ranged Genjutsu Attack",
		"spend your action to make a melee or ranged Genjutsu Attack"},
	{"use it to fuel your attack If this is in Reaction to an ally",
		"use it to fuel your attack. If this is in Reaction to an ally"},
	{"feel the moonlight on your skin Your arrow glows silver",
		"feel the moonlight on your skin. Your arrow glows silver"},
	{"1 rank of Bleeding for each failure If this jutsu is cast as a Chain 2",
		"1 rank of Bleeding for each failure. If this jutsu is cast as a Chain 2"},
	{"until the end of your next turn If this jutsu is cast as a Chain 2",
		"until the end of your next turn. If this jutsu is cast as a Chain 2"},
	{"impenetrable barrier of steel and chain When you or an ally",
		"impenetrable barrier of steel and chain. When you or an ally"},
	{"Strength based ability and skill checks When this jutsu ends",
		"Strength based ability and skill checks. When this jutsu ends"},
	{"their chakra efficiency or their chakra recovery If you choose to aid",
		"their chakra efficiency or their chakra recovery. If you choose to aid"},
	{"symbolizing your Chakra. For the duration increase your AC by +1",
		"symbolizing your Chakra. For the duration, increase your AC by +1"},
	{"it becomes a +2 weapons If cast at S-Rank",
		"it becomes a +2 weapon. If cast at S-Rank"},
	{"Reality Marble Genjutsu pledge When you are in an area",
		"Reality Marble Genjutsu pledge. When you are in an area"},
	{"contested by the target’s Charisma Saving throw If you have Mastery",
		"contested by the target’s Charisma Saving throw. If you have Mastery"},
	{"any Armory upgrades. If taken using If taken using a Silver or higher tier slot",
		"any Armory upgrades. If taken using a Silver or higher tier slot"},
	{"cast Chakra Mark to instead have Your seals take the shape",
		"cast Chakra Mark to instead have your seals take the shape"},
	{"you can pay the Drain cost and Your weapon gains the Double property",
		"you can pay the Drain cost and your weapon gains the Double property"},
	{"It is not considered concentrated on However it is only last until the end of your next turn",
		"It is not considered concentrated on. However, it only lasts until the end of your next turn"},
	{"It is not considered concentrated on However it only lasts until the end of your next turn",
		"It is not considered concentrated on. However, it only lasts until the end of your next turn"},
	{"per short rest.  When active This tool has a sensor",
		"per short rest. When active, this tool has a sensor"},
	{"does not cost chakra to concentrate however If you are attacked",
		"does not cost chakra to concentrate. However, if you are attacked"},
	{"Make a Ninjutsu Attack roll This attack deals Force damage",
		"Make a Ninjutsu Attack roll. This attack deals Force damage"},
	{"Beginning at 15th level When you would pass a saving throw",
		"Beginning at 15th level, when you would pass a saving throw"},
	{"At 7th level When you would make a Dexterity, Wisdom, or Charisma",
		"At 7th level, when you would make a Dexterity, Wisdom, or Charisma"},
	{"You can only take this ability If you already have Sharingan Dodge 2",
		"You can only take this ability if you already have Sharingan Dodge 2"},
	{"You can only take this ability If you already have Aggressive Jutsu Augmentation",
		"You can only take this ability if you already have Aggressive Jutsu Augmentation"},
	{"all 3 prerequisite genjutsu at once If you would end the effect",
		"all 3 prerequisite genjutsu at once. If you would end the effect"},
	{"advantage on attack rolls against a creature If at least one of your allies",
		"advantage on attack rolls against a creature if at least one of your allies"},
	{"while Hidden does not automatically reveal you This includes attacks",
		"while Hidden does not automatically reveal you. This includes attacks"},
	{"+2 bonuses to Saving throws The number of bonuses you can choose",
		"+2 bonuses to Saving throws. The number of bonuses you can choose"},
	{"listed at the end of this subclass and The creation points spent",
		"listed at the end of this subclass. The creation points spent"},
	{"Wisdom (Illusions) Shadow sword This jutsu can take on the shape",
		"Wisdom (Illusions). Shadow Sword: this jutsu can take on the shape"},
	{"9th Phantasmal killer This jutsu gains the Medical Keyword",
		"9th Phantasmal Killer: this jutsu gains the Medical Keyword"},
	{"only be used with a Web Short or Longbow or When you can a Tsuchigumo clan Hijutsu",
		"only be used with a Web Short or Longbow, or when you cast a Tsuchigumo clan Hijutsu"},
	{"long dragon made of pure lightning, this dragon flies in a straight line",
		"long dragon made of pure lightning. This dragon flies in a straight line"},
	{"needle like hair protruding from it., For the duration",
		"needle-like hair protruding from it. For the duration"},

	// --- fifth pass, the dropped-possessive sweep, 2026-09-13 ---
	//
	// The plural-possessive residue of that sweep. possessiveDetRe and
	// possessiveNounRe both insert "’s", which is wrong wherever the head
	// noun really is plural ("all targets AC", "both casters unarmed
	// damage"); possessivePluralContextRe holds those back deliberately and
	// they are corrected here one at a time, because the right rendering is a
	// judgment call rather than a mechanical apostrophe move — some read best
	// as a plural possessive ("creatures’ saving throws"), others as a
	// distributive singular ("an affected creature’s AC is reduced"), and the
	// grammatical number of the verb that follows is what decides which.
	// ("grant the affected creatures temporary hit points" is in neither
	// group: it is ditransitive, already correct, and deliberately untouched.)
	{"Affected creatures AC is reduced by 1", "An affected creature’s AC is reduced by 1"},
	{"Affected creatures AC is reduced by 2", "An affected creature’s AC is reduced by 2"},
	{"affected creatures AC is instead reduced by 3", "an affected creature’s AC is instead reduced by 3"},
	{"Affected creatures senses are damaged", "An affected creature’s senses are damaged"},
	{"Affected creatures jutsu they are casting has its shape size reduced",
		"An affected creature’s jutsu has its shape size reduced"},
	{"the affected creatures speed is reduced", "the affected creature’s speed is reduced"},
	{"the cost of the affected creatures jutsu", "the cost of the affected creature’s jutsu"},
	{"the collective total of all Casters Ninjutsu Save DC",
		"the collective total of all Casters’ Ninjutsu Save DCs"},
	{"the single attack roll with all targets AC", "the single attack roll with each target’s AC"},
	{"contested by all hostile creatures passive perception",
		"contested by all hostile creatures’ passive Perception"},
	{"manipulate other creatures perception of momentum",
		"manipulate other creatures’ perception of momentum"},
	{"manipulating other creatures perception of time",
		"manipulating other creatures’ perception of time"},
	{"a -1 penalty to creatures saving throws", "a -1 penalty to creatures’ saving throws"},
	{"the target takes all casters unarmed damage", "the target takes all casters’ unarmed damage"},
	{"equal to both casters unarmed damage", "equal to both casters’ unarmed damage"},
	{"Both casters Unarmed Damage", "Both casters’ Unarmed Damage"},

	// A mis-pluralized head noun rather than a dropped apostrophe — the
	// -ly guard in possessiveExtFix exists so this one is not "corrected"
	// into a possessive it never was.
	{"When a Creatures currently under the effects", "When a Creature currently under the effects"},

	// Grammar slips found in the same rows while reviewing the sweep's hits.
	{"the target gains a ranked of Staggered", "the target gains a rank of Staggered"},
	// Keyed against the possessive pass's own output, which runs first.
	{"ability to exert is fore of will is hindered", "ability to exert its force of will is hindered"},
	{"you and Any friendly creatures behind this radius, are considered",
		"you and any friendly creatures behind this radius are considered"},

	// Stray ".," left by the PDF extraction, in the three rows where the
	// comma is not the start of a real clause.
	{"triggers an Attack of opportunity.,", "triggers an Attack of opportunity."},
	{"as your levels in this class grows., becoming", "as your levels in this class grow, becoming"},
	{"(Chapter 6., SHB)", "(Chapter 6, SHB)"},

	// The two outcome lines missing the comma after their "On a ..." opener.
	// Every other outcome line in the corpus punctuates it.
	{"On a Success you treat this attack as a hit", "On a Success, you treat this attack as a hit"},
	{"On a Failure deal 1d6 of additional psychic damage", "On a Failure, deal 1d6 of additional psychic damage"},

	// "chakras die": a stray plural on "chakra" where the resource is the
	// Chakra Die. The corpus's own singular "N Chakra die" phrasing is left
	// alone; these four have both words wrong.
	{"Spend a number of chakras die up to your proficiency bonus.",
		"Spend a number of Chakra Dice up to your proficiency bonus."},
	{"can spend up to two chakras die, regaining", "can spend up to two Chakra Dice, regaining"},
	{"spend 3 or more chakras die, you regain 1 chakra die at",
		"spend 3 or more Chakra Dice, you regain 1 Chakra Die at"},
	{"you spend 2 chakras die and create a keycard", "you spend 2 Chakra Dice and create a keycard"},
}

// CuratedJutsuNameFixes corrects jutsu.Name source-book data-entry typos
// found during the pt/es glossary pass (see internal/i18n/glossary.md's
// "Jutsu names" sections). jutsu.Name is deliberately excluded from
// proseFields/curatedFix's own Sweep (Name fields don't get spellchecked),
// so a fix here needs its own exact-match table, applied once right after
// parsing (see internal/ingest.Jutsu) rather than through the Sweep
// machinery — this is what actually makes a fix durable across a fresh
// PDF re-ingest, unlike a one-off `UPDATE jutsu SET name=...` against a
// specific rules.db copy, which a later re-ingest silently overwrites.
var CuratedJutsuNameFixes = map[string]string{
	"O Encode Thoughts":   "Encode Thoughts",
	"One Man Front Lotud": "One Man Front Lotus",
}

// curatedFix applies every entry above as a plain substring replacement,
// logged the same way as the other tools. An entry that doesn't match the
// current text (e.g. a stale key after wording upstream changes) is
// silently a no-op — this list is meant to grow and shrink as fixes land
// upstream, not to error when one goes stale.
func curatedFix(s string) (string, []textDiff) {
	var diffs []textDiff
	for _, c := range curatedCorrections {
		if strings.Contains(s, c.Original) {
			s = strings.ReplaceAll(s, c.Original, c.Corrected)
			diffs = append(diffs, textDiff{tool: "curated", original: c.Original, corrected: c.Corrected})
		}
	}
	return s, diffs
}
