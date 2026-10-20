package main

// Webhook 通知：事件发生时向飞书 / 企业微信 / 钉钉 / 自定义机器人推送消息。
// 通过 core.AddEventHook 注册回调，所有 core.LogEvent 调用都会走到这里。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Liu670/rtp/internal/core"
)

var webhookHTTP = &http.Client{Timeout: 6 * time.Second}

// registerWebhookHook 在服务启动时调用：把 webhook 分发挂到事件流上
func (s *Server) registerWebhookHook() {
	core.AddEventHook(s.fireWebhook)
}

// fireWebhook 遍历已启用的机器人，命中订阅类型就异步发送
func (s *Server) fireWebhook(ev core.Event) {
	s.mu.Lock()
	whs := make([]core.WebhookConfig, len(s.cfg.Webhooks))
	copy(whs, s.cfg.Webhooks)
	s.mu.Unlock()
	for _, wh := range whs {
		if !wh.Enabled || wh.URL == "" {
			continue
		}
		hit := false
		for _, t := range wh.Events {
			if t == ev.Type {
				hit = true
				break
			}
		}
		if !hit {
			continue
		}
		go sendWebhook(wh, ev)
	}
}

// sendWebhook 按机器人类型构造消息体并发送（失败静默，不影响主流程）
func sendWebhook(wh core.WebhookConfig, ev core.Event) {
	defer func() { _ = recover() }()

	t := time.Unix(ev.Ts, 0).Format("2006-01-02 15:04:05")
	name := ev.Type
	if n, ok := core.EventTypes[ev.Type]; ok {
		name = n
	}
	full := fmt.Sprintf("【rtp 通知】%s\n%s\n%s", name, ev.Msg, t)

	var payload interface{}
	switch wh.Type {
	case "feishu":
		payload = map[string]interface{}{
			"msg_type": "text",
			"content":  map[string]string{"text": full},
		}
	case "wecom":
		payload = map[string]interface{}{
			"msgtype": "text",
			"text":    map[string]string{"content": full},
		}
	case "dingtalk":
		payload = map[string]interface{}{
			"msgtype": "text",
			"text":    map[string]string{"content": full},
		}
	default: // generic：原始 JSON，便于自建接收端
		payload = map[string]interface{}{
			"type":    ev.Type,
			"level":   ev.Level,
			"message": ev.Msg,
			"time":    ev.Ts,
		}
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	resp, err := webhookHTTP.Post(wh.URL, "application/json", bytes.NewReader(b))
	if err != nil {
		core.LogEvent("admin", "warn", "通知发送失败("+name+"): "+err.Error())
		return
	}
	resp.Body.Close()
}

// validWebhook 校验机器人配置；返回错误信息（空串=通过）
func validWebhook(wh *core.WebhookConfig) string {
	switch wh.Type {
	case "feishu", "wecom", "dingtalk", "generic":
	default:
		return "不支持的通知类型"
	}
	if len(wh.URL) < 12 || !(len(wh.URL) > 8 && (wh.URL[:7] == "http://" || wh.URL[:8] == "https://")) {
		return "Webhook 地址必须是合法的 http(s) 链接"
	}
	if len(wh.Events) == 0 {
		return "至少选择一种通知事件"
	}
	for _, e := range wh.Events {
		if _, ok := core.EventTypes[e]; !ok {
			return "未知的事件类型: " + e
		}
	}
	return ""
}

// ---------- Webhook 管理 API ----------

// handleWebhooks GET 列表 / POST 新建
func (s *Server) handleWebhooks(w http.ResponseWriter, r *http.Request) {
	if !s.authed(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.mu.Lock()
		whs := s.cfg.Webhooks
		if whs == nil {
			whs = []core.WebhookConfig{}
		}
		s.mu.Unlock()
		writeJSON(w, 200, map[string]interface{}{"webhooks": whs, "event_types": core.EventTypes})

	case http.MethodPost:
		var in core.WebhookConfig
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		in.ID = core.RandID()[:16]
		if msg := validWebhook(&in); msg != "" {
			http.Error(w, msg, 400)
			return
		}
		s.mu.Lock()
		s.cfg.Webhooks = append(s.cfg.Webhooks, in)
		s.saveConfig()
		s.mu.Unlock()
		core.LogEvent("admin", "ok", "新增通知机器人: "+in.Type)
		writeJSON(w, 200, in)

	default:
		http.Error(w, "method not allowed", 405)
	}
}

// handleWebhookOne PUT 修改 / DELETE 删除 / POST 子路径 test 发送测试
func (s *Server) handleWebhookOne(w http.ResponseWriter, r *http.Request) {
	if !s.authed(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/webhooks/")
	id := rest
	action := ""
	if i := strings.Index(rest, "/"); i >= 0 {
		id, action = rest[:i], rest[i+1:]
	}

	s.mu.Lock()
	idx := -1
	for i := range s.cfg.Webhooks {
		if s.cfg.Webhooks[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		s.mu.Unlock()
		http.Error(w, "not found", 404)
		return
	}

	switch {
	case r.Method == http.MethodPost && action == "test":
		wh := s.cfg.Webhooks[idx]
		s.mu.Unlock()
		go sendWebhook(wh, core.Event{Ts: time.Now().Unix(), Type: "test", Level: "ok", Msg: "这是一条测试通知，收到即表示配置成功 ✓"})
		writeJSON(w, 200, map[string]interface{}{"ok": true, "message": "测试消息已发送，请到群里查看"})
		return

	case r.Method == http.MethodPut && action == "":
		var in core.WebhookConfig
		s.mu.Unlock()
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		in.ID = id
		if msg := validWebhook(&in); msg != "" {
			http.Error(w, msg, 400)
			return
		}
		s.mu.Lock()
		s.cfg.Webhooks[idx] = in
		s.saveConfig()
		s.mu.Unlock()
		core.LogEvent("admin", "ok", "修改通知机器人: "+in.Type)
		writeJSON(w, 200, in)
		return

	case r.Method == http.MethodDelete && action == "":
		typ := s.cfg.Webhooks[idx].Type
		s.cfg.Webhooks = append(s.cfg.Webhooks[:idx], s.cfg.Webhooks[idx+1:]...)
		s.saveConfig()
		s.mu.Unlock()
		core.LogEvent("admin", "ok", "删除通知机器人: "+typ)
		writeJSON(w, 200, map[string]interface{}{"ok": true})
		return
	}
	s.mu.Unlock()
	http.Error(w, "method not allowed", 405)
}
