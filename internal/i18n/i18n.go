package i18n

import "database/sql"

// Row fetches every translated field for one slug in one content table, at
// the given locale, keyed by field name. Returns an empty (non-nil) map,
// not an error, when locale is English or nothing has been translated yet
// for that slug -- callers overlay unconditionally without a locale != "en"
// branch, and a missing field simply leaves the caller's existing English
// value in place.
func Row(db *sql.DB, locale, table, slug string) (map[string]string, error) {
	out := map[string]string{}
	if locale == "" || locale == "en" {
		return out, nil
	}
	rows, err := db.Query(
		`SELECT field, value FROM content_translations WHERE table_name = ? AND slug = ? AND locale = ?`,
		table, slug, locale,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var field, value string
		if err := rows.Scan(&field, &value); err != nil {
			return nil, err
		}
		out[field] = value
	}
	return out, rows.Err()
}

// Batch fetches translations for many slugs in one table at once, avoiding
// N+1 queries in list/browse handlers that loop over rows. Returns a map
// keyed by slug, each value being the same field->translation map Row
// would return for that slug (absent entirely if nothing is translated for
// it).
func Batch(db *sql.DB, locale, table string, slugs []string) (map[string]map[string]string, error) {
	out := map[string]map[string]string{}
	if locale == "" || locale == "en" || len(slugs) == 0 {
		return out, nil
	}

	placeholders := make([]byte, 0, len(slugs)*2)
	args := make([]any, 0, len(slugs)+2)
	args = append(args, table, locale)
	for i, slug := range slugs {
		if i > 0 {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, '?')
		args = append(args, slug)
	}

	query := `SELECT slug, field, value FROM content_translations WHERE table_name = ? AND locale = ? AND slug IN (` + string(placeholders) + `)`
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var slug, field, value string
		if err := rows.Scan(&slug, &field, &value); err != nil {
			return nil, err
		}
		if out[slug] == nil {
			out[slug] = map[string]string{}
		}
		out[slug][field] = value
	}
	return out, rows.Err()
}
