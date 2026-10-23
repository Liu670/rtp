package main

import (
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// 代理网关实时统计：连接明细、按来源 IP 汇总、累计出入流量。
// 内存态，服务重启后清零；UDP ASSOCIATE 中继流量暂不计入（TCP 已覆盖绝大多数场景）。
//
// 口径约定：
//   入口 Up   = 客户端 → 网关（请求方向）
//   出口 Down = 网关 → 客户端（响应方向）

type pxConn struct {
	ID     string `json:"id"`
	Client string `json:"client"` // 来源 IP:port
	Target string `json:"target"` // 目标 host:port
	Proto  string `json:"proto"`  // socks5 / http
	Start  int64  `json:"start"`  // 建立时间（秒）
	Up     int64  `json:"up"`     // 原子累加（core.Pump 计数）
	Down   int64  `json:"down"`
}

type pxIPStat struct {
	IP    string `json:"ip"`
	Conns int64  `json:"conns"` // 累计连接数
	Up    int64  `json:"up"`    // 已结算流量（连接关闭时累加）
	Down  int64  `json:"down"`
	Last  int64  `json:"last"` // 最近活动时间
}

type proxyStats struct {
	mu        sync.Mutex
	seq       int64
	conns     map[string]*pxConn
	perIP     map[string]*pxIPStat
	totalUp   int64
	totalDown int64
	peak      int
}

func newProxyStats() *proxyStats {
	return &proxyStats{conns: map[string]*pxConn{}, perIP: map[string]*pxIPStat{}}
}

func hostOnly(addr string) string {
	if h, _, err := net.SplitHostPort(addr); err == nil {
		return h
	}
	return addr
}

// openConn 登记一条代理连接，返回连接 ID 及两路计数指针（交给 core.Pump 原子累加）。
func (p *proxyStats) openConn(clientIP, target, proto string) (string, *int64, *int64) {
	seq := atomic.AddInt64(&p.seq, 1)
	id := "px" + strconv.FormatInt(seq, 10)
	c := &pxConn{
		ID: id, Client: clientIP, Target: target, Proto: proto,
		Start: time.Now().Unix(),
	}
	p.mu.Lock()
	p.conns[id] = c
	ip := hostOnly(clientIP)
	st := p.perIP[ip]
	if st == nil {
		st = &pxIPStat{IP: ip}
		p.perIP[ip] = st
	}
	st.Conns++
	st.Last = c.Start
	if n := len(p.conns); n > p.peak {
		p.peak = n
	}
	p.mu.Unlock()
	return id, &c.Up, &c.Down
}

// closeConn 连接结束时结算流量并从活动列表移除
func (p *proxyStats) closeConn(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	c := p.conns[id]
	if c == nil {
		return
	}
	delete(p.conns, id)
	up := atomic.LoadInt64(&c.Up)
	down := atomic.LoadInt64(&c.Down)
	p.totalUp += up
	p.totalDown += down
	st := p.perIP[hostOnly(c.Client)]
	if st != nil {
		st.Up += up
		st.Down += down
		st.Last = time.Now().Unix()
	}
}

type pxIPRow struct {
	IP    string `json:"ip"`
	Conns int64  `json:"conns"`
	Up    int64  `json:"up"`
	Down  int64  `json:"down"`
	Last  int64  `json:"last"`
}

// snapshot 生成给 Web API 的完整快照：活动连接 + 按来源 IP 汇总（含活动连接实时流量）
func (p *proxyStats) snapshot() map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()

	conns := make([]pxConn, 0, len(p.conns))
	// 活动连接的实时流量并入按 IP 视图
	live := map[string]*pxIPRow{}
	for _, c := range p.conns {
		cc := *c
		cc.Up = atomic.LoadInt64(&c.Up)
		cc.Down = atomic.LoadInt64(&c.Down)
		conns = append(conns, cc)
		ip := hostOnly(c.Client)
		r := live[ip]
		if r == nil {
			r = &pxIPRow{IP: ip}
			live[ip] = r
		}
		r.Conns++
		if c.Start > r.Last {
			r.Last = c.Start
		}
	}

	ips := make([]pxIPRow, 0, len(p.perIP))
	for _, st := range p.perIP {
		row := pxIPRow{IP: st.IP, Conns: st.Conns, Up: st.Up, Down: st.Down, Last: st.Last}
		if lr, ok := live[st.IP]; ok {
			row.Conns += lr.Conns
			row.Up += lr.Up
			row.Down += lr.Down
			if lr.Last > row.Last {
				row.Last = lr.Last
			}
		}
		ips = append(ips, row)
	}
	return map[string]any{
		"conns":      conns,
		"per_ip":     ips,
		"total_up":   p.totalUp,
		"total_down": p.totalDown,
		"cur":        len(p.conns),
		"peak":       p.peak,
	}
}

// countWriter 在写入底层Writer的同时原子累加字节数（用于 HTTP 代理请求方向计数）
type countWriter struct {
	w io.Writer
	n *int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	atomic.AddInt64(c.n, int64(len(p)))
	return c.w.Write(p)
}
