package ocr

import (
	"strings"
	"testing"
)

// A tiny fixture shaped like real tesseract --psm 6 tsv output: a header
// row (level 1-4 rows for page/block/par/line, which ParseTSV must skip)
// followed by word-level (level 5) rows for a 2-column, 2-row mini table
// ("Armor" / "Cost" header, "Padded Cloth" / "100 Ryo" data row).
const fixtureTSV = "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
	"1\t1\t0\t0\t0\t0\t0\t0\t800\t400\t-1\t\n" +
	"2\t1\t1\t0\t0\t0\t10\t10\t400\t200\t-1\t\n" +
	"4\t1\t1\t1\t1\t0\t10\t10\t400\t20\t-1\t\n" +
	"5\t1\t1\t1\t1\t1\t10\t10\t60\t20\t95.5\tArmor\n" +
	"5\t1\t1\t1\t1\t2\t210\t12\t40\t18\t93.2\tCost\n" +
	"5\t1\t1\t1\t1\t3\t10\t50\t70\t18\t88.1\tPadded\n" +
	"5\t1\t1\t1\t1\t4\t85\t51\t50\t18\t90.0\tCloth\n" +
	"5\t1\t1\t1\t1\t5\t210\t50\t40\t18\t91.4\t100\n" +
	"5\t1\t1\t1\t1\t6\t255\t50\t30\t18\t89.9\tRyo\n" +
	"5\t1\t1\t1\t1\t7\t400\t50\t20\t18\t-1\t \n" // blank text: must be skipped

func TestParseTSV(t *testing.T) {
	words, err := ParseTSV(strings.NewReader(fixtureTSV))
	if err != nil {
		t.Fatalf("ParseTSV: %v", err)
	}
	// 6 real words: Armor, Cost, Padded, Cloth, 100, Ryo — the level
	// 1/2/4 layout rows and the blank-text level-5 row must all be
	// dropped.
	if len(words) != 6 {
		t.Fatalf("got %d words, want 6: %+v", len(words), words)
	}
	if words[0].Text != "Armor" || words[0].Left != 10 || words[0].Top != 10 {
		t.Errorf("word[0] = %+v", words[0])
	}
	if words[0].Conf != 95.5 {
		t.Errorf("word[0].Conf = %v, want 95.5", words[0].Conf)
	}
}

func TestGroupRows(t *testing.T) {
	words, err := ParseTSV(strings.NewReader(fixtureTSV))
	if err != nil {
		t.Fatalf("ParseTSV: %v", err)
	}
	rows := GroupRows(words, 10)
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	// Header row ("Armor", "Cost") sorted left-to-right.
	if len(rows[0]) != 2 || rows[0][0].Text != "Armor" || rows[0][1].Text != "Cost" {
		t.Errorf("row 0 = %+v", rows[0])
	}
	// Data row ("Padded", "Cloth", "100", "Ryo") sorted left-to-right.
	if len(rows[1]) != 4 || rows[1][0].Text != "Padded" || rows[1][3].Text != "Ryo" {
		t.Errorf("row 1 = %+v", rows[1])
	}
}

func TestExtractTable(t *testing.T) {
	words, err := ParseTSV(strings.NewReader(fixtureTSV))
	if err != nil {
		t.Fatalf("ParseTSV: %v", err)
	}
	rows := GroupRows(words, 10)
	columns := []Column{
		{Name: "Armor", StartX: 0},
		{Name: "Cost", StartX: 200},
	}
	table := ExtractTable(rows, columns)
	if len(table) != 2 {
		t.Fatalf("got %d rows, want 2", len(table))
	}
	if table[0][0] != "Armor" || table[0][1] != "Cost" {
		t.Errorf("header row = %+v", table[0])
	}
	if table[1][0] != "Padded Cloth" || table[1][1] != "100 Ryo" {
		t.Errorf("data row = %+v", table[1])
	}
}

func TestExtractTableWordBeforeFirstColumn(t *testing.T) {
	// A word to the left of every declared column's StartX must still
	// land somewhere (column 0) rather than being silently dropped.
	rows := [][]Word{{{Left: -5, Top: 0, Width: 10, Height: 10, Text: "Stray"}}}
	columns := []Column{{Name: "Only", StartX: 0}}
	table := ExtractTable(rows, columns)
	if table[0][0] != "Stray" {
		t.Errorf("got %+v, want [Stray]", table[0])
	}
}
