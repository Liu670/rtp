// Package core 提供 rtp 服务端与客户端共享的基础设施：
// 行协议、隧道/客户端/配置结构、限速、字节搬运、流量时序与事件日志。
package core

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Msg 是控制/工作连接上的行协议：每行为一行 JSON + '\n'
type Msg struct {
	Type      string         `json:"type"`
	Token     string         `json:"token,omitempty"`
	ClientID  string         `json:"client_id,omitempty"`
	Ok        bool           `json:"ok,omitempty"`
	Message   string         `json:"message,omitempty"`
	ConnID    string         `json:"conn_id,omitempty"`
	TunnelID  string         `json:"tunnel_id,omitempty"`
	WorkPort  int            `json:"work_port,omitempty"`
	Tunnels   []TunnelConfig `json:"tunnels,omitempty"`
	Tunnel    *TunnelConfig  `json:"tunnel,omitempty"`
	Host      string         `json:"host,omitempty"`  // http host / https sni
	Data      []byte         `json:"data,omitempty"`  // udp payload (json base64)
	From      string         `json:"from,omitempty"`  // udp peer addr
	ServerID  string         `json:"server_id,omitempty"`
	// P2P 相关
	P2pPort      int    `json:"p2p_port,omitempty"`      // 服务端 P2P 信令 UDP 端口
	Role         string `json:"role,omitempty"`          // p2p_offer: target / visitor
	PeerAddr     string `json:"peer_addr,omitempty"`     // 对端 NAT 候选地址
	VisitorPort  int    `json:"visitor_port,omitempty"`  // visitor 本地监听端口
	Session      string `json:"session,omitempty"`       // p2p 会话 ID
	TunnelID2    string `json:"tunnel_id2,omitempty"`     // relay 回退用的第二个隧道 id
}

// TunnelConfig 一条隧道规则。proto: tcp / udp / http / https / stcp
type TunnelConfig struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Proto      string `json:"proto"`
	ClientID   string `json:"client_id"` // 归属客户端
	LocalHost  string `json:"local_host"`
	LocalPort  int    `json:"local_port"`
	PublicPort int    `json:"public_port"` // tcp/udp/stcp 使用
	Domain     string `json:"domain"`      // http/https 路由主机名
	Subdomain  string `json:"subdomain"`
	Secret     string `json:"secret"` // stcp 访问密钥
	Enabled    bool   `json:"enabled"`
	Note       string `json:"note"`
	// P2P 隧道（proto=p2p）：target=当前 ClientID 提供内网服务，visitor=另一客户端作为访问端
	VisitorClientID string `json:"visitor_client_id"`
	VisitorPort     int    `json:"visitor_port"` // visitor 本地监听端口
	P2PMode         string `json:"p2p_mode,omitempty"` // direct / relay / punching / waiting（运行时状态）
}

// ClientConfig 一个客户端（对应 nps 的每设备 vkey 概念）
type ClientConfig struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Token     string `json:"token"` // 客户端接入令牌（独立于 admin_token）
	Note      string `json:"note"`
	CreatedAt int64  `json:"created_at"`
	BwLimit   int    `json:"bw_limit_mbps"` // 0 = 不限速
}

// Config 服务端持久化配置
type Config struct {
	AdminToken  string         `json:"admin_token"`
	ControlPort int            `json:"control_port"`
	WorkPort    int            `json:"work_port"`
	WebPort     int            `json:"web_port"`
	HttpPort    int            `json:"http_port"`
	HttpsPort   int            `json:"https_port"`
	P2pPort     int            `json:"p2p_port"`
	BaseDomain  string         `json:"base_domain"`
	AdminUser     string `json:"admin_user"`      // 管理后台账号
	AdminSalt     string `json:"admin_salt"`      // 密码盐
	AdminPassHash string `json:"admin_pass_hash"` // sha256(salt:pass)
	Announcement  string `json:"announcement"`   // 登录页/控制台公告
	Clients     []ClientConfig `json:"clients"`
	Tunnels     []TunnelConfig `json:"tunnels"`
}

func ReadMsg(r *bufio.Reader) (Msg, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return Msg{}, err
	}
	var m Msg
	if err := json.Unmarshal(line, &m); err != nil {
		return Msg{}, err
	}
	return m, nil
}

func WriteMsg(w *bufio.Writer, m Msg) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if _, err := w.Write(append(b, '\n')); err != nil {
		return err
	}
	return w.Flush()
}

func RandID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Throttle 简单令牌桶限速（按字节/秒）。rate<=0 表示不限速。
type Throttle struct {
	mu        sync.Mutex
	rate      int64 // 当前速率（原子读取）
	allowance int64
	last      time.Time
}

func NewThrottle(rate int64) *Throttle {
	return &Throttle{rate: rate, last: time.Now()}
}

func (t *Throttle) SetRate(rate int64) { atomic.StoreInt64(&t.rate, rate) }

func (t *Throttle) maybe(n int64) {
	rate := atomic.LoadInt64(&t.rate)
	if rate <= 0 {
		return
	}
	t.mu.Lock()
	now := time.Now()
	elapsed := now.Sub(t.last).Seconds()
	t.allowance += int64(elapsed * float64(rate))
	if t.allowance > rate {
		t.allowance = rate
	}
	t.last = now
	if t.allowance >= n {
		t.allowance -= n
		t.mu.Unlock()
		return
	}
	deficit := n - t.allowance
	t.allowance = 0
	t.mu.Unlock()
	time.Sleep(time.Duration(float64(deficit) / float64(rate) * float64(time.Second)))
}

// Pump 在两个连接之间双向搬运字节；counter 非 nil 时累加流量；th 非 nil 时按方向限速
func Pump(dst net.Conn, src io.Reader, counter *int64, th *Throttle) {
	buf := make([]byte, 32*1024)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if th != nil {
				th.maybe(int64(n))
			}
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return
			}
			if counter != nil {
				atomic.AddInt64(counter, int64(n))
			}
		}
		if err != nil {
			return
		}
	}
}

func CloseBoth(a, b io.Closer) {
	a.Close()
	b.Close()
}

// 流量时序：累计值 + 每分钟一个桶，保留最近 bucketMinutes 个桶
const bucketMinutes = 180

type Bucket struct {
	Ts   int64 `json:"ts"`   // 该分钟的时间戳（秒）
	Up   int64 `json:"up"`   // 上行字节（内网 -> 用户）
	Down int64 `json:"down"` // 下行字节（用户 -> 内网）
}

type Traffic struct {
	mu      sync.Mutex
	cUp     int64
	cDown   int64
	buckets []Bucket
}

func NewTraffic() *Traffic {
	return &Traffic{buckets: make([]Bucket, 0, bucketMinutes)}
}

func (t *Traffic) Add(up, down int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cUp += up
	t.cDown += down
	now := time.Now().Unix()
	if len(t.buckets) == 0 || t.buckets[len(t.buckets)-1].Ts != now {
		t.buckets = append(t.buckets, Bucket{Ts: now})
		if len(t.buckets) > bucketMinutes {
			t.buckets = t.buckets[len(t.buckets)-bucketMinutes:]
		}
	}
	b := &t.buckets[len(t.buckets)-1]
	b.Up += up
	b.Down += down
}

// snapshot 返回累计值与最近若干分钟时序（用于绘图）
func (t *Traffic) Snapshot() (up, down int64, series []Bucket) {
	t.mu.Lock()
	defer t.mu.Unlock()
	series = append([]Bucket(nil), t.buckets...)
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

func LogEvent(level, msg string) {
	events.mu.Lock()
	events.items = append(events.items, Event{Ts: time.Now().Unix(), Level: level, Msg: msg})
	if len(events.items) > 300 {
		events.items = events.items[len(events.items)-300:]
	}
	events.mu.Unlock()
}

func RecentEvents() []Event {
	events.mu.Lock()
	defer events.mu.Unlock()
	out := append([]Event(nil), events.items...)
	return out
}
