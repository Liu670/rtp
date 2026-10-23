package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Liu670/rtp/internal/core"
)

// HTTP 代理网关：与 SOCKS5 共用同一套鉴权（proxyCreds/proxyAuthOK）与统计（px）。
//   - 普通 HTTP 请求（绝对地址 URI）→ 服务端代为访问并回传响应
//   - CONNECT host:port            → HTTPS 隧道（双向透传）
// 浏览器 / SwitchyOmega / 程序内「HTTP 代理」设置均可直连使用；默认关闭，需在
// Web 控制台「代理」页启用。

func (s *Server) serveHTTPProxy(addr string) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("[httpproxy] 监听 %s 失败: %v", addr, err)
		core.LogEvent("admin", "warn", fmt.Sprintf("HTTP 代理端口 %s 监听失败: %v", addr, err))
		return
	}
	s.mu.Lock()
	s.httpLn = ln
	s.mu.Unlock()
	log.Printf("[httpproxy] HTTP 代理已开启: %s", addr)
	core.LogEvent("admin", "ok", fmt.Sprintf("HTTP 代理已开启: %s", addr))
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go s.handleHTTPProxy(c)
	}
}

func (s *Server) handleHTTPProxy(conn net.Conn) {
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	br := bufio.NewReader(conn)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}

	// 鉴权：Proxy-Authorization: Basic base64(user:pass)
	authOK := false
	if auth := req.Header.Get("Proxy-Authorization"); strings.HasPrefix(strings.ToLower(auth), "basic ") {
		if raw, e := base64.StdEncoding.DecodeString(strings.TrimSpace(auth[6:])); e == nil {
			if i := strings.IndexByte(string(raw), ':'); i >= 0 {
				authOK = s.proxyAuthOK(string(raw[:i]), string(raw[i+1:]))
			}
		}
	}
	if !authOK {
		fmt.Fprint(conn, "HTTP/1.1 407 Proxy Authentication Required\r\nProxy-Authenticate: Basic realm=\"rtp proxy\"\r\nConnection: close\r\nContent-Length: 0\r\n\r\n")
		return
	}

	clientIP := hostOnly(conn.RemoteAddr().String())

	if req.Method == http.MethodConnect {
		host := req.Host
		if host == "" {
			host = req.URL.Host
		}
		s.httpTunnel(conn, clientIP, host)
		return
	}
	if req.URL.Host == "" {
		fmt.Fprint(conn, "HTTP/1.1 400 Bad Request\r\nConnection: close\r\nContent-Length: 0\r\n\r\n")
		return
	}
	s.httpRelay(conn, clientIP, req)
}

// httpTunnel 处理 CONNECT：建立到目标的 TCP 隧道（HTTPS 等任意 TCP 流量）
func (s *Server) httpTunnel(conn net.Conn, clientIP, host string) {
	if _, _, err := net.SplitHostPort(host); err != nil {
		host = net.JoinHostPort(host, "443")
	}
	remote, err := net.DialTimeout("tcp", host, 15*time.Second)
	if err != nil {
		fmt.Fprint(conn, "HTTP/1.1 502 Bad Gateway\r\nConnection: close\r\nContent-Length: 0\r\n\r\n")
		return
	}
	defer remote.Close()
	log.Printf("[httpproxy] CONNECT %s", host)

	id, upC, downC := s.px.openConn(clientIP, host, "http")
	defer s.px.closeConn(id)

	fmt.Fprint(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
	conn.SetReadDeadline(time.Time{})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); core.Pump(remote, conn, upC, nil) }()
	go func() { defer wg.Done(); core.Pump(conn, remote, downC, nil) }()
	wg.Wait()
}

// httpRelay 处理普通 HTTP 绝对地址请求：改写为 origin-form 转发给目标服务器并回传响应
func (s *Server) httpRelay(conn net.Conn, clientIP string, req *http.Request) {
	hostPort := req.URL.Host
	if _, _, err := net.SplitHostPort(hostPort); err != nil {
		hostPort = net.JoinHostPort(hostPort, "80")
	}
	id, upC, downC := s.px.openConn(clientIP, hostPort, "http")
	defer s.px.closeConn(id)

	remote, err := net.DialTimeout("tcp", hostPort, 15*time.Second)
	if err != nil {
		fmt.Fprint(conn, "HTTP/1.1 502 Bad Gateway\r\nConnection: close\r\nContent-Length: 0\r\n\r\n")
		return
	}
	defer remote.Close()
	log.Printf("[httpproxy] %s %s", req.Method, hostPort)

	// 改写为源站形式并强制 Connection: close（简化 keep-alive 处理）
	req.Close = true
	req.RequestURI = ""
	u := *req.URL
	u.Scheme = "http"
	u.Host = hostPort
	req.URL = &u
	req.Header.Del("Proxy-Authorization")
	if _, err := req.Write(&countWriter{w: remote, n: upC}); err != nil {
		return
	}
	// 响应方向整段回传（服务端收到 Connection: close 后会主动断开，Copy 随之结束）
	core.Pump(conn, remote, downC, nil)
}
