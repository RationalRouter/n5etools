package main

import "testing"

func TestRenderJutsuCost(t *testing.T) {
	cases := []struct{ locale, en, want string }{
		{"es", "5 Chakra", "5 de Chakra"},
		{"pt", "12 chakra", "12 de Chakra"},
		{"es", "Special", "Especial"},
		{"pt", "Special (36 Chakra)", "Especial (36 de Chakra)"},
		{"es", "Special (14 Chakra or 7 Star Chakra)", "Especial (14 de Chakra o 7 de Chakra Estelar)"},
		{"pt", "4 Chakra or 2 Calories", "4 de Chakra ou 2 Calorias"},
		// Plurality follows the count, not the book's "2 Hit Die".
		{"es", "5 Chakra, 1 Hit Die", "5 de Chakra, 1 Dado de Golpe"},
		{"es", "14 Chakra, 2 Hit Die", "14 de Chakra, 2 Dados de Golpe"},
		{"pt", "Special (20 Chakra), 5 Hit Die", "Especial (20 de Chakra), 5 Dados de Vida"},
		{"pt", "12 Chakra, X Fury", "12 de Chakra, X de Fúria"},
		{"es", "20 Chakra a Week for 52 weeks consistently.", "20 de Chakra a la semana durante 52 semanas seguidas."},
	}
	for _, c := range cases {
		got, ok := renderJutsuCost(c.locale, c.en)
		if !ok || got != c.want {
			t.Errorf("renderJutsuCost(%q, %q) = %q, %v; want %q", c.locale, c.en, got, ok, c.want)
		}
	}
	for _, en := range []string{"5 Chakra Keyword: Hijutsu", "Special (36 Chakra).", "a handful of chakra"} {
		if got, ok := renderJutsuCost("es", en); ok {
			t.Errorf("renderJutsuCost(es, %q) = %q; want a DeepL fallback", en, got)
		}
	}
	if _, ok := renderJutsuCost("fr", "5 Chakra"); ok {
		t.Error("renderJutsuCost(fr) rendered; want a DeepL fallback for a locale with no rendering")
	}
}
