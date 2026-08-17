// Core book Chapter 3: backgrounds.
//
// Template per background:
//
//	<NAME>                        caps heading
//	  flavor prose
//	Skill Proficiencies: …        labels; values wrap across lines
//	Tool Proficiencies: …
//	Equipment: …
//	Equipment Pack: …
//	FEATURE: <FEATURE NAME>       the background feature
//	  feature prose
//	<ASI TITLE>                   e.g. "ENTICING PERSONALITY"
//	  "Select one:" + ASI/feat bullets with (Recommended: …) lines
//
// A caps heading is a background start iff a "Skill Proficiencies:" line
// appears before the next caps heading — the chapter's explainer headings
// (PROFICIENCIES, LANGUAGES AND DIALECTS, …) never have one.
package parse

import "strings"

// Background is one Chapter 3 background.
type Background struct {
	Name          string
	Description   string
	SkillProfs    string // raw value; "Choose two from …" kept verbatim
	ToolProfs     string
	Equipment     string
	EquipmentPack string
	FeatureName   string
	FeatureText   string
	ASIText       string // the "Select one:" block, verbatim
	SourcePage    int
}

// The singular forms are book typos (Noble prints "Tool Proficiency:").
var backgroundLabels = []string{
	"Skill Proficiencies:", "Skill Proficiency:",
	"Tool Proficiencies:", "Tool Proficiency:",
	"Equipment Pack:", "Equipment:",
}

// startsBackground reports whether the caps line at i opens a background
// (a Skill Proficiencies label follows before any other caps line).
func startsBackground(ls []Line, i int) bool {
	if !capsLineRe.MatchString(ls[i].Text) {
		return false
	}
	for j := i + 1; j < len(ls); j++ {
		if capsLineRe.MatchString(ls[j].Text) {
			return false
		}
		if strings.HasPrefix(ls[j].Text, "Skill Proficienc") {
			return true
		}
	}
	return false
}

// ParseBackgrounds scans the core-book line stream for Chapter 3's
// backgrounds (they end where Chapter 4 begins).
func ParseBackgrounds(lines []Line) ([]Background, []Anomaly) {
	var (
		backgrounds []Background
		anomalies   []Anomaly
	)

	var ls []Line
	for _, ln := range lines {
		if pageNumberRe.MatchString(ln.Text) || punctOnlyRe.MatchString(ln.Text) {
			continue
		}
		ls = append(ls, ln)
	}

	const (
		modeFlavor = iota
		modeLabels
		modeFeature
		modeASI
	)
	var cur *Background
	var desc, featureText, asiText []string
	label, labelValue := "", ""

	setLabel := func() {
		if label == "" {
			return
		}
		v := strings.TrimSpace(labelValue)
		switch label {
		case "Skill Proficiencies:", "Skill Proficiency:":
			cur.SkillProfs = v
		case "Tool Proficiencies:", "Tool Proficiency:":
			cur.ToolProfs = v
		case "Equipment:":
			cur.Equipment = v
		case "Equipment Pack:":
			cur.EquipmentPack = v
		}
		label, labelValue = "", ""
	}
	flush := func() {
		if cur == nil {
			return
		}
		setLabel()
		cur.Description = strings.TrimSpace(strings.Join(desc, " "))
		cur.FeatureText = strings.TrimSpace(strings.Join(featureText, " "))
		cur.ASIText = strings.TrimSpace(strings.Join(asiText, " "))
		for _, missing := range []struct{ what, val string }{
			{"skill proficiencies", cur.SkillProfs},
			{"feature", cur.FeatureText},
			{"ASI block", cur.ASIText},
		} {
			if missing.val == "" {
				anomalies = append(anomalies, Anomaly{Page: cur.SourcePage, Subject: cur.Name,
					Problem: "background missing " + missing.what})
			}
		}
		backgrounds = append(backgrounds, *cur)
		cur = nil
		desc, featureText, asiText = nil, nil, nil
	}

	mode := modeFlavor
	for i := 0; i < len(ls); i++ {
		text := ls[i].Text

		// "CHAPTER 4" headings appear early too (TOC, the What's Different
		// chapter list) — only the one AFTER the backgrounds ends the
		// section. startsBackground anchors each entry structurally, so no
		// section-start marker is needed at all.
		if strings.HasPrefix(text, "CHAPTER 4") && (len(backgrounds) > 0 || cur != nil) {
			break
		}

		if startsBackground(ls, i) {
			flush()
			cur = &Background{Name: tidyName(text), SourcePage: ls[i].Page}
			mode = modeFlavor
			continue
		}
		if cur == nil {
			continue // chapter intro / explainer sections
		}

		if rest, ok := strings.CutPrefix(text, "FEATURE:"); ok {
			setLabel()
			cur.FeatureName = tidyName(strings.TrimSpace(rest))
			mode = modeFeature
			continue
		}
		if capsLineRe.MatchString(text) {
			// Inside a background, a caps line after the feature is the ASI
			// block title ("ENTICING PERSONALITY"); its name is flavor, the
			// content is what matters.
			if mode == modeFeature {
				mode = modeASI
				continue
			}
			// A caps line anywhere else (mid-flavor art caption etc.) is
			// noise — flag it, keep going.
			anomalies = append(anomalies, Anomaly{Page: ls[i].Page, Subject: cur.Name,
				Problem: "unexpected heading inside background: " + snippet(text)})
			continue
		}

		isLabel := false
		for _, l := range backgroundLabels {
			if strings.HasPrefix(text, l) {
				setLabel()
				label, labelValue = l, strings.TrimPrefix(text, l)
				mode = modeLabels
				isLabel = true
				break
			}
		}
		if isLabel {
			continue
		}

		switch mode {
		case modeFlavor:
			desc = append(desc, text)
		case modeLabels:
			labelValue += " " + text // wrapped label value
		case modeFeature:
			featureText = append(featureText, text)
		case modeASI:
			asiText = append(asiText, text)
		}
	}
	flush()

	if len(backgrounds) == 0 {
		anomalies = append(anomalies, Anomaly{Subject: "Backgrounds",
			Problem: "no backgrounds found"})
	}
	return backgrounds, anomalies
}
