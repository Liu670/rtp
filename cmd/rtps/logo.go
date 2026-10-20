package main

import "net/http"

// rtp 品牌 Logo（SVG，无需鉴权，登录页与控制台共用）。
// 图案：渐变圆角方块 + 白色「隧道口 + 箭头」，寓意流量进入隧道转发。
const logoSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 48 48">
<defs><linearGradient id="rtpg" x1="0" y1="0" x2="1" y2="1">
<stop offset="0" stop-color="#6366f1"/><stop offset="1" stop-color="#8b5cf6"/>
</linearGradient></defs>
<rect width="48" height="48" rx="12" fill="url(#rtpg)"/>
<rect x="9" y="15" width="11" height="18" rx="5.5" fill="none" stroke="#fff" stroke-width="3.5"/>
<path d="M25 24H36M31 19l5 5-5 5" fill="none" stroke="#fff" stroke-width="3.5" stroke-linecap="round" stroke-linejoin="round"/>
</svg>`

func (s *Server) handleLogo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write([]byte(logoSVG))
}
