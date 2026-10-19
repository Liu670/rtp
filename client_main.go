//go:build client

package main

import (
	"flag"
	"log"
	"strings"
)

func main() {
	var (
		server = flag.String("server", "127.0.0.1:7000", "服务端地址 host[:port]，端口缺省 7000")
		token  = flag.String("token", "", "管理员 token（与服务端一致）")
		name   = flag.String("name", "home-pc", "客户端名称")
	)
	flag.Parse()
	if *token == "" {
		log.Fatal("必须通过 -token 指定与管理端一致的 token")
	}
	host := *server
	addr := *server
	if !strings.Contains(host, ":") {
		addr = host + ":7000"
	} else {
		host = strings.SplitN(*server, ":", 2)[0]
	}
	c := &Client{serverHost: host, serverAddr: addr, token: *token, name: *name, tunnels: map[string]TunnelConfig{}}
	c.Run()
}
