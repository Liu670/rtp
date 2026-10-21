//go:build windows

// Package winui 提供一个基于 Windows 原生 API 的轻量 GUI 工具库，
// 仅使用 Go 标准库 syscall 实现，保持 rtp「零第三方依赖」的定位。
//
// 支持：窗口、标签、输入框、按钮、多行日志框、右键托盘菜单、定时回调，
// 以及跨线程安全的主线程消息投递。
package winui

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

// ---- Windows 常量 ----
const (
	WS_OVERLAPPED       = 0x00000000
	WS_CAPTION          = 0x00C00000
	WS_SYSMENU          = 0x00080000
	WS_MINIMIZEBOX      = 0x00020000
	WS_TABSTOP          = 0x00010000
	WS_VISIBLE          = 0x10000000
	WS_CHILD            = 0x40000000
	WS_BORDER           = 0x00800000
	WS_VSCROLL          = 0x00200000
	WS_CLIPSIBLINGS     = 0x04000000

	ES_AUTOHSCROLL      = 0x0080
	ES_MULTILINE        = 0x0004
	ES_PASSWORD         = 0x0020
	ES_READONLY         = 0x0800
	ES_AUTOVSCROLL      = 0x0040

	BS_DEFPUSHBUTTON    = 0x0001
	BS_PUSHBUTTON       = 0x0000

	SS_LEFT             = 0x0000
	SS_ENDELLIPSIS      = 0x4000

	WM_DESTROY          = 0x0002
	WM_CLOSE            = 0x0010
	WM_COMMAND          = 0x0111
	WM_TIMER            = 0x0213
	WM_SETICON          = 0x0080
	WM_APP              = 0x8000
	WM_CTLCOLORSTATIC   = 0x0138
	WM_CTLCOLOREDIT     = 0x0133
	WM_CTLCOLORBTN      = 0x0135
	WM_USER             = 0x0400

	BN_CLICKED          = 0

	ICON_SMALL          = 0
	ICON_BIG            = 1

	IMAGE_ICON          = 1
	LR_DEFAULTSIZE      = 0x00000040
	LR_SHARED           = 0x00008000

	TPM_RIGHTBUTTON     = 0x0002
	MF_STRING           = 0x0000
	MF_SEPARATOR        = 0x0800

	NIM_ADD             = 0x00000000
	NIM_MODIFY          = 0x00000001
	NIM_DELETE          = 0x00000002
	NOTIFYICON_VERSION  = 3
	NIF_MESSAGE         = 0x00000001
	NIF_ICON            = 0x00000002
	NIF_TIP             = 0x00000004
	WM_LBUTTONDBLCLK    = 0x0203
	WM_RBUTTONUP        = 0x0205
	WM_CONTEXTMENU      = 0x007B

	// 自绘按钮 / GDI 常量
	WM_DRAWITEM          = 0x002B
	ODT_BUTTON           = 4
	ODS_SELECTED         = 0x0001
	ODS_DISABLED         = 0x0004
	PS_SOLID             = 0
	DT_CENTER            = 0x0001
	DT_VCENTER           = 0x0004
	DT_SINGLELINE        = 0x0020
	BS_OWNERDRAW         = 0x000B
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	pRegisterClassExW      = user32.NewProc("RegisterClassExW")
	pCreateWindowExW       = user32.NewProc("CreateWindowExW")
	pDefWindowProcW        = user32.NewProc("DefWindowProcW")
	pGetMessageW           = user32.NewProc("GetMessageW")
	pTranslateMessage      = user32.NewProc("TranslateMessage")
	pDispatchMessageW      = user32.NewProc("DispatchMessageW")
	pShowWindow            = user32.NewProc("ShowWindow")
	pUpdateWindow          = user32.NewProc("UpdateWindow")
	pPostQuitMessage       = user32.NewProc("PostQuitMessage")
	pSendMessageW          = user32.NewProc("SendMessageW")
	pPostMessageW          = user32.NewProc("PostMessageW")
	pLoadImageW            = user32.NewProc("LoadImageW")
	pLoadCursorW           = user32.NewProc("LoadCursorW")
	pCreateFontIndirectW   = gdi32.NewProc("CreateFontIndirectW")
	pSetTimer              = user32.NewProc("SetTimer")
	pKillTimer             = user32.NewProc("KillTimer")
	pGetModuleHandleW      = kernel32.NewProc("GetModuleHandleW")
	pGetSystemMetrics      = user32.NewProc("GetSystemMetrics")
	pSetForegroundWindow   = user32.NewProc("SetForegroundWindow")
	pShowWindowCmd         = pShowWindow
	pInvalidateRect        = user32.NewProc("InvalidateRect")
	pCreateSolidBrush      = gdi32.NewProc("CreateSolidBrush")
	pShellNotifyIconW      = shell32.NewProc("Shell_NotifyIconW")
	pCreatePopupMenu       = user32.NewProc("CreatePopupMenu")
	pAppendMenuW           = user32.NewProc("AppendMenuW")
	pTrackPopupMenu        = user32.NewProc("TrackPopupMenu")
	pDestroyMenu           = user32.NewProc("DestroyMenu")
	pGetCursorPos          = user32.NewProc("GetCursorPos")
	pDestroyIcon           = user32.NewProc("DestroyIcon")
	pEndPaintDummy         = user32.NewProc("GetWindowRect")
	pShowWindowAsync       = pShowWindow
	pGetDpiForSystem       = user32.NewProc("GetDpiForSystem")
	pPostQuitMessageProc   = pPostQuitMessage
	pDispatchMessageWProc  = pDispatchMessageW

	// 自绘按钮所需 GDI / user32 过程
	pRoundRect       = gdi32.NewProc("RoundRect")
	pSetBkMode       = gdi32.NewProc("SetBkMode")
	pSetTextColor    = gdi32.NewProc("SetTextColor")
	pDrawTextW       = user32.NewProc("DrawTextW")
	pSelectObject    = gdi32.NewProc("SelectObject")
	pDeleteObject    = gdi32.NewProc("DeleteObject")
	pCreatePen       = gdi32.NewProc("CreatePen")
	pGetWindowTextLengthW = user32.NewProc("GetWindowTextLengthW")
	pGetWindowTextW  = user32.NewProc("GetWindowTextW")
)

const (
	idcArrow  = 32512
	swShow    = 5
	swHide    = 0
	smCXScreen = 0
	smCYScreen = 1
	cwDefault = 0x80000000
)

// 主窗口句柄，供 Post 使用
var (
	gMain       uintptr
	gApp        *App
	postMessage = syscall.NewLazyDLL("user32.dll").NewProc("PostMessageW")
)

// wndClassEx 对应 Win32 WNDCLASSEX。
type wndClassEx struct {
	Size       uint32
	Style      uint32
	LpfnWndProc uintptr
	CbClsExtra int32
	CbWndExtra int32
	HInstance  uintptr
	HIcon      uintptr
	HCursor    uintptr
	HbrBackground uintptr
	LpszMenuName *uint16
	LpszClassName *uint16
	HIconSm    uintptr
}

// App 表示一个顶层窗口。
type App struct {
	hwnd      uintptr
	ctrl      map[int32]uintptr
	labels    map[int32]string
	font      uintptr
	brush     uintptr // 主窗口/静态文本背景
	brushEdit uintptr // 输入框白色背景
	buttons   map[int32]int // id -> 1=主按钮(实心) 2=次按钮(描边)
	dpi       float64
	OnCmd     func(id int32)
	OnTick    func()
	OnClose   func() bool // 返回 false 阻止关闭
	tickMs    uintptr
	trayTip   string
	trayOn    bool
	trayMenu  []TrayItem
	hidden    bool
}

// TrayItem 托盘右键菜单项。
type TrayItem struct {
	Text string
	ID   int32
}

type point struct{ X, Y int32 }

type tagNOTIFYICONDATA struct {
	Size              uint32
	Hwnd              uintptr
	ID                uint32
	Flags             uint32
	CallbackMessage   uint32
	Icon              uintptr
	Tip               [128]uint16
	State             uint32
	StateMask         uint32
	GUID              [16]byte
	HBalloonInfo      uintptr
}

// New 创建并显示一个窗口。w/h 为客户区逻辑尺寸（会自动按 DPI 缩放）。
func New(title string, w, h int) *App {
	// 关键：把创建窗口的 goroutine 钉在当前的 OS 线程上。Win32 要求
	// 窗口的消息循环（GetMessage）必须在「创建窗口的那个线程」上运行，
	// 否则窗口创建后 Go 调度器可能把 goroutine 挪到别的线程，导致消息收不到、
	// 窗口「打开即卡死、点哪都没反应」。LockOSThread 可根治此问题。
	runtime.LockOSThread()

	dpi := 96.0
	if r, _, err := pGetDpiForSystem.Call(); err == nil && r > 0 {
		dpi = float64(r)
	}
	a := &App{
		ctrl:      map[int32]uintptr{},
		labels:    map[int32]string{},
		font:      makeFont(14, dpi),
		brush:     newBrush(0xF5F7FA),
		brushEdit: newBrush(0xFFFFFF),
		buttons:   map[int32]int{},
		dpi:       dpi,
	}
	gApp = a

	clsName, _ := syscall.UTF16PtrFromString("RtpWinUIClass")
	hInst, _, _ := pGetModuleHandleW.Call(0)

	hIconBig, _, _ := pLoadImageW.Call(hInst, 1, IMAGE_ICON, 0, 0, LR_DEFAULTSIZE)
	hIconSmall, _, _ := pLoadImageW.Call(hInst, 1, IMAGE_ICON, sysGetInt(16), sysGetInt(16), 0)
	hCursor, _, _ := pLoadCursorW.Call(0, idcArrow)

	wc := wndClassEx{
		Size:          uint32(unsafe.Sizeof(wndClassEx{})), // 必须等于 sizeof(WNDCLASSEXW)，否则注册失败
		Style:         0x0008 | 0x0002,                     // CS_DBLCLKS | CS_HREDRAW
		LpfnWndProc:   syscall.NewCallback(a.wndProc),
		HInstance:     hInst,
		HIcon:         hIconBig,
		HCursor:       hCursor,
		HbrBackground: newBrush(0xF5F7FA),
		LpszClassName: clsName,
		HIconSm:       hIconSmall,
	}
	if atom, _, rerr := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); atom == 0 {
		// 1410 = ERROR_CLASS_ALREADY_EXISTS（同名类已注册，属正常可忽略）
		if e, ok := rerr.(syscall.Errno); ok && e != 0 && e != 1410 {
			panic("注册窗口类失败: " + e.Error())
		}
	}

	ti, _ := syscall.UTF16PtrFromString(title)
	hwnd, _, _ := pCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(clsName)),
		uintptr(unsafe.Pointer(ti)),
		WS_OVERLAPPED|WS_CAPTION|WS_SYSMENU|WS_MINIMIZEBOX,
		uintptr(centerX(w)),
		uintptr(centerY(h)),
		uintptr(int(a.s(w))+16),
		uintptr(int(a.s(h))+39),
		0, 0, hInst, 0)
	// 注意：syscall.LazyProc.Call 返回的 err 在调用成功时也是 Errno(0)（非 nil），
	// 因此不能以 err != nil 判定成败；必须检查返回值 hwnd，并改用 GetLastError() 取真实错误。
	if hwnd == 0 {
		panic("创建窗口失败: " + syscall.GetLastError().Error())
	}
	a.hwnd = hwnd
	gMain = hwnd
	pShowWindow.Call(hwnd, swShow)
	pUpdateWindow.Call(hwnd)

	if hIconBig != 0 {
		pSendMessageW.Call(hwnd, WM_SETICON, ICON_BIG, hIconBig)
	}
	if hIconSmall != 0 {
		pSendMessageW.Call(hwnd, WM_SETICON, ICON_SMALL, hIconSmall)
	}
	return a
}

func sysGetInt(v int) uintptr { return uintptr(int32(v)) }

func makeFont(size int, dpi float64) uintptr {
	lf := struct {
		Height         int32
		Width          int32
		Escapement     int32
		Orientation    int32
		Weight         int32
		Italic         byte
		Underline      byte
		StrikeOut      byte
		CharSet        byte
		OutPrecision   byte
		ClipPrecision  byte
		Quality        byte
		PitchAndFamily byte
		FaceName       [32]uint16
	}{}
	lf.Height = -int32(float64(size) * dpi / 72.0)
	lf.Weight = 400
	lf.CharSet = 1
	lf.Quality = 5
	lf.PitchAndFamily = 34
	copy(lf.FaceName[:], utf16("Microsoft YaHei UI"))
	h, _, _ := pCreateFontIndirectW.Call(uintptr(unsafe.Pointer(&lf)))
	return h
}

func newBrush(color uint32) uintptr {
	h, _, _ := pCreateSolidBrush.Call(uintptr(color))
	return h
}

func utf16(s string) []uint16 {
	r, _ := syscall.UTF16FromString(s)
	return r
}

func (a *App) s(v int) int { return int(float64(v) * a.dpi / 96.0) }

func centerX(w int) int {
	sw, _, _ := pGetSystemMetrics.Call(smCXScreen)
	return int((int(sw) - w) / 2)
}

func centerY(h int) int {
	sh, _, _ := pGetSystemMetrics.Call(smCYScreen)
	return int((int(sh) - h) / 2)
}

func (a *App) create(class string, id int32, style uint32, text string, x, y, w, h int) uintptr {
	ti, _ := syscall.UTF16PtrFromString(text)
	cl, _ := syscall.UTF16PtrFromString(class)
	hwnd, _, _ := pCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(cl)),
		uintptr(unsafe.Pointer(ti)),
		uintptr(style)|WS_CHILD|WS_VISIBLE,
		uintptr(a.s(x)), uintptr(a.s(y)), uintptr(a.s(w)), uintptr(a.s(h)),
		a.hwnd, uintptr(id), 0, 0)
	if hwnd != 0 {
		pSendMessageW.Call(hwnd, 0x0030, uintptr(a.font), 0) // WM_SETFONT
	}
	a.ctrl[id] = hwnd
	return hwnd
}

// Label 创建静态文本。id 可传 0（不参与事件）。
func (a *App) Label(id int32, text string, x, y, w, h int) {
	a.create("STATIC", id, SS_LEFT, text, x, y, w, h)
}

// Input 创建单行输入框。
func (a *App) Input(id int32, text string, x, y, w, h int) {
	a.create("EDIT", id, WS_BORDER|WS_TABSTOP|ES_AUTOHSCROLL, text, x, y, w, h)
}

// Password 创建密码输入框。
func (a *App) Password(id int32, text string, x, y, w, h int) {
	a.create("EDIT", id, WS_BORDER|WS_TABSTOP|ES_AUTOHSCROLL|ES_PASSWORD, text, x, y, w, h)
}

// Multi 创建多行只读日志框。
func (a *App) Multi(id int32, text string, x, y, w, h int) {
	a.create("EDIT", id, WS_BORDER|WS_TABSTOP|WS_VSCROLL|ES_MULTILINE|ES_READONLY|ES_AUTOVSCROLL,
		text, x, y, w, h)
}

// Button 创建次按钮（描边风格）。
func (a *App) Button(id int32, text string, x, y, w, h int) {
	a.buttons[id] = 2
	a.create("BUTTON", id, WS_TABSTOP|BS_PUSHBUTTON|BS_OWNERDRAW, text, x, y, w, h)
}

// DefButton 创建主按钮（实心强调色，回车触发）。
func (a *App) DefButton(id int32, text string, x, y, w, h int) {
	a.buttons[id] = 1
	a.create("BUTTON", id, WS_TABSTOP|BS_DEFPUSHBUTTON|BS_OWNERDRAW, text, x, y, w, h)
}

// getWindowTextOf 读取某控件句柄的文本。
func getWindowTextOf(h uintptr) string {
	n, _, _ := pGetWindowTextLengthW.Call(h)
	if n <= 0 {
		return ""
	}
	buf := make([]uint16, n+1)
	pGetWindowTextW.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(n+1))
	return syscall.UTF16ToString(buf)
}

func createPen(style, width int, color uint32) uintptr {
	h, _, _ := pCreatePen.Call(uintptr(style), uintptr(width), uintptr(color))
	return h
}

// drawButton 自绘按钮：圆角矩形 + 居中文字，含正常/按下/禁用三态。
func (a *App) drawButton(di *drawItemStruct, kind int) {
	hdc := di.HDC
	r := di.RcItem
	state := di.ItemState
	disabled := state&ODS_DISABLED != 0
	var fill, border, textCol uint32
	if kind == 1 { // 主按钮：实心强调蓝
		switch {
		case disabled:
			fill, border, textCol = 0xBFC6CF, 0xBFC6CF, 0xFFFFFF
		case state&ODS_SELECTED != 0:
			fill, border, textCol = 0x2257C8, 0x2257C8, 0xFFFFFF
		default:
			fill, border, textCol = 0x2F6DF6, 0x2F6DF6, 0xFFFFFF
		}
	} else { // 次按钮：白底描边
		switch {
		case disabled:
			fill, border, textCol = 0xE6E9EE, 0xD5DAE1, 0x9AA3AD
		case state&ODS_SELECTED != 0:
			fill, border, textCol = 0xDDE7FF, 0x2F6DF6, 0x2F6DF6
		default:
			fill, border, textCol = 0xFFFFFF, 0xC9D1DC, 0x2F6DF6
		}
	}
	brush := newBrush(fill)
	pen := createPen(PS_SOLID, 1, border)
	pSetBkMode.Call(hdc, 1) // TRANSPARENT
	pSetTextColor.Call(hdc, uintptr(textCol))
	oldPen, _, _ := pSelectObject.Call(hdc, uintptr(pen))
	oldBrush, _, _ := pSelectObject.Call(hdc, uintptr(brush))
	pRoundRect.Call(hdc, r.Left, r.Top, r.Right, r.Bottom, 10, 10)
	pSelectObject.Call(hdc, oldBrush)
	pSelectObject.Call(hdc, oldPen)
	pDeleteObject.Call(brush)
	pDeleteObject.Call(pen)

	text := getWindowTextOf(di.HwndItem)
	tr := r
	pDrawTextW.Call(hdc, uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(text))),
		int32(-1), uintptr(unsafe.Pointer(&tr)), DT_CENTER|DT_VCENTER|DT_SINGLELINE)
}

// Text 读取控件文本。
func (a *App) Text(id int32) string {
	h, ok := a.ctrl[id]
	if !ok {
		return ""
	}
	n, _, _ := pSendMessageW.Call(uintptr(h), 0x000E, 0, 0) // WM_GETTEXTLENGTH
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n+2)
	pSendMessageW.Call(uintptr(h), 0x000D, uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])))
	return syscall.UTF16ToString(buf)
}

// SetText 设置控件文本。
func (a *App) SetText(id int32, s string) {
	h, ok := a.ctrl[id]
	if !ok {
		return
	}
	b, _ := syscall.UTF16FromString(s)
	pSendMessageW.Call(uintptr(h), 0x000C, 0, uintptr(unsafe.Pointer(&b[0])))
}

// Enable 启用/禁用控件。
func (a *App) Enable(id int32, on bool) {
	h, ok := a.ctrl[id]
	if !ok {
		return
	}
	if on {
		pSendMessageW.Call(uintptr(h), 0x00F1, 1, 0) // WM_ENABLE
	} else {
		pSendMessageW.Call(uintptr(h), 0x00F1, 0, 0)
	}
}

// Show 显示或隐藏控件。
func (a *App) Show(id int32, on bool) {
	h, ok := a.ctrl[id]
	if !ok {
		return
	}
	if on {
		pShowWindow.Call(uintptr(h), swShow)
	} else {
		pShowWindow.Call(uintptr(h), swHide)
	}
}

// Append 向多行文本框追加一行（自动滚动到底）。
func (a *App) Append(id int32, line string) {
	h, ok := a.ctrl[id]
	if !ok {
		return
	}
	n := len(utf16(line))
	buf := make([]uint16, n)
	copy(buf, utf16(line))
	// EM_SETSEL 到末尾，再 EM_REPLACESEL
	pSendMessageW.Call(uintptr(h), 0x00B1, ^uintptr(0), ^uintptr(0)) // EM_SETSEL(-1,-1)
	pSendMessageW.Call(uintptr(h), 0x00C2, 0, uintptr(unsafe.Pointer(&buf[0])))
	pInvalidateRect.Call(uintptr(h), 0, 0)
}

// Tick 注册定时回调（毫秒），用于刷新状态。
func (a *App) Tick(ms int, fn func()) {
	a.OnTick = fn
	a.tickMs = uintptr(ms)
	pSetTimer.Call(uintptr(a.hwnd), 1, uintptr(ms), 0)
}

// Tray 启用系统托盘图标。menu 为右键菜单项。
func (a *App) Tray(tip string, menu []TrayItem) {
	a.trayTip = tip
	a.trayMenu = menu
	nid := tagNOTIFYICONDATA{}
	nid.Size = uint32(unsafe.Sizeof(nid))
	nid.Hwnd = a.hwnd
	nid.ID = 1
	nid.CallbackMessage = WM_APP + 1
	nid.Flags = NIF_MESSAGE | NIF_ICON | NIF_TIP
	nid.Icon = a.smallIcon()
	copy(nid.Tip[:], utf16(tip))
	ret, _, _ := pShellNotifyIconW.Call(NIM_ADD, uintptr(unsafe.Pointer(&nid)))
	a.trayOn = ret != 0
}

func (a *App) smallIcon() uintptr {
	hInst, _, _ := pGetModuleHandleW.Call(0)
	h, _, _ := pLoadImageW.Call(hInst, 1, IMAGE_ICON, sysGetInt(16), sysGetInt(16), 0)
	return h
}

// Toast 更新托盘提示文字。
func (a *App) Toast(tip string) {
	a.trayTip = tip
	if !a.trayOn {
		return
	}
	nid := tagNOTIFYICONDATA{}
	nid.Size = uint32(unsafe.Sizeof(nid))
	nid.Hwnd = a.hwnd
	nid.ID = 1
	nid.Flags = NIF_MESSAGE | NIF_ICON | NIF_TIP
	nid.Icon = a.smallIcon()
	copy(nid.Tip[:], utf16(tip))
	pShellNotifyIconW.Call(NIM_MODIFY, uintptr(unsafe.Pointer(&nid)))
}

// Hide 隐藏主窗口（收进托盘）。
func (a *App) Hide() {
	a.hidden = true
	pShowWindow.Call(uintptr(a.hwnd), swHide)
}

// Show2 显示主窗口并置前。
func (a *App) Show2() {
	a.hidden = false
	pShowWindow.Call(uintptr(a.hwnd), swShow)
	pSetForegroundWindow.Call(uintptr(a.hwnd))
}

// Quit 退出程序。
func (a *App) Quit() {
	if a.trayOn {
		nid := tagNOTIFYICONDATA{}
		nid.Size = uint32(unsafe.Sizeof(nid))
		nid.Hwnd = a.hwnd
		nid.ID = 1
		pShellNotifyIconW.Call(NIM_DELETE, uintptr(unsafe.Pointer(&nid)))
	}
	pPostQuitMessage.Call(0)
}

// Post 从任意 goroutine 安全地投递回调到 UI 线程。
func Post(fn func()) {
	if gMain == 0 || fn == nil {
		return
	}
	// 用堆上的结构携带回调，避免闭包被回收
	v := &postItem{fn: fn}
	postQueue = append(postQueue, v)
	postMessage.Call(gMain, WM_APP+2, 0, uintptr(unsafe.Pointer(v)))
}

type postItem struct{ fn func() }

// msgSpec 对应 Win32 MSG。
type msgSpec struct {
	Hwnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       struct{ X, Y int32 }
	LPrivate uint32
}

// drawItemStruct 对应 Win32 DRAWITEMSTRUCT（自绘按钮绘图时传入）。
type rect struct{ Left, Top, Right, Bottom int32 }

type drawItemStruct struct {
	CtlType    uint32
	CtlID      uint32
	ItemID     uint32
	ItemAction  uint32
	ItemState  uint32
	HwndItem   uintptr
	HDC        uintptr
	RcItem     rect
	ItemData   uintptr
}

// Run 启动消息循环，阻塞直到窗口关闭。
func (a *App) Run() {
	var msg msgSpec
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

var postQueue []*postItem

// wndProc 窗口过程。注意：所有参数必须是指针宽度（uintptr），否则 syscall.NewCallback
// 在 amd64 上会因参数对齐错误而 panic，且 -H windowsgui 编译下无控制台，表现为
// 「双击无反应」。故 msg 也用 uintptr（不要用 uint32）。
func (a *App) wndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_COMMAND:
		id := int32(wParam & 0xFFFF)
		hi := int32((wParam >> 16) & 0xFFFF)
		if hi == BN_CLICKED && a.OnCmd != nil {
			go a.OnCmd(id)
		}
		return 0
	case WM_DRAWITEM:
		di := (*drawItemStruct)(unsafe.Pointer(lParam))
		if di.CtlType == ODT_BUTTON {
			if kind, ok := a.buttons[int32(di.CtlID)]; ok {
				a.drawButton(di, kind)
				return 0
			}
		}
		return 0
	case WM_CTLCOLORSTATIC:
		pSetBkMode.Call(wParam, 1) // TRANSPARENT
		pSetTextColor.Call(wParam, uintptr(0x1F2329))
		return a.brush
	case WM_CTLCOLOREDIT:
		pSetBkMode.Call(wParam, 1)
		pSetTextColor.Call(wParam, uintptr(0x1F2329))
		return a.brushEdit
	case WM_TIMER:
		if a.OnTick != nil {
			go a.OnTick()
		}
		return 0
	case WM_APP + 1:
		switch lParam {
		case WM_LBUTTONDBLCLK:
			a.Show2()
		case WM_RBUTTONUP, WM_CONTEXTMENU:
			a.trayMenuPopup()
		}
		return 0
	case WM_APP + 2:
		// 执行 Post 投递的回调
		p := (*postItem)(unsafe.Pointer(lParam))
		if p != nil && p.fn != nil {
			p.fn()
		}
		return 0
	case WM_CLOSE:
		if a.trayOn {
			a.Hide()
			return 0
		}
		if a.OnClose != nil && !a.OnClose() {
			return 0
		}
		pPostQuitMessage.Call(0)
		return 0
	case WM_DESTROY:
		if a.trayOn {
			nid := tagNOTIFYICONDATA{}
			nid.Size = uint32(unsafe.Sizeof(nid))
			nid.Hwnd = hwnd
			nid.ID = 1
			pShellNotifyIconW.Call(NIM_DELETE, uintptr(unsafe.Pointer(&nid)))
		}
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return r
}

func (a *App) trayMenuPopup() {
	hMenu, _, _ := pCreatePopupMenu.Call()
	for _, it := range a.trayMenu {
		pAppendMenuW.Call(hMenu, MF_STRING, uintptr(it.ID), uintptr(unsafe.Pointer(strPtr(it.Text))))
	}
	var pt point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pSetForegroundWindow.Call(uintptr(a.hwnd))
	cmd, _, _ := pTrackPopupMenu.Call(hMenu, TPM_RIGHTBUTTON, uintptr(pt.X), uintptr(pt.Y), 0, uintptr(a.hwnd), 0)
	pPostMessageW.Call(uintptr(a.hwnd), 0x0281, 0, 0) // WM_NULL
	pDestroyMenu.Call(hMenu)
	if cmd != 0 && a.OnCmd != nil {
		go a.OnCmd(int32(cmd))
	}
}

func strPtr(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

// OpenBrowser 用系统默认浏览器打开链接（内嵌 WebView 不可用时的兜底方案）。
func OpenBrowser(url string) {
	sh := shell32.NewProc("ShellExecuteW")
	t, _ := syscall.UTF16PtrFromString(url)
	sh.Call(uintptr(unsafe.Pointer(t)), 0, 0, 0, 0, 1)
}

// Fatal 用 MessageBox 弹窗显示致命错误。用于替代「双击无反应」式的静默崩溃：
// 桌面程序以 -H windowsgui 编译（无控制台），一旦 panic 用户看不到任何提示，
// 因此统一在崩溃时弹窗并把堆栈写入日志文件。
func Fatal(title, msg string) {
	mb := user32.NewProc("MessageBoxW")
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(msg)
	mb.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0)
}

// CrashLog 把致命错误写入 exe 同目录的日志文件，便于排错。
func CrashLog(name, msg string) {
	if exe, err := os.Executable(); err == nil {
		_ = os.WriteFile(filepath.Join(filepath.Dir(exe), name), []byte(msg), 0o644)
	}
}

// Sprintf 是 fmt.Sprintf 的简写，供调用方拼接窗口文案。
func Sprintf(format string, a ...any) string { return fmt.Sprintf(format, a...) }

// Since 判断是否超过 duration，用于定时回调节流。
func Since(t time.Time, d time.Duration) bool { return time.Since(t) >= d }