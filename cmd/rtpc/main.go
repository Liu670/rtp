package main

import (
	"flag"
	"log"
	"os"
	"strings"
)

// guiAvailable 报告当前平台是否提供桌面界面（仅 Windows）。
var guiAvailable bool

func main() {
	var (
		server  = flag.String("server", "", "服务端地址 host[:port]，端口缺省 7000")
		token   = flag.String("token", "", "客户端令牌（服务端控制台「客户端」页复制）")
		name    = flag.String("name", "windows-pc", "客户端名称")
		cliMode = flag.Bool("cli", false, "命令行模式（不启动桌面界面）")
	)
	flag.Parse()

	// Windows 下未传参数时启动桌面界面；传了 -server/-token 或 -cli 则走命令行。
	if guiAvailable && !*cliMode && *token == "" {
		runGUI()
		return
	}

	if *token == "" {
		log.Fatal("必须通过 -token 指定客户端令牌（服务端控制台「客户端」页复制）")
	}
	host := *server
	addr := *server
	if addr == "" {
		addr = "127.0.0.1:7000"
	}
	if !strings.Contains(addr, ":") {
		addr = addr + ":7000"
	}
	if host == "" {
		host = strings.SplitN(addr, ":", 2)[0]
	} else if !strings.Contains(host, ":") {
		host = strings.SplitN(*server, ":", 2)[0]
	}

	c := &Client{
		serverHost: host,
		serverAddr: addr,
		token:      *token,
		name:       *name,
		tunnels:    map[string]TunnelConfig{},
	}
	log.Printf("[client] 连接 %s（名称 %s）", addr, *name)
	c.Run()
}

// clientNameFromEnv 允许通过 RTP_NAME 环境变量覆盖名称（供 systemd / 计划任务使用）。
func clientNameFromEnv(def string) string {
	if v := os.Getenv("RTP_NAME"); v != "" {
		return v
	}
	return def
}