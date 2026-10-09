package main

import (
	"database/sql"
	"flag"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/RationalRouter/n5e/internal/i18n"
)

// normalize-terms repairs core D&D mechanic terms that DeepL rendered several
// different ways across the corpus: "saving throw", "save DC", "Hit Die",
// "hit points", "Flurry Die"/"Flurry Technique", and in Portuguese "Cooking
// Die" and "attack roll". glossary.md's "Core mechanics canonical renderings
// (pinned 2026-10-08)" section records the approved rendering per locale and
// why; this pass rewrites every drifted variant to it, in both
// content_translations and the UI string catalog.
//
// It differs from normalize-glossary in what it can safely assume. Those
// Release terms are proper mechanic names; these are common nouns inside
// running prose, so a rewrite has to keep the sentence grammatical:
//
//   - Portuguese "jogada"/"salvaguarda" (feminine) becomes "teste"
//     (masculine), so the determiner before it and an agreeing participle
//     after it are re-inflected too ("na jogada de resistência bem-sucedida"
//     -> "no teste de resistência bem-sucedido").
//   - Portuguese "CD" (classe de dificuldade) is feminine; DeepL frequently
//     gave it masculine determiners ("seu CD"), which is fixed everywhere.
//
// Anything a rule cannot re-inflect with confidence (a pronoun referring
// back to the noun several words later, say) is left to the human pass the
// dry-run output exists for. Every rule only matches wrong renderings, so
// re-running over already-normalized text is a no-op.
//
// Dry-run by default; -apply writes.

// termRule rewrites one family of drifted renderings. enGate, when set,
// skips text whose English source is known and does not match it -- a
// guard for variant words that are also ordinary prose ("defesa" is only a
// saving throw when the English says save). With enStrict the rule also
// skips text whose English is unknown, for variants too generic to trust
// without it ("série" is only the Flurry Die when the English says Flurry).
type termRule struct {
	name     string
	enGate   *regexp.Regexp
	enStrict bool
	rewrite  func(string) string
}

// wordRuneBefore/wordRuneAt report whether the rune ending/starting at byte
// i is part of a word, i.e. whether a match boundary at i would split one. Go's \b is
// ASCII-only and misses "à", "às", "próxima", so word boundaries are
// checked by hand.
func wordRuneBefore(s string, i int) bool {
	if i <= 0 {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(s[:i])
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_'
}

func wordRuneAt(s string, i int) bool {
	if i >= len(s) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s[i:])
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_'
}

func isCapitalized(w string) bool {
	r, _ := utf8.DecodeRuneInString(w)
	return unicode.IsUpper(r)
}

func capitalize(w string) string {
	r, n := utf8.DecodeRuneInString(w)
	if r == utf8.RuneError {
		return w
	}
	return string(unicode.ToUpper(r)) + w[n:]
}

// withCaseOf gives repl the leading capitalization of orig.
func withCaseOf(orig, repl string) string {
	if isCapitalized(orig) {
		return capitalize(repl)
	}
	return repl
}

// inflect swaps w through m (keys lowercase), preserving w's leading
// capitalization. ok is false when w has no entry.
func inflect(m map[string]string, w string) (string, bool) {
	out, ok := m[strings.ToLower(w)]
	if !ok {
		return w, false
	}
	return withCaseOf(w, out), true
}

// replaceWords runs re over s and replaces each match whose span sits on
// word boundaries with fn(submatches). fn receives s and the submatch index
// slice and returns the replacement for the whole match.
func replaceWords(re *regexp.Regexp, s string, fn func(s string, m []int) string) string {
	var b strings.Builder
	last := 0
	for _, m := range re.FindAllStringSubmatchIndex(s, -1) {
		if wordRuneBefore(s, m[0]) || wordRuneAt(s, m[1]) {
			continue
		}
		b.WriteString(s[last:m[0]])
		b.WriteString(fn(s, m))
		last = m[1]
	}
	if last == 0 {
		return s
	}
	b.WriteString(s[last:])
	return b.String()
}

func group(s string, m []int, g int) string {
	if m[2*g] < 0 {
		return ""
	}
	return s[m[2*g]:m[2*g+1]]
}

// --- Portuguese -------------------------------------------------------------

// ptFemToMasc re-inflects a determiner/quantifier that agreed with a
// feminine noun ("jogada", "salvaguarda") so it agrees with "teste". Plain
// "a" is handled by ptMascDet instead, since it is only an article before
// a singular noun.
var ptFemToMasc = map[string]string{
	"as": "os", "na": "no", "nas": "nos", "da": "do", "das": "dos",
	"à": "ao", "às": "aos", "uma": "um", "umas": "uns", "numa": "num",
	"pela": "pelo", "pelas": "pelos", "sua": "seu", "suas": "seus",
	"minha": "meu", "nossa": "nosso", "essa": "esse", "essas": "esses",
	"esta": "este", "estas": "estes", "nessa": "nesse", "nesta": "neste",
	"dessa": "desse", "desta": "deste", "aquela": "aquele",
	"primeira": "primeiro", "segunda": "segundo", "próxima": "próximo",
	"última": "último", "outra": "outro", "outras": "outros",
	"mesma": "mesmo", "nova": "novo", "toda": "todo", "todas": "todos",
	"cuja": "cujo", "cujas": "cujos", "duas": "dois", "única": "único",
	"alguma": "algum", "algumas": "alguns", "nenhuma": "nenhum",
	"quantas": "quantos", "várias": "vários", "tal": "tal",
}

// ptFemAdjToMasc re-inflects a participle/adjective directly after the
// saving-throw phrase. Only forms observed agreeing with the noun in the
// corpus are listed; anything else is left for the human pass.
var ptFemAdjToMasc = map[string]string{
	"bem-sucedida": "bem-sucedido", "bem-sucedidas": "bem-sucedidos",
	"malsucedida": "malsucedido", "malsucedidas": "malsucedidos",
	"fracassada": "fracassado", "fracassadas": "fracassados",
	"falhada": "falhado", "falhadas": "falhados", "feita": "feito", "feitas": "feitos",
	"imposta": "imposto", "impostas": "impostos", "exigida": "exigido",
	"exigidas": "exigidos", "escolhida": "escolhido",
	"escolhidas": "escolhidos", "baseada": "baseado", "baseadas": "baseados",
	"associada": "associado", "associadas": "associados",
	"realizada": "realizado", "realizadas": "realizados",
	"necessária": "necessário", "necessárias": "necessários",
	"obrigatória": "obrigatório", "perdida": "perdido",
	"perdidas": "perdidos", "repetida": "repetido", "refeita": "refeito",
	"bem sucedida": "bem-sucedido", "concedida": "concedido",
	"forçada": "forçado", "forçadas": "forçados", "causada": "causado",
	"causadas": "causados", "permitida": "permitido",
}

// ptMascToFem is ptFemToMasc's inverse for "CD", plus the masculine-only
// determiners DeepL attached to it.
var ptMascToFem = map[string]string{
	"o": "a", "os": "as", "no": "na", "nos": "nas", "do": "da", "dos": "das",
	"ao": "à", "aos": "às", "um": "uma", "num": "numa", "pelo": "pela",
	"seu": "sua", "seus": "suas", "esse": "essa", "este": "esta",
	"nesse": "nessa", "neste": "nesta", "desse": "dessa", "deste": "desta",
	"mesmo": "mesma", "próprio": "própria", "novo": "nova", "cujo": "cuja",
}

var ptCDAdjToFem = map[string]string{
	"listado": "listada", "aumentado": "aumentada", "indicado": "indicada",
	"predefinido": "predefinida", "reduzido": "reduzida",
	"determinado": "determinada", "definido": "definida",
	"especificado": "especificada", "normal": "normal",
}

func keys(m map[string]string) string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, regexp.QuoteMeta(k))
	}
	// Longest first so "nas" wins over "na" in the alternation.
	sort.Slice(ks, func(i, j int) bool { return len(ks[i]) > len(ks[j]) })
	return strings.Join(ks, "|")
}

// ptSaveTail is what may sit between the saving-throw phrase and an
// agreeing participle: the ability ("de Destreza") or "contra a morte",
// then a copula ("Se o teste de resistência for bem-sucedido"). "falha" is
// deliberately absent from ptFemAdjToMasc: after the noun it is far more
// often the verb ("se o teste falha") than the adjective.
const ptSaveTail = `(?:\s+(?:de|contra)\s+(?:a\s+)?\p{L}+)?(?:\s+(?:for|forem|seja|sejam|foi|foram|fosse|fossem|é|são))?`

var (
	// Group 1 determiner, 2 head noun, 3 the "de X" qualifier word.
	ptFemSaveRe = regexp.MustCompile(`(?i)(?:(` + keys(ptFemToMasc) + `|a)\s+)?(jogadas?)\s+de\s+(resist[eê]ncia|salvamento|salva[çc][ãa]o|salvaguarda|defesa)` +
		`|(?:(` + keys(ptFemToMasc) + `|a)\s+)?(salvaguardas?)`)
	ptMascSaveRe = regexp.MustCompile(`(?i)(jogos?|testes?|tiros?)\s+de\s+(defesa|salvamento|salva[çc][ãa]o)`)
	ptSaveNounRe = regexp.MustCompile(`(?i)testes?\s+de\s+resist[eê]ncia`)
	ptPrevDetRe  = regexp.MustCompile(`(?i)(` + keys(ptFemToMasc) + `|a)\s+$`)
	ptTailAdjRe  = regexp.MustCompile(`^(` + ptSaveTail + `\s+)(` + keys(ptFemAdjToMasc) + `)`)

	ptCDVariantRe = regexp.MustCompile(`(?i)CD\s+d[ea]\s+(?:sua\s+)?(jogo\s+de\s+defesa|defesa|salvamento|salva[çc][ãa]o|salvaguarda|teste\s+de\s+resist[eê]ncia)`)
	ptCDDetRe     = regexp.MustCompile(`(?i)(` + keys(ptMascToFem) + `)\s+CD`)
	ptCDStackRe   = regexp.MustCompile(`(?i)(` + keys(ptMascToFem) + `)\s+(?:sua|suas|essa|esta|mesma|própria|nova)\s+CD`)
	ptCDTailRe    = regexp.MustCompile(`^((?:\s+de\s+[Rr]esist[eê]ncia)?(?:\s+d[eo]\s+\p{L}+)?\s+)(` + keys(ptCDAdjToFem) + `)`)
)

// rewritePTSave normalizes every Portuguese saving-throw variant to
// "teste(s) de resistência", re-inflecting the agreement around it.
func rewritePTSave(s string) string {
	s = replaceWords(ptMascSaveRe, s, func(s string, m []int) string {
		noun := group(s, m, 1)
		out := "teste"
		if strings.HasSuffix(strings.ToLower(noun), "s") {
			out = "testes"
		}
		return withCaseOf(noun, out) + " de " + withCaseOf(group(s, m, 2), "resistência")
	})

	var b strings.Builder
	last := 0
	for _, m := range ptFemSaveRe.FindAllStringSubmatchIndex(s, -1) {
		det, noun, qual := 1, 2, 3
		if m[2*noun] < 0 { // the bare "salvaguarda" alternative
			det, noun, qual = 4, 5, -1
		}
		start := m[0]
		if m[2*det] >= 0 && wordRuneBefore(s, m[2*det]) {
			// The "determiner" is the tail of a longer word ("nossa" ->
			// "a"); match from the noun instead.
			start = m[2*noun]
		}
		if wordRuneBefore(s, start) || wordRuneAt(s, m[1]) {
			continue
		}
		nounText := group(s, m, noun)
		plural := strings.HasSuffix(strings.ToLower(nounText), "s")
		head := "teste"
		if plural {
			head = "testes"
		}
		qualCap := isCapitalized(nounText)
		if qual > 0 {
			qualCap = isCapitalized(group(s, m, qual))
		}
		repl := withCaseOf(nounText, head) + " de " + map[bool]string{true: "Resistência", false: "resistência"}[qualCap]
		if start == m[0] && m[2*det] >= 0 {
			repl = ptMascDet(group(s, m, det), plural) + s[m[2*det+1]:m[2*noun]] + repl
			// Determiners stack ("as outras jogadas", "todas as
			// jogadas"); walk back over each one directly before.
			for start > last {
				pm := ptPrevDetRe.FindStringSubmatchIndex(s[last:start])
				if pm == nil || wordRuneBefore(s, last+pm[2]) {
					break
				}
				repl = ptMascDet(s[last+pm[2]:last+pm[3]], plural) + s[last+pm[3]:start] + repl
				start = last + pm[2]
			}
		}
		b.WriteString(s[last:start])
		b.WriteString(repl)
		last = m[1]
	}
	if last > 0 {
		b.WriteString(s[last:])
		s = b.String()
	}

	// A feminine participle agreeing with the (now masculine) noun, whether
	// this pass just rewrote it or DeepL produced "teste" with feminine
	// agreement in the first place ("Se o teste de resistência for
	// bem-sucedida").
	b.Reset()
	last = 0
	for _, m := range ptSaveNounRe.FindAllStringIndex(s, -1) {
		if m[0] < last || wordRuneBefore(s, m[0]) {
			continue
		}
		t := ptTailAdjRe.FindStringSubmatchIndex(s[m[1]:])
		if t == nil || wordRuneAt(s, m[1]+t[1]) {
			continue
		}
		adj, _ := inflect(ptFemAdjToMasc, s[m[1]+t[4]:m[1]+t[5]])
		b.WriteString(s[last : m[1]+t[3]])
		b.WriteString(adj)
		last = m[1] + t[1]
	}
	if last == 0 {
		return s
	}
	b.WriteString(s[last:])
	return b.String()
}

// ptMascDet re-inflects one determiner of a feminine saving-throw noun for
// "teste". Plain "a" is the article before a singular noun but the
// preposition before a plural one ("+2 a jogadas de resistência").
func ptMascDet(w string, plural bool) string {
	if strings.EqualFold(w, "a") {
		if plural {
			return w
		}
		return withCaseOf(w, "o")
	}
	out, _ := inflect(ptFemToMasc, w)
	return out
}

// rewritePTSaveDC normalizes every "save DC" variant to "CD de resistência"
// and makes every determiner/participle agreeing with "CD" feminine.
func rewritePTSaveDC(s string) string {
	s = replaceWords(ptCDVariantRe, s, func(s string, m []int) string {
		return "CD de " + withCaseOf(group(s, m, 1), "resistência")
	})
	s = replaceWords(ptCDDetRe, s, func(s string, m []int) string {
		d, _ := inflect(ptMascToFem, group(s, m, 1))
		return d + s[m[3]:m[1]]
	})
	// A stacked determiner ("o seu CD") is left with its first word still
	// masculine by the pass above, now directly before a feminine one.
	s = replaceWords(ptCDStackRe, s, func(s string, m []int) string {
		d, _ := inflect(ptMascToFem, group(s, m, 1))
		return d + s[m[3]:m[1]]
	})
	// Participles agreeing with CD: scan each "CD" occurrence's tail.
	var b strings.Builder
	last := 0
	for i := 0; ; {
		j := strings.Index(s[i:], "CD")
		if j < 0 {
			break
		}
		pos := i + j + 2
		i = pos
		if wordRuneBefore(s, pos-2) || wordRuneAt(s, pos) {
			continue
		}
		t := ptCDTailRe.FindStringSubmatchIndex(s[pos:])
		if t == nil || wordRuneAt(s, pos+t[1]) {
			continue
		}
		adj, _ := inflect(ptCDAdjToFem, s[pos+t[4]:pos+t[5]])
		b.WriteString(s[last : pos+t[3]])
		b.WriteString(adj)
		last = pos + t[1]
		i = last
	}
	if last == 0 {
		return s
	}
	b.WriteString(s[last:])
	return b.String()
}

var (
	ptFlurryDieRe  = regexp.MustCompile(`(?i)(dados?)(?:\s+de)?\s+(flurry|s[ée]rie|r[aá]faga|enxurrada|rajada)`)
	ptFlurryTechRe = regexp.MustCompile(`(?i)(t[ée]cnicas?)\s+de\s+(s[ée]rie\s+de\s+ataques|ataques?\s+em\s+s[ée]rie|s[ée]rie|enxurrada|flurry|rajada)`)
)

// rewritePTFlurry moves every Flurry Die / Flurry Technique variant to
// "dado de rajada" / "técnica de rajada". The head noun's casing decides the
// qualifier's, since the source's "Flurry" is capitalized even mid-sentence:
// "dado de Flurry" becomes "dado de rajada", "Dado de Flurry" "Dado de Rajada".
func rewritePTFlurry(s string) string {
	s = replaceWords(ptFlurryDieRe, s, func(s string, m []int) string {
		return group(s, m, 1) + " de " + withCaseOf(group(s, m, 1), "rajada")
	})
	return replaceWords(ptFlurryTechRe, s, func(s string, m []int) string {
		return group(s, m, 1) + " de " + withCaseOf(group(s, m, 1), "rajada")
	})
}

// ptMascToFemAttack re-inflects a determiner/quantifier that agreed with
// the masculine "teste"/"lançamento" so it agrees with "jogada". Words that
// are also prepositions or adverbs ("segundo" = according to, "mesmo" =
// even) are left out, since a determiner walk-back cannot tell them apart.
var ptMascToFemAttack = func() map[string]string {
	m := map[string]string{
		"todo": "toda", "todos": "todas", "outro": "outra", "outros": "outras",
		"pelos": "pelas", "próximo": "próxima", "próximos": "próximas",
		"primeiro": "primeira", "primeiros": "primeiras", "terceiro": "terceira",
		"último": "última", "últimos": "últimas", "vários": "várias",
		"único": "única", "nenhum": "nenhuma", "dois": "duas",
		"múltiplos": "múltiplas", "esses": "essas", "estes": "estas",
		"nesses": "nessas", "nestes": "nestas", "desses": "dessas",
		"destes": "destas", "aquele": "aquela", "aqueles": "aquelas",
		"meu": "minha", "meus": "minhas", "nosso": "nossa", "nossos": "nossas",
		"algum": "alguma", "alguns": "algumas", "uns": "umas", "quantos": "quantas",
	}
	for k, v := range ptMascToFem {
		if k != "mesmo" {
			m[k] = v
		}
	}
	return m
}()

// ptMascArticles are the articles (bare or contracted with a preposition)
// in ptMascToFemAttack.
var ptMascArticles = map[string]bool{
	"o": true, "os": true, "um": true, "uns": true, "do": true, "dos": true,
	"no": true, "nos": true, "ao": true, "aos": true, "pelo": true,
	"pelos": true, "num": true,
}

// ptMascAdjToFem is ptFemAdjToMasc inverted, for a participle agreeing with
// the attack-roll noun ("um teste de ataque bem-sucedido").
var ptMascAdjToFem = func() map[string]string {
	m := map[string]string{}
	for fem, masc := range ptFemAdjToMasc {
		if !strings.Contains(fem, " ") {
			m[masc] = fem
		}
	}
	return m
}()

var (
	ptAttackRollRe  = regexp.MustCompile(`(?i)(testes?|lançamentos?)\s+de\s+(ataque)`)
	ptPrevMascDetRe = regexp.MustCompile(`(?i)(` + keys(ptMascToFemAttack) + `)\s+$`)
	ptAttackTailRe  = regexp.MustCompile(`^((?:\s+(?:de|com)\s+\p{L}+)?(?:\s+corpo\s+a\s+corpo|\s+à\s+distância)?(?:\s+(?:for|forem|seja|sejam|foi|foram|fosse|fossem|é|são))?\s+)(` + keys(ptMascAdjToFem) + `)`)
	ptCookingDieRe  = regexp.MustCompile(`(?i)(dados?)\s+(?:de\s+)?(culin[aá]ria|culin[aá]rios?|cozinha|cozinhar)`)
)

// rewritePTAttackRoll moves "teste(s) de ataque" and "lançamento(s) de
// ataque" to the Galápagos 2014 "jogada(s) de ataque". "jogada" is
// feminine, so the determiners directly before it and a participle after it
// are re-inflected ("o seu próximo teste de ataque bem-sucedido" -> "a sua
// próxima jogada de ataque bem-sucedida"). A pronoun referring back to it
// later in the sentence ("compare-o") is left for the dry-run review.
func rewritePTAttackRoll(s string) string {
	var b strings.Builder
	last := 0
	for _, m := range ptAttackRollRe.FindAllStringSubmatchIndex(s, -1) {
		if m[0] < last || wordRuneBefore(s, m[0]) || wordRuneAt(s, m[1]) {
			continue
		}
		noun := group(s, m, 1)
		plural := strings.HasSuffix(strings.ToLower(noun), "s")
		head := "jogada"
		if plural {
			head = "jogadas"
		}
		repl := withCaseOf(noun, head) + " de " + group(s, m, 2)
		start := m[0]
		prevArticle := false
		for start > last {
			pm := ptPrevMascDetRe.FindStringSubmatchIndex(s[last:start])
			if pm == nil || wordRuneBefore(s, last+pm[2]) {
				break
			}
			w := s[last+pm[2] : last+pm[3]]
			// Two articles never stack on one noun: in "superior ao do
			// teste" the "ao" belongs to an elided "resultado".
			article := ptMascArticles[strings.ToLower(w)]
			if article && prevArticle {
				break
			}
			prevArticle = article
			d, _ := inflect(ptMascToFemAttack, w)
			repl = d + s[last+pm[3]:start] + repl
			start = last + pm[2]
		}
		end := m[1]
		if t := ptAttackTailRe.FindStringSubmatchIndex(s[end:]); t != nil && !wordRuneAt(s, end+t[1]) {
			adj, _ := inflect(ptMascAdjToFem, s[end+t[4]:end+t[5]])
			repl += s[end:end+t[3]] + adj
			end += t[1]
		}
		b.WriteString(s[last:start])
		b.WriteString(repl)
		last = end
	}
	if last == 0 {
		return s
	}
	b.WriteString(s[last:])
	return b.String()
}

// rewritePTCookingDie moves every Cooking Die variant ("Dado de Culinária",
// "dados de culinária") to "Dado(s) de Cozinha". Both nouns are masculine,
// so nothing around it changes. The resource name is always capitalized.
func rewritePTCookingDie(s string) string {
	return replaceWords(ptCookingDieRe, s, func(s string, m []int) string {
		head := "Dado"
		if strings.HasSuffix(strings.ToLower(group(s, m, 1)), "s") {
			head = "Dados"
		}
		return head + " de Cozinha"
	})
}

// --- Spanish ----------------------------------------------------------------

var (
	esHitRe = regexp.MustCompile(`(?i)(dados?|puntos?)\s+de\s+(vida)`)
	esPVRe  = regexp.MustCompile(`PV`)
)

// rewriteESHit moves "Hit Die" and "hit points" to the official Spanish
// "dado de golpe"/"puntos de golpe"/"PG" pair. Both nouns are masculine in
// either rendering, so no agreement changes.
func rewriteESHit(s string) string {
	s = replaceWords(esHitRe, s, func(s string, m []int) string {
		return group(s, m, 1) + " de " + withCaseOf(group(s, m, 2), "golpe")
	})
	return replaceWords(esPVRe, s, func(string, []int) string { return "PG" })
}

var termRules = map[string][]termRule{
	"pt": {
		{name: "save DC", enGate: regexp.MustCompile(`(?i)\bDCs?\b`), rewrite: rewritePTSaveDC},
		{name: "saving throw", enGate: regexp.MustCompile(`(?i)\bsav(?:e|es|ing)\b`), rewrite: rewritePTSave},
		{name: "Flurry Die/Technique", enGate: regexp.MustCompile(`(?i)flurry`), enStrict: true, rewrite: rewritePTFlurry},
		{name: "Cooking Die", enGate: regexp.MustCompile(`(?i)cooking`), enStrict: true, rewrite: rewritePTCookingDie},
		{name: "attack roll", enGate: regexp.MustCompile(`(?i)\battack`), rewrite: rewritePTAttackRoll},
	},
	"es": {
		{name: "Hit Die/hit points", rewrite: rewriteESHit},
	},
}

func normalizeTerms(locale, en, s string) string {
	for _, r := range termRules[locale] {
		if r.enGate != nil && (en != "" || r.enStrict) && !r.enGate.MatchString(en) {
			continue
		}
		s = r.rewrite(s)
	}
	return s
}

func cmdNormalizeTerms(args []string) error {
	fs := flag.NewFlagSet("normalize-terms", flag.ContinueOnError)
	dbPath := fs.String("db", "", "path to rules.db (content_translations)")
	catalogPath := fs.String("catalog", "", "path to a locale's UI catalog JSON, instead of -db")
	enCatalogPath := fs.String("en", "internal/i18n/locales/en.json", "English UI catalog, for -catalog's English gate")
	locale := fs.String("locale", "", "locale code (es, pt)")
	apply := fs.Bool("apply", false, "write the rewrites (default: dry run, print them)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if (*dbPath == "") == (*catalogPath == "") || *locale == "" {
		return fmt.Errorf("usage: n5e-i18n normalize-terms (-db <path> | -catalog <path>) -locale <code> [-apply]")
	}
	if _, ok := termRules[*locale]; !ok {
		return fmt.Errorf("no term rules for locale %q", *locale)
	}
	mode := "dry run (re-run with -apply to write)"
	if *apply {
		mode = "applied"
	}

	if *catalogPath != "" {
		cat, err := readCatalog(*catalogPath)
		if err != nil {
			return err
		}
		en, err := readCatalog(*enCatalogPath)
		if err != nil {
			return err
		}
		ks := make([]string, 0, len(cat))
		for k := range cat {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		changed := 0
		for _, k := range ks {
			updated := normalizeTerms(*locale, en[k], cat[k])
			if updated == cat[k] {
				continue
			}
			changed++
			fmt.Printf("FIX   %s\n  - %s\n  + %s\n", k, cat[k], updated)
			cat[k] = updated
		}
		if *apply && changed > 0 {
			if err := writeCatalog(*catalogPath, cat); err != nil {
				return err
			}
		}
		fmt.Printf("\nn5e-i18n normalize-terms: catalog=%s rewritten=%d -- %s\n", *catalogPath, changed, mode)
		return nil
	}

	db, err := openRulesDB(*dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	// English source per (table, slug, field), for rules with an enGate.
	// Tables without a TranslatableFields entry (or rows the loader can't
	// key) simply have no English and skip gated rules.
	english := map[string]string{}
	for t := range i18n.TranslatableFields {
		rv, err := loadEnglishValues(db, t, "")
		if err != nil {
			continue
		}
		for _, r := range rv {
			for f, v := range r.Fields {
				english[t+"\x00"+r.Key+"\x00"+f] = v
			}
		}
	}

	rows, err := db.Query(`SELECT table_name, slug, field, value FROM content_translations WHERE locale = ? ORDER BY table_name, slug, field`, *locale)
	if err != nil {
		return err
	}
	type change struct{ table, slug, field, value string }
	var changes []change
	for rows.Next() {
		var t, slug, field, value string
		if err := rows.Scan(&t, &slug, &field, &value); err != nil {
			rows.Close()
			return err
		}
		updated := normalizeTerms(*locale, english[t+"\x00"+slug+"\x00"+field], value)
		if updated == value {
			continue
		}
		changes = append(changes, change{t, slug, field, updated})
		fmt.Printf("FIX   %s/%s.%s\n  - %s\n  + %s\n", t, slug, field, value, updated)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if *apply {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		for _, c := range changes {
			if err := updateTranslationTx(tx, c.table, c.slug, *locale, c.field, c.value); err != nil {
				tx.Rollback()
				return err
			}
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	fmt.Printf("\nn5e-i18n normalize-terms: locale=%s rewritten=%d -- %s\n", *locale, len(changes), mode)
	return nil
}

// updateTranslationTx is updateTranslation inside a transaction, so a full
// -apply run either lands completely or not at all.
func updateTranslationTx(tx *sql.Tx, table, slug, locale, field, value string) error {
	_, err := tx.Exec(`UPDATE content_translations SET value = ?, updated_at = datetime('now')
	                   WHERE table_name = ? AND slug = ? AND locale = ? AND field = ?`,
		value, table, slug, locale, field)
	return err
}
