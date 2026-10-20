package main

import (
	"github.com/Liu670/rtp/internal/core"
	"flag"
	"log"
)

func main() {
	var (
		cfgPath     = flag.String("config", "config.json", "配置文件路径")
		controlPort = flag.Int("control", 0, "控制端口(覆盖配置)")
		workPort    = flag.Int("work", 0, "工作端口(覆盖配置)")
		webPort     = flag.Int("web", 0, "Web 端口(覆盖配置)")
		token       = flag.String("token", "", "管理员 token(覆盖配置)")
	)
	flag.Parse()

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
	s.Run()
}
