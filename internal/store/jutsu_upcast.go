package store

import (
	"database/sql"
	"regexp"
	"strconv"
	"strings"
)

// upcastOpenerPattern matches a sentence's opening "For each rank ...",
// "For every rank ...", or the book's occasional typo "For Rank, ..."
// (missing "each"/"every") — the shape every genuine rank-based upcast
// rule in the jutsu compendium opens with. Anchoring to this opener,
// rather than scanning the whole at_higher_ranks blob for any
// "cost ... by N", is what keeps a handful of jutsu with bespoke,
// non-linear upcast rules (Flying Thunder God's teleport-distance rule is
// the confirmed example: it mentions "the cost to teleport by 10" but that
// number is not a per-rank chakra delta) from producing a confidently
// wrong number — those jutsu's cost mentions never sit inside a sentence
// that opens this way.
var upcastOpenerPattern = regexp.MustCompile(`(?i)^for\s+(?:each|every)?\s*rank\b`)

// upcastCostPattern extracts the chakra delta from a sentence already
// confirmed to be an upcast rule by upcastOpenerPattern. Both "cost" and
// "chakra spent" occur in the real data ("increase the cost of this jutsu
// by 3", "increase the chakra spent activating this jutsu... by 3"); the
// non-greedy .*? finds the FIRST "by N" after whichever comes first, so
// "increase the Chakra cost of this jutsu by 3 or the Calorie cost by 1"
// (an Akimichi jutsu with an alternate resource) correctly yields 3, not
// 1 — the unrelated resource's own "cost ... by N" never precedes the
// chakra one in these sentences.
var upcastCostPattern = regexp.MustCompile(`(?i)(?:cost|chakra spent).*?\bby\s+(\d+)`)

// upcastDefaultDelta is the book's unstated default upcast cost, confirmed
// directly with the project's rules authority: a jutsu whose "At Higher
// Ranks" text is a genuine per-rank upcast rule (opens with "For each/every
// rank...") but never prints its own chakra delta still costs the standard
// 3 chakra per rank — the same delta the overwhelming majority of jutsu
// that DO state it explicitly use.
const upcastDefaultDelta = 3

// ParseChakraPerRank extracts a jutsu's per-rank chakra-upcast delta from
// its printed "At Higher Ranks" text.
//
// Returns nil when the text isn't a rank-based upcast rule at all — a
// level-gated auto-scaling rule with no extra cost, or a bespoke rule like
// Flying Thunder God's teleport-distance mechanic (confirmed directly
// against the PDF: it never opens with "For each/every rank", so it never
// reaches the default-delta fallback either) — rather than guess.
//
// When a sentence DOES open with "For each/every rank..." but states no
// chakra number of its own, this returns upcastDefaultDelta rather than
// nil (as of the friend's confirmation above) — that opener is what marks
// a sentence as a genuine per-rank upcast rule in the first place, so
// "found the shape but not the number" means "uses the book's default,"
// not "not upcastable." An explicit number anywhere in the text always
// wins over the default.
//
// Sampled directly against the real jutsu compendium: ~1043 jutsu state
// their own delta explicitly (almost always 3, with confirmed 4s and 5s);
// a handful more only imply the rule shape and fall to the default here.
// Callers should treat a nil result as "this jutsu has no automatic upcast
// cost" and fall back to the sheet's existing manual chakra-edit path.
func ParseChakraPerRank(atHigherRanks string) *int {
	sawOpener := false
	for _, sentence := range strings.Split(atHigherRanks, ".") {
		sentence = strings.TrimSpace(sentence)
		if sentence == "" || !upcastOpenerPattern.MatchString(sentence) {
			continue
		}
		sawOpener = true
		m := upcastCostPattern.FindStringSubmatch(sentence)
		if m == nil {
			continue
		}
		if n, err := strconv.Atoi(m[1]); err == nil {
			return &n
		}
	}
	if sawOpener {
		n := upcastDefaultDelta
		return &n
	}
	return nil
}

// upsertJutsuUpcastRule keeps jutsu_upcast_rules in sync with a jutsu row's
// current at_higher_ranks text, following the same override-preservation
// shape as upsertJutsu: chakra_per_rank_override is never touched here, and
// a human 'verified'/'manual' status is only demoted to 'needs_review' when
// the freshly PARSED value actually changes underneath it. A jutsu whose
// at_higher_ranks text is empty gets its row removed entirely — "no row"
// means "no upcast rule", distinct from "a rule exists but didn't parse"
// (chakra_per_rank IS NULL).
func upsertJutsuUpcastRule(tx *sql.Tx, slug, atHigherRanks string) error {
	if atHigherRanks == "" {
		_, err := tx.Exec(`DELETE FROM jutsu_upcast_rules WHERE jutsu_slug = ?`, slug)
		return err
	}

	parsed := ParseChakraPerRank(atHigherRanks)
	var parsedVal any
	if parsed != nil {
		parsedVal = *parsed
	}
	defaultStatus := "needs_review"
	if parsed != nil {
		defaultStatus = "auto"
	}

	var old struct {
		chakraPerRank sql.NullInt64
		status        string
	}
	err := tx.QueryRow(`
		SELECT chakra_per_rank, detection_status FROM jutsu_upcast_rules WHERE jutsu_slug = ?`,
		slug).Scan(&old.chakraPerRank, &old.status)

	if err == sql.ErrNoRows {
		_, err := tx.Exec(`
			INSERT INTO jutsu_upcast_rules (jutsu_slug, chakra_per_rank, detection_status)
			VALUES (?, ?, ?)`, slug, parsedVal, defaultStatus)
		return err
	}
	if err != nil {
		return err
	}

	if nullableIntEq(old.chakraPerRank, parsed) {
		return nil
	}

	newStatus := defaultStatus
	if old.status == "verified" || old.status == "manual" {
		newStatus = "needs_review"
	}
	_, err = tx.Exec(`
		UPDATE jutsu_upcast_rules SET chakra_per_rank = ?, detection_status = ?
		WHERE jutsu_slug = ?`, parsedVal, newStatus, slug)
	return err
}
