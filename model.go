package main

import (
	"sync"
	"time"
)

// 流量时序：累计值 + 每分钟一个桶，保留最近 bucketMinutes 个桶
const bucketMinutes = 180

type tBucket struct {
	Ts   int64 `json:"ts"`   // 该分钟的时间戳（秒）
	Up   int64 `json:"up"`   // 上行字节（内网 -> 用户）
	Down int64 `json:"down"` // 下行字节（用户 -> 内网）
}

type Traffic struct {
	mu      sync.Mutex
	cUp     int64
	cDown   int64
	buckets []tBucket
}

func newTraffic() *Traffic {
	return &Traffic{buckets: make([]tBucket, 0, bucketMinutes)}
}

func (t *Traffic) add(up, down int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cUp += up
	t.cDown += down
	now := time.Now().Unix()
	if len(t.buckets) == 0 || t.buckets[len(t.buckets)-1].Ts != now {
		t.buckets = append(t.buckets, tBucket{Ts: now})
		if len(t.buckets) > bucketMinutes {
			t.buckets = t.buckets[len(t.buckets)-bucketMinutes:]
		}
	}
	b := &t.buckets[len(t.buckets)-1]
	b.Up += up
	b.Down += down
}

// snapshot 返回累计值与最近若干分钟时序（用于绘图）
func (t *Traffic) snapshot() (up, down int64, series []tBucket) {
	t.mu.Lock()
	defer t.mu.Unlock()
	series = append([]tBucket(nil), t.buckets...)
	return t.cUp, t.cDown, series
}

// Event 连接/系统事件，用于前端事件流
type Event struct {
	Ts    int64  `json:"ts"`
	Level string `json:"level"` // info / warn / ok
	Msg   string `json:"msg"`
}

type eventLog struct {
	mu    sync.Mutex
	items []Event
}

var events = &eventLog{}

func logEvent(level, msg string) {
	events.mu.Lock()
	events.items = append(events.items, Event{Ts: time.Now().Unix(), Level: level, Msg: msg})
	if len(events.items) > 300 {
		events.items = events.items[len(events.items)-300:]
	}
	events.mu.Unlock()
}

func recentEvents() []Event {
	events.mu.Lock()
	defer events.mu.Unlock()
	out := append([]Event(nil), events.items...)
	return out
}
