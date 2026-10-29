package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

const pluginID = "model-quality"
const pluginVersion = "0.1.2"
const apiPrefix = "/v0/management/plugins/" + pluginID
const resourcePrefix = "/v0/resource/plugins/" + pluginID

//go:embed web/*
var assets embed.FS

var engineMu sync.Mutex
var activeEngine *Engine

func shutdownEngine() {
	engineMu.Lock()
	e := activeEngine
	activeEngine = nil
	engineMu.Unlock()
	if e != nil {
		e.close()
	}
}

func registration() any {
	d := defaultConfig()
	return map[string]any{"schema_version": 6, "metadata": map[string]any{"Name": pluginID, "Version": pluginVersion, "Author": "Renwang-Huang", "GitHubRepository": "https://github.com/Renwang-Huang/cpa-model-quality", "Logo": resourcePrefix + "/logo.svg", "ConfigFields": []any{
		map[string]any{"Name": "data_dir", "Type": "string", "Default": d.DataDir, "Description": "测试历史目录；应位于持久化可写卷内。"},
		map[string]any{"Name": "retention_days", "Type": "integer", "Default": d.RetentionDays, "Description": "历史保留天数（1–3650）；基线单独保留。"},
		map[string]any{"Name": "max_history", "Type": "integer", "Default": d.MaxHistory, "Description": "最多保留批次数（1–1000）。"},
		map[string]any{"Name": "max_disk_mb", "Type": "integer", "Default": d.MaxDiskMB, "Description": "历史总大小上限MiB（64–4096）；当前任务和基线保留。"},
	}}, "capabilities": map[string]bool{"management_api": true}}
}

func parseConfig(raw []byte) (Config, error) {
	c := defaultConfig()
	if len(raw) > 0 {
		var doc map[string]any
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			return c, fmt.Errorf("invalid plugin config")
		}
		// CPA lifecycle config_yaml is the plugin-owned mapping, not the entire host config.
		if err := yaml.Unmarshal(raw, &c); err != nil {
			return c, err
		}
	}
	if strings.TrimSpace(c.DataDir) == "" {
		return c, fmt.Errorf("data_dir不能为空")
	}
	c.DataDir = filepath.Clean(c.DataDir)
	if c.RetentionDays < 1 || c.RetentionDays > 3650 || c.MaxHistory < 1 || c.MaxHistory > 1000 || c.MaxDiskMB < 64 || c.MaxDiskMB > 4096 {
		return c, fmt.Errorf("无效历史保留配置")
	}
	return c, nil
}

func dispatch(method string, raw []byte) (any, error) {
	switch method {
	case "plugin.register", "plugin.reconfigure":
		var req struct {
			ConfigYAML []byte `json:"config_yaml"`
		}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &req); err != nil {
				return nil, err
			}
		}
		c, err := parseConfig(req.ConfigYAML)
		if err != nil {
			return nil, err
		}
		engineMu.Lock()
		defer engineMu.Unlock()
		if activeEngine != nil {
			e := activeEngine
			e.mu.Lock()
			if e.active != "" {
				e.mu.Unlock()
				return nil, fmt.Errorf("测试运行中，请先取消或等待完成再修改插件配置")
			}
			same := e.config.DataDir == c.DataDir
			if same {
				e.config = c
				e.mu.Unlock()
				return registration(), nil
			}
			e.mu.Unlock()
			e.close()
		}
		e, err := newEngine(abiHost{}, c)
		if err != nil {
			return nil, err
		}
		activeEngine = e
		return registration(), nil
	case "plugin.quiesce", "plugin.shutdown":
		shutdownEngine()
		return map[string]any{}, nil
	case "management.register":
		var routes []any
		for _, s := range []string{"GET bootstrap", "GET batches", "POST batches", "GET batch", "GET artifact", "POST cancel", "POST retry", "DELETE batch", "POST baseline", "POST cleanup", "GET export"} {
			m, p, _ := strings.Cut(s, " ")
			routes = append(routes, map[string]any{"Method": m, "Path": apiPrefix + "/" + p})
		}
		resources := []any{map[string]any{"Path": "/page", "Menu": "模型测智", "Description": "按账号测试与对比模型表现、查看评分及历史"}}
		for _, p := range []string{"/app.js", "/style.css", "/logo.svg"} {
			resources = append(resources, map[string]any{"Path": p})
		}
		return map[string]any{"routes": routes, "resources": resources}, nil
	case "management.handle":
		var req managementRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, err
		}
		if strings.HasPrefix(req.Path, resourcePrefix+"/") {
			return resource(req.Path), nil
		}
		engineMu.Lock()
		e := activeEngine
		engineMu.Unlock()
		if e == nil {
			return respond(503, map[string]string{"error": "插件尚未就绪"}), nil
		}
		return handleAPI(e, req), nil
	default:
		return nil, fmt.Errorf("unsupported plugin method: %s", method)
	}
}

type managementRequest struct {
	Method  string
	Path    string
	Headers http.Header
	Query   map[string][]string
	Body    []byte
}

func (r managementRequest) query(k string) string {
	if v := r.Query[k]; len(v) > 0 {
		return v[0]
	}
	return ""
}

type managementResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

func respond(status int, v any) managementResponse {
	b, _ := json.Marshal(v)
	return managementResponse{status, http.Header{"Content-Type": {"application/json; charset=utf-8"}, "Cache-Control": {"no-store"}, "X-Content-Type-Options": {"nosniff"}}, b}
}
func resource(path string) managementResponse {
	name := strings.TrimPrefix(path, resourcePrefix+"/")
	mime := "text/plain"
	switch name {
	case "page":
		name = "index.html"
		mime = "text/html; charset=utf-8"
	case "app.js":
		mime = "text/javascript; charset=utf-8"
	case "style.css":
		mime = "text/css; charset=utf-8"
	case "logo.svg":
		mime = "image/svg+xml"
	default:
		return respond(404, map[string]string{"error": "resource not found"})
	}
	b, err := assets.ReadFile("web/" + name)
	if err != nil {
		return respond(404, map[string]string{"error": "resource not found"})
	}
	headers := http.Header{"Content-Type": {mime}, "Cache-Control": {"no-cache"}, "X-Content-Type-Options": {"nosniff"}, "Referrer-Policy": {"no-referrer"}}
	if name == "index.html" {
		headers.Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; frame-src about: blob:; connect-src 'self'; object-src 'none'; base-uri 'none'; form-action 'none'")
	}
	return managementResponse{200, headers, b}
}
func decodeRequest(r managementRequest, v any) error {
	if len(r.Body) > 64<<10 {
		return fmt.Errorf("请求过大")
	}
	d := json.NewDecoder(strings.NewReader(string(r.Body)))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("请求必须是唯一JSON对象")
	}
	return nil
}

func handleAPI(e *Engine, r managementRequest) managementResponse {
	var result any
	var err error
	status := 200
	path := strings.TrimPrefix(r.Path, apiPrefix+"/")
	switch r.Method + " " + path {
	case "GET bootstrap":
		var accounts []Account
		accounts, err = e.accounts()
		if err == nil {
			e.mu.Lock()
			result = map[string]any{"version": pluginVersion, "suite_version": suiteVersion, "grader_version": graderVersion, "accounts": accounts, "cases": catalog(), "config": e.config, "active": e.active, "baseline": e.baseline, "max_jobs": maxJobs}
			e.mu.Unlock()
		}
	case "POST batches":
		var v StartRequest
		if err = decodeRequest(r, &v); err == nil {
			result, err = e.start(v)
			status = 201
		}
	case "GET batch":
		result, err = e.batch(r.query("id"), false)
	case "GET batches":
		e.mu.Lock()
		rows := []any{}
		for _, b := range e.orderedLocked() {
			done, failed := 0, 0
			for _, it := range b.Items {
				if it.Status == "completed" {
					done++
				}
				if it.Status == "failed" {
					failed++
				}
			}
			rows = append(rows, map[string]any{"id": b.ID, "created_at": b.CreatedAt, "status": b.Status, "effort": b.Effort, "total": len(b.Items), "completed": done, "failed": failed, "baseline": b.ID == e.baseline, "retry_of": b.RetryOf})
		}
		result = map[string]any{"batches": rows, "active": e.active, "baseline": e.baseline}
		e.mu.Unlock()
	case "GET artifact":
		var b *Batch
		b, err = e.batch(r.query("id"), true)
		if err == nil {
			found := false
			for _, it := range b.Items {
				if it.ID == r.query("item") {
					result = map[string]any{"item": it, "html": extractHTML(it.Reply)}
					found = true
					break
				}
			}
			if !found {
				err = fmt.Errorf("结果不存在")
			}
		}
	case "GET export":
		result, err = e.batch(r.query("id"), true)
	case "POST cancel", "POST retry", "DELETE batch", "POST baseline":
		var v struct {
			ID    string   `json:"id"`
			Items []string `json:"items,omitempty"`
		}
		err = decodeRequest(r, &v)
		if err == nil {
			switch path {
			case "cancel":
				err = e.cancelBatch(v.ID)
			case "retry":
				result, err = e.retry(v.ID, v.Items)
			case "batch":
				err = e.remove(v.ID)
			case "baseline":
				err = e.setBaseline(v.ID)
			}
		}
	case "POST cleanup":
		e.mu.Lock()
		before := len(e.batches)
		err = e.pruneLocked()
		result = map[string]int{"deleted": before - len(e.batches)}
		e.mu.Unlock()
	default:
		return respond(404, map[string]string{"error": "接口不存在"})
	}
	if err != nil {
		return respond(400, map[string]string{"error": redact(err.Error())})
	}
	if result == nil {
		result = map[string]bool{"ok": true}
	}
	return respond(status, result)
}
