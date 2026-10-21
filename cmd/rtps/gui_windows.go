//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Liu670/rtp/internal/winui"
)

// 控件 ID
const (
	idCfgPath = iota + 100
	idWebPort
	idCtlPort
	idWorkPort
	idToken
	idStatus
	idBtnStart
	idBtnStop
	idBtnBrowser
	idBtnOpenDir
	idBtnCopyToken
	idBtnSave
	idLog
)

const (
	idMenuBrowser = 500
	idMenuDir     = 501
	idMenuQuit    = 502
)

var (
	guiApp    *winui.App
	guiServer *Server
	guiMu     sync.Mutex
	guiCfg    *Config
)

// runGUI 是 rtps 在 Windows 上的桌面界面入口。
// GUI 只负责「配置 + 启停 + 一键打开 Web 控制台」，所有隧道/客户端管理仍由
// Web 控制台完成（服务端与 Web 共用同一套 HTTP 接口，避免两套逻辑分叉）。
func runGUI() {
	// 桌面程序以 -H windowsgui 编译（无控制台），任何 panic 都会表现为「双击无反应」。
	// 这里统一兜底：弹窗提示并把堆栈写入 rtp.crash.log，便于定位。
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 8192)
			n := runtime.Stack(buf, false)
			m := fmt.Sprintf("rtp 桌面端启动失败：\n%v\n\n%s", r, string(buf[:n]))
			winui.CrashLog("rtp.crash.log", m)
			winui.Fatal("rtp 启动失败", m)
		}
	}()
	cfgPath := "config.json"
	if len(os.Args) > 2 && os.Args[1] == "-config" {
		cfgPath = os.Args[2]
	}
	cfg := loadConfig(cfgPath)
	guiCfg = &cfg

	app := winui.New("rtp 内网穿透服务端", 560, 520)
	guiApp = app

	app.Label(0, "rtp 内网穿透服务端  ·  Windows 桌面界面", 20, 14, 520, 22)
	app.Label(0, "在这里配置端口并启停服务；隧道与客户端管理请使用 Web 控制台。", 20, 38, 520, 20)

	y := 68
	app.Label(idCfgPath, "配置文件", 20, y+2, 100, 20)
	app.Input(idCfgPath+1, cfgPath, 120, y, 230, 24)
	app.Button(idBtnOpenDir, "打开目录", 356, y, 84, 24)
	app.Button(idBtnSave, "保存配置", 446, y, 84, 24)
	y += 34

	app.Label(idWebPort, "Web 控制台端口", 20, y+2, 100, 20)
	app.Input(idWebPort+1, strconv.Itoa(cfg.WebPort), 120, y, 230, 24)
	y += 32

	app.Label(idCtlPort, "隧道控制端口", 20, y+2, 100, 20)
	app.Input(idCtlPort+1, strconv.Itoa(cfg.ControlPort), 120, y, 230, 24)
	y += 32

	app.Label(idWorkPort, "工作端口", 20, y+2, 100, 20)
	app.Input(idWorkPort+1, strconv.Itoa(cfg.WorkPort), 120, y, 230, 24)
	y += 32

	app.Label(idToken, "管理员令牌", 20, y+2, 100, 20)
	tok := cfg.AdminToken
	if tok == "" {
		tok = "(留空自动生成)"
	}
	app.Input(idToken+1, tok, 120, y, 230, 24)
	app.Button(idBtnCopyToken, "复制", 356, y, 46, 24)
	app.Button(idBtnBrowser, "Web 控制台", 408, y, 122, 24)
	y += 40

	app.DefButton(idBtnStart, "启动服务", 20, y, 100, 30)
	app.Button(idBtnStop, "停止服务", 128, y, 100, 30)
	app.Label(idStatus, "状态：未启动", 240, y+6, 300, 20)
	y += 34

	app.Label(0, "提示：点「Web 控制台」可直接在浏览器打开管理页；关闭窗口会收进托盘，右键托盘图标或任务栏可退出。", 20, y, 520, 18)
	y += 24

	app.Label(0, "运行日志", 20, y, 100, 20)
	app.Multi(idLog, "就绪。点击「启动服务」开始运行，首次启动会自动生成配置文件与管理员令牌。", 20, y+20, 520, 150)

	app.OnCmd = onCmd
	app.Tray("rtp 服务端", []winui.TrayItem{
		{Text: "打开主窗口", ID: idMenuBrowser},
		{Text: "打开 Web 控制台", ID: idMenuDir},
		{Text: "退出", ID: idMenuQuit},
	})
	app.Tick(1000, tick)
	appendLog("rtp 服务端桌面界面已就绪")

	app.Run()
}

// ---- 事件 ----

func onCmd(id int32) {
	switch id {
	case idBtnSave:
		if !saveFromUI() {
			return
		}
	case idBtnStart:
		if !saveFromUI() {
			return
		}
		startServer()
	case idBtnStop:
		stopServer()
	case idBtnBrowser, idMenuDir:
		openWeb()
	case idBtnOpenDir:
		openConfigDir()
	case idBtnCopyToken:
		copyToken()
	case idMenuBrowser:
		guiApp.Show2()
	case idMenuQuit:
		guiMu.Lock()
		if guiServer != nil {
			guiServer.Stop()
			guiServer = nil
		}
		guiMu.Unlock()
		guiApp.Quit()
	}
}

func tick() {
	guiMu.Lock()
	run := guiServer != nil
	port := guiCfg.WebPort
	guiMu.Unlock()
	if !run {
		return
	}
	winui.Post(func() {
		guiApp.SetText(idStatus, fmt.Sprintf("状态：运行中   Web: http://127.0.0.1:%d/", port))
	})
}

// ---- 逻辑 ----

func saveFromUI() bool {
	cfgPath := strings.TrimSpace(guiApp.Text(idCfgPath + 1))
	if cfgPath == "" {
		cfgPath = "config.json"
	}
	base := loadConfig(cfgPath)
	// 端口：空或非法时回退默认值，保证始终可启动（不再因「0」卡死在启动前）
	webPort := parsePort(strings.TrimSpace(guiApp.Text(idWebPort+1)), base.WebPort, "Web 控制台端口")
	ctlPort := parsePort(strings.TrimSpace(guiApp.Text(idCtlPort+1)), base.ControlPort, "隧道控制端口")
	workPort := parsePort(strings.TrimSpace(guiApp.Text(idWorkPort+1)), base.WorkPort, "工作端口")
	if webPort == ctlPort || webPort == workPort || ctlPort == workPort {
		appendLog("三个端口不能两两相同，请调整后再启动")
		return false
	}

	cfg := loadConfig(cfgPath)
	cfg.WebPort = webPort
	cfg.ControlPort = ctlPort
	cfg.WorkPort = workPort
	if t := strings.TrimSpace(guiApp.Text(idToken + 1)); t != "" && t != "(留空自动生成)" {
		cfg.AdminToken = t
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		appendLog("配置序列化失败: " + err.Error())
		return false
	}
	abs, _ := filepath.Abs(cfgPath)
	if err := os.WriteFile(abs, data, 0o600); err != nil {
		appendLog("保存失败: " + err.Error())
		return false
	}

	guiMu.Lock()
	guiCfg = &cfg
	guiMu.Unlock()
	appendLog("配置已保存: " + abs)
	return true
}

// parsePort 解析端口输入：合法返回该值；空或非法则沿用默认值并提示。
func parsePort(s string, def int, name string) int {
	if s == "" {
		return def
	}
	p, e := strconv.Atoi(s)
	if e != nil || p < 1 || p > 65535 {
		appendLog(name + " 非法（需 1-65535），已使用默认 " + strconv.Itoa(def))
		return def
	}
	return p
}

func startServer() {
	guiMu.Lock()
	if guiServer != nil {
		guiMu.Unlock()
		appendLog("服务已在运行中")
		return
	}
	cfg := guiCfg
	guiMu.Unlock()

	cfgPath := strings.TrimSpace(guiApp.Text(idCfgPath + 1))
	if cfgPath == "" {
		cfgPath = "config.json"
	}
	abs, _ := filepath.Abs(cfgPath)

	srv := NewServer(*cfg, abs)
	guiMu.Lock()
	guiServer = srv
	guiMu.Unlock()

	appendLog(fmt.Sprintf("启动中...  控制端口=%d  工作端口=%d  Web端口=%d  P2P端口=%d",
		cfg.ControlPort, cfg.WorkPort, cfg.WebPort, cfg.P2pPort))
	appendLog("管理员令牌: " + cfg.AdminToken)

	go srv.Run()

	time.Sleep(700 * time.Millisecond)
	guiApp.Toast("rtp 服务端 - 运行中")
	appendLog("服务已启动，点击「Web 控制台」开始配置客户端与隧道")
}

func stopServer() {
	guiMu.Lock()
	if guiServer == nil {
		guiMu.Unlock()
		appendLog("服务未在运行")
		return
	}
	guiServer.Stop()
	guiServer = nil
	guiMu.Unlock()
	guiApp.Toast("rtp 服务端 - 已停止")
	appendLog("服务已停止")
}

func openWeb() {
	guiMu.Lock()
	port := guiCfg.WebPort
	guiMu.Unlock()
	url := fmt.Sprintf("http://127.0.0.1:%d/", port)
	winui.OpenBrowser(url)
	guiApp.Toast("rtp 服务端 - 已打开控制台")
}

func openConfigDir() {
	cfgPath := strings.TrimSpace(guiApp.Text(idCfgPath + 1))
	abs, _ := filepath.Abs(cfgPath)
	dir := filepath.Dir(abs)
	winui.OpenBrowser("file:///" + filepath.ToSlash(dir))
	appendLog("已打开配置目录: " + dir)
}

func copyToken() {
	guiMu.Lock()
	t := guiCfg.AdminToken
	guiMu.Unlock()
	if t == "" {
		appendLog("令牌尚未生成，请先启动服务")
		return
	}
	cmd := exec.Command("clip")
	cmd.Stdin = strings.NewReader(t)
	if err := cmd.Run(); err != nil {
		appendLog("复制失败: " + err.Error())
		return
	}
	appendLog("管理员令牌已复制到剪贴板")
}

func appendLog(line string) {
	t := time.Now().Format("15:04:05")
	winui.Post(func() {
		if guiApp != nil {
			guiApp.Append(idLog, "["+t+"] "+line)
		}
	})
}