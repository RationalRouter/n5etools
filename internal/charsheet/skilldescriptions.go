package charsheet

// SkillDescriptions gives each of the 21 skills a short description of
// what it actually covers — hand-transcribed directly from the core book
// (Naruto 5e - Full Document.pdf, Chapter 6 "Using Each Ability Score",
// pages 51-54), the same source SkillAbility's ability mapping came from
// and nowhere in either database (confirmed: no skills/skill_descriptions
// table exists anywhere in rules.db or its migrations). Several of these
// skills (Chakra Control, Ninshou, Illusions) are N5e-specific, not
// standard 5e, so this can't be filled in from general system knowledge —
// every entry below is condensed from the book's own per-skill paragraph,
// not invented.
var SkillDescriptions = map[string]string{
	"Athletics":       "Covers difficult situations you encounter while climbing, jumping, or swimming.",
	"Martial Arts":    "Covers situations you encounter while training, learning, or performing complex Martial Arts maneuvers.",
	"Acrobatics":      "Covers your attempt to stay on your feet in a tricky situation, such as running across ice, balancing on a tightrope, or pulling off acrobatic stunts.",
	"Sleight of Hand": "Covers an act of legerdemain or manual trickery, such as planting something on someone or concealing an object on your person.",
	"Stealth":         "Made when you attempt to conceal yourself from enemies, slip past guards, sneak up on someone, or move without being seen or heard.",
	"Chakra Control":  "Covers your attempt to manipulate your chakra in any way, including maintaining concentration on a jutsu.",
	"Crafting":        "Measures your ability to appraise, repair, disable, or create objects of varying fields such as technology, architecture, and demolitions.",
	"History":         "Measures your ability to recall information about artifacts, histories, religions, cultures, and other known or discovered facts or theories.",
	"Investigation":   "Made when you look around for clues and make deductions based on those clues.",
	"Nature":          "Measures your ability to recall lore about terrain, plants, animals, the weather, and natural cycles.",
	"Ninshou":         "Measures your ability to control, enhance, recognize, and create Ninjutsu of any type you're experienced with — also used to resolve a Ninjutsu Clash.",
	"Animal Handling": "Called for when there's any question whether you can calm a domesticated animal, keep a mount from getting spooked, or intuit an animal's intentions.",
	"Illusions":       "Measures your ability to control, enhance, recognize, and create Genjutsu of any type you're experienced with.",
	"Insight":         "Decides whether you can determine the true intentions of a creature, such as when searching out a lie or predicting someone's next move.",
	"Medicine":        "Lets you try to stabilize a dying companion or diagnose an illness.",
	"Perception":      "Lets you spot, hear, or otherwise detect the presence of something.",
	"Survival":        "Covers following tracks, hunting wild game, guiding a group through hostile terrain, predicting the weather, or avoiding natural hazards.",
	"Deception":       "Determines whether you can convincingly hide the truth, either verbally or through your actions.",
	"Intimidation":    "Called for when you attempt to influence someone through overt threats, hostile actions, or physical violence.",
	"Performance":     "Determines how well you can delight an audience with music, dance, acting, storytelling, or some other form of entertainment.",
	"Persuasion":      "Called for when you attempt to influence someone or a group with tact, social graces, or good nature.",
}
