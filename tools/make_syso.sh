#!/bin/bash
# 生成 Windows 图标资源 .syso（编译进 exe，资源管理器/任务栏显示图标）
# 需在装有 mingw-w64 的环境执行：x86_64-w64-mingw32-windres
set -e
HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
ICONS=$ROOT/assets/icons
WINDRES=${WINDRES:-x86_64-w64-mingw32-windres}

if ! command -v $WINDRES > /dev/null 2>&1; then
  echo "找不到 $WINDRES，请先安装 mingw-w64（apt install mingw-w64 / brew install mingw-w64）"
  exit 1
fi

# 服务端图标 -> cmd/rtps/rtps.syso
cat > /tmp/rtps.rc << EOF
1 ICON "$ICONS/server.ico"
1 VERSIONINFO
FILEVERSION 1,2,0,0
PRODUCTVERSION 1,2,0,0
FILEOS 0x4L
FILETYPE 0x1L
BEGIN
  BLOCK "StringFileInfo"
  BEGIN
    BLOCK "080404B0"
    BEGIN
      VALUE "CompanyName", "rtp"
      VALUE "FileDescription", "rtp 内网穿透服务端"
      VALUE "FileVersion", "1.2.0"
      VALUE "InternalName", "rtps"
      VALUE "OriginalFilename", "rtps.exe"
      VALUE "ProductName", "rtp 内网穿透"
      VALUE "ProductVersion", "1.2.0"
    END
  END
  BLOCK "VarFileInfo"
  BEGIN
    VALUE "Translation", 0x0804, 1200
  END
END
EOF

# 客户端图标 -> cmd/rtpc/rtpc.syso
cat > /tmp/rtpc.rc << EOF
1 ICON "$ICONS/client.ico"
1 VERSIONINFO
FILEVERSION 1,2,0,0
PRODUCTVERSION 1,2,0,0
FILEOS 0x4L
FILETYPE 0x1L
BEGIN
  BLOCK "StringFileInfo"
  BEGIN
    BLOCK "080404B0"
    BEGIN
      VALUE "CompanyName", "rtp"
      VALUE "FileDescription", "rtp 内网穿透客户端"
      VALUE "FileVersion", "1.2.0"
      VALUE "InternalName", "rtpc"
      VALUE "OriginalFilename", "rtpc.exe"
      VALUE "ProductName", "rtp 内网穿透"
      VALUE "ProductVersion", "1.2.0"
    END
  END
  BLOCK "VarFileInfo"
  BEGIN
    VALUE "Translation", 0x0804, 1200
  END
END
EOF

$WINDRES -i /tmp/rtps.rc -O coff -o "$ROOT/cmd/rtps/rtps.syso"
$WINDRES -i /tmp/rtpc.rc -O coff -o "$ROOT/cmd/rtpc/rtpc.syso"
ls -la "$ROOT/cmd/rtps/rtps.syso" "$ROOT/cmd/rtpc/rtpc.syso"
echo "图标资源生成完毕（后续 go build 会自动嵌入）"