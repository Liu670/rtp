//go:build windows

// Package winui 在 Windows 上提供完整的原生 GUI 能力。
package winui

// Supported 报告当前平台是否支持 GUI。
func Supported() bool { return true }