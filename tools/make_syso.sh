#!/bin/bash
# 生成 Windows 图标资源 .syso（编译进 exe，资源管理器/任务栏显示图标）
# 需在装有 mingw-w64 的环境执行：x86_64-w64-mingw32-windres
#
# 重要：输出文件名必须带 _amd64 架构后缀（rtps_amd64.syso / rtpc_amd64.syso）。
# go build 只会拾取与目标 GOARCH 匹配的 .syso；COFF 对象里的重定位类型是
# 架构相关的，x86_64 的 .syso 在 GOARCH=arm64 下会报
# "unknown ARM64 relocation type 3" 而编译失败。加上后缀后，
# arm64 构建会自动跳过图标资源，从而保证全平台交叉编译都能通过。
set -e
HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/.." && pwd)
ICONS=$ROOT/assets/icons
WINDRES=${WINDRES:-x86_64-w64-mingw32-windres}

if ! command -v $WINDRES > /dev/null 2>&1; then
  echo "找不到 $WINDRES，请先安装 mingw-w64（apt install mingw-w64 / brew install mingw-w64）"
  exit 1
fi

if [ ! -f "$ICONS/server.ico" ] || [ ! -f "$ICONS/client.ico" ]; then
  echo "缺少 $ICONS/*.ico，请先运行：python3 android/tools/make_icons.py"
  exit 1
fi

VERSION=${VERSION:-1.3.0}

# 服务端图标 + 版本信息 -> cmd/rtps/rtps_amd64.syso
cat > /tmp/rtps.rc << EOF
1 ICON "$ICONS/server.ico"
1 VERSIONINFO
FILEVERSION 1,3,0,0
PRODUCTVERSION 1,3,0,0
FILEOS 0x4L
FILETYPE 0x1L
BEGIN
  BLOCK "StringFileInfo"
  BEGIN
    BLOCK "080404B0"
    BEGIN
      VALUE "CompanyName", "rtp"
      VALUE "FileDescription", "rtp 内网穿透服务端"
      VALUE "FileVersion", "$VERSION"
      VALUE "InternalName", "rtps"
      VALUE "OriginalFilename", "rtps.exe"
      VALUE "ProductName", "rtp 内网穿透"
      VALUE "ProductVersion", "$VERSION"
    END
  END
  BLOCK "VarFileInfo"
  BEGIN
    VALUE "Translation", 0x0804, 1200
  END
END
EOF

# 客户端图标 + 版本信息 -> cmd/rtpc/rtpc_amd64.syso
cat > /tmp/rtpc.rc << EOF
1 ICON "$ICONS/client.ico"
1 VERSIONINFO
FILEVERSION 1,3,0,0
PRODUCTVERSION 1,3,0,0
FILEOS 0x4L
FILETYPE 0x1L
BEGIN
  BLOCK "StringFileInfo"
  BEGIN
    BLOCK "080404B0"
    BEGIN
      VALUE "CompanyName", "rtp"
      VALUE "FileDescription", "rtp 内网穿透客户端"
      VALUE "FileVersion", "$VERSION"
      VALUE "InternalName", "rtpc"
      VALUE "OriginalFilename", "rtpc.exe"
      VALUE "ProductName", "rtp 内网穿透"
      VALUE "ProductVersion", "$VERSION"
    END
  END
  BLOCK "VarFileInfo"
  BEGIN
    VALUE "Translation", 0x0804, 1200
  END
END
EOF

$WINDRES -i /tmp/rtps.rc -O coff -o "$ROOT/cmd/rtps/rtps_amd64.syso"
$WINDRES -i /tmp/rtpc.rc -O coff -o "$ROOT/cmd/rtpc/rtpc_amd64.syso"
ls -la "$ROOT/cmd/rtps/rtps_amd64.syso" "$ROOT/cmd/rtpc/rtpc_amd64.syso"
echo "图标资源生成完毕（后续 GOOS=windows GOARCH=amd64 的 go build 会自动嵌入）"