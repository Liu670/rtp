package main

// TLS 加密的 SOCKS5 网关。
// 背景：明文 SOCKS5 跨境传输会被 GFW 识别为代理协议并动态封禁（连接被"积极拒绝"）。
// 在 TCP 之上包一层 TLS 后，外部观察到的是普通加密流，无法识别载荷，端口不再被封。
// Clash 客户端节点使用 tls: true + skip-cert-verify: true 即可连接（证书为服务端自签）。

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"log"
	"math/big"
	"net"
	"time"

	"github.com/Liu670/rtp/internal/core"
)

// selfSignedCert 生成一次性自签证书（内存态，重启重新生成）
func selfSignedCert() (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "rtp-proxy"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, nil
}

// serveSOCKS5TLS 在 addr 上以 TLS 包裹 SOCKS5 服务，复用同一套鉴权与统计。
func (s *Server) serveSOCKS5TLS(addr string) {
	cert, err := selfSignedCert()
	if err != nil {
		log.Printf("[socks5-tls] 生成自签证书失败: %v", err)
		return
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("[socks5-tls] 监听 %s 失败: %v", addr, err)
		core.LogEvent("admin", "warn", fmt.Sprintf("TLS SOCKS5 端口 %s 监听失败: %v", addr, err))
		return
	}
	tlsLn := tls.NewListener(ln, &tls.Config{Certificates: []tls.Certificate{cert}})
	s.mu.Lock()
	s.socksTLSLn = tlsLn
	s.mu.Unlock()
	log.Printf("[socks5-tls] TLS 加密网关已开启: %s", addr)
	core.LogEvent("admin", "ok", fmt.Sprintf("TLS 加密 SOCKS5 网关已开启: %s", addr))
	for {
		c, err := tlsLn.Accept()
		if err != nil {
			return
		}
		go s.handleSOCKS5(c)
	}
}
