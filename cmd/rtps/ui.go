package main

// 管理控制台前端（单文件原生 HTML/CSS/JS，无外部依赖）。
// 注意：Go 原始字符串用反引号包裹，因此 JS 不使用反引号模板字符串，统一用单引号 + 拼接；
// 模板经 fmt.Fprintf 注入 6 个端口，所有字面 %% 必须写成 %%%%。

const uiHTML = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>rtp 控制台</title>
<style>
:root{
  --bg:#0b1020; --bg2:#0e1428; --card:rgba(255,255,255,.05); --card2:rgba(255,255,255,.08);
  --border:rgba(255,255,255,.10); --text:#e8ecf6; --muted:#8b93ad;
  --accent:#6366f1; --accent2:#8b5cf6; --ok:#22c55e; --warn:#f59e0b; --err:#ef4444;
  --up:#38bdf8; --down:#34d399;
  --shadow:0 10px 34px rgba(0,0,0,.38); --r:14px;
}
body.light{
  --bg:#eef1f8; --bg2:#ffffff; --card:rgba(255,255,255,.9); --card2:#ffffff;
  --border:rgba(15,23,42,.10); --text:#1e293b; --muted:#64748b;
  --shadow:0 10px 30px rgba(15,23,42,.10);
}
html[data-accent="emerald"]{--accent:#10b981;--accent2:#34d399;}
html[data-accent="rose"]{--accent:#f43f5e;--accent2:#fb7185;}
html[data-accent="amber"]{--accent:#f59e0b;--accent2:#fbbf24;}
html[data-accent="sky"]{--accent:#0ea5e9;--accent2:#38bdf8;}
*{box-sizing:border-box;margin:0;padding:0}
body{background:var(--bg);color:var(--text);font:14px/1.55 system-ui,-apple-system,"Segoe UI","PingFang SC","Microsoft YaHei",sans-serif;min-height:100vh;overflow-x:hidden}
body::before{content:"";position:fixed;inset:0;background:radial-gradient(60rem 30rem at 15%% -10rem,rgba(99,102,241,.16),transparent 60%%),radial-gradient(50rem 26rem at 110%% 10%%,rgba(139,92,246,.12),transparent 60%%);pointer-events:none;z-index:0}
.glass{background:var(--card);border:1px solid var(--border);border-radius:var(--r);backdrop-filter:blur(14px);-webkit-backdrop-filter:blur(14px);box-shadow:var(--shadow)}
button{font:inherit;color:inherit;cursor:pointer;background:none;border:none}
input,select,textarea{font:inherit;color:var(--text);background:var(--card2);border:1px solid var(--border);border-radius:10px;padding:9px 12px;outline:none;width:100%%;transition:border-color .15s}
input:focus,select:focus,textarea:focus{border-color:var(--accent)}
a{color:var(--accent);text-decoration:none}
.mono{font-family:ui-monospace,SFMono-Regular,Consolas,monospace;font-size:12.5px}
::-webkit-scrollbar{width:9px;height:9px}::-webkit-scrollbar-thumb{background:var(--border);border-radius:8px}

/* ============ 登录 ============ */
.login-wrap{position:relative;z-index:1;min-height:100vh;display:flex;align-items:center;justify-content:center;padding:24px}
.login-card{width:400px;max-width:100%%;padding:34px 32px;text-align:center}
.login-logo{font-size:34px;font-weight:800;letter-spacing:.5px;background:linear-gradient(120deg,var(--accent),var(--accent2));-webkit-background-clip:text;background-clip:text;color:transparent}
.login-sub{color:var(--muted);font-size:13px;margin-top:4px}
.login-tabs{display:flex;gap:6px;margin:22px 0 16px;background:var(--card2);border:1px solid var(--border);border-radius:12px;padding:4px}
.login-tabs button{flex:1;padding:8px;border-radius:9px;font-size:13px;color:var(--muted)}
.login-tabs button.on{background:linear-gradient(120deg,var(--accent),var(--accent2));color:#fff;font-weight:600}
.login-card input{margin-bottom:12px;text-align:left}
.pw-wrap{position:relative}
.pw-wrap button{position:absolute;right:8px;top:50%%;transform:translateY(-50%%);color:var(--muted);font-size:12px;padding:4px 6px}
.btn{display:inline-flex;align-items:center;justify-content:center;gap:6px;padding:9px 16px;border-radius:10px;border:1px solid var(--border);background:var(--card2);font-size:13px;font-weight:500;transition:all .15s;white-space:nowrap}
.btn:hover{border-color:var(--accent);transform:translateY(-1px)}
.btn.primary{background:linear-gradient(120deg,var(--accent),var(--accent2));border:none;color:#fff;font-weight:600}
.btn.danger{color:var(--err);border-color:rgba(239,68,68,.4)}
.btn.sm{padding:5px 10px;font-size:12px;border-radius:8px}
.btn.block{width:100%%}
.login-hint{margin-top:14px;font-size:12px;color:var(--muted)}
.login-err{margin-top:10px;font-size:12.5px;color:var(--err);min-height:18px}
.ann-banner{margin:0 0 14px;padding:9px 12px;border-radius:10px;background:rgba(245,158,11,.12);border:1px solid rgba(245,158,11,.35);color:var(--warn);font-size:12.5px;text-align:left}

/* ============ 框架 ============ */
#shell{display:none;position:relative;z-index:1;min-height:100vh}
#shell.on{display:flex}
#sidebar{width:216px;flex-shrink:0;padding:18px 12px;display:flex;flex-direction:column;gap:4px;position:sticky;top:0;height:100vh;background:var(--card);border-right:1px solid var(--border);backdrop-filter:blur(14px)}
.brand{font-size:22px;font-weight:800;padding:4px 10px 16px;background:linear-gradient(120deg,var(--accent),var(--accent2));-webkit-background-clip:text;background-clip:text;color:transparent}
.brand small{font-size:11px;color:var(--muted);-webkit-text-fill-color:var(--muted);font-weight:500;margin-left:6px}
#sidebar nav a{display:flex;align-items:center;gap:10px;padding:10px 12px;border-radius:10px;color:var(--muted);font-size:13.5px;font-weight:500;cursor:pointer;transition:all .15s}
#sidebar nav a:hover{background:var(--card2);color:var(--text)}
#sidebar nav a.on{background:linear-gradient(120deg,rgba(99,102,241,.22),rgba(139,92,246,.14));color:var(--text);border:1px solid rgba(99,102,241,.35)}
#sidebar nav a .ico{width:20px;text-align:center}
.side-foot{margin-top:auto;display:flex;flex-direction:column;gap:10px;padding:10px 4px 0;border-top:1px solid var(--border)}
.accents{display:flex;gap:8px;justify-content:center}
.acc-dot{width:16px;height:16px;border-radius:50%%;cursor:pointer;border:2px solid transparent;transition:transform .15s}
.acc-dot:hover{transform:scale(1.18)}
.acc-dot.on{border-color:var(--text)}
#main{flex:1;min-width:0;display:flex;flex-direction:column}
#topbar{position:sticky;top:0;z-index:50;display:flex;align-items:center;gap:12px;padding:12px 22px;background:rgba(11,16,32,.72);border-bottom:1px solid var(--border);backdrop-filter:blur(14px)}
body.light #topbar{background:rgba(255,255,255,.78)}
#burger{display:none;font-size:18px}
#page-title{font-size:16px;font-weight:700}
#top-ann{flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:12.5px;color:var(--warn);background:rgba(245,158,11,.1);border:1px solid rgba(245,158,11,.3);border-radius:999px;padding:4px 14px}
#top-ann:empty{display:none}
.kbd{border:1px solid var(--border);border-bottom-width:2px;border-radius:6px;padding:1px 6px;font-size:11px;color:var(--muted);background:var(--card2)}
#user-chip{display:flex;align-items:center;gap:7px;padding:6px 12px;border-radius:999px;border:1px solid var(--border);background:var(--card2);font-size:12.5px;cursor:pointer}
#user-chip .ava{width:22px;height:22px;border-radius:50%%;background:linear-gradient(120deg,var(--accent),var(--accent2));display:flex;align-items:center;justify-content:center;font-size:11px;color:#fff;font-weight:700}
#content{padding:22px;max-width:1280px;width:100%%;margin:0 auto}
.page-head{display:flex;align-items:flex-end;justify-content:space-between;gap:12px;margin-bottom:18px;flex-wrap:wrap}
.page-head h1{font-size:21px;font-weight:750}
.page-sub{color:var(--muted);font-size:12.5px;margin-top:2px}
.toolbar{display:flex;gap:10px;align-items:center;margin-bottom:14px;flex-wrap:wrap}
.toolbar input,.toolbar select{max-width:280px}
.count{margin-left:auto;color:var(--muted);font-size:12px}
.grid-4{display:grid;grid-template-columns:repeat(4,1fr);gap:14px;margin-bottom:14px}
.grid-3{display:grid;grid-template-columns:repeat(3,1fr);gap:14px;margin-bottom:14px}
.grid-2{display:grid;grid-template-columns:repeat(2,1fr);gap:14px;margin-bottom:14px}
.card{padding:16px 18px;margin-bottom:14px}
.card-head{display:flex;align-items:center;justify-content:space-between;font-size:13.5px;font-weight:650;margin-bottom:12px}
.stat-label{color:var(--muted);font-size:12.5px}
.stat-val{font-size:26px;font-weight:750;margin-top:4px;font-variant-numeric:tabular-nums}
.stat-sub{color:var(--muted);font-size:12px;margin-top:2px}
.legend{display:flex;align-items:center;gap:12px;font-size:12px;color:var(--muted);font-weight:400}
.dot{display:inline-block;width:8px;height:8px;border-radius:50%%;margin-right:5px;vertical-align:1px}
.dot.up{background:var(--up)}.dot.down{background:var(--down)}
#bw{width:100%%;height:190px;display:block}
.chips{display:flex;gap:8px;flex-wrap:wrap}
.chip{display:inline-flex;align-items:center;gap:6px;padding:6px 12px;border-radius:999px;border:1px solid var(--border);background:var(--card2);font-size:12.5px;cursor:pointer;transition:all .15s;color:var(--muted)}
.chip:hover{color:var(--text);border-color:var(--accent)}
.chip.on{background:linear-gradient(120deg,var(--accent),var(--accent2));color:#fff;border-color:transparent;font-weight:600}
.badge{display:inline-block;padding:2px 9px;border-radius:999px;font-size:11.5px;font-weight:650;border:1px solid var(--border);background:var(--card2)}
.badge.tcp{color:var(--up);border-color:rgba(56,189,248,.4);background:rgba(56,189,248,.1)}
.badge.udp{color:var(--warn);border-color:rgba(245,158,11,.4);background:rgba(245,158,11,.1)}
.badge.http{color:var(--ok);border-color:rgba(34,197,94,.4);background:rgba(34,197,94,.1)}
.badge.https{color:#a3e635;border-color:rgba(163,230,53,.4);background:rgba(163,230,53,.1)}
.badge.stcp{color:var(--accent2);border-color:rgba(139,92,246,.45);background:rgba(139,92,246,.12)}
.badge.p2p{color:#f472b6;border-color:rgba(244,114,182,.45);background:rgba(244,114,182,.12)}
.badge.gray{color:var(--muted)}
.badge.ok{color:var(--ok);border-color:rgba(34,197,94,.4)}
.badge.warn{color:var(--warn);border-color:rgba(245,158,11,.4)}
.badge.err{color:var(--err);border-color:rgba(239,68,68,.4)}
.pulse{display:inline-block;width:9px;height:9px;border-radius:50%%;background:#475569;vertical-align:1px}
.pulse.on{background:var(--ok);box-shadow:0 0 0 0 rgba(34,197,94,.5);animation:pl 1.8s infinite}
@keyframes pl{70%%{box-shadow:0 0 0 7px rgba(34,197,94,0)}100%%{box-shadow:0 0 0 0 rgba(34,197,94,0)}}
table{width:100%%;border-collapse:collapse}
th{font-size:11.5px;color:var(--muted);font-weight:600;text-align:left;padding:8px 10px;border-bottom:1px solid var(--border);white-space:nowrap}
td{padding:10px;border-bottom:1px solid var(--border);font-size:13px;vertical-align:middle}
tr:last-child td{border-bottom:none}
tbody tr{transition:background .15s}
tbody tr:hover{background:var(--card2)}
tr.flash{animation:fl 1.6s}
@keyframes fl{0%%,60%%{background:rgba(99,102,241,.22)}100%%{background:transparent}}
.sub{color:var(--muted);font-size:11.5px;margin-top:1px}
.row-acts{display:flex;gap:6px;justify-content:flex-end}
.empty{padding:38px 10px;text-align:center;color:var(--muted)}
.empty .big{font-size:34px;margin-bottom:8px;opacity:.7}
.switch{position:relative;width:36px;height:20px;display:inline-block}
.switch input{opacity:0;width:0;height:0}
.switch .sl{position:absolute;inset:0;border-radius:999px;background:var(--card2);border:1px solid var(--border);transition:.15s;cursor:pointer}
.switch .sl::before{content:"";position:absolute;width:14px;height:14px;border-radius:50%%;left:2px;top:2px;background:var(--muted);transition:.15s}
.switch input:checked+.sl{background:linear-gradient(120deg,var(--accent),var(--accent2));border-color:transparent}
.switch input:checked+.sl::before{transform:translateX(16px);background:#fff}
.kv{display:flex;justify-content:space-between;padding:7px 0;border-bottom:1px dashed var(--border);font-size:13px}
.kv:last-child{border-bottom:none}
.kv .k{color:var(--muted)}
.kv .v{font-variant-numeric:tabular-nums}
.quick-grid{display:grid;grid-template-columns:repeat(2,1fr);gap:10px}
.quick-grid .btn{justify-content:flex-start;padding:11px 14px}
.bar-row{display:flex;align-items:center;gap:10px;padding:6px 0;font-size:12.5px}
.bar-row .nm{width:130px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;flex-shrink:0}
.bar-row .bar{flex:1;height:8px;border-radius:99px;background:var(--card2);overflow:hidden}
.bar-row .bar i{display:block;height:100%%;border-radius:99px;background:linear-gradient(90deg,var(--accent),var(--accent2))}
.bar-row .val{width:86px;text-align:right;color:var(--muted);font-variant-numeric:tabular-nums;flex-shrink:0}
.ev{display:flex;gap:10px;padding:7px 0;border-bottom:1px dashed var(--border);font-size:12.5px;align-items:baseline}
.ev:last-child{border-bottom:none}
.ev time{color:var(--muted);font-size:11.5px;white-space:nowrap;font-variant-numeric:tabular-nums}
.ev .msg{flex:1;word-break:break-all}
.pre{background:var(--card2);border:1px solid var(--border);border-radius:10px;padding:12px 14px;font-family:ui-monospace,Consolas,monospace;font-size:12px;line-height:1.7;white-space:pre-wrap;word-break:break-all;position:relative}
.pre .cp{position:absolute;top:8px;right:8px}
.tabs{display:flex;gap:4px;border-bottom:1px solid var(--border);margin-bottom:12px}
.tabs button{padding:8px 14px;font-size:13px;color:var(--muted);border-bottom:2px solid transparent;margin-bottom:-1px}
.tabs button.on{color:var(--text);border-bottom-color:var(--accent);font-weight:600}
.form-row{margin-bottom:12px}
.form-row label{display:block;font-size:12.5px;color:var(--muted);margin-bottom:5px}
.form-2col{display:grid;grid-template-columns:1fr 1fr;gap:12px}
.hint{font-size:11.5px;color:var(--muted);margin-top:4px}

/* ============ 弹窗 ============ */
#modal-root{display:none;position:fixed;inset:0;z-index:200}
.mask{position:absolute;inset:0;background:rgba(3,6,15,.62);backdrop-filter:blur(3px)}
.modal{position:absolute;left:50%%;top:50%%;transform:translate(-50%%,-50%%);width:560px;max-width:calc(100vw - 40px);max-height:calc(100vh - 60px);display:flex;flex-direction:column;background:var(--bg2);border:1px solid var(--border);border-radius:16px;box-shadow:var(--shadow);overflow:hidden}
.modal-head{display:flex;align-items:center;justify-content:space-between;padding:14px 18px;border-bottom:1px solid var(--border);font-weight:700;font-size:15px}
.modal-body{padding:18px;overflow-y:auto}
.modal-foot{padding:13px 18px;border-top:1px solid var(--border);display:flex;justify-content:flex-end;gap:10px}
.icon-btn{width:28px;height:28px;border-radius:8px;color:var(--muted);font-size:13px}
.icon-btn:hover{background:var(--card2);color:var(--text)}
.preset-grid{display:grid;grid-template-columns:repeat(4,1fr);gap:10px}
.preset{padding:14px 8px;border-radius:12px;border:1px solid var(--border);background:var(--card2);text-align:center;cursor:pointer;transition:all .15s}
.preset:hover,.preset.on{border-color:var(--accent);background:rgba(99,102,241,.12);transform:translateY(-2px)}
.preset .pi{font-size:22px}
.preset .pt{font-size:12px;margin-top:6px;font-weight:600}
.preset .pd{font-size:10.5px;color:var(--muted);margin-top:2px}
/* 可点击元素 */
.clickable{cursor:pointer}
.clickable:hover{color:var(--accent)}
tr.rowlink td:first-child{position:relative}
.link-name{cursor:pointer;border-bottom:1px dashed transparent}
.link-name:hover{color:var(--accent);border-bottom-color:var(--accent)}
.stat.clickable{transition:transform .15s,border-color .15s}
.stat.clickable:hover{transform:translateY(-2px);border-color:var(--accent)}
.bar-row.clickable{border-radius:8px;padding:6px 8px;margin:0 -8px}
.bar-row.clickable:hover{background:var(--card2)}
.ev.clickable:hover .msg{color:var(--accent)}
.chip.port{cursor:pointer}
.mini-link{font-size:11.5px;font-weight:500;cursor:pointer}
.mini-link:hover{color:var(--accent)}
/* 向导分组 */
.wiz-step{display:flex;align-items:center;gap:8px;margin-bottom:12px;font-size:12px;color:var(--muted)}
.wiz-step b{color:var(--text)}
.wiz-sec{border:1px solid var(--border);border-radius:12px;padding:14px 14px 4px;margin-bottom:12px;position:relative}
.wiz-sec-title{position:absolute;top:-9px;left:12px;background:var(--bg2);padding:0 8px;font-size:11.5px;font-weight:650;color:var(--accent)}
.pw-inline{display:flex;gap:8px}
.pw-inline input{flex:1}
.pw-inline .btn{flex-shrink:0}
/* 令牌输入 */
.tk-input{font-family:ui-monospace,Consolas,monospace;font-size:12px}
.tk-show{display:flex;align-items:center;gap:8px;background:var(--card2);border:1px solid var(--border);border-radius:10px;padding:8px 12px}
.tk-show .mono{flex:1;word-break:break-all}

/* ============ 命令面板 ============ */
#palette-root{display:none;position:fixed;inset:0;z-index:300}
.palette{position:absolute;left:50%%;top:14vh;transform:translateX(-50%%);width:600px;max-width:calc(100vw - 40px);background:var(--bg2);border:1px solid var(--border);border-radius:16px;box-shadow:var(--shadow);overflow:hidden}
.palette input{border:none;border-radius:0;padding:15px 18px;font-size:15px;background:transparent;border-bottom:1px solid var(--border)}
.pal-list{max-height:46vh;overflow-y:auto;padding:6px}
.pal-item{display:flex;align-items:center;gap:10px;padding:10px 12px;border-radius:10px;cursor:pointer;font-size:13.5px}
.pal-item .sub{margin-left:auto}
.pal-item.on{background:linear-gradient(120deg,rgba(99,102,241,.25),rgba(139,92,246,.15))}
.pal-item .pi{width:22px;text-align:center}
.pal-foot{padding:8px 14px;border-top:1px solid var(--border);font-size:11px;color:var(--muted);display:flex;gap:14px}

/* ============ Toast ============ */
#toast-root{position:fixed;top:18px;right:18px;z-index:400;display:flex;flex-direction:column;gap:8px}
.toast{padding:10px 16px;border-radius:12px;background:var(--bg2);border:1px solid var(--border);box-shadow:var(--shadow);font-size:13px;opacity:0;transform:translateX(20px);transition:all .25s;max-width:320px}
.toast.show{opacity:1;transform:none}
.toast.ok{border-color:rgba(34,197,94,.5)}
.toast.err{border-color:rgba(239,68,68,.5)}
.toast.warn{border-color:rgba(245,158,11,.5)}

@media(max-width:960px){
  #sidebar{position:fixed;left:0;top:0;z-index:100;transform:translateX(-100%%);transition:transform .2s;background:var(--bg2)}
  #sidebar.open{transform:none}
  #burger{display:block}
  .grid-4{grid-template-columns:repeat(2,1fr)}
  .grid-3,.grid-2{grid-template-columns:1fr}
  .preset-grid{grid-template-columns:repeat(2,1fr)}
  #content{padding:16px}
  #top-ann{display:none}
}
</style>
</head>
<body>
<div id="app"></div>
<div id="shell">
  <aside id="sidebar">
    <div class="brand">rtp<small>控制台</small></div>
    <nav id="nav"></nav>
    <div class="side-foot">
      <button class="btn sm" id="theme-btn" title="切换亮暗主题">🌙 暗色模式</button>
      <div class="accents" id="accents"></div>
      <div style="text-align:center;font-size:10.5px;color:var(--muted)" id="ver-foot"></div>
    </div>
  </aside>
  <div id="main">
    <header id="topbar">
      <button id="burger" class="icon-btn">☰</button>
      <div id="page-title">仪表盘</div>
      <div id="top-ann"></div>
      <button class="btn sm" id="palette-btn" title="命令面板 (Ctrl+K)">🔍 搜索 <span class="kbd">Ctrl K</span></button>
      <div id="user-chip"><span class="ava" id="user-ava">A</span><span id="user-name">admin</span> ▾</div>
    </header>
    <div id="content"></div>
  </div>
</div>
<div id="modal-root"></div>
<div id="palette-root"></div>
<div id="toast-root"></div>

<script>
var _p=[%d,%d,%d,%d,%d,%d];
var VERSION='';
var NAV=[['dashboard','📊','仪表盘'],['clients','🖥','客户端'],['tunnels','🔀','隧道'],['tools','🧰','工具'],['events','📜','事件'],['settings','⚙️','设置']];
var state={me:null,view:'dashboard',clients:[],tunnels:[],events:[],stats:null,settings:null,webhooks:[],eventTypes:{},
  search:{clients:'',tunnels:'',events:''},tf:'all',ef:'all',hist:[],last:null,lastTs:0,
  probeHist:[],wiz:null,palIdx:0,palItems:[],palShown:[],deployTab:'linux'};

/* ---------- 基础 ---------- */
function $(id){return document.getElementById(id)}
function esc(s){return String(s==null?'':s).replace(/[&<>"']/g,function(c){return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]})}
function fmtBytes(b){b=+b||0;if(b<1024)return b+' B';if(b<1048576)return (b/1024).toFixed(1)+' KB';if(b<1073741824)return (b/1048576).toFixed(1)+' MB';return (b/1073741824).toFixed(2)+' GB'}
function fmtRate(v){v=+v||0;if(v<1024)return Math.round(v)+' B/s';if(v<1048576)return (v/1024).toFixed(1)+' KB/s';return (v/1048576).toFixed(2)+' MB/s'}
function relTime(ts){var d=Date.now()/1000-ts;if(d<0)d=0;if(d<60)return '刚刚';if(d<3600)return Math.floor(d/60)+' 分钟前';if(d<86400)return Math.floor(d/3600)+' 小时前';return Math.floor(d/86400)+' 天前'}
function lsToken(){try{return localStorage.getItem('rtp_tk')||''}catch(e){return ''}}
function api(path,opts){
  opts=opts||{};opts.headers=opts.headers||{};
  var t=lsToken();if(t)opts.headers['X-Token']=t;
  return fetch(path,opts).then(function(res){
    if(res.status===401){if(t){try{localStorage.removeItem('rtp_tk')}catch(e){}}showLogin('登录状态已过期，请重新登录');throw new Error('401')}
    return res;
  });
}
function apiJSON(path,opts){
  return api(path,opts).then(function(r){
    if(!r.ok){
      return r.text().then(function(m){
        var msg=m||('HTTP '+r.status);
        try{var j=JSON.parse(m);if(j&&j.error)msg=j.error}catch(e){}
        var err=new Error(msg);err.status=r.status;throw err;
      });
    }
    return r.json();
  });
}
function setText(id,v){var e=$(id);if(e)e.textContent=v}
function toast(msg,type){
  var d=document.createElement('div');d.className='toast '+(type||'ok');d.textContent=msg;
  $('toast-root').appendChild(d);
  requestAnimationFrame(function(){d.classList.add('show')});
  setTimeout(function(){d.classList.remove('show');setTimeout(function(){d.remove()},300)},3200);
}
function copyText(t,msg){
  var done=function(){toast(msg||'已复制')};
  var fb=function(){var ta=document.createElement('textarea');ta.value=t;document.body.appendChild(ta);ta.select();try{document.execCommand('copy')}catch(e){}ta.remove();done()};
  if(navigator.clipboard&&navigator.clipboard.writeText){navigator.clipboard.writeText(t).then(done,fb)}else fb();
}
function protoLabel(p){return {tcp:'TCP',udp:'UDP',http:'HTTP',https:'HTTPS',stcp:'STCP',p2p:'P2P'}[p]||p}
function publicHost(){return location.hostname||'127.0.0.1'}

/* ---------- 登录 ---------- */
function showLogin(err){
  $('shell').classList.remove('on');
  var me=state.me||{};
  $('app').innerHTML='<div class="login-wrap"><div class="login-card glass">'
    +'<div class="login-logo">rtp</div><div class="login-sub">反向隧道代理 · 管理控制台</div>'
    +(me.announcement?'<div class="ann-banner" style="margin-top:16px">📢 '+esc(me.announcement)+'</div>':'')
    +'<div class="login-tabs"><button id="tab-acc" class="on" onclick="switchLoginTab(0)">账号登录</button><button id="tab-tok" onclick="switchLoginTab(1)">令牌登录</button></div>'
    +'<div id="pane-acc">'
    +'<input id="lg-user" placeholder="用户名" autocomplete="username">'
    +'<div class="pw-wrap"><input id="lg-pass" type="password" placeholder="密码" autocomplete="current-password"><button onclick="var p=$(\'lg-pass\');p.type=p.type===\'password\'?\'text\':\'password\'">显示</button></div>'
    +'<button class="btn primary block" id="lg-btn" onclick="doLogin()">登 录</button>'
    +'<div class="login-hint">默认账号 <b>admin</b> / <b>admin123</b>，登录后请在「设置」中修改</div>'
    +'</div>'
    +'<div id="pane-tok" style="display:none">'
    +'<input id="lg-token" placeholder="管理员令牌 admin_token" class="mono">'
    +'<button class="btn primary block" onclick="doTokenLogin()">进入控制台</button>'
    +'<div class="login-hint">令牌即服务端配置文件中的 admin_token</div>'
    +'</div>'
    +'<div class="login-err">'+(err?esc(err):'')+'</div>'
    +'</div></div>';
  setTimeout(function(){var u=$('lg-user');if(u)u.focus()},50);
}
function switchLoginTab(i){
  $('tab-acc').className=i===0?'on':'';
  $('tab-tok').className=i===1?'on':'';
  $('pane-acc').style.display=i===0?'':'none';
  $('pane-tok').style.display=i===1?'':'none';
  var e=$('lg-err');if(e)e.textContent='';
}
function doLogin(){
  var u=$('lg-user').value.trim(),p=$('lg-pass').value;
  if(!u||!p){var e=$('lg-err');if(e)e.textContent='请输入用户名和密码';return}
  var b=$('lg-btn');b.textContent='登录中…';b.disabled=true;
  fetch('/api/login',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({username:u,password:p})})
  .then(function(r){return r.json().then(function(j){return {ok:r.ok,j:j}})})
  .then(function(o){
    b.textContent='登 录';b.disabled=false;
    if(o.ok&&o.j.ok){toast('欢迎回来，'+o.j.user);boot()}
    else{var e=$('lg-err');if(e)e.textContent=o.j.error||'登录失败'}
  }).catch(function(){b.textContent='登 录';b.disabled=false;toast('网络错误','err')});
}
function doTokenLogin(){
  var t=$('lg-token').value.trim();
  if(!t){toast('请输入令牌','warn');return}
  try{localStorage.setItem('rtp_tk',t)}catch(e){}
  boot();
}
function doLogout(){
  fetch('/api/logout',{method:'POST'}).finally(function(){
    try{localStorage.removeItem('rtp_tk')}catch(e){}
    state.me=null;showLogin();
  });
}

/* ---------- 启动 ---------- */
function boot(){
  var h={};var t=lsToken();if(t)h['X-Token']=t;
  fetch('/api/me',{headers:h}).then(function(r){return r.json()}).then(function(me){
    state.me=me;VERSION=me.version||'';
    if(me.authed){initApp()}else{showLogin()}
  }).catch(function(){showLogin('无法连接服务器')});
}
function initApp(){
  $('app').innerHTML='';
  $('shell').classList.add('on');
  var nav='';NAV.forEach(function(n,i){nav+='<a data-v="'+n[0]+'" onclick="go(\''+n[0]+'\')"><span class="ico">'+n[1]+'</span>'+n[2]+'<span class="kbd" style="margin-left:auto">'+(i+1)+'</span></a>'});
  $('nav').innerHTML=nav;
  $('ver-foot').textContent='rtp v'+(VERSION||'');
  $('user-name').textContent=state.me.user||'admin';
  $('user-ava').textContent=(state.me.user||'a').charAt(0).toUpperCase();
  $('user-chip').onclick=function(){openModal('账号','<div class="kv"><span class="k">当前用户</span><span class="v">'+esc(state.me.user||'')+'</span></div><div class="kv"><span class="k">登录方式</span><span class="v">'+(state.me.via==='token'?'管理员令牌':'账号密码')+'</span></div><div class="kv"><span class="k">版本</span><span class="v">'+(VERSION||'')+'</span></div>','<button class="btn" onclick="closeModal()">关闭</button><button class="btn danger" onclick="closeModal();doLogout()">退出登录</button>')};
  $('burger').onclick=function(){$('sidebar').classList.toggle('open')};
  $('palette-btn').onclick=openPalette;
  try{
    if(localStorage.getItem('rtp_theme')==='light')document.body.classList.add('light');
    var ac=localStorage.getItem('rtp_accent')||'indigo';document.documentElement.dataset.accent=ac;
  }catch(e){}
  syncThemeBtn();renderAccents();
  $('theme-btn').onclick=toggleTheme;
  $('top-ann').textContent=state.me.announcement||'';
  go('dashboard');
  loadAll();
  try{
    var es=new EventSource('/api/stream'+(lsToken()?'?token='+encodeURIComponent(lsToken()):''));
    es.onmessage=function(ev){try{onSnap(JSON.parse(ev.data))}catch(e){}};
    es.onerror=function(){};
  }catch(e){}
  setInterval(loadStats,5000);
}
function renderAccents(){
  var cols=[['indigo','#6366f1'],['emerald','#10b981'],['rose','#f43f5e'],['amber','#f59e0b'],['sky','#0ea5e9']];
  var h='';cols.forEach(function(c){h+='<div class="acc-dot'+((document.documentElement.dataset.accent||'indigo')===c[0]?' on':'')+'" style="background:'+c[1]+'" title="'+c[0]+'" onclick="setAccent(\''+c[0]+'\')"></div>'});
  $('accents').innerHTML=h;
}
function setAccent(a){document.documentElement.dataset.accent=a;try{localStorage.setItem('rtp_accent',a)}catch(e){}renderAccents();toast('主题配色已切换')}
function toggleTheme(){document.body.classList.toggle('light');try{localStorage.setItem('rtp_theme',document.body.classList.contains('light')?'light':'dark')}catch(e){}syncThemeBtn()}
function syncThemeBtn(){$('theme-btn').textContent=document.body.classList.contains('light')?'☀️ 亮色模式':'🌙 暗色模式'}
function go(v){
  state.view=v;
  var a=document.querySelectorAll('#nav a');
  for(var i=0;i<a.length;i++)a[i].classList.toggle('on',a[i].getAttribute('data-v')===v);
  var t='';NAV.forEach(function(n){if(n[0]===v)t=n[2]});
  $('page-title').textContent=t;
  $('sidebar').classList.remove('open');
  render();
}
function render(){
  var c=$('content');
  if(state.view==='dashboard')c.innerHTML=viewDashboard();
  else if(state.view==='clients')c.innerHTML=viewClients();
  else if(state.view==='tunnels')c.innerHTML=viewTunnels();
  else if(state.view==='tools')c.innerHTML=viewTools();
  else if(state.view==='events')c.innerHTML=viewEvents();
  else if(state.view==='settings'){c.innerHTML=viewSettings();renderWebhooks()}
  updateLive();
}

/* ---------- 数据 ---------- */
function loadAll(){
  apiJSON('/api/overview').then(function(ov){
    state.clients=ov.clients||[];state.tunnels=ov.tunnels||[];
    if((ov.events||[]).length)state.events=ov.events;
    if(state.view)render();
  }).catch(function(){});
  loadSettings();loadStats();loadWebhooks();
}
function loadWebhooks(){
  apiJSON('/api/webhooks').then(function(d){state.webhooks=d.webhooks||[];state.eventTypes=d.event_types||{};if(state.view==='settings')renderWebhooks()}).catch(function(){});
}
function loadSettings(){
  apiJSON('/api/settings').then(function(s){state.settings=s;renderRuntime();renderPorts()}).catch(function(){});
}
function loadStats(){
  apiJSON('/api/stats').then(function(s){state.stats=s;renderRuntime()}).catch(function(){});
}
function onSnap(snap){
  var cm={},tm={},i;
  for(i=0;i<(snap.clients||[]).length;i++)cm[snap.clients[i].id]=snap.clients[i];
  for(i=0;i<state.clients.length;i++){var lc=cm[state.clients[i].id];if(lc){state.clients[i].online=lc.online;state.clients[i].up=lc.up;state.clients[i].down=lc.down;state.clients[i].remote_addr=lc.remote_addr}}
  for(i=0;i<(snap.tunnels||[]).length;i++)tm[snap.tunnels[i].id]=snap.tunnels[i];
  for(i=0;i<state.tunnels.length;i++){var lt=tm[state.tunnels[i].id];if(lt){state.tunnels[i].online=lt.online;state.tunnels[i].up=lt.up;state.tunnels[i].down=lt.down;if(lt.p2p_mode)state.tunnels[i].p2p_mode=lt.p2p_mode}}
  if(snap.events)state.events=snap.events;
  var now=Date.now();
  if(state.last){
    var dt=(now-state.lastTs)/1000;if(dt<=0)dt=1;
    var ur=(snap.total_up-state.last.up)/dt,dr=(snap.total_down-state.last.down)/dt;
    if(ur<0)ur=0;if(dr<0)dr=0;
    state.hist.push({u:ur,d:dr});if(state.hist.length>60)state.hist.shift();
  }
  state.last={up:snap.total_up||0,down:snap.total_down||0};state.lastTs=now;
  updateLive();
}
function updateLive(){
  var i,online=0;
  for(i=0;i<state.clients.length;i++)if(state.clients[i].online)online++;
  setText('st-online',online+' / '+state.clients.length);
  setText('st-online-sub','共 '+state.clients.length+' 个客户端');
  var en=0;for(i=0;i<state.tunnels.length;i++)if(state.tunnels[i].enabled!==false)en++;
  setText('st-tunnels',String(state.tunnels.length));
  setText('st-tunnels-sub',en+' 条已启用');
  var ur=0,dr=0;
  if(state.hist.length){ur=state.hist[state.hist.length-1].u;dr=state.hist[state.hist.length-1].d}
  setText('st-up',fmtRate(ur));setText('st-down',fmtRate(dr));
  drawChart();renderTop();renderEventsMini();renderRuntime();renderPorts();
  if(state.view==='clients')renderClientsBody();
  if(state.view==='tunnels')renderTunnelsBody();
  if(state.view==='events')renderEventsBody();
}
function drawChart(){
  var cv=$('bw');if(!cv)return;
  var dpr=window.devicePixelRatio||1,W=cv.clientWidth,H=190;
  if(!W)return;
  if(cv.width!==Math.round(W*dpr)){cv.width=Math.round(W*dpr);cv.height=Math.round(H*dpr)}
  var ctx=cv.getContext('2d');ctx.setTransform(dpr,0,0,dpr,0,0);ctx.clearRect(0,0,W,H);
  var hist=state.hist;if(!hist.length){ctx.fillStyle='rgba(139,147,173,.6)';ctx.font='12px system-ui';ctx.fillText('等待数据…',10,20);return}
  var max=0,i;
  for(i=0;i<hist.length;i++){if(hist[i].u>max)max=hist[i].u;if(hist[i].d>max)max=hist[i].d}
  max=max*1.3;if(max<=0)max=1024;
  var cs=getComputedStyle(document.body);
  var bd=cs.getPropertyValue('--border').trim()||'rgba(255,255,255,.1)';
  var up=cs.getPropertyValue('--up').trim()||'#38bdf8',dn=cs.getPropertyValue('--down').trim()||'#34d399';
  var mu=cs.getPropertyValue('--muted').trim()||'#888';
  ctx.strokeStyle=bd;ctx.lineWidth=1;
  for(var g=1;g<=4;g++){var y=H-H*g/5;ctx.beginPath();ctx.moveTo(0,y);ctx.lineTo(W,y);ctx.stroke()}
  function series(key,color,fill){
    ctx.beginPath();
    for(var i=0;i<hist.length;i++){
      var x=W-(hist.length-1-i)*(W/59),y=H-(hist[i][key]/max)*(H-10)-5;
      if(i===0)ctx.moveTo(x,y);else ctx.lineTo(x,y);
    }
    ctx.strokeStyle=color;ctx.lineWidth=2;ctx.stroke();
    if(fill){
      ctx.lineTo(W,H);ctx.lineTo(W-(hist.length-1)*(W/59),H);ctx.closePath();
      var gr=ctx.createLinearGradient(0,0,0,H);gr.addColorStop(0,color+'44');gr.addColorStop(1,color+'00');
      ctx.fillStyle=gr;ctx.fill();
    }
  }
  series('u',up,true);series('d',dn,true);
  ctx.fillStyle=mu;ctx.font='11px system-ui';
  ctx.fillText('峰值 '+fmtRate(max/1.3),8,14);
}

/* ---------- 视图：仪表盘 ---------- */
function viewDashboard(){
  var ann=state.me&&state.me.announcement;
  var h='<div class="page-head"><div><h1>仪表盘</h1><div class="page-sub">实时掌握服务与隧道状态</div></div>'
    +'<div style="display:flex;gap:8px"><button class="btn" onclick="go(\'tunnels\');openWizard()">＋ 隧道向导</button><button class="btn primary" onclick="openClientModal()">＋ 新建客户端</button></div></div>'
    +(ann?'<div class="ann-banner" style="margin-bottom:14px">📢 '+esc(ann)+'</div>':'')
    +'<div class="grid-4">'
    +'<div class="card glass stat clickable" onclick="go(\'clients\')" title="点击查看客户端"><div class="stat-label">在线客户端 ↗</div><div class="stat-val" id="st-online">-</div><div class="stat-sub" id="st-online-sub"></div></div>'
    +'<div class="card glass stat clickable" onclick="go(\'tunnels\')" title="点击管理隧道"><div class="stat-label">隧道总数 ↗</div><div class="stat-val" id="st-tunnels">-</div><div class="stat-sub" id="st-tunnels-sub"></div></div>'
    +'<div class="card glass stat"><div class="stat-label">实时上行 <span class="dot up"></span></div><div class="stat-val" id="st-up" style="color:var(--up)">-</div><div class="stat-sub">外网 → 内网</div></div>'
    +'<div class="card glass stat"><div class="stat-label">实时下行 <span class="dot down"></span></div><div class="stat-val" id="st-down" style="color:var(--down)">-</div><div class="stat-sub">内网 → 外网</div></div>'
    +'</div>'
    +'<div class="card glass"><div class="card-head"><span>📈 实时带宽（近 60 秒）</span><span class="legend"><span><i class="dot up"></i>上行</span><span><i class="dot down"></i>下行</span></span></div><canvas id="bw"></canvas></div>'
    +'<div class="grid-2">'
    +'<div class="card glass"><div class="card-head">💓 运行状态</div><div id="rt-box"></div></div>'
    +'<div class="card glass"><div class="card-head">⚡ 快捷操作</div><div class="quick-grid">'
    +'<button class="btn" onclick="openClientModal()">🖥 新建客户端</button>'
    +'<button class="btn" onclick="openWizard()">🧙 隧道向导</button>'
    +'<button class="btn" onclick="openProbe()">📡 连通性测试</button>'
    +'<button class="btn" onclick="backupConfig()">💾 备份配置</button>'
    +'<button class="btn" onclick="go(\'settings\')">⬇️ 下载客户端</button>'
    +'<button class="btn" onclick="go(\'tools\')">🧰 更多工具</button>'
    +'</div></div>'
    +'</div>'
    +'<div class="grid-2">'
    +'<div class="card glass"><div class="card-head"><span>🏆 流量 Top 5 隧道</span><span class="mini-link" onclick="go(\'tunnels\')">管理全部 →</span></div><div id="top-list"></div></div>'
    +'<div class="card glass"><div class="card-head"><span>🕐 最近事件</span><span class="mini-link" onclick="go(\'events\')">查看全部 →</span></div><div id="events-mini"></div></div>'
    +'</div>'
    +'<div class="card glass"><div class="card-head"><span>🔌 服务端口</span><span class="mini-link" onclick="go(\'settings\')">端口设置 →</span></div><div class="chips" id="ports-box"></div><div class="hint" style="margin-top:10px">公网 TCP/UDP 隧道端口按需自动分配；HTTP/HTTPS 需在配置文件中启用并放行防火墙。</div></div>';
  return h;
}
function renderRuntime(){
  var st=state.stats;
  if(!st){var b=$('rt-box');if(b)b.innerHTML='<div class="empty">加载中…</div>';return}
  var b=$('rt-box');if(!b)return;
  b.innerHTML='<div class="kv"><span class="k">版本</span><span class="v">rtp v'+esc(st.version||VERSION)+'</span></div>'
    +'<div class="kv"><span class="k">Go 版本</span><span class="v">'+esc(st.go_version||'-')+'</span></div>'
    +'<div class="kv"><span class="k">运行时长</span><span class="v">'+esc(st.uptime_human||'-')+'</span></div>'
    +'<div class="kv"><span class="k">内存（堆 / 系统）</span><span class="v">'+esc(st.mem_alloc_mb||'0')+' MB / '+esc(st.mem_sys_mb||'0')+' MB</span></div>'
    +'<div class="kv"><span class="k">Goroutines / GC</span><span class="v">'+esc(String(st.goroutines||0))+' / '+esc(String(st.gc_count||0))+'</span></div>';
}
function renderPorts(){
  var s=state.settings;if(!s||!$('ports-box'))return;
  var ps=[['控制端口',s.control_port,'客户端长连接'],['工作端口',s.work_port,'隧道数据回连'],['Web 控制台',s.web_port,'当前页面'],['HTTP',s.http_port,'域名虚拟主机'],['HTTPS',s.https_port,'SNI 透传'],['P2P 信令',s.p2p_port,'UDP 打洞协调']];
  var h='';
  ps.forEach(function(p){h+='<span class="chip port" title="点击到设置页" onclick="go(\'settings\')"><b style="color:var(--text)">'+esc(p[0])+'</b> <span class="mono">'+(p[1]>0?p[1]:'未启用')+'</span> <span style="opacity:.7">'+esc(p[2])+'</span></span>'});
  $('ports-box').innerHTML=h;
}
function renderTop(){
  var box=$('top-list');if(!box)return;
  var ts=state.tunnels.slice().sort(function(a,b){return (b.up+b.down)-(a.up+a.down)}).slice(0,5);
  if(!ts.length){box.innerHTML='<div class="empty"><div class="big">📭</div>暂无隧道，<span class="mini-link" onclick="openWizard()">立即创建 →</span></div>';return}
  var max=(ts[0].up+ts[0].down)||1,h='';
  ts.forEach(function(t){
    var v=t.up+t.down,w=Math.round(v/max*100);
    h+='<div class="bar-row clickable" title="点击编辑该隧道" onclick="go(\'tunnels\');openTunnelEdit(\''+esc(t.id)+'\')"><span class="nm">'+esc(t.name)+'</span><span class="bar"><i style="width:'+w+'%%"></i></span><span class="val">'+fmtBytes(v)+'</span></div>';
  });
  box.innerHTML=h;
}
function renderEventsMini(){
  var box=$('events-mini');if(!box)return;
  var ev=(state.events||[]).slice(0,6);
  if(!ev.length){box.innerHTML='<div class="empty">暂无事件</div>';return}
  var h='';ev.forEach(function(e){
    var c=e.level==='ok'?'var(--ok)':(e.level==='warn'?'var(--warn)':'var(--err)');
    h+='<div class="ev clickable" title="点击查看全部事件" onclick="go(\'events\')"><span class="dot" style="background:'+c+'"></span><time>'+esc(relTime(e.ts))+'</time><span class="msg">'+esc(e.msg)+'</span></div>';
  });
  box.innerHTML=h;
}
setInterval(function(){if(state.view==='dashboard')renderRuntime()},5000);
setInterval(function(){if(state.view==='dashboard')renderPorts()},5000);

/* ---------- 视图：客户端 ---------- */
function viewClients(){
  return '<div class="page-head"><div><h1>客户端</h1><div class="page-sub">每个客户端独立令牌，可归属多条隧道</div></div>'
    +'<button class="btn primary" onclick="openClientModal()">＋ 新建客户端</button></div>'
    +'<div class="toolbar"><input placeholder="🔍 搜索名称 / ID / 备注" value="'+esc(state.search.clients)+'" oninput="state.search.clients=this.value;renderClientsBody()"><span class="count" id="cl-count"></span></div>'
    +'<div class="card glass" style="padding:6px 12px"><table><thead><tr><th style="width:36px"></th><th>客户端</th><th>令牌</th><th>远程地址</th><th>流量</th><th>限速</th><th style="text-align:right">操作</th></tr></thead><tbody id="clients-body"></tbody></table></div>';
}
function renderClientsBody(){
  var tb=$('clients-body');if(!tb)return;
  var q=state.search.clients.toLowerCase(),rows=[],i;
  for(i=0;i<state.clients.length;i++){
    var c=state.clients[i];
    if(q&&(c.name+' '+c.id+' '+(c.note||'')).toLowerCase().indexOf(q)<0)continue;
    rows.push(c);
  }
  setText('cl-count','共 '+rows.length+' 个');
  if(!rows.length){tb.innerHTML='<tr><td colspan="7"><div class="empty"><div class="big">🖥</div>暂无客户端，点击右上角新建</div></td></tr>';return}
  var h='';
  rows.forEach(function(c){
    var tk=c.token||'';
    var mask=tk.length>12?tk.slice(0,6)+'…'+tk.slice(-4):tk;
    h+='<tr id="cl-'+esc(c.id)+'">'
      +'<td><span class="pulse'+(c.online?' on':'')+'" title="'+(c.online?'在线':'离线')+'"></span></td>'
      +'<td><span class="link-name" title="点击编辑该客户端" onclick="openClientEdit(\''+esc(c.id)+'\')"><b>'+esc(c.name)+'</b></span>'+(c.note?'<div class="sub">'+esc(c.note)+'</div>':'')+'<div class="sub mono">'+esc(c.id)+'</div></td>'
      +'<td><span class="mono" title="点击复制" style="cursor:pointer" onclick="copyText(\''+esc(tk)+'\',\'令牌已复制\')">'+esc(mask)+'</span> <button class="btn sm" onclick="copyText(\''+esc(tk)+'\',\'令牌已复制\')">复制</button></td>'
      +'<td class="mono">'+esc(c.remote_addr||'-')+'</td>'
      +'<td class="mono">↑'+fmtBytes(c.up)+'<br>↓'+fmtBytes(c.down)+'</td>'
      +'<td>'+(c.bw_limit_mbps>0?'<span class="badge ok">'+esc(c.bw_limit_mbps)+' Mbps</span>':'<span class="badge gray">不限</span>')+'</td>'
      +'<td><div class="row-acts">'
      +'<button class="btn sm" onclick="openClientEdit(\''+esc(c.id)+'\')">编辑</button>'
      +'<button class="btn sm" onclick="openCmdModal(\''+esc(c.id)+'\')">连接命令</button>'
      +'<button class="btn sm" onclick="openRateModal(\''+esc(c.id)+'\')">限速</button>'
      +'<button class="btn sm danger" onclick="delClient(\''+esc(c.id)+'\')">删除</button>'
      +'</div></td></tr>';
  });
  tb.innerHTML=h;
}
function genToken(){
  var cs='abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789',s='';
  for(var i=0;i<32;i++)s+=cs[Math.floor(Math.random()*cs.length)];
  return s;
}
function tokenField(id,val){
  return '<div class="pw-inline"><input id="'+id+'" class="tk-input" value="'+esc(val||'')+'" placeholder="留空自动生成；或填 8-64 位自定义令牌">'
    +'<button class="btn sm" type="button" title="随机生成" onclick="$(\''+id+'\').value=genToken()">🎲</button></div>'
    +'<div class="hint">仅限字母、数字、- 、_；长度 8-64，不能与其他客户端或管理员令牌重复。</div>';
}
function openClientModal(){
  openModal('新建客户端',
    '<div class="form-row"><label>名称</label><input id="nc-name" placeholder="例如：家里台式机"></div>'
    +'<div class="form-row"><label>备注（可选）</label><input id="nc-note" placeholder="例如：客厅 NAS"></div>'
    +'<div class="form-row"><label>限速（Mbps，0 = 不限速）</label><input id="nc-bw" type="number" min="0" value="0"></div>'
    +'<div class="form-row"><label>接入令牌</label>'+tokenField('nc-token','')+'</div>',
    '<button class="btn" onclick="closeModal()">取消</button><button class="btn primary" onclick="createClient()">创建</button>');
  setTimeout(function(){var e=$('nc-name');if(e)e.focus()},60);
}
function createClient(){
  var body={name:$('nc-name').value.trim()||'客户端',note:$('nc-note').value.trim(),bw_limit_mbps:parseInt($('nc-bw').value||'0',10)||0,token:$('nc-token').value.trim()};
  apiJSON('/api/clients',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)})
  .then(function(c){toast('客户端已创建：'+c.name);closeModal();loadAll()})
  .catch(function(r){r.text().then(function(m){toast(m||'创建失败','err')})});
}
function openClientEdit(id){
  var c=null;for(var i=0;i<state.clients.length;i++)if(state.clients[i].id===id)c=state.clients[i];
  if(!c)return;
  openModal('编辑客户端 · '+esc(c.name),
    '<div class="form-row"><label>名称</label><input id="ce-name" value="'+esc(c.name)+'"></div>'
    +'<div class="form-row"><label>备注</label><input id="ce-note" value="'+esc(c.note||'')+'"></div>'
    +'<div class="form-row"><label>限速（Mbps，0 = 不限速）</label><input id="ce-bw" type="number" min="0" value="'+(c.bw_limit_mbps||0)+'"></div>'
    +'<div class="form-row"><label>当前令牌</label><div class="tk-show"><span class="mono">'+esc(c.token||'')+'</span>'
    +'<button class="btn sm" onclick="copyText(\''+esc(c.token||'')+'\',\'令牌已复制\')">复制</button></div></div>'
    +'<div class="form-row"><label>更换令牌（留空保持不变）</label>'+tokenField('ce-token','')+'</div>'
    +'<div class="hint">⚠️ 更换令牌后，旧令牌在该客户端下次重连后失效，需用新令牌重新运行 rtpc。</div>',
    '<button class="btn" onclick="closeModal()">取消</button><button class="btn primary" onclick="saveClientEdit(\''+esc(id)+'\')">保存</button>');
}
function saveClientEdit(id){
  var body={name:$('ce-name').value.trim(),note:$('ce-note').value.trim(),bw_limit_mbps:parseInt($('ce-bw').value||'0',10)||0};
  var tk=$('ce-token').value.trim();
  if(tk)body.token=tk;
  apiJSON('/api/clients/'+id,{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)})
  .then(function(){toast('客户端已保存');closeModal();loadAll()})
  .catch(function(r){r.text().then(function(m){toast(m||'保存失败','err')})});
}
function openRateModal(id){
  var c=null;for(var i=0;i<state.clients.length;i++)if(state.clients[i].id===id)c=state.clients[i];
  if(!c)return;
  openModal('限速 · '+c.name,
    '<div class="form-row"><label>带宽上限（Mbps，0 = 不限速）</label><input id="rt-bw" type="number" min="0" value="'+(c.bw_limit_mbps||0)+'"></div>'
    +'<div class="hint">令牌桶算法，对该客户端全部隧道生效，可随时调整。</div>',
    '<button class="btn" onclick="closeModal()">取消</button><button class="btn primary" onclick="saveRate(\''+esc(id)+'\')">保存</button>');
}
function saveRate(id){
  var v=parseInt($('rt-bw').value||'0',10)||0;
  apiJSON('/api/clients/'+id,{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({bw_limit_mbps:v})})
  .then(function(){toast('限速已更新');closeModal();loadAll()})
  .catch(function(){toast('更新失败','err')});
}
function openCmdModal(id){
  var c=null;for(var i=0;i<state.clients.length;i++)if(state.clients[i].id===id)c=state.clients[i];
  if(!c)return;
  var cmd='rtpc -server '+publicHost()+':'+_p[1]+' -token '+c.token+' -name '+c.name;
  var dl='http://'+location.host+'/api/download?name=rtpc.exe'+(lsToken()?'&token='+encodeURIComponent(lsToken()):'');
  openModal('连接命令 · '+c.name,
    '<div class="form-row"><label>1. 下载客户端（「设置」页也可直接下载）</label><div class="pre"><button class="btn sm cp" onclick="copyText(\''+esc(dl)+'\',\'下载链接已复制\')">复制</button>'+esc(dl)+'</div></div>'
    +'<div class="form-row"><label>2. 在需要被访问的电脑上运行</label><div class="pre"><button class="btn sm cp" onclick="copyText(\''+esc(cmd)+'\',\'命令已复制\')">复制</button>'+esc(cmd)+'</div></div>'
    +'<div class="hint">运行后回到控制台「隧道」页添加规则即可，无需重启客户端。</div>',
    '<button class="btn primary" onclick="closeModal()">完成</button>');
}
function delClient(id){
  var c=null;for(var i=0;i<state.clients.length;i++)if(state.clients[i].id===id)c=state.clients[i];
  if(!c)return;
  openModal('删除客户端',
    '<p>确定删除客户端 <b>'+esc(c.name)+'</b> 吗？其名下隧道也会一并删除，该操作不可恢复。</p>',
    '<button class="btn" onclick="closeModal()">取消</button><button class="btn danger" onclick="closeModal();api(\'/api/clients/'+esc(id)+'\',{method:\'DELETE\'}).then(function(){toast(\'已删除\');loadAll()})">确认删除</button>');
}

/* ---------- 视图：隧道 ---------- */
function viewTunnels(){
  var chips=[['all','全部'],['tcp','TCP'],['udp','UDP'],['http','HTTP'],['https','HTTPS'],['stcp','STCP'],['p2p','P2P']];
  var h='<div class="page-head"><div><h1>隧道</h1><div class="page-sub">TCP / UDP / HTTP / HTTPS / STCP / P2P 直连</div></div>'
    +'<button class="btn primary" onclick="openWizard()">＋ 向导新建</button></div>'
    +'<div class="toolbar">';
  chips.forEach(function(c){h+='<button class="chip'+(state.tf===c[0]?' on':'')+'" onclick="state.tf=\''+c[0]+'\';render()">'+c[1]+'</button>'});
  h+='<input placeholder="🔍 搜索隧道" value="'+esc(state.search.tunnels)+'" oninput="state.search.tunnels=this.value;renderTunnelsBody()" style="margin-left:auto"><span class="count" id="tu-count"></span></div>'
    +'<div class="card glass" style="padding:6px 12px"><table><thead><tr><th style="width:44px">启用</th><th>名称</th><th>协议</th><th>归属</th><th>公网地址</th><th>内网目标</th><th>流量</th><th>状态</th><th style="text-align:right">操作</th></tr></thead><tbody id="tunnels-body"></tbody></table></div>';
  return h;
}
function renderTunnelsBody(){
  var tb=$('tunnels-body');if(!tb)return;
  var q=state.search.tunnels.toLowerCase(),rows=[],i;
  for(i=0;i<state.tunnels.length;i++){
    var t=state.tunnels[i];
    if(state.tf!=='all'&&t.proto!==state.tf)continue;
    if(q&&(t.name+' '+t.id+' '+(t.note||'')).toLowerCase().indexOf(q)<0)continue;
    rows.push(t);
  }
  setText('tu-count','共 '+rows.length+' 条');
  if(!rows.length){tb.innerHTML='<tr><td colspan="9"><div class="empty"><div class="big">🔀</div>暂无隧道，用「向导新建」开启第一条</div></td></tr>';return}
  var h='';
  rows.forEach(function(t){
    var addr='';
    if(t.proto==='tcp'||t.proto==='udp'||t.proto==='stcp')addr=publicHost()+':'+t.public_port;
    else if(t.proto==='http'||t.proto==='https'){
      var dom=t.domain||(t.subdomain?(state.settings&&state.settings.base_domain?t.subdomain+'.'+state.settings.base_domain:t.subdomain):'');
      addr=dom||'未设置域名';
    }
    else if(t.proto==='p2p')addr='127.0.0.1:'+t.visitor_port+' → 服务方';
    var st='';
    if(t.proto==='p2p'){
      var m=t.p2p_mode||'waiting',mp={waiting:'等待候选',punching:'打洞中',direct:'已直连',relay:'服务器中转'}[m]||m;
      var cls=m==='direct'?'ok':(m==='relay'?'warn':'gray');
      st='<span class="badge '+cls+'">'+mp+'</span>';
    }else{
      st=t.enabled===false?'<span class="badge gray">已停用</span>':'<span class="badge '+(t.online?'ok':'gray')+'">'+(t.online?'在线':'离线')+'</span>';
    }
    var pub=addr.indexOf('未设置')<0&&addr?'<button class="btn sm" onclick="copyText(\''+esc(addr)+'\',\'地址已复制\')">复制</button>':'';
    if(t.proto==='http'&&addr.indexOf('未设置')<0)pub+=' <a class="btn sm" href="http://'+esc(addr)+'" target="_blank">打开</a>';
    h+='<tr id="tu-'+esc(t.id)+'">'
      +'<td><label class="switch" onclick="event.stopPropagation()"><input type="checkbox" '+(t.enabled!==false?'checked':'')+' onchange="toggleTunnel(\''+esc(t.id)+'\',this.checked)"><span class="sl"></span></label></td>'
      +'<td><span class="link-name" title="点击编辑该隧道" onclick="openTunnelEdit(\''+esc(t.id)+'\')"><b>'+esc(t.name)+'</b></span>'+(t.note?'<div class="sub">'+esc(t.note)+'</div>':'')+'</td>'
      +'<td><span class="badge '+esc(t.proto)+'">'+protoLabel(t.proto)+'</span></td>'
      +'<td><span class="pulse'+(t.online?' on':'')+'"></span> <span class="link-name" title="查看该客户端" onclick="go(\'clients\');flash(\'cl-'+esc(t.client_id)+'\')">'+esc(t.client_name||t.client_id)+'</span></td>'
      +'<td class="mono">'+esc(addr)+' '+pub+'</td>'
      +'<td class="mono">'+esc(t.local_host||'127.0.0.1')+':'+esc(t.local_port)+'</td>'
      +'<td class="mono">↑'+fmtBytes(t.up)+' ↓'+fmtBytes(t.down)+'</td>'
      +'<td>'+st+'</td>'
      +'<td><div class="row-acts">'
      +'<button class="btn sm" onclick="openTunnelEdit(\''+esc(t.id)+'\')">编辑</button>'
      +'<button class="btn sm" title="重建监听 / 重新打洞" onclick="reloadTunnel(\''+esc(t.id)+'\')">⟳</button>'
      +'<button class="btn sm danger" onclick="delTunnel(\''+esc(t.id)+'\')">删除</button>'
      +'</div></td></tr>';
  });
  tb.innerHTML=h;
}
function toggleTunnel(id,on){
  apiJSON('/api/tunnels/'+id,{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({enabled:on})})
  .then(function(){toast(on?'隧道已启用':'隧道已停用');loadAll()})
  .catch(function(){toast('操作失败','err');loadAll()});
}
function delTunnel(id){
  var t=null;for(var i=0;i<state.tunnels.length;i++)if(state.tunnels[i].id===id)t=state.tunnels[i];
  if(!t)return;
  openModal('删除隧道','<p>确定删除隧道 <b>'+esc(t.name)+'</b> 吗？</p>',
    '<button class="btn" onclick="closeModal()">取消</button><button class="btn danger" onclick="closeModal();api(\'/api/tunnels/'+esc(id)+'\',{method:\'DELETE\'}).then(function(){toast(\'已删除\');loadAll()})">确认删除</button>');
}
/* ---- 隧道编辑 / 重载 ---- */
function openTunnelEdit(id){
  var t=null;for(var i=0;i<state.tunnels.length;i++)if(state.tunnels[i].id===id)t=state.tunnels[i];
  if(!t)return;
  var cname=t.client_name||t.client_id;
  var f='<div class="form-row"><label>名称</label><input id="te-name" value="'+esc(t.name)+'"></div>'
    +'<div class="form-row"><label>备注</label><input id="te-note" value="'+esc(t.note||'')+'"></div>'
    +'<div class="form-2col"><div class="form-row"><label>归属客户端（只读，如需迁移请删除重建）</label><input value="'+esc(cname)+'" disabled></div>'
    +'<div class="form-row"><label>内网主机</label><input id="te-lh" value="'+esc(t.local_host||'127.0.0.1')+'"></div></div>'
    +'<div class="form-2col"><div class="form-row"><label>内网端口</label><input id="te-lp" type="number" value="'+(t.local_port||0)+'"></div>';
  if(t.proto==='tcp'||t.proto==='udp'||t.proto==='stcp'){
    f+='<div class="form-row"><label>公网端口（0 = 自动分配）</label><input id="te-pp" type="number" value="'+(t.public_port||0)+'"></div></div>';
  }else if(t.proto==='http'||t.proto==='https'){
    f+='<div class="form-row"><label>完整域名（优先）</label><input id="te-domain" value="'+esc(t.domain||'')+'" placeholder="example.com"></div></div>'
      +'<div class="form-row"><label>或子域名（自动拼接基础域名）</label><input id="te-sub" value="'+esc(t.subdomain||'')+'" placeholder="nas"></div>';
  }else if(t.proto==='p2p'){
    var vocps='';state.clients.forEach(function(c){vocps+='<option value="'+esc(c.id)+'"'+(c.id===t.visitor_client_id?' selected':'')+'>'+esc(c.name)+'</option>'});
    f+='<div class="form-row"><label>访问端本地端口</label><input id="te-vp" type="number" value="'+(t.visitor_port||0)+'"></div></div>'
      +'<div class="form-row"><label>访问端客户端</label><select id="te-vc">'+vocps+'</select></div>';
  }else{f+='</div>'}
  if(t.proto==='stcp'){
    f+='<div class="form-row"><label>访问密钥</label><input id="te-secret" value="'+esc(t.secret||'')+'"></div>';
  }
  f+='<div class="form-row"><label style="display:flex;align-items:center;gap:8px;cursor:pointer"><input type="checkbox" id="te-enabled" style="width:auto" '+(t.enabled!==false?'checked':'')+'> 启用该隧道</label></div>'
    +'<div class="hint">保存后配置热生效：TCP/UDP/STCP 会自动重建监听，P2P 重新发起打洞，客户端无需重启。</div>';
  openModal('编辑隧道 · '+protoLabel(t.proto),f,
    '<button class="btn" onclick="closeModal()">取消</button><button class="btn primary" onclick="saveTunnelEdit(\''+esc(id)+'\')">保存并生效</button>');
}
function saveTunnelEdit(id){
  var body={name:$('te-name').value.trim(),note:$('te-note').value.trim(),enabled:$('te-enabled').checked,
    local_host:$('te-lh').value.trim()||'127.0.0.1',local_port:parseInt($('te-lp').value||'0',10)||0};
  if($('te-pp'))body.public_port=parseInt($('te-pp').value||'0',10)||0;
  if($('te-domain'))body.domain=$('te-domain').value.trim();
  if($('te-sub'))body.subdomain=$('te-sub').value.trim();
  if($('te-secret'))body.secret=$('te-secret').value.trim();
  if($('te-vp'))body.visitor_port=parseInt($('te-vp').value||'0',10)||0;
  if($('te-vc'))body.visitor_client_id=$('te-vc').value;
  apiJSON('/api/tunnels/'+id,{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)})
  .then(function(){toast('隧道已保存并热生效');closeModal();loadAll()})
  .catch(function(e){toast(e.message||'保存失败','err')});
}
function reloadTunnel(id){
  apiJSON('/api/tunnels/'+id,{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({reload:true})})
  .then(function(){toast('隧道已重载（监听重建 / 重新打洞）');loadAll()})
  .catch(function(e){toast(e.message||'重载失败','err')});
}
/* ---- 隧道向导 ---- */
var PRESETS=[
  ['rdp','🖥','远程桌面','RDP 3389'],
  ['ssh','🔐','SSH','22 端口'],
  ['web','🌐','Web 网站','HTTP 80'],
  ['https','🔒','HTTPS','TLS 443'],
  ['game-udp','🎮','游戏 UDP','低延迟'],
  ['game-tcp','🕹','游戏 TCP','联机'],
  ['p2p','⚡','P2P 直连','不经服务器'],
  ['custom','✏️','自定义','任意协议']
];
function openWizard(){
  state.wiz={preset:''};
  var g='<div class="preset-grid">';PRESETS.forEach(function(p){g+='<div class="preset" onclick="wizPick(\''+p[0]+'\')"><div class="pi">'+p[1]+'</div><div class="pt">'+p[2]+'</div><div class="pd">'+p[3]+'</div></div>'});g+='</div>';
  openModal('隧道向导 · 选择场景',g,'<button class="btn" onclick="closeModal()">取消</button>');
}
function wizPick(p){state.wiz={preset:p};wizForm()}
function wizForm(){
  var p=state.wiz.preset;
  var names={rdp:'远程桌面(RDP)',ssh:'SSH',web:'Web 网站',https:'HTTPS 网站','game-udp':'游戏(UDP)','game-tcp':'游戏(TCP)',p2p:'P2P 直连',custom:''};
  var ports={rdp:3389,ssh:22,web:80,https:443,'game-udp':27015,'game-tcp':25565,p2p:3389,custom:0};
  var pubs={rdp:33890,ssh:22022,'game-udp':27015,'game-tcp':25565};
  var ttl={rdp:'远程桌面',ssh:'SSH',web:'Web 网站',https:'HTTPS 网站','game-udp':'游戏 UDP','game-tcp':'游戏 TCP',p2p:'P2P 直连',custom:'自定义'};
  var cops='';state.clients.forEach(function(c){cops+='<option value="'+esc(c.id)+'">'+esc(c.name)+(c.online?'（在线）':'')+'</option>'});
  var vocps='';state.clients.forEach(function(c){vocps+='<option value="'+esc(c.id)+'">'+esc(c.name)+'</option>'});
  var customSel='';
  if(p==='custom'){
    customSel='<div class="form-row"><label>协议类型</label><select id="wz-proto">'
      +'<option value="tcp">TCP — 任意 TCP 服务</option><option value="udp">UDP — 游戏 / 语音</option>'
      +'<option value="http">HTTP — 域名虚拟主机</option><option value="https">HTTPS — SNI 透传</option>'
      +'<option value="stcp">STCP — 密钥握手</option></select></div>';
  }
  var f='<div class="wiz-step">步骤 2 / 2 · 场景：<b>'+esc(ttl[p]||'')+'</b></div>'
    +'<div class="wiz-sec"><span class="wiz-sec-title">基本信息</span>'
    +'<div class="form-2col"><div class="form-row"><label>隧道名称</label><input id="wz-name" value="'+esc(names[p]||'')+'" placeholder="给隧道起个名字"></div>'
    +'<div class="form-row"><label>归属客户端（提供服务）</label><select id="wz-client">'+cops+'</select></div></div>'
    +customSel
    +'</div>'
    +'<div class="wiz-sec"><span class="wiz-sec-title">内网目标</span>'
    +'<div class="form-2col"><div class="form-row"><label>内网主机</label><input id="wz-lh" value="127.0.0.1"></div>'
    +'<div class="form-row"><label>内网端口</label><input id="wz-lp" type="number" value="'+(ports[p]||0)+'"></div></div>'
    +'</div>';
  if(p==='tcp'||p==='udp'||p==='stcp'||p==='custom'||p==='rdp'||p==='ssh'||p==='game-udp'||p==='game-tcp'){
    var autoHint=(p==='tcp'||p==='custom')?' <span style="opacity:.75">（0 = 自动分配空闲端口）</span>':'';
    var fw=(p==='udp'||p==='game-udp')?'UDP':'TCP';
    f+='<div class="wiz-sec"><span class="wiz-sec-title">公网暴露</span>'
      +'<div class="form-row"><label>公网端口'+autoHint+'</label><input id="wz-pp" type="number" value="'+(pubs[p]||0)+'"></div>'
      +(p==='stcp'?'<div class="form-row"><label>访问密钥（留空自动生成）</label><input id="wz-secret" placeholder="连接前需先发送 secret:密钥"></div>':'')
      +'<div class="hint">创建后需在云防火墙放行该端口（'+fw+'）。</div>'
      +'</div>';
  }else if(p==='web'||p==='https'){
    f+='<div class="wiz-sec"><span class="wiz-sec-title">域名路由</span>'
      +'<div class="form-2col"><div class="form-row"><label>完整域名（优先）</label><input id="wz-domain" placeholder="example.com"></div>'
      +'<div class="form-row"><label>或子域名（拼接基础域名）</label><input id="wz-sub" placeholder="nas"></div></div>'
      +'<div class="hint">需先在「设置」配置基础域名并启用 HTTP'+' 端口，DNS 解析到本服务器。</div></div>';
  }else if(p==='p2p'){
    f+='<div class="wiz-sec"><span class="wiz-sec-title">P2P 访问端</span>'
      +'<div class="form-2col"><div class="form-row"><label>访问端客户端</label><select id="wz-vc">'+vocps+'</select></div>'
      +'<div class="form-row"><label>访问端本地端口</label><input id="wz-vp" type="number" value="23389"></div></div>'
      +'<div class="hint">在访问端机器上连 127.0.0.1:本地端口 即直达服务方内网服务；需放行服务端 UDP '+_p[5]+'。</div></div>';
  }
  var foot='<button class="btn" onclick="openWizard()">← 上一步</button><button class="btn primary" onclick="wizSubmit()">创建隧道</button>';
  openModal('隧道向导 · '+(ttl[p]||''),f,foot);
}
function wizSubmit(){
  var p=state.wiz.preset;
  var body={preset:p,name:$('wz-name').value.trim(),client_id:$('wz-client').value,local_host:$('wz-lh').value.trim()||'127.0.0.1',local_port:parseInt($('wz-lp').value||'0',10)||0};
  if($('wz-pp'))body.public_port=parseInt($('wz-pp').value||'0',10)||0;
  if($('wz-domain'))body.domain=$('wz-domain').value.trim();
  if($('wz-sub'))body.subdomain=$('wz-sub').value.trim();
  if($('wz-vp'))body.visitor_port=parseInt($('wz-vp').value||'0',10)||0;
  if($('wz-vc'))body.visitor_client_id=$('wz-vc').value;
  if(p==='custom'){
    var pt=$('wz-proto').value;
    var tc={name:body.name,proto:pt,client_id:body.client_id,local_host:body.local_host,local_port:body.local_port,public_port:body.public_port||0,secret:$('wz-secret')?$('wz-secret').value.trim():'',note:''};
    if(!tc.name){toast('请填写隧道名称','warn');return}
    if(!tc.local_port){toast('请填写内网端口','warn');return}
    apiJSON('/api/tunnels',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(tc)})
    .then(function(t){toast('隧道已创建：'+t.name);closeModal();loadAll()})
    .catch(function(e){toast(e.message||'创建失败，请检查参数','err')});
    return;
  }
  if(!body.name){toast('请填写隧道名称','warn');return}
  if(!body.local_port){toast('请填写内网端口','warn');return}
  apiJSON('/api/wizard',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)})
  .then(function(t){toast('隧道已创建：'+t.name);closeModal();loadAll()})
  .catch(function(e){toast(e.message||'创建失败，请检查参数','err')});
}

/* ---------- 视图：工具 ---------- */
function viewTools(){
  var cops='<option value="">选择客户端…</option>';
  state.clients.forEach(function(c){cops+='<option value="'+esc(c.id)+'">'+esc(c.name)+'</option>'});
  return '<div class="page-head"><div><h1>工具</h1><div class="page-sub">连通性诊断、配置备份恢复、部署命令生成</div></div></div>'
    +'<div class="grid-2">'
    +'<div class="card glass"><div class="card-head">📡 连通性测试 <span class="hint">从服务端发起 TCP 探测</span></div>'
    +'<div class="form-2col"><div class="form-row"><label>主机</label><input id="pb-host" value="127.0.0.1" placeholder="IP 或域名"></div>'
    +'<div class="form-row"><label>端口</label><input id="pb-port" type="number" value="7000"></div></div>'
    +'<button class="btn primary" onclick="doProbe()">开始测试</button>'
    +'<div id="pb-result" style="margin-top:12px"></div>'
    +'<div id="pb-hist"></div></div>'
    +'<div class="card glass"><div class="card-head">💾 配置备份 / 恢复</div>'
    +'<p style="font-size:12.5px;color:var(--muted);margin-bottom:12px">备份导出完整配置 JSON（含客户端、隧道、账号）；恢复将覆盖现有规则，端口变更需重启生效。</p>'
    +'<div style="display:flex;gap:8px;margin-bottom:14px"><button class="btn" onclick="backupConfig()">⬇️ 下载备份</button>'
    +'<button class="btn" onclick="$(\'rs-json\').value=\'\';toast(\'已清空\')">清空</button></div>'
    +'<div class="form-row"><label>粘贴备份 JSON 后恢复</label><textarea id="rs-json" rows="6" placeholder=\'{"control_port":7000,...}\' class="mono"></textarea></div>'
    +'<button class="btn danger" onclick="restoreConfig()">⚠️ 恢复配置</button></div>'
    +'</div>'
    +'<div class="card glass"><div class="card-head">🚀 部署命令生成器</div>'
    +'<div class="form-row"><label>选择客户端令牌</label><select id="dp-client" onchange="renderDeploy()">'+cops+'</select></div>'
    +'<div class="tabs" id="dp-tabs">'
    +'<button class="on" onclick="state.deployTab=\'linux\';renderDeployTabs()">Linux</button>'
    +'<button onclick="state.deployTab=\'win\';renderDeployTabs()">Windows</button>'
    +'<button onclick="state.deployTab=\'mac\';renderDeployTabs()">macOS</button>'
    +'<button onclick="state.deployTab=\'systemd\';renderDeployTabs()">systemd 常驻</button>'
    +'</div><div id="dp-out"></div></div>';
}
function renderDeployTabs(){
  var tb=$('dp-tabs');if(!tb)return;
  var bs=tb.querySelectorAll('button');
  var tabs=['linux','win','mac','systemd'];
  for(var i=0;i<bs.length;i++)bs[i].className=(tabs[i]===state.deployTab?'on':'');
  renderDeploy();
}
function deployCmd(){
  var sel=$('dp-client');if(!sel||!sel.value)return null;
  var c=null,i;
  for(i=0;i<state.clients.length;i++)if(state.clients[i].id===sel.value)c=state.clients[i];
  if(!c)return null;
  var host=location.host,dl='http://'+host+'/api/download?name=';
  var tk=lsToken();
  if(tk)dl+='&token='+encodeURIComponent(tk);
  var srv=publicHost()+':'+_p[1];
  var m={
    linux:'# 1) 下载客户端\ncurl -O "'+dl+'rtpc-linux"\nchmod +x rtpc-linux\n\n# 2) 运行（前台，Ctrl+C 退出）\n./rtpc-linux -server '+srv+' -token '+c.token+' -name '+c.name,
    win:'# PowerShell\ncurl.exe -O "'+dl+'rtpc.exe"\n\n.\\rtpc.exe -server '+srv+' -token '+c.token+' -name '+c.name,
    mac:'curl -O "'+dl+'rtpc-darwin"\nchmod +x rtpc-darwin\n\n./rtpc-darwin -server '+srv+' -token '+c.token+' -name '+c.name,
    systemd:'# 下载并安装\ncurl -o /usr/local/bin/rtpc "'+dl+'rtpc-linux"\nchmod +x /usr/local/bin/rtpc\n\n# 写入服务\ncat > /etc/systemd/system/rtpc.service <<EOF\n[Unit]\nDescription=rtp client\nAfter=network.target\n\n[Service]\nExecStart=/usr/local/bin/rtpc -server '+srv+' -token '+c.token+' -name '+c.name+' -config /etc/rtpc.json\nRestart=always\n\n[Install]\nWantedBy=multi-user.target\nEOF\n\nsystemctl enable --now rtpc'
  };
  return m[state.deployTab]||m.linux;
}
function renderDeploy(){
  var out=$('dp-out');if(!out)return;
  var cmd=deployCmd();
  if(!cmd){out.innerHTML='<div class="empty">请先选择客户端</div>';return}
  out.innerHTML='<div class="pre"><button class="btn sm cp" onclick="copyText(deployCmd(),\'命令已复制\')">复制</button>'+esc(cmd)+'</div>';
}
function openProbe(){
  go('tools');
  setTimeout(function(){var h=$('pb-host');if(h)h.focus()},80);
}
function doProbe(){
  var host=$('pb-host').value.trim(),port=parseInt($('pb-port').value||'0',10)||0;
  if(!host||!port){toast('请填写主机和端口','warn');return}
  apiJSON('/api/probe',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({host:host,port:port})})
  .then(function(r){
    var box=$('pb-result');
    if(r.ok){box.innerHTML='<div class="badge ok" style="font-size:13px;padding:6px 14px">✅ 可达 · '+esc(r.addr)+' · '+esc(r.ms)+' ms</div>';toast('连通正常')}
    else{box.innerHTML='<div class="badge err" style="font-size:13px;padding:6px 14px">❌ 不可达 · '+esc(r.addr)+(r.error?' · '+esc(r.error):'')+'</div>';toast('无法连通','err')}
    state.probeHist.unshift({addr:r.addr,ok:r.ok,ms:r.ms});
    if(state.probeHist.length>8)state.probeHist.pop();
    var h='';state.probeHist.forEach(function(p){
      h+='<div class="ev"><span class="dot" style="background:'+(p.ok?'var(--ok)':'var(--err)')+'"></span><span class="msg mono">'+esc(p.addr)+'</span><span class="badge '+(p.ok?'ok':'err')+'">'+(p.ok?p.ms+' ms':'失败')+'</span></div>';
    });
    $('pb-hist').innerHTML=h;
  }).catch(function(){toast('测试请求失败','err')});
}
function backupConfig(){
  api('/api/backup').then(function(r){return r.blob()}).then(function(b){
    var a=document.createElement('a');a.href=URL.createObjectURL(b);
    a.download='rtp-config.json';document.body.appendChild(a);a.click();a.remove();
    toast('配置备份已下载');
  }).catch(function(){toast('备份失败','err')});
}
function restoreConfig(){
  var txt=$('rs-json').value.trim();
  if(!txt){toast('请先粘贴备份 JSON','warn');return}
  var cfg;try{cfg=JSON.parse(txt)}catch(e){toast('JSON 解析失败','err');return}
  openModal('恢复配置','<p>将用备份覆盖当前 <b>客户端 / 隧道 / 公告 / 域名 / 账号</b>设置，现有规则会被停用重建。确定继续吗？</p>',
    '<button class="btn" onclick="closeModal()">取消</button><button class="btn danger" onclick="closeModal();apiJSON(\'/api/restore\',{method:\'POST\',headers:{\'Content-Type\':\'application/json\'},body:JSON.stringify(cfg)}).then(function(r){toast(r.restart_required?\'已恢复（端口变更需重启生效）\':\'配置已恢复\');loadAll()}).catch(function(){toast(\'恢复失败\',\'err\')})">确认恢复</button>');
}

/* ---------- 视图：事件 ---------- */
function viewEvents(){
  var chips=[['all','全部'],['ok','成功'],['warn','警告'],['err','错误']];
  var h='<div class="page-head"><div><h1>事件</h1><div class="page-sub">服务端最近事件，实时推送</div></div></div><div class="toolbar">';
  chips.forEach(function(c){h+='<button class="chip'+(state.ef===c[0]?' on':'')+'" onclick="state.ef=\''+c[0]+'\';render()">'+c[1]+'</button>'});
  h+='<input placeholder="🔍 搜索事件" value="'+esc(state.search.events)+'" oninput="state.search.events=this.value;renderEventsBody()" style="margin-left:auto"><span class="count" id="ev-count"></span></div>'
    +'<div class="card glass" style="padding:6px 16px" id="events-list"></div>';
  return h;
}
function renderEventsBody(){
  var box=$('events-list');if(!box)return;
  var q=state.search.events.toLowerCase(),rows=[],i;
  for(i=0;i<(state.events||[]).length;i++){
    var e=state.events[i];
    if(state.ef!=='all'&&e.level!==state.ef)continue;
    if(q&&e.msg.toLowerCase().indexOf(q)<0)continue;
    rows.push(e);
  }
  setText('ev-count','共 '+rows.length+' 条');
  if(!rows.length){box.innerHTML='<div class="empty"><div class="big">📜</div>暂无事件</div>';return}
  var h='';
  rows.forEach(function(e){
    var c=e.level==='ok'?'var(--ok)':(e.level==='warn'?'var(--warn)':'var(--err)');
    var t=new Date((e.ts||0)*1000);
    var hm=('0'+t.getHours()).slice(-2)+':'+('0'+t.getMinutes()).slice(-2)+':'+('0'+t.getSeconds()).slice(-2);
    h+='<div class="ev"><span class="dot" style="background:'+c+'"></span><time>'+hm+'</time><span class="msg">'+esc(e.msg)+'</span></div>';
  });
  box.innerHTML=h;
}

/* ---------- 视图：设置 ---------- */
function viewSettings(){
  var s=state.settings||{},st=state.stats||{};
  var host=location.host;
  return '<div class="page-head"><div><h1>设置</h1><div class="page-sub">服务配置、账号安全与客户端下载</div></div></div>'
    +'<div class="grid-2">'
    +'<div class="card glass"><div class="card-head">🌐 基础域名</div>'
    +'<div class="form-row"><label>HTTP/HTTPS 隧道的默认域名（子域名拼接为其下级）</label><input id="st-domain" value="'+esc(s.base_domain||'')+'" placeholder="example.com"></div>'
    +'<button class="btn primary" onclick="saveBaseDomain()">保存</button>'
    +'<div class="hint" style="margin-top:8px">需先将域名 A 记录指向 '+esc(publicHost())+'，并在服务端配置启用 http_port 后重启。</div></div>'
    +'<div class="card glass"><div class="card-head">📢 公告栏</div>'
    +'<div class="form-row"><label>展示在登录页与控制台顶部（留空关闭）</label><textarea id="st-ann" rows="3" maxlength="200" placeholder="例如：周末 22:00-24:00 停机维护">'+esc(s.announcement||'')+'</textarea></div>'
    +'<button class="btn primary" onclick="saveAnnouncement()">保存公告</button></div>'
    +'</div>'
    +'<div class="card glass"><div class="card-head">🔔 通知机器人 <button class="btn sm primary" style="float:right" onclick="openWebhookModal()">＋ 新建通知</button></div>'
    +'<div class="hint" style="margin-bottom:10px">支持飞书 / 企业微信 / 钉钉群机器人 Webhook，按事件类型订阅推送（客户端上下线、隧道变更等）。</div>'
    +'<div id="wh-list"></div></div>'
    +'<div class="card glass"><div class="card-head">🔐 账号安全</div>'
    +(s.default_pass?'<div class="ann-banner" style="margin-bottom:12px">⚠️ 仍在使用默认密码 admin / admin123，请立即修改</div>':'')
    +'<div class="kv"><span class="k">当前用户名</span><span class="v">'+esc(s.admin_user||'admin')+'</span></div>'
    +'<div class="form-2col" style="margin-top:12px"><div class="form-row"><label>当前密码</label><input id="ac-cur" type="password" placeholder="验证身份"></div>'
    +'<div class="form-row"><label>新用户名（留空不变）</label><input id="ac-user" placeholder="'+esc(s.admin_user||'admin')+'"></div></div>'
    +'<div class="form-row"><label>新密码（至少 6 位，留空不变）</label><input id="ac-pass" type="password" placeholder="••••••"></div>'
    +'<button class="btn primary" onclick="saveAccount()">保存修改</button></div>'
    +'<div class="card glass"><div class="card-head">🎫 管理令牌（Token 登录）</div>'
    +'<div class="form-row"><label>当前管理员令牌（点击复制）</label>'
    +'<div class="tk-show"><span class="mono" id="tk-cur" style="cursor:pointer" title="点击复制" onclick="copyText((state.settings||{}).admin_token||\'\',\'令牌已复制\')">'+esc((s.admin_token||'').slice(0,6)+'••••••'+(s.admin_token||'').slice(-4))+'</span>'
    +'<button class="btn sm" onclick="$(\'tk-cur\').textContent=(state.settings||{}).admin_token||\'\'">显示</button></div></div>'
    +'<div class="form-row"><label>新令牌（8-64 位字母数字- _，需配合当前密码验证）</label>'+tokenField('tk-new','')+'</div>'
    +'<div class="form-row"><label>当前密码（验证身份）</label><input id="tk-curpass" type="password" placeholder="••••••"></div>'
    +'<button class="btn primary" onclick="saveAdminToken()">更换令牌</button>'
    +'<div class="hint" style="margin-top:8px">令牌用于 ?token= 直达链接与 default 客户端连接；更换后旧令牌链接立即失效，账号密码登录的会话不受影响。</div></div>'
    +'<div class="grid-2">'
    +'<div class="card glass"><div class="card-head">⬇️ 客户端下载</div>'
    +'<div style="display:flex;gap:8px;flex-wrap:wrap">'
    +'<a class="btn" href="/api/download?name=rtpc.exe'+(lsToken()?'&token='+encodeURIComponent(lsToken()):'')+'">🪟 Windows</a>'
    +'<a class="btn" href="/api/download?name=rtpc-linux'+(lsToken()?'&token='+encodeURIComponent(lsToken()):'')+'">🐧 Linux</a>'
    +'<a class="btn" href="/api/download?name=rtpc-darwin'+(lsToken()?'&token='+encodeURIComponent(lsToken()):'')+'">🍎 macOS</a>'
    +'<a class="btn" href="/api/download?name=rtpc-android-arm64'+(lsToken()?'&token='+encodeURIComponent(lsToken()):'')+'">📱 Android</a>'
    +'</div><div class="hint" style="margin-top:10px">单文件、零依赖；Android 版运行于 Termux（见使用文档 4.7 节）；也可用「工具 → 部署命令生成器」一键部署。</div></div>'
    +'<div class="card glass"><div class="card-head">ℹ️ 关于</div>'
    +'<div class="kv"><span class="k">版本</span><span class="v">rtp v'+(VERSION||'')+'</span></div>'
    +'<div class="kv"><span class="k">Go</span><span class="v">'+esc(st.go_version||'-')+'</span></div>'
    +'<div class="kv"><span class="k">运行时长</span><span class="v">'+esc(st.uptime_human||'-')+'</span></div>'
    +'<div class="kv"><span class="k">控制台地址</span><span class="v mono">'+esc(host)+'</span></div>'
    +'</div></div>';
}
function saveBaseDomain(){
  var v=$('st-domain').value.trim();
  apiJSON('/api/settings',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({base_domain:v})})
  .then(function(){toast('基础域名已保存');loadSettings()}).catch(function(){toast('保存失败','err')});
}
function saveAnnouncement(){
  var v=$('st-ann').value.trim();
  apiJSON('/api/settings',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({announcement:v})})
  .then(function(){toast('公告已保存');state.me.announcement=v;$('top-ann').textContent=v;loadSettings()}).catch(function(){toast('保存失败','err')});
}
function saveAccount(){
  var cur=$('ac-cur').value,u=$('ac-user').value.trim(),p=$('ac-pass').value;
  if(!cur){toast('请输入当前密码','warn');return}
  if(p&&p.length<6){toast('新密码至少 6 位','warn');return}
  apiJSON('/api/settings',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({cur_pass:cur,new_user:u,new_pass:p})})
  .then(function(){toast('账号信息已更新');$('ac-cur').value='';$('ac-pass').value='';loadSettings()})
  .catch(function(e){toast(e.message||'修改失败','err')});
}

function saveAdminToken(){
  var tk=$('tk-new').value.trim(),cur=$('tk-curpass').value;
  if(!tk){toast('请填写新令牌','warn');return}
  if(!cur){toast('请输入当前密码验证身份','warn');return}
  apiJSON('/api/settings',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({new_token:tk,cur_pass:cur})})
  .then(function(){
    toast('管理令牌已更换，旧令牌链接已失效');
    $('tk-new').value='';$('tk-curpass').value='';
    try{localStorage.setItem('rtp_tk',tk)}catch(e){}
    loadSettings();
  }).catch(function(e){toast(e.message||'更换失败','err')});
}

/* ---------- 通知机器人 ---------- */
var WH_TYPES=[['client_online','客户端上线'],['client_offline','客户端下线'],['client_create','客户端创建'],['client_update','客户端修改'],['tunnel_create','隧道创建'],['tunnel_update','隧道修改'],['tunnel_delete','隧道删除'],['p2p','P2P 直连'],['admin','管理操作']];
var WH_KINDS=[['feishu','飞书'],['wecom','企业微信'],['dingtalk','钉钉'],['generic','自定义 JSON']];
function whTypeName(k){var a=state.eventTypes&&state.eventTypes[k];if(a)return a;for(var i=0;i<WH_TYPES.length;i++)if(WH_TYPES[i][0]===k)return WH_TYPES[i][1];return k}
function whKindName(k){for(var i=0;i<WH_KINDS.length;i++)if(WH_KINDS[i][0]===k)return WH_KINDS[i][1];return k}
function renderWebhooks(){
  var el=$('wh-list');if(!el)return;
  var ws=state.webhooks||[];
  if(!ws.length){el.innerHTML='<div class="empty">尚未配置通知机器人，点击右上角「＋ 新建通知」添加</div>';return}
  var h='';
  ws.forEach(function(x){
    var ev='';(x.events||[]).forEach(function(e){ev+='<span class="chip">'+esc(whTypeName(e))+'</span>'});
    var kind='';for(var i=0;i<WH_KINDS.length;i++)if(WH_KINDS[i][0]===x.type)kind=WH_KINDS[i][1];
    h+='<div class="wh-item" style="display:flex;align-items:center;gap:12px;padding:10px 0;border-bottom:1px solid var(--line)">'
      +'<span class="badge">'+esc(kind||x.type)+'</span>'
      +'<div style="flex:1;min-width:0"><div class="mono" style="font-size:12px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">'+esc(x.url)+'</div><div style="margin-top:4px">'+ev+'</div></div>'
      +'<label style="display:flex;align-items:center;gap:4px;cursor:pointer;font-size:12px"><input type="checkbox" '+(x.enabled?'checked':'')+' onchange="toggleWebhook(\''+esc(x.id)+'\',this.checked)">启用</label>'
      +'<button class="btn sm" title="发送测试消息" onclick="testWebhook(\''+esc(x.id)+'\')">🧪 测试</button>'
      +'<button class="btn sm" onclick="openWebhookModal(\''+esc(x.id)+'\')">编辑</button>'
      +'<button class="btn sm danger" onclick="delWebhook(\''+esc(x.id)+'\')">删除</button>'
      +'</div>';
  });
  el.innerHTML=h;
}
function openWebhookModal(id){
  var cur=null;if(id){var ws=state.webhooks||[];for(var i=0;i<ws.length;i++)if(ws[i].id===id)cur=ws[i]}
  var kinds='';WH_KINDS.forEach(function(k){kinds+='<option value="'+k[0]+'"'+(cur&&cur.type===k[0]?' selected':'')+'>'+k[1]+'</option>'});
  var evs='';WH_TYPES.forEach(function(t){
    var on=!cur||(cur.events||[]).indexOf(t[0])>=0;
    evs+='<label style="display:inline-flex;align-items:center;gap:4px;margin:2px 10px 2px 0;font-size:13px;cursor:pointer"><input type="checkbox" class="wh-ev" value="'+t[0]+'"'+(on?' checked':'')+'>'+t[1]+'</label>';
  });
  var body='<div class="form-row"><label>机器人类型</label><select id="wh-type" style="width:100%%;padding:8px;border:1px solid var(--line);border-radius:8px;background:var(--bg2);color:var(--fg)">'+kinds+'</select></div>'
    +'<div class="form-row"><label>Webhook 地址（群机器人 → 添加机器人 → 复制 Webhook 地址）</label><input id="wh-url" value="'+esc(cur?cur.url:'')+'" placeholder="https://open.feishu.cn/open-apis/bot/v2/hit/... 或 https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=..."></div>'
    +'<div class="form-row"><label>订阅事件类型</label><div style="line-height:1.9">'+evs+'</div>'
    +'<div style="margin-top:6px"><button class="btn sm" onclick="whEvAll(true)">全选</button> <button class="btn sm" onclick="whEvAll(false)">全不选</button></div></div>'
    +'<div class="form-row"><label style="display:flex;align-items:center;gap:6px;cursor:pointer"><input type="checkbox" id="wh-enabled"'+(!cur||cur.enabled?' checked':'')+'>启用该通知</label></div>';
  var foot='<button class="btn" onclick="closeModal()">取消</button><button class="btn primary" onclick="saveWebhook('+(id?'\''+esc(id)+'\'':'null')+')">保存</button>';
  openModal(cur?'编辑通知机器人':'新建通知机器人',body,foot);
}
function whEvAll(on){var l=document.querySelectorAll('.wh-ev');for(var i=0;i<l.length;i++)l[i].checked=on}
function saveWebhook(id){
  var type=$('wh-type').value,url=$('wh-url').value.trim();
  var evs=[],l=document.querySelectorAll('.wh-ev');
  for(var i=0;i<l.length;i++)if(l[i].checked)evs.push(l[i].value);
  if(!url){toast('请填写 Webhook 地址','warn');return}
  if(!evs.length){toast('请至少订阅一种事件类型','warn');return}
  var body=JSON.stringify({type:type,url:url,events:evs,enabled:$('wh-enabled').checked});
  apiJSON(id?'/api/webhooks/'+id:'/api/webhooks',{method:id?'PUT':'POST',headers:{'Content-Type':'application/json'},body:body})
  .then(function(){toast('通知机器人已保存');closeModal();loadWebhooks()})
  .catch(function(e){toast(e.message||'保存失败','err')});
}
function delWebhook(id){
  if(!confirm('确定删除该通知机器人？'))return;
  apiJSON('/api/webhooks/'+id,{method:'DELETE'}).then(function(){toast('已删除');loadWebhooks()}).catch(function(e){toast(e.message||'删除失败','err')});
}
function testWebhook(id){
  toast('测试消息发送中…');
  apiJSON('/api/webhooks/'+id+'/test',{method:'POST',headers:{'Content-Type':'application/json'},body:'{}'})
  .then(function(){toast('测试消息已发送，请到群里查看是否收到')})
  .catch(function(e){toast(e.message||'发送失败，请检查 Webhook 地址','err')});
}
function toggleWebhook(id,on){
  var ws=state.webhooks||[],cur=null;
  for(var i=0;i<ws.length;i++)if(ws[i].id===id)cur=ws[i];
  if(!cur)return;
  apiJSON('/api/webhooks/'+id,{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({type:cur.type,url:cur.url,events:cur.events,enabled:on})})
  .then(function(){toast(on?'已启用':'已停用');loadWebhooks()})
  .catch(function(e){toast(e.message||'操作失败','err');loadWebhooks()});
}

/* ---------- 弹窗 ---------- */
function openModal(title,body,foot){
  var r=$('modal-root');
  r.innerHTML='<div class="mask" onclick="if(event.target===this)closeModal()"></div>'
    +'<div class="modal"><div class="modal-head"><span>'+title+'</span><button class="icon-btn" onclick="closeModal()">✕</button></div>'
    +'<div class="modal-body">'+body+'</div>'+(foot?'<div class="modal-foot">'+foot+'</div>':'')+'</div>';
  r.style.display='block';
}
function closeModal(){$('modal-root').style.display='none';$('modal-root').innerHTML=''}

/* ---------- 命令面板 ---------- */
function palItems(){
  var it=[],i;
  NAV.forEach(function(n){it.push({t:'前往：'+n[2],i:n[1],run:function(){go(n[0])}})});
  it.push({t:'新建客户端',i:'🖥',run:openClientModal});
  it.push({t:'隧道向导',i:'🧙',run:openWizard});
  it.push({t:'连通性测试',i:'📡',run:openProbe});
  it.push({t:'备份配置',i:'💾',run:backupConfig});
  it.push({t:'恢复配置',i:'♻️',run:function(){go('tools');setTimeout(function(){var t=$('rs-json');if(t)t.focus()},100)}});
  it.push({t:'下载客户端',i:'⬇️',run:function(){go('settings')}});
  it.push({t:'切换亮/暗主题',i:'🌗',run:toggleTheme});
  it.push({t:'更换主题配色',i:'🎨',run:function(){var a=['indigo','emerald','rose','amber','sky'];var cur=document.documentElement.dataset.accent||'indigo';setAccent(a[(a.indexOf(cur)+1)%%a.length])}});
  for(i=0;i<state.clients.length;i++){var c=state.clients[i];it.push({t:'客户端：'+c.name,s:'点击编辑',i:'🖥',run:function(cc){return function(){go('clients');openClientEdit(cc.id)}}(c)})}
  for(i=0;i<state.tunnels.length;i++){var t=state.tunnels[i];it.push({t:'隧道：'+t.name,s:protoLabel(t.proto)+' · 编辑',i:'🔀',run:function(tt){return function(){go('tunnels');openTunnelEdit(tt.id)}}(t)})}
  return it;
}
function flash(id){var e=$(id);if(e){e.classList.remove('flash');void e.offsetWidth;e.classList.add('flash');e.scrollIntoView({block:'nearest',behavior:'smooth'})}}
function openPalette(){
  state.palItems=palItems();state.palIdx=0;state.palShown=state.palItems;
  var r=$('palette-root');
  r.innerHTML='<div class="mask" onclick="closePalette()"></div><div class="palette">'
    +'<input id="pal-q" placeholder="搜索命令、客户端、隧道…" oninput="palFilter(this.value)">'
    +'<div class="pal-list" id="pal-list"></div>'
    +'<div class="pal-foot"><span><span class="kbd">↑↓</span> 选择</span><span><span class="kbd">Enter</span> 执行</span><span><span class="kbd">Esc</span> 关闭</span></div></div>';
  r.style.display='block';
  setTimeout(function(){var q=$('pal-q');if(q)q.focus()},50);
  palFilter('');
}
function closePalette(){$('palette-root').style.display='none';$('palette-root').innerHTML=''}
function palFilter(q){
  q=(q||'').toLowerCase();
  var items=state.palItems.filter(function(x){return !q||x.t.toLowerCase().indexOf(q)>=0||(x.s||'').toLowerCase().indexOf(q)>=0});
  state.palShown=items;state.palIdx=0;
  var h='';
  items.forEach(function(x,i){h+='<div class="pal-item'+(i===0?' on':'')+'" data-i="'+i+'" onmouseenter="state.palIdx='+i+';palHl()" onclick="palGo('+i+')"><span class="pi">'+x.i+'</span>'+esc(x.t)+(x.s?'<span class="sub mono">'+esc(x.s)+'</span>':'')+'</div>'});
  var l=$('pal-list');if(l)l.innerHTML=h||'<div class="empty">没有匹配项</div>';
}
function palHl(){
  var items=$('pal-list').querySelectorAll('.pal-item');
  for(var i=0;i<items.length;i++)items[i].classList.toggle('on',i===state.palIdx);
}
function palGo(i){
  var x=state.palShown[i];if(!x)return;
  closePalette();x.run();
}
document.addEventListener('keydown',function(e){
  if((e.ctrlKey||e.metaKey)&&e.key.toLowerCase()==='k'){e.preventDefault();if($('palette-root').style.display==='block')closePalette();else openPalette();return}
  if(e.key==='Escape'){closeModal();closePalette();return}
  if($('palette-root').style.display==='block'){
    if(e.key==='ArrowDown'){e.preventDefault();state.palIdx=Math.min(state.palIdx+1,(state.palShown||[]).length-1);palHl()}
    else if(e.key==='ArrowUp'){e.preventDefault();state.palIdx=Math.max(state.palIdx-1,0);palHl()}
    else if(e.key==='Enter'){e.preventDefault();palGo(state.palIdx)}
    return;
  }
  var tag=(e.target.tagName||'').toLowerCase();
  if(tag==='input'||tag==='textarea'||tag==='select')return;
  if(e.key>='1'&&e.key<='6'){go(['dashboard','clients','tunnels','tools','events','settings'][e.key-'1'])}
  else if(e.key==='/'){e.preventDefault();var s=document.querySelector('.toolbar input');if(s)s.focus()}
});

boot();
</script>
</body>
</html>`
