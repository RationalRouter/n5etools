package encounter

import (
	"fmt"
	"math/rand"
	"testing"
)

func jutsuFixture() []Jutsu {
	var out []Jutsu
	for _, rank := range []string{"D", "C", "B"} {
		for i := 0; i < 8; i++ {
			class := []string{"Ninjutsu", "Genjutsu", "Taijutsu", "Bukijutsu"}[i%4]
			out = append(out, Jutsu{Slug: fmt.Sprintf("jutsu/%s-%d", rank, i), Name: fmt.Sprintf("%s %d", rank, i), Rank: rank, Classification: class})
		}
	}
	return append(out,
		Jutsu{Slug: "jutsu/fireball", Name: "Fireball", Rank: "C", Classification: "Hijutsu", ClanSlug: "clan/uchiha"},
		Jutsu{Slug: "jutsu/gentle-fist", Name: "Gentle Fist", Rank: "D", Classification: "Hijutsu", ClanSlug: "clan/hyuga"},
	)
}

func TestClanJutsuSlug(t *testing.T) {
	for tag, want := range map[string]string{
		"Uchiha":               "clan/uchiha",
		"Synthetic Human":      "clan/synthetic-human",
		"Wukong, Low Class T7": "clan/wukong-low-class-t7",
	} {
		if got := ClanJutsuSlug(tag); got != want {
			t.Errorf("ClanJutsuSlug(%q) = %q, want %q", tag, got, want)
		}
	}
}

func TestJutsuEligibleHonorsRankAndClan(t *testing.T) {
	adv := GeneratedAdversary{AdvRank: "C", ClanTag: "Uchiha"}
	cases := []struct {
		j    Jutsu
		want bool
	}{
		{Jutsu{Rank: "D"}, true},
		{Jutsu{Rank: "C"}, true},
		{Jutsu{Rank: "B"}, false},
		{Jutsu{Rank: ""}, false},
		{Jutsu{Rank: "C", ClanSlug: "clan/uchiha"}, true},
		{Jutsu{Rank: "D", ClanSlug: "clan/hyuga"}, false},
	}
	for _, c := range cases {
		if got := JutsuEligible(c.j, adv); got != c.want {
			t.Errorf("JutsuEligible(%+v) = %v, want %v", c.j, got, c.want)
		}
	}
	if JutsuEligible(Jutsu{Rank: "D", ClanSlug: "clan/non-clan"}, GeneratedAdversary{AdvRank: "C", ClanTag: NoClan}) {
		t.Error("an adversary without a clan knows no hijutsu")
	}
}

func TestJutsuInClassMatchesEitherClassification(t *testing.T) {
	j := Jutsu{Classification: "Hijutsu, Bukijutsu"}
	if !JutsuInClass(j, "hijutsu") || !JutsuInClass(j, "bukijutsu") || JutsuInClass(j, "ninjutsu") || JutsuInClass(j, "") {
		t.Errorf("JutsuInClass misread %q", j.Classification)
	}
}

func TestPickJutsuFillsQuotasWithinRank(t *testing.T) {
	kit := fixtureToolkit()
	kit.Jutsu = jutsuFixture()
	adv := GeneratedAdversary{
		RankSlug: "adversary-rank/solo", RoleSlug: "adversary-builder-role/lurker", Level: 12,
		AdvRank: "C", ClanTag: "Uchiha", AffiliationTag: NoAffiliation, Armor: "light",
	}
	quotas := Compute(adv.Choices(1), kit.Tables).Jutsu
	want := 0
	for _, q := range quotas {
		want += q.Count
	}
	if want != 10 {
		t.Fatalf("fixture quotas = %+v, want 10 jutsu in all", quotas)
	}

	for seed := int64(1); seed <= 20; seed++ {
		got := pickJutsu(rand.New(rand.NewSource(seed)), kit, adv)
		if len(got) != want {
			t.Fatalf("seed %d: picked %d jutsu, want %d", seed, len(got), want)
		}
		seen := map[string]bool{}
		dCount := 0
		for i, j := range got {
			if seen[j.Slug] {
				t.Fatalf("seed %d: %s picked twice", seed, j.Slug)
			}
			seen[j.Slug] = true
			if RankIndex(j.Rank) > RankIndex("C") {
				t.Fatalf("seed %d: %s is above the adversary's Rank", seed, j.Slug)
			}
			if j.Slug == "jutsu/gentle-fist" {
				t.Fatalf("seed %d: picked another clan's hijutsu", seed)
			}
			if j.Rank == "D" {
				dCount++
			}
			if i > 0 && RankIndex(got[i-1].Rank) > RankIndex(j.Rank) {
				t.Fatalf("seed %d: jutsu not sorted by rank: %+v", seed, got)
			}
		}
		if dCount < 4 {
			t.Fatalf("seed %d: %d D-rank jutsu, want at least the D quota of 4", seed, dCount)
		}
	}
}

func TestGenerateOneAndRerollPickJutsu(t *testing.T) {
	kit := fixtureToolkit()
	kit.Jutsu = jutsuFixture()
	rng := rand.New(rand.NewSource(3))
	adv := GenerateOne(rng, kit, classOf(kit, "solo"), 12, GenOptions{})
	if len(adv.Jutsu) == 0 {
		t.Fatal("a generated Solo knows no jutsu")
	}
	re := Reroll(rng, kit, classOf(kit, "solo"), adv)
	if len(re.Jutsu) != len(adv.Jutsu) {
		t.Fatalf("reroll picked %d jutsu, want %d", len(re.Jutsu), len(adv.Jutsu))
	}
}

func TestMaxJutsuRankAndQuotaFit(t *testing.T) {
	quotas := []JutsuQuota{{Rank: "D", Count: 4}, {Rank: "C", Count: 3}, {Rank: "B", Count: 1}}
	if got := MaxJutsuRank(quotas); got != "B" {
		t.Errorf("MaxJutsuRank = %q, want B", got)
	}
	if got := MaxJutsuRank(nil); got != "" {
		t.Errorf("MaxJutsuRank(nil) = %q, want empty", got)
	}
	cases := []struct {
		ranks []string
		want  bool
	}{
		{nil, true},
		{[]string{"D", "D", "C", "B"}, true},
		{[]string{"C", "C", "C", "C"}, true},       // a C may fill a D slot
		{[]string{"B", "B"}, false},                // one B slot
		{[]string{"A"}, false},                     // above every quota
		{[]string{"C", "C", "C", "B", "B"}, false}, // two B
		{[]string{"D", "D", "D", "D", "D", "C", "C", "C"}, true},
		{[]string{"D", "D", "D", "D", "D", "C", "C", "C", "B"}, false}, // 9 > 8
	}
	for _, c := range cases {
		if got := JutsuWithinQuotas(quotas, c.ranks); got != c.want {
			t.Errorf("JutsuWithinQuotas(%v) = %v, want %v", c.ranks, got, c.want)
		}
	}
}
