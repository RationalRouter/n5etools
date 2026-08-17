package puppetupgrades

import "testing"

func TestChassisHasMobile(t *testing.T) {
	cases := map[string]bool{
		"puppet-armor-chassis/weaved-mail":    true,
		"puppet-armor-chassis/wooden-suit":    true,
		"puppet-armor-chassis/iron-shell":     false,
		"puppet-armor-chassis/steel-fortress": false,
		"":                                    false,
		"not-a-real-chassis":                  false,
	}
	for slug, want := range cases {
		if got := ChassisHasMobile(slug); got != want {
			t.Errorf("ChassisHasMobile(%q) = %v, want %v", slug, got, want)
		}
	}
}

func TestChassisHasPowerfulBuild(t *testing.T) {
	cases := map[string]bool{
		"puppet-armor-chassis/steel-fortress": true,
		"puppet-armor-chassis/weaved-mail":    false,
		"puppet-armor-chassis/wooden-suit":    false,
		"puppet-armor-chassis/iron-shell":     false,
		"":                                    false,
	}
	for slug, want := range cases {
		if got := ChassisHasPowerfulBuild(slug); got != want {
			t.Errorf("ChassisHasPowerfulBuild(%q) = %v, want %v", slug, got, want)
		}
	}
}
