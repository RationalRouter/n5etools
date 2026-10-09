// Loading internal/bingobookdata's hand transcription of the Bingo Book
// (book/bingo-book) into the rules database: the NPC template stat blocks,
// the General Traits glossary and the Specialist Jutsu Lists (migration
// 0080). The transcription is the whole source of truth for these tables, so
// every load is a delete+reload, the same convention as
// adversary_builder_sheet.go.
package store

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/RationalRouter/n5e/internal/bingobookdata"
)

// BingoBookTemplateCounts says how many rows each table family received.
type BingoBookTemplateCounts struct {
	StatBlocks, Skills, Entries, Jutsu, GeneralTraits, SpecialistJutsu int
}

// LoadBingoBookTemplates fully replaces the 0080 tables with the given data,
// in one transaction.
func LoadBingoBookTemplates(db *sql.DB, book SourceBook, blocks []bingobookdata.StatBlock,
	traits []bingobookdata.GeneralTrait, lists []bingobookdata.SpecialistList) (BingoBookTemplateCounts, error) {
	var n BingoBookTemplateCounts
	tx, err := db.Begin()
	if err != nil {
		return n, err
	}
	defer tx.Rollback()

	if err := upsertSourceBook(tx, book); err != nil {
		return n, err
	}
	for _, table := range []string{
		"adversary_template_skills", "adversary_template_entries", "adversary_template_jutsu",
		"adversary_template_statblocks", "adversary_beta_general_traits", "adversary_specialist_jutsu_lists",
	} {
		if _, err := tx.Exec(`DELETE FROM ` + table); err != nil {
			return n, fmt.Errorf("clearing %s: %w", table, err)
		}
	}

	for _, b := range blocks {
		if err := insertTemplateBlock(tx, book, b, &n); err != nil {
			return n, fmt.Errorf("%s: %w", b.Slug, err)
		}
	}

	for i, t := range traits {
		if _, err := tx.Exec(
			`INSERT INTO adversary_beta_general_traits
			 (slug, name, rank, keywords, description, sort_order, source_book, source_version,
			  detection_status, notes)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'manual', ?)`,
			bingobookdata.TraitSlug(t.Name), t.Name, t.Rank, t.Keywords, t.Description, i,
			book.Slug, book.Version, t.Notes); err != nil {
			return n, fmt.Errorf("general trait %q: %w", t.Name, err)
		}
		n.GeneralTraits++
	}

	for li, l := range lists {
		for ji, name := range l.Jutsu {
			if _, err := tx.Exec(
				`INSERT INTO adversary_specialist_jutsu_lists
				 (list_name, jutsu_name, list_order, sort_order, source_book, source_version)
				 VALUES (?, ?, ?, ?, ?, ?)`,
				l.Name, name, li, ji, book.Slug, book.Version); err != nil {
				return n, fmt.Errorf("specialist list %s/%s: %w", l.Name, name, err)
			}
			n.SpecialistJutsu++
		}
	}
	return n, tx.Commit()
}

func insertTemplateBlock(tx *sql.Tx, book SourceBook, b bingobookdata.StatBlock, n *BingoBookTemplateCounts) error {
	hp, err := bingobookdata.FirstHP(b.HP)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO adversary_template_statblocks
		 (slug, name, clan_slug, clan_name, rank_tier, class, role, affiliation, size, level, xp,
		  prof_bonus, tenacity_die, tenacity_count, ac, ac_note, dr, initiative_bonus, speed,
		  hp, hp_text, cp,
		  str_score, str_mod, str_save, dex_score, dex_mod, dex_save,
		  con_score, con_mod, con_save, int_score, int_mod, int_save,
		  wis_score, wis_mod, wis_save, cha_score, cha_mod, cha_save,
		  damage_resistances, damage_immunities, damage_vulnerabilities,
		  condition_resistances, condition_immunities, senses, passive_perception,
		  source_book, source_version, source_page, detection_status, notes)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
		         ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
		         ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'manual', ?)`,
		b.Slug, b.Name, b.ClanSlug, b.Clan, b.Rank, b.Class, b.Role, b.Affiliation, b.Size, b.Level, b.XP,
		b.Prof, b.TenacityDie, b.TenacityCount, b.AC, b.ACNote, b.DR, b.Initiative, b.Speed,
		hp, b.HP, b.CP,
		b.Str.Score, b.Str.Mod, b.Str.Save, b.Dex.Score, b.Dex.Mod, b.Dex.Save,
		b.Con.Score, b.Con.Mod, b.Con.Save, b.Int.Score, b.Int.Mod, b.Int.Save,
		b.Wis.Score, b.Wis.Mod, b.Wis.Save, b.Cha.Score, b.Cha.Mod, b.Cha.Save,
		b.DamageResistances, b.DamageImmunities, b.DamageVulnerabilities,
		b.ConditionResistances, b.ConditionImmunities, b.Senses, b.PassivePerception,
		book.Slug, book.Version, b.Page, b.Notes); err != nil {
		return err
	}
	n.StatBlocks++

	for i, s := range b.Skills {
		if _, err := tx.Exec(
			`INSERT INTO adversary_template_skills (statblock_slug, skill_name, modifier, stars, sort_order)
			 VALUES (?, ?, ?, ?, ?)`, b.Slug, s.Name, s.Mod, s.Stars, i); err != nil {
			return err
		}
		n.Skills++
	}

	for si, sec := range b.Sections {
		for ei, e := range sec.Entries {
			if _, err := tx.Exec(
				`INSERT INTO adversary_template_entries
				 (statblock_slug, section_order, section_kind, section_title, sort_order, name, tags, description)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				b.Slug, si, sec.Kind, sec.Title, ei, e.Name, e.Tags, e.Text); err != nil {
				return err
			}
			n.Entries++
		}
	}

	order := 0
	for _, d := range b.Jutsu {
		for _, r := range d.Ranks {
			isNew := map[string]bool{}
			for _, name := range r.New {
				isNew[strings.TrimSpace(name)] = true
			}
			for _, name := range r.Names {
				if _, err := tx.Exec(
					`INSERT INTO adversary_template_jutsu
					 (statblock_slug, discipline, hit_bonus, save_dc, rank, cost, jutsu_name, is_new, sort_order)
					 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					b.Slug, d.Discipline, d.Hit, d.DC, r.Rank, r.Cost, name, isNew[name], order); err != nil {
					return err
				}
				order++
				n.Jutsu++
			}
		}
	}
	return nil
}
