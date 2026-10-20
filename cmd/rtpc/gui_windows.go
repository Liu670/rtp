//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Liu670/rtp/internal/winui"
)

// 控件 ID
const (
	cServer = iota + 100
	cToken
	cName
	cStatus
	cTraffic
	cBtnConnect
	cBtnDisconnect
	cBtnCopy
	cBtnConfig
	cBtnAutostart
	cLog
	cCfgPath
)

const (
	cMenuShow = 500
	cMenuQuit = 501
)

var (
	cApp    *winui.App
	cClient *Client
	cMu     sync.Mutex
	cRun    bool
	cCfg    clientCfg
	cBytesU int64
	cBytesD int64
)

type clientCfg struct {
	Server string `json:"server"`
	Token  string `json:"token"`
	Name   string `json:"name"`
}

// runGUI 是 rtpc 的 Windows 桌面界面入口。
// 双击运行不再出现黑窗一闪而过：缺少参数时直接打开本界面，填完点「连接」即可。
func runGUI() {
	cfg := loadClientCfg()

	cApp = winui.New("rtp 内网穿透客户端", 540, 500)
	cCfg = cfg

	cApp.Label(0, "rtp 内网穿透客户端  ·  Windows 桌面界面", 20, 14, 500, 22)
	cApp.Label(0, "填写服务端地址与客户端令牌，点「连接」即可接入内网穿透。", 20, 38, 500, 20)

	y := 68
	cApp.Label(cServer, "服务端地址", 20, y+2, 100, 20)
	cApp.Input(cServer+1, cfg.Server, 120, y, 260, 24)
	cApp.Label(0, "如 1.2.3.4:7000", 390, y+4, 140, 18)
	y += 32

	cApp.Label(cToken, "客户端令牌", 20, y+2, 100, 20)
	cApp.Password(cToken+1, cfg.Token, 120, y, 260, 24)
	cApp.Button(cBtnCopy, "复制自配置文件", 388, y, 142, 24)
	y += 32

	cApp.Label(cName, "本机名称", 20, y+2, 100, 20)
	cApp.Input(cName+1, cfg.Name, 120, y, 260, 24)
	cApp.Button(cBtnConfig, "打开目录", 388, y, 80, 24)
	cApp.Button(cBtnAutostart, "开机自启", 474, y, 66, 24)
	y += 40

	cApp.DefButton(cBtnConnect, "连接", 20, y, 100, 30)
	cApp.Button(cBtnDisconnect, "断开", 128, y, 100, 30)
	cApp.Label(cStatus, "状态：未连接", 240, y+6, 280, 20)
	y += 34

	cApp.Label(0, "流量", 20, y, 60, 20)
	cApp.Label(cTraffic, "↑ 0 B    ↓ 0 B", 80, y+2, 240, 20)
	y += 26

	cApp.Label(0, "运行日志", 20, y, 100, 20)
	cApp.Multi(cLog, "就绪。首次使用请到服务端 Web 控制台「客户端」页复制令牌。", 20, y+20, 510, 130)

	cApp.OnCmd = onCmd
	cApp.Tray("rtp 客户端", []winui.TrayItem{
		{Text: "显示主窗口", ID: cMenuShow},
		{Text: "退出", ID: cMenuQuit},
	})
	cApp.Tick(1000, tick)

	cApp.Run()
	if cRun {
		disconnect()
	}
}

func loadClientCfg() clientCfg {
	cfg := clientCfg{Server: "127.0.0.1:7000", Name: defaultClientName()}
	if data, err := os.ReadFile(clientCfgPath()); err == nil {
		_ = json.Unmarshal(data, &cfg)
	}
	if cfg.Server == "" {
		cfg.Server = "127.0.0.1:7000"
	}
	if cfg.Name == "" {
		cfg.Name = defaultClientName()
	}
	return cfg
}

func clientCfgPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "rtp", "client.json")
}

func saveClientCfg() {
	p := clientCfgPath()
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	data, err := json.MarshalIndent(cCfg, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(p, data, 0o600)
}

func defaultClientName() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "windows-pc"
	}
	return h
}

func onCmd(id int32) {
	switch id {
	case cBtnConnect:
		connectFromUI()
	case cBtnDisconnect:
		disconnect()
		cApp.Toast("rtp 客户端 - 已断开")
	case cBtnCopy:
		pasteTokenFromFile()
	case cBtnConfig:
		openCfgDir()
	case cBtnAutostart:
		toggleAutostart()
	case cMenuShow:
		cApp.Show2()
	case cMenuQuit:
		disconnect()
		cApp.Quit()
	}
}

func connectFromUI() {
	cMu.Lock()
	if cRun {
		cMu.Unlock()
		cLogLine("已在连接中")
		return
	}
	cMu.Unlock()

	server := strings.TrimSpace(cApp.Text(cServer + 1))
	token := strings.TrimSpace(cApp.Text(cToken + 1))
	name := strings.TrimSpace(cApp.Text(cName + 1))
	if token == "" {
		cLogLine("请填写客户端令牌（服务端 Web 控制台「客户端」页可复制）")
		return
	}
	host, port, ok := splitServer(server)
	if !ok {
		cLogLine("服务端地址格式不正确，应为 host:port")
		return
	}
	if name == "" {
		name = defaultClientName()
	}

	cMu.Lock()
	cCfg = clientCfg{Server: host + ":" + strconv.Itoa(port), Token: token, Name: name}
	cRun = true
	cBytesU, cBytesD = 0, 0
	cMu.Unlock()
	saveClientCfg()

	cl := &Client{
		serverHost: host,
		serverAddr: host + ":" + strconv.Itoa(port),
		token:      token,
		name:       name,
		tunnels:    map[string]TunnelConfig{},
	}
	cMu.Lock()
	cClient = cl
	cMu.Unlock()

	cLogLine(fmt.Sprintf("正在连接 %s:%d，名称=%s", host, port, name))
	go func() {
		cl.Run()
		cMu.Lock()
		cRun = false
		cMu.Unlock()
		winui.Post(func() {
			if cApp != nil {
				cApp.SetText(cStatus, "状态：连接已断开")
			}
		})
	}()
	cApp.Toast("rtp 客户端 - 连接中")
}

func disconnect() {
	cMu.Lock()
	cl := cClient
	cRun = false
	cMu.Unlock()
	if cl != nil {
		cl.Stop()
	}
	cLogLine("已断开连接")
	winui.Post(func() {
		if cApp != nil {
			cApp.SetText(cStatus, "状态：已断开")
			cApp.SetText(cTraffic, "↑ 0 B    ↓ 0 B")
		}
	})
}

func pasteTokenFromFile() {
	// 从服务端 Web 控制台导出的配置里找令牌，或让用户手动粘贴
	p := clientCfgPath()
	if data, err := os.ReadFile(p); err == nil {
		var cfg clientCfg
		if json.Unmarshal(data, &cfg) == nil && cfg.Token != "" {
			cApp.SetText(cToken+1, cfg.Token)
			cLogLine("已从本地配置读取令牌")
			return
		}
	}
	cLogLine("本地暂无保存的令牌，请手动粘贴服务端控制台复制的令牌")
}

func openCfgDir() {
	p := clientCfgPath()
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	winui.OpenBrowser("file:///" + filepath.ToSlash(filepath.Dir(p)))
	cLogLine("已打开配置目录")
}

func toggleAutostart() {
	p := clientCfgPath()
	startup := filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu",
		"Programs", "Startup")
	exe, err := os.Executable()
	if err != nil {
		cLogLine("无法定位程序路径: " + err.Error())
		return
	}
	lnk := filepath.Join(startup, "rtp-client.lnk")
	if _, err := os.Stat(lnk); err == nil {
		_ = os.Remove(lnk)
		cLogLine("已取消开机自启")
		cApp.Toast("rtp 客户端 - 已取消自启")
		return
	}
	_ = os.MkdirAll(startup, 0o755)
	if err := createShortcut(lnk, exe, "-cli"); err != nil {
		cLogLine("创建开机启动项失败: " + err.Error())
		return
	}
	cLogLine("已设置开机自启（配置保存在 " + p + "）")
	cApp.Toast("rtp 客户端 - 已设置自启")
}

func tick() {
	cMu.Lock()
	run := cRun
	up, down := cBytesU, cBytesD
	cMu.Unlock()
	if !run {
		return
	}
	winui.Post(func() {
		if cApp != nil {
			cApp.SetText(cTraffic, fmt.Sprintf("↑ %s    ↓ %s", human(up), human(down)))
		}
	})
}

func human(b int64) string {
	switch {
	case b < 1024:
		return fmt.Sprintf("%d B", b)
	case b < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	case b < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(b)/1024/1024)
	default:
		return fmt.Sprintf("%.2f GB", float64(b)/1024/1024/1024)
	}
}

func splitServer(s string) (string, int, bool) {
	if s == "" {
		return "", 0, false
	}
	i := strings.LastIndex(s, ":")
	if i < 0 {
		return s, 7000, true
	}
	port, err := strconv.Atoi(s[i+1:])
	if err != nil || port < 1 || port > 65535 {
		return "", 0, false
	}
	return s[:i], port, true
}

func cLogLine(line string) {
	t := time.Now().Format("15:04:05")
	winui.Post(func() {
		if cApp != nil {
			cApp.Append(cLog, "["+t+"] "+line)
		}
	})
}

// createShortcut 用 PowerShell 创建启动项快捷方式（纯系统能力，无需第三方库）。
func createShortcut(lnk, exe string, args string) error {
	ps := fmt.Sprintf(`$s=(New-Object -COM WScript.Shell).CreateShortcut('%s');$s.TargetPath='%s';$s.Arguments='%s';$s.WorkingDirectory='%s';$s.Save()`,
		lnk, exe, args, filepath.Dir(exe))
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}