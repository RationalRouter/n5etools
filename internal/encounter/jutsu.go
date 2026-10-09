package encounter

import (
	"math/rand"
	"sort"
	"strings"
)

// Jutsu is one rules.db jutsu an adversary can know (Bingo Book p.6, step
// 9). ClanSlug is set for a clan's hijutsu.
type Jutsu struct {
	Slug, Name, Rank, Classification, CostText, ClanSlug string
}

// GeneratedJutsu is one jutsu an adversary knows, as stored with it.
type GeneratedJutsu struct {
	Slug, Name, Rank, Classification, CostText string
}

// JutsuClasses are the classifications step 9's pool can be narrowed to.
var JutsuClasses = []string{"ninjutsu", "genjutsu", "taijutsu", "bukijutsu", "hijutsu"}

// JutsuInClass reports whether j is of class, one of JutsuClasses. A jutsu
// listed under two classifications ("Hijutsu, Bukijutsu") is in both.
func JutsuInClass(j Jutsu, class string) bool {
	return class != "" && strings.Contains(strings.ToLower(j.Classification), class)
}

// ClanJutsuSlug is the jutsu.clan_slug a clan tag's hijutsu carry
// ("Synthetic Human" -> "clan/synthetic-human").
func ClanJutsuSlug(tag string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(tag) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			dash = false
		} else {
			dash = true
		}
	}
	return "clan/" + b.String()
}

// JutsuEligible reports whether adv can know j: its rank is at most adv's
// Rank, and a hijutsu belongs to adv's clan.
func JutsuEligible(j Jutsu, adv GeneratedAdversary) bool {
	ri := RankIndex(j.Rank)
	if ri < 0 || ri > RankIndex(adv.AdvRank) {
		return false
	}
	return j.ClanSlug == "" || (adv.ClanTag != NoClan && j.ClanSlug == ClanJutsuSlug(adv.ClanTag))
}

// MaxJutsuRank is the highest rank among quotas, or "" when they allow no
// jutsu. The book's Jutsu Known lists (pp.9-11) set it by level, so it can
// sit below the adversary's own Rank.
func MaxJutsuRank(quotas []JutsuQuota) string {
	best := ""
	for _, q := range quotas {
		if q.Count > 0 && RankIndex(q.Rank) > RankIndex(best) {
			best = q.Rank
		}
	}
	return best
}

// JutsuWithinQuotas reports whether jutsu of ranks can fill quotas: at
// every rank, no more of them are at or above it than the quotas allow at
// or above it. A jutsu above every quota fails.
func JutsuWithinQuotas(quotas []JutsuQuota, ranks []string) bool {
	for _, t := range AdversaryRanks {
		ti := RankIndex(t)
		have, room := 0, 0
		for _, r := range ranks {
			if RankIndex(r) >= ti {
				have++
			}
		}
		for _, q := range quotas {
			if RankIndex(q.Rank) >= ti {
				room += q.Count
			}
		}
		if have > room {
			return false
		}
	}
	return true
}

// Jutsu weights for pickJutsu: a Caster's or Striker's own discipline, the
// adversary's clan hijutsu, and a jutsu of exactly the quota's rank are
// favored over the rest of the pool.
const (
	jutsuDisciplineWeight = 4
	jutsuClanWeight       = 4
	jutsuQuotaRankWeight  = 3
)

// pickJutsu fills adv's jutsu quotas from kit's pool, lowest rank first,
// with weighted sampling without replacement.
func pickJutsu(rng *rand.Rand, kit Toolkit, adv GeneratedAdversary) []GeneratedJutsu {
	if len(kit.Jutsu) == 0 {
		return nil
	}
	quotas := Compute(adv.Choices(1), kit.Tables).Jutsu
	discipline := RoleDiscipline(adv.RoleSlug)
	var pool []Jutsu
	for _, j := range kit.Jutsu {
		if JutsuEligible(j, adv) {
			pool = append(pool, j)
		}
	}

	taken := map[string]bool{}
	var picked []Jutsu
	for _, q := range quotas {
		type keyed struct {
			j   Jutsu
			key float64
		}
		var cands []keyed
		for _, j := range pool {
			if taken[j.Slug] || RankIndex(j.Rank) > RankIndex(q.Rank) {
				continue
			}
			w := 1.0
			if JutsuInClass(j, discipline) {
				w *= jutsuDisciplineWeight
			}
			if j.ClanSlug != "" {
				w *= jutsuClanWeight
			}
			if j.Rank == q.Rank {
				w *= jutsuQuotaRankWeight
			}
			cands = append(cands, keyed{j, rng.ExpFloat64() / w})
		}
		sort.Slice(cands, func(a, b int) bool { return cands[a].key < cands[b].key })
		for _, c := range cands[:min(q.Count, len(cands))] {
			taken[c.j.Slug] = true
			picked = append(picked, c.j)
		}
	}
	SortJutsu(picked)
	out := make([]GeneratedJutsu, len(picked))
	for i, j := range picked {
		out[i] = GeneratedJutsu{Slug: j.Slug, Name: j.Name, Rank: j.Rank, Classification: j.Classification, CostText: j.CostText}
	}
	return out
}

// SortJutsu orders jutsu by rank, then name.
func SortJutsu(list []Jutsu) {
	sort.SliceStable(list, func(a, b int) bool {
		ra, rb := RankIndex(list[a].Rank), RankIndex(list[b].Rank)
		if ra != rb {
			return ra < rb
		}
		return list[a].Name < list[b].Name
	})
}
