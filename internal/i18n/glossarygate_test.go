package i18n

import (
	"reflect"
	"testing"
)

func TestFindUnglossaried(t *testing.T) {
	glossary := map[string]string{
		"Chakra":         "Chakra",
		"Fire Release":   "Liberación de Fuego",
		"Hidden Village": "Aldea Oculta",
	}
	known := map[string]struct{}{
		"Konoha": {},
	}

	tests := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "every term covered",
			text: "Spend one Chakra to use Fire Release near a Hidden Village.",
			want: nil,
		},
		{
			name: "known DB name is not flagged",
			text: "Konoha grants a bonus.",
			want: nil,
		},
		{
			name: "brand-new term is flagged",
			text: "The Whispering Cinderfall Technique deals damage.",
			want: []string{"The Whispering Cinderfall Technique"},
		},
		{
			name: "duplicate occurrences collapse to one",
			text: "Cinderfall Wisp appears twice: Cinderfall Wisp.",
			want: []string{"Cinderfall Wisp"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FindUnglossaried(tt.text, glossary, known)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FindUnglossaried(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}
