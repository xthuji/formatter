#!/usr/bin/env bash
set -euo pipefail

# =============================================================================
# run_tools.sh - Formatter 项目工具脚本
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
PROJECT_NAME="formatter"
PROJECT_BUNDLE_ID="com.xthuji.formatter"
APP_VERSION="$(cat "${PROJECT_ROOT}/data/version.txt" 2>/dev/null | tr -d '[:space:]')"
APP_VERSION="${APP_VERSION:-1.0.0}" # 源自 data/version.txt，回退默认值
BUILD_DIR="${PROJECT_ROOT}/build"
RELEASE_DIR="${PROJECT_ROOT}/release"
DATA_DIR="${PROJECT_ROOT}/data"
BIN_DIR="${DATA_DIR}/bin"
TESTS_DIR="${PROJECT_ROOT}/tests"
ICON_SVG="${DATA_DIR}/icon/icon.svg"
GO_MIN_VERSION="1.21"
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
PASSTHROUGH_ARGS=()

# ---- 基础工具函数 ----
# 使用 printf 替代 echo -e，更安全且跨平台兼容性更好
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

# =============================================================================
# 平台检测
# =============================================================================
detect_platform() {
    if [[ -z "$TARGET_GOOS" ]]; then
        case "$(uname -s)" in
            Darwin) TARGET_GOOS="darwin" ;;
            Linux)  TARGET_GOOS="linux" ;;
            MINGW*|MSYS*|CYGWIN*) TARGET_GOOS="windows" ;;
            *) TARGET_GOOS="darwin" ;;
        esac
    fi
    if [[ -z "$TARGET_GOARCH" ]]; then
        case "$(uname -m)" in
            x86_64|amd64) TARGET_GOARCH="amd64" ;;
            arm64|aarch64) TARGET_GOARCH="arm64" ;;
            *) TARGET_GOARCH="arm64" ;;
        esac
    fi
}

# =============================================================================
# 图标生成逻辑
# =============================================================================
generate_icon_png() {
    local size="$1" output="$2"
    [[ ! -f "$ICON_SVG" ]] && { print_warn "icon.svg 不存在"; return 1; }

    if command -v rsvg-convert &>/dev/null; then
        rsvg-convert -w "$size" -h "$size" "$ICON_SVG" -o "$output" 2>/dev/null && return 0
    fi
    if command -v qlmanage &>/dev/null; then
        local tmp_dir generated
        tmp_dir="$(mktemp -d)"
        qlmanage -t -s "$size" -o "$tmp_dir" "$ICON_SVG" 2>/dev/null
        generated="${tmp_dir}/icon.svg.png"
        if [[ -f "$generated" ]]; then
            cp "$generated" "$output"
            rm -rf "$tmp_dir"
            return 0
        fi
        rm -rf "$tmp_dir"
    fi
    if [[ -f "$BUILD_DIR/appicon.png" ]]; then
        sips -z "$size" "$size" "$BUILD_DIR/appicon.png" --out "$output" 2>/dev/null && return 0
    fi
    return 1
}

generate_icns() {
    local output_icns="$1"
    [[ ! -f "$ICON_SVG" ]] && { print_warn "icon.svg 不存在，跳过 .icns 生成"; return 1; }

    print_step "从 icon.svg 生成 .icns ..."
    local tmp_base iconset_dir
    tmp_base="$(mktemp -d)"
    iconset_dir="${tmp_base}/AppIcon.iconset"
    mkdir -p "$iconset_dir"

    # 使用清理函数确保无论如何退出都能清理临时文件
    local cleanup_status=1
    local sizes=(16 32 128 256 512)
    for s in "${sizes[@]}"; do
        local s2=$((s * 2))
        if ! generate_icon_png "$s" "${iconset_dir}/icon_${s}x${s}.png" || \
           ! generate_icon_png "$s2" "${iconset_dir}/icon_${s}x${s}@2x.png"; then
            print_fail "生成 ${s}x${s} / ${s}x${s}@2x PNG 失败"
            rm -rf "$tmp_base"
            return 1
        fi
    done

    if command -v iconutil &>/dev/null; then
        if iconutil -c icns "$iconset_dir" -o "$output_icns" 2>/dev/null; then
            print_ok ".icns 生成成功: $(basename "$output_icns")"
            cleanup_status=0
        fi
    fi
    
    rm -rf "$tmp_base"
    return $cleanup_status
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
    <string>Formatter</string>
    <key>CFBundleDisplayName</key>
    <string>Formatter</string>
    <key>CFBundleIdentifier</key>
    <string>${PROJECT_BUNDLE_ID}</string>
    <key>CFBundleVersion</key>
    <string>${APP_VERSION}</string>
    <key>CFBundleShortVersionString</key>
    <string>${APP_VERSION}</string>
    <key>CFBundleExecutable</key>
    <string>Formatter</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleIconFile</key>
    <string>AppIcon</string>
    <key>NSHighResolutionCapable</key>
    <true/>
    <key>LSMinimumSystemVersion</key>
    <string>10.13.0</string>
</dict>
</plist>
PLIST
}

# =============================================================================
# 步骤函数
# =============================================================================
step_check_env() {
    print_banner "环境检查"
    if ! command -v go &>/dev/null; then
        print_fail "未找到 go 命令"; fail "env-check"; return 1
    fi
    local go_version
    go_version=$(go version | awk '{print $3}' | sed 's/^go//' || echo "0.0.0")
    print_info "Go 版本: ${go_version}    要求: >= ${GO_MIN_VERSION}"
    if ! version_gte "$go_version" "$GO_MIN_VERSION"; then
        print_fail "Go 版本过低"; fail "env-check"; return 1
    fi
    print_ok "Go 版本满足要求"
    detect_platform
    print_info "目标平台: ${TARGET_GOOS}/${TARGET_GOARCH}"
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
    print_step "委托给 install-bin.sh ..."
    if bash "$INSTALL_BIN_SCRIPT" "${PASSTHROUGH_ARGS[@]}"; then
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

step_build() {
    print_banner "构建 Formatter App"
    detect_platform
    mkdir -p "$BUILD_DIR"

    # 检测交叉编译: 目标平台 != 宿主平台 (此时无法运行刚构建的二进制)
    local host_goos host_goarch
    host_goos="$(go env GOOS)"
    host_goarch="$(go env GOARCH)"
    local is_cross=false
    if [[ "$TARGET_GOOS" != "$host_goos" ]] || [[ "$TARGET_GOARCH" != "$host_goarch" ]]; then
        is_cross=true
        print_info "交叉编译: ${host_goos}/${host_goarch} → ${TARGET_GOOS}/${TARGET_GOARCH}"
    fi

    # 1. 下载 Go 依赖
    print_step "go mod download ..."
    go mod download && print_ok "Go 依赖就绪"

    # 2. 编译二进制
    # CGO 规则:
    #   - 三平台均启用 CGO_ENABLED=1 + build tags (desktop,production) (Wails v2 依赖)
    #   - darwin: UniformTypeIdentifiers 框架
    #   - linux:  GTK3 + WebKit2GTK (pkg-config)
    #   - windows: go-webview2 自带 WebView2Loader，无需额外系统库
    #   - 交叉编译 darwin arch (amd64↔arm64): 需 CGO + 对应 arch 的 clang 目标
    local ext=""
    [[ "$TARGET_GOOS" == "windows" ]] && ext=".exe"
    local bin_name="${PROJECT_NAME}_${TARGET_GOOS}_${TARGET_GOARCH}${ext}"
    local bin_path="${BUILD_DIR}/${bin_name}"

    print_step "go build → ${bin_name}"
    local build_cgo=1 build_tags="-tags desktop,production"
    if [[ "$TARGET_GOOS" == "linux" ]]; then
        # Linux 桌面构建需要 GTK3 + WebKit2GTK (Wails v2 依赖)
        if ! pkg-config --exists gtk+-3.0 webkit2gtk-4.0 2>/dev/null; then
            print_fail "Linux 桌面构建需要 GTK3 和 WebKit2GTK 开发库"
            print_info "  Ubuntu/Debian: sudo apt-get install libgtk-3-dev libwebkit2gtk-4.0-dev"
            fail "build"; return 1
        fi
        export CGO_CFLAGS="$(pkg-config --cflags gtk+-3.0 webkit2gtk-4.0)"
        export CGO_LDFLAGS="$(pkg-config --libs gtk+-3.0 webkit2gtk-4.0)"
    elif [[ "$TARGET_GOOS" == "darwin" ]]; then
        export CGO_LDFLAGS="-framework UniformTypeIdentifiers"
        # darwin 跨架构编译需要指定目标 triple (amd64↔arm64)
        if $is_cross; then
            local darwin_target
            case "$TARGET_GOARCH" in
                amd64)  darwin_target="x86_64-apple-darwin" ;;
                arm64)  darwin_target="arm64-apple-darwin" ;;
            esac
            if [[ -n "$darwin_target" ]]; then
                export CGO_CFLAGS="-target ${darwin_target}"
                print_info "  交叉编译目标: ${darwin_target}"
            fi
        fi
    fi
    # Windows: CGO_ENABLED=1 即可，go-webview2 自带 WebView2Loader，无需额外系统库

    if CGO_ENABLED="$build_cgo" GOOS="$TARGET_GOOS" GOARCH="$TARGET_GOARCH" \
        go build -trimpath ${build_tags} -ldflags="-s -w" -o "$bin_path" ./src/; then
        local size
        size=$(du -h "$bin_path" | cut -f1)
        print_ok "二进制构建成功: ${bin_name} (${size})"
    else
        print_fail "二进制构建失败"; fail "build"; return 1
    fi

    # 3. 下载独立二进制工具到 data/bin/
    # 使用 install-bin.sh 而非运行刚构建的二进制:
    #   - 支持交叉编译 (无法运行目标平台二进制)
    #   - 有更完善的代理重试/错误处理
    #   - --platform= 指定目标平台，下载对应的预编译二进制
    print_step "检查并下载独立二进制工具..."
    if dir_has_files "$BIN_DIR"; then
        local existing=$(find "$BIN_DIR" -maxdepth 1 -mindepth 1 | wc -l | tr -d ' ')
        print_info "  data/bin/ 已有 ${existing} 个工具，跳过下载 (如需重装先执行 clean)"
    else
        local install_platform="${TARGET_GOOS}/${TARGET_GOARCH}"
        if bash "$INSTALL_BIN_SCRIPT" --platform="$install_platform" --yes 2>&1 | sed 's/^/    /'; then
            if dir_has_files "$BIN_DIR"; then
                local dl_count=$(find "$BIN_DIR" -maxdepth 1 -mindepth 1 | wc -l | tr -d ' ')
                print_ok "独立二进制工具已就绪 (${dl_count} 个)"
            else
                print_warn "独立二进制工具下载为空 (网络问题? 可手动运行: ./scripts/install-bin.sh --proxy)"
            fi
        else
            print_warn "部分独立二进制工具下载失败 (可后续手动运行: ./scripts/install-bin.sh --proxy)"
        fi
    fi

    # 4. 复制 bin/ 工具到 build/bin/
    if dir_has_files "$BIN_DIR"; then
        local build_bin="${BUILD_DIR}/bin"
        rm -rf "$build_bin"
        mkdir -p "$build_bin"
        cp -R "$BIN_DIR"/* "$build_bin/" 2>/dev/null || true
        local count=$(find "$build_bin" -maxdepth 1 -mindepth 1 | wc -l | tr -d ' ')
        print_ok "已复制 bin/ 工具到 build/bin/ (${count} 个)"
    fi

    # 5. 平台打包
    if [[ "$TARGET_GOOS" == "darwin" ]]; then
        # --- macOS: .app bundle + 7z ---
        print_step "打包 macOS .app ..."
        local app_name="Formatter"
        local app_dir="${BUILD_DIR}/${app_name}.app"
        local contents_dir="${app_dir}/Contents"
        local macos_dir="${contents_dir}/MacOS"
        local resources_dir="${contents_dir}/Resources"

        rm -rf "$app_dir"
        mkdir -p "$macos_dir" "$resources_dir"

        cp "$bin_path" "${macos_dir}/${app_name}"
        chmod +x "${macos_dir}/${app_name}"

        local default_config="${DATA_DIR}/config.json"
        if [[ -f "$default_config" ]]; then
            cp "$default_config" "${macos_dir}/config.json"
            print_ok "默认配置已嵌入 MacOS/config.json"
        fi

        if ! $is_cross; then
            local icns_path="${resources_dir}/AppIcon.icns"
            if generate_icns "$icns_path"; then
                print_ok "图标已嵌入"
            else
                print_warn "图标生成失败，跳过"
            fi
        else
            print_warn "交叉编译模式，跳过图标生成"
        fi

        if dir_has_files "$BIN_DIR"; then
            local app_bin_dir="${resources_dir}/bin"
            mkdir -p "$app_bin_dir"
            cp -R "$BIN_DIR"/* "$app_bin_dir/" 2>/dev/null || true
            local app_count=$(find "$app_bin_dir" -maxdepth 1 -mindepth 1 | wc -l | tr -d ' ')
            print_ok "三方工具已嵌入 Resources/bin/ (${app_count} 个)"
        else
            print_warn "bin/ 为空，跳过三方工具嵌入"
        fi

        generate_info_plist "${contents_dir}/Info.plist"
        print_ok ".app 打包完成: ${app_dir}"

        mkdir -p "$RELEASE_DIR"
        rm -rf "${RELEASE_DIR}/${app_name}.app"
        cp -R "$app_dir" "$RELEASE_DIR/"
        print_ok "已复制到 release/: ${RELEASE_DIR}/${app_name}.app"

        # 7z 打包 (同 OS 即可)
        if [[ "$TARGET_GOOS" == "$host_goos" ]] && command -v 7z &>/dev/null; then
            local seven_name="${app_name}-${APP_VERSION}-darwin-${TARGET_GOARCH}.7z"
            local seven_path="${RELEASE_DIR}/${seven_name}"

            print_step "打包 7z (最大压缩): ${seven_name}"
            rm -f "$seven_path"
            xattr -cr "$app_dir" 2>/dev/null || true

            if 7z a \
                -mx=9 \
                -m0=lzma2 \
                -mmt=on \
                -x'!.DS_Store' \
                "$seven_path" \
                "${app_dir}" 2>&1 | sed 's/^/    /'; then
                local seven_sz
                seven_sz=$(du -h "$seven_path" | cut -f1)
                print_ok "7z 打包成功: ${seven_name} (${seven_sz})"
            else
                print_warn "7z 打包失败 (7z 错误)，跳过"
            fi
        elif [[ "$TARGET_GOOS" != "$host_goos" ]]; then
            print_warn "跨 OS 交叉编译 (${host_goos}→${TARGET_GOOS})，跳过 7z 打包"
        else
            print_warn "未找到 7z (brew install p7zip)，跳过 7z 打包"
        fi

        printf "\n  %b使用方式:%b\n" "$CYAN" "$NC"
        printf "    %b桌面应用:%b open %s/%s.app\n" "$BOLD" "$NC" "$RELEASE_DIR" "$app_name"
        printf "    %bCLI 格式化:%b %s/%s format main.go\n" "$BOLD" "$NC" "$macos_dir" "$app_name"
        printf "    %bCLI 压缩:%b     %s/%s compress main.js\n" "$BOLD" "$NC" "$macos_dir" "$app_name"
        printf "    %bCLI 高亮:%b     %s/%s highlight main.py\n" "$BOLD" "$NC" "$macos_dir" "$app_name"
        printf "    %bCLI 一键执行:%b %s/%s run main.go\n" "$BOLD" "$NC" "$macos_dir" "$app_name"
        printf "    %b查看语言:%b     %s/%s langs\n" "$BOLD" "$NC" "$macos_dir" "$app_name"
        printf "    %b启动 Web:%b     %s/%s serve\n" "$BOLD" "$NC" "$macos_dir" "$app_name"
    else
        # --- Linux/Windows: 桌面应用 + CLI + Web UI ---
        mkdir -p "$RELEASE_DIR"
        cp "$bin_path" "$RELEASE_DIR/"
        print_ok "已复制到 release/: ${RELEASE_DIR}/$(basename "$bin_path")"

        local archive_name="${PROJECT_NAME}-${APP_VERSION}-${TARGET_GOOS}-${TARGET_GOARCH}"

        if [[ "$TARGET_GOOS" == "linux" ]]; then
            # Linux 桌面应用: 构建标准目录结构 (可安装到 /opt/formatter)
            print_step "打包 Linux 桌面应用 ..."
            local stage_dir="${BUILD_DIR}/linux_stage/Formatter"
            rm -rf "${BUILD_DIR}/linux_stage"
            mkdir -p "$stage_dir/bin"
            mkdir -p "$stage_dir/share/applications"
            mkdir -p "$stage_dir/share/icons/hicolor/256x256/apps"

            cp "$bin_path" "$stage_dir/bin/formatter"
            chmod +x "$stage_dir/bin/formatter"

            local default_config="${DATA_DIR}/config.json"
            [[ -f "$default_config" ]] && cp "$default_config" "$stage_dir/bin/config.json"

            if [[ -f "${DATA_DIR}/linux/formatter.desktop" ]]; then
                cp "${DATA_DIR}/linux/formatter.desktop" "$stage_dir/share/applications/"
                print_ok ".desktop 文件已嵌入"
            fi

            local icon_png="$stage_dir/share/icons/hicolor/256x256/apps/formatter.png"
            if generate_icon_png 256 "$icon_png"; then
                print_ok "图标已嵌入 (256x256)"
            fi

            if dir_has_files "$BIN_DIR"; then
                mkdir -p "$stage_dir/bin/tools"
                cp -R "$BIN_DIR"/* "$stage_dir/bin/tools/" 2>/dev/null || true
                local tool_count=$(find "$stage_dir/bin/tools" -maxdepth 1 -mindepth 1 | wc -l | tr -d ' ')
                print_ok "三方工具已嵌入 (${tool_count} 个)"
            fi

            # 创建安装脚本
            cat > "$stage_dir/install.sh" << 'INSTALL_EOF'
#!/bin/bash
# Formatter 桌面应用安装脚本 (Linux)
set -euo pipefail
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
            chmod +x "$stage_dir/install.sh"

            local archive_path="${RELEASE_DIR}/${archive_name}.tar.gz"
            print_step "打包 tar.gz: ${archive_name}.tar.gz"
            tar -czf "$archive_path" -C "${BUILD_DIR}/linux_stage" "Formatter" 2>&1 && \
                print_ok "tar.gz 打包成功" || print_warn "tar.gz 打包失败，跳过"
            rm -rf "${BUILD_DIR}/linux_stage"

            printf "\n  %b使用方式:%b\n" "$CYAN" "$NC"
            printf "    %b桌面应用:%b 解压后运行 install.sh 安装，或 ./Formatter/bin/formatter\n" "$BOLD" "$NC"
            printf "    %bCLI 格式化:%b Formatter/bin/formatter format main.go\n" "$BOLD" "$NC"
            printf "    %b启动 Web:%b  Formatter/bin/formatter serve\n" "$BOLD" "$NC"

        elif [[ "$TARGET_GOOS" == "windows" ]]; then
            # Windows 桌面应用: zip 含 exe + config + 图标
            if command -v 7z &>/dev/null; then
                local stage_dir="${BUILD_DIR}/win_stage"
                rm -rf "$stage_dir"
                mkdir -p "$stage_dir"

                cp "$bin_path" "$stage_dir/Formatter.exe"

                local default_config="${DATA_DIR}/config.json"
                [[ -f "$default_config" ]] && cp "$default_config" "$stage_dir/config.json"

                # 尝试生成 icon.ico (需要 ImageMagick)
                if [[ -f "$ICON_SVG" ]] && command -v magick &>/dev/null; then
                    magick "$ICON_SVG" -define icon:auto-resize=256,128,64,48,32,16 \
                        "$stage_dir/icon.ico" 2>/dev/null && \
                        print_ok "图标已嵌入 (icon.ico)" || true
                elif [[ -f "$ICON_SVG" ]] && command -v convert &>/dev/null; then
                    convert "$ICON_SVG" -define icon:auto-resize=256,128,64,48,32,16 \
                        "$stage_dir/icon.ico" 2>/dev/null && \
                        print_ok "图标已嵌入 (icon.ico)" || true
                fi

                if dir_has_files "$BIN_DIR"; then
                    mkdir -p "$stage_dir/tools"
                    cp -R "$BIN_DIR"/* "$stage_dir/tools/" 2>/dev/null || true
                    local tool_count=$(find "$stage_dir/tools" -maxdepth 1 -mindepth 1 | wc -l | tr -d ' ')
                    print_ok "三方工具已嵌入 (${tool_count} 个)"
                fi

                local archive_path="${RELEASE_DIR}/${archive_name}.zip"
                print_step "打包 zip: ${archive_name}.zip"
                7z a -tzip -mx=9 "$archive_path" "$stage_dir"/* 2>&1 | sed 's/^/    /' && \
                    print_ok "zip 打包成功" || print_warn "zip 打包失败，跳过"
                rm -rf "$stage_dir"
            else
                print_warn "未找到 7z，跳过 zip 打包"
            fi

            printf "\n  %b使用方式:%b\n" "$CYAN" "$NC"
            printf "    %b桌面应用:%b 解压后运行 Formatter.exe\n" "$BOLD" "$NC"
            printf "    %bCLI 格式化:%b Formatter.exe format main.go\n" "$BOLD" "$NC"
            printf "    %b启动 Web:%b  Formatter.exe serve\n" "$BOLD" "$NC"
        fi
    fi
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

    1) build / b        构建桌面应用 (三平台均支持 GUI + CLI + Web UI)
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
  build / b        构建桌面应用 (三平台均支持 GUI + CLI + Web UI)
  install-bin / i  下载三方二进制工具到 bin/ (委托给 install-bin.sh)
  test / t         运行单元测试
  clean / c        清理构建产物
  run / r          编译并运行 App (构建后启动桌面应用)
  server / s       编译并运行 Web Server (serve 命令，前台运行)

${BOLD}选项:${NC}
  --platform=OS/ARCH   目标平台 (darwin/amd64 darwin/arm64 linux/amd64 windows/amd64)
  --tool=<name>        install-bin 仅下载指定工具
  --all                install-bin 含 npm/pip/gem 工具
  --addr=<addr>        server 监听地址 (如 :8080)
  --no-open            server 不自动打开浏览器
  --browser            server 使用默认浏览器打开

${BOLD}说明:${NC}
  - 菜单支持数字 (1-6) 或英文首字母 (b/i/t/c/r/s) 匹配操作
  - macOS: build 自动打包 .app (含图标 + 内嵌三方工具)
  - Linux: build 生成桌面应用 tar.gz (含 .desktop + 图标 + install.sh)
  - Windows: build 生成桌面应用 zip (含 exe + config + 图标)
  - install-bin 命令委托给 scripts/install-bin.sh 脚本执行
  - server 以前台方式运行，按 Ctrl+C 停止

${BOLD}示例:${NC}
  $0                    # 交互式菜单
  $0 build              # 构建 App
  $0 b                  # 构建 App (简写)
  $0 run                # 编译并运行 App
  $0 r                  # 编译并运行 App (简写)
  $0 server             # 编译并运行 Web Server
  $0 s --addr :8080     # 指定端口运行 Server (简写)
  $0 s --no-open        # 启动 Server 但不打开浏览器
  $0 install-bin        # 安装全部独立二进制
  $0 i --tool=stylua   # 仅安装 stylua (简写)
  $0 i --all           # 含 npm/pip/gem 工具
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

    # 启动 App
    local app_path="${RELEASE_DIR}/Formatter.app"
    if [[ "$TARGET_GOOS" == "darwin" && -d "$app_path" ]]; then
        print_step "启动 macOS App: ${app_path}"
        if open -a "$app_path" 2>/dev/null; then
            print_ok "App 已启动"
        else
            print_fail "App 启动失败"
            fail "run"
        fi
    else
        # 非 macOS: 直接运行编译后的二进制
        local ext=""
        [[ "$TARGET_GOOS" == "windows" ]] && ext=".exe"
        local bin_path="${BUILD_DIR}/${PROJECT_NAME}_${TARGET_GOOS}_${TARGET_GOARCH}${ext}"
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
    local ext=""
    [[ "$TARGET_GOOS" == "windows" ]] && ext=".exe"
    local bin_path="${BUILD_DIR}/${PROJECT_NAME}_${TARGET_GOOS}_${TARGET_GOARCH}${ext}"
    if [[ ! -f "$bin_path" ]]; then
        print_fail "未找到可执行文件: ${bin_path}"
        fail "server"; step_report; return 1
    fi

    chmod +x "$bin_path" 2>/dev/null || true
    # 前台运行 server，用户可 Ctrl+C 退出
    # PASSTHROUGH_ARGS 可包含 --addr / --no-open / --browser 等参数
    print_step "启动 Web Server: ${bin_path} serve ${PASSTHROUGH_ARGS[*]:-}"
    printf "  %b按 Ctrl+C 停止服务器%b\n\n" "$DIM" "$NC"
    "$bin_path" serve "${PASSTHROUGH_ARGS[@]}"
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