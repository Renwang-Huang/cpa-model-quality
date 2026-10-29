package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Check struct {
	Field    string `json:"field"`
	Expected any    `json:"expected"`
	Actual   any    `json:"actual"`
	Passed   bool   `json:"passed"`
}
type Grade struct {
	Version string   `json:"version"`
	Format  string   `json:"format"`
	Answer  string   `json:"answer"`
	Score   *float64 `json:"score"`
	Passed  int      `json:"passed"`
	Total   int      `json:"total"`
	Checks  []Check  `json:"checks"`
	Reason  string   `json:"reason,omitempty"`
}

// Read tokens recursively so duplicate keys at any depth cannot win by overwrite.
func strictJSON(raw string) (any, error) {
	d := json.NewDecoder(strings.NewReader(raw))
	d.UseNumber()
	var read func(int) (any, error)
	read = func(depth int) (any, error) {
		if depth > 64 {
			return nil, fmt.Errorf("JSON nesting too deep")
		}
		t, err := d.Token()
		if err != nil {
			return nil, err
		}
		if delim, ok := t.(json.Delim); ok {
			switch delim {
			case '{':
				out := map[string]any{}
				for d.More() {
					key, err := d.Token()
					if err != nil {
						return nil, err
					}
					k, ok := key.(string)
					if !ok {
						return nil, fmt.Errorf("invalid object key")
					}
					if _, ok := out[k]; ok {
						return nil, fmt.Errorf("duplicate JSON key")
					}
					v, err := read(depth + 1)
					if err != nil {
						return nil, err
					}
					out[k] = v
				}
				_, err = d.Token()
				return out, err
			case '[':
				out := []any{}
				for d.More() {
					v, err := read(depth + 1)
					if err != nil {
						return nil, err
					}
					out = append(out, v)
				}
				_, err = d.Token()
				return out, err
			default:
				return nil, fmt.Errorf("unexpected JSON delimiter")
			}
		}
		return t, nil
	}
	v, err := read(0)
	if err != nil {
		return nil, err
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, fmt.Errorf("trailing JSON content")
	}
	return v, nil
}

func equalJSON(a, b any) bool {
	if na, ok := a.(json.Number); ok {
		var nb string
		switch n := b.(type) {
		case json.Number:
			nb = string(n)
		case float64:
			nb = fmt.Sprint(n)
		case int:
			nb = fmt.Sprint(n)
		case int64:
			nb = fmt.Sprint(n)
		default:
			return false
		}
		if !boundedNumber(string(na)) || !boundedNumber(nb) {
			return false
		}
		x, okx := new(big.Rat).SetString(string(na))
		y, oky := new(big.Rat).SetString(nb)
		return okx && oky && x.Cmp(y) == 0
	}
	if aa, ok := a.([]any); ok {
		bb, ok := b.([]any)
		if !ok || len(aa) != len(bb) {
			return false
		}
		for i := range aa {
			if !equalJSON(aa[i], bb[i]) {
				return false
			}
		}
		return true
	}
	if aa, ok := a.(map[string]any); ok {
		bb, ok := b.(map[string]any)
		if !ok || len(aa) != len(bb) {
			return false
		}
		for k, v := range aa {
			w, ok := bb[k]
			if !ok || !equalJSON(v, w) {
				return false
			}
		}
		return true
	}
	return reflect.DeepEqual(a, b)
}

func boundedNumber(s string) bool {
	if len(s) > 256 {
		return false
	}
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		n, err := strconv.Atoi(s[i+1:])
		if err != nil || n > 1000 || n < -1000 {
			return false
		}
	}
	return true
}

func grade(c Case, reply string) *Grade {
	if c.Kind == "choice" {
		return gradeChoice(c, reply)
	}
	if c.Kind == "format-json" {
		s := strings.TrimSpace(reply)
		if strings.HasPrefix(s, "```") {
			if i := strings.IndexByte(s, '\n'); i >= 0 && strings.HasSuffix(s, "```") {
				s = strings.TrimSpace(s[i+1 : len(s)-3])
			}
		}
		g := &Grade{Version: graderVersion, Format: "failed", Answer: "unavailable", Checks: []Check{}, Reason: "仅检查JSON格式，不判断内容质量"}
		if len(reply) <= maxReplyBytes {
			if _, err := strictJSON(s); err == nil {
				g.Format = "passed"
			}
		}
		return g
	}
	if c.Kind != "json" {
		return nil
	}
	g := &Grade{Version: graderVersion, Format: "failed", Answer: "unavailable", Total: len(c.Expected), Checks: []Check{}}
	if len(reply) > maxReplyBytes {
		g.Reason = "回复过长，未评分"
		return g
	}
	v, err := strictJSON(strings.TrimSpace(reply))
	if err != nil {
		g.Reason = "未返回唯一且无重复键的严格 JSON 对象"
		return g
	}
	obj, ok := v.(map[string]any)
	if !ok {
		g.Reason = "需要 JSON 对象"
		return g
	}
	format := len(obj) == len(c.Expected)
	keys := make([]string, 0, len(c.Expected))
	for k := range c.Expected {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		actual, present := obj[k]
		if !present {
			format = false
		}
		b, _ := json.Marshal(c.Expected[k])
		expected, _ := strictJSON(string(b))
		passed := present && equalJSON(actual, expected)
		g.Checks = append(g.Checks, Check{Field: k, Expected: c.Expected[k], Actual: actual, Passed: passed})
		if passed {
			g.Passed++
		}
	}
	if format {
		g.Format = "passed"
	}
	g.Answer = "failed"
	if g.Passed == g.Total {
		g.Answer = "passed"
	} else if g.Passed > 0 {
		g.Answer = "partial"
	}
	score := float64(g.Passed) * 100 / float64(g.Total)
	g.Score = &score
	return g
}

var finalChoice = regexp.MustCompile(`(?im)^FINAL:[ \t]*([A-F])[ \t]*\r?$`)

func gradeChoice(c Case, reply string) *Grade {
	g := &Grade{Version: graderVersion, Format: "failed", Answer: "unavailable", Total: 1, Checks: []Check{}}
	matches := finalChoice.FindAllStringSubmatchIndex(reply, -1)
	if len(reply) > maxReplyBytes || len(matches) != 1 || strings.TrimSpace(reply[matches[0][1]:]) != "" {
		g.Reason = "缺少唯一的末行 FINAL: X，未评分"
		return g
	}
	actual := strings.ToUpper(reply[matches[0][2]:matches[0][3]])
	expected, _ := c.Expected["answer"].(string)
	passed := actual == expected
	g.Format = "passed"
	g.Answer = "failed"
	score := float64(0)
	if passed {
		g.Answer = "passed"
		g.Passed = 1
		score = 100
	}
	g.Score = &score
	g.Checks = []Check{{Field: "answer", Expected: expected, Actual: actual, Passed: passed}}
	return g
}

func jsonClone[T any](v T) T {
	b, _ := json.Marshal(v)
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var out T
	_ = d.Decode(&out)
	return out
}
