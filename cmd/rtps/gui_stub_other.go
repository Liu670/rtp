//go:build !windows

package main

// runGUI 在非 Windows 平台不可用（Linux/macOS 用命令行 + Web 控制台，
// Android 用图形版 APK）。
func runGUI() {
	panic("图形界面仅在 Windows 上可用，请使用命令行模式或 Web 控制台")
}