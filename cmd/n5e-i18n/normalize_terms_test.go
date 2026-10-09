package main

import "testing"

func TestNormalizeTerms(t *testing.T) {
	cases := []struct {
		locale, en, in, want string
	}{
		// Feminine variants become the masculine "teste", with the determiner
		// and a trailing participle re-inflected.
		{"pt", "on a successful save", "caso a jogada de resistência seja bem-sucedida",
			"caso o teste de resistência seja bem-sucedido"},
		{"pt", "make a saving throw", "faz uma jogada de salvamento", "faz um teste de resistência"},
		{"pt", "all other saving throws", "todas as outras jogadas de resistência",
			"todos os outros testes de resistência"},
		{"pt", "saving throws", "Jogo de Defesa de Sabedoria", "Teste de Resistência de Sabedoria"},
		// "a" before a plural noun is the preposition and stays.
		{"pt", "add it to saving throws", "adicione-o a jogadas de resistência",
			"adicione-o a testes de resistência"},
		// CD is feminine.
		{"pt", "your save DC", "contra o seu CD de Defesa", "contra a sua CD de Resistência"},
		{"pt", "Save DC", "CD de Salvação", "CD de Resistência"},
		{"pt", "the save DC", "a CD de defesa", "a CD de resistência"},
		// The English gate keeps an unrelated "defesa" (defense) untouched.
		{"pt", "your defense", "sua jogada de defesa", "sua jogada de defesa"},
		{"pt", "roll 1 Flurry Die", "role 1 dado de Flurry", "role 1 dado de rajada"},
		{"pt", "roll 1 Flurry Die", "role 1 dado de Rajada", "role 1 dado de rajada"},
		{"pt", "a Flurry Technique", "uma Técnica de Série de Ataques", "uma Técnica de Rajada"},
		{"pt", "a Flurry Technique", "uma Técnica de Série existente", "uma Técnica de Rajada existente"},
		{"pt", "your Cooking Dice", "seus dados de culinária", "seus Dados de Cozinha"},
		{"pt", "your Cooking Die", "seu Dado de Culinária", "seu Dado de Cozinha"},
		// The English gate keeps the Cooking Kit ("Kit de Culinária") untouched.
		{"pt", "Cooking Kit", "Kit de Culinária", "Kit de Culinária"},
		// "jogada" is feminine: stacked determiners and a trailing participle
		// are re-inflected.
		{"pt", "your next attack roll", "o seu próximo teste de ataque bem-sucedido",
			"a sua próxima jogada de ataque bem-sucedida"},
		{"pt", "+1 to attack rolls", "+1 nos testes de ataque", "+1 nas jogadas de ataque"},
		{"pt", "all attack rolls", "em todos os lançamentos de Ataque", "em todas as jogadas de Ataque"},
		{"pt", "attack rolls made against it", "testes de ataque feitos contra ele",
			"jogadas de ataque feitas contra ele"},
		// Two articles never stack: "ao" belongs to an elided noun.
		{"pt", "higher than the attack roll", "superior ao do lançamento de ataque",
			"superior ao da jogada de ataque"},
		{"es", "regain hit points", "recupera puntos de vida", "recupera puntos de golpe"},
		{"es", "spend Hit Dice", "gasta Dados de Vida", "gasta Dados de Golpe"},
		{"es", "10 HP", "10 PV", "10 PG"},
	}
	for _, c := range cases {
		if got := normalizeTerms(c.locale, c.en, c.in); got != c.want {
			t.Errorf("normalizeTerms(%q, %q, %q)\n got  %q\n want %q", c.locale, c.en, c.in, got, c.want)
		}
	}
}

func TestMarshalCatalogStringKeepsAmpersand(t *testing.T) {
	if got := string(marshalCatalogString(`D&D <b> "x"`)); got != `"D&D <b> \"x\""` {
		t.Errorf("marshalCatalogString = %s", got)
	}
}
