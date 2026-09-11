package i18n

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// This file implements the compiled coverage audit that replaced the old
// hand-run regex sweep (grep -rnE '(Name|SourceLabel|...):\s*"[A-Z]' ...,
// documented as superseded in translation_methodology.md). It is deliberately
// AST-based rather than line-regex-based so it can see the two gap shapes the
// regex structurally could not: a suspect field fed by a same-file
// package-level var/const rather than an inline literal, and any file
// outside cmd/n5e (the regex was only ever run there).
//
// Two violation classes, see AuditKind:
//
//   - hardcoded-literal: a struct-literal field whose name looks
//     player-facing (auditSuspectFields) is assigned an English string that
//     never touches TranslatableFields or the UI catalog.
//   - missing-overlay: a file reads a TranslatableFields table (a SQL
//     "FROM <table>") but never calls i18n.Row/i18n.Batch for that table
//     anywhere in the same file.
//
// `n5e-i18n audit` reports every NEW violation (baseline-filtered, see
// LoadAuditBaseline); `n5e-i18n audit -fix` additionally rewrites the safe
// subset of hardcoded-literal violations in place (auditFixableFields, and
// only where a translator is provably already in scope -- see fixLiteral).
// Everything else -- missing-overlay, and any hardcoded-literal field
// excluded from auto-fix -- is left for a human/AI review pass; the
// methodology log has repeated, confirmed incidents of exactly those field
// names being reused as runtime matching keys, so guessing a fix
// automatically is not safe. See translation_methodology.md's "Automated
// coverage pipeline" section for the full rationale.

// AuditScopeDirs are the source trees swept for player-facing text gaps,
// relative to the repo root.
var AuditScopeDirs = []string{"cmd/n5e", "internal"}

// auditExcludeDirs are subtrees within AuditScopeDirs that are
// infrastructure/tooling, not player-facing rendering code -- scanning them
// for hardcoded-literal produces mostly noise (CLI flag help text, log
// messages, migration SQL) and they're never expected to call
// i18n.Row/Batch, so missing-overlay would be all false positives there too.
var auditExcludeDirs = []string{
	"internal/i18n",     // this package itself, plus the glossary/catalog machinery
	"internal/schema",   // migrations
	"internal/autoupdate",
	"internal/backup",
	"internal/correct",  // grammar-correction tooling, not player-facing
	"internal/statlines", // machine stat-line parsing, not player-facing
	"cmd/n5e-ingest",
	"cmd/n5e-i18n",
}

// auditSuspectFields is the full set of struct-literal field names treated
// as likely player-facing prose for reporting purposes (hardcoded-literal).
var auditSuspectFields = map[string]bool{
	"Name": true, "Label": true, "Title": true, "SourceLabel": true,
	"Restriction": true, "Hint": true, "EmptyHint": true, "AddLabel": true,
	"GrantedHint": true, "TierName": true, "Text": true, "Description": true,
	"Error": true, "Placeholder": true, "Tooltip": true,
}

// auditFixableFields is the subset of auditSuspectFields that `audit -fix`
// is allowed to rewrite automatically. Name/Description/Text/Error are
// deliberately excluded: translation_methodology.md documents real
// incidents of exactly these field names being reused elsewhere as a
// runtime matching key (jutsu.keywords, S.N.B upgrade Name matching, a used-
// count lookup keyed by an already-translated name) or as an internal log
// message rather than player-facing prose. Those still get reported (and
// still get a catalog key registered, so the text exists for translation
// once a human confirms the fix), just never a call-site rewrite.
var auditFixableFields = map[string]bool{
	"Label": true, "Title": true, "SourceLabel": true, "Restriction": true,
	"Hint": true, "EmptyHint": true, "AddLabel": true, "GrantedHint": true,
	"TierName": true, "Tooltip": true, "Placeholder": true,
}

// AuditKind distinguishes the two violation shapes RunAudit reports.
type AuditKind string

const (
	AuditHardcodedLiteral AuditKind = "hardcoded-literal"
	AuditMissingOverlay   AuditKind = "missing-overlay"
)

// AuditViolation is one finding from RunAudit, already baseline-filtered.
type AuditViolation struct {
	Kind    AuditKind
	File    string // repo-root-relative path
	Line    int
	Field   string // hardcoded-literal only
	Table   string // missing-overlay only
	Text    string // the literal (hardcoded-literal) or a short description
	Fixable bool   // hardcoded-literal only: eligible for -fix
	Key     string // generated catalog key, once fixed (hardcoded-literal only)
}

// Fingerprint returns a stable string identifying this violation for
// baseline comparison -- deliberately excludes Text (English copy can
// legitimately change without the violation becoming a new one) and Key
// (generated fresh each run).
func (v AuditViolation) Fingerprint() string {
	switch v.Kind {
	case AuditMissingOverlay:
		return fmt.Sprintf("%s|%s|%s", v.Kind, v.File, v.Table)
	default:
		return fmt.Sprintf("%s|%s|%d|%s", v.Kind, v.File, v.Line, v.Field)
	}
}

// AuditResult is RunAudit's return value.
type AuditResult struct {
	New   []AuditViolation // not present in the baseline -- these are what fail the gate
	Fixed []AuditViolation // hardcoded-literal violations actually rewritten (fix mode only)
}

// RunAudit sweeps AuditScopeDirs (rooted at root) for both violation
// classes, filters out anything already present in the baseline file
// (internal/i18n/audit_baseline.json under root), and -- if fix is true --
// rewrites the safe subset of hardcoded-literal findings in place,
// registering a new catalog key in internal/i18n/locales/en.json for each.
func RunAudit(root string, fix bool) (AuditResult, error) {
	files, err := collectAuditFiles(root)
	if err != nil {
		return AuditResult{}, err
	}

	baseline, err := LoadAuditBaseline(root)
	if err != nil {
		return AuditResult{}, err
	}

	fset := token.NewFileSet()
	var all []AuditViolation
	parsed := make(map[string]*ast.File, len(files))
	for _, f := range files {
		af, err := parser.ParseFile(fset, f, nil, parser.ParseComments)
		if err != nil {
			return AuditResult{}, fmt.Errorf("parsing %s: %w", f, err)
		}
		parsed[f] = af
		lits, err := findHardcodedLiterals(fset, af, f, root)
		if err != nil {
			return AuditResult{}, err
		}
		all = append(all, lits...)
	}
	overlay, err := findMissingOverlays(files, root)
	if err != nil {
		return AuditResult{}, err
	}
	all = append(all, overlay...)

	sort.Slice(all, func(i, j int) bool {
		if all[i].File != all[j].File {
			return all[i].File < all[j].File
		}
		return all[i].Line < all[j].Line
	})

	var result AuditResult
	for _, v := range all {
		if baseline[v.Fingerprint()] {
			continue
		}
		result.New = append(result.New, v)
	}

	if fix {
		fixed, err := applyFixes(fset, parsed, root, result.New)
		if err != nil {
			return result, err
		}
		result.Fixed = fixed
		// Re-check: anything successfully fixed is no longer "new" -- it's
		// been wired into the catalog and the call site rewritten.
		if len(fixed) > 0 {
			fixedFP := make(map[string]bool, len(fixed))
			for _, v := range fixed {
				fixedFP[v.Fingerprint()] = true
			}
			kept := result.New[:0]
			for _, v := range result.New {
				if !fixedFP[v.Fingerprint()] {
					kept = append(kept, v)
				}
			}
			result.New = kept
		}
	}

	return result, nil
}

func collectAuditFiles(root string) ([]string, error) {
	var out []string
	for _, dir := range AuditScopeDirs {
		base := filepath.Join(root, dir)
		err := filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			for _, ex := range auditExcludeDirs {
				if rel == ex || strings.HasPrefix(rel, ex+"/") {
					return nil
				}
			}
			out = append(out, path)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(out)
	return out, nil
}

// suspectLiteral reports whether text looks like player-facing English
// prose worth flagging, rather than a short symbolic constant (a CSS class,
// a struct-tag-like key, an ALL_CAPS constant, an empty/placeholder string).
func suspectLiteral(text string) bool {
	if len(text) < 3 {
		return false
	}
	r := []rune(text)
	if r[0] < 'A' || r[0] > 'Z' {
		return false
	}
	hasLower := false
	for _, c := range r {
		if c >= 'a' && c <= 'z' {
			hasLower = true
			break
		}
	}
	return hasLower
}

// resolveStringIdent looks up a package-level (top-level) var or const named
// name in file and returns its value if it's a single, plain string literal
// -- covers the "value-fed-by-a-variable" gap shape the old regex missed
// (e.g. internal/features/choices.go's Label fed from a variable), without
// needing full type-checking or cross-file dataflow.
func resolveStringIdent(file *ast.File, name string) (string, bool) {
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || (gd.Tok != token.VAR && gd.Tok != token.CONST) {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || len(vs.Names) != 1 || len(vs.Values) != 1 {
				continue
			}
			if vs.Names[0].Name != name {
				continue
			}
			lit, ok := vs.Values[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			s, err := strconv.Unquote(lit.Value)
			if err != nil {
				continue
			}
			return s, true
		}
	}
	return "", false
}

// funcHasTrParam reports whether fn's parameter list has one literally
// named "tr" -- the project's own convention for an in-scope
// i18n.TranslateFunc (see cmd/n5e's handler signatures). -fix only rewrites
// a call site when this is true, so it never has to guess an unfamiliar
// translator-call shape.
func funcHasTrParam(fn *ast.FuncDecl) bool {
	if fn == nil || fn.Type.Params == nil {
		return false
	}
	for _, field := range fn.Type.Params.List {
		for _, n := range field.Names {
			if n.Name == "tr" {
				return true
			}
		}
	}
	return false
}

// enclosingFunc returns the FuncDecl whose body contains pos, if any.
func enclosingFunc(file *ast.File, pos token.Pos) *ast.FuncDecl {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		if pos >= fn.Body.Pos() && pos <= fn.Body.End() {
			return fn
		}
	}
	return nil
}

func findHardcodedLiterals(fset *token.FileSet, file *ast.File, path, root string) ([]AuditViolation, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(raw), "\n")
	rel, _ := filepath.Rel(root, path)
	rel = filepath.ToSlash(rel)

	var out []AuditViolation
	ast.Inspect(file, func(n ast.Node) bool {
		cl, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		for _, elt := range cl.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok || !auditSuspectFields[key.Name] {
				continue
			}

			var text string
			switch v := kv.Value.(type) {
			case *ast.BasicLit:
				if v.Kind != token.STRING {
					continue
				}
				s, err := strconv.Unquote(v.Value)
				if err != nil {
					continue
				}
				text = s
			case *ast.Ident:
				s, ok := resolveStringIdent(file, v.Name)
				if !ok {
					continue
				}
				text = s
			default:
				continue
			}

			if !suspectLiteral(text) {
				continue
			}

			pos := fset.Position(kv.Pos())
			if ignoredAtLine(lines, pos.Line) {
				continue
			}

			fn := enclosingFunc(file, kv.Pos())
			fixable := auditFixableFields[key.Name] && funcHasTrParam(fn)

			out = append(out, AuditViolation{
				Kind:    AuditHardcodedLiteral,
				File:    rel,
				Line:    pos.Line,
				Field:   key.Name,
				Text:    text,
				Fixable: fixable,
			})
		}
		return true
	})
	return out, nil
}

// ignoredAtLine reports whether the given 1-indexed line, or the line
// immediately before it, carries an "i18n:ignore" suppression comment.
func ignoredAtLine(lines []string, line int) bool {
	for _, l := range []int{line, line - 1} {
		if l < 1 || l > len(lines) {
			continue
		}
		if strings.Contains(lines[l-1], "i18n:ignore") {
			return true
		}
	}
	return false
}

var fromTableRe = func(table string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)from\s+"?` + regexp.QuoteMeta(table) + `"?\b`)
}

var overlayCallRe = func(table string) *regexp.Regexp {
	return regexp.MustCompile(`i18n\.(Row|Batch)\([^)]*"` + regexp.QuoteMeta(table) + `"`)
}

// findMissingOverlays flags, per file, any TranslatableFields table it
// queries (a SQL "FROM <table>") with no i18n.Row/i18n.Batch call anywhere
// in that same file naming that table. File-level granularity, not
// function-level: cheap and safe against false negatives for the gap shape
// actually seen so far (a whole handler/file never wired at all), but can't
// tell apart two functions in the same file where only one needs the
// overlay -- a real limitation, documented rather than hidden.
func findMissingOverlays(files []string, root string) ([]AuditViolation, error) {
	var out []AuditViolation
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		src := string(raw)
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		for table := range TranslatableFields {
			if !fromTableRe(table).MatchString(src) {
				continue
			}
			if overlayCallRe(table).MatchString(src) {
				continue
			}
			out = append(out, AuditViolation{
				Kind:  AuditMissingOverlay,
				File:  rel,
				Table: table,
				Text:  fmt.Sprintf("reads %q but never calls i18n.Row/i18n.Batch for it in this file", table),
			})
		}
	}
	return out, nil
}

// applyFixes rewrites the call site of every fixable hardcoded-literal
// violation to tr("<generated key>"), and registers the key/English text
// pair in internal/i18n/locales/en.json so the next catalog-translate run
// picks it up automatically. Files are rewritten by precise byte-offset
// replacement of just the literal token, preserving everything else
// (formatting, comments) untouched -- safer than a full AST print, which can
// reflow unrelated parts of a hand-formatted file.
func applyFixes(fset *token.FileSet, parsed map[string]*ast.File, root string, violations []AuditViolation) ([]AuditViolation, error) {
	type editSpec struct {
		key       string
		startLine int
	}
	byFile := map[string][]AuditViolation{}
	for _, v := range violations {
		if v.Kind == AuditHardcodedLiteral && v.Fixable {
			byFile[v.File] = append(byFile[v.File], v)
		}
	}
	if len(byFile) == 0 {
		return nil, nil
	}

	catalogPath := filepath.Join(root, "internal/i18n/locales/en.json")
	catalog, err := readCatalogFile(catalogPath)
	if err != nil {
		return nil, err
	}

	var fixed []AuditViolation
	for rel, vs := range byFile {
		absPath := filepath.Join(root, rel)
		af := parsed[absPath]
		if af == nil {
			continue
		}
		raw, err := os.ReadFile(absPath)
		if err != nil {
			return nil, err
		}
		src := string(raw)

		// Find, for each violation, the exact BasicLit/Ident node again (we
		// don't keep node references across the RunAudit call boundary) by
		// re-walking and matching on (line, field name, text).
		type edit struct {
			start, end int
			key        string
		}
		var edits []edit
		want := map[string]AuditViolation{}
		for _, v := range vs {
			want[fmt.Sprintf("%d|%s", v.Line, v.Field)] = v
		}
		_ = editSpec{}

		ast.Inspect(af, func(n ast.Node) bool {
			cl, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			for _, elt := range cl.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := kv.Key.(*ast.Ident)
				if !ok {
					continue
				}
				lit, ok := kv.Value.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue // only rewrite direct literals; ident-fed values are reported only
				}
				pos := fset.Position(kv.Pos())
				v, ok := want[fmt.Sprintf("%d|%s", pos.Line, key.Name)]
				if !ok {
					continue
				}
				genKey := generateCatalogKey(rel, key.Name, v.Text)
				startOff := fset.Position(lit.Pos()).Offset
				endOff := fset.Position(lit.End()).Offset
				edits = append(edits, edit{start: startOff, end: endOff, key: genKey})
				v.Key = genKey
				catalog[genKey] = v.Text
				fixed = append(fixed, v)
			}
			return true
		})

		if len(edits) == 0 {
			continue
		}
		sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
		for _, e := range edits {
			replacement := `tr("` + e.key + `")`
			src = src[:e.start] + replacement + src[e.end:]
		}
		if err := os.WriteFile(absPath, []byte(src), 0o644); err != nil {
			return nil, err
		}
	}

	if len(fixed) > 0 {
		if err := writeCatalogFile(catalogPath, catalog); err != nil {
			return nil, err
		}
	}
	return fixed, nil
}

// generateCatalogKey builds a stable, human-scannable UI catalog key for an
// auto-fixed literal: "audit.<file-stem>.<field-lower>.<hash8>". The hash
// covers file+field+text so re-running audit -fix on an unrelated new
// violation never collides with an existing key, and so the same literal
// fixed twice (e.g. after a revert) gets the same key back.
func generateCatalogKey(relFile, field, text string) string {
	stem := strings.TrimSuffix(filepath.Base(relFile), ".go")
	stem = sanitizeKeySegment(stem)
	sum := sha256.Sum256([]byte(relFile + "|" + field + "|" + text))
	hash := hex.EncodeToString(sum[:])[:10]
	return "audit." + stem + "." + strings.ToLower(field) + "." + hash
}

func sanitizeKeySegment(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

func readCatalogFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var cat map[string]string
	if err := json.Unmarshal(data, &cat); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if cat == nil {
		cat = map[string]string{}
	}
	return cat, nil
}

func writeCatalogFile(path string, cat map[string]string) error {
	keys := make([]string, 0, len(cat))
	for k := range cat {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("{\n")
	for i, k := range keys {
		kb, _ := json.Marshal(k)
		vb, _ := json.Marshal(cat[k])
		b.WriteString("  ")
		b.Write(kb)
		b.WriteString(": ")
		b.Write(vb)
		if i < len(keys)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("}\n")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// LoadAuditBaseline reads internal/i18n/audit_baseline.json (a JSON array of
// AuditViolation.Fingerprint() strings) under root, returning a set for
// RunAudit to filter against. A missing file is treated as an empty
// baseline, not an error, so a fresh checkout without the file yet still
// runs (it will just report every current finding as new).
func LoadAuditBaseline(root string) (map[string]bool, error) {
	path := filepath.Join(root, "internal/i18n/audit_baseline.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	var list []string
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	out := make(map[string]bool, len(list))
	for _, fp := range list {
		out[fp] = true
	}
	return out, nil
}

// WriteAuditBaseline sweeps root the same way RunAudit does (ignoring any
// existing baseline) and writes every current finding's fingerprint to
// internal/i18n/audit_baseline.json -- used once to bootstrap the gate
// against the repo's state at the time this tool was introduced (via
// `n5e-i18n audit -seed-baseline`), and again deliberately whenever a batch
// of findings has been reviewed and accepted rather than fixed.
func WriteAuditBaseline(root string) (int, error) {
	files, err := collectAuditFiles(root)
	if err != nil {
		return 0, err
	}
	fset := token.NewFileSet()
	var all []AuditViolation
	for _, f := range files {
		af, err := parser.ParseFile(fset, f, nil, parser.ParseComments)
		if err != nil {
			return 0, fmt.Errorf("parsing %s: %w", f, err)
		}
		lits, err := findHardcodedLiterals(fset, af, f, root)
		if err != nil {
			return 0, err
		}
		all = append(all, lits...)
	}
	overlay, err := findMissingOverlays(files, root)
	if err != nil {
		return 0, err
	}
	all = append(all, overlay...)

	seen := map[string]bool{}
	var fps []string
	for _, v := range all {
		fp := v.Fingerprint()
		if seen[fp] {
			continue
		}
		seen[fp] = true
		fps = append(fps, fp)
	}
	sort.Strings(fps)

	data, err := json.MarshalIndent(fps, "", "  ")
	if err != nil {
		return 0, err
	}
	data = append(data, '\n')
	path := filepath.Join(root, "internal/i18n/audit_baseline.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return 0, err
	}
	return len(fps), nil
}
