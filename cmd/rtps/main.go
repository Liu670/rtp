package main

import (
	"github.com/Liu670/rtp/internal/core"
	"flag"
	"log"
	"os"
)

// guiAvailable 报告当前平台是否提供图形界面（仅 Windows）。
var guiAvailable bool

func main() {
	var (
		cfgPath     = flag.String("config", "config.json", "配置文件路径")
		controlPort = flag.Int("control", 0, "控制端口(覆盖配置)")
		workPort    = flag.Int("work", 0, "工作端口(覆盖配置)")
		webPort     = flag.Int("web", 0, "Web 端口(覆盖配置)")
		token       = flag.String("token", "", "管理员 token(覆盖配置)")
		cliMode     = flag.Bool("cli", false, "命令行模式（不启动图形界面，适合服务器 / 无桌面环境）")
	)
	flag.Parse()

	// Windows 下默认启动图形界面；传入 -cli 或非 Windows 则走命令行模式。
	if guiAvailable && !*cliMode {
		if hasNoFlags() {
			runGUI()
			return
		}
	}

	cfg := loadConfig(*cfgPath)
	if *token != "" {
		cfg.AdminToken = *token
	}
	if *controlPort != 0 {
		cfg.ControlPort = *controlPort
	}
	if *workPort != 0 {
		cfg.WorkPort = *workPort
	}
	if *webPort != 0 {
		cfg.WebPort = *webPort
	}
	if cfg.AdminToken == "" {
		cfg.AdminToken = core.RandID()
	}

	s := NewServer(cfg, *cfgPath)
	s.saveConfig()
	log.Printf("管理员 token: %s", cfg.AdminToken)
	log.Printf("控制端口: %d  工作端口: %d  Web端口: %d", cfg.ControlPort, cfg.WorkPort, cfg.WebPort)
	log.Printf("Web 控制台: http://<本机IP>:%d/", cfg.WebPort)
	s.Run()
}

// hasNoFlags 判断用户是否未显式传入参数（未传时 Windows 才弹图形界面）。
func hasNoFlags() bool {
	for _, a := range os.Args[1:] {
		if len(a) > 1 && a[0] == '-' {
			return false
		}
	}
	return true
}