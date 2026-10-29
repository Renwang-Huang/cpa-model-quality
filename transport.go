package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
)

type streamParser struct {
	pending    []byte
	text       string
	model      string
	responseID string
	usage      map[string]any
	completed  bool
	failed     string
}

func (s *streamParser) feed(chunk []byte) error {
	// Native Codex callbacks emit complete SSE records without line separators;
	// translated OpenAI-compatible callbacks emit framed SSE bytes. Accept both.
	trimmed := bytes.TrimSpace(chunk)
	if len(s.pending) == 0 && !bytes.Contains(trimmed, []byte("\n")) {
		data := bytes.TrimSpace(bytes.TrimPrefix(trimmed, []byte("data:")))
		if bytes.HasPrefix(trimmed, []byte("data:")) && (json.Valid(data) || bytes.Equal(data, []byte("[DONE]"))) {
			if err := s.event(trimmed); err != nil {
				return err
			}
			if len(s.text) > maxReplyBytes {
				return fmt.Errorf("回复超过256 KiB，未评分")
			}
			return nil
		}
		for _, prefix := range []string{"event:", "id:", "retry:", ":"} {
			if bytes.HasPrefix(trimmed, []byte(prefix)) {
				return nil
			}
		}
	}
	s.pending = append(s.pending, chunk...)
	if len(s.pending) > 1<<20 {
		return fmt.Errorf("上游事件过大，未评分")
	}
	s.pending = bytes.ReplaceAll(s.pending, []byte("\r\n"), []byte("\n"))
	for {
		i := bytes.Index(s.pending, []byte("\n\n"))
		if i < 0 {
			break
		}
		part := s.pending[:i]
		s.pending = s.pending[i+2:]
		if err := s.event(part); err != nil {
			return err
		}
	}
	if len(s.text) > maxReplyBytes {
		return fmt.Errorf("回复超过256 KiB，未评分")
	}
	return nil
}
func (s *streamParser) event(raw []byte) error {
	var data []byte
	for _, line := range bytes.Split(raw, []byte("\n")) {
		if bytes.HasPrefix(line, []byte("data:")) {
			if len(data) > 0 {
				data = append(data, '\n')
			}
			data = append(data, bytes.TrimPrefix(bytes.TrimPrefix(line, []byte("data:")), []byte(" "))...)
		}
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil
	}
	if bytes.Equal(data, []byte("[DONE]")) {
		return nil
	}
	var ev struct {
		Type     string         `json:"type"`
		Delta    string         `json:"delta"`
		Model    string         `json:"model"`
		ID       string         `json:"id"`
		Usage    map[string]any `json:"usage"`
		Response *struct {
			ID     string         `json:"id"`
			Model  string         `json:"model"`
			Status string         `json:"status"`
			Usage  map[string]any `json:"usage"`
			Output []struct {
				Type    string `json:"type"`
				Content []struct {
					Type    string `json:"type"`
					Text    string `json:"text"`
					Refusal string `json:"refusal"`
				} `json:"content"`
			} `json:"output"`
		} `json:"response"`
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
			FinishReason *string `json:"finish_reason"`
		} `json:"choices"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("上游流包含无效JSON事件，未评分")
	}
	if len(ev.Error) > 0 && string(ev.Error) != "null" {
		s.failed = "上游返回错误事件"
	}
	if ev.Model != "" {
		s.model = ev.Model
	}
	if ev.ID != "" {
		s.responseID = ev.ID
	}
	if ev.Usage != nil {
		s.usage = ev.Usage
	}
	if ev.Response != nil {
		r := ev.Response
		if r.Model != "" {
			s.model = r.Model
		}
		if r.ID != "" {
			s.responseID = r.ID
		}
		if r.Usage != nil {
			s.usage = r.Usage
		}
	}
	if len(s.model) > 512 || len(s.responseID) > 512 {
		return fmt.Errorf("上游响应标识过长，未评分")
	}
	if s.usage != nil {
		s.usage = usageSummary(s.usage)
	}
	switch ev.Type {
	case "response.output_text.delta":
		s.text += ev.Delta
	case "response.failed", "error":
		s.failed = "上游生成失败，未评分"
	case "response.incomplete":
		s.failed = "上游回复被截断或未完成，未评分"
	case "response.completed":
		if ev.Response == nil || ev.Response.Status != "completed" {
			s.failed = "上游没有确认完整生成，未评分"
			break
		}
		s.completed = true
		var text strings.Builder
		for _, o := range ev.Response.Output {
			if o.Type == "message" {
				for _, c := range o.Content {
					if c.Type == "output_text" {
						text.WriteString(c.Text)
					}
					if c.Type == "refusal" {
						text.WriteString(c.Refusal)
					}
				}
			}
		}
		if text.Len() > 0 {
			s.text = text.String()
		}
	}
	for _, c := range ev.Choices {
		s.text += c.Delta.Content
		if c.FinishReason != nil {
			if *c.FinishReason == "stop" {
				s.completed = true
			} else {
				s.failed = "上游结束原因为 " + *c.FinishReason + "，未评分"
			}
		}
	}
	return nil
}

func usageSummary(raw map[string]any) map[string]any {
	out := map[string]any{}
	for _, key := range []string{"input_tokens", "output_tokens", "total_tokens", "prompt_tokens", "completion_tokens"} {
		if n, ok := raw[key].(float64); ok && n >= 0 && n <= 1e12 && math.Trunc(n) == n {
			out[key] = n
		}
	}
	for _, key := range []string{"input_tokens_details", "output_tokens_details", "prompt_tokens_details", "completion_tokens_details"} {
		if detail, ok := raw[key].(map[string]any); ok {
			filtered := map[string]any{}
			for _, name := range []string{"cached_tokens", "reasoning_tokens"} {
				if n, ok := detail[name].(float64); ok && n >= 0 && n <= 1e12 && math.Trunc(n) == n {
					filtered[name] = n
				}
			}
			if len(filtered) > 0 {
				out[key] = filtered
			}
		}
	}
	return out
}
func (s *streamParser) finish() error {
	if len(bytes.TrimSpace(s.pending)) > 0 {
		if err := s.event(s.pending); err != nil {
			return err
		}
		s.pending = nil
	}
	if s.failed != "" {
		return fmt.Errorf("%s", s.failed)
	}
	if !s.completed {
		return fmt.Errorf("连接结束前未收到完整完成事件，未评分")
	}
	if len(s.text) > maxReplyBytes {
		return fmt.Errorf("回复超过256 KiB，未评分")
	}
	if strings.TrimSpace(s.text) == "" {
		return fmt.Errorf("没有完整的文本回复，未评分")
	}
	return nil
}

func (e *Engine) execute(ctx context.Context, b *Batch, it *Item) {
	var c Case
	for _, candidate := range b.Cases {
		if candidate.ID == it.CaseID {
			c = candidate
			break
		}
	}
	e.mu.Lock()
	if ctx.Err() != nil {
		e.mu.Unlock()
		return
	}
	it.Status = "running"
	it.StartedAt = nowMS()
	b.Revision++
	e.mu.Unlock()
	parser := &streamParser{}
	var runErr error
	defer func() {
		if recover() != nil {
			runErr = fmt.Errorf("内部处理失败，未评分")
		}
		e.mu.Lock()
		stream := it.streamID
		it.streamID = ""
		e.mu.Unlock()
		if stream != "" {
			_ = e.host.Call("host.model.stream_close", map[string]any{"stream_id": stream}, nil)
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		it.FinishedAt = nowMS()
		it.DurationMS = it.FinishedAt - it.StartedAt
		it.Reply = parser.text
		it.Preview = previewText(parser.text)
		it.ResponseModel = parser.model
		it.ResponseID = parser.responseID
		it.Usage = parser.usage
		it.HasHTML = extractHTML(parser.text) != ""
		if ctx.Err() != nil {
			it.Status = "cancelled"
			it.Error = "用户已取消，部分回复未评分"
			it.Grade = nil
		} else if runErr != nil {
			it.Status = "failed"
			it.Error = redact(runErr.Error())
			it.Grade = nil
		} else {
			it.Status = "completed"
			it.Grade = grade(c, parser.text)
		}
		b.Revision++
		if err := e.saveLocked(b); err != nil {
			b.Error = "历史保存失败: " + err.Error()
		}
	}()
	body := map[string]any{"model": it.Model, "stream": true, "store": false, "input": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": c.Prompt}}}}}
	if b.Effort != "auto" {
		body["reasoning"] = map[string]string{"effort": b.Effort}
	}
	raw, _ := json.Marshal(body)
	var started struct {
		StatusCode int    `json:"status_code"`
		StreamID   string `json:"stream_id"`
	}
	// Omit the handler callback ID: this user-owned job must outlive the short start request.
	runErr = e.host.Call("host.model.execute_stream", map[string]any{"entry_protocol": "openai-response", "exit_protocol": "openai-response", "model": it.Model, "auth_id": it.AuthID, "forced_provider": it.Provider, "stream": true, "body": raw}, &started)
	if runErr != nil {
		return
	}
	if started.StreamID == "" {
		runErr = fmt.Errorf("CPA未返回可读取的模型流")
		return
	}
	e.mu.Lock()
	it.streamID = started.StreamID
	e.mu.Unlock()
	if started.StatusCode < 200 || started.StatusCode >= 300 {
		runErr = fmt.Errorf("上游HTTP %d，未评分", started.StatusCode)
		return
	}
	closed := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = e.host.Call("host.model.stream_close", map[string]any{"stream_id": started.StreamID}, nil)
		case <-closed:
		}
	}()
	defer close(closed)
	lastProgress := time.Time{}
	for {
		if ctx.Err() != nil {
			return
		}
		var chunk struct {
			Payload []byte `json:"payload"`
			Done    bool   `json:"done"`
			Error   string `json:"error"`
		}
		if runErr = e.host.Call("host.model.stream_read", map[string]any{"stream_id": started.StreamID}, &chunk); runErr != nil {
			return
		}
		if runErr = parser.feed(chunk.Payload); runErr != nil {
			return
		}
		if chunk.Error != "" {
			runErr = fmt.Errorf("%s", redact(chunk.Error))
			return
		}
		if (parser.text != "" && (lastProgress.IsZero() || time.Since(lastProgress) > 200*time.Millisecond)) || chunk.Done {
			e.mu.Lock()
			it.Preview = previewText(parser.text)
			if it.TTFTMS == 0 && parser.text != "" {
				it.TTFTMS = nowMS() - it.StartedAt
				if it.TTFTMS == 0 {
					it.TTFTMS = 1
				}
			}
			b.Revision++
			e.mu.Unlock()
			lastProgress = time.Now()
		}
		if chunk.Done {
			runErr = parser.finish()
			return
		}
	}
}
func previewText(s string) string {
	r := []rune(s)
	if len(r) > 1000 {
		return string(r[:1000]) + "…"
	}
	return s
}

var htmlStart = regexp.MustCompile(`(?i)<!doctype\s+html|<html\b|<svg\b`)

func extractHTML(raw string) string {
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(s, "```") {
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = s[i+1:]
			if end := strings.LastIndex(s, "```"); end >= 0 {
				s = s[:end]
			}
		}
	}
	loc := htmlStart.FindStringIndex(s)
	if loc == nil {
		return ""
	}
	return strings.TrimSpace(s[loc[0]:])
}
