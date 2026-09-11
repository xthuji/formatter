# Formatter

代码格式化、压缩与语法高亮的统一工具，支持 20 种语言，提供 CLI 命令行与 Wails 桌面应用双模式。

## 功能特性

- **20 种语言**：Go、JSON、YAML、XML、HTML、JavaScript、TypeScript、CSS、Shell（含 bash/sh/zsh 别名）、Java、Python、Lua、SQL、AppleScript、Ruby、Scala、Rust、INI、Properties、TOML
- **三阶段流水线**：格式化 → 压缩 → 语法高亮，可灵活组合；「美化&高亮」(run) 默认仅执行格式化+高亮，不含压缩
- **双模式入口**：CLI 命令行工具 + Wails 原生桌面应用（同一二进制）
- **配置驱动**：外部工具的命令参数与工具元数据均定义在 JSON 配置中，无需修改代码即可调整
- **三类工具架构**：内置 Go 实现（格式化 JSON/YAML/XML/HTML/Go/INI/Properties/TOML；压缩 JSON/XML/HTML/JS/CSS/SQL）、内嵌脚本/二进制（Java/AppleScript）、外部下载二进制（oxfmt/ruff/shfmt 等），统一配置化执行
- **二进制 strip 优化**：所有独立二进制工具下载后自动执行 `strip` 去除调试符号，并通过版本验证确保功能完整，显著减小 App 体积
- **语言别名**：bash/sh/zsh 统一解析为 shell，yml 解析为 yaml，无需重复配置
- **15 款高亮主题**：7 款浅色 + 8 款深色，支持实时预览
- **单文件部署**：默认配置与前端资源均通过 `//go:embed` 嵌入二进制，配置文件与可执行文件同目录持久化
- **跨平台**：三平台均支持桌面应用 + CLI + Web UI；macOS 提供 amd64 (Intel) 与 arm64 (Apple Silicon) 两种架构

## 快速开始

### 构建

```bash
# 交互式菜单（无参数默认进入菜单模式）
./scripts/run_tools.sh

# 构建当前平台 App（自动准备系统依赖 → 编译 → 内嵌工具 → 打包到 release/）
./scripts/run_tools.sh build

# 指定目标平台构建（macOS 跨架构自动加 clang -target）
./scripts/run_tools.sh build --platform=darwin/arm64

# 编译并立即启动 App
./scripts/run_tools.sh run
```

> 三平台（macOS / Linux / Windows）的构建依赖安装、CGO 参数、build tags、图标与
> 打包逻辑全部集中在 `scripts/run_tools.sh` 中，CI 只负责 runner 引导与产物上传。

### CLI 使用

```bash
# 格式化 (stdin)
echo '{"b":1,"a":2}' | ./Formatter format --lang json

# 自动检测语言 (通过扩展名/shebang/内容识别)
echo 'def hello(): print("world")' | ./Formatter format --lang auto

# 美化&高亮（格式化 + 高亮，默认不含压缩）
./Formatter run --lang go --input main.go

# 显式启用压缩阶段
./Formatter run --lang json --no-compress=false --input main.json

# 查看支持的语言
./Formatter langs

# 启动 Web UI
./Formatter serve
```

> `run` 命令默认 `--no-compress=true`，即「美化&高亮」仅执行 format + highlight；
> 如需在 run 流水线中包含压缩，请显式传入 `--no-compress=false`。

### 桌面应用

```bash
./scripts/run_tools.sh build
open ./release/Formatter.app
```

构建产物 `release/Formatter.app` 双击即可打开原生窗口。App 提供 3 个功能页面（侧边栏按钮鼠标悬停时显示对应标题提示）：

| 页面 | 功能 | tooltip |
|------|------|---------|
| 工作台 | 输入代码，选择操作，一键获得格式化/压缩/美化&高亮结果 | 工作台 |
| 配置管理 | 运行时环境、二进制工具、编程语言、高亮配置的统一管理（含 CRUD） | 配置管理 |
| 关于 | 版本与项目信息 | 关于 |

### App 中的二进制支持 CLI

构建得到的 App 内部二进制同时支持桌面应用和 CLI 命令：

```bash
APP=./release/Formatter.app/Contents/MacOS/Formatter
$APP format main.go                       # 格式化
$APP compress main.js                     # 压缩
$APP highlight main.py                    # 高亮
$APP run main.go                          # 美化&高亮 (format + highlight，默认不含压缩)
$APP run --no-compress=false main.json    # 完整流水线 (format + compress + highlight)
$APP langs                                # 查看支持语言
$APP serve                                # 启动 Web UI
```

## 项目结构

```
formatter/
├── go.mod / go.sum       # Go 模块
├── wails.json            # Wails 配置
├── data/
│   ├── embed.go          # //go:embed 配置嵌入 (package data)
│   ├── config.json       # 默认配置（//go:embed 嵌入二进制）
│   ├── icon/             # App 图标源文件
│   └── bin/              # 三方二进制工具（构建时打包进 .app）
├── scripts/
│   ├── run_tools.sh      # 主构建脚本（依赖准备/编译/打包，三平台统一入口）
│   ├── install-bin.sh    # 三方二进制工具安装脚本（--bundled-only 供构建调用）
│   └── release.sh        # 打 tag 发布脚本
├── src/
│   ├── main.go           # 统一入口（CLI / Wails 自动分发）
│   ├── cli.go            # CLI 命令定义（cobra）
│   ├── appcommon/        # 共享核心（Core/BuildRegistry/CRUD/runtime/io）
│   ├── binary/           # 二进制工具管理（下载/注册/查找/strip）
│   ├── compressor/       # 压缩器（native + tool CmdAdapter）
│   ├── config/           # 配置加载与解析（含 cmd_args.go 模板展开）
│   ├── formatter/        # 格式化器（native + tool CmdAdapter）
│   ├── highlighter/      # 语法高亮（chroma + 主题注册）
│   ├── io/               # 输入输出与语言检测
│   ├── pipeline/         # 流水线引擎
│   ├── registry/         # 格式化器注册中心（含语言别名解析）
│   ├── ui/               # Web UI 服务 + 静态前端资源
│   └── wailsapp/         # Wails 桌面应用绑定
├── tests/                # 单元测试 + 配置化集成测试
│   └── testdata/         # 测试数据与 test_cases.json 配置
├── docs/                 # 技术文档
├── .github/workflows/
│   └── release.yml       # CI：仅 runner 引导 + 调用 run_tools.sh + 产物上传
├── build/                # 中间构建产物（可删除）
└── release/              # 最终发布产物（可删除）
```

## 配置架构

默认配置位于 [data/config.json](data/config.json)，构建时通过 `//go:embed` 嵌入二进制。
运行时配置文件按以下优先级自动定位：

- **开发模式**：工作目录下的 `data/config.json`（统一使用项目内的配置文件）
- **App 模式**：可执行文件同目录的 `config.json`（如 macOS `Formatter.app/Contents/MacOS/config.json`）
- **只读回退**：若目标目录不可写，则回退到 `~/.formatter/config.json`

修改后重启仍保留。

### 配置结构

```json
{
  "server":   { "port": 3020, "auto_port": true },
  "window":    { "width": 1000, "height": 800 },
  "binary":    { "runtimes": [...], "tools": [...] },
  "format":    { "tab_width": 2, "line_ending": "\n" },
  "highlight": { "style": "github", "line_numbers": false, "font_size": 14, "compat_html": false },
  "aliases":   { "bash": "shell", "sh": "shell", "zsh": "shell", "yml": "yaml" },
  "languages": { ... }
}
```

| 顶层字段 | 说明 |
|---------|------|
| `server` | Web UI 服务端口与自动分配开关 |
| `window` | 桌面窗口初始尺寸与最小尺寸 |
| `binary` | 二进制工具与运行时环境定义（详见下文） |
| `format` | 全局格式化参数（缩进、行结束符） |
| `highlight` | 高亮主题、行号开关、字体大小、兼容输出开关 |
| `aliases` | 语言别名映射（如 bash→shell），仅对 formatter/compressor 生效 |
| `languages` | 各语言的具体工具配置（formatter/compressor/indent） |

### 语言别名

部分语言通过别名统一处理，无需在 `languages` 中重复配置。别名映射定义在 `config.json` 的顶层 `aliases` 字段中，可自由扩展：

| 别名 | 规范语言 | 说明 |
|------|----------|------|
| `bash` | `shell` | shfmt 统一处理 |
| `sh` | `shell` | shfmt 统一处理 |
| `zsh` | `shell` | shfmt 统一处理 |
| `yml` | `yaml` | native YAML 格式化器 |

别名解析仅在 formatter/compressor 查找时生效；高亮器由 chroma 原生处理各 shell 变体。

### 运行时环境（binary.runtimes）

`binary.runtimes` 是一个数组，合并了运行时**定义**与用户**自定义路径**。每个运行时包含：

| 字段 | 说明 |
|------|------|
| `name` | 逻辑名称：node / python / java / ruby |
| `exe` | 默认查找的可执行文件名 |
| `path` | 用户自定义路径（空=自动从 PATH 查找，非空=优先使用，支持 `~` 前缀） |
| `version_cmd` | 版本检测参数，如 `["--version"]` |
| `detect_exes` | 备选可执行文件名，如 `["python3", "python"]` |
| `install_cmds` | 各平台安装命令（darwin/linux），空=不支持自动安装 |

用户可在 App 的「配置管理」页面为每个运行时指定自定义路径（`path` 字段），留空时自动从系统 PATH 查找。

> **macOS GUI 应用 PATH 增强**：macOS 上从 Finder 启动的 GUI 应用不继承用户 shell 的 PATH 修改（如 Homebrew、nvm 等），App 在初始化阶段自动通过登录 shell 获取完整 PATH 并补充常见安装目录，确保通过 Homebrew/nvm 安装的运行时能被正确检测。

### 二进制工具（binary.tools）

`binary.tools` 数组定义了 12 个二进制工具的元数据，构建时通过配置驱动注册到全局注册表。每个工具包含：

| 字段 | 说明 |
|------|------|
| `name` | 工具名称 |
| `version` | 版本号 |
| `languages` | 支持的语言列表（数组） |
| `runtime` | 运行时名称（node/python/java/ruby），**空=独立二进制** |
| `path` | 用户直接指定的二进制路径（空=走默认下载/查找流程，支持 `~` 前缀） |
| `executable` | 解压后的可执行文件名（Windows 自动追加 `.exe`） |
| `source` | 安装来源：`download`（下载并打包）/ `preset`（已静态打包）/ `install`（运行时命令安装，不打包） |
| `urls` | 各平台下载 URL 映射（key 为 `os/arch`，支持 `{version}` 占位符；仅 `download` 生效） |
| `archive` | 归档类型：`raw` / `tar.gz` / `tar.xz` / `zip` |
| `install_cmds` | 各平台安装命令（仅 `source=install` 时生效，如 `gem install rubocop`） |
| `verify_cmd` | 版本验证命令模板，如 `{exe} --version` |
| `run_cmd` | 自定义执行命令模板（如 `{runtime} -jar {exe}`），支持 `{runtime}`/`{exe}` 占位符 |

**`source` 字段语义**（决定是否打包进 App）：
- `download`：从 URL 下载二进制，构建时打包进 App Resources/bin；下载后自动 `strip` 去除调试符号
- `preset`：已静态打包进 App（包装脚本+资源），运行时仅验证存在性
- `install`：通过运行时命令安装（如 `gem install`），不打包进 App；运行时通过系统 PATH 查找

**二进制路径查找优先级**（从高到低）：
1. **用户指定路径**（`path` 非空）：展开 `~` 后直接使用
2. **安装目录**（`InstallDir`）：`download`/`preset` 类型工具的默认位置
3. **系统 PATH**：通过 `exec.LookPath` 查找（`install` 类型工具的主要定位方式）

### 工具列表

| 工具 | 语言 | 角色 | 类型 | 运行时 | 说明 |
|------|------|------|------|--------|------|
| `oxfmt` | JavaScript, TypeScript, CSS | 格式化 | download | 独立 | oxc 项目，需通过 wrapper 支持 stdin |
| `oxfmt-wrapper` | JavaScript, TypeScript, CSS | 格式化 | preset | 独立 | 包装 oxfmt 通过临时文件支持 stdin |
| `rubocop` | Ruby | 格式化 | install | 独立 | 通过 `gem install` 安装，不打包进 App |
| `ruff` | Python | 格式化 | download | 独立 | Rust 实现 |
| `prettyplease-cli` | Rust | 格式化 | download | 独立 | 基于 prettyplease 的独立二进制 |
| `shfmt` | Shell | 格式化 | download | 独立 | mvdan/sh 项目 |
| `stylua` | Lua | 格式化 | download | 独立 | JohnnyMorganz/StyLua |
| `scalafmt` | Scala | 格式化 | download | 独立 | scalameta 项目 |
| `google-java-format` | Java | 格式化 | download | Java | JAR 形式，通过 `run_cmd` 模板 `{runtime} -jar {exe}` 调用 |
| `applescript-wrapper` | AppleScript | 格式化 | preset | 独立 | 已静态打包 |
| `java-wrapper` | Java | 格式化 | preset | 独立 | 已静态打包 |
| `sleek` | SQL | 格式化 | download | 独立 | SQL 格式化工具 |

8 个 `download` 工具构建时自动打包进 `Formatter.app/Contents/Resources/bin/`，开箱即用。仅 `google-java-format` 依赖 Java 运行时（JAR 分发）；`rubocop` 通过 `gem install` 在用户环境中安装，不打包进 App。

### 二进制 strip 优化

所有独立二进制工具（`runtime=""`）下载后自动执行 `strip` 去除调试符号：

- **执行时机**：Go 端 `Manager.downloadFromURL` 和 Shell 端 `install-bin.sh` 双重实现
- **跳过场景**：Windows 平台、依赖运行时的工具（JAR/脚本）、`strip` 命令不可用
- **安全验证**：strip 后通过 `verify_cmd` 或 `--version`/`-version`/`-h` 验证工具仍可运行；验证失败自动恢复原文件
- **效果**：显著减小 App 体积，strip 前后字节数对比在 `install-bin.sh` 输出

### 命令模板驱动

外部工具的命令参数定义在配置的 `cmd` 字段中，业务代码通过 `config.ToolConfig.ExpandCmdArgs()` 统一解析和执行。`run_cmd` 模板（仅用于运行时依赖工具）通过 `binary.Manager.BuildToolCmd()` 解析。

#### cmd 模板占位符（ExpandCmdArgs）

| 占位符 | 说明 | 示例值 |
|--------|------|--------|
| `{tab_width}` | 缩进宽度（来自 format 配置或语言级 indent） | `2` / `4` |
| `{use_tabs}` | 是否使用 Tab（由 `tab_width=-1` 派生） | `true` / `false` |
| `{indent_style}` | 缩进风格（space/tab 转换） | `space` / `tab` |
| `{line_ending}` | 行结束符 | `\n` / `\r\n` |
| `{ext}` | 语言对应文件扩展名 | `js` / `ts` / `css` / `go` |
| `{lang}` | 语言名称 | `javascript` |
| `{dialect}` | SQL 方言（默认 ansi，可通过 options.dialect 覆盖） | `ansi` |
| `{edition}` | Rust edition（默认 2021，可通过 options.edition 覆盖） | `2021` |
| `{line_length}` | Python 行长度（默认 88，可通过 options.line_length 覆盖） | `88` |
| `{options.xxx}` | 自定义 options 字段 | — |

#### run_cmd 模板占位符（BuildToolCmd）

仅用于 `runtime != ""` 的工具（如 JAR），支持 `{runtime}` 和 `{exe}` 两个占位符：

| 占位符 | 说明 | 示例值 |
|--------|------|--------|
| `{runtime}` | 运行时可执行文件路径 | `/usr/bin/java` |
| `{exe}` | 工具可执行文件路径 | `google-java-format.jar` |

#### 添加新工具

无需修改 Go 代码，只需在配置文件 `languages` 中添加：

```json
"mylang": {
  "formatter": {
    "backend": "tool",
    "tool": "myformatter",
    "cmd": ["--stdin", "--indent", "{tab_width}"]
  },
  "indent": { "tab_width": 4 }
}
```

业务代码会自动读取配置并注册对应的适配器。若工具为独立二进制，还需在 `binary.tools` 数组中补充工具元数据（`name`/`version`/`urls`/`executable` 等字段）。

## 语言支持一览

| 语言 | 格式化 | 压缩 | 高亮 | 格式化工具 | 压缩实现 |
|------|--------|------|------|----------|----------|
| Go | ✅ native | — | ✅ | 内置 go | — |
| JSON | ✅ native | ✅ native | ✅ | 内置 json | 内置 json |
| YAML | ✅ native | — | ✅ | 内置 yaml | — |
| XML | ✅ native | ✅ native | ✅ | 内置 xml | 内置 xml |
| HTML | ✅ native | ✅ native | ✅ | 内置 html | 内置 html |
| JavaScript | ✅ tool | ✅ native | ✅ | oxfmt-wrapper | 内置 js |
| TypeScript | ✅ tool | — | ✅ | oxfmt-wrapper | — |
| CSS | ✅ tool | ✅ native | ✅ | oxfmt-wrapper | 内置 css |
| Shell | ✅ tool | — | ✅ | shfmt | — |
| Java | ✅ tool | — | ✅ | java-wrapper / google-java-format | — |
| Python | ✅ tool | — | ✅ | ruff | — |
| Lua | ✅ tool | — | ✅ | stylua | — |
| SQL | ✅ tool | ✅ native | ✅ | sleek | 内置 sql |
| AppleScript | ✅ tool | — | ✅ | applescript-wrapper | — |
| Ruby | ✅ tool | — | ✅ | rubocop (gem) | — |
| Scala | ✅ tool | — | ✅ | scalafmt | — |
| TOML | ✅ native | — | ✅ | 内置 toml | — |
| INI | ✅ native | — | ✅ | 内置 ini | — |
| Properties | ✅ native | — | ✅ | 内置 properties | — |
| Rust | ✅ tool | — | ✅ | prettyplease-cli | — |

> 压缩器全部为内置 Go 实现（JSON/XML/HTML/JS/CSS/SQL），通过 `tdewolff/minify` 等纯 Go 库与 GoSQLX CompactStyle 完成，无外部二进制依赖。

## 高亮主题

支持 15 款 chroma 主题：

- **浅色（7 款）**：GitHub、Eclipse、IntelliJ IDEA、Emacs、Visual Studio、Xcode、Solarized Light
- **深色（8 款）**：Monokai、IDEA Darcula、Dracula、GitHub Dark、Solarized Dark、Atom One Dark、Nord、Gruvbox

### HTML 兼容输出

针对 OneNote / Quiver 等应用粘贴高亮 HTML 时空白与换行丢失的问题，提供 `compat_html` 配置项与 `--compat-html` CLI flag：

- **配置文件**：`highlight.compat_html` 控制默认行为（配置管理页可修改）
- **CLI**：`$APP highlight main.py --compat-html` 或 `$APP run main.go --compat-html`
- **工作台**：快速操作区「兼容输出」复选框临时指定（仅本次请求生效）

启用后高亮输出会移除 `display:flex` 样式、将空格/Tab 替换为 `&nbsp;`、将换行替换为 `<br>`，确保粘贴后缩进与换行正确保留。

## 构建命令一览

```bash
./scripts/run_tools.sh build        # 构建当前平台 App（依赖 → 编译 → 内嵌工具 → 打包）
./scripts/run_tools.sh install-bin  # 下载三方二进制到 data/bin/
./scripts/run_tools.sh run          # 编译并立即启动 App
./scripts/run_tools.sh server       # 编译并运行 Web Server（前台）
./scripts/run_tools.sh test         # 单元测试 + 覆盖率
./scripts/run_tools.sh clean        # 清理构建产物
```

支持英文首字母简写（CLI 和交互式菜单均可）：

```bash
./scripts/run_tools.sh b            # = build
./scripts/run_tools.sh i            # = install-bin
./scripts/run_tools.sh r            # = run
./scripts/run_tools.sh t            # = test
./scripts/run_tools.sh c            # = clean
```

选项：

- `--platform=OS/ARCH`：指定目标平台（`darwin/amd64`、`darwin/arm64`、`linux/amd64`、`windows/amd64`）
- `--skip-deps`：跳过系统构建依赖自动安装（环境已就绪时用）
- `--tool=<name>`：install-bin 仅下载指定工具
- `--all`：install-bin 含运行时环境检测
- `--addr=<addr>` / `--no-open` / `--browser`：server 运行参数

### 平台构建

| 平台 | 架构 | CGO | Build Tags | 产物 | 说明 |
|------|------|-----|-----------|------|------|
| macOS | amd64 (Intel), arm64 (Apple Silicon) | `CGO_ENABLED=1` | `desktop,production` | `Formatter-<ver>-darwin-<arch>.7z` | 链接 `-framework UniformTypeIdentifiers`；跨架构自动加 `clang -target`；打包 .app（icns + Info.plist + 内嵌工具）+ ad-hoc 签名；无 7z 时回退 .zip |
| Linux | amd64 | `CGO_ENABLED=1` | `desktop,production[,webkit2_41]` | `Formatter-<ver>-linux-amd64.tar.gz` | 脚本自动 apt/dnf/pacman/zypper 安装 GTK3 + WebKit2GTK，并探测 4.0/4.1 自动追加 `webkit2_41` tag；包内含 .desktop + 图标 + install.sh |
| Windows | amd64 | `CGO_ENABLED=0` | `desktop,production` | `Formatter-<ver>-windows-amd64.zip` | go-webview2 动态加载 WebView2Loader.dll，无需 MinGW/gcc；包含 Formatter.exe + config.json + icon.ico + tools/ |

- 三平台产物命名统一为 `Formatter-<版本>-<os>-<arch>.<ext>`，版本取自 `data/version.txt`，输出到 `release/`
- macOS 最低部署版本由脚本统一定为 `12.0`（无需在 CI 设置 `MACOSX_DEPLOYMENT_TARGET`）
- 构建时调用 `install-bin.sh --bundled-only`，仅拉取需内嵌的 `source=download` 工具（`rubocop` 等 `source=install` 工具不在构建机上安装）

### CI 构建

`.github/workflows/release.yml`（`v*` tag 触发）只做三件事：checkout、`setup-go`（`go-version-file: go.mod`）、
调用 `./scripts/run_tools.sh build --platform=...` 并上传 `release/` 产物。
所有系统依赖（apt GTK/WebKit、brew p7zip）、编译参数与打包均由脚本自己完成，
因此本地执行一条命令就能得到与 CI 完全一致的产物。

## 测试

测试采用配置化驱动，所有测试用例定义在 [tests/testdata/test_cases.json](tests/testdata/test_cases.json) 中：

- **格式化测试**：覆盖所有 20 种语言，含别名解析验证（bash/sh/zsh → shell）
- **压缩测试**：覆盖所有有压缩器的语言
- **高亮测试**：覆盖 20 种语言（含 shell 变体）
- **配置一致性**：验证 test_cases.json 与 config.json 的一致性
- **CLI 端到端**：使用编译的 App 二进制执行格式化/压缩/高亮，完整验证业务流程

```bash
./scripts/run_tools.sh test    # 运行所有测试
```

## 开发

```bash
# 环境要求
Go >= 1.26        # 以 go.mod 的 go 指令为准，脚本会自动解析
macOS: Xcode Command Line Tools (CGO 依赖)

# 开发流程
./scripts/run_tools.sh test    # 单元测试
./scripts/run_tools.sh build   # 构建验证

# Wails 开发模式（热重载）
wails dev
```

## 许可证

[MIT License](LICENSE)

本项目使用 AI 辅助编程工具（Cursor / Qoder）开发。
