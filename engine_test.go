package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeStream struct {
	payload []byte
	read    bool
	closed  chan struct{}
	once    sync.Once
}
type fakeHost struct {
	mu         sync.Mutex
	streams    map[string]*fakeStream
	requests   []map[string]any
	block      bool
	incomplete bool
	started    chan struct{}
}

func newFakeHost() *fakeHost {
	return &fakeHost{streams: map[string]*fakeStream{}, started: make(chan struct{}, 20)}
}
func (h *fakeHost) Call(method string, input, output any) error {
	b, _ := json.Marshal(input)
	var in map[string]any
	_ = json.Unmarshal(b, &in)
	var result any
	switch method {
	case "host.auth.list":
		result = map[string]any{"files": []Account{{ID: "auth-one", Email: "one@example.test", Note: "工作账号", Provider: "codex"}, {ID: "auth-two", Email: "two@example.test", Provider: "codex"}, {ID: "disabled", Provider: "codex", Disabled: true}}}
	case "host.auth.models":
		result = map[string]any{"credentials": []any{map[string]any{"id": "auth-one", "models": []string{"test-model"}}, map[string]any{"id": "auth-two", "models": []string{"test-model"}}, map[string]any{"id": "disabled", "models": []string{"test-model"}}}}
	case "host.model.execute_stream":
		h.mu.Lock()
		h.requests = append(h.requests, in)
		id := fmt.Sprintf("stream-%d", len(h.requests))
		reply := `{"valid":120,"total":495,"probability":"8/33"}`
		event := map[string]any{"type": "response.completed", "response": map[string]any{"id": "synthetic-response", "model": "test-reported-model", "status": "completed", "output": []any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": reply}}}}, "usage": map[string]any{"input_tokens": 90, "output_tokens": 20}}}
		if h.incomplete {
			event["type"] = "response.incomplete"
		}
		payload, _ := json.Marshal(event)
		h.streams[id] = &fakeStream{payload: append(append([]byte("data: "), payload...), []byte("\n\n")...), closed: make(chan struct{})}
		h.mu.Unlock()
		h.started <- struct{}{}
		result = map[string]any{"status_code": 200, "stream_id": id}
	case "host.model.stream_read":
		id := in["stream_id"].(string)
		h.mu.Lock()
		s := h.streams[id]
		block := h.block
		if s == nil {
			h.mu.Unlock()
			return fmt.Errorf("stream absent")
		}
		if s.read {
			h.mu.Unlock()
			result = map[string]any{"done": true}
			break
		}
		s.read = true
		h.mu.Unlock()
		if block {
			<-s.closed
			return fmt.Errorf("stream cancelled")
		}
		result = map[string]any{"payload": s.payload, "done": true}
	case "host.model.stream_close":
		h.mu.Lock()
		s := h.streams[in["stream_id"].(string)]
		h.mu.Unlock()
		if s != nil {
			s.once.Do(func() { close(s.closed) })
		}
		result = map[string]any{}
	default:
		return fmt.Errorf("unexpected host call: %s", method)
	}
	if output != nil {
		b, _ := json.Marshal(result)
		return json.Unmarshal(b, output)
	}
	return nil
}
func testEngine(t *testing.T, h hostCaller) *Engine {
	t.Helper()
	c := defaultConfig()
	c.DataDir = t.TempDir()
	e, err := newEngine(h, c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.close)
	return e
}
func request() StartRequest {
	return StartRequest{Targets: []Target{{"auth-one", "test-model"}, {"auth-two", "test-model"}}, CaseIDs: []string{"probability"}, Effort: "high", Concurrency: 2, Repeats: 2}
}
func awaitDone(t *testing.T, e *Engine) {
	t.Helper()
	done := make(chan struct{})
	go func() { e.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not finish")
	}
}

func TestPinnedExecutionAndHistory(t *testing.T) {
	h := newFakeHost()
	e := testEngine(t, h)
	b, err := e.start(request())
	if err != nil {
		t.Fatal(err)
	}
	awaitDone(t, e)
	b, err = e.batch(b.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Items) != 4 || b.Status != "completed" {
		t.Fatalf("unexpected batch: %+v", b)
	}
	for _, it := range b.Items {
		if it.Status != "completed" || it.Grade == nil || it.Grade.Score == nil || *it.Grade.Score != 100 {
			t.Fatalf("bad result: %+v", it)
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	seen := map[string]int{}
	for _, r := range h.requests {
		seen[r["auth_id"].(string)]++
		if r["forced_provider"] != "codex" {
			t.Fatal("provider not pinned")
		}
		if _, ok := r["host_callback_id"]; ok {
			t.Fatal("async work attached to expired HTTP context")
		}
		var body []byte
		raw, _ := json.Marshal(r["body"])
		_ = json.Unmarshal(raw, &body)
		var payload map[string]any
		_ = json.Unmarshal(body, &payload)
		if strings.Contains(string(body), "Expected") || strings.Contains(string(body), b.Cases[0].Reference) {
			t.Fatal("reference leaked into model prompt")
		}
		if payload["stream"] != true {
			t.Fatal("not streamed")
		}
	}
	if seen["auth-one"] != 2 || seen["auth-two"] != 2 {
		t.Fatalf("wrong account selection: %v", seen)
	}
	info, err := os.Stat(filepath.Join(e.config.DataDir, "batch-"+b.ID+".json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("history permissions")
	}
	loaded, err := newEngine(h, e.config)
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.close()
	saved, _ := loaded.batch(b.ID, true)
	if saved.Items[0].Reply != b.Items[0].Reply {
		t.Fatal("history lost original reply")
	}
}
func TestCancellationDoesNotGradePartialOutput(t *testing.T) {
	h := newFakeHost()
	h.block = true
	e := testEngine(t, h)
	r := request()
	r.Concurrency = 1
	b, err := e.start(r)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-h.started:
	case <-time.After(2 * time.Second):
		t.Fatal("not started")
	}
	if err = e.cancelBatch(b.ID); err != nil {
		t.Fatal(err)
	}
	awaitDone(t, e)
	b, _ = e.batch(b.ID, true)
	if b.Status != "cancelled" {
		t.Fatal(b.Status)
	}
	for _, it := range b.Items {
		if it.Grade != nil || it.Status != "cancelled" {
			t.Fatal("cancelled work acquired a score")
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.requests) != 1 {
		t.Fatal("queued work ran after cancellation")
	}
}
func TestTruncationAndRetryKeepOriginalEvidence(t *testing.T) {
	h := newFakeHost()
	h.incomplete = true
	e := testEngine(t, h)
	r := request()
	r.Repeats = 1
	b, err := e.start(r)
	if err != nil {
		t.Fatal(err)
	}
	awaitDone(t, e)
	old, _ := e.batch(b.ID, true)
	for _, it := range old.Items {
		if it.Status != "failed" || it.Grade != nil {
			t.Fatal("truncated output was scored")
		}
	}
	h.incomplete = false
	retry, err := e.retry(b.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	awaitDone(t, e)
	if retry.ID == old.ID || retry.RetryOf != old.ID {
		t.Fatal("retry overwrote history")
	}
	for _, c := range retry.Cases {
		if c.Hash != old.Cases[0].Hash {
			t.Fatal("retry changed task version")
		}
	}
	after, _ := e.batch(old.ID, true)
	if after.Items[0].Status != "failed" {
		t.Fatal("old failure was rewritten")
	}
}
func TestValidationRejectsDisabledAndUnknownModel(t *testing.T) {
	e := testEngine(t, newFakeHost())
	r := request()
	r.Targets = []Target{{"disabled", "test-model"}}
	if _, err := e.start(r); err == nil {
		t.Fatal("disabled credential accepted")
	}
	r.Targets = []Target{{"auth-one", "unknown"}}
	if _, err := e.start(r); err == nil {
		t.Fatal("unknown model accepted")
	}
	r = request()
	r.Targets = make([]Target, 0)
	if _, err := e.start(r); err == nil {
		t.Fatal("empty batch accepted")
	}
}
func TestRestartMarksUnfinishedWithoutScoring(t *testing.T) {
	h := newFakeHost()
	e := testEngine(t, h)
	b := &Batch{ID: newID(), Status: "running", CreatedAt: nowMS(), Items: []*Item{{ID: newID(), Status: "running", Reply: "partial"}}}
	e.mu.Lock()
	err := e.saveLocked(b)
	e.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := newEngine(h, e.config)
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.close()
	got, _ := loaded.batch(b.ID, true)
	if got.Status != "interrupted" || got.Items[0].Status != "interrupted" || got.Items[0].Grade != nil || got.Items[0].Reply != "partial" {
		t.Fatal("restart destroyed incomplete evidence")
	}
}
func TestBaselineIsProtectedFromRetentionAndDeletion(t *testing.T) {
	e := testEngine(t, newFakeHost())
	b, err := e.start(request())
	if err != nil {
		t.Fatal(err)
	}
	awaitDone(t, e)
	if err = e.setBaseline(b.ID); err != nil {
		t.Fatal(err)
	}
	if err = e.remove(b.ID); err == nil {
		t.Fatal("baseline deleted")
	}
	e.mu.Lock()
	e.batches[b.ID].CreatedAt = 0
	err = e.pruneLocked()
	e.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.batch(b.ID, false); err != nil {
		t.Fatal("retention deleted baseline")
	}
	if err = e.setBaseline(""); err != nil {
		t.Fatal(err)
	}
	if err = e.remove(b.ID); err != nil {
		t.Fatal(err)
	}
}

type registryHost struct{ *fakeHost }

func (h registryHost) Call(method string, input, output any) error {
	if method == "host.auth.models" {
		raw := []byte(`{"credentials":[{"id":"auth-one","models":["test-model"]},{"id":"auth-two","models":["test-model"]},{"id":"disabled","models":["test-model"]},{"id":"config-key","auth_index":"config-index","provider":"openai-compatible-test","models":["test-model"]}]}`)
		return json.Unmarshal(raw, output)
	}
	return h.fakeHost.Call(method, input, output)
}
func TestRegistryCredentialsKeepDisabledState(t *testing.T) {
	e := testEngine(t, registryHost{newFakeHost()})
	accounts, err := e.accounts()
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 4 {
		t.Fatalf("accounts=%+v", accounts)
	}
	for _, a := range accounts {
		if a.ID == "disabled" && !a.Disabled {
			t.Fatal("disabled account resurrected")
		}
		if a.ID == "config-key" && (a.AuthIndex != "config-index" || a.Provider != "openai-compatible-test" || !contains(a.Models, "test-model")) {
			t.Fatalf("registry identity lost: %+v", a)
		}
	}
}
