//go:build !windows

// Package winui 在非 Windows 平台不提供 GUI 能力（rtp 的桌面 GUI 仅面向 Windows）。
package winui

// TrayItem 托盘菜单项（占位定义）。
type TrayItem struct {
	Text string
	ID   int32
}

// Supported 报告当前平台是否支持 GUI。
func Supported() bool { return false }