//go:build !windows

package main

// 非 Windows 平台只有命令行 + Web 控制台两种方式。
func init() { guiAvailable = false }