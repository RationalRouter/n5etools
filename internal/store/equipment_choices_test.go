package store

import "testing"

func TestSplitEquipmentChoice(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []equipmentChoiceOption
	}{
		{
			name: "two lettered alternatives, both resolve",
			raw:  "(a) Padded Cloth or (b) Combat Jacket",
			want: []equipmentChoiceOption{
				{Description: "Padded Cloth", ItemSlug: "armor/padded-cloth", Quantity: 1},
				{Description: "Combat Jacket", ItemSlug: "armor/combat-jacket", Quantity: 1},
			},
		},
		{
			name: "three lettered alternatives",
			raw:  "(a) Padded Cloth or (b) Combat Jacket or (c) Combat Armor",
			want: []equipmentChoiceOption{
				{Description: "Padded Cloth", ItemSlug: "armor/padded-cloth", Quantity: 1},
				{Description: "Combat Jacket", ItemSlug: "armor/combat-jacket", Quantity: 1},
				{Description: "Combat Armor", ItemSlug: "armor/combat-armor", Quantity: 1},
			},
		},
		{
			name: "quantity words parsed, still resolves",
			raw:  "(a) One Kunai Stack or (b) One Shuriken Stack",
			want: []equipmentChoiceOption{
				{Description: "One Kunai Stack", ItemSlug: "weapon/kunai", Quantity: 1},
				{Description: "One Shuriken Stack", ItemSlug: "weapon/shuriken", Quantity: 1},
			},
		},
		{
			name: "leading digit quantity kept on the resolved item",
			raw:  "(a) 2 Paper Bombs or (b) 2 Flash tags",
			want: []equipmentChoiceOption{
				{Description: "2 Paper Bombs", ItemSlug: "tool/paper-bombs", Quantity: 2},
				{Description: "2 Flash tags", ItemSlug: "tool/flash-tag", Quantity: 2},
			},
		},
		{
			name: "category/free-choice text never resolves",
			raw:  "(a) 1 Simple Weapon or (b) 1 Martial Weapon",
			want: []equipmentChoiceOption{
				{Description: "1 Simple Weapon", ItemSlug: "", Quantity: 1},
				{Description: "1 Martial Weapon", ItemSlug: "", Quantity: 1},
			},
		},
		{
			name: "no alternation, single item resolves",
			raw:  "One Simple weapon",
			want: []equipmentChoiceOption{
				{Description: "One Simple weapon", ItemSlug: "", Quantity: 1},
			},
		},
		{
			name: "multi-item bundle stays a single unresolved row, not split",
			raw:  "Cooking Tools, Flash Tag, Paper Bomb",
			want: []equipmentChoiceOption{
				{Description: "Cooking Tools, Flash Tag, Paper Bomb", ItemSlug: "", Quantity: 1},
			},
		},
		{
			name: "free-text alternative naming no known item stays unresolved",
			raw:  "(a) a Hand crossbow and one stack of bolts or (b) any two simple weapons",
			want: []equipmentChoiceOption{
				{Description: "a Hand crossbow and one stack of bolts", ItemSlug: "", Quantity: 1},
				{Description: "any two simple weapons", ItemSlug: "", Quantity: 1},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitEquipmentChoice(tc.raw)
			if len(got) != len(tc.want) {
				t.Fatalf("splitEquipmentChoice(%q) = %+v, want %+v", tc.raw, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("splitEquipmentChoice(%q)[%d] = %+v, want %+v", tc.raw, i, got[i], tc.want[i])
				}
			}
		})
	}
}
