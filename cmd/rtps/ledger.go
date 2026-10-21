package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// dayRec 单日上行/下行字节累计
type dayRec struct {
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

// metaRec 实体展示信息（记录写入时刻的名称等，便于实体被改名/删除后仍可读出「是谁」）
type metaRec struct {
	Name       string `json:"n"`
	Proto      string `json:"p,omitempty"`  // 仅隧道：协议
	ClientID   string `json:"c,omitempty"`  // 仅隧道：归属客户端 ID
	ClientName string `json:"cn,omitempty"` // 仅隧道：归属客户端名称
}

// TrafficLedger 服务级流量台账：
//   - days：服务级按自然日累计（用于累计/当月/上月/按日/按月统计与图表）
//   - tunnels / clients：按隧道 / 客户端维度的按自然日累计（用于「谁跑的流量」归因）
//   - meta：实体展示信息（名称/协议/归属），落盘持久化，重启/改名/删除后不丢失
//
// 落盘到 traffic.json（与配置文件同目录），重启后历史不丢。
type TrafficLedger struct {
	mu      sync.Mutex
	path    string
	days    map[string]dayRec             // date(2006-01-02) -> 总计
	tunnels map[string]map[string]dayRec  // tunnelID -> date -> 该隧道
	clients map[string]map[string]dayRec  // clientID -> date -> 该客户端
	meta    map[string]metaRec            // id -> 展示信息
	dirty   bool
}

// ledgerFile 落盘结构（顶层对象，兼容旧版「整文件即 map[date]dayRec」）
type ledgerFile struct {
	Days    map[string]dayRec            `json:"days"`
	Tunnels map[string]map[string]dayRec `json:"tunnels"`
	Clients map[string]map[string]dayRec `json:"clients"`
	Meta    map[string]metaRec           `json:"meta"`
}

// newLedger 创建台账。path 为空时仅内存（不持久化）。
func newLedger(cfgPath string) *TrafficLedger {
	l := &TrafficLedger{
		days:    map[string]dayRec{},
		tunnels: map[string]map[string]dayRec{},
		clients: map[string]map[string]dayRec{},
		meta:    map[string]metaRec{},
	}
	if cfgPath != "" {
		l.path = filepath.Join(filepath.Dir(cfgPath), "traffic.json")
	}
	return l
}

// load 从磁盘加载历史台账（兼容旧格式：整文件即 map[date]dayRec）
func (l *TrafficLedger) load() {
	if l == nil || l.path == "" {
		return
	}
	data, err := os.ReadFile(l.path)
	if err != nil {
		return
	}
	var lf ledgerFile
	if json.Unmarshal(data, &lf) == nil {
		if lf.Days != nil {
			l.days = lf.Days
		}
		if lf.Tunnels != nil {
			l.tunnels = lf.Tunnels
		}
		if lf.Clients != nil {
			l.clients = lf.Clients
		}
		if lf.Meta != nil {
			l.meta = lf.Meta
		}
	}
	// 兼容旧版：顶层直接是 map[date]dayRec（无 days 字段）
	if l.days == nil {
		var old map[string]dayRec
		if json.Unmarshal(data, &old) == nil && len(old) > 0 {
			l.days = old
		}
	}
}

func (l *TrafficLedger) addDay(m map[string]dayRec, key string, up, down int64) {
	r := m[key]
	r.Up += up
	r.Down += down
	m[key] = r
}

// AddTotal 累加服务级某时刻的上行/下流量（来自 sampler 的每秒增量）
func (l *TrafficLedger) AddTotal(up, down int64, t time.Time) {
	if l == nil {
		return
	}
	if up <= 0 && down <= 0 {
		return
	}
	l.mu.Lock()
	key := t.Format("2006-01-02")
	l.addDay(l.days, key, up, down)
	l.dirty = true
	l.mu.Unlock()
}

// AddTunnel 将某隧道某时刻的增量记入该隧道维度，并刷新展示信息
func (l *TrafficLedger) AddTunnel(id, name, proto, clientID, clientName string, up, down int64, t time.Time) {
	if l == nil || id == "" {
		return
	}
	if up <= 0 && down <= 0 {
		return
	}
	l.mu.Lock()
	key := t.Format("2006-01-02")
	if l.tunnels[id] == nil {
		l.tunnels[id] = map[string]dayRec{}
	}
	l.addDay(l.tunnels[id], key, up, down)
	l.meta[id] = metaRec{Name: name, Proto: proto, ClientID: clientID, ClientName: clientName}
	l.dirty = true
	l.mu.Unlock()
}

// AddClient 将某客户端某时刻的增量记入该客户端维度，并刷新展示信息
func (l *TrafficLedger) AddClient(id, name string, up, down int64, t time.Time) {
	if l == nil || id == "" {
		return
	}
	if up <= 0 && down <= 0 {
		return
	}
	l.mu.Lock()
	key := t.Format("2006-01-02")
	if l.clients[id] == nil {
		l.clients[id] = map[string]dayRec{}
	}
	l.addDay(l.clients[id], key, up, down)
	if m := l.meta[id]; m.Name == "" {
		l.meta[id] = metaRec{Name: name}
	}
	l.dirty = true
	l.mu.Unlock()
}

// save 若发生过变更则落盘；path 为空时跳过
func (l *TrafficLedger) save() {
	if l == nil || l.path == "" {
		return
	}
	l.mu.Lock()
	if !l.dirty {
		l.mu.Unlock()
		return
	}
	lf := ledgerFile{Days: l.days, Tunnels: l.tunnels, Clients: l.clients, Meta: l.meta}
	data, err := json.MarshalIndent(lf, "", "  ")
	if err == nil {
		_ = os.WriteFile(l.path, data, 0644)
		l.dirty = false
	}
	l.mu.Unlock()
}

// trafficReport 流量汇总查询结果
type trafficReport struct {
	Granularity string        `json:"granularity"`
	From        string        `json:"from"`
	To          string        `json:"to"`
	TotalUp     int64         `json:"total_up"`
	TotalDown   int64         `json:"total_down"`
	CurMonthUp  int64         `json:"cur_month_up"`
	CurMonthDn  int64         `json:"cur_month_down"`
	LastMonthUp int64         `json:"last_month_up"`
	LastMonthDn int64         `json:"last_month_down"`
	PeriodUp    int64         `json:"period_up"`
	PeriodDn    int64         `json:"period_down"`
	Rows        []trafficRow  `json:"rows"`
}

type trafficRow struct {
	Key   string `json:"key"`
	Up    int64  `json:"up"`
	Down  int64  `json:"down"`
}

// Query 按时间段与粒度汇总流量。
// from/to 形如 2006-01-02；gran 为 "day"（默认）或 "month"。
// 返回 rows 为升序（图表用）；前端表格展示时再倒序为最新在前。
func (l *TrafficLedger) Query(from, to, gran string) trafficReport {
	rep := trafficReport{Granularity: "day", Rows: []trafficRow{}}
	if l == nil {
		return rep
	}
	loc := time.Local
	parse := func(s string) (time.Time, bool) {
		t, err := time.ParseInLocation("2006-01-02", s, loc)
		return t, err == nil
	}
	now := time.Now()
	f, okF := parse(from)
	t, okT := parse(to)
	if !okF {
		f = now.AddDate(0, 0, -29)
	}
	if !okT {
		t = now
	}
	if t.Before(f) {
		f, t = t, f
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// 区间逐日累计（period 汇总始终按天计算，与粒度无关）
	var periodUp, periodDown int64
	type mr struct{ up, down int64 }
	mm := map[string]*mr{}
	var ks []string
	for d := f; !d.After(t); d = d.AddDate(0, 0, 1) {
		k := d.Format("2006-01-02")
		rec := l.days[k]
		periodUp += rec.Up
		periodDown += rec.Down
		if gran == "month" {
			mk := d.Format("2006-01")
			if mm[mk] == nil {
				mm[mk] = &mr{}
				ks = append(ks, mk)
			}
			mm[mk].up += rec.Up
			mm[mk].down += rec.Down
		} else {
			rep.Rows = append(rep.Rows, trafficRow{Key: k, Up: rec.Up, Down: rec.Down})
		}
	}
	if gran == "month" {
		sort.Strings(ks)
		for _, k := range ks {
			rep.Rows = append(rep.Rows, trafficRow{Key: k, Up: mm[k].up, Down: mm[k].down})
		}
		rep.Granularity = "month"
	}

	// 累计（全部历史）
	var totalUp, totalDown int64
	// 当月 / 上月（自然月，与查询区间无关）
	curY, curM, _ := now.Date()
	curStart := time.Date(curY, curM, 1, 0, 0, 0, 0, loc)
	nextMonth := time.Date(curY, curM+1, 1, 0, 0, 0, 0, loc)
	lastStart := curStart.AddDate(0, -1, 0)
	var curUp, curDown, lastUp, lastDown int64
	for k, rec := range l.days {
		totalUp += rec.Up
		totalDown += rec.Down
		kt, err := time.ParseInLocation("2006-01-02", k, loc)
		if err != nil {
			continue
		}
		if (kt.Equal(curStart) || kt.After(curStart)) && kt.Before(nextMonth) {
			curUp += rec.Up
			curDown += rec.Down
		}
		if (kt.Equal(lastStart) || kt.After(lastStart)) && kt.Before(curStart) {
			lastUp += rec.Up
			lastDown += rec.Down
		}
	}

	rep.From = f.Format("2006-01-02")
	rep.To = t.Format("2006-01-02")
	rep.TotalUp = totalUp
	rep.TotalDown = totalDown
	rep.CurMonthUp = curUp
	rep.CurMonthDn = curDown
	rep.LastMonthUp = lastUp
	rep.LastMonthDn = lastDown
	rep.PeriodUp = periodUp
	rep.PeriodDn = periodDown
	return rep
}

// trafficItem 单条隧道/客户端的区间流量
type trafficItem struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Proto      string `json:"proto,omitempty"`  // 仅隧道
	ClientID   string `json:"client_id,omitempty"` // 仅隧道：归属客户端
	ClientName string `json:"client_name,omitempty"`
	Up         int64  `json:"up"`
	Down       int64  `json:"down"`
	Total      int64  `json:"total"`
}

// trafficBreakdown 「谁跑的流量」归因结果
type trafficBreakdown struct {
	By    string        `json:"by"` // "tunnel" | "client"
	From  string        `json:"from"`
	To    string        `json:"to"`
	Items []trafficItem `json:"items"`
}

// Breakdown 按隧道或客户端维度，对给定时间段做累加与排序（按区间总流量降序）。
func (l *TrafficLedger) Breakdown(from, to, by string) trafficBreakdown {
	rep := trafficBreakdown{By: by, Items: []trafficItem{}}
	if l == nil {
		return rep
	}
	loc := time.Local
	parse := func(s string) (time.Time, bool) {
		t, err := time.ParseInLocation("2006-01-02", s, loc)
		return t, err == nil
	}
	now := time.Now()
	f, okF := parse(from)
	t, okT := parse(to)
	if !okF {
		f = now.AddDate(0, 0, -29)
	}
	if !okT {
		t = now
	}
	if t.Before(f) {
		f, t = t, f
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	src := l.clients
	if by != "client" {
		src = l.tunnels
	}
	for id, byDay := range src {
		var up, down int64
		for d := f; !d.After(t); d = d.AddDate(0, 0, 1) {
			k := d.Format("2006-01-02")
			rec := byDay[k]
			up += rec.Up
			down += rec.Down
		}
		if up == 0 && down == 0 {
			continue
		}
		m := l.meta[id]
		name := m.Name
		if name == "" {
			name = id
		}
		it := trafficItem{ID: id, Name: name, Up: up, Down: down, Total: up + down}
		if by != "client" {
			it.Proto = m.Proto
			it.ClientID = m.ClientID
			it.ClientName = m.ClientName
		}
		rep.Items = append(rep.Items, it)
	}
	sort.Slice(rep.Items, func(i, j int) bool {
		if rep.Items[i].Total != rep.Items[j].Total {
			return rep.Items[i].Total > rep.Items[j].Total
		}
		return rep.Items[i].ID < rep.Items[j].ID
	})
	rep.From = f.Format("2006-01-02")
	rep.To = t.Format("2006-01-02")
	return rep
}
