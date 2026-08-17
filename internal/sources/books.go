// Package sources holds the canonical, creator-maintained locations of the
// official N5E sourcebooks.
//
// The community creator ("King") deliberately keeps these Google Drive file
// IDs immutable as the game updates — the same link always points at the
// current version of a book, so hardcoding them here is intentional, not a
// fragile shortcut. This is what makes the "point the app at a Drive link
// and it re-ingests whatever's new" feature possible: Slug matches the
// SourceBook.Slug each ingest function already uses in cmd/n5e-ingest and
// internal/ingest, so internal/autoupdate can hand a downloaded file
// straight to the existing loaders on every app startup.
package sources

// BookSource is one official sourcebook's canonical location.
type BookSource struct {
	Slug     string // matches store.SourceBook.Slug used at ingestion
	Title    string
	DriveURL string
	Required bool // false for the Kage Guide — DM-facing, not needed to play
}

// OfficialBooks lists every official N5E sourcebook, in the order King's
// own book list presents them.
var OfficialBooks = []BookSource{
	{
		Slug:     "book/core",
		Title:    "Shinobi Hand Book",
		DriveURL: "https://drive.google.com/file/d/1lSqTKAbzWptSUhov9iHsslxk8H-B3W5b/view?usp=drive_link",
		Required: true,
	},
	{
		Slug:     "book/class-compendium",
		Title:    "Orochimaru's Observation Compendium",
		DriveURL: "https://drive.google.com/file/d/1Q_VwNvOs1ry6Zw9SW9DLqWnbZyNHSZdu/view?usp=drive_link",
		Required: true,
	},
	{
		Slug:     "book/clan-compendium",
		Title:    "Tsunade's Studies Compendium",
		DriveURL: "https://drive.google.com/file/d/19cqLmA1fXgtaVFufdw8iSVVw3vrSlzk-/view?usp=drive_link",
		Required: true,
	},
	{
		Slug:     "book/jutsu-compendium",
		Title:    "Jiraiya's Jutsu Compendium",
		DriveURL: "https://drive.google.com/file/d/1wOsPeLLN85kQ2knL9slxyoZCL6VepR22/view?usp=sharing",
		Required: true,
	},
	{
		Slug:     "book/kage-guide",
		Title:    "Kage Guide (Dungeon Master Guide)",
		DriveURL: "https://drive.google.com/file/d/1gbs5MxYxfalxSNtr1fVGF3SD-S6-xT15/view",
		Required: false,
	},
	{
		Slug:     "book/bingo-book",
		Title:    "Bingo Book Pack 1 — Adversaries",
		DriveURL: "https://drive.google.com/file/d/14XB-_aiLeSPnqDHad--q0A9rhw4_b7in/view?usp=sharing",
		Required: false, // GM-facing, like the Kage Guide
	},
}
