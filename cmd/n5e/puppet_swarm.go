package main

import (
	"database/sql"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// This file covers Red Technique's Performance of 10 Puppets (L10) and its
// L20 escalation, Master of the Red Technique's Performance of 100 Puppets
// — together "the Puppet Swarm," see migration
// 0035_puppet_swarm_stat_block.sql's own header comment for where its
// prose lived before this split. The Puppet Swarm has no combat-resolution
// engine behind it (this app tracks no turn-by-turn action economy at
// all), so only its own AC/HP/Speed/ability-score card is computed live;
// Commands, the three Special Actions, and every other fixed trait render
// straight from puppet_swarm_stat_block's own raw_text as reference text —
// same "no computed hook -> reference text" boundary every other
// Foundation/Role trait in this codebase already uses.

// loadPuppetSwarmReferenceText reads back puppet_swarm_stat_block's own
// raw_text — "" if the row is missing (a fresh/test rulesDB with no
// migration-time seed data for 'class/puppet-master').
func (s *server) loadPuppetSwarmReferenceText() (string, error) {
	var raw string
	err := s.rulesDB.QueryRow(
		`SELECT raw_text FROM puppet_swarm_stat_block WHERE class_slug = 'class/puppet-master'`,
	).Scan(&raw)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return raw, nil
}

// puppetSwarmStatsView is the Puppets tab's own computed, read-only Puppet
// Swarm card.
type puppetSwarmStatsView struct {
	AC                                             int
	HP, HP100                                      int // HP100 is what HP becomes once Performance of 100 is active (x2 instead of x1.5)
	Speed                                          int
	Str, Dex, Con, Int, Wis, Cha                   int
	StrMod, DexMod, ConMod, IntMod, WisMod, ChaMod int
	// Has100 gates the Performance of 100 Puppets escalation itself
	// (Master of the Red Technique, L20) — below it, only Performance of
	// 10 Puppets (L10) is available and Commands100/HP100 don't apply.
	Has100      bool
	Commands    int
	Commands100 int
	// ReferenceText is puppet_swarm_stat_block's own raw_text — the fixed
	// traits (One of Many, Puppet Tool, Puppet Swarm, Performance of 100
	// Puppets) and the three Commands-spending Special Actions (Attack,
	// Defend, Swarm), none of which this app can resolve mechanically.
	ReferenceText string
}

// puppetSwarmStats computes the Puppet Swarm's own stat card from the
// character's current kind="puppet" companions, per the book's own text:
// "AC equal to your Puppet Tool with the highest AC + 2," HP "(The Sum of
// all your Puppet's Hit Points) x 1.5" (x2 once Performance of 100 is
// active), Speed a flat 60ft., and "choose the highest stat for each
// ability score from each of your Puppets." A companion with no AC/HP/
// ability score entered yet simply contributes nothing for that field —
// same blank-means-0 treatment companionAbilityModifier already gives an
// individual companion's own unset ability scores.
func puppetSwarmStats(companions []charstore.Companion, level int) puppetSwarmStatsView {
	v := puppetSwarmStatsView{Speed: 60, Commands: 5}
	v.Has100 = level >= 20
	if v.Has100 {
		v.Commands100 = 10
	}

	var hpSum int
	for _, c := range companions {
		if c.Kind != "puppet" {
			continue
		}
		if c.AC.Valid && int(c.AC.Int64) > v.AC {
			v.AC = int(c.AC.Int64)
		}
		if c.HPMax.Valid {
			hpSum += int(c.HPMax.Int64)
		}
		if c.Str.Valid && int(c.Str.Int64) > v.Str {
			v.Str = int(c.Str.Int64)
		}
		if c.Dex.Valid && int(c.Dex.Int64) > v.Dex {
			v.Dex = int(c.Dex.Int64)
		}
		if c.Con.Valid && int(c.Con.Int64) > v.Con {
			v.Con = int(c.Con.Int64)
		}
		if c.Int.Valid && int(c.Int.Int64) > v.Int {
			v.Int = int(c.Int.Int64)
		}
		if c.Wis.Valid && int(c.Wis.Int64) > v.Wis {
			v.Wis = int(c.Wis.Int64)
		}
		if c.Cha.Valid && int(c.Cha.Int64) > v.Cha {
			v.Cha = int(c.Cha.Int64)
		}
	}
	if v.AC > 0 {
		v.AC += 2
	}
	v.HP = int(float64(hpSum) * 1.5)
	v.HP100 = hpSum * 2

	v.StrMod = charsheet.AbilityModifier(v.Str)
	v.DexMod = charsheet.AbilityModifier(v.Dex)
	v.ConMod = charsheet.AbilityModifier(v.Con)
	v.IntMod = charsheet.AbilityModifier(v.Int)
	v.WisMod = charsheet.AbilityModifier(v.Wis)
	v.ChaMod = charsheet.AbilityModifier(v.Cha)
	return v
}
