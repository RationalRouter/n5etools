-- Upcasting: many jutsu's free-text `at_higher_ranks` column describes a
-- rule of the shape "For each rank you cast this jutsu above X-Rank,
-- increase the cost of this jutsu by N" (real wording varies — "activation
-- cost", "chakra spent activating", occasionally missing the word
-- "increase" entirely). internal/store/jutsu_upcast.go regex-parses that
-- prose at ingest time and lands the per-rank chakra delta here, so the
-- character sheet can offer a real "cast at a higher rank" choice instead
-- of always spending the printed base cost.
--
-- One row per jutsu that HAS at_higher_ranks text, whether or not the
-- delta parsed — chakra_per_rank IS NULL means "there's an upcast rule,
-- but we couldn't extract a number from it" (rank-gated auto-scaling with
-- no extra cost, an alternate resource like Akimichi's Calorie cost, or
-- genuinely unparseable prose), distinct from "no upcast rule at all"
-- (no row here). The raw prose itself is NOT duplicated onto this table —
-- it already lives on jutsu.at_higher_ranks and is already rendered on the
-- jutsu detail page.
CREATE TABLE jutsu_upcast_rules (
    jutsu_slug               TEXT PRIMARY KEY REFERENCES jutsu(slug),
    chakra_per_rank          INTEGER,          -- parsed delta, NULL if unparsed
    chakra_per_rank_override INTEGER,          -- human correction, same convention
                                                -- as jutsu.rank_override — maintainer-only,
                                                -- no runtime edit UI
    detection_status         TEXT NOT NULL DEFAULT 'auto'
                             CHECK (detection_status IN ('auto','needs_review','verified','manual'))
);

DROP VIEW v_jutsu;
CREATE VIEW v_jutsu AS
SELECT j.slug, j.name, j.classification,
       COALESCE(j.rank_override, j.rank) AS rank,
       j.casting_time, j.range, j.duration, j.components, j.cost_chakra, j.cost_text,
       j.keywords, j.description, j.at_higher_ranks, j.category_group, j.clan_slug,
       COALESCE(u.chakra_per_rank_override, u.chakra_per_rank) AS chakra_per_rank,
       j.detection_status, j.notes
FROM jutsu j
LEFT JOIN jutsu_upcast_rules u ON u.jutsu_slug = j.slug;
