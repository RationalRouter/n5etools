package store

import "testing"

func TestParseChakraPerRank(t *testing.T) {
	i := func(n int) *int { return &n }

	tests := []struct {
		name string
		text string
		want *int
	}{
		{
			name: "standard phrasing, delta 3",
			text: "For each rank you cast this jutsu above D-Rank, increase the cost of this jutsu by 3.",
			want: i(3),
		},
		{
			name: "damage clause after the cost clause",
			text: "For each rank you cast this jutsu above A-Rank, increase the cost of this jutsu by 3 and the damage by 1d6.",
			want: i(3),
		},
		{
			name: "missing increase verb (OCR/typo)",
			text: "For each rank you cast this jutsu above C- Rank, Chakra cost of this jutsu by 3 or the Calorie cost by 2 and the damage by 2d8.",
			want: i(3),
		},
		{
			name: "activation cost wording",
			text: "For each rank you cast this jutsu above D-Rank increase the activation cost of this jutsu by 3. If this jutsu is cast at B-Rank, increase the AC bonus by +1.",
			want: i(3),
		},
		{
			name: "chakra spent wording, no literal cost word",
			text: "For each rank you cast this jutsu above D-Rank, increase the chakra spent activating this jutsu's initial & additional effect by 3. If this jutsu is cast at B-Rank you have advantage on checks to maintain the grapple.",
			want: i(3),
		},
		{
			name: "alternate resource before the real chakra number",
			text: "For each rank you cast this jutsu above D-Rank, increase the Chakra cost of this jutsu by 3 or the Calorie cost by 1 and damage by 2d6",
			want: i(3),
		},
		{
			name: "delta of 4, not the usual 3",
			text: "For each rank you cast this jutsu above D-Rank, increase the cost of this jutsu by 4 and the rank of the Genjutsu that can be cast by 1 (D>C>B>A>S). C-Rank",
			want: i(4),
		},
		{
			name: "delta of 5",
			text: "For each rank you cast this jutsu above B-Rank, increase the cost of this jutsu by 5 and the number of Doki summoned by 1. You cannot summon more than 1 of the same Doki this way",
			want: i(5),
		},
		{
			name: "For Rank typo (missing each/every)",
			text: "For Rank, you cast this jutsu above B-rank, increase the cost of this jutsu by 3. Startint at A- rank, you ignore 3/4 cover.",
			want: i(3),
		},
		{
			name: "lowercase for every rank, mid-sentence phrasing",
			text: "for every rank above C-rank this jutsu is cast, increase the chakra cost by 3 and damage die by 1 step. If this is cast at A-Rank they instead lose all movement on a failed save and half on a success.",
			want: i(3),
		},
		{
			name: "cost clause is not the opening sentence",
			text: "If this Jutsu is upcasted to at least B- Rank you immediately perform the Melee Ninjutsu Attack instead of waiting till your next turn. For each rank above C-Rank, increase the cost by 3, increasing the damage done by 2d10 and Critical threat range by +1.",
			want: i(3),
		},
		{
			name: "level-gated auto-scaling, not upcasting at all",
			text: "This Jutsu's movement speed boost increases by 5ft at 5th level (15ft), 11th level (20ft), 17th level (25ft)",
			want: nil,
		},
		{
			// Real jutsu/shadow-clone-technique text. Never opens with "For
			// each/every rank" (it's "If this jutsu is cast at..." instead),
			// so it doesn't qualify for the book-default fallback either —
			// distinguishing a genuine per-rank upcast rule from a
			// rank-gated threshold bonus is exactly what the opener check
			// is for.
			name: "rank-gated threshold bonus, no per-rank cost, no opener",
			text: "If this jutsu is cast at B-Rank or higher, increase the number of clones you can create with this jutsu by 2. If this jutsu is cast at A-Rank, Shadow Clones you summon can cast jutsu of B-Rank or lower.",
			want: nil,
		},
		{
			// Real jutsu/jugo/iron-shine-guard text. The book's confirmed
			// default (per the project's rules authority) applies: a genuine
			// per-rank upcast rule (the opener) that never states its own
			// delta still costs the standard 3/rank.
			name: "For each rank opener but no cost mentioned at all — book default applies",
			text: "For each rank above C-Rank you cast this jutsu, increase the HP of the shield by 10.",
			want: i(3),
		},
		{
			// Real jutsu/iburi/incinerating-burst text.
			name: "For each rank opener, damage scales but cost is never stated — book default applies",
			text: "For each rank you cast this jutsu above C-Rank, increase the damage by 1d8+1. If cast to B-Rank, you can perform this jutsu as a reaction, when you would take an attack of opportunity, as if you cast this jutsu at C-Rank.",
			want: i(3),
		},
		{
			name: "bespoke non-linear rule must not false-positive on its own cost mentions",
			text: "When you would upcast this jutsu, Beginning at S-Rank; You are able to mark a single object, surface, or a weapon within 5 feet as a Bonus Action at the cost of 10 chakras. You also multiply your teleportation distance by 10 by increasing the cost to teleport by 10.",
			want: nil,
		},
		{
			// Real jutsu/futton/boil-release-unrivaled-strength text —
			// truncated/anomalous, but the opener sentence alone is still a
			// genuine (if incomplete) per-rank upcast rule, so the default
			// still applies rather than giving up entirely.
			name: "truncated/anomalous prose, opener present — book default applies",
			text: "For each rank you cast this jutsu above C-Rank. If this jutsu is cast at A- Rank or higher, your [Unarmed Damage] die instead becomes 3d8.",
			want: i(3),
		},
		{
			// Real jutsu/summoning-technique text — no explicit cost, but
			// the opener still marks it as upcastable.
			name: "opener present, effect described instead of cost — book default applies",
			text: "For each rank you cast this jutsu above D-Rank, you summon a corresponding creature equal in level and rank to the Rank used upon activation.",
			want: i(3),
		},
		{
			name: "empty text",
			text: "",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseChakraPerRank(tt.text)
			switch {
			case tt.want == nil && got != nil:
				t.Fatalf("got %d, want nil", *got)
			case tt.want != nil && got == nil:
				t.Fatalf("got nil, want %d", *tt.want)
			case tt.want != nil && got != nil && *got != *tt.want:
				t.Fatalf("got %d, want %d", *got, *tt.want)
			}
		})
	}
}
