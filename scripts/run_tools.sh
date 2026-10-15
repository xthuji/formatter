#!/usr/bin/env bash
# =============================================================================
# Formatter - 构建与运行工具 (跨平台: macOS(amd64/arm64 交叉) / Linux / Windows Git Bash)
# =============================================================================
# 命令:
#   1|b,build     构建桌面 App (当前平台; --platform=<os>/<arch> 可指定目标)
#   2|r,run       构建并前台运行 App (Ctrl+C 关闭)
#   3|s,server    构建并前台运行 Web Server (serve 命令)
#   4|d,dev       开发模式: go run 源码直起 Web Server
#   5|i,install-bin下载/校验三方二进制工具到 data/bin/<os>/<arch>
#   6|t,test      运行 Go 单元测试 (覆盖率)
#   7|c,clean     清理构建产物
#   0|q|exit        退出
# =============================================================================
# 薄入口：公共实现全部在 run_tools_master.sh（跨项目逐字节一致，禁止单项目
# 私改），项目差异全部写在本文件的「常量与命令表」与「项目专属实现」内联区；
# 全部构建/打包/测试逻辑集中在这里，CI workflow 只做 runner 引导与产物上传
#（规范 4.2）。
# =============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_DIR"

# ============================================================
# 公共模块：母版实现（与其余桌面应用逐字节一致，请勿在此修改）
# 先于常量区加载：母版只定义函数不执行业务，差异常量声明在下一区
# ============================================================
source "$SCRIPT_DIR/run_tools_master.sh"
# ============================================================
# 项目差异区 · 常量与命令表（脚本唯一需要按项目调整的部分）
# ============================================================

# --- 项目标识 ---
APP_NAME="Formatter"                    # 可执行文件 / Wails outputfilename
BUNDLE_NAME="Formatter"              # .app bundle 名（留空同 APP_NAME）
RELEASE_PREFIX="Formatter"        # release/ 产物命名前缀（留空同 APP_NAME）
APP_DISPLAY_NAME="Formatter"
PROJECT_BUNDLE_ID="com.xthuji.formatter"
export PROJECT_BUNDLE_ID

# --- 目录与产物 ---
BUILD_DIR="$PROJECT_DIR/build"
FRONTEND_DIR="$PROJECT_DIR/src/ui/static"
# 产物目录：默认 <frontend_dir>/dist；Vite outDir 指到别处（如 easy_diff 直写 embed
# 目录 frontend/dist）时必须按项目覆盖，否则母版 ensure_frontend 会误判为未构建
FRONTEND_DIST="$FRONTEND_DIR/dist"
WAILS_CMD=""
VERSION=""

# --- 构建策略 ---
IS_WAILS=0                     # 0=纯 Go 桌面（自行覆盖 build_impl / get_app_bundle）
PLATFORM_SCOPE="all"        # darwin=仅面向 macOS 产物
MACOS_MIN_VERSION="12.0"
MACOS_UNIVERSAL_DEFAULT=0     # 1=恒产 universal；0=本地单架构 + 环境变量开关
UNIVERSAL_ENV_VAR=""     # 例：EASYDIFF_UNIVERSAL
WAILS_BUILD_CLEAN=0
WAILS_PIN_VERSION=""                    # 留空=对齐 go.mod 的 wails 版本（真源）
NEEDS_CGO=0                   # 1=Windows 构建需要 MinGW-w64
ADHOC_SIGN=1                      # 1=macOS 构建后 ad-hoc 签名
GO_BUILD_LDFLAGS="-s -w"
VERSION_LD_VAR="formatter/src/appcommon.Version"        # 版本注入的 Go 变量（ldflags -X）
LDFLAGS_EXTRA=""
GOTOOLCHAIN_VER=""

# --- 资源与配置 ---
CONFIG_SRC=""
CONFIG_DEST_DIR=""
ICON_SRC="$PROJECT_DIR/data/icon/icon.png"

# --- 菜单与分发 ---
DEFAULT_CMD="run"
FALLBACK_CMD=""
SRC_FIND_EXPR=( -name '*.go' -o -path '*/src/*' -o -name 'VERSION' -o -name 'go.mod' -o -name 'config.json' -o -name 'icon.png' -o -name '*.desktop' )             # needs_build 的源文件匹配表达式
TEST_PKGS=( ./tests/... )
CLEAN_EXTRA=( coverage .downloads )
DEV_CMD=( go run -tags desktop,production ./src serve )                    # 留空=母版默认 wails dev

# --- 命令表：序号|别名(逗号分隔)|说明|处理函数（菜单/帮助/分发唯一真源）---
commands=(
  "1|b,build|构建桌面 App (当前平台; --platform=<os>/<arch> 可指定目标)|cmd_build"
  "2|r,run|构建并前台运行 App (Ctrl+C 关闭)|cmd_run"
  "3|s,server|构建并前台运行 Web Server (serve 命令)|cmd_server"
  "4|d,dev|开发模式: go run 源码直起 Web Server|cmd_dev"
  "5|i,install-bin|下载/校验三方二进制工具到 data/bin/<os>/<arch>|cmd_install_bin"
  "6|t,test|运行 Go 单元测试 (覆盖率)|cmd_test"
  "7|c,clean|清理构建产物|cmd_clean"
)

# ============================================================
# 项目专属实现（覆盖母版默认 / 追加项目专属命令）
# 内联区位于常量表之后、main "$@" 之前：顶层可安全展开差异常量
# ============================================================
# Formatter 特性：同一二进制兼作桌面 App / CLI / Web Server，不走 wails build CLI，
# 由 go build -tags desktop,production ./src/ 编译后手工装配各平台产物；
# CI 四个 job 用母版全局选项 --platform=<os>/<arch> 指定目标（darwin 双架构在
# arm64 runner 上交叉编译，自动补 clang -target）。
PROJECT_NAME="formatter"
BUILD_TAGS="desktop,production"
WEBKIT_PC=""                            # Linux 使用的 pkg-config 模块（webkit2gtk-4.0 / 4.1）
WEBKIT_TAG=""                           # 用 4.1 时追加的 build tag（webkit2_41）
WEBKIT_PKG_TO_INSTALL=""
SEVENZIP=""

DATA_DIR="${PROJECT_DIR}/data"
BIN_ROOT="${DATA_DIR}/bin"              # 三方工具仓库目录：data/bin/<os>/<arch>/
ICON_PNG="${ICON_SRC}"                  # 预生成 1024x1024 位图（构建时不再依赖 SVG 光栅化）
ICON_SVG="${DATA_DIR}/icon/icon.svg"
LINUX_DESKTOP="${DATA_DIR}/linux/formatter.desktop"
INSTALL_BIN_SCRIPT="${SCRIPT_DIR}/install-bin.sh"

have() { command -v "$1" >/dev/null 2>&1; }

# 跨架构编译判定直接用母版的 NATIVE_ARCH（--platform 只改写 HOST_*）
is_cross_arch() { [ "$HOST_ARCH" != "$NATIVE_ARCH" ]; }

# 三方工具目录随目标平台变化，必须在函数内取（不能在文件顶层固化）
bin_dir() { echo "${BIN_ROOT}/${HOST_OS}/${HOST_ARCH}"; }

# 三平台产物命名统一: Formatter-<版本>-<os>-<arch>
artifact_base() { echo "${APP_NAME}-$(get_version)-${HOST_OS}-${HOST_ARCH}"; }

get_app_bundle()   { echo "$BUILD_DIR/${APP_NAME}.app"; }
get_build_binary() { echo "$BUILD_DIR/${PROJECT_NAME}_${HOST_OS}_${HOST_ARCH}${EXT}"; }
get_run_binary() {
  if [ "$HOST_OS" = "darwin" ]; then
    echo "$(get_app_bundle)/Contents/MacOS/${APP_NAME}"
  else
    echo "$(get_build_binary)"
  fi
}

# Formatter 不用 wails build，wails.json 仅作项目元数据存档，不做版本回写
sync_wails_version() { :; }

# release/ 只放最终归档；.app 仅留在 build/ 供本地调试
sync_release() { mkdir -p "$PROJECT_DIR/release"; }

# Wails 不用 wails build，Info.plist 由 generate_info_plist 整份生成于打包阶段，
# 母版 fix_app_info（plutil 增量改）在此无用且会误改上一次的陈旧 bundle
fix_app_info() { :; }

# Wails 的 build/appicon.png 流程无关：图标在打包阶段按平台生成 .icns / .ico / png
sync_app_icon() {
  [ -f "$ICON_PNG" ] || log_warn "未找到图标源 ${ICON_PNG}，.icns/.ico 生成将跳过"
}

# 7z 在 p7zip 包中为 7z/7za，在 7zip 包中为 7zz
resolve_sevenzip() {
  [ -n "$SEVENZIP" ] && return 0
  local cand
  for cand in 7z 7zz 7za p7zip; do
    if have "$cand"; then SEVENZIP="$cand"; return 0; fi
  done
  return 1
}

dir_has_files() {
  local dir="$1"
  [ -d "$dir" ] || return 1
  [ -n "$(ls -A "$dir" 2>/dev/null)" ]
}

# archive_zip <目标 zip> <待打包目录>：7z → zip → PowerShell Compress-Archive 逐级回退
archive_zip() {
  local zip_path="$1" src_dir="$2"
  local parent entry
  parent="$(dirname "$src_dir")"
  entry="$(basename "$src_dir")"
  mkdir -p "$parent"
  rm -f "$zip_path"

  if resolve_sevenzip; then
    (cd "$parent" && "$SEVENZIP" a -tzip -mx=9 "$zip_path" "$entry" >/dev/null 2>&1)
    [ -s "$zip_path" ] && return 0
    rm -f "$zip_path"
  fi
  if have zip; then
    (cd "$parent" && zip -qr "$zip_path" "$entry")
    [ -s "$zip_path" ] && return 0
    rm -f "$zip_path"
  fi
  # Git Bash 无 7z/zip 时交给 PowerShell
  local ps="" cand
  for cand in powershell.exe powershell pwsh.exe pwsh; do
    if have "$cand"; then ps="$cand"; break; fi
  done
  [ -n "$ps" ] || { log_warn "未找到 7z / zip / PowerShell，无法生成 zip 归档"; return 1; }

  local win_src win_dst
  if have cygpath; then
    win_src="$(cygpath -w "${parent}/${entry}")"
    win_dst="$(cygpath -w "$zip_path")"
  else
    win_src="${parent}/${entry}"
    win_dst="$zip_path"
  fi
  log_info "使用 $ps Compress-Archive 打包"
  "$ps" -NoProfile -NonInteractive -ExecutionPolicy Bypass -Command \
    "Compress-Archive -Path '${win_src}' -DestinationPath '${win_dst}' -Force" >/dev/null 2>&1
  [ -s "$zip_path" ]
}

# generate_icon_png <尺寸> <输出>：sips(macOS) → ImageMagick → rsvg-convert(矢量) → 原图复制
generate_icon_png() {
  local size="$1" output="$2"
  if [ ! -f "$ICON_PNG" ] && [ ! -f "$ICON_SVG" ]; then
    log_warn "icon.png / icon.svg 均不存在"
    return 1
  fi
  if have sips && [ -f "$ICON_PNG" ]; then
    sips -z "$size" "$size" "$ICON_PNG" --out "$output" 2>/dev/null && return 0
  fi
  if [ -f "$ICON_PNG" ]; then
    if have magick; then
      magick "$ICON_PNG" -resize "${size}x${size}" "$output" 2>/dev/null && return 0
    elif have convert; then
      convert "$ICON_PNG" -resize "${size}x${size}" "$output" 2>/dev/null && return 0
    fi
  fi
  # Linux/CI: 直接从 SVG 渲染，质量优于位图缩放
  if have rsvg-convert && [ -f "$ICON_SVG" ]; then
    rsvg-convert -w "$size" -h "$size" -o "$output" "$ICON_SVG" 2>/dev/null && return 0
  fi
  [ -f "$ICON_PNG" ] && { cp "$ICON_PNG" "$output" 2>/dev/null && return 0; }
  return 1
}

# Windows .ico：ImageMagick 优先，缺失时用 PowerShell + System.Drawing 组装
# 多尺寸 PNG-in-ICO（Vista+ 支持），避免为构建额外安装图像工具
generate_ico() {
  local output="$1"
  [ -f "$ICON_PNG" ] || { log_warn "icon.png 不存在，跳过 .ico 生成"; return 1; }

  if have magick; then
    magick "$ICON_PNG" -define icon:auto-resize=256,128,64,48,32,16 "$output" 2>/dev/null \
      && [ -s "$output" ] && return 0
  elif have convert; then
    convert "$ICON_PNG" -define icon:auto-resize=256,128,64,48,32,16 "$output" 2>/dev/null \
      && [ -s "$output" ] && return 0
  fi

  local ps="" cand
  for cand in powershell.exe powershell pwsh.exe pwsh; do
    if have "$cand"; then ps="$cand"; break; fi
  done
  [ -n "$ps" ] || { log_warn "无 ImageMagick / PowerShell，跳过 .ico 生成"; return 1; }

  log_info "使用 $ps 从 icon.png 生成 .ico ..."
  # Git Bash 下路径是 /c/... 形式，.NET 不识别，需 cygpath 转换
  local icon_arg="$ICON_PNG" out_arg="$output"
  if have cygpath; then
    icon_arg="$(cygpath -w "$ICON_PNG")"
    out_arg="$(cygpath -w "$output")"
  fi
  mkdir -p "$(dirname "$output")"
  FMT_ICON_PNG="$icon_arg" FMT_ICO_OUT="$out_arg" "$ps" -NoProfile -NonInteractive \
    -ExecutionPolicy Bypass -Command "$(cat <<'PS_EOF'
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Drawing
$src = $env:FMT_ICON_PNG
$dst = $env:FMT_ICO_OUT
$sizes = @(16, 32, 48, 64, 128, 256)
$img = [System.Drawing.Image]::FromFile($src)
$blobs = New-Object System.Collections.ArrayList
foreach ($s in $sizes) {
    $bmp = New-Object System.Drawing.Bitmap ($s, $s)
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    $g.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
    $g.DrawImage($img, 0, 0, $s, $s)
    $g.Dispose()
    $ms = New-Object System.IO.MemoryStream
    $bmp.Save($ms, [System.Drawing.Imaging.ImageFormat]::Png)
    [void]$blobs.Add(@{ size = $s; data = $ms.ToArray() })
    $ms.Dispose(); $bmp.Dispose()
}
$img.Dispose()
$fs = [System.IO.File]::Create($dst)
$bw = New-Object System.IO.BinaryWriter $fs
$bw.Write([uint16]0); $bw.Write([uint16]1); $bw.Write([uint16]$blobs.Count)
$offset = 6 + (16 * $blobs.Count)
foreach ($e in $blobs) {
    $dim = if ($e.size -ge 256) { 0 } else { $e.size }
    $bw.Write([byte]$dim); $bw.Write([byte]$dim)
    $bw.Write([byte]0); $bw.Write([byte]0)
    $bw.Write([uint16]1); $bw.Write([uint16]32)
    $bw.Write([uint32]$e.data.Length); $bw.Write([uint32]$offset)
    $bw.Write($e.data)
    $offset += $e.data.Length
}
$bw.Flush(); $bw.Close(); $fs.Dispose()
PS_EOF
)" >/dev/null 2>&1
  [ -s "$output" ]
}

generate_icns() {
  local output_icns="$1"
  [ -f "$ICON_PNG" ] || { log_warn "icon.png 不存在，跳过 .icns 生成"; return 1; }

  log_info "从 icon.png 生成 .icns ..."
  local tmp_base iconset_dir
  tmp_base="$(mktemp -d)"
  iconset_dir="${tmp_base}/AppIcon.iconset"
  mkdir -p "$iconset_dir"

  # 生成所有尺寸，允许部分失败（至少需要一个成功）
  local ok_count=0 total=0 s s2
  for s in 16 32 128 256 512; do
    s2=$((s * 2))
    total=$((total + 2))
    if generate_icon_png "$s" "${iconset_dir}/icon_${s}x${s}.png"; then ok_count=$((ok_count + 1)); fi
    if generate_icon_png "$s2" "${iconset_dir}/icon_${s}x${s}@2x.png"; then ok_count=$((ok_count + 1)); fi
  done
  if [ "$ok_count" -eq 0 ]; then
    log_warn "所有尺寸 PNG 生成均失败，无法创建 .icns"
    rm -rf "$tmp_base"
    return 1
  fi
  log_info "PNG 生成: ${ok_count}/${total} 成功"

  if have iconutil && iconutil -c icns "$iconset_dir" -o "$output_icns" 2>/dev/null; then
    rm -rf "$tmp_base"
    log_success ".icns 生成成功: $(basename "$output_icns")"
    return 0
  fi
  log_warn "iconutil 不可用或 .icns 转换失败"
  rm -rf "$tmp_base"
  return 1
}

# 手工装配 .app 的 Info.plist（非 Wails 生成，故整份写出）
generate_info_plist() {
  local version plist="$1"
  version=$(get_version)
  cat > "$plist" << PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleName</key>
    <string>${APP_NAME}</string>
    <key>CFBundleDisplayName</key>
    <string>${APP_NAME}</string>
    <key>CFBundleIdentifier</key>
    <string>${PROJECT_BUNDLE_ID}</string>
    <key>CFBundleVersion</key>
    <string>${version}</string>
    <key>CFBundleShortVersionString</key>
    <string>${version}</string>
    <key>CFBundleExecutable</key>
    <string>${APP_NAME}</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleIconFile</key>
    <string>AppIcon</string>
    <key>NSHighResolutionCapable</key>
    <true/>
    <key>LSMinimumSystemVersion</key>
    <string>${MACOS_MIN_VERSION}</string>
</dict>
</plist>
PLIST
}

# 整包 ad-hoc 签名：内嵌工具改变后需重签 bundle（母版 adhoc_sign 只签裸二进制）。
# bundle 内有 Resources/bin/* 独立二进制，必须逐个签名后再签整包
codesign_bundle() {
  # 复用母版 adhoc_sign：清 xattr + 内嵌 Mach-O 逐个签名 + 整包签名 + 校验。
  # 曾只用 --deep 整包签，Resources/bin 下的工具仍是 not signed at all，
  # 分发后 Gatekeeper 会判「已损坏」；内嵌工具改动后必须重签。
  local target="$1" app
  app=$(get_app_bundle)
  [ "$target" = "$app" ] || log_warn "签名目标与 get_app_bundle 不一致: ${target} vs ${app}"
  adhoc_sign
}

# =============================================================================
# 依赖准备（Linux 多发行版包管理器；macOS 额外备 7z 供 DMG 回退归档）
# =============================================================================
linux_pkg_has() {
  local pkg="$1"
  if have apt-get; then
    apt_refresh_once
    apt-cache show "$pkg" >/dev/null 2>&1
  elif have dnf;   then dnf -q list "$pkg" >/dev/null 2>&1
  elif have yum;   then yum -q list "$pkg" >/dev/null 2>&1
  elif have pacman; then pacman -Si "$pkg" >/dev/null 2>&1
  elif have zypper; then zypper info -t package "$pkg" >/dev/null 2>&1
  else return 1
  fi
}

linux_pkg_install() {
  [ $# -eq 0 ] && return 0
  log_info "安装 Linux 构建依赖: $*"
  local pm=""
  if have apt-get; then pm="apt-get"
  elif have dnf; then pm="dnf"
  elif have yum; then pm="yum"
  elif have pacman; then pm="pacman"
  elif have zypper; then pm="zypper"
  fi
  # shellcheck disable=SC2086
  case "$pm" in
    apt-get) apt_refresh_once
             DEBIAN_FRONTEND=noninteractive $SUDO apt-get install -y --no-install-recommends "$@" ;;
    dnf|yum) $SUDO "$pm" install -y "$@" ;;
    pacman)  $SUDO pacman -S --noconfirm --needed "$@" ;;
    zypper)  $SUDO zypper --non-interactive install "$@" ;;
    *)       log_warn "未识别的包管理器，请手动安装: $*"; return 1 ;;
  esac || { log_warn "包安装失败: $*"; return 1; }
  return 0
}

# Wails v2 源码只用 webkit2gtk-4.0 的 pkg-config 名，要链 4.1 必须加 webkit2_41 tag
# （Ubuntu 23.10+ / Debian 12+ 已不再提供 4.0 开发包）
resolve_webkit_variant() {
  if pkg-config --exists webkit2gtk-4.1 2>/dev/null; then
    WEBKIT_PC="webkit2gtk-4.1"; WEBKIT_TAG="webkit2_41"; return 0
  fi
  if pkg-config --exists webkit2gtk-4.0 2>/dev/null; then
    WEBKIT_PC="webkit2gtk-4.0"; WEBKIT_TAG=""; return 0
  fi
  if linux_pkg_has libwebkit2gtk-4.1-dev; then
    WEBKIT_PC="webkit2gtk-4.1"; WEBKIT_TAG="webkit2_41"; WEBKIT_PKG_TO_INSTALL="libwebkit2gtk-4.1-dev"
  elif linux_pkg_has libwebkit2gtk-4.0-dev; then
    WEBKIT_PC="webkit2gtk-4.0"; WEBKIT_TAG=""; WEBKIT_PKG_TO_INSTALL="libwebkit2gtk-4.0-dev"
  else
    WEBKIT_PC=""; WEBKIT_TAG=""; WEBKIT_PKG_TO_INSTALL=""
    return 1
  fi
  return 0
}

ensure_macos_deps() {
  [ -n "$(command -v clang 2>/dev/null)" ] \
    || log_error "缺少 clang，请安装 Xcode Command Line Tools: xcode-select --install"
  xcrun --sdk macosx --show-sdk-path >/dev/null 2>&1 \
    || log_error "缺少 macOS SDK (xcrun --sdk macosx --show-sdk-path)"
  # 7z 压缩率最高（.app 内嵌工具体量大）；缺失时尝试 brew，仍缺失则归档回退 zip
  if ! resolve_sevenzip; then
    if have brew; then
      log_info "brew install p7zip（用于 7z 打包）..."
      run brew install p7zip >/dev/null 2>&1 || true
      resolve_sevenzip || log_warn "p7zip 安装失败，将回退 zip 打包"
    else
      log_warn "未找到 7z 且无 brew，将回退 zip 打包"
    fi
  fi
  log_success "macOS 构建依赖就绪 (clang + macOS SDK，最低部署版本 ${MACOS_MIN_VERSION})"
}

ensure_linux_deps() {
  local need=()
  have pkg-config || need+=(pkg-config)
  have gcc        || need+=(build-essential)
  have zip        || need+=(zip)
  pkg-config --exists gtk+-3.0 2>/dev/null || need+=(libgtk-3-dev)

  if ! resolve_webkit_variant; then
    log_error "未找到可匹配的 WebKitGTK 开发包 (libwebkit2gtk-4.0-dev / 4.1-dev)，请确认软件源已刷新"
  fi
  [ -n "$WEBKIT_PKG_TO_INSTALL" ] && need+=("$WEBKIT_PKG_TO_INSTALL")

  # 图标缩放：Linux 无 sips，用 librsvg 直接从 SVG 矢量渲染（已有缩放工具则不装）
  if ! have magick && ! have convert && ! have rsvg-convert; then
    linux_pkg_has librsvg2-bin && need+=(librsvg2-bin)
  fi

  [ ${#need[@]} -eq 0 ] || linux_pkg_install "${need[@]}" || log_error "Linux 构建依赖安装失败"

  if ! pkg-config --exists gtk+-3.0 "$WEBKIT_PC" 2>/dev/null; then
    log_error "pkg-config 无法解析 gtk+-3.0 / ${WEBKIT_PC}：$(pkg-config --errors --exists gtk+-3.0 "$WEBKIT_PC" 2>&1 | head -3)"
  fi
  log_success "Linux 构建依赖就绪 (${WEBKIT_PC} $(pkg-config --modversion "$WEBKIT_PC")${WEBKIT_TAG:+, build tag: ${WEBKIT_TAG}})"
}

# =============================================================================
# 编译 / 三方工具内嵌（母版 cmd_build 依次调用 build_impl → hook_post_build →
# adhoc_sign → package_platform）
# =============================================================================
build_impl() {
  local ldflags="$1"
  local bin_path cgo_enabled=1 tags="$BUILD_TAGS"
  bin_path=$(get_build_binary)
  mkdir -p "$BUILD_DIR"

  case "$HOST_OS" in
    linux)
      # Linux 桌面构建需 GTK3 + WebKit2GTK；cgo 标志由源码内 `#cgo pkg-config:` 自行解析
      # （跨 OS 构建已被母版 cross_os_guard 早拦，此处只校验依赖是否就绪）
      [ -n "$WEBKIT_PC" ] || resolve_webkit_variant || true
      if [ -z "$WEBKIT_PC" ] || ! pkg-config --exists gtk+-3.0 "$WEBKIT_PC" 2>/dev/null; then
        log_error "Linux 桌面构建需要 GTK3 与 ${WEBKIT_PC:-WebKit2GTK} 开发库（去掉 --skip-deps 可自动安装）"
      fi
      [ -n "$WEBKIT_TAG" ] && tags="${tags},${WEBKIT_TAG}"
      log_info "WebKit: ${WEBKIT_PC}${WEBKIT_TAG:+ (build tag: ${WEBKIT_TAG})}"
      ;;
    darwin)
      # Wails 在 macOS 需 Cocoa/WebKit + UniformTypeIdentifiers 框架（CGO）
      export CGO_LDFLAGS="${CGO_LDFLAGS:-} -framework UniformTypeIdentifiers"
      # darwin 跨架构编译（amd64↔arm64）需给 clang 指定 target triple
      if is_cross_arch; then
        local target=""
        case "$HOST_ARCH" in
          amd64) target="x86_64-apple-darwin" ;;
          arm64) target="arm64-apple-darwin" ;;
        esac
        if [ -n "$target" ]; then
          export CGO_CFLAGS="${CGO_CFLAGS:-} -target ${target}"
          export CGO_LDFLAGS="${CGO_LDFLAGS:-} -target ${target}"
          log_info "交叉编译目标: ${target}"
        fi
      fi
      log_info "最低部署版本: macOS ${MACOS_MIN_VERSION}"
      ;;
    windows)
      # go-webview2 自带 WebView2Loader，纯 Go 链接，不需要 C 工具链（MinGW）
      cgo_enabled=0
      ;;
  esac

  log_info "Go 依赖 download / verify ..."
  run go mod download
  run go mod verify

  log_info "go build (-tags ${tags}, CGO_ENABLED=${cgo_enabled}) → ${bin_path#$PROJECT_DIR/}"
  # shellcheck disable=SC2086
  run env CGO_ENABLED="$cgo_enabled" GOOS="$HOST_OS" GOARCH="$HOST_ARCH" \
    go build -trimpath -tags "$tags" -ldflags "$ldflags" -o "$bin_path" ./src/

  if $DRY_RUN; then return 0; fi
  [ -f "$bin_path" ] || log_error "编译产物不存在: $bin_path"
  log_success "二进制构建完成 ($(du -h "$bin_path" | cut -f1))"
}

# download_bundled_tools 下载随 App 分发的独立二进制到 data/bin/<os>/<arch>/。
# 必须每次执行（而非目录非空即跳过）：源码树只提交了 preset 包装脚本，
# download 类工具需构建时拉取；install-bin.sh 自身会跳过已安装工具（幂等）
download_bundled_tools() {
  [ -f "$INSTALL_BIN_SCRIPT" ] || { log_warn "install-bin.sh 不存在，跳过工具下载"; return 0; }
  log_info "检查并下载随 App 分发的独立二进制工具（${HOST_OS}/${HOST_ARCH}）..."
  # --bundled-only: 仅处理 download 类工具，跳过 source=install（如 rubocop 的 gem install，
  # 不随 App 分发，在构建机上安装也没有意义）
  run bash "$INSTALL_BIN_SCRIPT" "--platform=${HOST_OS}/${HOST_ARCH}" --bundled-only --yes
}

# embed_tools <目标目录>：平台特定工具 + 跨平台文件（JAR 等）拷入发布包
embed_tools() {
  local dest="$1" src_dir
  src_dir="$(bin_dir)"
  local has_platform=false has_cross=false
  dir_has_files "$src_dir" && has_platform=true
  dir_has_files "$BIN_ROOT" && has_cross=true
  if ! $has_platform && ! $has_cross; then
    log_warn "bin/ 为空，跳过三方工具内嵌"
    return 0
  fi
  rm -rf "$dest"
  mkdir -p "$dest"
  $has_platform && { cp -R "${src_dir}"/* "$dest/" 2>/dev/null || true; }
  # 跨平台工具（google-java-format.jar 等，安装时直接放在 bin/ 而非平台子目录）
  if $has_cross; then
    local f base
    for f in "$BIN_ROOT"/*; do
      [ -f "$f" ] || continue
      base="$(basename "$f")"
      [ -f "$dest/$base" ] || cp "$f" "$dest/" 2>/dev/null || true
    done
  fi
  local count
  count=$(find "$dest" -maxdepth 1 -mindepth 1 2>/dev/null | wc -l | tr -d ' ')
  log_success "三方工具已内嵌 → ${dest#$PROJECT_DIR/}/ (${count} 个)"
}

hook_post_build() {
  download_bundled_tools
  if $DRY_RUN; then
    log_warn "[dry-run] embed_tools → $BUILD_DIR/bin"
    return 0
  fi
  embed_tools "$BUILD_DIR/bin"
}

# ============================================================
# 分平台打包（覆盖母版 package_platform）
# ============================================================
package_platform() {
  if $DRY_RUN; then
    log_warn "[dry-run] 装配并归档 ${HOST_OS}/${HOST_ARCH} 产物 → release/ ($(artifact_base))"
    return 0
  fi
  case "$HOST_OS" in
    darwin)  package_macos ;;
    linux)   package_linux ;;
    windows) package_windows ;;
    *)       log_error "未知目标平台: $HOST_OS" ;;
  esac
}

# macOS: .app bundle（icns + 内嵌工具 + Info.plist）→ DMG（回退 7z / zip）
package_macos() {
  local app_dir contents_dir macos_dir resources_dir bin_path
  app_dir=$(get_app_bundle)
  contents_dir="${app_dir}/Contents"
  macos_dir="${contents_dir}/MacOS"
  resources_dir="${contents_dir}/Resources"
  bin_path=$(get_build_binary)

  log_info "打包 macOS .app ..."
  rm -rf "$app_dir"
  mkdir -p "$macos_dir" "$resources_dir"
  cp -f "$bin_path" "${macos_dir}/${APP_NAME}"
  chmod +x "${macos_dir}/${APP_NAME}"

  # 不在 bundle 内落 config.json：默认配置已由 //go:embed data/config.json 编进二进制，
  # 再拷一份到 Contents/ 只是把运行时写入点引进 codesign 封印范围（写任何文件都会让
  # 整包签名失效，分发后被 Gatekeeper 判「已损坏，无法打开」）。
  # App 内改的配置统一落 ~/.formatter/config.json，查找顺序见 src/config/config.go。

  # 图标生成是纯图像处理（PNG→多尺寸→icns），与目标架构无关，交叉编译时照常执行
  generate_icns "${resources_dir}/AppIcon.icns" || log_warn "图标生成失败，跳过"
  embed_tools "${resources_dir}/bin"
  generate_info_plist "${contents_dir}/Info.plist"
  log_success ".app 打包完成: ${app_dir#$PROJECT_DIR/}（最低系统版本 ${MACOS_MIN_VERSION}）"

  # 整包重签：内嵌二进制路径变化后重新签名，降低 Gatekeeper 拦截概率
  codesign_bundle "$app_dir"

  # .app 仅留在 build/；release/ 只放成品归档
  if ! build_dmg_macos "$app_dir"; then
    archive_macos "$app_dir" || log_error "macOS 归档失败"
  fi
  print_usage_hint macos
}

# DMG（拖拽安装）: 卷内 App + Applications 快捷方式。hdiutil 把符号链接写成 Finder
# 别名，一次直出 UDZO，全程不 attach、不做 Finder 定制，打包零窗口弹出
build_dmg_macos() {
  local app_dir="$1"
  have hdiutil || { log_warn "hdiutil 不可用，回退 7z/zip 打包"; return 1; }

  local dmg_path="${PROJECT_DIR}/release/$(artifact_base).dmg"
  rm -f "$dmg_path"
  mkdir -p "${PROJECT_DIR}/release"

  local staging
  staging="$(mktemp -d "${TMPDIR:-/tmp}/${APP_NAME}_dmg.XXXXXX")" || {
    log_warn "mktemp 失败，回退 7z/zip 打包"; return 1; }
  trap "rm -rf '$staging'" EXIT

  log_info "打包 DMG（拖拽安装）: $(basename "$dmg_path") ..."
  # ditto 保真复制（权限位/符号链接/xattr）；暂存副本再校验一次签名，
  # 坏了宁可告警也不要静默产出「装完打不开」的安装包
  if ditto "$app_dir" "${staging}/${APP_NAME}.app" &&
     ln -s /Applications "${staging}/Applications"; then
    codesign --verify --deep --strict "${staging}/${APP_NAME}.app" >/dev/null 2>&1 \
      || log_warn "DMG 暂存副本签名校验失败，分发后可能无法打开"
    if hdiutil create -volname "$APP_NAME" -srcfolder "$staging" -ov -format UDZO "$dmg_path" \
         >/dev/null 2>&1 && [ -s "$dmg_path" ]; then
      rm -rf "$staging"
      trap - EXIT
      log_success "DMG 打包成功: $(basename "$dmg_path") ($(du -h "$dmg_path" | cut -f1))"
      log_info "首次打开若提示「无法验证开发者」：右键 → 打开；或执行："
      log_info "  xattr -dr com.apple.quarantine \"/Applications/${APP_NAME}.app\""
      return 0
    fi
  fi
  rm -rf "$staging"
  trap - EXIT
  rm -f "$dmg_path"
  log_warn "DMG 生成失败，回退 7z/zip 打包"
  return 1
}

# 归档回退: 7z（最大压缩）→ zip（ditto / 7z / PowerShell），仅 DMG 失败时使用
archive_macos() {
  local app_dir="$1" base archive
  base="$(artifact_base)"
  # 清理隔离属性，避免归档进 com.apple.quarantine 等扩展属性
  xattr -cr "$app_dir" 2>/dev/null || true

  if resolve_sevenzip; then
    archive="${PROJECT_DIR}/release/${base}.7z"
    log_info "打包 7z（最大压缩）: ${base}.7z"
    rm -f "$archive"
    if (cd "$(dirname "$app_dir")" &&
        "$SEVENZIP" a -m0=lzma2 -mx=9 -mmt=on "-x!*.DS_Store" "$archive" "${APP_NAME}.app" \
        >/dev/null 2>&1) && [ -s "$archive" ]; then
      log_success "7z 打包成功: $(basename "$archive") ($(du -h "$archive" | cut -f1))"
      return 0
    fi
    log_warn "7z 打包失败，回退 zip"
    rm -f "$archive"
  else
    log_warn "未找到 7z（brew install p7zip），回退 zip 打包"
  fi

  archive="${PROJECT_DIR}/release/${base}.zip"
  log_info "打包 zip: ${base}.zip"
  rm -f "$archive"
  # macOS 自带 ditto 能保留 .app 的扩展属性与目录结构
  if have ditto; then
    ditto -c -k --keepParent "$app_dir" "$archive" >/dev/null 2>&1 || true
  fi
  [ -s "$archive" ] || archive_zip "$archive" "$app_dir" || return 1
  log_success "zip 打包成功: $(basename "$archive") ($(du -h "$archive" | cut -f1))"
}

# Linux: bin/ + share/{applications,icons} + install.sh → tar.gz
package_linux() {
  local stage_root="${BUILD_DIR}/linux_stage" stage_dir bin_path
  stage_dir="${stage_root}/${APP_NAME}"
  bin_path=$(get_build_binary)

  log_info "打包 Linux 桌面应用 ..."
  rm -rf "$stage_root"
  mkdir -p "${stage_dir}/bin" \
           "${stage_dir}/share/applications" \
           "${stage_dir}/share/icons/hicolor/256x256/apps"

  cp -f "$bin_path" "${stage_dir}/bin/${PROJECT_NAME}"
  chmod +x "${stage_dir}/bin/${PROJECT_NAME}"
  [ -f "${DATA_DIR}/config.json" ] && cp -f "${DATA_DIR}/config.json" "${stage_dir}/bin/config.json"

  if [ -f "$LINUX_DESKTOP" ]; then
    cp -f "$LINUX_DESKTOP" "${stage_dir}/share/applications/"
    log_success ".desktop 文件已嵌入"
  else
    log_warn "缺少 ${LINUX_DESKTOP#$PROJECT_DIR/}，跳过 .desktop"
  fi

  local icon_png="${stage_dir}/share/icons/hicolor/256x256/apps/${PROJECT_NAME}.png"
  if generate_icon_png 256 "$icon_png"; then
    log_success "图标已嵌入（256x256）"
  else
    log_warn "图标生成失败，跳过"
    rm -f "$icon_png"
  fi

  embed_tools "${stage_dir}/bin/tools"

  cat > "${stage_dir}/install.sh" << 'INSTALL_EOF'
#!/bin/bash
# Formatter 桌面应用安装脚本 (Linux)
set -euo pipefail
cd "$(dirname "$0")"   # 无论从哪里调用，bin/ share/ 均相对解压目录
INSTALL_DIR="/opt/formatter"
DESKTOP_DIR="$HOME/.local/share/applications"
ICON_DIR="$HOME/.local/share/icons/hicolor/256x256/apps"

echo "安装 Formatter 到 ${INSTALL_DIR} ..."
sudo mkdir -p "$INSTALL_DIR"
sudo cp -R bin/* "$INSTALL_DIR/"

mkdir -p "$DESKTOP_DIR" "$ICON_DIR"
[ -f share/applications/formatter.desktop ] && \
    cp share/applications/formatter.desktop "$DESKTOP_DIR/"
[ -f share/icons/hicolor/256x256/apps/formatter.png ] && \
    cp share/icons/hicolor/256x256/apps/formatter.png "$ICON_DIR/"

sudo ln -sf "$INSTALL_DIR/formatter" /usr/local/bin/formatter 2>/dev/null || true
echo "安装完成！运行 'formatter' 或从应用菜单启动"
INSTALL_EOF
  chmod +x "${stage_dir}/install.sh"

  local archive="${PROJECT_DIR}/release/$(artifact_base).tar.gz"
  log_info "打包 tar.gz: $(artifact_base).tar.gz"
  rm -f "$archive"
  if tar -czf "$archive" -C "$stage_root" "$APP_NAME" 2>/dev/null && [ -s "$archive" ]; then
    log_success "tar.gz 打包成功: $(basename "$archive") ($(du -h "$archive" | cut -f1))"
  else
    rm -rf "$stage_root"
    log_error "tar.gz 打包失败"
  fi
  rm -rf "$stage_root"
  print_usage_hint linux
}

# Windows: Formatter.exe + config.json + icon.ico + tools/ → zip
package_windows() {
  local stage_root="${BUILD_DIR}/win_stage" stage_dir bin_path
  stage_dir="${stage_root}/${APP_NAME}"
  bin_path=$(get_build_binary)

  log_info "打包 Windows 桌面应用 ..."
  rm -rf "$stage_root"
  mkdir -p "$stage_dir"
  cp -f "$bin_path" "${stage_dir}/${APP_NAME}.exe"
  [ -f "${DATA_DIR}/config.json" ] && cp -f "${DATA_DIR}/config.json" "${stage_dir}/config.json"

  if generate_ico "${stage_dir}/icon.ico"; then
    log_success "图标已嵌入（icon.ico）"
  else
    rm -f "${stage_dir}/icon.ico"
  fi

  embed_tools "${stage_dir}/tools"

  local archive="${PROJECT_DIR}/release/$(artifact_base).zip"
  mkdir -p "${PROJECT_DIR}/release"
  log_info "打包 zip: $(artifact_base).zip"
  archive_zip "$archive" "$stage_dir" || { rm -rf "$stage_root"; log_error "zip 打包失败"; }
  log_success "zip 打包成功: $(basename "$archive") ($(du -h "$archive" | cut -f1))"
  rm -rf "$stage_root"
  print_usage_hint windows
}

# 产物使用方式提示（Linux/Windows 路径相对解压目录）
print_usage_hint() {
  case "$1" in
    macos)
      local app_dir; app_dir=$(get_app_bundle)
      log_info "拖拽安装: 双击挂载 release/$(artifact_base).dmg，将 ${APP_NAME}.app 拖入 Applications"
      log_info "本地调试: open ${app_dir#$PROJECT_DIR/}"
      log_info "CLI 调用: ${app_dir}/Contents/MacOS/${APP_NAME} format main.go"
      log_info "Web UI:   ${app_dir}/Contents/MacOS/${APP_NAME} serve"
      ;;
    linux)
      log_info "桌面应用: 解压后运行 ${APP_NAME}/install.sh，或直接执行 ${APP_NAME}/bin/${PROJECT_NAME}"
      log_info "CLI 调用: ${APP_NAME}/bin/${PROJECT_NAME} format main.go"
      log_info "Web UI:   ${APP_NAME}/bin/${PROJECT_NAME} serve"
      ;;
    windows)
      log_info "桌面应用: 解压后运行 ${APP_NAME}/${APP_NAME}.exe"
      log_info "CLI 调用: ${APP_NAME}/${APP_NAME}.exe format main.go"
      log_info "Web UI:   ${APP_NAME}/${APP_NAME}.exe serve"
      ;;
  esac
}

# ============================================================
# 项目专属命令
# ============================================================
# install-bin: 下载/校验随 App 分发的三方二进制工具（委托 install-bin.sh）
cmd_install_bin() {
  check_go
  local args=("--platform=${HOST_OS}/${HOST_ARCH}")
  if [ $# -gt 0 ]; then
    args=("$@")
    case "${args[*]}" in
      *--platform*) ;;
      *) args+=("--platform=${HOST_OS}/${HOST_ARCH}") ;;
    esac
  fi
  [ -f "$INSTALL_BIN_SCRIPT" ] || log_error "安装脚本不存在: $INSTALL_BIN_SCRIPT"
  log_info "委托给 install-bin.sh ${args[*]} ..."
  run bash "$INSTALL_BIN_SCRIPT" "${args[@]}" || log_warn "部分工具安装失败，请查看上方日志"
}

# server: 构建后以 serve 命令前台启动 Web Server（同一二进制的服务端形态）
cmd_server() {
  cmd_build
  local bin_path
  bin_path=$(get_build_binary)
  if $DRY_RUN; then
    log_warn "[dry-run] $bin_path serve $*"
    return 0
  fi
  [ -x "$bin_path" ] || log_error "未找到可执行文件: $bin_path"
  log_info "启动 Web Server: ${bin_path#$PROJECT_DIR/} serve $*（Ctrl+C 停止）"
  set +e
  if [ $# -gt 0 ]; then
    "$bin_path" serve "$@"
  else
    "$bin_path" serve
  fi
  set -e
}

# test: 覆盖率统计以 src/ 为被测面（tests/ 为外置测试包）
cmd_test() {
  check_go
  [ -d "${PROJECT_DIR}/tests" ] || log_error "tests/ 目录不存在"
  local cover rc
  cover="$(mktemp -t formatter_cover_XXXXXX.out)"
  if $DRY_RUN; then
    log_warn "[dry-run] go test -coverpkg=./src/... -coverprofile=$cover -covermode=atomic ./tests/..."
    rm -f "$cover"
    return 0
  fi
  log_info "运行 Go 单元测试（覆盖率: ./src/...）..."
  set +e
  go test -coverpkg=./src/... -coverprofile="$cover" -covermode=atomic ./tests/...
  rc=$?
  set -e
  if [ "$rc" -ne 0 ]; then
    rm -f "$cover"
    log_error "测试失败（exit=$rc）"
  fi
  if [ -s "$cover" ]; then
    log_success "全部测试通过，总覆盖率: $(go tool cover -func="$cover" | tail -1 | awk '{print $NF}')"
  else
    log_success "全部测试通过"
  fi
  rm -f "$cover"
}

show_help_extra() {
  cat <<'EOF'

Formatter 专属说明:
  - 同一二进制兼作桌面 App / CLI / Web Server，编译入口 go build -tags desktop,production ./src/
  - 三平台交叉产物由 --platform=<os>/<arch> 指定，CI 四个 job 分别构建：
      darwin/amd64 darwin/arm64（同一 arm64 runner 交叉，自动补 clang -target）
      linux/amd64 windows/amd64（Windows 为 CGO_ENABLED=0，无需 MinGW）
  - 产物命名统一 Formatter-<版本>-<os>-<arch>，全部落 release/：
      macOS .dmg（hdiutil 不可用回退 .7z/.zip）/ Linux .tar.gz / Windows .zip
  - 三方工具由 install-bin 下载到 data/bin/<os>/<arch>/，打包时内嵌进
      macOS: Contents/Resources/bin  Linux: <包>/bin/tools  Windows: <包>/tools
EOF
}

main "$@"
