package store

import "database/sql"

// GetTranslationBundleState reads the last-applied translation bundle's
// version/ETag for one locale, mirroring GetSourceBookLastModified's
// found/not-found shape.
func GetTranslationBundleState(db *sql.DB, locale string) (ingestSchemaVersion, etag string, found bool, err error) {
	err = db.QueryRow(
		`SELECT ingest_schema_version, etag FROM translation_bundles WHERE locale = ?`,
		locale,
	).Scan(&ingestSchemaVersion, &etag)
	if err == sql.ErrNoRows {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return ingestSchemaVersion, etag, true, nil
}

// SetTranslationBundleState records that locale is now current as of the
// given ingest schema version and asset ETag.
func SetTranslationBundleState(db *sql.DB, locale, ingestSchemaVersion, etag string) error {
	_, err := db.Exec(`
		INSERT INTO translation_bundles (locale, ingest_schema_version, etag, applied_at)
		VALUES (?, ?, ?, datetime('now'))
		ON CONFLICT (locale) DO UPDATE SET
			ingest_schema_version = excluded.ingest_schema_version,
			etag = excluded.etag,
			applied_at = excluded.applied_at`,
		locale, ingestSchemaVersion, etag)
	return err
}
