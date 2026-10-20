package main

// 共享类型别名：core 包中的结构与本包一一对应。
import "github.com/Liu670/rtp/internal/core"

type (
	Msg          = core.Msg
	TunnelConfig = core.TunnelConfig
	ClientConfig = core.ClientConfig
	Config       = core.Config
	Traffic      = core.Traffic
	Throttle     = core.Throttle
	Event        = core.Event
)
