#!/usr/bin/env bash
set -euo pipefail

# =============================================================================
# run_tools.sh - Formatter 项目工具脚本
# =============================================================================
# 全部构建相关逻辑 (系统依赖安装 / CGO 参数 / build tags / 图标生成 / 打包)
# 都集中在本脚本内实现。CI (.github/workflows/release.yml) 只负责 runner 引导
# (checkout、Go 工具链) 与产物上传，不写任何依赖安装或打包细节，避免 CI 与本地漂移。
#
# 三平台 App 打包 (CI 中每个 job 在对应 OS 的 runner 上构建，脚本按宿主 OS 自动准备依赖):
#   darwin/amd64|arm64 → .app bundle (icns + 内嵌工具) + ad-hoc 签名 → .7z (无 7z 时回退 .zip)
#   linux/amd64        → tar.gz (bin/ + .desktop + 图标 + tools/ + install.sh)
#   windows/amd64      → zip (Formatter.exe + config.json + icon.ico + tools/)
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
PROJECT_NAME="formatter"
APP_NAME="Formatter"            # App / 可执行文件展示名
PROJECT_BUNDLE_ID="com.xthuji.formatter"
APP_VERSION="$(cat "${PROJECT_ROOT}/data/version.txt" 2>/dev/null | tr -d '[:space:]')"
APP_VERSION="${APP_VERSION:-1.0.0}" # 源自 data/version.txt，回退默认值
BUILD_DIR="${PROJECT_ROOT}/build"
RELEASE_DIR="${PROJECT_ROOT}/release"
DATA_DIR="${PROJECT_ROOT}/data"
BIN_ROOT="${DATA_DIR}/bin"          # 跨平台文件目录 (JAR 等)
# BIN_DIR 在 detect_platform() 后设置为 ${BIN_ROOT}/${TARGET_GOOS}/${TARGET_GOARCH}
TESTS_DIR="${PROJECT_ROOT}/tests"
ICON_PNG="${DATA_DIR}/icon/icon.png"   # 预生成 1024x1024 位图 (构建时不再依赖 SVG 光栅化)
LINUX_DESKTOP="${DATA_DIR}/linux/formatter.desktop"
# macOS 最低部署版本: 由脚本统一决定 (workflow 不再设置环境变量)
MACOS_MIN_VERSION="${MACOSX_DEPLOYMENT_TARGET:-12.0}"
GO_MIN_VERSION=""               # 由 go.mod 的 go 指令解析 (见 resolve_go_min)
INSTALL_BIN_SCRIPT="${SCRIPT_DIR}/install-bin.sh"

# ---- 颜色定义 ----
RED=$'\033[0;31m'
GREEN=$'\033[0;32m'
YELLOW=$'\033[1;33m'
CYAN=$'\033[0;36m'
BOLD=$'\033[1m'
DIM=$'\033[2m'
NC=$'\033[0m'

# ---- 全局状态 ----
FAILED_STEPS=()
TARGET_GOOS="${TARGET_GOOS:-}"
TARGET_GOARCH="${TARGET_GOARCH:-}"
HOST_GOOS=""                  # 宿主平台 (决定可用的编译工具链)
HOST_GOARCH=""
PASSTHROUGH_ARGS=()
SKIP_DEPS=false               # --skip-deps: 跳过系统构建依赖自动安装 (已就绪的环境可用)
WEBKIT_PC=""                  # linux 桌面构建使用的 pkg-config 模块 (webkit2gtk-4.0 / 4.1)
WEBKIT_TAG=""                 # 使用 4.1 时追加的 build tag (webkit2_41)
SUDO=""                       # 非 root 且存在 sudo 时为 "sudo"
SUDO_RESOLVED=0
APT_REFRESHED=0

# ---- 基础工具函数 ----
# 使用 printf 替代 echo -e，更安全且跨平台兼容性更好
have() { command -v "$1" >/dev/null 2>&1; }
print_banner() {
    printf "\n%b═══════════════════════════════════════════════════════════════%b\n" "$CYAN" "$NC"
    printf "%b  %b%s%b\n" "$CYAN" "$BOLD" "$1" "$NC"
    printf "%b═══════════════════════════════════════════════════════════════%b\n" "$CYAN" "$NC"
}
print_step() { printf "%b[STEP]%b %s\n" "$CYAN" "$NC" "$1"; }
print_ok()   { printf "%b[OK]%b   %s\n" "$GREEN" "$NC" "$1"; }
print_fail() { printf "%b[FAIL]%b %s\n" "$RED" "$NC" "$1"; }
print_warn() { printf "%b[WARN]%b %s\n" "$YELLOW" "$NC" "$1"; }
print_info() { printf "%b[INFO]%b %s\n" "$CYAN" "$NC" "$1"; }

version_gte() {
    local ver1="$1" ver2="$2" IFS=. i
    read -ra v1 <<< "$ver1"
    read -ra v2 <<< "$ver2"
    for ((i=0; i<${#v2[@]}; i++)); do
        local a=${v1[i]:-0} b=${v2[i]:-0}
        if ((10#$a > 10#$b)); then return 0; fi
        if ((10#$a < 10#$b)); then return 1; fi
    done
    return 0
}

fail() { FAILED_STEPS+=("$1"); return 1; }

# 更安全和高效的目录空检查
dir_has_files() {
    local dir="$1"
    [[ -d "$dir" ]] || return 1
    shopt -s nullglob dotglob
    local files=("$dir"/*)
    shopt -u nullglob dotglob
    [[ ${#files[@]} -gt 0 ]]
}

# init_sudo 决定包安装命令是否加 sudo 前缀。
# root (容器) 直接执行；非 root 但有 sudo (GitHub Actions runner 为免密 sudo) 用 sudo；
# 否则仅告警 (依赖需用户手动安装，避免构建过程卡在密码提示上)。
init_sudo() {
    [[ "$SUDO_RESOLVED" == "1" ]] && return 0
    SUDO_RESOLVED=1
    if [[ "$(id -u 2>/dev/null || echo 0)" == "0" ]]; then
        SUDO=""
    elif have sudo; then
        SUDO="sudo"
    else
        SUDO=""
        print_warn "当前非 root 且无 sudo，系统依赖需手动安装"
    fi
    return 0
}

# resolve_go_min 从 go.mod 的 go 指令解析最低 Go 版本，避免脚本与模块声明漂移
resolve_go_min() {
    local from_gomod=""
    if [[ -f "${PROJECT_ROOT}/go.mod" ]]; then
        from_gomod="$(awk '/^go[[:space:]]+/ {print $2; exit}' "${PROJECT_ROOT}/go.mod" | tr -d '[:space:]')"
    fi
    [[ -n "$from_gomod" ]] || from_gomod="1.21"
    # 只取 major.minor，忽略 patch (go1.26.1 → 1.26)
    echo "$from_gomod" | awk -F. '{printf "%s.%s\n", $1, $2}'
}

# =============================================================================
# 归档工具定位 (7z 在 p7zip 包中为 7z/7za，在 7zip 包中为 7zz)
# =============================================================================
SEVENZIP=""
resolve_sevenzip() {
    [[ -n "$SEVENZIP" ]] && return 0
    local cand
    for cand in 7z 7zz 7za p7zip; do
        if have "$cand"; then SEVENZIP="$cand"; return 0; fi
    done
    return 1
}

# =============================================================================
# 归档辅助: zip 归档 (7z → zip → PowerShell Compress-Archive 逐级回退)
#   $1: 目标 zip 路径   $2: 待打包目录 (归档内保留该目录作为顶层条目)
# =============================================================================
archive_zip() {
    local zip_path="$1" src_dir="$2"
    local parent entry
    parent="$(dirname "$src_dir")"
    entry="$(basename "$src_dir")"
    mkdir -p "$parent"
    rm -f "$zip_path"

    if resolve_sevenzip; then
        (cd "$parent" && "$SEVENZIP" a -tzip -mx=9 "$zip_path" "$entry" >/dev/null 2>&1)
        [[ -s "$zip_path" ]] && return 0
        rm -f "$zip_path"
    fi
    if have zip; then
        (cd "$parent" && zip -qr "$zip_path" "$entry")
        [[ -s "$zip_path" ]] && return 0
        rm -f "$zip_path"
    fi
    # Git Bash 无 7z/zip 时交给 PowerShell
    local ps=""
    local cand
    for cand in powershell.exe powershell pwsh.exe pwsh; do
        if have "$cand"; then ps="$cand"; break; fi
    done
    [[ -n "$ps" ]] || { print_warn "未找到 7z / zip / PowerShell，无法生成 zip 归档"; return 1; }

    local win_src win_dst
    if have cygpath; then
        win_src="$(cygpath -w "${parent}/${entry}")"
        win_dst="$(cygpath -w "$zip_path")"
    else
        win_src="${parent}/${entry}"
        win_dst="$zip_path"
    fi
    print_info "  使用 $ps Compress-Archive 打包"
    "$ps" -NoProfile -NonInteractive -ExecutionPolicy Bypass -Command \
        "Compress-Archive -Path '$win_src' -DestinationPath '$win_dst' -Force" >/dev/null 2>&1
    [[ -s "$zip_path" ]]
}

# =============================================================================
# 平台检测
# =============================================================================
detect_platform() {
    # 宿主平台: 决定能否准备系统依赖 (桌面 App 无法跨 OS 编译，Linux 需要本机 GTK)
    if [[ -z "$HOST_GOOS" ]]; then
        case "$(uname -s)" in
            Darwin) HOST_GOOS="darwin" ;;
            Linux)  HOST_GOOS="linux" ;;
            MINGW*|MSYS*|CYGWIN*) HOST_GOOS="windows" ;;
            *) HOST_GOOS="unknown" ;;
        esac
    fi
    if [[ -z "$HOST_GOARCH" ]]; then
        case "$(uname -m)" in
            x86_64|amd64) HOST_GOARCH="amd64" ;;
            arm64|aarch64) HOST_GOARCH="arm64" ;;
            *) HOST_GOARCH="amd64" ;;
        esac
    fi
    if [[ -z "$TARGET_GOOS" ]]; then
        TARGET_GOOS="$HOST_GOOS"
        [[ "$TARGET_GOOS" == "unknown" ]] && TARGET_GOOS="darwin"
    fi
    if [[ -z "$TARGET_GOARCH" ]]; then
        TARGET_GOARCH="$HOST_GOARCH"
    fi
    # 平台特定的二进制目录: data/bin/{os}/{arch}/
    BIN_DIR="${BIN_ROOT}/${TARGET_GOOS}/${TARGET_GOARCH}"
}

# is_cross_arch 仅在「同 OS 不同架构」时为真 (macOS amd64↔arm64 交叉编译)
is_cross_arch() {
    [[ "$TARGET_GOOS" == "$HOST_GOOS" ]] && [[ "$TARGET_GOARCH" != "$HOST_GOARCH" ]]
}

# =============================================================================
# 图标生成逻辑 (基于预转换的 icon.png 1024x1024，缺失时回退 icon.svg 矢量渲染)
# =============================================================================
ICON_SVG="${DATA_DIR}/icon/icon.svg"

# generate_icon_png 缩放到指定尺寸。工具回退链:
#   sips (macOS 自带) → ImageMagick → rsvg-convert (从 SVG 矢量渲染) → 直接复制源 PNG
generate_icon_png() {
    local size="$1" output="$2"
    if [[ ! -f "$ICON_PNG" ]] && [[ ! -f "$ICON_SVG" ]]; then
        print_warn "icon.png/icon.svg 均不存在"
        return 1
    fi

    if have sips && [[ -f "$ICON_PNG" ]]; then
        sips -z "$size" "$size" "$ICON_PNG" --out "$output" 2>/dev/null && return 0
    fi
    if [[ -f "$ICON_PNG" ]]; then
        if have magick; then
            magick "$ICON_PNG" -resize "${size}x${size}" "$output" 2>/dev/null && return 0
        elif have convert; then
            convert "$ICON_PNG" -resize "${size}x${size}" "$output" 2>/dev/null && return 0
        fi
    fi
    # Linux/CI: 直接从 SVG 渲染，质量优于位图缩放
    if have rsvg-convert && [[ -f "$ICON_SVG" ]]; then
        rsvg-convert -w "$size" -h "$size" -o "$output" "$ICON_SVG" 2>/dev/null && return 0
    fi
    # 最终回退: 直接复制源 PNG (无缩放工具时，至少保证有图标)
    [[ -f "$ICON_PNG" ]] && { cp "$ICON_PNG" "$output" 2>/dev/null && return 0; }
    return 1
}

# =============================================================================
# Windows .ico 生成: ImageMagick 优先，缺失时用 PowerShell + System.Drawing
# 组装多尺寸 PNG-in-ICO (Vista+ 支持)，避免为构建额外安装图像工具
# =============================================================================
generate_ico() {
    local output="$1"
    [[ -f "$ICON_PNG" ]] || { print_warn "icon.png 不存在，跳过 .ico 生成"; return 1; }

    if have magick; then
        magick "$ICON_PNG" -define icon:auto-resize=256,128,64,48,32,16 "$output" 2>/dev/null \
            && [[ -s "$output" ]] && return 0
    elif have convert; then
        convert "$ICON_PNG" -define icon:auto-resize=256,128,64,48,32,16 "$output" 2>/dev/null \
            && [[ -s "$output" ]] && return 0
    fi

    local ps=""
    local cand
    for cand in powershell.exe powershell pwsh.exe pwsh; do
        if have "$cand"; then ps="$cand"; break; fi
    done
    [[ -n "$ps" ]] || { print_warn "无 ImageMagick / PowerShell，跳过 .ico 生成"; return 1; }

    print_step "使用 $ps 从 icon.png 生成 .ico ..."
    # Git Bash 下 $ICON_PNG 是 /c/... 形式的 POSIX 路径，.NET 不识别，需 cygpath 转换
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
    [[ -s "$output" ]] && return 0
    return 1
}

# =============================================================================
# macOS ad-hoc 签名 (无开发者证书；arm64 二进制必须签名才能加载，且可降低
# Gatekeeper 拦截风险)。失败仅告警，不阻断打包。
# =============================================================================
codesign_adhoc() {
    local target="$1"
    have codesign || { print_warn "codesign 不可用，跳过签名"; return 0; }
    if [[ -d "$target" ]]; then
        codesign --force --deep --sign - --timestamp=none "$target" >/dev/null 2>&1 \
            && print_ok ".app 已 ad-hoc 签名" \
            || print_warn ".app ad-hoc 签名失败 (不影响打包产物)"
    else
        codesign --force --sign - --timestamp=none "$target" >/dev/null 2>&1 \
            && print_ok "二进制已 ad-hoc 签名" \
            || print_warn "二进制 ad-hoc 签名失败 (不影响打包产物)"
    fi
    return 0
}

generate_icns() {
    local output_icns="$1"
    [[ ! -f "$ICON_PNG" ]] && { print_warn "icon.png 不存在，跳过 .icns 生成"; return 1; }

    print_step "从 icon.png 生成 .icns ..."
    local tmp_base iconset_dir
    tmp_base="$(mktemp -d)"
    iconset_dir="${tmp_base}/AppIcon.iconset"
    mkdir -p "$iconset_dir"

    # 生成所有尺寸，允许部分失败 (至少需要一个成功)
    local success_count=0 total_count=0
    local sizes=(16 32 128 256 512)
    for s in "${sizes[@]}"; do
        local s2=$((s * 2))
        total_count=$((total_count + 2))
        if generate_icon_png "$s" "${iconset_dir}/icon_${s}x${s}.png"; then
            success_count=$((success_count + 1))
        else
            print_warn "生成 ${s}x${s} PNG 失败"
        fi
        if generate_icon_png "$s2" "${iconset_dir}/icon_${s}x${s}@2x.png"; then
            success_count=$((success_count + 1))
        else
            print_warn "生成 ${s}x${s}@2x PNG 失败"
        fi
    done

    if [[ $success_count -eq 0 ]]; then
        print_fail "所有尺寸 PNG 生成均失败，无法创建 .icns"
        rm -rf "$tmp_base"
        return 1
    fi
    print_info "  PNG 生成: ${success_count}/${total_count} 成功"

    if command -v iconutil &>/dev/null; then
        if iconutil -c icns "$iconset_dir" -o "$output_icns" 2>/dev/null; then
            print_ok ".icns 生成成功: $(basename "$output_icns")"
            rm -rf "$tmp_base"
            return 0
        fi
    fi

    print_fail "iconutil 不可用或 .icns 转换失败"
    rm -rf "$tmp_base"
    return 1
}

# 生成 Info.plist 内容
generate_info_plist() {
    local target_file="$1"
    cat > "$target_file" << PLIST
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
    <string>${APP_VERSION}</string>
    <key>CFBundleShortVersionString</key>
    <string>${APP_VERSION}</string>
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

# =============================================================================
# 系统构建依赖准备 (由 build 自动调用，CI workflow 不再声明任何依赖安装步骤)
# =============================================================================
apt_refresh_once() {
    [[ "$APT_REFRESHED" == "1" ]] && return 0
    APT_REFRESHED=1
    print_step "刷新 apt 索引 ..."
    $SUDO apt-get update -qq >/dev/null 2>&1 || print_warn "apt-get update 未完全成功，继续尝试安装"
    return 0
}

# linux_pkg_has 判断包是否可由当前发行版的包管理器提供
linux_pkg_has() {
    local pkg="$1"
    if have apt-get; then
        apt_refresh_once
        apt-cache show "$pkg" >/dev/null 2>&1
    elif have dnf; then
        dnf -q list "$pkg" >/dev/null 2>&1
    elif have yum; then
        yum -q list "$pkg" >/dev/null 2>&1
    elif have pacman; then
        pacman -Si "$pkg" >/dev/null 2>&1
    elif have zypper; then
        zypper info -t package "$pkg" >/dev/null 2>&1
    else
        return 1
    fi
}

# linux_pkg_install 安装缺失的构建依赖 (按发行版分派包管理器)
linux_pkg_install() {
    [[ $# -eq 0 ]] && return 0
    print_step "安装 Linux 构建依赖: $*"
    local pm=""
    if have apt-get; then pm="apt-get"; elif have dnf; then pm="dnf"
    elif have yum; then pm="yum"; elif have pacman; then pm="pacman"
    elif have zypper; then pm="zypper"; fi

    case "$pm" in
        apt-get)
            apt_refresh_once
            $SUDO apt-get install -y --no-install-recommends "$@" || { print_warn "apt-get install 失败: $*"; return 1; } ;;
        dnf|yum)
            $SUDO "$pm" install -y "$@" || { print_warn "$pm install 失败: $*"; return 1; } ;;
        pacman)
            $SUDO pacman -S --noconfirm --needed "$@" || { print_warn "pacman -S 失败: $*"; return 1; } ;;
        zypper)
            $SUDO zypper --non-interactive install "$@" || { print_warn "zypper install 失败: $*"; return 1; } ;;
        *)
            print_warn "未识别的包管理器，请手动安装: $*"; return 1 ;;
    esac
    return 0
}

# resolve_webkit_variant 选定 WebKitGTK 版本与对应的 Wails build tag。
# Wails v2 仅用 webkit2gtk-4.0 的 pkg-config 名，要链 4.1 必须加 webkit2_41 tag
# (Ubuntu 23.10+ / Debian 12+ 已不再提供 4.0 开发包)。
WEBKIT_PKG_TO_INSTALL=""
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
    if ! have clang; then
        print_fail "缺少 clang，请安装 Xcode Command Line Tools: xcode-select --install"
        fail "deps"; return 1
    fi
    if ! xcrun --sdk macosx --show-sdk-path >/dev/null 2>&1; then
        print_fail "缺少 macOS SDK (xcrun --sdk macosx --show-sdk-path)"
        fail "deps"; return 1
    fi
    # 7z 压缩率最高 (.app 内工具体量大)；缺失时尝试 brew，仍缺失则打包回退 zip
    if ! resolve_sevenzip; then
        if have brew; then
            print_step "brew install p7zip (用于 7z 打包) ..."
            brew install p7zip >/dev/null 2>&1 || print_warn "p7zip 安装失败，将回退 zip 打包"
            resolve_sevenzip || true
        else
            print_warn "未找到 7z 且无 brew，将回退 zip 打包"
        fi
    fi
    print_ok "macOS 构建依赖就绪 (clang + macOS SDK，最低部署版本 ${MACOS_MIN_VERSION})"
    return 0
}

ensure_linux_deps() {
    init_sudo
    local need=()

    have pkg-config || need+=(pkg-config)
    have gcc      || need+=(build-essential)
    pkg-config --exists gtk+-3.0 2>/dev/null || need+=(libgtk-3-dev)

    if ! resolve_webkit_variant; then
        print_fail "未能在本系统找到或匹配到 WebKitGTK 开发包 (libwebkit2gtk-4.0-dev / 4.1-dev)"
        print_info "  请确认发行版软件源已刷新，或手动安装后重试"
        fail "deps"; return 1
    fi
    [[ -n "$WEBKIT_PKG_TO_INSTALL" ]] && need+=("$WEBKIT_PKG_TO_INSTALL")

    # 图标缩放: Linux 无 sips，用 librsvg 直接从 icon.svg 矢量渲染 (已具备缩放工具则不装)
    if ! have magick && ! have convert && ! have rsvg-convert; then
        linux_pkg_has librsvg2-bin && need+=(librsvg2-bin)
    fi

    if [[ ${#need[@]} -gt 0 ]]; then
        linux_pkg_install "${need[@]}" || { print_fail "Linux 构建依赖安装失败"; fail "deps"; return 1; }
    fi

    if ! pkg-config --exists gtk+-3.0 "$WEBKIT_PC" 2>/dev/null; then
        print_fail "pkg-config 无法解析 gtk+-3.0 / ${WEBKIT_PC}: $(pkg-config --errors --exists gtk+-3.0 "$WEBKIT_PC" 2>&1 | head -3)"
        print_info "  提示: 已用 --skip-deps 跳过安装时，需确保上述开发包已就绪"
        fail "deps"; return 1
    fi
    print_ok "Linux 构建依赖就绪 (${WEBKIT_PC} $(pkg-config --modversion "$WEBKIT_PC")${WEBKIT_TAG:+, build tag: ${WEBKIT_TAG}})"
    return 0
}

ensure_windows_deps() {
    # Wails v2 在 Windows 不需要 CGO (go-webview2 通过 winloader 动态加载
    # WebView2Loader.dll)，因此无需 MinGW-w64/gcc，避免在 runner 上安装工具链
    if ! resolve_sevenzip; then
        print_info "未找到 7z，zip 将由 PowerShell Compress-Archive 生成"
    fi
    print_ok "Windows 构建依赖就绪 (CGO_ENABLED=0，无需 C 工具链)"
    return 0
}

# step_prepare_deps 按目标平台自动准备系统级构建依赖 (幂等)
step_prepare_deps() {
    detect_platform
    if [[ "$SKIP_DEPS" == "true" ]]; then
        print_info "--skip-deps: 跳过系统构建依赖自动安装"
        return 0
    fi
    if [[ "$TARGET_GOOS" != "$HOST_GOOS" ]]; then
        print_warn "跨 OS 构建 (${HOST_GOOS} → ${TARGET_GOOS})，跳过目标 OS 的依赖安装"
        print_info "  Linux/macOS 桌面 App 需目标 OS 本机的 GTK/WebKit/Cocoa；Windows 无 CGO 需求可跨 OS 构建"
        return 0
    fi
    case "$TARGET_GOOS" in
        darwin)  ensure_macos_deps ;;
        linux)   ensure_linux_deps ;;
        windows) ensure_windows_deps ;;
        *)       print_warn "未知目标平台: ${TARGET_GOOS}，跳过依赖安装"; return 0 ;;
    esac
}

# =============================================================================
# 步骤函数
# =============================================================================
step_check_env() {
    print_banner "环境检查"
    if ! have go; then
        print_fail "未找到 go 命令"; fail "env-check"; return 1
    fi
    local go_version
    go_version=$(go version | awk '{print $3}' | sed 's/^go//' || echo "0.0.0")
    [[ -n "$GO_MIN_VERSION" ]] || GO_MIN_VERSION="$(resolve_go_min)"
    print_info "Go 版本: ${go_version}    要求: >= ${GO_MIN_VERSION} (源自 go.mod)"
    if ! version_gte "$go_version" "$GO_MIN_VERSION"; then
        print_fail "Go 版本过低"; fail "env-check"; return 1
    fi
    print_ok "Go 版本满足要求"
    detect_platform
    print_info "宿主平台: ${HOST_GOOS}/${HOST_GOARCH}    目标平台: ${TARGET_GOOS}/${TARGET_GOARCH}"
    if [[ "$TARGET_GOOS" != "$HOST_GOOS" ]]; then
        print_warn "跨 OS 构建: 产物无法在本机运行，且目标 OS 系统依赖不会被安装"
    elif is_cross_arch; then
        print_info "跨架构编译: ${HOST_GOARCH} → ${TARGET_GOARCH} (同 OS，图标等纯图像步骤照常执行)"
    fi
    print_ok "环境检查通过"
}

step_deps() {
    print_banner "Go 依赖"
    cd "${PROJECT_ROOT}"
    print_step "go mod download ..."
    if go mod download; then print_ok "Go 依赖下载成功"
    else print_fail "Go 依赖下载失败"; fail "deps"; return 1; fi
    print_step "go mod verify ..."
    if go mod verify; then print_ok "Go 依赖校验通过"
    else print_fail "Go 依赖校验失败"; fail "deps"; return 1; fi
}

step_install_bin() {
    print_banner "三方二进制工具安装"
    if [[ ! -f "$INSTALL_BIN_SCRIPT" ]]; then
        print_fail "安装脚本不存在: $INSTALL_BIN_SCRIPT"
        fail "install-bin"; return 1
    fi
    detect_platform
    # 未指定目标平台时，默认当前宿主平台 (避免 BIN_DIR 未初始化)
    local args=("--platform=${TARGET_GOOS}/${TARGET_GOARCH}")
    if [[ ${#PASSTHROUGH_ARGS[@]} -gt 0 ]]; then
        args=("${PASSTHROUGH_ARGS[@]}")
        [[ "${args[*]:-}" == *--platform* ]] || args+=("--platform=${TARGET_GOOS}/${TARGET_GOARCH}")
    fi
    print_step "委托给 install-bin.sh ${args[*]} ..."
    if bash "$INSTALL_BIN_SCRIPT" "${args[@]}"; then
        print_ok "安装完成"
    else
        print_warn "部分工具安装失败，请查看上方日志"
        fail "install-bin"; return 1
    fi
}

step_test() {
    print_banner "单元测试"
    [[ ! -d "$TESTS_DIR" ]] && { print_fail "tests/ 目录不存在"; fail "test"; return 1; }
    local tmp_cover
    tmp_cover="$(mktemp -t formatter_cover_XXXXXX.out)"
    # 无论成功失败，确保退出时清理文件
    trap "rm -f '$tmp_cover'" EXIT
    cd "${PROJECT_ROOT}"
    print_step "go test -cover ./tests/..."
    if go test -coverpkg=./src/... -coverprofile="$tmp_cover" -covermode=atomic ./tests/... 2>&1; then
        print_ok "全部测试通过"
        if [[ -s "$tmp_cover" ]]; then
            local total_coverage
            total_coverage=$(go tool cover -func="$tmp_cover" | tail -n 1 | awk '{print $NF}')
            print_info "总覆盖率: ${total_coverage:-N/A}"
        fi
    else
        print_fail "测试失败"; fail "test"; return 1
    fi
    trap - EXIT # 测试成功且执行完毕后解除trap并手动清理
    rm -f "$tmp_cover"
}

# =============================================================================
# 编译参数组装 (CGO / build tags / 平台特定 flags)
# =============================================================================
BUILD_TAGS="desktop,production"
BUILD_CGO=1
setup_build_env() {
    case "$TARGET_GOOS" in
        linux)
            # Linux 桌面构建需要 GTK3 + WebKit2GTK (Wails v2 依赖)；cgo 标志由
            # 源码内的 `#cgo pkg-config:` 指令自行解析，无需手动注入
            if [[ -z "$WEBKIT_PC" ]]; then resolve_webkit_variant || true; fi
            if [[ -z "$WEBKIT_PC" ]] || ! pkg-config --exists gtk+-3.0 "$WEBKIT_PC" 2>/dev/null; then
                print_fail "Linux 桌面构建需要 GTK3 与 ${WEBKIT_PC:-WebKit2GTK} 开发库"
                print_info "  去掉 --skip-deps 可让脚本自动安装，或手动执行:"
                print_info "    sudo apt-get install pkg-config build-essential libgtk-3-dev libwebkit2gtk-4.1-dev"
                return 1
            fi
            [[ -n "$WEBKIT_TAG" ]] && BUILD_TAGS="${BUILD_TAGS},${WEBKIT_TAG}"
            print_info "  WebKit: ${WEBKIT_PC}${WEBKIT_TAG:+ (build tag: ${WEBKIT_TAG})}"
            ;;
        darwin)
            # Wails 在 macOS 需要 Cocoa/WebKit + UniformTypeIdentifiers 框架 (CGO)
            export MACOSX_DEPLOYMENT_TARGET="$MACOS_MIN_VERSION"
            export CGO_CFLAGS="${CGO_CFLAGS:-} -mmacosx-version-min=${MACOS_MIN_VERSION}"
            export CGO_LDFLAGS="${CGO_LDFLAGS:-} -framework UniformTypeIdentifiers -mmacosx-version-min=${MACOS_MIN_VERSION}"
            # darwin 跨架构编译 (amd64↔arm64) 需指定 clang 目标 triple
            if is_cross_arch; then
                local darwin_target=""
                case "$TARGET_GOARCH" in
                    amd64) darwin_target="x86_64-apple-darwin" ;;
                    arm64) darwin_target="arm64-apple-darwin" ;;
                esac
                if [[ -n "$darwin_target" ]]; then
                    export CGO_CFLAGS="${CGO_CFLAGS} -target ${darwin_target}"
                    export CGO_LDFLAGS="${CGO_LDFLAGS} -target ${darwin_target}"
                    print_info "  交叉编译目标: ${darwin_target}"
                fi
            fi
            print_info "  最低部署版本: macOS ${MACOS_MIN_VERSION}"
            ;;
        windows)
            # go-webview2 自带 WebView2Loader，纯 Go 链接，不需要 C 工具链 (MinGW)
            BUILD_CGO=0
            ;;
    esac
    return 0
}

# =============================================================================
# 构建前序步骤: Go 依赖 / 编译 / 下载待内嵌工具
# =============================================================================
build_go_binary() {
    local ext=""
    [[ "$TARGET_GOOS" == "windows" ]] && ext=".exe"
    BIN_NAME="${PROJECT_NAME}_${TARGET_GOOS}_${TARGET_GOARCH}${ext}"
    BIN_PATH="${BUILD_DIR}/${BIN_NAME}"

    print_step "go build (${BUILD_TAGS}, CGO_ENABLED=${BUILD_CGO}) → ${BIN_NAME}"
    if CGO_ENABLED="$BUILD_CGO" GOOS="$TARGET_GOOS" GOARCH="$TARGET_GOARCH" \
        go build -trimpath -tags "${BUILD_TAGS}" -ldflags="-s -w" -o "$BIN_PATH" ./src/; then
        local size
        size=$(du -h "$BIN_PATH" | cut -f1)
        print_ok "二进制构建成功: ${BIN_NAME} (${size})"
    else
        print_fail "二进制构建失败 (${TARGET_GOOS}/${TARGET_GOARCH})"
        fail "build"; return 1
    fi
    # macOS: 二进制必须签名才能加载 (arm64 强制)，补 ad-hoc 签名
    [[ "$TARGET_GOOS" == "darwin" ]] && codesign_adhoc "$BIN_PATH"
    return 0
}

# download_bundled_tools 下载随 App 分发的独立二进制到 data/bin/{os}/{arch}/
# 必须始终执行 (而非“目录非空则跳过”)：源码树中只提交了 preset 包装脚本，
# download 类工具需在构建时拉取；install-bin.sh 自身会跳过已安装的工具 (幂等)
download_bundled_tools() {
    print_step "检查并下载随 App 分发的独立二进制工具..."
    if [[ ! -f "$INSTALL_BIN_SCRIPT" ]]; then
        print_warn "install-bin.sh 不存在，跳过工具下载"
        return 0
    fi
    # --bundled-only: 仅处理 download 类工具，跳过 source=install (如 rubocop 的 gem install，
    # 不随 App 分发，在构建机器上安装也没有意义)
    if bash "$INSTALL_BIN_SCRIPT" --platform="${TARGET_GOOS}/${TARGET_GOARCH}" \
            --bundled-only --yes 2>&1 | sed 's/^/    /'; then
        print_ok "工具安装/校验完成"
    else
        print_warn "部分工具安装失败 (可手动运行: ./scripts/install-bin.sh --proxy)"
    fi
}

# embed_tools <目标目录>: 将 data/bin/{os}/{arch} 内容 + BIN_ROOT 跨平台文件 (JAR 等) 拷入发布包
embed_tools() {
    local dest="$1"
    local has_platform=false has_cross=false
    dir_has_files "$BIN_DIR" && has_platform=true
    dir_has_files "$BIN_ROOT" && has_cross=true
    if ! $has_platform && ! $has_cross; then
        print_warn "bin/ 为空，跳过三方工具内嵌"
        return 0
    fi
    rm -rf "$dest"
    mkdir -p "$dest"
    # 平台特定工具 (darwin/amd64/ 等)
    $has_platform && { cp -R "$BIN_DIR"/* "$dest/" 2>/dev/null || true; }
    # 跨平台工具 (google-java-format.jar 等，安装时直接放在 bin/ 而非平台子目录)
    $has_cross && {
        local f base
        for f in "$BIN_ROOT"/*; do
            [[ -f "$f" ]] || continue
            base="$(basename "$f")"
            [[ -f "$dest/$base" ]] || cp "$f" "$dest/" 2>/dev/null || true
        done
    }
    local count
    count=$(find "$dest" -maxdepth 1 -mindepth 1 2>/dev/null | wc -l | tr -d ' ')
    print_ok "三方工具已内嵌 → ${dest#${PROJECT_ROOT}/}/ (${count} 个)"
    return 0
}

# =============================================================================
# 产物归档基础名 (三平台统一: Formatter-<版本>-<os>-<arch>)
# =============================================================================
artifact_base() { echo "${APP_NAME}-${APP_VERSION}-${TARGET_GOOS}-${TARGET_GOARCH}"; }

# =============================================================================
# 构建入口: 依赖准备 → 编译参数 → 编译 → 内嵌工具 → 分平台打包
# =============================================================================
step_build() {
    detect_platform
    print_banner "构建 ${APP_NAME} App (${TARGET_GOOS}/${TARGET_GOARCH})"
    cd "${PROJECT_ROOT}"
    mkdir -p "$BUILD_DIR" "$RELEASE_DIR"

    # 1. 系统构建依赖 (Linux: GTK3/WebKit2GTK; macOS: clang/7z; Windows: 无需 C 工具链)
    if ! step_prepare_deps; then return 1; fi

    # 2. 编译参数 (CGO_ENABLED / build tags / cgo flags)
    if ! setup_build_env; then fail "build"; return 1; fi

    # 3. Go 依赖
    print_step "go mod download ..."
    if go mod download; then print_ok "Go 依赖就绪"
    else print_fail "Go 依赖下载失败"; fail "build"; return 1; fi

    # 4. 编译二进制
    if ! build_go_binary; then return 1; fi

    # 5. 三方工具: 下载 download 类 → 内嵌到 build/bin (跨平台文件由 embed_tools 统一处理)
    download_bundled_tools
    embed_tools "${BUILD_DIR}/bin"

    # 6. 分平台打包 (三平台均产出可分发的 App 包)
    case "$TARGET_GOOS" in
        darwin)  package_macos ;;
        linux)   package_linux ;;
        windows) package_windows ;;
        *)       print_fail "未知目标平台: ${TARGET_GOOS}"; fail "package"; return 1 ;;
    esac
}

# =============================================================================
# macOS: .app bundle + ad-hoc 签名 + 7z (回退 zip)
# =============================================================================
package_macos() {
    print_step "打包 macOS .app ..."
    local app_dir="${BUILD_DIR}/${APP_NAME}.app"
    local contents_dir="${app_dir}/Contents"
    local macos_dir="${contents_dir}/MacOS"
    local resources_dir="${contents_dir}/Resources"

    rm -rf "$app_dir"
    mkdir -p "$macos_dir" "$resources_dir"

    cp "$BIN_PATH" "${macos_dir}/${APP_NAME}"
    chmod +x "${macos_dir}/${APP_NAME}"

    if [[ -f "${DATA_DIR}/config.json" ]]; then
        cp "${DATA_DIR}/config.json" "${macos_dir}/config.json"
        print_ok "默认配置已嵌入 MacOS/config.json"
    fi

    # 图标生成是纯图像处理 (PNG→多尺寸→icns)，与目标架构无关，交叉编译时同样执行
    if generate_icns "${resources_dir}/AppIcon.icns"; then
        print_ok "图标已嵌入 Resources/AppIcon.icns"
    else
        print_warn "图标生成失败，跳过"
    fi

    embed_tools "${resources_dir}/bin"

    generate_info_plist "${contents_dir}/Info.plist"
    print_ok ".app 打包完成: ${app_dir#${PROJECT_ROOT}/} (最低系统版本 ${MACOS_MIN_VERSION})"

    # 整包 ad-hoc 签名: 内嵌二进制路径变化后重新签名，降低 Gatekeeper 拦截概率
    codesign_adhoc "$app_dir"

    rm -rf "${RELEASE_DIR}/${APP_NAME}.app"
    cp -R "$app_dir" "$RELEASE_DIR/"
    print_ok "已复制到 release/: ${APP_NAME}.app"

    archive_macos "$app_dir" || return 1
    print_usage_hint macos
}

archive_macos() {
    local app_dir="$1"
    local base archive out
    base="$(artifact_base)"

    # 清理隔离属性，避免 7z/zip 打包进 com.apple.quarantine 等扩展属性
    xattr -cr "$app_dir" 2>/dev/null || true

    if resolve_sevenzip; then
        archive="${RELEASE_DIR}/${base}.7z"
        print_step "打包 7z (最大压缩): ${base}.7z"
        rm -f "$archive"
        if (cd "$(dirname "$app_dir")" && \
                "$SEVENZIP" a -m0=lzma2 -mx=9 -mmt=on "-x!*.DS_Store" "$archive" "$APP_NAME.app" \
                >/dev/null 2>&1) && [[ -s "$archive" ]]; then
            print_ok "7z 打包成功: $(basename "$archive") ($(du -h "$archive" | cut -f1))"
            return 0
        fi
        print_warn "7z 打包失败，回退 zip"
        rm -f "$archive"
    else
        print_warn "未找到 7z (brew install p7zip)，回退 zip 打包"
    fi

    archive="${RELEASE_DIR}/${base}.zip"
    print_step "打包 zip: ${base}.zip"
    rm -f "$archive"
    # macOS 自带 ditto 能保留 .app 的扩展属性与目录结构
    if have ditto; then
        ditto -c -k --keepParent "$app_dir" "$archive" >/dev/null 2>&1 || true
    fi
    [[ -s "$archive" ]] || archive_zip "$archive" "$app_dir" || {
        print_fail "macOS 归档失败"; fail "package"; return 1; }
    print_ok "zip 打包成功: $(basename "$archive") ($(du -h "$archive" | cut -f1))"
    return 0
}

# =============================================================================
# Linux: bin/ + share/{applications,icons} + tools/ + install.sh → tar.gz
# =============================================================================
package_linux() {
    print_step "打包 Linux 桌面应用 ..."
    local stage_root="${BUILD_DIR}/linux_stage"
    local stage_dir="${stage_root}/${APP_NAME}"
    rm -rf "$stage_root"
    mkdir -p "${stage_dir}/bin" \
             "${stage_dir}/share/applications" \
             "${stage_dir}/share/icons/hicolor/256x256/apps"

    cp "$BIN_PATH" "${stage_dir}/bin/${PROJECT_NAME}"
    chmod +x "${stage_dir}/bin/${PROJECT_NAME}"

    [[ -f "${DATA_DIR}/config.json" ]] && \
        cp "${DATA_DIR}/config.json" "${stage_dir}/bin/config.json"

    if [[ -f "$LINUX_DESKTOP" ]]; then
        cp "$LINUX_DESKTOP" "${stage_dir}/share/applications/"
        print_ok ".desktop 文件已嵌入"
    else
        print_warn "缺少 ${LINUX_DESKTOP#${PROJECT_ROOT}/}，跳过 .desktop"
    fi

    local icon_png="${stage_dir}/share/icons/hicolor/256x256/apps/${PROJECT_NAME}.png"
    if generate_icon_png 256 "$icon_png"; then
        print_ok "图标已嵌入 (256x256)"
    else
        print_warn "图标生成失败，跳过"
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

    local base archive
    base="$(artifact_base)"
    archive="${RELEASE_DIR}/${base}.tar.gz"
    print_step "打包 tar.gz: ${base}.tar.gz"
    rm -f "$archive"
    if tar -czf "$archive" -C "$stage_root" "$APP_NAME" 2>/dev/null && [[ -s "$archive" ]]; then
        print_ok "tar.gz 打包成功: $(basename "$archive") ($(du -h "$archive" | cut -f1))"
    else
        print_fail "tar.gz 打包失败"; fail "package"; rm -rf "$stage_root"; return 1
    fi
    rm -rf "$stage_root"

    print_usage_hint linux
}

# =============================================================================
# Windows: Formatter.exe + config.json + icon.ico + tools/ → zip
# =============================================================================
package_windows() {
    print_step "打包 Windows 桌面应用 ..."
    local stage_root="${BUILD_DIR}/win_stage"
    local stage_dir="${stage_root}/${APP_NAME}"
    rm -rf "$stage_root"
    mkdir -p "$stage_dir"

    cp "$BIN_PATH" "${stage_dir}/${APP_NAME}.exe"

    [[ -f "${DATA_DIR}/config.json" ]] && \
        cp "${DATA_DIR}/config.json" "${stage_dir}/config.json"

    if generate_ico "${stage_dir}/icon.ico"; then
        print_ok "图标已嵌入 (icon.ico)"
    else
        rm -f "${stage_dir}/icon.ico"
    fi

    embed_tools "${stage_dir}/tools"

    local base archive
    base="$(artifact_base)"
    archive="${RELEASE_DIR}/${base}.zip"
    print_step "打包 zip: ${base}.zip"
    if archive_zip "$archive" "$stage_dir"; then
        print_ok "zip 打包成功: $(basename "$archive") ($(du -h "$archive" | cut -f1))"
    else
        print_fail "zip 打包失败"; fail "package"; rm -rf "$stage_root"; return 1
    fi
    rm -rf "$stage_root"

    print_usage_hint windows
}

# =============================================================================
# 使用方式提示 (路径相对解压目录，Linux/Windows 产物解压后才存在)
# =============================================================================
print_usage_hint() {
    local platform="$1"
    printf "\n  %b使用方式:%b\n" "$CYAN" "$NC"
    case "$platform" in
        macos)
            printf "    %b桌面应用:%b open %s/%s.app\n" "$BOLD" "$NC" "$RELEASE_DIR" "$APP_NAME"
            printf "    %bCLI 调用:%b %s/${APP_NAME} format main.go\n" "$BOLD" "$NC" \
                "${RELEASE_DIR}/${APP_NAME}.app/Contents/MacOS"
            printf "    %bWeb UI:%b   %s/${APP_NAME} serve\n" "$BOLD" "$NC" \
                "${RELEASE_DIR}/${APP_NAME}.app/Contents/MacOS"
            ;;
        linux)
            printf "    %b桌面应用:%b 解压后运行 %s/install.sh，或直接执行 %s/bin/%s\n" \
                "$BOLD" "$NC" "$APP_NAME" "$APP_NAME" "$PROJECT_NAME"
            printf "    %bCLI 调用:%b %s/bin/%s format main.go\n" "$BOLD" "$NC" "$APP_NAME" "$PROJECT_NAME"
            printf "    %bWeb UI:%b   %s/bin/%s serve\n" "$BOLD" "$NC" "$APP_NAME" "$PROJECT_NAME"
            ;;
        windows)
            printf "    %b桌面应用:%b 解压后运行 %s/%s.exe\n" "$BOLD" "$NC" "$APP_NAME" "$APP_NAME"
            printf "    %bCLI 调用:%b %s/%s.exe format main.go\n" "$BOLD" "$NC" "$APP_NAME" "$APP_NAME"
            printf "    %bWeb UI:%b   %s/%s.exe serve\n" "$BOLD" "$NC" "$APP_NAME" "$APP_NAME"
            ;;
    esac
    printf "\n"
}

step_clean() {
    print_banner "清理构建产物"
    local cleaned=0
    for dir in "$BUILD_DIR" "$RELEASE_DIR" "${PROJECT_ROOT}/coverage" "${PROJECT_ROOT}/.downloads"; do
        if [[ -d "$dir" ]]; then
            rm -rf "$dir"
            print_ok "删除 $(basename "$dir")/"
            cleaned=$((cleaned + 1))
        fi
    done
    find "${PROJECT_ROOT}" -maxdepth 2 \( -name "*.test" -o -name "*.out" \) -delete 2>/dev/null || true
    if [[ $cleaned -gt 0 ]]; then
        print_ok "清理完成，共 ${cleaned} 项"
    else
        print_ok "项目已干净"
    fi
}

step_report() {
    local total=${#FAILED_STEPS[@]} exit_code=0
    printf "\n%b═══════════════════════════════════════════════════════════════%b\n" "$CYAN" "$NC"
    if [[ $total -eq 0 ]]; then
        printf "  %b%b✓ 所有步骤执行成功%b\n" "$GREEN" "$BOLD" "$NC"
    else
        exit_code=1
        printf "  %b%b✗ %d 个步骤失败:%b\n" "$RED" "$BOLD" "$total" "$NC"
        for s in "${FAILED_STEPS[@]}"; do 
            printf "    %b• %s%b\n" "$RED" "$s" "$NC"
        done
    fi
    printf "\n  %b构建产物 (build/):%b\n" "$CYAN" "$NC"
    if [[ -d "$BUILD_DIR" ]]; then
        while IFS= read -r item; do
            if [[ -d "$item" ]]; then
                local sz; sz=$(du -sh "$item" 2>/dev/null | cut -f1)
                printf "    %s/ (%s)\n" "$(basename "$item")" "$sz"
            else
                local sz; sz=$(du -h "$item" 2>/dev/null | cut -f1)
                printf "    %s (%s)\n" "$(basename "$item")" "$sz"
            fi
        done < <(find "$BUILD_DIR" -maxdepth 1 -mindepth 1 2>/dev/null)
    else
        printf "    (空)\n"
    fi
    printf "  %b发布产物 (release/):%b\n" "$CYAN" "$NC"
    if [[ -d "$RELEASE_DIR" ]]; then
        while IFS= read -r item; do
            if [[ -d "$item" ]]; then
                local sz; sz=$(du -sh "$item" 2>/dev/null | cut -f1)
                printf "    %s/ (%s)\n" "$(basename "$item")" "$sz"
            else
                local sz; sz=$(du -h "$item" 2>/dev/null | cut -f1)
                printf "    %s (%s)\n" "$(basename "$item")" "$sz"
            fi
        done < <(find "$RELEASE_DIR" -maxdepth 1 -mindepth 1 2>/dev/null)
    else
        printf "    (空)\n"
    fi
    printf "%b═══════════════════════════════════════════════════════════════%b\n\n" "$CYAN" "$NC"
    return $exit_code
}

# =============================================================================
# 帮助与菜单
# =============================================================================
show_help() {
    local mode="${1:-cli}"
    detect_platform
    local cur_platform="${TARGET_GOOS}/${TARGET_GOARCH}"

    if [[ "$mode" == "menu" ]]; then
        cat <<MENU

${CYAN}╔══════════════════════════════════════════════════════════════╗
║              Formatter 工具脚本 (v${APP_VERSION})                         ║
╚══════════════════════════════════════════════════════════════╝${NC}

  当前平台:  ${cur_platform}
  Go 版本:   $(go version 2>/dev/null | awk '{print $3}')

  ${BOLD}请选择操作 (数字或英文首字母):${NC}

    1) build / b        构建当前平台 App (自动安装构建依赖 + 打包)
    2) install-bin / i  安装三方二进制工具到 bin/
    3) test / t         运行单元测试
    4) clean / c        清理构建产物
    5) run / r          编译并运行 App (构建后启动桌面应用) [默认]
    6) server / s       编译并运行 Web Server (serve 命令，前台运行)
    0) exit / q         退出

MENU
        printf "  %b请输入选项:%b " "$CYAN" "$NC"
    else
        cat <<CLI
${BOLD}用法:${NC} $0 [command] [options]

${BOLD}命令:${NC}
  build / b        构建桌面 App (自动准备系统依赖 → 编译 → 内嵌工具 → 打包)
  install-bin / i  下载三方二进制工具到 data/bin/ (委托给 install-bin.sh)
  test / t         运行单元测试
  clean / c        清理构建产物
  run / r          编译并运行 App (构建后启动桌面应用)
  server / s       编译并运行 Web Server (serve 命令，前台运行)

${BOLD}选项:${NC}
  --platform=OS/ARCH   目标平台 (darwin/amd64 darwin/arm64 linux/amd64 windows/amd64)
  --skip-deps          跳过系统构建依赖自动安装 (环境已就绪时用)
  --tool=<name>        install-bin 仅下载指定工具
  --all                install-bin 含运行时环境检测
  --addr=<addr>        server 监听地址 (如 :8080)
  --no-open            server 不自动打开浏览器
  --browser            server 使用默认浏览器打开

${BOLD}说明:${NC}
  - 菜单支持数字 (1-6) 或英文首字母 (b/i/t/c/r/s) 匹配操作
  - 所有构建逻辑 (依赖安装 / CGO / 图标 / 打包) 均在本脚本实现，
    .github/workflows/release.yml 仅做 runner 引导与产物上传
  - macOS: .app bundle (icns + 内嵌工具) + ad-hoc 签名 → .7z (无 7z 时回退 .zip)
    CGO_ENABLED=1 + -tags desktop,production；跨架构自动加 clang -target
  - Linux: tar.gz (bin/ + .desktop + 图标 + tools/ + install.sh)
    自动 apt/dnf/pacman/zypper 安装 GTK3 + WebKit2GTK (4.0/4.1 自动选择 build tag)
  - Windows: zip (Formatter/ + Formatter.exe + config.json + icon.ico + tools/)
    CGO_ENABLED=0 (go-webview2 自带 WebView2Loader)，无需 MinGW/gcc
  - 三平台产物命名统一: Formatter-<版本>-<os>-<arch>.<ext>，输出到 release/
  - install-bin 命令委托给 scripts/install-bin.sh 脚本执行
  - server 以前台方式运行，按 Ctrl+C 停止

${BOLD}示例:${NC}
  $0                    # 交互式菜单
  $0 build              # 构建当前平台 App
  $0 b                  # 构建 App (简写)
  $0 b --platform=darwin/arm64   # 指定目标平台构建
  $0 b --skip-deps      # 依赖已就绪，跳过自动安装
  $0 run                # 编译并运行 App
  $0 server             # 编译并运行 Web Server
  $0 s --addr=:8080     # 指定端口运行 Server (简写)
  $0 install-bin        # 安装全部独立二进制
  $0 i --tool=stylua    # 仅安装 stylua (简写)
  $0 i --all            # 含运行时环境检测
CLI
    fi
}

prompt_platform() {
    local default_p="${TARGET_GOOS:-$(go env GOOS)}/${TARGET_GOARCH:-$(go env GOARCH)}"
    printf "\n  %b目标平台%b [默认: %s]: " "$CYAN" "$NC" "$default_p"
    read -r input
    if [[ -n "$input" ]]; then
        TARGET_GOOS="${input%%/*}"
        TARGET_GOARCH="${input##*/}"
    else
        TARGET_GOOS="${default_p%%/*}"
        TARGET_GOARCH="${default_p##*/}"
    fi
}

# =============================================================================
# 菜单选项解析 (支持数字或英文首字母)
# =============================================================================
resolve_menu_choice() {
    local choice="$1"
    case "$(echo "$choice" | tr '[:upper:]' '[:lower:]')" in
        1|b|build)       echo "build" ;;
        2|i|install-bin) echo "install-bin" ;;
        3|t|test)        echo "test" ;;
        4|c|clean)       echo "clean" ;;
        5|r|run)         echo "run" ;;
        6|s|server)      echo "server" ;;
        0|q|exit|quit)   echo "exit" ;;
        *)               echo "" ;;
    esac
}

# =============================================================================
# 编译并运行 App
# =============================================================================
step_run() {
    local interactive="${1:-false}"
    print_banner "编译并运行 App"
    [[ "$interactive" == "true" && -z "$TARGET_GOOS" ]] && prompt_platform

    if ! step_check_env; then step_report; return 1; fi
    if ! step_build; then step_report; return 1; fi

    # 启动 App (macOS 优先用 .app bundle，其他平台直接跑二进制)
    local app_path="${RELEASE_DIR}/${APP_NAME}.app"
    if [[ "$TARGET_GOOS" == "darwin" && -d "$app_path" ]]; then
        print_step "启动 macOS App: ${app_path}"
        if open "$app_path" 2>/dev/null; then
            print_ok "App 已启动"
        else
            print_fail "App 启动失败"
            fail "run"
        fi
    else
        local bin_path="${BIN_PATH:-${BUILD_DIR}/${PROJECT_NAME}_${TARGET_GOOS}_${TARGET_GOARCH}}"
        [[ "$TARGET_GOOS" == "windows" ]] && bin_path="${bin_path}.exe"
        if [[ -f "$bin_path" ]]; then
            print_step "启动二进制: ${bin_path}"
            chmod +x "$bin_path" 2>/dev/null || true
            "$bin_path" >/dev/null 2>&1 &
            local pid=$!
            if kill -0 "$pid" 2>/dev/null; then
                print_ok "App 已启动 (PID: ${pid})"
            else
                print_fail "二进制启动失败"
                fail "run"
            fi
        else
            print_fail "未找到可执行文件: ${bin_path}"
            fail "run"
        fi
    fi
    step_report
}

# =============================================================================
# 编译并运行 Web Server (serve 命令)
# =============================================================================
step_server() {
    local interactive="${1:-false}"
    print_banner "编译并运行 Web Server"
    [[ "$interactive" == "true" && -z "$TARGET_GOOS" ]] && prompt_platform

    if ! step_check_env; then step_report; return 1; fi
    if ! step_build; then step_report; return 1; fi

    # 使用原始二进制 (非 .app)，以 serve 命令启动 Web Server
    local bin_path="${BIN_PATH:-${BUILD_DIR}/${PROJECT_NAME}_${TARGET_GOOS}_${TARGET_GOARCH}}"
    [[ "$TARGET_GOOS" == "windows" ]] && bin_path="${bin_path}.exe"
    if [[ ! -f "$bin_path" ]]; then
        print_fail "未找到可执行文件: ${bin_path}"
        fail "server"; step_report; return 1
    fi

    chmod +x "$bin_path" 2>/dev/null || true
    # 前台运行 server，用户可 Ctrl+C 退出
    # PASSTHROUGH_ARGS 可包含 --addr / --no-open / --browser 等参数
    print_step "启动 Web Server: ${bin_path} serve ${PASSTHROUGH_ARGS[*]:-}"
    printf "  %b按 Ctrl+C 停止服务器%b\n\n" "$DIM" "$NC"
    # bash 3.2 + set -u 下空数组展开会报 unbound variable，先判长度
    if [[ ${#PASSTHROUGH_ARGS[@]} -gt 0 ]]; then
        "$bin_path" serve "${PASSTHROUGH_ARGS[@]}"
    else
        "$bin_path" serve
    fi
}

# =============================================================================
# 命令调度
# =============================================================================
dispatch_cmd() {
    local cmd="$1"
    local interactive="${2:-false}"

    case "$cmd" in
        build)
            [[ "$interactive" == "true" && -z "$TARGET_GOOS" ]] && prompt_platform
            step_check_env && step_build; step_report ;;
        install-bin)
            [[ "$interactive" == "true" ]] && PASSTHROUGH_ARGS+=("--interactive")
            step_check_env >/dev/null 2>&1 || true
            step_install_bin; step_report ;;
        test)
            step_check_env && step_deps && step_test; step_report ;;
        clean)
            step_clean; step_report ;;
        run)
            step_run "$interactive" ;;
        server)
            step_server "$interactive" ;;
        help|-h|--help)
            show_help "cli" ;;
        *)
            printf "%b错误: 未知命令 '%s'%b\n" "$RED" "$cmd" "$NC"; exit 1 ;;
    esac
}

# =============================================================================
# 主入口
# =============================================================================
main() {
    local cmd=""
    for a in "$@"; do
        case "$a" in
            --platform=*)
                local p="${a#*=}"
                TARGET_GOOS="${p%%/*}"; TARGET_GOARCH="${p##*/}" ;;
            --skip-deps)
                SKIP_DEPS=true ;;
            --tool=*)
                PASSTHROUGH_ARGS+=("$a") ;;
            --all)
                PASSTHROUGH_ARGS+=("$a") ;;
            --addr=*)
                PASSTHROUGH_ARGS+=("$a") ;;
            --no-open)
                PASSTHROUGH_ARGS+=("$a") ;;
            --browser)
                PASSTHROUGH_ARGS+=("$a") ;;
            -h|--help|help)
                cmd="help" ;;
            build|b)
                cmd="build" ;;
            install-bin|i)
                cmd="install-bin" ;;
            test|t)
                cmd="test" ;;
            clean|c)
                cmd="clean" ;;
            run|r)
                cmd="run" ;;
            server|s)
                cmd="server" ;;
            "")
                ;;
            *)
                printf "%b未知参数: %s%b\n" "$RED" "$a" "$NC"
                echo "运行 '$0 help' 查看帮助"
                exit 1 ;;
        esac
    done

    if [[ -z "$cmd" ]]; then
        while true; do
            show_help "menu"
            local choice
            read -r choice
            # 空输入默认为 run (编译并运行 App)
            [[ -z "$choice" ]] && choice="5"
            local resolved
            resolved="$(resolve_menu_choice "$choice")"
            case "$resolved" in
                build)       dispatch_cmd "build" "true" ;;
                install-bin) dispatch_cmd "install-bin" "true" ;;
                test)        dispatch_cmd "test" "true" ;;
                clean)       dispatch_cmd "clean" "true" ;;
                run)         dispatch_cmd "run" "true" ;;
                server)      dispatch_cmd "server" "true" ;;
                exit)        printf "%b再见%b\n" "$CYAN" "$NC"; exit 0 ;;
                *)           printf "%b无效选项: %s%b\n" "$RED" "$choice" "$NC"; sleep 1 ;;
            esac
            echo ""
            break
        done
    else
        dispatch_cmd "$cmd" "false"
    fi
}

main "$@"