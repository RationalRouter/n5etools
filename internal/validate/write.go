package validate

import (
	"fmt"
	"io"
)

// Write renders the report as the plain, human-readable text a maintainer
// reads top to bottom — same content whether it lands on stdout or in a
// saved file.
func (r *Report) Write(w io.Writer) error {
	if _, err := fmt.Fprintf(w, "needs_review: %d rows across %d tables\n",
		r.TotalNeedsReview, len(r.NeedsReview)); err != nil {
		return err
	}
	for _, tc := range r.NeedsReview {
		if _, err := fmt.Fprintf(w, "  %-28s %4d\n", tc.Table, tc.Count); err != nil {
			return err
		}
	}

	if r.CrossCheck == nil {
		_, err := fmt.Fprintln(w, "\n(no Sheet1 cross-check run — pass -sheet to include one)")
		return err
	}

	if _, err := fmt.Fprintln(w, "\nSheet1 cross-check (approximate — see internal/validate for known noise sources):"); err != nil {
		return err
	}
	for _, cc := range r.CrossCheck {
		if _, err := fmt.Fprintf(w, "\n  %s: %d in sheet, %d in DB\n", cc.Category, cc.SheetCount, cc.DBCount); err != nil {
			return err
		}
		if len(cc.InDBNotSheet) > 0 {
			if _, err := fmt.Fprintf(w, "    in DB, not in sheet (%d%s):\n",
				len(cc.InDBNotSheet), truncSuffix(cc.TruncatedDB)); err != nil {
				return err
			}
			for _, n := range cc.InDBNotSheet {
				if _, err := fmt.Fprintf(w, "      %s\n", n); err != nil {
					return err
				}
			}
		}
		if len(cc.InSheetNotDB) > 0 {
			if _, err := fmt.Fprintf(w, "    in sheet, not in DB (%d%s):\n",
				len(cc.InSheetNotDB), truncSuffix(cc.TruncatedSheet)); err != nil {
				return err
			}
			for _, n := range cc.InSheetNotDB {
				if _, err := fmt.Fprintf(w, "      %s\n", n); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func truncSuffix(truncated bool) string {
	if truncated {
		return "+, sample capped"
	}
	return ""
}
