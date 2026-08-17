package puppetupgrades

import "testing"

// TestLurkerRoleEffect locks in Puppet Roles' Lurker's own Role Effect
// text: "+1 to critical hit range" (CritRangeBonus) and "on a critical hit,
// add your Proficiency Bonus to the damage dealt" (CritDamageBonus) — the
// two real hooks Item 3 wires into a companion's attack rows.
func TestLurkerRoleEffect(t *testing.T) {
	entry, ok := FoundationEntries["class/puppet-master/option/puppet-roles/lurker"]
	if !ok {
		t.Fatal("Lurker entry not found in FoundationEntries")
	}
	if entry.RoleEffect == nil {
		t.Fatal("Lurker has no RoleEffect")
	}
	if entry.RoleEffect.CritRangeBonus != 1 {
		t.Errorf("CritRangeBonus = %d, want 1", entry.RoleEffect.CritRangeBonus)
	}
	if entry.RoleEffect.CritDamageBonus == nil {
		t.Fatal("CritDamageBonus is nil")
	}
	if got := entry.RoleEffect.CritDamageBonus(4); got != 4 {
		t.Errorf("CritDamageBonus(4) = %d, want 4 (a flat Proficiency Bonus add)", got)
	}

	// No other Role should carry a CritDamageBonus — Lurker is the only
	// entry whose text grants one.
	for slug, e := range FoundationEntries {
		if slug == entry.EntrySlug || e.RoleEffect == nil {
			continue
		}
		if e.RoleEffect.CritDamageBonus != nil {
			t.Errorf("%s unexpectedly has a CritDamageBonus — only Lurker should", slug)
		}
	}
}
