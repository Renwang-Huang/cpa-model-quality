package main

import (
	"encoding/json"
	"testing"
)

func completionEvent(status string) []byte {
	b, _ := json.Marshal(map[string]any{"type": "response." + status, "response": map[string]any{"status": status, "model": "synthetic-model", "output": []any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": "中文答复"}}}}}})
	return append(append([]byte("data: "), b...), []byte("\r\n\r\n")...)
}
func TestSSEAcrossByteBoundaries(t *testing.T) {
	raw := completionEvent("completed")
	for size := 1; size < 16; size++ {
		s := &streamParser{}
		for i := 0; i < len(raw); i += size {
			end := i + size
			if end > len(raw) {
				end = len(raw)
			}
			if err := s.feed(raw[i:end]); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.finish(); err != nil || s.text != "中文答复" {
			t.Fatalf("size %d: %v %s", size, err, s.text)
		}
	}
}
func TestMissingOrIncompleteCompletionIsNotSuccess(t *testing.T) {
	for _, raw := range [][]byte{completionEvent("incomplete"), []byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"valid looking answer\"}\n\ndata: [DONE]\n\n"), append(completionEvent("completed"), []byte("data: {\"type\":\"error\",\"error\":{\"message\":\"fail\"}}\n\n")...)} {
		s := &streamParser{}
		if err := s.feed(raw); err != nil {
			t.Fatal(err)
		}
		if err := s.finish(); err == nil {
			t.Fatal("incomplete stream accepted")
		}
	}
}
func TestRawTextAndHTMLExtraction(t *testing.T) {
	if extractHTML("not HTML") != "" {
		t.Fatal("plain text rendered as HTML")
	}
	if extractHTML("```html\n<html><body>x</body></html>\n```") != "<html><body>x</body></html>" {
		t.Fatal("fenced HTML not extracted")
	}
}

func TestNativeCodexCallbackRecordsWithoutSeparators(t *testing.T) {
	s := &streamParser{}
	for _, record := range [][]byte{
		[]byte("event: response.created"),
		[]byte(`data: {"type":"response.created","response":{"model":"native-model"}}`),
		{},
		[]byte("event: response.output_text.delta"),
		[]byte(`data: {"type":"response.output_text.delta","delta":"first text"}`),
		{},
		[]byte("event: response.completed"),
		[]byte(`data: {"type":"response.completed","response":{"status":"completed","model":"native-model","output":[{"type":"message","content":[{"type":"output_text","text":"first text"}]}]}}`),
	} {
		if err := s.feed(record); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.finish(); err != nil || s.text != "first text" || s.model != "native-model" {
		t.Fatalf("native callback lost: text=%q model=%q err=%v", s.text, s.model, err)
	}
}
