package encounter

import (
	"math/rand"
	"strings"
)

// Adversary names mix modern Japanese given names and family names with
// names borrowed from popular anime. At most one half of a name comes from
// the anime lists, so a generated name is never a well-known character's
// full name; famousNames catches the few pairs a modern name still forms.

var modernGivenNames = []string{
	"Akari", "Arata", "Asahi", "Ayaka", "Ayane", "Chihiro", "Daichi", "Daiki",
	"Ema", "Hana", "Haruka", "Haruki", "Haruto", "Hayato", "Hikari", "Himari",
	"Hinata", "Hiroto", "Itsuki", "Kaede", "Kaito", "Kanon", "Kazuki", "Kenta",
	"Koharu", "Kota", "Mai", "Mei", "Minato", "Mio", "Misaki", "Miyu",
	"Nanami", "Nao", "Natsuki", "Ren", "Riko", "Riku", "Rin", "Ryota",
	"Saki", "Sakura", "Shiori", "Shota", "Sora", "Sota", "Taiga", "Takumi",
	"Tsubasa", "Tsumugi", "Yamato", "Yui", "Yuma", "Yuna", "Yuto",
}

var modernFamilyNames = []string{
	"Abe", "Aoki", "Endo", "Fujii", "Fujita", "Fukuda", "Goto", "Harada",
	"Hasegawa", "Hashimoto", "Hayashi", "Ikeda", "Inoue", "Ishii", "Ishikawa", "Ito",
	"Kaneko", "Kato", "Kimura", "Kobayashi", "Kondo", "Matsuda", "Matsumoto", "Miura",
	"Mori", "Murakami", "Nakagawa", "Nakamura", "Nakano", "Nishimura", "Ogawa", "Okada",
	"Okamoto", "Ono", "Ota", "Sakamoto", "Sasaki", "Sato", "Shimizu", "Suzuki",
	"Takahashi", "Takeda", "Tanaka", "Wada", "Watanabe", "Yamada", "Yamaguchi", "Yamamoto",
	"Yamazaki", "Yoshida",
}

var animeGivenNames = []string{
	// Naruto
	"Asuma", "Choji", "Danzo", "Deidara", "Haku", "Hashirama", "Hiruzen", "Itachi",
	"Jiraiya", "Kabuto", "Kakashi", "Kisame", "Konan", "Kurenai", "Kushina", "Madara",
	"Nagato", "Neji", "Obito", "Shikamaru", "Shisui", "Tobirama", "Tsunade", "Zabuza",
	// Demon Slayer
	"Giyu", "Inosuke", "Kyojuro", "Mitsuri", "Muichiro", "Nezuko", "Shinobu", "Tanjiro",
	"Tengen", "Zenitsu",
	// Jujutsu Kaisen
	"Kento", "Maki", "Megumi", "Nobara", "Satoru", "Suguru", "Toge", "Yuji", "Yuta",
	// My Hero Academia
	"Eijiro", "Fumikage", "Izuku", "Katsuki", "Momo", "Ochaco", "Shoto", "Tenya",
	"Toshinori", "Tsuyu",
	// Bleach
	"Byakuya", "Ichigo", "Kenpachi", "Orihime", "Renji", "Rukia", "Shunsui", "Sosuke",
	"Toshiro", "Uryu",
	// Hunter x Hunter, Neon Genesis Evangelion, Rurouni Kenshin, Attack on Titan
	"Gon", "Hisoka", "Killua", "Kurapika", "Asuka", "Rei", "Shinji", "Kaoru",
	"Kenshin", "Sanosuke", "Mikasa",
}

var animeFamilyNames = []string{
	// Demon Slayer
	"Agatsuma", "Hashibira", "Kamado", "Kanroji", "Kocho", "Rengoku", "Tokito", "Tomioka",
	"Uzui",
	// Jujutsu Kaisen
	"Fushiguro", "Geto", "Gojo", "Inumaki", "Itadori", "Kugisaki", "Okkotsu", "Zenin",
	// My Hero Academia
	"Asui", "Bakugo", "Iida", "Kirishima", "Midoriya", "Todoroki", "Tokoyami", "Uraraka",
	"Yaoyorozu",
	// Bleach
	"Abarai", "Aizen", "Hitsugaya", "Ishida", "Kuchiki", "Kurosaki", "Kyoraku", "Zaraki",
	// Neon Genesis Evangelion, Rurouni Kenshin
	"Ayanami", "Ikari", "Soryu", "Himura", "Kamiya", "Sagara",
}

// famousNames are well-known characters' full names that a modern half
// can still form with an anime or clan half.
var famousNames = map[string]bool{
	"Hinata Hyuga":    true,
	"Minato Namikaze": true,
	"Orihime Inoue":   true,
	"Mai Zenin":       true,
}

// clanFamilyNames are the adversary clan tags that are also family names;
// an adversary of one of these clans carries it as its own.
var clanFamilyNames = map[string]string{
	"Aburame": "Aburame", "Akimichi": "Akimichi", "Chinoike": "Chinoike", "Fuma": "Fuma",
	"Hatake": "Hatake", "Hoshigaki": "Hoshigaki", "Hozuki": "Hozuki", "Hyuga": "Hyuga",
	"Iburi": "Iburi", "Inuzuka": "Inuzuka", "Kageyama T7": "Kageyama", "Kaguya": "Kaguya",
	"Kato T7": "Kato", "Kurama": "Kurama", "Namikaze": "Namikaze", "Nara": "Nara",
	"Otsutsuki": "Otsutsuki", "Sarutobi": "Sarutobi", "Senju": "Senju", "Uchiha": "Uchiha",
	"Uzumaki": "Uzumaki", "Yamada": "Yamada", "Yamada T7": "Yamada", "Yamanaka": "Yamanaka",
	"Yuki": "Yuki",
}

// ClanFamilyName is the family name an adversary of clanTag carries, or ""
// when the clan is not a family.
func ClanFamilyName(clanTag string) string {
	return clanFamilyNames[clanTag]
}

// RandomName gives an adversary of clanTag a name, given name first. A clan
// that is a family supplies the family name, paired with a modern given
// name; otherwise one half comes from the anime lists or neither does.
func RandomName(rng *rand.Rand, clanTag string) string {
	for {
		var given, family string
		if clan := ClanFamilyName(clanTag); clan != "" {
			given, family = pickName(rng, modernGivenNames), clan
		} else {
			switch rng.Intn(3) {
			case 0:
				given, family = pickName(rng, animeGivenNames), pickName(rng, modernFamilyNames)
			case 1:
				given, family = pickName(rng, modernGivenNames), pickName(rng, animeFamilyNames)
			default:
				given, family = pickName(rng, modernGivenNames), pickName(rng, modernFamilyNames)
			}
		}
		name := given + " " + family
		if !strings.EqualFold(given, family) && !famousNames[name] {
			return name
		}
	}
}

func pickName(rng *rand.Rand, list []string) string {
	return list[rng.Intn(len(list))]
}
