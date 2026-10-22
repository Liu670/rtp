//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
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
// 每个标签用基值 id，对应输入框用 id+1。此前 idCfgPath+1=101 与 idWebPort=101 撞号，
// 导致 Text(idCfgPath+1) 读到标签「Web 控制台端口」而非配置路径，端口/令牌也全读错，
// 服务端因此一直「未启动」。这里按 10 步进基值，杜绝 id+1 撞号。
const (
	idCfgPath     = 100
	idWebPort     = 110
	idCtlPort     = 120
	idWorkPort    = 130
	idToken       = 140
	idStatus      = 150
	idBtnStart    = 160
	idBtnStop     = 170
	idBtnBrowser  = 180
	idBtnOpenDir  = 190
	idBtnCopyToken= 200
	idBtnSave     = 210
	idLog         = 220
	idProxyPort   = 230
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

	// 服务真实运行状态：startServer 轮询 Web 端口确认真实监听后才置 ready，
	// 失败则置 failed，避免界面误显示「运行中」或一直卡在「未启动」。
	guiServerReady  bool
	guiServerFailed bool
)

// guiLogWriter 把服务端内部 log 输出重定向到桌面端日志框，
// 这样端口被占用等错误能在界面上看到（否则 -H windowsgui 编译下错误被静默丢弃）。
type guiLogWriter struct{}

func (guiLogWriter) Write(p []byte) (int, error) {
	s := strings.TrimRight(string(p), "\n")
	for _, ln := range strings.Split(s, "\n") {
		if ln != "" {
			appendLog(ln)
		}
	}
	return len(p), nil
}

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
	// 单实例：若已有一个 GUI 在运行（收在托盘也算），把旧实例唤到前台并直接退出本进程，
	// 避免再生成第二个托盘图标 / 第二个同名进程。
	if !winui.SingleInstance("server") {
		return
	}
	cfgPath := "config.json"
	if len(os.Args) > 2 && os.Args[1] == "-config" {
		cfgPath = os.Args[2]
	}
	cfg := loadConfig(cfgPath)
	guiCfg = &cfg

	// 服务端内部日志（含端口监听失败等）重定向到界面日志框
	log.SetOutput(guiLogWriter{})

	app := winui.New("rtp 内网穿透服务端", 640, 600)
	guiApp = app

	app.Label(0, "rtp 内网穿透服务端  ·  Windows 桌面界面", 20, 14, 600, 22)
	app.Label(0, "在这里配置端口并启停服务；隧道与客户端管理请使用 Web 控制台。", 20, 40, 600, 20)

	y := 72
	app.Label(idCfgPath, "配置文件", 20, y+4, 120, 20)
	app.Input(idCfgPath+1, cfgPath, 150, y, 250, 26)
	app.Button(idBtnOpenDir, "打开目录", 408, y, 100, 26)
	app.Button(idBtnSave, "保存配置", 514, y, 100, 26)
	y += 38

	app.Label(idWebPort, "Web 控制台端口", 20, y+4, 130, 20)
	app.Input(idWebPort+1, strconv.Itoa(cfg.WebPort), 150, y, 250, 26)
	y += 36

	app.Label(idCtlPort, "隧道控制端口", 20, y+4, 130, 20)
	app.Input(idCtlPort+1, strconv.Itoa(cfg.ControlPort), 150, y, 250, 26)
	y += 36

	app.Label(idWorkPort, "工作端口", 20, y+4, 120, 20)
	app.Input(idWorkPort+1, strconv.Itoa(cfg.WorkPort), 150, y, 250, 26)
	y += 36

	app.Label(idToken, "管理员令牌", 20, y+4, 120, 20)
	tok := cfg.AdminToken
	if tok == "" {
		tok = "(留空自动生成)"
	}
	app.Input(idToken+1, tok, 150, y, 250, 26)
	app.Button(idBtnCopyToken, "复制", 408, y, 70, 26)
	app.Button(idBtnBrowser, "Web 控制台", 484, y, 130, 26)
	y += 44

	app.Label(idProxyPort, "SOCKS5 网关端口", 20, y+4, 140, 20)
	app.Input(idProxyPort+1, strconv.Itoa(cfg.ProxyPort), 150, y, 250, 26)
	app.LabelW(0, "（0=关闭；填端口后，Clash 填 本机IP:此端口+令牌 即可当局域网网关）", 408, y+4, 220, 20)
	y += 36

	app.DefButton(idBtnStart, "启动服务", 20, y, 110, 32)
	app.Button(idBtnStop, "停止服务", 138, y, 110, 32)
	app.Label(idStatus, "状态：未启动", 260, y+8, 360, 20)
	y += 42

	app.LabelW(0, "提示：点「Web 控制台」可直接在浏览器打开管理页；关闭窗口会收进托盘，右键托盘图标或任务栏可退出。", 20, y, 600, 34)
	y += 42

	app.Label(0, "运行日志", 20, y, 100, 20)
	app.Multi(idLog, "就绪。点击「启动服务」开始运行，首次启动会自动生成配置文件与管理员令牌。", 20, y+22, 600, 170)

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
	failed := guiServerFailed
	ready := guiServerReady
	port := guiCfg.WebPort
	guiMu.Unlock()
	if failed {
		// 保持「启动失败」文案，不被覆盖
		return
	}
	if ready {
		winui.Post(func() {
			if guiApp != nil {
				guiApp.SetText(idStatus, fmt.Sprintf("状态：运行中（端口 %d）", port))
			}
		})
		return
	}
	// run 为 true 但端口尚未就绪：保持「启动中...」，由轮询协程最终置 ready/failed
	_ = run
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
	proxyPort := parsePort(strings.TrimSpace(guiApp.Text(idProxyPort+1)), base.ProxyPort, "SOCKS5 网关端口")
	if webPort == ctlPort || webPort == workPort || ctlPort == workPort {
		appendLog("三个端口不能两两相同，请调整后再启动")
		return false
	}
	if proxyPort != 0 && (proxyPort == webPort || proxyPort == ctlPort || proxyPort == workPort) {
		appendLog("SOCKS5 网关端口不能与其他端口相同，请调整后再启动")
		return false
	}

	cfg := loadConfig(cfgPath)
	cfg.WebPort = webPort
	cfg.ControlPort = ctlPort
	cfg.WorkPort = workPort
	cfg.ProxyPort = proxyPort
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

	// 端口预检：被占用则直接提示，避免服务内部 log.Fatalf 直接退出整个桌面程序（无任何报错）
	precheck := []int{cfg.ControlPort, cfg.WorkPort, cfg.WebPort}
	if cfg.ProxyPort > 0 {
		precheck = append(precheck, cfg.ProxyPort)
	}
	if busy, which := checkPortBusy(precheck...); busy {
		appendLog(fmt.Sprintf("端口 %s 被占用，无法启动；请修改端口或关闭占用程序后重试", which))
		winui.Post(func() {
			if guiApp != nil {
				guiApp.SetText(idStatus, "状态：启动失败（端口被占用）")
			}
		})
		return
	}

	srv := NewServer(*cfg, abs)
	guiMu.Lock()
	guiServer = srv
	guiServerReady = false
	guiServerFailed = false
	guiMu.Unlock()

	winui.Post(func() {
		if guiApp != nil {
			guiApp.SetText(idStatus, "状态：启动中...")
		}
	})
	appendLog(fmt.Sprintf("启动中...  控制端口=%d  工作端口=%d  Web端口=%d  P2P端口=%d  SOCKS5端口=%d",
		cfg.ControlPort, cfg.WorkPort, cfg.WebPort, cfg.P2pPort, cfg.ProxyPort))
	appendLog("管理员令牌: " + cfg.AdminToken)

	go srv.Run()

	// 轮询 Web 端口，确认真实监听成功后再更新状态，并自动打开浏览器（无浏览器则跳过）
	go func() {
		url := fmt.Sprintf("http://127.0.0.1:%d/", cfg.WebPort)
		addr := fmt.Sprintf("127.0.0.1:%d", cfg.WebPort)
		ok := false
		for i := 0; i < 50; i++ {
			conn, err := net.DialTimeout("tcp", addr, 300*time.Millisecond)
			if err == nil {
				conn.Close()
				ok = true
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		guiMu.Lock()
		if ok {
			guiServerReady = true
		} else {
			guiServerFailed = true
		}
		guiMu.Unlock()
		if ok {
			winui.Post(func() {
				if guiApp != nil {
					guiApp.SetText(idStatus, fmt.Sprintf("状态：运行中（端口 %d）", cfg.WebPort))
				}
			})
			appendLog("服务已启动，正在尝试自动打开浏览器...")
			if winui.OpenBrowser(url) {
				appendLog("已自动打开浏览器：" + url)
			} else {
				appendLog("未检测到可用浏览器，请手动打开：" + url)
			}
		} else {
			winui.Post(func() {
				if guiApp != nil {
					guiApp.SetText(idStatus, "状态：启动失败（Web 端口无法监听）")
				}
			})
			appendLog("服务启动失败：Web 端口 " + strconv.Itoa(cfg.WebPort) + " 无法监听，可能被占用或权限不足")
		}
	}()
}

// checkPortBusy 依次尝试在 127.0.0.1 上监听给定端口，返回是否任一被占用及端口号。
func checkPortBusy(ports ...int) (bool, string) {
	for _, p := range ports {
		l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err != nil {
			return true, strconv.Itoa(p)
		}
		l.Close()
	}
	return false, ""
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
	guiServerReady = false
	guiServerFailed = false
	guiMu.Unlock()
	winui.Post(func() {
		if guiApp != nil {
			guiApp.SetText(idStatus, "状态：已停止")
		}
	})
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