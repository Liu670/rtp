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

// TrafficLedger 服务级流量台账：按自然日累计上行/下行，落盘到 traffic.json，
// 重启后历史不丢失。用于「累计 / 当月 / 上月 / 按日 / 按月」统计与图表展示。
type TrafficLedger struct {
	mu     sync.Mutex
	path   string
	days   map[string]dayRec // key: 2006-01-02
	dirty  bool
}

// newLedger 创建台账。path 为空时仅内存（不持久化）。
func newLedger(cfgPath string) *TrafficLedger {
	l := &TrafficLedger{days: map[string]dayRec{}}
	if cfgPath != "" {
		l.path = filepath.Join(filepath.Dir(cfgPath), "traffic.json")
	}
	return l
}

// load 从磁盘加载历史台账
func (l *TrafficLedger) load() {
	if l == nil || l.path == "" {
		return
	}
	data, err := os.ReadFile(l.path)
	if err != nil {
		return
	}
	var tmp map[string]dayRec
	if json.Unmarshal(data, &tmp) == nil && tmp != nil {
		l.days = tmp
	}
}

// Add 累加某时刻的上行/下流量（来自 sampler 的每秒增量）
func (l *TrafficLedger) Add(up, down int64, t time.Time) {
	if l == nil {
		return
	}
	if up <= 0 && down <= 0 {
		return
	}
	l.mu.Lock()
	key := t.Format("2006-01-02")
	rec := l.days[key]
	rec.Up += up
	rec.Down += down
	l.days[key] = rec
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
	data, err := json.MarshalIndent(l.days, "", "  ")
	if err == nil {
		_ = os.WriteFile(l.path, data, 0644)
		l.dirty = false
	}
	l.mu.Unlock()
}

// Report 流量汇总查询结果
type trafficReport struct {
	Granularity string         `json:"granularity"`
	From        string         `json:"from"`
	To          string         `json:"to"`
	TotalUp     int64          `json:"total_up"`
	TotalDown   int64          `json:"total_down"`
	CurMonthUp  int64          `json:"cur_month_up"`
	CurMonthDn  int64          `json:"cur_month_down"`
	LastMonthUp int64          `json:"last_month_up"`
	LastMonthDn int64          `json:"last_month_down"`
	PeriodUp    int64          `json:"period_up"`
	PeriodDn    int64          `json:"period_down"`
	Rows        []trafficRow   `json:"rows"`
}

type trafficRow struct {
	Key   string `json:"key"`
	Up    int64  `json:"up"`
	Down  int64  `json:"down"`
}

// Query 按时间段与粒度汇总流量。
// from/to 形如 2006-01-02；gran 为 "day"（默认）或 "month"。
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
