package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const maxReplyBytes = 256 << 10
const maxJobs = 200

type Config struct {
	DataDir       string `yaml:"data_dir" json:"data_dir"`
	RetentionDays int    `yaml:"retention_days" json:"retention_days"`
	MaxHistory    int    `yaml:"max_history" json:"max_history"`
	MaxDiskMB     int    `yaml:"max_disk_mb" json:"max_disk_mb"`
}

func defaultConfig() Config {
	return Config{DataDir: "./logs/model-quality-data", RetentionDays: 7, MaxHistory: 100, MaxDiskMB: 256}
}

type Account struct {
	ID          string   `json:"id"`
	AuthIndex   string   `json:"auth_index"`
	Provider    string   `json:"provider"`
	Name        string   `json:"name"`
	Email       string   `json:"email"`
	Account     string   `json:"account"`
	Label       string   `json:"label"`
	Note        string   `json:"note"`
	Disabled    bool     `json:"disabled"`
	Unavailable bool     `json:"unavailable"`
	Status      string   `json:"status"`
	Models      []string `json:"models"`
}

func (a Account) Display() string {
	for _, s := range []string{a.Email, a.Account, a.Label, a.Name, a.ID} {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return "未命名账号"
}

type Target struct {
	AuthID string `json:"auth_id"`
	Model  string `json:"model"`
}
type StartRequest struct {
	Targets      []Target `json:"targets"`
	CaseIDs      []string `json:"case_ids"`
	CustomPrompt string   `json:"custom_prompt"`
	Effort       string   `json:"effort"`
	Concurrency  int      `json:"concurrency"`
	Repeats      int      `json:"repeats"`
}
type Item struct {
	ID            string         `json:"id"`
	AuthID        string         `json:"auth_id"`
	Account       string         `json:"account"`
	Note          string         `json:"note"`
	Provider      string         `json:"provider"`
	Model         string         `json:"model"`
	CaseID        string         `json:"case_id"`
	Trial         int            `json:"trial"`
	Status        string         `json:"status"`
	StartedAt     int64          `json:"started_at,omitempty"`
	FinishedAt    int64          `json:"finished_at,omitempty"`
	TTFTMS        int64          `json:"ttft_ms,omitempty"`
	DurationMS    int64          `json:"duration_ms,omitempty"`
	Reply         string         `json:"reply,omitempty"`
	Preview       string         `json:"preview,omitempty"`
	ResponseModel string         `json:"response_model,omitempty"`
	ResponseID    string         `json:"response_id,omitempty"`
	Usage         map[string]any `json:"usage,omitempty"`
	Grade         *Grade         `json:"grade,omitempty"`
	HasHTML       bool           `json:"has_html"`
	Error         string         `json:"error,omitempty"`
	streamID      string
}
type Batch struct {
	ID            string  `json:"id"`
	Revision      uint64  `json:"revision"`
	SuiteVersion  string  `json:"suite_version"`
	GraderVersion string  `json:"grader_version"`
	CreatedAt     int64   `json:"created_at"`
	FinishedAt    int64   `json:"finished_at,omitempty"`
	Status        string  `json:"status"`
	Effort        string  `json:"effort"`
	Concurrency   int     `json:"concurrency"`
	Cases         []Case  `json:"cases"`
	Items         []*Item `json:"items"`
	RetryOf       string  `json:"retry_of,omitempty"`
	Error         string  `json:"error,omitempty"`
}
type Engine struct {
	mu       sync.Mutex
	host     hostCaller
	config   Config
	batches  map[string]*Batch
	active   string
	baseline string
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	closing  bool
}

func newEngine(h hostCaller, c Config) (*Engine, error) {
	e := &Engine{host: h, config: c, batches: map[string]*Batch{}}
	if err := os.MkdirAll(c.DataDir, 0700); err != nil {
		return nil, fmt.Errorf("create history directory: %w", err)
	}
	_ = os.Chmod(c.DataDir, 0700)
	files, err := filepath.Glob(filepath.Join(c.DataDir, "batch-*.json"))
	if err != nil {
		return nil, err
	}
	if len(files) > 2000 {
		return nil, fmt.Errorf("history directory contains too many files")
	}
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			return nil, err
		}
		if info.Size() > 64<<20 {
			return nil, fmt.Errorf("history file too large: %s", filepath.Base(f))
		}
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var batch Batch
		if err = json.Unmarshal(b, &batch); err != nil || !validID(batch.ID) {
			return nil, fmt.Errorf("invalid history file: %s", filepath.Base(f))
		}
		if filepath.Base(f) != "batch-"+batch.ID+".json" {
			return nil, fmt.Errorf("history identity mismatch")
		}
		e.batches[batch.ID] = &batch
		if batch.Status == "running" || batch.Status == "cancelling" {
			batch.Status = "interrupted"
			batch.FinishedAt = nowMS()
			batch.Revision++
			for _, it := range batch.Items {
				if it.Status == "running" || it.Status == "queued" {
					it.Status = "interrupted"
					it.Grade = nil
					it.Error = "服务重启，未完整收到回复；可手动重试"
				}
			}
			if err = e.saveLocked(&batch); err != nil {
				return nil, err
			}
		}
	}
	if b, err := os.ReadFile(filepath.Join(c.DataDir, "settings.json")); err == nil {
		var s struct {
			Baseline string `json:"baseline"`
		}
		if json.Unmarshal(b, &s) == nil && e.batches[s.Baseline] != nil {
			e.baseline = s.Baseline
		}
	}
	if err = e.pruneLocked(); err != nil {
		return nil, err
	}
	return e, nil
}
func nowMS() int64 { return time.Now().UnixMilli() }

var idPattern = regexp.MustCompile(`^[0-9]{13}-[a-f0-9]{16}$`)

func validID(s string) bool { return idPattern.MatchString(s) }
func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("random source unavailable")
	}
	return fmt.Sprintf("%d-%s", nowMS(), hex.EncodeToString(b[:]))
}

var secretPattern = regexp.MustCompile(`(?i)(bearer\s+[a-z0-9._~+/=-]+|sk-[a-z0-9_-]{8,}|cpamp_[a-z0-9_-]+|(?:access_token|refresh_token|api_key)["\s:=]+[a-z0-9._~+/=-]{8,})`)

func redact(s string) string {
	s = secretPattern.ReplaceAllString(s, "[REDACTED]")
	r := []rune(s)
	if len(r) > 1600 {
		return string(r[:1600]) + "…"
	}
	return s
}

func (e *Engine) accounts() ([]Account, error) {
	var a struct {
		Files []Account `json:"files"`
	}
	if err := e.host.Call("host.auth.list", map[string]any{}, &a); err != nil {
		return nil, err
	}
	var m struct {
		Credentials []struct {
			ID        string   `json:"id"`
			AuthIndex string   `json:"auth_index"`
			Provider  string   `json:"provider"`
			Models    []string `json:"models"`
		} `json:"credentials"`
	}
	if err := e.host.Call("host.auth.models", map[string]any{}, &m); err != nil {
		return nil, err
	}
	models := map[string][]string{}
	known := map[string]bool{}
	for _, account := range a.Files {
		known[account.ID] = true
	}
	for _, v := range m.Credentials {
		models[v.ID] = v.Models
		// Config credentials are in the model registry but omitted from host.auth.list.
		// The host validates their current availability again for each pinned call.
		if !known[v.ID] && v.ID != "" && len(v.Models) > 0 {
			a.Files = append(a.Files, Account{ID: v.ID, AuthIndex: v.AuthIndex, Provider: v.Provider, Label: v.Provider + " · " + v.AuthIndex, Status: "registry"})
			known[v.ID] = true
		}
	}
	for i := range a.Files {
		a.Files[i].Models = models[a.Files[i].ID]
	}
	sort.Slice(a.Files, func(i, j int) bool { return a.Files[i].Display() < a.Files[j].Display() })
	return a.Files, nil
}

func (e *Engine) start(r StartRequest) (*Batch, error) {
	if r.Concurrency < 1 || r.Concurrency > 10 {
		return nil, fmt.Errorf("并发数须为1–10")
	}
	if r.Repeats < 1 || r.Repeats > 5 {
		return nil, fmt.Errorf("重复次数须为1–5")
	}
	if !strings.Contains("|auto|none|minimal|low|medium|high|xhigh|max|ultra|", "|"+r.Effort+"|") || r.Effort == "" {
		return nil, fmt.Errorf("无效推理档位")
	}
	if len(r.Targets) < 1 || len(r.Targets) > 200 {
		return nil, fmt.Errorf("请选择1–200个账号/模型组合")
	}
	accounts, err := e.accounts()
	if err != nil {
		return nil, err
	}
	byID := map[string]Account{}
	for _, a := range accounts {
		byID[a.ID] = a
	}
	cases := map[string]Case{}
	for _, c := range catalog() {
		cases[c.ID] = c
	}
	if strings.TrimSpace(r.CustomPrompt) != "" {
		if len(r.CustomPrompt) > 32<<10 {
			return nil, fmt.Errorf("自定义提示词最多32 KiB")
		}
		cases["custom"] = sealCase(Case{ID: "custom", Title: "自定义测试", Category: "自定义", Kind: "manual", Prompt: r.CustomPrompt, Reference: "自定义题仅保留原始回复并人工比较；不自动评分。", Source: "用户输入"})
	}
	b := &Batch{ID: newID(), SuiteVersion: suiteVersion, GraderVersion: graderVersion, CreatedAt: nowMS(), Status: "running", Effort: r.Effort, Concurrency: r.Concurrency, Cases: []Case{}, Items: []*Item{}}
	seen := map[string]bool{}
	for _, id := range r.CaseIDs {
		if seen[id] {
			continue
		}
		c, ok := cases[id]
		if !ok {
			return nil, fmt.Errorf("未知测试题: %s", id)
		}
		b.Cases = append(b.Cases, c)
		seen[id] = true
	}
	if len(b.Cases) == 0 {
		return nil, fmt.Errorf("请选择测试题")
	}
	seen = map[string]bool{}
	for _, t := range r.Targets {
		a, ok := byID[t.AuthID]
		if !ok || a.Disabled {
			return nil, fmt.Errorf("账号不存在或已禁用，请刷新账号列表")
		}
		t.Model = strings.TrimSpace(t.Model)
		if !contains(a.Models, t.Model) {
			return nil, fmt.Errorf("所选模型未注册到账号 %s，请刷新模型列表", a.Display())
		}
		key := t.AuthID + "\x00" + t.Model
		if seen[key] {
			continue
		}
		seen[key] = true
		for trial := 1; trial <= r.Repeats; trial++ {
			for _, c := range b.Cases {
				b.Items = append(b.Items, &Item{ID: newID(), AuthID: a.ID, Account: a.Display(), Note: a.Note, Provider: a.Provider, Model: t.Model, CaseID: c.ID, Trial: trial, Status: "queued"})
			}
		}
	}
	if len(b.Items) > maxJobs {
		return nil, fmt.Errorf("单批最多%d次请求，当前%d次", maxJobs, len(b.Items))
	}
	return e.launch(b)
}
func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func (e *Engine) launch(b *Batch) (*Batch, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closing {
		return nil, fmt.Errorf("插件正在停止")
	}
	if e.active != "" {
		return nil, fmt.Errorf("已有测试运行，请先完成或取消")
	}
	if err := e.pruneLocked(); err != nil {
		return nil, err
	}
	e.batches[b.ID] = b
	b.Revision++
	if err := e.saveLocked(b); err != nil {
		delete(e.batches, b.ID)
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel
	e.active = b.ID
	e.wg.Add(1)
	go func() { defer e.wg.Done(); e.run(ctx, b) }()
	return e.viewLocked(b, false), nil
}

func (e *Engine) run(ctx context.Context, b *Batch) {
	jobs := make(chan *Item)
	var workers sync.WaitGroup
	for i := 0; i < b.Concurrency; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for it := range jobs {
				if ctx.Err() != nil {
					continue
				}
				e.execute(ctx, b, it)
			}
		}()
	}
	for _, it := range b.Items {
		if ctx.Err() != nil {
			break
		}
		select {
		case jobs <- it:
		case <-ctx.Done():
		}
	}
	close(jobs)
	workers.Wait()
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, it := range b.Items {
		if it.Status == "queued" || it.Status == "running" {
			it.Status = "cancelled"
			it.FinishedAt = nowMS()
			it.Grade = nil
		}
	}
	b.Status = "completed"
	if ctx.Err() != nil {
		b.Status = "cancelled"
	}
	b.FinishedAt = nowMS()
	b.Revision++
	if err := e.saveLocked(b); err != nil {
		b.Error = "历史保存失败: " + err.Error()
	}
	e.active = ""
	e.cancel = nil
	if err := e.pruneLocked(); err != nil {
		b.Error = "历史清理失败: " + err.Error()
	}
}

func (e *Engine) cancelBatch(id string) error {
	e.mu.Lock()
	b := e.batches[id]
	if b == nil {
		e.mu.Unlock()
		return fmt.Errorf("测试不存在")
	}
	if e.active != id {
		e.mu.Unlock()
		return nil
	}
	b.Status = "cancelling"
	b.Revision++
	e.cancel()
	var streams []string
	for _, it := range b.Items {
		if it.Status == "queued" {
			it.Status = "cancelled"
			it.FinishedAt = nowMS()
		}
		if it.streamID != "" {
			streams = append(streams, it.streamID)
		}
	}
	e.mu.Unlock()
	for _, id := range streams {
		_ = e.host.Call("host.model.stream_close", map[string]any{"stream_id": id}, nil)
	}
	return nil
}
func (e *Engine) close() {
	e.mu.Lock()
	e.closing = true
	id := e.active
	e.mu.Unlock()
	if id != "" {
		_ = e.cancelBatch(id)
	}
	e.wg.Wait()
}

func (e *Engine) retry(id string, selected []string) (*Batch, error) {
	e.mu.Lock()
	original := e.batches[id]
	if original == nil {
		e.mu.Unlock()
		return nil, fmt.Errorf("测试不存在")
	}
	source := jsonClone(*original)
	e.mu.Unlock()
	if source.Status == "running" || source.Status == "cancelling" {
		return nil, fmt.Errorf("请先等待本批测试结束")
	}
	accounts, err := e.accounts()
	if err != nil {
		return nil, err
	}
	enabled := map[string]Account{}
	for _, a := range accounts {
		if !a.Disabled {
			enabled[a.ID] = a
		}
	}
	b := &Batch{ID: newID(), SuiteVersion: source.SuiteVersion, GraderVersion: source.GraderVersion, CreatedAt: nowMS(), Status: "running", Effort: source.Effort, Concurrency: source.Concurrency, Cases: source.Cases, RetryOf: source.ID, Items: []*Item{}}
	for _, it := range source.Items {
		if len(selected) > 0 && !contains(selected, it.ID) {
			continue
		}
		if len(selected) == 0 && it.Status == "completed" {
			continue
		}
		a, ok := enabled[it.AuthID]
		if !ok || !contains(a.Models, it.Model) {
			return nil, fmt.Errorf("原账号已禁用、移除或不再支持模型；未换号重试")
		}
		copy := *it
		copy.ID = newID()
		copy.Account = a.Display()
		copy.Note = a.Note
		copy.Status = "queued"
		copy.StartedAt = 0
		copy.FinishedAt = 0
		copy.DurationMS = 0
		copy.TTFTMS = 0
		copy.Reply = ""
		copy.Preview = ""
		copy.Error = ""
		copy.Grade = nil
		copy.HasHTML = false
		copy.Usage = nil
		copy.ResponseID = ""
		copy.ResponseModel = ""
		copy.streamID = ""
		b.Items = append(b.Items, &copy)
	}
	if len(b.Items) == 0 {
		return nil, fmt.Errorf("没有需要重试的项目")
	}
	return e.launch(b)
}

func (e *Engine) viewLocked(b *Batch, full bool) *Batch {
	copy := *b
	copy.Items = make([]*Item, len(b.Items))
	for i, it := range b.Items {
		v := *it
		if !full {
			v.Reply = ""
		}
		copy.Items[i] = &v
	}
	return jsonClone(&copy)
}
func (e *Engine) batch(id string, full bool) (*Batch, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	b := e.batches[id]
	if b == nil {
		return nil, fmt.Errorf("测试不存在")
	}
	return e.viewLocked(b, full), nil
}

func atomicFile(path string, body []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".writing-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(body)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, path)
}
func (e *Engine) saveLocked(b *Batch) error {
	raw, err := json.Marshal(b)
	if err != nil {
		return err
	}
	return atomicFile(filepath.Join(e.config.DataDir, "batch-"+b.ID+".json"), raw)
}

func (e *Engine) orderedLocked() []*Batch {
	out := make([]*Batch, 0, len(e.batches))
	for _, b := range e.batches {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}
func (e *Engine) pruneLocked() error {
	all := e.orderedLocked()
	var total int64
	for _, b := range all {
		if s, err := os.Stat(filepath.Join(e.config.DataDir, "batch-"+b.ID+".json")); err == nil {
			total += s.Size()
		}
	}
	count := len(all)
	for i := len(all) - 1; i >= 0; i-- {
		b := all[i]
		if b.ID == e.active || b.ID == e.baseline || b.Status == "running" || b.Status == "cancelling" {
			continue
		}
		if time.Now().UnixMilli()-b.CreatedAt > int64(e.config.RetentionDays)*86400000 || count > e.config.MaxHistory || total > int64(e.config.MaxDiskMB)<<20 {
			p := filepath.Join(e.config.DataDir, "batch-"+b.ID+".json")
			s, err := os.Stat(p)
			if err != nil {
				return err
			}
			if err = os.Remove(p); err != nil {
				return err
			}
			total -= s.Size()
			count--
			delete(e.batches, b.ID)
		}
	}
	return nil
}

func (e *Engine) remove(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if id == e.active {
		return fmt.Errorf("运行中的测试不能删除")
	}
	if e.batches[id] == nil {
		return fmt.Errorf("测试不存在")
	}
	if id == e.baseline {
		return fmt.Errorf("请先取消基线，再删除该批记录")
	}
	if err := os.Remove(filepath.Join(e.config.DataDir, "batch-"+id+".json")); err != nil {
		return err
	}
	delete(e.batches, id)
	return nil
}
func (e *Engine) setBaseline(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if id != "" {
		b := e.batches[id]
		if b == nil || b.Status != "completed" {
			return fmt.Errorf("只能将已结束的完整批次设为基线")
		}
	}
	b, _ := json.Marshal(map[string]string{"baseline": id})
	if err := atomicFile(filepath.Join(e.config.DataDir, "settings.json"), b); err != nil {
		return err
	}
	e.baseline = id
	return nil
}
