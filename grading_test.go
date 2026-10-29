package main

import (
	"encoding/json"
	"testing"
)

func TestStrictJSONGrading(t *testing.T) {
	c := sealCase(Case{Kind: "json", Expected: map[string]any{"x": 9007199254740993, "a": []any{1, "b"}}})
	for _, tt := range []struct {
		reply  string
		format string
		answer string
		score  float64
	}{
		{`{"x":9007199254740993,"a":[1,"b"]}`, "passed", "passed", 100},
		{`{"x":9007199254740992,"a":[1,"b"]}`, "passed", "partial", 50},
		{`{"x":"9007199254740993","a":[1,"b"]}`, "passed", "partial", 50},
		{`{"x":9007199254740993,"a":[1,"b"],"extra":true}`, "failed", "passed", 100},
	} {
		g := grade(c, tt.reply)
		if g.Format != tt.format || g.Answer != tt.answer || g.Score == nil || *g.Score != tt.score {
			t.Fatalf("%s: %+v", tt.reply, g)
		}
	}
	for _, reply := range []string{`{"x":1,"x":9007199254740993,"a":[1,"b"]}`, `{"x":9007199254740993,"a":{"b":1,"b":2}}`, `{"x":1} trailing`, "```json\n{}\n```", `[]`, ``} {
		g := grade(c, reply)
		if g.Score != nil || g.Answer != "unavailable" || g.Format != "failed" {
			t.Fatalf("ambiguous answer scored: %s %+v", reply, g)
		}
	}
}
func TestCatalogReferencesAndPromptHashes(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range catalog() {
		if seen[c.ID] || len(c.Hash) != 64 || c.Prompt == "" || c.Reference == "" {
			t.Fatalf("invalid case: %s", c.ID)
		}
		seen[c.ID] = true
		if c.Kind == "json" {
			b, _ := json.Marshal(c.Expected)
			g := grade(c, string(b))
			if g.Score == nil || *g.Score != 100 || g.Format != "passed" {
				t.Fatalf("reference cannot pass: %s %+v", c.ID, g)
			}
		}
	}
}
func TestProbabilityReferenceByEnumeration(t *testing.T) {
	total, valid := 0, 0
	for a := 0; a < 12; a++ {
		for b := a + 1; b < 12; b++ {
			for c := b + 1; c < 12; c++ {
				for d := c + 1; d < 12; d++ {
					total++
					red, blue, green := 0, 0, 0
					for _, n := range []int{a, b, c, d} {
						switch {
						case n < 5:
							red++
						case n < 9:
							blue++
						default:
							green++
						}
					}
					if red > blue && blue > 0 && green > 0 {
						valid++
					}
				}
			}
		}
	}
	if total != 495 || valid != 120 {
		t.Fatalf("reference mismatch %d/%d", valid, total)
	}
}

func TestPublicChoiceRequiresUniqueTerminalAnswer(t *testing.T) {
	c := Case{Kind: "choice", Expected: map[string]any{"answer": "C"}}
	for _, reply := range []string{"C", "The answer C appears in my reasoning", "FINAL: C\nFINAL: B", "FINAL: C\nmore text"} {
		if g := grade(c, reply); g.Score != nil {
			t.Fatalf("ambiguous choice scored: %q", reply)
		}
	}
	if g := grade(c, "Reasoning\nFINAL: C"); g.Score == nil || *g.Score != 100 {
		t.Fatal("valid final choice rejected")
	}
	if g := grade(c, "FINAL: B"); g.Score == nil || *g.Score != 0 {
		t.Fatal("wrong choice was not a scored failure")
	}
}
