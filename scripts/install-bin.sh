#!/usr/bin/env bash
set -euo pipefail

# =============================================================================
# install-bin.sh - 配置驱动的三方二进制工具安装脚本
#
# 从 data/config.json 读取工具定义，自动:
#   1. 下载独立二进制工具 (source=download) 到 data/bin/
#   2. 校验预置工具 (source=preset) 是否存在
#   3. 检测运行时环境 (node/python/java/ruby)
#   4. 交互式询问是否安装缺失运行时 (使用 config.json 中的 install_cmds)
#
# 用法:
#   ./install-bin.sh                          # 安装全部独立二进制 (默认)
#   ./install-bin.sh --tool=stylua            # 仅安装指定工具
#   ./install-bin.sh --tool=shfmt,biome       # 安装多个指定工具 (逗号分隔)
#   ./install-bin.sh --all                    # 含运行时环境检测
#   ./install-bin.sh --platform=darwin/arm64  # 指定目标平台
#   ./install-bin.sh --interactive            # 交互式选择
#   ./install-bin.sh --yes                    # 非交互模式，所有询问自动确认
#
# 安装目录: <项目根>/data/bin/
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
CONFIG_FILE="${PROJECT_ROOT}/data/config.json"
DATA_DIR="${PROJECT_ROOT}/data"
BIN_ROOT="${DATA_DIR}/bin"          # 跨平台文件目录 (JAR 等)
# BIN_DIR 在 detect_platform() 后根据平台设置 (见下方)
DOWNLOADS_DIR="${PROJECT_ROOT}/.downloads"

# ---- GitHub 代理配置 ----
# 通过环境变量 USE_GH_PROXY=true 启用代理，或设置 GH_PROXY_URL 自定义代理地址
# 示例: USE_GH_PROXY=true ./install-bin.sh
#       GH_PROXY_URL=https://gh-proxy.com ./install-bin.sh
# 默认 auto: 未显式设置时，若直连 GitHub 失败则自动重试代理 (AUTO_PROXY_RETRY=true)
GH_PROXY_URL="${GH_PROXY_URL:-https://gh-proxy.com}"
USE_GH_PROXY="${USE_GH_PROXY:-false}"
# 自动代理重试: 直连失败时尝试代理 (仅在 USE_GH_PROXY 未显式开启时生效)
AUTO_PROXY_RETRY="${AUTO_PROXY_RETRY:-true}"

# 对 GitHub URL 应用代理前缀
apply_proxy() {
    local url="$1"
    if [[ "$USE_GH_PROXY" == "true" ]] && [[ "$url" == https://github.com/* ]]; then
        echo "${GH_PROXY_URL%/}/${url}"
    else
        echo "$url"
    fi
}

# is_github_url 判断是否为 GitHub 下载链接 (用于自动代理重试)
is_github_url() {
    [[ "$1" == https://github.com/* ]]
}

# ---- 颜色 (ANSI-C quotes，与 run_tools.sh 一致) ----
RED=$'\033[0;31m'
GREEN=$'\033[0;32m'
YELLOW=$'\033[1;33m'
CYAN=$'\033[0;36m'
BOLD=$'\033[1m'
DIM=$'\033[2m'
NC=$'\033[0m'

# ---- 全局状态 ----
TARGET_GOOS=""
TARGET_GOARCH=""
INSTALL_TOOL=""
INSTALL_ALL=false
INTERACTIVE=false
AUTO_YES=false
CHECK_ONLY=false
SELECTED_TOOLS=()
FAILED_TOOLS=()
SKIPPED_TOOLS=()
INSTALLED_TOOLS=()
ALREADY_INSTALLED=()
JQ_BIN=""

# ---- 基础输出函数 ----
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
print_skip() { printf "%b[SKIP]%b %s\n" "$YELLOW" "$NC" "$1"; }

# ---- 工具函数 ----
fail_tool() { FAILED_TOOLS+=("$1"); print_fail "$1: $2"; }
skip_tool() { SKIPPED_TOOLS+=("$1"); print_skip "$1: $2"; }
ok_tool()   { INSTALLED_TOOLS+=("$1"); print_ok "$1: $2"; }
already_tool() { ALREADY_INSTALLED+=("$1"); }

# 确认提示 (非交互模式或 --yes 时返回 0)
confirm() {
    local prompt="$1" default="${2:-y}"
    if $AUTO_YES; then return 0; fi
    if ! $INTERACTIVE && [[ ! -t 0 ]]; then return 0; fi
    local yn
    printf "  %b%s%b [%s]: " "$CYAN" "$prompt" "$NC" "$default"
    read -r yn
    yn="${yn:-$default}"
    [[ "${yn:0:1}" =~ ^[Yy] ]]
}

# =============================================================================
# 平台检测
# =============================================================================
detect_platform() {
    if [[ -z "$TARGET_GOOS" ]]; then
        case "$(uname -s)" in
            Darwin) TARGET_GOOS="darwin" ;;
            Linux)  TARGET_GOOS="linux" ;;
            MINGW*|MSYS*|CYGWIN*) TARGET_GOOS="windows" ;;
            *) print_fail "不支持的操作系统: $(uname -s)"; exit 1 ;;
        esac
    fi
    if [[ -z "$TARGET_GOARCH" ]]; then
        case "$(uname -m)" in
            x86_64|amd64) TARGET_GOARCH="amd64" ;;
            arm64|aarch64) TARGET_GOARCH="arm64" ;;
            armv7l) TARGET_GOARCH="arm" ;;
            i386|i686) TARGET_GOARCH="386" ;;
            *) print_fail "不支持的架构: $(uname -m)"; exit 1 ;;
        esac
    fi
    # 平台特定的二进制目录: data/bin/{os}/{arch}/
    BIN_DIR="${BIN_ROOT}/${TARGET_GOOS}/${TARGET_GOARCH}"
}

# =============================================================================
# JSON 解析工具引导 (jq)
# =============================================================================
bootstrap_jq() {
    # 1. 系统 PATH 中查找
    if command -v jq &>/dev/null; then
        JQ_BIN="jq"
        return 0
    fi
    # 2. data/bin/ 中查找 (之前安装过)
    local ext_suf
    ext_suf="$(exe_suffix)"
    if [[ -x "${BIN_DIR}/jq${ext_suf}" ]]; then
        JQ_BIN="${BIN_DIR}/jq${ext_suf}"
        return 0
    fi
    # 3. 尝试下载 jq 到 BIN_DIR
    print_step "引导下载 jq (JSON 解析器)..."
    mkdir -p "$BIN_DIR" "$DOWNLOADS_DIR"
    local jq_version="1.7.1"
    local jq_os jq_arch jq_url jq_path="${BIN_DIR}/jq${ext_suf}"

    case "$TARGET_GOOS" in
        darwin)  jq_os="macos" ;;
        linux)   jq_os="linux" ;;
        windows) jq_os="windows" ;;
        *) print_warn "无法为 $TARGET_GOOS 引导 jq，请手动安装 jq"; return 1 ;;
    esac
    case "$TARGET_GOARCH" in
        amd64) jq_arch="amd64" ;;
        arm64)
            if [[ "$TARGET_GOOS" == "darwin" ]]; then jq_arch="arm64"
            else print_warn "linux/windows arm64 无官方 jq 构建，请手动安装 jq"; return 1; fi ;;
        *) print_warn "无法为 $TARGET_GOARCH 引导 jq，请手动安装 jq"; return 1 ;;
    esac

    local dl_ext=""
    [[ "$TARGET_GOOS" == "windows" ]] && dl_ext=".exe"
    jq_url="https://github.com/jqlang/jq/releases/download/jq-${jq_version}/jq-${jq_os}-${jq_arch}${dl_ext}"
    jq_url="$(apply_proxy "$jq_url")"

    if download_url "$jq_url" "$jq_path"; then
        chmod +x "$jq_path" 2>/dev/null || true
        JQ_BIN="$jq_path"
        print_ok "jq 引导成功: ${JQ_BIN}"
        return 0
    fi
    print_warn "jq 下载失败，请手动安装 jq"
    return 1
}

# 确保 jq 可用
ensure_jq() {
    [[ -n "$JQ_BIN" ]] && return 0
    bootstrap_jq
}

# jq 查询快捷方式
jq_get() { "$JQ_BIN" -r "$1" "$CONFIG_FILE" 2>/dev/null; }

# =============================================================================
# 下载工具函数
# 支持:
#   - curl/wget 自动选择
#   - 单文件 max-time 防止大文件卡死 (默认 300s)
#   - 失败重试 (max_retries=3)
#   - GitHub 直连失败时自动尝试代理 (AUTO_PROXY_RETRY=true)
# =============================================================================
# _do_download 实际执行一次下载 (无重试)，返回 0 成功
_do_download() {
    local url="$1" dest="$2"
    if command -v curl &>/dev/null; then
        # -f: HTTP 错误返回非 0; -L: 跟随重定向; --connect-timeout: 连接超时; --max-time: 总超时
        curl -fL --connect-timeout 20 --max-time 600 --retry 1 \
            -o "$dest" "$url" 2>&1
    elif command -v wget &>/dev/null; then
        wget -q --timeout=20 -O "$dest" "$url" 2>&1
    else
        print_fail "  未找到 curl 或 wget"
        return 127
    fi
}

download_url() {
    local url="$1" dest="$2"
    local max_retries=3 attempt=1
    mkdir -p "$(dirname "$dest")"

    # 下载策略:
    #   USE_GH_PROXY=true  → 直接用代理下载 (仅 GitHub URL)
    #   AUTO_PROXY_RETRY=true 且为 GitHub URL → 先直连，失败后用代理重试
    #   其他情况 → 直连
    local use_proxy_now=false
    if [[ "$USE_GH_PROXY" == "true" ]] && is_github_url "$url"; then
        use_proxy_now=true
    fi

    while (( attempt <= max_retries )); do
        if (( attempt > 1 )); then
            print_info "  重试 ${attempt}/${max_retries}..."
            sleep 2
        fi

        local dl_url="$url"
        $use_proxy_now && dl_url="$(apply_proxy "$url")"

        if _do_download "$dl_url" "$dest"; then
            # 校验下载文件非空
            if [[ -s "$dest" ]]; then
                return 0
            fi
            print_warn "  下载完成但文件为空，可能 URL 失效"
            rm -f "$dest"
        else
            # 清理可能的残留部分文件
            rm -f "$dest" 2>/dev/null || true
        fi

        # GitHub 直连失败 → 自动切换代理重试一次
        if ! $use_proxy_now && [[ "$AUTO_PROXY_RETRY" == "true" ]] && is_github_url "$url"; then
            print_warn "  直连 GitHub 失败，切换代理重试..."
            use_proxy_now=true
            attempt=$((attempt - 1))  # 不消耗重试次数，仅切换源
            # 防止无限循环: AUTO_PROXY_RETRY 仅触发一次
            AUTO_PROXY_RETRY=false
            continue
        fi

        attempt=$((attempt + 1))
    done
    return 1
}

# =============================================================================
# Windows 扩展名处理
# =============================================================================
exe_suffix() {
    [[ "$TARGET_GOOS" == "windows" ]] && echo ".exe" || echo ""
}

# =============================================================================
# URL 查找: 从 urls 映射中获取当前平台的下载 URL (与 Go 端 PlatformURL 逻辑对齐)
#
# 查找优先级:
#   1. 精确匹配 "os/arch" (如 "darwin/arm64")
#   2. 通配符匹配 "os/*" (架构无关的通用二进制)
#   3. 完全通配符 "*" (跨所有平台的资源，如 all-deps JAR)
#   4. 返回空字符串 (该平台不支持)
# =============================================================================
lookup_url() {
    local idx="$1" version="$2"
    local platform_key="${TARGET_GOOS}/${TARGET_GOARCH}"
    local wildcard_key="${TARGET_GOOS}/*"

    # 1. 精确匹配
    local url
    url="$("$JQ_BIN" -r --arg k "$platform_key" \
        ".binary.tools[$idx].urls[\$k] // empty" "$CONFIG_FILE")"
    if [[ -n "$url" ]]; then
        echo "${url//\{version\}/$version}"
        return 0
    fi

    # 2. 通配符 "os/*"
    url="$("$JQ_BIN" -r --arg k "$wildcard_key" \
        ".binary.tools[$idx].urls[\$k] // empty" "$CONFIG_FILE")"
    if [[ -n "$url" ]]; then
        echo "${url//\{version\}/$version}"
        return 0
    fi

    # 3. 完全通配符 "*" (跨平台资源，如 JAR 包)
    url="$("$JQ_BIN" -r '.binary.tools['"$idx"'].urls["*"] // empty' "$CONFIG_FILE")"
    if [[ -n "$url" ]]; then
        echo "${url//\{version\}/$version}"
        return 0
    fi

    return 1
}

# =============================================================================
# 归档解压
# =============================================================================
extract_archive() {
    local archive="$1" archive_type="$2" dest_dir="$3"
    mkdir -p "$dest_dir"
    case "$archive_type" in
        raw)
            return 0
            ;;
        zip)
            if command -v unzip &>/dev/null; then
                unzip -qo "$archive" -d "$dest_dir" 2>/dev/null
            elif command -v python3 &>/dev/null; then
                python3 -c "
import zipfile, sys
with zipfile.ZipFile('$archive', 'r') as z:
    z.extractall('$dest_dir')
" 2>/dev/null
            else
                print_fail "  未找到 unzip 或 python3，无法解压 zip"
                return 1
            fi
            ;;
        tar.gz|tgz)
            if command -v tar &>/dev/null; then
                tar -xzf "$archive" -C "$dest_dir" 2>/dev/null
            elif command -v python3 &>/dev/null; then
                python3 -c "
import tarfile, sys
with tarfile.open('$archive', 'r:gz') as t:
    t.extractall('$dest_dir')
" 2>/dev/null
            else
                print_fail "  未找到 tar，无法解压 tar.gz"
                return 1
            fi
            ;;
        tar.xz|txz)
            if command -v tar &>/dev/null; then
                tar -xJf "$archive" -C "$dest_dir" 2>/dev/null
            elif command -v python3 &>/dev/null; then
                python3 -c "
import tarfile, sys
with tarfile.open('$archive', 'r:xz') as t:
    t.extractall('$dest_dir')
" 2>/dev/null
            else
                print_fail "  未找到 tar，无法解压 tar.xz"
                return 1
            fi
            ;;
        *)
            print_fail "  不支持的归档类型: $archive_type"
            return 1
            ;;
    esac
}

# 在解压目录中查找可执行文件
find_executable() {
    local dir="$1" name="$2"
    local ext
    ext="$(exe_suffix)"
    local target="${name}${ext}"
    local found=""

    # 1. 浅层精确匹配 (优先 bin/ 子目录或根目录，最多3层深度)
    found="$(find "$dir" -maxdepth 3 -type f -name "$target" -print -quit 2>/dev/null)"
    if [[ -n "$found" ]]; then
        echo "$found"
        return 0
    fi
    # 2. 不带扩展名 (回退查找)
    if [[ -n "$ext" ]]; then
        found="$(find "$dir" -maxdepth 3 -type f -name "${name}" -print -quit 2>/dev/null)"
        if [[ -n "$found" ]]; then
            echo "$found"
            return 0
        fi
    fi
    # 3. 深层精确匹配 (全目录递归)
    found="$(find "$dir" -type f -name "$target" -print -quit 2>/dev/null)"
    if [[ -n "$found" ]]; then
        echo "$found"
        return 0
    fi
    # 4. 模糊匹配 (文件名包含工具名，排除文档文件)
    found="$(find "$dir" -maxdepth 4 -type f -name "*${name}*" \
        ! -name "*.md" ! -name "*.txt" ! -name "LICENSE*" ! -name "*.html" \
        ! -name "*.json" ! -name "*.toml" ! -name "*.yaml" ! -name "*.yml" \
        -print -quit 2>/dev/null)"
    echo "$found"
}

# =============================================================================
# 检查工具是否已安装到 BIN_DIR
# - 先按工具名检查 (name + exe_suffix)
# - 再按可执行文件名检查 (executable 字段，处理 JAR/脚本等)
# - 对 source=install 的工具，检查系统 PATH 中是否已存在可执行命令
# =============================================================================
is_installed() {
    local name="$1"
    local ext
    ext="$(exe_suffix)"
    [[ -x "${BIN_DIR}/${name}${ext}" ]] && return 0
    [[ -f "${BIN_DIR}/${name}${ext}" ]] && return 0
    # 按 executable 字段检查 (处理 JAR 如 google-java-format.jar，跨平台文件在 BIN_ROOT)
    local exe_name
    exe_name="$(jq_get ".binary.tools[] | select(.name==\"${name}\") | .executable")"
    if [[ -n "$exe_name" ]]; then
        [[ -f "${BIN_DIR}/${exe_name}" ]] && return 0
        [[ -f "${BIN_ROOT}/${exe_name}" ]] && return 0
    fi

    # 对 source=install 的工具 (如 rubocop)，检查系统 PATH 是否已安装
    local source
    source="$(jq_get ".binary.tools[] | select(.name==\"${name}\") | .source // empty")"
    if [[ "$source" == "install" ]]; then
        # 检查 executable 字段对应的命令是否在 PATH 中
        if [[ -n "$exe_name" ]] && command -v "$exe_name" &>/dev/null; then
            return 0
        fi
        # 回退到工具名检测
        if command -v "$name" &>/dev/null; then
            return 0
        fi
    fi

    return 1
}

# =============================================================================
# 从源码构建安装 (cargo: URL scheme)
# 适用于无预编译二进制的平台，通过 cargo install --git 从源码编译
# =============================================================================
install_from_cargo() {
    local name="$1" executable="$2" git_url="$3" verify_cmd="${4:-}"
    local ext
    ext="$(exe_suffix)"

    if ! command -v cargo &>/dev/null; then
        skip_tool "$name" "cargo 未安装，无法从源码构建 (需要 Rust 工具链)"
        return 0
    fi

    print_info "  从源码构建 (cargo): ${git_url}"
    local cargo_root="${DOWNLOADS_DIR}/.cargo-install-${name}"
    rm -rf "$cargo_root"

    if cargo install --git "$git_url" --root "$cargo_root" --force 2>&1 | sed 's/^/    /'; then
        local cargo_bin="${cargo_root}/bin/${executable}${ext}"
        if [[ -f "$cargo_bin" ]]; then
            mkdir -p "$BIN_DIR"
            cp "$cargo_bin" "${BIN_DIR}/${executable}${ext}"
            chmod +x "${BIN_DIR}/${executable}${ext}" 2>/dev/null || true
            strip_binary "${BIN_DIR}/${executable}${ext}" "$verify_cmd"
            ok_tool "$name" "已安装 (cargo build) -> bin/${executable}${ext}"
        else
            fail_tool "$name" "cargo build 完成，但未找到二进制文件 ${cargo_bin}"
        fi
    else
        fail_tool "$name" "cargo install 失败"
    fi
    rm -rf "$cargo_root"
}

# =============================================================================
# 通过命令安装工具 (source=install)
# 适用于依赖运行时环境的工具 (如 rubocop 通过 gem install 安装)
# 从 config.json 读取 install_cmds 字段，执行对应平台的安装命令
# =============================================================================
install_from_command() {
    local idx="$1" name="$2" version="$3"

    # 从 config.json 获取当前平台的安装命令数组
    local cmd_json
    cmd_json="$("$JQ_BIN" -r --arg goos "$TARGET_GOOS" \
        ".binary.tools[$idx].install_cmds[\$goos] // empty" "$CONFIG_FILE")"

    if [[ -z "$cmd_json" || "$cmd_json" == "null" ]]; then
        skip_tool "$name" "配置中未定义 ${TARGET_GOOS} 平台的安装命令"
        return 0
    fi

    # 解析命令数组 (每行一个参数)
    local -a cmd_parts=()
    while IFS= read -r arg; do
        [[ -n "$arg" && "$arg" != "null" ]] && cmd_parts+=("$arg")
    done < <("$JQ_BIN" -r --arg goos "$TARGET_GOOS" \
        ".binary.tools[$idx].install_cmds[\$goos][]" "$CONFIG_FILE")

    if [[ ${#cmd_parts[@]} -eq 0 ]]; then
        skip_tool "$name" "安装命令为空"
        return 0
    fi

    # 替换 {version} 占位符
    for i in "${!cmd_parts[@]}"; do
        cmd_parts[$i]="${cmd_parts[$i]//\{version\}/$version}"
    done

    print_info "  执行: ${cmd_parts[*]}"
    local out
    if out=$("${cmd_parts[@]}" 2>&1); then
        printf "%s\n" "$out" | sed 's/^/    /'
        ok_tool "$name" "已安装 (command) -> ${cmd_parts[*]}"
    else
        printf "%s\n" "$out" | sed 's/^/    /'
        fail_tool "$name" "命令安装失败: ${cmd_parts[*]}"
    fi
}

# =============================================================================
# 安装单个独立二进制工具 (source=download)
# =============================================================================

# strip 体积阈值: 仅当二进制大于 5MB 时才执行 strip (小体积工具 strip 收益低，避免破坏签名等风险)
STRIP_MIN_SIZE=$((5 * 1024 * 1024))

# file_size 获取文件字节数
file_size() {
    stat -f%z "$1" 2>/dev/null || stat -c%s "$1" 2>/dev/null || wc -c <"$1" 2>/dev/null || echo 0
}

# fmt_size 人类可读的文件大小 (numfmt 在 macOS 上通常不可用，提供 awk 兜底)
fmt_size() {
    if command -v numfmt >/dev/null 2>&1; then
        numfmt --to=iec "$1" 2>/dev/null && return 0
    fi
    awk -v b="$1" 'BEGIN {
        if (b >= 1073741824) printf "%.1fG", b / 1073741824
        else if (b >= 1048576) printf "%.1fM", b / 1048576
        else if (b >= 1024) printf "%.1fK", b / 1024
        else printf "%dB", b
    }'
}

# strip_binary 对已安装的二进制文件执行 strip 以去除调试符号，减小体积。
# 规则:
#   - 仅当文件体积大于 STRIP_MIN_SIZE (5MB) 时才执行
#   - strip 失败或 strip 后验证失败 (--version/-version/-h/verify_cmd) 时恢复原文件
#   - 仅对目标平台与宿主平台一致时有效 (Windows 目标 / 无 strip 命令时跳过)
strip_binary() {
    local dest="$1"
    local verify_cmd="${2:-}"

    # Windows 目标或无 strip 命令时跳过
    if [[ "$TARGET_GOOS" == "windows" ]] || ! command -v strip >/dev/null 2>&1; then
        return 0
    fi

    [[ -f "$dest" ]] || return 0

    # 体积未超过阈值时跳过
    local size
    size=$(file_size "$dest")
    if (( size <= STRIP_MIN_SIZE )); then
        return 0
    fi

    # 备份原文件 (备份失败则放弃 strip)
    if ! cp "$dest" "${dest}.strip-bak" 2>/dev/null; then
        return 0
    fi

    # 执行 strip; 失败则立即恢复原文件
    local strip_err
    if ! strip_err=$(strip "$dest" 2>&1); then
        mv -f "${dest}.strip-bak" "$dest"
        print_info "  strip: 跳过 (执行失败: ${strip_err})"
        return 0
    fi
    chmod +x "$dest" 2>/dev/null || true

    # 验证 strip 后仍可运行 (优先使用 verify_cmd，否则尝试 --version / -version / -h)
    local ok=false
    if [[ -n "$verify_cmd" ]]; then
        # verify_cmd 可能包含 {exe} 占位符
        local cmd="${verify_cmd//\{exe\}/\"$dest\"}"
        eval "$cmd" >/dev/null 2>&1 && ok=true
    fi
    if ! $ok; then
        "$dest" --version >/dev/null 2>&1 && ok=true
    fi
    if ! $ok; then
        "$dest" -version >/dev/null 2>&1 && ok=true
    fi
    if ! $ok; then
        "$dest" -h >/dev/null 2>&1 && ok=true
    fi

    local before after
    before=$(file_size "${dest}.strip-bak")
    after=$(file_size "$dest")

    if $ok && (( after > 0 && after <= before )); then
        rm -f "${dest}.strip-bak"
        if (( after < before )); then
            print_info "  strip: $(fmt_size "$before") -> $(fmt_size "$after")"
        else
            print_info "  strip: 无体积变化，保持原文件"
        fi
    else
        # strip 后无法运行或体积异常，恢复原文件
        mv -f "${dest}.strip-bak" "$dest"
        print_info "  strip: 跳过 (strip 后验证失败，已恢复原文件)"
    fi
}

install_binary_tool() {
    local idx="$1"
    local name version archive_type executable verify_cmd source

    name="$(jq_get ".binary.tools[$idx].name")"
    version="$(jq_get ".binary.tools[$idx].version")"
    archive_type="$(jq_get ".binary.tools[$idx].archive")"
    executable="$(jq_get ".binary.tools[$idx].executable")"
    verify_cmd="$(jq_get ".binary.tools[$idx].verify_cmd // empty")"
    source="$(jq_get ".binary.tools[$idx].source // empty")"
    [[ -z "$source" ]] && source="download"

    # 如果指定了 --tool，过滤工具
    if is_tool_filtered "$name"; then
        return 0
    fi

    # 跳过已安装
    if is_installed "$name"; then
        already_tool "$name"
        return 0
    fi

    print_step "安装 ${name} v${version}"

    # source=install: 通过配置中的 install_cmds 安装 (适用于依赖运行时环境的工具，如 rubocop)
    if [[ "$source" == "install" ]]; then
        install_from_command "$idx" "$name" "$version"
        return 0
    fi

    # source=preset: 仅校验存在性 (不应走到这里，因为 is_installed 已处理)
    if [[ "$source" == "preset" ]]; then
        skip_tool "$name" "preset 工具应在 data/bin/ 中已存在"
        return 0
    fi

    # source=download: 从 urls 映射查找当前平台的下载 URL
    local url
    if ! url="$(lookup_url "$idx" "$version")"; then
        skip_tool "$name" "当前平台 ${TARGET_GOOS}/${TARGET_GOARCH} 无下载 URL"
        return 0
    fi

    # cargo: 前缀的 URL 表示从源码构建 (适用于无预编译二进制的平台)
    if [[ "$url" == cargo:* ]]; then
        install_from_cargo "$name" "$executable" "${url#cargo:}" "$verify_cmd"
        return 0
    fi

    print_info "  URL: ${url}"
    if [[ "$USE_GH_PROXY" == "true" ]] && [[ "$url" == https://github.com/* ]]; then
        print_info "  代理: ${GH_PROXY_URL%/}/<github-url>"
    fi

    local ext_suffix archive_ext
    ext_suffix="$(exe_suffix)"

    # 从 URL 自动检测归档类型 (覆盖配置中的 archive 字段，支持跨平台不同归档格式)
    case "$url" in
        *.zip)    archive_type="zip" ;;
        *.tar.gz) archive_type="tar.gz" ;;
        *.tar.xz) archive_type="tar.xz" ;;
    esac

    case "$archive_type" in
        zip)     archive_ext=".zip" ;;
        tar.gz)  archive_ext=".tar.gz" ;;
        tar.xz)  archive_ext=".tar.xz" ;;
        raw|*)   archive_ext="${ext_suffix}" ;;
    esac

    local archive="${DOWNLOADS_DIR}/${name}-${version}${archive_ext}"
    local extract_dir="${DOWNLOADS_DIR}/.extract-${name}"

    # 下载
    rm -rf "$extract_dir"
    local dl_url
    dl_url="$(apply_proxy "$url")"
    if ! download_url "$dl_url" "$archive"; then
        if [[ -f "$archive" ]]; then
            local size
            size=$(wc -c <"$archive" 2>/dev/null || echo 0)
            if (( size < 1024 )); then
                print_info "  下载文件过小 (${size}B)，可能是 404 或重定向"
                rm -f "$archive"
            fi
        fi
        fail_tool "$name" "下载失败 (URL: ${url})"
        return 0
    fi

    # 检查下载文件是否合理
    if [[ ! -s "$archive" ]]; then
        fail_tool "$name" "下载文件为空"
        rm -f "$archive"
        return 0
    fi

    # 安装
    mkdir -p "$BIN_DIR"
    if [[ "$archive_type" == "raw" ]]; then
        # raw 二进制: 直接复制
        local dest="${BIN_DIR}/${executable}${ext_suffix}"
        cp "$archive" "$dest"
        chmod +x "$dest" 2>/dev/null || true
        strip_binary "$dest" "$verify_cmd"
        ok_tool "$name" "已安装 -> bin/${executable}${ext_suffix}"
    else
        # 解压归档
        if ! extract_archive "$archive" "$archive_type" "$extract_dir"; then
            fail_tool "$name" "解压失败"
            rm -rf "$extract_dir"
            return 0
        fi

        # 在解压目录中查找可执行文件
        local exe_path
        exe_path="$(find_executable "$extract_dir" "$executable")"
        if [[ -z "$exe_path" ]]; then
            print_info "  解压目录内容 (前30项):"
            find "$extract_dir" -type f 2>/dev/null | head -30 | while read -r f; do
                printf "    %s\n" "${f#$extract_dir/}"
            done
            fail_tool "$name" "未找到可执行文件 ${executable}${ext_suffix}"
            rm -rf "$extract_dir"
            return 0
        fi

        local dest="${BIN_DIR}/${executable}${ext_suffix}"
        cp "$exe_path" "$dest"
        chmod +x "$dest" 2>/dev/null || true
        strip_binary "$dest" "$verify_cmd"
        ok_tool "$name" "已安装 -> bin/${executable}${ext_suffix}"
        rm -rf "$extract_dir"
    fi

    # 清理归档文件 (节省空间)
    rm -f "$archive"
}

# =============================================================================
# 检查/安装预置工具 (source=preset)
# =============================================================================
check_preset_tools() {
    local count
    count=$(jq_get ".binary.tools | length")
    for ((i=0; i<count; i++)); do
        local name source
        name="$(jq_get ".binary.tools[$i].name")"
        source="$(jq_get ".binary.tools[$i].source")"

        if [[ "$source" != "preset" ]]; then
            continue
        fi

        if is_tool_filtered "$name"; then
            continue
        fi

        if is_installed "$name"; then
            already_tool "$name"
        else
            print_warn "${name}: 预置工具未找到 (bin/${name})，需要手动放置或重新获取"
            SKIPPED_TOOLS+=("$name")
        fi
    done
}

# =============================================================================
# 检查所有工具的安装状态 (统一标注已安装/未安装)
# =============================================================================
check_all_tools() {
    print_banner "工具状态检查"
    local count
    count=$(jq_get ".binary.tools | length")

    local installed_list=() missing_list=()

    for ((i=0; i<count; i++)); do
        local name source
        name="$(jq_get ".binary.tools[$i].name")"
        source="$(jq_get ".binary.tools[$i].source")"

        # --tool 过滤: 仅检查指定的工具
        if is_tool_filtered "$name"; then
            continue
        fi

        if is_installed "$name"; then
            printf "  %b✓%b %-16s %b已安装%b\n" "$GREEN" "$NC" "$name" "$DIM" "$NC"
            installed_list+=("$name")
        else
            # 标注未安装原因
            local reason="未安装"
            if [[ "$source" == "preset" ]]; then
                reason="预置工具缺失"
            elif [[ "$source" == "install" ]]; then
                reason="未安装 (命令安装)"
            elif ! has_download_url "$i"; then
                reason="无当前平台下载URL"
            fi
            printf "  %b✗%b %-16s %b%s%b\n" "$RED" "$NC" "$name" "$YELLOW" "$reason" "$NC"
            missing_list+=("$name")
        fi
    done

    # 汇总
    echo ""
    local total=$(( ${#installed_list[@]} + ${#missing_list[@]} ))
    printf "  %b已安装:%b %d/%d" "$GREEN" "$NC" "${#installed_list[@]}" "$total"
    [[ ${#installed_list[@]} -gt 0 ]] && printf "  %b(%s)%b" "$DIM" "${installed_list[*]}" "$NC"
    echo ""
    printf "  %b未安装:%b %d/%d" "$YELLOW" "$NC" "${#missing_list[@]}" "$total"
    [[ ${#missing_list[@]} -gt 0 ]] && printf "  %b(%s)%b" "$DIM" "${missing_list[*]}" "$NC"
    echo ""
}

# =============================================================================
# 打印安装结果汇总 (bin 目录内容 + 各分类列表)
# =============================================================================
print_install_result() {
    echo ""
    print_banner "安装结果"
    printf "  %bbin/ 目录内容:%b\n" "$CYAN" "$NC"
    if [[ -d "$BIN_DIR" ]] && [[ -n "$(ls -A "$BIN_DIR" 2>/dev/null)" ]]; then
        ls -la "$BIN_DIR/" | tail -n +2 | while read -r line; do
            printf "    %s\n" "$line"
        done
    else
        printf "    %b(空)%b\n" "$YELLOW" "$NC"
    fi

    if [[ ${#ALREADY_INSTALLED[@]} -gt 0 ]]; then
        echo ""
        printf "  %b已存在 (%d):%b\n" "$GREEN" "${#ALREADY_INSTALLED[@]}" "$NC"
        printf "    %s\n" "${ALREADY_INSTALLED[*]}"
    fi
    if [[ ${#INSTALLED_TOOLS[@]} -gt 0 ]]; then
        echo ""
        printf "  %b新安装 (%d):%b\n" "$GREEN" "${#INSTALLED_TOOLS[@]}" "$NC"
        for t in "${INSTALLED_TOOLS[@]}"; do printf "    - %s\n" "$t"; done
    fi
    if [[ ${#FAILED_TOOLS[@]} -gt 0 ]]; then
        echo ""
        printf "  %b失败 (%d):%b\n" "$RED" "${#FAILED_TOOLS[@]}" "$NC"
        for t in "${FAILED_TOOLS[@]}"; do printf "    - %s\n" "$t"; done
    fi
    if [[ ${#SKIPPED_TOOLS[@]} -gt 0 ]]; then
        echo ""
        printf "  %b跳过 (%d):%b\n" "$YELLOW" "${#SKIPPED_TOOLS[@]}" "$NC"
        for t in "${SKIPPED_TOOLS[@]}"; do printf "    - %s\n" "$t"; done
    fi
    echo ""
}

# =============================================================================
# 运行时环境检测 (--all 或交互模式时执行)
# =============================================================================
run_runtime_check() {
    if ! $INSTALL_ALL && ! $INTERACTIVE && ! $CHECK_ONLY; then
        return 0
    fi
    print_banner "运行时环境检测"

    local rt_count
    rt_count=$(jq_get ".binary.runtimes | length")
    local missing_runtimes=()
    local missing_rt_indices=()

    for ((i=0; i<rt_count; i++)); do
        local rt_name rt_exe
        rt_name="$(jq_get ".binary.runtimes[$i].name")"
        rt_exe="$(jq_get ".binary.runtimes[$i].exe")"

        # 收集 detect_exes
        local de_count de_exes=()
        de_count=$(jq_get ".binary.runtimes[$i].detect_exes | length")
        for ((j=0; j<de_count; j++)); do
            local de
            de="$(jq_get ".binary.runtimes[$i].detect_exes[$j]")"
            [[ -n "$de" && "$de" != "null" ]] && de_exes+=("$de")
        done

        local rt_info=""
        if rt_info=$(detect_runtime "$rt_name" "$rt_exe" "${de_exes[@]}" 2>/dev/null); then
            local rt_path rt_ver
            rt_path="${rt_info%%|*}"
            rt_ver="${rt_info#*|}"
            printf "  %b✓%b %-10s %b%s%b\n" "$GREEN" "$NC" "$rt_name" "$DIM" "$rt_ver" "$NC"
            printf "    %s\n" "$rt_path"
        else
            printf "  %b✗%b %-10s %b未安装%b\n" "$RED" "$NC" "$rt_name" "$YELLOW" "$NC"
            missing_runtimes+=("$rt_name")
            missing_rt_indices+=("$i")
        fi
    done

    # 询问安装缺失运行时 (仅检查模式跳过安装)
    if [[ ${#missing_runtimes[@]} -gt 0 ]]; then
        echo ""
        if $CHECK_ONLY; then
            print_info "检测到 ${#missing_runtimes[@]} 个运行时未安装 (${missing_runtimes[*]})"
        elif $INSTALL_ALL || confirm "检测到 ${#missing_runtimes[@]} 个运行时未安装 (${missing_runtimes[*]})，是否安装?" "n"; then
            for idx in "${missing_rt_indices[@]}"; do
                local rt_name
                rt_name="$(jq_get ".binary.runtimes[$idx].name")"
                print_step "安装 ${rt_name}..."
                if install_runtime_from_config "$idx"; then
                    ok_tool "$rt_name (runtime)" "安装成功"
                else
                    fail_tool "$rt_name (runtime)" "安装失败 (请手动安装)"
                fi
            done
        else
            print_info "跳过运行时安装 (部分工具可能无法运行)"
        fi
    else
        printf "\n  %b所有运行时环境已就绪%b\n" "$GREEN" "$NC"
    fi
}

# =============================================================================
# 最终汇总
# =============================================================================
final_summary() {
    echo ""
    printf "%b═══════════════════════════════════════════════════════════════%b\n" "$CYAN" "$NC"
    local total_failed=${#FAILED_TOOLS[@]}
    if $CHECK_ONLY; then
        printf "  %b%b✓ 检查完成%b\n" "$GREEN" "$BOLD" "$NC"
    elif [[ $total_failed -eq 0 ]]; then
        printf "  %b%b✓ 安装完成%b\n" "$GREEN" "$BOLD" "$NC"
    else
        printf "  %b%b✗ %d 个工具安装失败，请检查网络或手动安装%b\n" "$RED" "$BOLD" "$total_failed" "$NC"
    fi
    echo ""
    printf "  下载缓存: ${DOWNLOADS_DIR#$PROJECT_ROOT/}/ (可随时清理: rm -rf ${DOWNLOADS_DIR#$PROJECT_ROOT/})"
    echo ""
    echo ""

    [[ $total_failed -eq 0 ]]
}

# =============================================================================
# 工具过滤 (--tool 参数)
# =============================================================================
is_tool_filtered() {
    local name="$1"
    # 无过滤: 全部安装
    [[ ${#SELECTED_TOOLS[@]} -eq 0 ]] && return 1
    for t in "${SELECTED_TOOLS[@]}"; do
        [[ "$t" == "$name" ]] && return 1
    done
    return 0
}

# 判断工具是否为 download 类型且有当前平台的下载 URL
has_download_url() {
    local idx="$1"
    local source
    source="$(jq_get ".binary.tools[$idx].source")"
    if [[ "$source" != "download" ]]; then
        return 1
    fi
    local version
    version="$(jq_get ".binary.tools[$idx].version")"
    local url
    url="$(lookup_url "$idx" "$version")"
    [[ -n "$url" ]]
}

# =============================================================================
# 运行时环境检测
# =============================================================================
detect_runtime() {
    local rt_name="$1"
    local rt_exe="$2"
    shift 2
    local detect_exes=("$@")

    local exe_path=""
    local version_str=""

    # 1. 尝试配置中的候选可执行文件名
    for exe in "${detect_exes[@]}"; do
        if path="$(command -v "$exe" 2>/dev/null)"; then
            exe_path="$path"
            break
        fi
    done

    # 2. 也尝试默认 exe 名
    if [[ -z "$exe_path" && -n "$rt_exe" ]]; then
        exe_path="$(command -v "$rt_exe" 2>/dev/null || true)"
    fi

    if [[ -n "$exe_path" ]]; then
        local vc
        case "$rt_name" in
            java) vc="$("$exe_path" -version 2>&1 | head -1 || true)" ;;
            *)    vc="$("$exe_path" --version 2>&1 | head -1 || true)" ;;
        esac
        version_str="$(echo "$vc" | tr -d '\r' | head -c 80)"
        echo "${exe_path}|${version_str}"
        return 0
    fi
    return 1
}

# =============================================================================
# 安装运行时环境 (从 config.json 读取 install_cmds)
# =============================================================================
install_runtime_from_config() {
    local rt_idx="$1"
    local rt_name
    rt_name="$(jq_get ".binary.runtimes[$rt_idx].name")"

    # 从 config.json 获取当前平台的安装命令数组
    local cmd_json
    cmd_json="$("$JQ_BIN" -r --arg goos "$TARGET_GOOS" \
        ".binary.runtimes[$rt_idx].install_cmds[\$goos] // empty" "$CONFIG_FILE")"

    if [[ -z "$cmd_json" ]]; then
        print_warn "  ${rt_name}: 配置中未定义 ${TARGET_GOOS} 平台的安装命令"
        return 1
    fi

    # 解析命令数组并执行
    local -a cmd_parts=()
    while IFS= read -r arg; do
        [[ -n "$arg" && "$arg" != "null" ]] && cmd_parts+=("$arg")
    done < <("$JQ_BIN" -r --arg goos "$TARGET_GOOS" \
        ".binary.runtimes[$rt_idx].install_cmds[\$goos][]" "$CONFIG_FILE")

    if [[ ${#cmd_parts[@]} -eq 0 ]]; then
        print_warn "  ${rt_name}: 安装命令为空"
        return 1
    fi

    print_info "  执行: ${cmd_parts[*]}"
    local out
    if out=$("${cmd_parts[@]}" 2>&1); then
        printf "%s\n" "$out" | sed 's/^/    /'
        return 0
    else
        printf "%s\n" "$out" | sed 's/^/    /'
        return 1
    fi
}

# =============================================================================
# 交互式工具选择
# =============================================================================
prompt_tools() {
    echo ""
    printf "  %b可选独立二进制工具:%b\n" "$BOLD" "$NC"

    local count
    count=$(jq_get ".binary.tools | length")
    local dl_tools=()
    for ((i=0; i<count; i++)); do
        local name
        name="$(jq_get ".binary.tools[$i].name")"
        if has_download_url "$i"; then
            dl_tools+=("$name")
        fi
    done

    # 每行显示 4 个工具
    local line="    "
    for ((i=0; i<${#dl_tools[@]}; i++)); do
        line+=" ${CYAN}${dl_tools[$i]}${NC}"
        if (( (i+1) % 4 == 0 )); then
            printf "%b\n" "$line"
            line="    "
        fi
    done
    [[ -n "$line" && "$line" != "    " ]] && printf "%b\n" "$line"

    echo ""
    printf "  %b安装选项:%b\n" "$BOLD" "$NC"
    printf "    1) 全部独立二进制 (默认，推荐)\n"
    printf "    2) 自定义选择\n"
    printf "    3) 仅检查，不下载\n"
    printf "  %b请选择 [1-3] (默认 1):%b " "$CYAN" "$NC"
    read -r choice
    choice="${choice:-1}"

    case "$choice" in
        2)
            printf "  %b输入工具名称 (空格分隔，如: biome shfmt stylua):%b " "$CYAN" "$NC"
            read -r tools
            if [[ -n "$tools" ]]; then
                read -ra SELECTED_TOOLS <<<"$tools"
            fi
            ;;
        3)
            CHECK_ONLY=true
            SELECTED_TOOLS=()
            ;;
        *)
            SELECTED_TOOLS=()
            ;;
    esac
}

# =============================================================================
# 主入口
# =============================================================================
main() {
    # 解析参数
    local tools_arg=""
    for a in "$@"; do
        case "$a" in
            --platform=*)
                local p="${a#*=}"
                TARGET_GOOS="${p%%/*}"; TARGET_GOARCH="${p##*/}" ;;
            --tool=*)
                tools_arg="${a#*=}" ;;
            --all)
                INSTALL_ALL=true ;;
            --interactive|-i)
                INTERACTIVE=true ;;
            --yes|-y)
                AUTO_YES=true ;;
            --check)
                CHECK_ONLY=true ;;
            --proxy)
                USE_GH_PROXY=true ;;
            --no-proxy)
                USE_GH_PROXY=false ;;
            -h|--help)
                cat <<HELP
${BOLD}用法:${NC} $0 [options]

${BOLD}选项:${NC}
  --platform=OS/ARCH   目标平台 (darwin/amd64 darwin/arm64 linux/amd64 windows/amd64)
  --tool=<names>       仅安装指定工具 (逗号分隔，如: stylua,shfmt,biome)
  --all                含运行时环境检测
  --interactive, -i    交互式选择工具
  --yes, -y            非交互模式，所有询问自动确认
  --check              仅检查工具安装状态，不下载安装
  --proxy              使用 GitHub 代理 (https://gh-proxy.com) 加速下载
  --no-proxy           不使用代理 (默认)
  -h, --help           显示帮助

${BOLD}环境变量:${NC}
  USE_GH_PROXY=true    同 --proxy，使用代理下载
  GH_PROXY_URL=<url>   自定义代理地址前缀 (默认: https://gh-proxy.com)
  AUTO_PROXY_RETRY=true  直连 GitHub 失败时自动切换代理重试 (默认: true)

${BOLD}示例:${NC}
  $0                           # 安装全部独立二进制
  $0 --tool=stylua             # 仅安装 stylua
  $0 --tool=shfmt,biome        # 安装 shfmt 和 biome
  $0 --all                     # 含运行时检测
  $0 --check                   # 仅检查工具状态，不下载
  $0 --proxy                   # 使用代理加速下载
  USE_GH_PROXY=true $0         # 通过环境变量启用代理
  $0 --platform=linux/amd64    # 指定 Linux 平台
HELP
                exit 0 ;;
            *)
                printf "%b未知参数: %s%b\n" "$RED" "$a" "$NC"
                echo "运行 '$0 --help' 查看帮助"
                exit 1 ;;
        esac
    done

    detect_platform

    # 解析 --tool 参数 (逗号分隔)
    if [[ -n "$tools_arg" ]]; then
        IFS=',' read -ra tool_arr <<<"$tools_arg"
        for t in "${tool_arr[@]}"; do
            t="$(echo "$t" | xargs)"  # trim
            [[ -n "$t" ]] && SELECTED_TOOLS+=("$t")
        done
    fi

    # 确保配置文件存在
    if [[ ! -f "$CONFIG_FILE" ]]; then
        print_fail "配置文件不存在: $CONFIG_FILE"
        exit 1
    fi

    # 确保 jq 可用
    if ! ensure_jq; then
        print_fail "需要 jq 来解析 config.json，请先安装 jq"
        exit 1
    fi

    local platform_label="${TARGET_GOOS}/${TARGET_GOARCH}"
    echo ""
    printf "%b═══════════════════════════════════════════════════════════════%b\n" "$CYAN" "$NC"
    printf "%b  %b三方二进制工具安装 (配置驱动)%b\n" "$CYAN" "$BOLD" "$NC"
    printf "%b═══════════════════════════════════════════════════════════════%b\n" "$CYAN" "$NC"
    printf "  %b配置文件:%b %s\n" "$CYAN" "$NC" "${CONFIG_FILE#$PROJECT_ROOT/}"
    printf "  %b目标平台:%b %s\n" "$CYAN" "$NC" "$platform_label"
    printf "  %b安装目录:%b %s/\n" "$CYAN" "$NC" "${BIN_DIR#$PROJECT_ROOT/}"

    # 交互式选择
    if $INTERACTIVE; then
        prompt_tools
    fi

    # 统计工具数量
    local tool_count
    tool_count=$(jq_get ".binary.tools | length")
    printf "  %b工具总数:%b %s (配置中)\n" "$CYAN" "$NC" "$tool_count"
    if [[ ${#SELECTED_TOOLS[@]} -gt 0 ]]; then
        printf "  %b筛选工具:%b %s\n" "$CYAN" "$NC" "${SELECTED_TOOLS[*]}"
    fi
    if $CHECK_ONLY; then
        printf "  %b模式:%b 仅检查\n" "$CYAN" "$NC"
    fi
    echo ""

    mkdir -p "$BIN_DIR" "$DOWNLOADS_DIR"

    # =========================================================================
    # 第零步: 检查所有工具安装状态 (安装前统一标注已安装/未安装)
    # =========================================================================
    check_all_tools

    # 仅检查模式: 跳过下载安装，直接进入运行时检测和汇总
    if $CHECK_ONLY; then
        check_preset_tools
        print_install_result
        run_runtime_check
        final_summary
        exit 0
    fi

    # =========================================================================
    # 第一步: 下载独立二进制工具 (source=download)
    # =========================================================================
    print_banner "独立二进制工具"
    for ((i=0; i<tool_count; i++)); do
        local source
        source="$(jq_get ".binary.tools[$i].source")"
        if [[ "$source" == "download" ]]; then
            install_binary_tool "$i"
        fi
    done

    # =========================================================================
    # 第二步: 检查预置工具 (source=preset)
    # =========================================================================
    print_banner "预置工具校验"
    check_preset_tools

    # =========================================================================
    # 第三步: 命令安装工具 (source=install, 如 rubocop)
    # =========================================================================
    print_banner "命令安装工具"
    for ((i=0; i<tool_count; i++)); do
        local source
        source="$(jq_get ".binary.tools[$i].source")"
        if [[ "$source" == "install" ]]; then
            install_binary_tool "$i"
        fi
    done

    # =========================================================================
    # 第四步: 安装结果汇总
    # =========================================================================
    print_install_result

    # =========================================================================
    # 第五步: 运行时环境检测 (--all 或交互模式)
    # =========================================================================
    run_runtime_check

    # =========================================================================
    # 最终汇总
    # =========================================================================
    final_summary
}

main "$@"
