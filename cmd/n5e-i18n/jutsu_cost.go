package main

import (
	"regexp"
	"strings"
)

// jutsu.cost_text is a short resource line ("5 Chakra", "Special (20
// Chakra), 5 Hit Die", "4 Chakra or 2 Calories"). Sent to DeepL without
// context it comes back mangled ("Los 5 Chakras", "Chakra de 13", "4.
// Chakra"), so translate renders it deterministically from the English
// instead. The handful of sentence-like costs in the corpus are listed
// verbatim; any other shape that the grammar below cannot read falls back
// to DeepL.

var jutsuCostSentences = map[string]map[string]string{
	"20 Chakra a Week for 52 weeks consistently.": {
		"es": "20 de Chakra a la semana durante 52 semanas seguidas.",
		"pt": "20 de Chakra por semana, durante 52 semanas, de forma constante.",
	},
	"14 Chakra, X Hit Dice. (You decide)": {
		"es": "14 de Chakra, X Dados de Golpe. (Tú decides)",
		"pt": "14 de Chakra, X Dados de Vida. (Você decide)",
	},
	"16 Chakra, all your current Fury stacks": {
		"es": "16 de Chakra, todas tus acumulaciones actuales de Furia",
		"pt": "16 de Chakra, todas as suas acumulações atuais de Fúria",
	},
	"12 Chakra. Special": {
		"es": "12 de Chakra. Especial",
		"pt": "12 de Chakra. Especial",
	},
	"20 Chakra (Special)": {
		"es": "20 de Chakra (Especial)",
		"pt": "20 de Chakra (Especial)",
	},
	"Special (See Summoning Section for Jutsu breakdown)": {
		"es": "Especial (consulta la sección de Invocación para ver el desglose de los jutsu)",
		"pt": "Especial (consulte a seção de Invocação para ver o detalhamento dos jutsu)",
	},
}

// jutsuCostUnits maps a lowercased English unit to its rendering per
// locale, singular then plural. Resource names match the glossary and the
// cr_name: catalog entries (Chakra Estelar, Calorías/Calorias).
var jutsuCostUnits = map[string]map[string][2]string{
	"chakra":      {"es": {"de Chakra", "de Chakra"}, "pt": {"de Chakra", "de Chakra"}},
	"star chakra": {"es": {"de Chakra Estelar", "de Chakra Estelar"}, "pt": {"de Chakra Estelar", "de Chakra Estelar"}},
	"calories":    {"es": {"Calorías", "Calorías"}, "pt": {"Calorias", "Calorias"}},
	"hit die":     {"es": {"Dado de Golpe", "Dados de Golpe"}, "pt": {"Dado de Vida", "Dados de Vida"}},
	"hit dice":    {"es": {"Dado de Golpe", "Dados de Golpe"}, "pt": {"Dado de Vida", "Dados de Vida"}},
	"fury":        {"es": {"de Furia", "de Furia"}, "pt": {"de Fúria", "de Fúria"}},
}

var jutsuCostOr = map[string]string{"es": " o ", "pt": " ou "}

var (
	jutsuCostAtomRe    = regexp.MustCompile(`^(\d+|X) (Chakra|chakra|Star Chakra|Calories|Hit Die|Hit Dice|Fury)$`)
	jutsuCostSpecialRe = regexp.MustCompile(`^Special \(([^)]*)\)(.*)$`)
)

// renderJutsuCost returns the locale's rendering of an English
// jutsu.cost_text, or ok=false when the line is not in a recognized shape
// or the locale has no rendering. The count decides plurality, so the
// corpus's "2 Hit Die" still becomes plural.
func renderJutsuCost(locale, en string) (string, bool) {
	if _, ok := jutsuCostOr[locale]; !ok {
		return "", false
	}
	if s, ok := jutsuCostSentences[en][locale]; ok {
		return s, true
	}
	if m := jutsuCostSpecialRe.FindStringSubmatch(en); m != nil {
		inner, ok := renderJutsuCost(locale, m[1])
		if !ok {
			return "", false
		}
		out := "Especial (" + inner + ")"
		if m[2] == "" {
			return out, true
		}
		rest, found := strings.CutPrefix(m[2], ", ")
		if !found {
			return "", false
		}
		tail, ok := renderJutsuCost(locale, rest)
		if !ok {
			return "", false
		}
		return out + ", " + tail, true
	}
	var parts []string
	for _, p := range strings.Split(en, ", ") {
		var alts []string
		for _, a := range strings.Split(p, " or ") {
			s, ok := renderJutsuCostAtom(locale, a)
			if !ok {
				return "", false
			}
			alts = append(alts, s)
		}
		parts = append(parts, strings.Join(alts, jutsuCostOr[locale]))
	}
	return strings.Join(parts, ", "), true
}

func renderJutsuCostAtom(locale, a string) (string, bool) {
	if a == "Special" {
		return "Especial", true
	}
	m := jutsuCostAtomRe.FindStringSubmatch(a)
	if m == nil {
		return "", false
	}
	forms := jutsuCostUnits[strings.ToLower(m[2])][locale]
	form := forms[1]
	if m[1] == "1" {
		form = forms[0]
	}
	return m[1] + " " + form, true
}
