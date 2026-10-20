//go:build !windows

package main

// 非 Windows 平台（Linux / macOS / Android）走命令行或图形 APK。
func init() { guiAvailable = false }