//go:build !windows

package main

// runGUI 在非 Windows 平台不可用（Linux/macOS 用命令行，Android 用桌面版 APK）。
func runGUI() {
	panic("桌面界面仅在 Windows 上可用，请使用命令行模式（-server / -token / -name）")
}