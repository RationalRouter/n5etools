// Package embedded holds the rules database exactly as it ships inside the
// n5e binary — read-only game content built by n5e-ingest, frozen at build
// time. rules.db is a build artifact (copied in by `make embed-rules`), not
// source; it's gitignored the same way out/rules.db already is.
package embedded

import _ "embed"

//go:embed rules.db
var RulesDB []byte
