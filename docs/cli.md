# Formatter 命令行使用手册

> 本文档梳理 `formatter` 的 CLI 用法、参数与示例，供直接查阅使用。
> 内置帮助：运行 `formatter help`、`formatter <命令> --help` 或 `formatter help <命令>` 可查看同类说明。

## 目录

- [1. 运行模式](#1-运行模式)
- [2. 可执行文件位置](#2-可执行文件位置)
- [3. 输入 / 输出](#3-输入--输出)
- [4. 语言自动检测](#4-语言自动检测)
- [5. 全局参数](#5-全局参数)
- [6. 命令总览](#6-命令总览)
- [7. 处理命令](#7-处理命令)
  - [format — 格式化](#format--格式化)
  - [compress — 压缩](#compress--压缩)
  - [highlight — 语法高亮](#highlight--语法高亮)
  - [run — 完整流水线](#run--完整流水线)
- [8. 辅助命令](#8-辅助命令)
  - [langs](#langs)
  - [version](#version)
  - [serve](#serve)
- [9. binary — 外部工具管理](#9-binary--外部工具管理)
- [10. 后端 (backend) 说明](#10-后端-backend-说明)
- [11. 配置与环境变量](#11-配置与环境变量)
- [12. 典型工作流示例](#12-典型工作流示例)
- [13. 常见问题](#13-常见问题)

---

## 1. 运行模式

同一个二进制既是桌面 App 也是 CLI 工具，靠**第一个位置参数**区分：

| 调用方式 | 行为 |
|----------|------|
| `formatter`（无参数） | 启动桌面 GUI (Wails) |
| `formatter <子命令> ...` | 以 CLI 模式运行 |
| `formatter <未知命令>` | 报错退出，提示 `Run 'formatter help' for usage.` |

CLI 识别的子命令：`format`、`compress`、`highlight`、`run`、`binary`、`langs`、`version`、`serve`、`help`。

> 解析时会跳过全局标志及其值（如 `-l json`、`--style monokai`），因此 `formatter -l json format a.txt` 这类写法也能正确进入 CLI 模式。

## 2. 可执行文件位置

发布包中的 CLI 入口（构建产物路径，来自 `scripts/run_tools.sh`）：

| 平台 | CLI 调用 |
|------|----------|
| macOS | `Formatter.app/Contents/MacOS/Formatter format main.go` |
| Linux | `formatter format main.go`（安装后位于 `/usr/local/bin/formatter`） |
| Windows | `Formatter.exe format main.go` |

下文示例统一以 `formatter` 代指可执行文件。开发环境可用 `go run ./src/ <子命令>`。

## 3. 输入 / 输出

**输入来源优先级**：`--clipboard` > 位置参数 `[file]` > `--input/-i` > `stdin`

**输出去向**：`--clipboard` 时写回剪贴板；否则 `--output/-o 文件` > `stdout`

```bash
# 位置参数指定输入文件
formatter format main.go

# -i / -o 显式指定输入输出
formatter format -i src/app.js -o dist/app.js

# 管道 / stdin（stdin 无法从文件名推断语言，务必用 -l 指定）
cat query.sql | formatter format -l sql
formatter format -l go < main.go > main.fmt.go

# 剪贴板读写：把剪贴板内容格式化后写回剪贴板
pbcopy < app.js && formatter compress -c   # macOS 示例
```

> ⚠️ `format`/`compress`/`highlight`/`run` 均只接受**最多 1 个**位置参数（`MaximumNArgs(1)`）。

## 4. 语言自动检测

不指定 `-l` 时，按以下优先级检测（规则全部来自 `config.json` 的 `languages.*.detection`）：

1. **文件扩展名**（仅当输入为文件时可用，优先于内容判断）
2. **Shebang**（首行以 `#!` 开头时匹配）
3. **内容强信号正则**（对前 2048 字节按 `priority` 排序匹配）

检测不到时回退为 `text`（仅高亮有效，格式化/压缩会跳过）。

**从 stdin / 剪贴板读取时不携带文件名，无法用扩展名检测**，此时请显式 `-l`。

支持的语言及别名：

| 语言 | `-l` 值 | 备注 |
|------|---------|------|
| AppleScript | `applescript` | |
| CSS | `css` | 支持压缩 |
| Go | `go` | |
| HTML | `html` | 支持压缩 |
| INI | `ini` | |
| Java | `java` | |
| JavaScript | `javascript` | 支持压缩 |
| JSON | `json` | 支持压缩 |
| Lua | `lua` | |
| Properties | `properties` | |
| Python | `python` | |
| Ruby | `ruby` | |
| Rust | `rust` | |
| Scala | `scala` | |
| Shell | `shell` | 别名：`bash` / `sh` / `zsh` |
| SQL | `sql` | 支持压缩 |
| TOML | `toml` | |
| TypeScript | `typescript` | |
| XML | `xml` | 支持压缩 |
| YAML | `yaml` | 别名：`yml` |

以 `formatter langs` 查看运行时实际支持的语言矩阵。

## 5. 全局参数

所有子命令共享以下持久化标志（可放在子命令前或后）：

| 参数 | 简写 | 默认值 | 说明 |
|------|------|--------|------|
| `--lang` | `-l` | `auto` | 指定语言，`auto` 时自动检测 |
| `--input` | `-i` | stdin | 输入文件 |
| `--output` | `-o` | stdout | 输出文件 |
| `--clipboard` | `-c` | false | 使用剪贴板进行输入/输出 |
| `--tab-width` | | `0` | 缩进宽度：`-1`=Tab，`>0`=空格数，`0`=用配置默认值 |
| `--line-ending` | | 配置值 | 行结束符：`lf` / `crlf` |
| `--style` | | `github` | 高亮主题（见下） |
| `--font-size` | | `0` | 高亮字体大小 px，`0`=用配置默认值 |
| `--line-numbers` | | false | 高亮输出显示行号 |
| `--compat-html` | | false | 兼容性 HTML 输出（适配 OneNote / Quiver） |
| `--formatter-backend` | | 自动 | 强制格式化后端：`native` / `tool` |
| `--compressor-backend` | | 自动 | 强制压缩后端：`native` / `tool` |
| `--highlighter-backend` | | 自动 | 强制高亮后端 |
| `--config` | | 自动定位 | 指定配置文件路径 |
| `--bin-dir` | | 自动探测 | 指定外部二进制工具目录 |
| `--verbose` | `-v` | false | 详细输出 |
| `--quiet` | `-q` | false | 静默模式 |
| `--help` | `-h` | | 显示帮助 |
| `--version` | | | 显示版本 |

**高亮主题** (`--style`)：如 `github`、`monokai`、`eclipse`、`idea`、`idea-darcula` 等（chroma 主题 + 项目内置 `src/highlighter/themes/`）。未指定时用配置文件 `highlight.style`。

## 6. 命令总览

```
formatter <命令> [文件] [全局参数] [命令专属参数]

处理命令    format / compress / highlight / run
辅助命令    langs / version / serve
工具管理    binary {install | check | list | remove | update}
```

## 7. 处理命令

### format — 格式化

仅执行格式化，输出纯代码（非 HTML）。

```bash
formatter format main.go                    # 结果打印到 stdout
formatter format app.js --tab-width 4       # 4 空格缩进
formatter format main.go --tab-width -1     # 使用 Tab 缩进
formatter format data.xml -o data.fmt.xml   # 写入文件
formatter format --formatter-backend native config.json   # 强制用 Go 原生格式化器
```

### compress — 压缩

仅执行压缩 / 最小化。**仅 `css`、`html`、`javascript`、`json`、`sql`、`xml` 支持**，其余语言会被静默跳过（输出保持原样）。

```bash
formatter compress styles.css
formatter compress app.js -o app.min.js
formatter compress --lang json -i data.json
```

### highlight — 语法高亮

仅执行高亮，输出为 **HTML 片段**（内联样式，非独立文档）。

```bash
formatter highlight main.go --style monokai -o main.html
formatter highlight app.js --line-numbers --font-size 16 -o app.html
# 兼容 OneNote/Quiver（内联样式 + 空格保护 + <br> 换行）
formatter highlight -l sql -i query.sql --compat-html -o out.html
```

> 因输出为 HTML，保存到文件时建议 `-o xxx.html`。

### run — 完整流水线

组合 格式化 + 压缩 + 高亮。**默认 = 格式化 + 高亮（不含压缩）**，即"美化 & 高亮"。

命令专属参数：

| 参数 | 默认 | 说明 |
|------|------|------|
| `--no-format` | false | 跳过格式化步骤 |
| `--no-compress` | **true** | 跳过压缩步骤（默认就跳过） |
| `--no-highlight` | false | 跳过高亮步骤 |

```bash
formatter run main.go --style idea -o main.html   # 格式化 + 高亮
formatter run app.js --no-highlight               # 仅格式化（等价 format）
formatter run data.json --no-compress=false       # 显式启用压缩
formatter run -l java -i Service.java -o out.html
```

## 8. 辅助命令

### langs

列出所有支持的语言及其格式化 / 压缩 / 高亮后端：

```bash
formatter langs
#   json [fmt: native/json] [cmp: native/json] [hl: chroma]
#   python [fmt: tool/ruff] [hl: chroma]
#   ...
```

### version

```bash
formatter version      # formatter vX.Y.Z
formatter --version    # 等效
```

### serve

启动 Web UI 界面（图形化操作）。

| 参数 | 说明 |
|------|------|
| `--addr` | 监听地址（默认用配置文件端口，如 `:8080`） |
| `--no-open` | 启动但不自动打开窗口 |
| `--browser` | 使用默认浏览器打开（不用 Chrome app 模式） |

```bash
formatter serve
formatter serve --addr :8080
formatter serve --browser --no-open
```

## 9. binary — 外部工具管理

管理依赖外部二进制的格式化工具（`tool` 后端）。`native` 后端语言无需安装。

| 子命令 | 专属参数 | 说明 |
|--------|----------|------|
| `install` | `--lang` / `--tool` / `--embedded-only` | 下载 / 安装工具 |
| `check` | | 检查已安装工具状态与路径 |
| `list` | | 列出已安装的二进制 |
| `remove` | `--tool`（必填） | 卸载指定工具 |
| `update` | | 更新到配置文件中指定的版本 |

```bash
formatter binary install                 # 安装全部工具
formatter binary install --tool ruff     # 仅安装指定工具（逗号分隔多个）
formatter binary install --tool ruff,shfmt
formatter binary install --lang python   # 安装某语言所需工具
formatter binary install --embedded-only # 仅装随 App 打包的独立二进制
formatter binary check
formatter binary list
formatter binary update
formatter binary remove --tool ruff
```

工具与语言对应（源自 `config.json` 的 `binary.tools`）：

| 工具 | 语言 | 运行时依赖 |
|------|------|-----------|
| `oxfmt` | javascript / typescript / css | 无（独立二进制） |
| `ruff` | python | 无 |
| `shfmt` | shell | 无 |
| `stylua` | lua | 无 |
| `scalafmt` | scala | 无 |
| `sleek` | sql | 无 |
| `prettyplease-cli` | rust | 无 |
| `google-java-format` | java | java |
| `rubocop` | ruby | ruby |

> `java` / `ruby` 等依赖运行时的工具需要本机已安装 `java` / `ruby`（或用 `scripts/install-bin.sh` 安装）。

## 10. 后端 (backend) 说明

每个语言的格式化 / 压缩可选两类后端：

- **`native`**：Go 内置实现，零外部依赖，随二进制即用即走。
- **`tool`**：调用外部二进制（见第 9 节），需先安装。

优先级由 `config.json` 中该语言的 `formatter.backend` 决定；CLI 可用
`--formatter-backend` / `--compressor-backend` / `--highlighter-backend` 临时强制覆盖。

```bash
# SQL 采用双后端设计：格式化默认 sleek(tool)，压缩为 native；可临时切换
formatter format q.sql --formatter-backend native   # 强制用 Go 原生格式化 SQL
```

## 11. 配置与环境变量

**配置文件定位优先级**：

1. `--config <path>` 显式指定
2. 工作目录下 `data/config.json`（开发模式）
3. 可执行文件同目录的 `config.json`（App 模式）
4. 均不存在时使用内嵌默认配置（写回时若目标不可写则回退 `~/.formatter/config.json`）

**环境变量**：

| 变量 | 说明 |
|------|------|
| `FORMATTER_BIN_HOME` | 外部二进制工具目录。优先级低于 `--bin-dir`，高于自动探测 |

外部二进制目录探测顺序：显式参数（`--bin-dir`）→ `FORMATTER_BIN_HOME` → App bundle `Resources/bin` → `exeDir/bin` → `exeDir/data/bin` → `~/.formatter`。

```bash
# 用自定义配置与工具目录运行
formatter format main.go --config ./my-config.json --bin-dir ./tools

# 或用环境变量指定工具目录
FORMATTER_BIN_HOME=./data/bin/darwin/arm64 formatter binary list
```

## 12. 典型工作流示例

```bash
# ① 格式化单个文件并原地查看结果
formatter format src/handler.go

# ② 批量：配合 shell 循环格式化目录下 JS
for f in src/**/*.js; do formatter format "$f" -o "$f"; done

# ③ 生成带高亮的 HTML 用于文档/笔记
formatter run README_CODE.ts --style github --line-numbers -o code.html

# ④ 粘贴到 OneNote 前用兼容模式导出
formatter highlight -l python -i script.py --compat-html -o script.html

# ⑤ 剪贴板美化（复制 → 格式化+高亮 → 写回剪贴板）
pbcopy < App.jsx && formatter run -c

# ⑥ 首次使用前安装外部工具，再确认状态
formatter binary install --lang python
formatter binary check
```

## 13. 常见问题

- **Q：从管道读取结果不对 / 语言识别错？**
  A：stdin 无文件名，无法按扩展名检测，请显式加 `-l`，如 `formatter format -l sql`。

- **Q：`compress` 某语言没反应（原样输出）？**
  A：该语言无压缩器（仅 css/html/javascript/json/sql/xml 支持），属预期的静默跳过。

- **Q：`highlight` / `run` 输出是 HTML 而非代码？**
  A：这是设计行为——高亮输出为内联样式 HTML 片段，请配 `-o xxx.html` 保存或用浏览器查看。

- **Q：格式化 Python/Java/Shell 等报错找不到工具？**
  A：这些语言用 `tool` 后端，需先 `formatter binary install --tool <名称>`；依赖运行时的（java/ruby）还需本机装有对应运行时。

- **Q：想临时换缩进 / 主题 / 后端而不动配置文件？**
  A：用全局参数 `--tab-width`、`--style`、`--formatter-backend` 等，优先级高于配置文件。

- **Q：`run` 为什么没压缩？**
  A：`run` 默认 `--no-compress=true`（美化 & 高亮不含压缩），需要压缩请显式 `--no-compress=false`。
