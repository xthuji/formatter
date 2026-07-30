(function () {
  "use strict";

  const $ = (id) => document.getElementById(id);
  const loading = $("loading");
  const loadingText = $("loadingText");
  const toast = $("toast");

  const IS_WAILS = typeof window.runtime !== "undefined" || typeof window.go !== "undefined";

  // ---- Wails Bridge ----
  // Wails generates bindings under window.go.<package>.<Struct>
  // Our App struct is in package "wailsapp", so bindings are at window.go.wailsapp.App
  function getWailsApp() {
    if (window.go) {
      // Try wailsapp package first (our actual binding location)
      if (window.go.wailsapp && window.go.wailsapp.App) {
        return window.go.wailsapp.App;
      }
      // Fallback: try main package (in case of different Wails config)
      if (window.go.main && window.go.main.App) {
        return window.go.main.App;
      }
    }
    return null;
  }

  async function wailsCall(method, ...args) {
    if (IS_WAILS && window.runtime && window.runtime.Call) {
      return window.runtime.Call(method, ...args);
    }
    const app = getWailsApp();
    if (app) {
      const fn = app[method];
      if (typeof fn === "function") {
        return fn(...args);
      }
    }
    throw new Error("Wails runtime not available: method " + method);
  }

  // 生成后端类型彩色标签: native → 绿色"内置", tool → 蓝色"工具"
  function backendTag(backend) {
    if (backend === "native") return '<span class="backend-tag be-native">内置</span>';
    if (backend === "tool") return '<span class="backend-tag be-tool">工具</span>';
    return '<span class="backend-tag">' + escapeHtml(backend || "") + "</span>";
  }

  // 从配置动态构建 扩展名→语言 映射 (替代硬编码的 LANG_MAP)
  // 遍历 currentConfig.languages 中每个语言的 detection.extensions
  function buildLangMap() {
    const map = {};
    const langs = currentConfig.languages || {};
    for (const lang in langs) {
      const det = langs[lang].detection;
      if (det && det.extensions) {
        det.extensions.forEach((ext) => {
          map[ext] = lang;
        });
      }
    }
    return map;
  }

  // 从配置获取语言的首个文件扩展名 (无点前缀)，用于下载文件名
  function langExtension(lang) {
    const lc = (currentConfig.languages || {})[lang];
    if (lc && lc.detection && lc.detection.extensions && lc.detection.extensions.length > 0) {
      return lc.detection.extensions[0].replace(/^\./, "");
    }
    return "txt";
  }

  // 语言标签背景色列表 (全部深色背景 + 白色文字，确保对比度)
  // 按语言在配置中的出现顺序循环取色，支持后期自定义添加语言
  const LANG_COLORS = [
    "#00ADD8", // 0 - 青色
    "#3776AB", // 1 - 蓝色
    "#D4A017", // 2 - 金色
    "#3178C6", // 3 - 蓝
    "#2965F1", // 4 - 蓝
    "#E34F26", // 5 - 橙
    "#5B6B7D", // 6 - 灰蓝
    "#CB171E", // 7 - 红
    "#0060AC", // 8 - 深蓝
    "#B87400", // 9 - 棕
    "#4EAA25", // 10 - 绿
    "#CC342D", // 11 - 红
    "#A33A2A", // 12 - 深红
    "#DC322F", // 13 - 红
    "#2C2C72", // 14 - 深蓝
    "#4A5568", // 15 - 灰
    "#B07219", // 16 - 棕
    "#6B46C1", // 17 - 紫
    "#0EA5E9", // 18 - 天蓝
    "#10B981", // 19 - 翠绿
  ];

  // 语言颜色缓存: 语言名 -> 颜色值，避免同一语言颜色变化
  const langColorCache = {};

  // 获取语言标签背景色: 按首次出现顺序从颜色列表循环取色
  function langColor(lang) {
    if (langColorCache[lang]) {
      return langColorCache[lang];
    }
    const keys = Object.keys(langColorCache);
    const color = LANG_COLORS[keys.length % LANG_COLORS.length];
    langColorCache[lang] = color;
    return color;
  }

  function showLoading(text) {
    loadingText.textContent = text || "处理中...";
    loading.classList.add("show");
  }

  function hideLoading() {
    loading.classList.remove("show");
  }

  let toastTimer;
  function showToast(msg, type) {
    toast.textContent = msg;
    toast.className = "toast show" + (type ? " " + type : "");
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => toast.classList.remove("show"), 2500);
  }

  // 自定义确认对话框 (Wails v2 webview 中原生 confirm() 被屏蔽，需用 DOM 实现)
  // 返回 Promise<boolean>: true=确定, false=取消
  function confirmDialog(message) {
    return new Promise((resolve) => {
      const overlay = document.createElement("div");
      overlay.className = "confirm-overlay";
      overlay.innerHTML =
        '<div class="confirm-dialog">' +
        '<div class="confirm-message">' + escapeHtml(message) + "</div>" +
        '<div class="confirm-buttons">' +
        '<button class="mini-btn" data-action="cancel">取消</button>' +
        '<button class="mini-btn primary" data-action="ok">确定</button>' +
        "</div></div>";
      document.body.appendChild(overlay);
      const close = (result) => {
        document.body.removeChild(overlay);
        resolve(result);
      };
      overlay.querySelector('[data-action="ok"]').addEventListener("click", () => close(true));
      overlay.querySelector('[data-action="cancel"]').addEventListener("click", () => close(false));
      overlay.addEventListener("click", (e) => {
        if (e.target === overlay) close(false);
      });
    });
  }

  async function api(path, body) {
    if (IS_WAILS) {
      const methodMap = {
        "/api/format": "Format",
        "/api/compress": "Compress",
        "/api/highlight": "Highlight",
        "/api/run": "Run",
        "/api/binaries/install": "InstallBinary",
        "/api/binaries/uninstall": "UninstallBinary",
        "/api/binaries/verify": "VerifyBinary",
        "/api/runtime/install": "InstallRuntime",
        "/api/config": "SaveConfig",
        "/api/runtimes/add": "AddRuntime",
        "/api/runtimes/update": "UpdateRuntime",
        "/api/runtimes/delete": "DeleteRuntime",
        "/api/tools/add": "AddTool",
        "/api/tools/update": "UpdateTool",
        "/api/tools/delete": "DeleteTool",
        "/api/languages/add": "AddLanguage",
        "/api/languages/update": "UpdateLanguage",
        "/api/languages/delete": "DeleteLanguage",
        "/api/detect": "Detect",
      };
      // 这些 Wails 绑定方法接收单个 string 参数 (而非对象)，需要从 body 中提取对应字段
      const stringArgMethods = {
        InstallBinary: "name",
        UninstallBinary: "name",
        VerifyBinary: "name",
        InstallRuntime: "name",
        DeleteRuntime: "name",
        DeleteTool: "name",
        DeleteLanguage: "name",
        Detect: "code",
      };
      const method = methodMap[path];
      if (method) {
        try {
          // 对于 string 参数方法，提取对应字段作为纯字符串参数传递
          if (stringArgMethods[method] && body && typeof body === "object") {
            const fieldName = stringArgMethods[method];
            return await wailsCall(method, body[fieldName]);
          }
          return await wailsCall(method, body);
        } catch (e) {
          return { success: false, error: e.message };
        }
      }
    }
    const resp = await fetch(path, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    return resp.json();
  }

  async function apiGet(path) {
    if (IS_WAILS) {
      const methodMap = {
        "/api/languages": "GetLanguages",
        "/api/status": "GetStatus",
        "/api/binaries": "GetBinaries",
        "/api/config": "GetConfig",
      };
      const method = methodMap[path];
      if (method) {
        try {
          return await wailsCall(method);
        } catch (e) {
          return { success: false, error: e.message };
        }
      }
    }
    const resp = await fetch(path);
    return resp.json();
  }

  // outputIsHighlight 跟踪当前输出是否为高亮 HTML (用于复制富文本)
  let outputIsHighlight = false;

  function setOutput(html, isHighlight) {
    const out = $("outputCode");
    outputIsHighlight = false;
    if (!html) {
      out.innerHTML = '<span class="placeholder">结果将显示在这里...</span>';
      return;
    }
    if (isHighlight) {
      out.innerHTML = html;
      outputIsHighlight = true;
    } else {
      out.innerHTML = "<pre></pre>";
      out.querySelector("pre").textContent = html;
    }
  }

  async function execute(action) {
    // 检查操作按钮是否可用 (语言不支持该功能时禁用)
    const btn = document.querySelector(`.action-btn[data-action="${action}"]`);
    if (btn && btn.disabled) {
      showToast("当前语言不支持此操作", "error");
      return;
    }

    const code = $("inputCode").value;
    if (!code.trim()) {
      showToast("请输入代码", "error");
      return;
    }

    const lang = $("langSelect").value;
    const tabWidth = parseInt($("tabWidth").value) || 2;
    // 主题: 工作台快速选项，未设置时回退到配置
    let style = currentConfig.highlight.style || "github";
    const quickStyle = $("quickStyle");
    if (quickStyle) style = quickStyle.value;

    // 字体大小: 工作台快速选项中的临时值 (0=默认), 不保存到配置
    let fontSize = currentConfig.highlight.font_size || 0;
    const quickFontSize = $("quickFontSize");
    if (quickFontSize) fontSize = parseInt(quickFontSize.value, 10) || 0;

    // 行号: 工作台快速选项，元素未加载时回退到配置默认值
    const lnEl = $("lineNumbers");
    const lineNumbers = lnEl ? lnEl.checked : (currentConfig.highlight.line_numbers || false);

    // 兼容输出: 工作台快速选项，与行号处理方式一致 (配置页可修改，工作台临时指定)
    const quickCompat = $("quickCompatHTML");
    const compatHTML = quickCompat ? quickCompat.checked : (currentConfig.highlight.compat_html || false);

    const body = {
      language: lang,
      code: code,
      tab_width: tabWidth,
      style: style,
      font_size: fontSize,
      line_numbers: lineNumbers,
      compat_html: compatHTML,
    };

    showLoading(getLoadingText(action));

    try {
      let endpoint,
        isHighlight = false;
      switch (action) {
        case "format":
          endpoint = "/api/format";
          break;
        case "compress":
          endpoint = "/api/compress";
          break;
        case "highlight":
          endpoint = "/api/highlight";
          isHighlight = true;
          break;
        case "run":
          endpoint = "/api/run";
          body.no_format = false;
          body.no_compress = true;
          body.no_highlight = false;
          isHighlight = true;
          break;
      }
      const result = await api(endpoint, body);
      if (result.success) {
        setOutput(result.result, isHighlight);
        showToast(getSuccessMsg(action), "success");
      } else {
        setOutput("", false);
        showToast(result.error || "处理失败", "error");
      }
    } catch (err) {
      setOutput("", false);
      showToast("网络错误: " + err.message, "error");
    } finally {
      hideLoading();
    }
  }

  function getLoadingText(action) {
    const map = {
      format: "格式化中...",
      compress: "压缩中...",
      highlight: "高亮中...",
      run: "格式化&高亮中...",
    };
    return map[action] || "处理中...";
  }

  function getSuccessMsg(action) {
    const map = {
      format: "格式化完成",
      compress: "压缩完成",
      highlight: "高亮完成",
      run: "格式化&高亮完成",
    };
    return map[action] || "完成";
  }

  // ---- Page Navigation ----
  function initNavigation() {
    document.querySelectorAll(".nav-item").forEach((item) => {
      item.addEventListener("click", () => {
        const page = item.dataset.page;
        document
          .querySelectorAll(".nav-item")
          .forEach((n) => n.classList.toggle("active", n === item));
        document.querySelectorAll(".page").forEach((p) => {
          p.classList.toggle("active", p.dataset.page === page);
        });
        onPageEnter(page);
      });
    });

    // 侧边栏折叠/展开
    const sidebar = $("sidebar");
    const toggleBtn = $("sidebarToggle");
    if (toggleBtn && sidebar) {
      // 恢复上次状态
      try {
        if (localStorage.getItem("sidebarCollapsed") === "1") {
          sidebar.classList.add("collapsed");
        }
      } catch (e) {}
      toggleBtn.addEventListener("click", () => {
        sidebar.classList.toggle("collapsed");
        try {
          localStorage.setItem(
            "sidebarCollapsed",
            sidebar.classList.contains("collapsed") ? "1" : "0",
          );
        } catch (e) {}
      });
    }
  }

  function onPageEnter(page) {
    if (page === "binaries") {
      initManageTabs();
      loadRuntimeList();
    }
  }

  // ---- Language Loading ----
  // 根据配置中的 languages 动态渲染工作台语言下拉列表
  function loadLanguages() {
    const select = $("langSelect");
    if (!select) return;
    // 清除除 auto 外的选项
    select.innerHTML = '<option value="auto">自动检测</option>';

    const langs = currentConfig.languages || {};
    // 按字母排序
    const sortedKeys = Object.keys(langs).sort();
    sortedKeys.forEach((lang) => {
      const cfg = langs[lang] || {};
      const opt = document.createElement("option");
      opt.value = lang;
      let label = lang;
      const parts = [];
      if (cfg.formatter && cfg.formatter.tool) parts.push(cfg.formatter.tool);
      if (cfg.compressor && cfg.compressor.tool) parts.push("压缩");
      parts.push("高亮");
      if (parts.length) label += " (" + parts.join(", ") + ")";
      opt.textContent = label;
      select.appendChild(opt);
    });
  }

  // ---- Status ----
  // 工作台右侧系统状态展示: 运行平台、运行时可用性、工具安装情况
  async function loadStatus() {
    const el = $("statusInfo");
    if (!el) return;
    el.textContent = "加载中...";
    try {
      const data = await apiGet("/api/status");
      if (!data.success) {
        el.innerHTML = '<div class="item"><span class="value err">状态加载失败</span></div>';
        return;
      }
      // 更新版本号显示 (源自 data/version.txt，通过 /api/status 返回)
      if (data.version) {
        const appVer = $("appVersion");
        if (appVer) appVer.textContent = "v" + data.version;
        const aboutTitle = $("aboutTitle");
        if (aboutTitle) aboutTitle.textContent = "Formatter v" + data.version;
        const aboutVer = $("aboutVersion");
        if (aboutVer) aboutVer.textContent = data.version;
      }
      let html = "";
      // 运行平台
      html +=
        '<div class="item"><span class="label">运行平台</span><span class="value">' +
        escapeHtml(data.platform || "-") +
        "</span></div>";
      // 运行时环境
      if (Array.isArray(data.runtimes) && data.runtimes.length > 0) {
        const okList = data.runtimes.filter((rt) => (typeof rt === "object" ? rt.installed : true));
        const missList = data.runtimes.filter((rt) =>
          typeof rt === "object" ? !rt.installed : false,
        );
        const cls = okList.length > 0 ? "ok" : "err";
        const tip = okList
          .map((rt) =>
            typeof rt === "object" ? rt.name + (rt.version ? " " + rt.version : "") : String(rt),
          )
          .join(", ");
        html +=
          '<div class="item"><span class="label">运行时</span><span class="value ' +
          cls +
          '" title="' +
          escapeHtml(tip) +
          '">' +
          okList.length +
          "/" +
          data.runtimes.length +
          " 可用" +
          (missList.length ? " (" + missList.map((r) => r.name).join(", ") + " 缺失)" : "") +
          "</span></div>";
      }
      // 工具安装情况: 从 /api/binaries 获取准确的安装状态 (含系统 PATH 工具)
      try {
        const binResp = await apiGet("/api/binaries");
        if (binResp.success && Array.isArray(binResp.data)) {
          const total = binResp.data.length;
          const installed = binResp.data.filter((b) => b.installed).length;
          if (total > 0) {
            const cls = total - installed === 0 ? "ok" : installed === 0 ? "err" : "";
            const installedNames = binResp.data
              .filter((b) => b.installed)
              .map((b) => b.name)
              .join(", ");
            html +=
              '<div class="item"><span class="label">工具</span><span class="value ' +
              cls +
              '" title="' +
              escapeHtml(installedNames) +
              '">' +
              installed +
              "/" +
              total +
              " 已安装</span></div>";
          }
        }
      } catch (e2) {
        /* 忽略工具统计错误 */
      }
      el.innerHTML = html;
    } catch (e) {
      el.innerHTML =
        '<div class="item"><span class="value err">状态加载失败: ' +
        escapeHtml(e.message) +
        "</span></div>";
    }
  }

  // ---- Binary Management ----
  async function loadBinaries() {
    const listEl = $("binList");
    if (!listEl) return;
    listEl.textContent = "加载中...";

    try {
      const resp = await apiGet("/api/binaries");
      if (!resp.success) {
        listEl.textContent = "加载失败: " + (resp.error || "未知错误");
        return;
      }
      const data = resp.data || [];

      // 工具列表 + 汇总 (按 config_source 配置来源类型统计)
      let dlCount = 0,
        presetCount = 0,
        installCount = 0,
        missingCount = 0;
      data.forEach((bin) => {
        if (bin.config_source === "preset") presetCount++;
        else if (bin.config_source === "install") installCount++;
        else dlCount++;
        if (!bin.installed) missingCount++;
      });

      let html =
        '<div class="bin-summary">' +
        '<span class="bs-item">共 <b>' +
        data.length +
        "</b></span>" +
        '<span class="bs-item bs-ok">下载 <b>' +
        dlCount +
        "</b></span>" +
        '<span class="bs-item bs-preset">预置 <b>' +
        presetCount +
        "</b></span>" +
        '<span class="bs-item bs-sys">安装 <b>' +
        installCount +
        "</b></span>" +
        '<span class="bs-item bs-miss">未安装 <b>' +
        missingCount +
        "</b></span>" +
        "</div>";
      html +=
        '<div class="cfg-table-wrap"><table class="cfg-table">' +
        "<thead><tr>" +
        '<th>名称</th><th>版本</th><th>语言</th><th>运行时</th><th>来源</th><th>状态</th><th class="col-actions">操作</th>' +
        "</tr></thead><tbody>";
      data.forEach((bin) => {
        html += renderBinRow(bin);
      });
      html += "</tbody></table></div>";
      listEl.innerHTML = html;

      listEl.querySelectorAll("tr.bin-row").forEach((row) => {
        const name = row.dataset.name;
        row.querySelector(".btn-install")?.addEventListener("click", () => installBinary(name));
        row.querySelector(".btn-uninstall")?.addEventListener("click", async () => {
          if (await confirmDialog("确定卸载 " + name + "?")) uninstallBinary(name);
        });
        row.querySelector(".btn-verify")?.addEventListener("click", () => verifyBinary(name));
        row
          .querySelector('.edit-btn[data-type="tool"]')
          ?.addEventListener("click", () => openToolModal(name));
        row
          .querySelector('.del-btn[data-type="tool"]')
          ?.addEventListener("click", () => deleteCRUD("tool", name));
      });
    } catch (e) {
      listEl.textContent = "加载失败: " + e.message;
    }
  }

  // 安装运行时环境 (调用后端从配置读取安装命令并执行)
  async function installRuntime(name) {
    showToast("正在安装 " + name + " 运行时...", "info");
    try {
      const result = await api("/api/runtime/install", { name: name });
      if (result.success) {
        showToast(name + " 运行时安装成功", "success");
        // 重新加载二进制列表与运行时检测
        loadBinaries();
      } else {
        showToast(name + " 安装失败: " + (result.error || "未知错误"), "error");
      }
    } catch (e) {
      showToast("安装失败: " + e.message, "error");
    }
  }

  // 转义 HTML 防注入
  function escapeHtml(s) {
    return String(s).replace(
      /[&<>"']/g,
      (c) =>
        ({
          "&": "&amp;",
          "<": "&lt;",
          ">": "&gt;",
          '"': "&quot;",
          "'": "&#39;",
        })[c],
    );
  }

  function renderBinRow(bin) {
    // 状态分类
    let statusClass, statusText;
    if (bin.installed) {
      statusClass = "installed";
      statusText = "已安装";
    } else if (bin.runtime && !bin.runtime_ready) {
      statusClass = "blocked";
      statusText = "缺运行时";
    } else if (bin.runtime) {
      statusClass = "runtime";
      statusText = "可安装";
    } else {
      statusClass = "missing";
      statusText = "未安装";
    }

    // 来源标签 (基于 config_source 配置来源类型)
    let srcTag = "";
    if (bin.custom_path) {
      srcTag =
        '<span class="src-tag src-custom" title="用户指定: ' +
        escapeHtml(bin.path || "") +
        '">自定义</span>';
    } else if (bin.config_source === "preset") {
      srcTag = '<span class="src-tag src-preset" title="预置工具，不可修改">预置</span>';
    } else if (bin.config_source === "download") {
      srcTag = '<span class="src-tag src-download" title="下载工具，打包进 App">下载</span>';
    } else if (bin.config_source === "install") {
      srcTag = '<span class="src-tag src-install" title="命令安装工具，不打包进 App">安装</span>';
    } else {
      srcTag = '<span class="src-tag src-missing">未知</span>';
    }

    // 运行时标签
    const rtTag = bin.runtime
      ? '<span class="rt-tag rt-dep" title="运行时: ' +
        escapeHtml(bin.runtime) +
        '">' +
        escapeHtml(bin.runtime) +
        (bin.runtime_ready ? "" : "!") +
        "</span>"
      : '<span class="rt-tag rt-standalone" title="独立二进制，无需运行时">独立</span>';

    // 操作按钮
    let btns = "";
    if (!bin.editable) {
      if (bin.installed) {
        btns =
          '<button class="mini-btn btn-verify" data-name="' +
          escapeHtml(bin.name) +
          '">验证</button>';
      } else {
        btns = '<span class="cell-muted">内置缺失</span>';
      }
    } else if (bin.installed) {
      if (bin.source === "system" || bin.custom_path) {
        btns =
          '<button class="mini-btn btn-verify" data-name="' +
          escapeHtml(bin.name) +
          '">验证</button><button class="mini-btn edit-btn" data-type="tool" data-name="' +
          escapeHtml(bin.name) +
          '">编辑</button><button class="mini-btn danger del-btn" data-type="tool" data-name="' +
          escapeHtml(bin.name) +
          '">删除</button>';
      } else {
        btns =
          '<button class="mini-btn btn-verify" data-name="' +
          escapeHtml(bin.name) +
          '">验证</button><button class="mini-btn btn-uninstall" data-name="' +
          escapeHtml(bin.name) +
          '">卸载</button><button class="mini-btn edit-btn" data-type="tool" data-name="' +
          escapeHtml(bin.name) +
          '">编辑</button><button class="mini-btn danger del-btn" data-type="tool" data-name="' +
          escapeHtml(bin.name) +
          '">删除</button>';
      }
    } else if (bin.runtime && !bin.runtime_ready) {
      btns =
        '<button class="mini-btn btn-install" data-name="' +
        escapeHtml(bin.name) +
        '" disabled title="运行时未安装，请先安装 ' +
        escapeHtml(bin.runtime) +
        ' 运行时">安装</button><button class="mini-btn edit-btn" data-type="tool" data-name="' +
        escapeHtml(bin.name) +
        '">编辑</button><button class="mini-btn danger del-btn" data-type="tool" data-name="' +
        escapeHtml(bin.name) +
        '">删除</button>';
    } else {
      btns =
        '<button class="mini-btn primary btn-install" data-name="' +
        escapeHtml(bin.name) +
        '">安装</button><button class="mini-btn edit-btn" data-type="tool" data-name="' +
        escapeHtml(bin.name) +
        '">编辑</button><button class="mini-btn danger del-btn" data-type="tool" data-name="' +
        escapeHtml(bin.name) +
        '">删除</button>';
    }

    // 兼容旧版 language (string) 与新版 languages (array)
    const langDisplay = Array.isArray(bin.languages)
      ? bin.languages.join(", ")
      : bin.language || "";
    const sizeStr = bin.size ? formatSize(bin.size) : "";
    const verText = bin.version
      ? "v" + escapeHtml(bin.version)
      : '<span class="cell-muted">—</span>';

    // 工具名标签背景色: 按配置来源类型着色
    let nameBg = "#5B6B7D"; // 默认灰色
    if (bin.custom_path)
      nameBg = "#6B46C1"; // 自定义 - 紫色
    else if (bin.config_source === "preset")
      nameBg = "#4EAA25"; // 预置 - 绿色
    else if (bin.config_source === "download")
      nameBg = "#3776AB"; // 下载 - 蓝色
    else if (bin.config_source === "install")
      nameBg = "#00ADD8"; // 安装 - 青色
    else if (statusClass === "missing") nameBg = "#B87400"; // 未安装 - 棕色

    return (
      '<tr class="bin-row" data-name="' +
      escapeHtml(bin.name) +
      '">' +
      '<td><span class="lang-tag" style="background:' +
      nameBg +
      '">' +
      escapeHtml(bin.name) +
      "</span>" +
      (sizeStr
        ? '<br><span class="cell-muted" style="font-size:10px">' + sizeStr + "</span>"
        : "") +
      "</td>" +
      '<td class="cell-muted">' +
      verText +
      "</td>" +
      '<td class="cell-muted">' +
      escapeHtml(langDisplay) +
      "</td>" +
      "<td>" +
      rtTag +
      "</td>" +
      "<td>" +
      srcTag +
      "</td>" +
      '<td><span class="status-badge ' +
      statusClass +
      '">' +
      statusText +
      "</span></td>" +
      '<td class="col-actions">' +
      btns +
      "</td>" +
      "</tr>"
    );
  }

  function formatSize(bytes) {
    if (bytes < 1024) return bytes + "B";
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + "K";
    return (bytes / 1024 / 1024).toFixed(1) + "M";
  }

  async function installBinary(name) {
    showLoading("安装 " + name + "...");
    try {
      const result = await api("/api/binaries/install", { name: name });
      showToast(
        result.success ? result.result : result.error || "安装失败",
        result.success ? "success" : "error",
      );
    } catch (err) {
      showToast("网络错误: " + err.message, "error");
    } finally {
      hideLoading();
      loadBinaries();
    }
  }

  async function uninstallBinary(name) {
    showLoading("卸载 " + name + "...");
    try {
      const result = await api("/api/binaries/uninstall", { name: name });
      showToast(
        result.success ? result.result : result.error || "卸载失败",
        result.success ? "success" : "error",
      );
    } catch (err) {
      showToast("网络错误: " + err.message, "error");
    } finally {
      hideLoading();
      loadBinaries();
    }
  }

  async function verifyBinary(name) {
    showLoading("验证 " + name + "...");
    try {
      const result = await api("/api/binaries/verify", { name: name });
      showToast(
        name + ": " + (result.success ? result.result : result.error || "验证失败"),
        result.success ? "success" : "error",
      );
    } catch (err) {
      showToast("网络错误: " + err.message, "error");
    } finally {
      hideLoading();
    }
  }

  async function installAllBinaries() {
    const resp = await apiGet("/api/binaries");
    if (!resp.success) return;
    const toInstall = resp.data.filter((b) => !b.installed);
    if (toInstall.length === 0) {
      showToast("没有可安装的工具", "error");
      return;
    }
    if (!(await confirmDialog("将安装 " + toInstall.length + " 个工具，确定继续?"))) return;
    showLoading("批量安装中...");
    let success = 0,
      fail = 0;
    for (const bin of toInstall) {
      try {
        const result = await api("/api/binaries/install", { name: bin.name });
        if (result.success) success++;
        else fail++;
      } catch (e) {
        fail++;
      }
    }
    hideLoading();
    showToast("完成: 成功 " + success + ", 失败 " + fail, success > 0 ? "success" : "error");
    loadBinaries();
    loadStatus();
  }

  // ---- Config Loading & Saving ----
  let currentConfig = {
    format: { tab_width: 2, line_ending: "\n" },
    highlight: { style: "github", font_size: 14, line_numbers: false, compat_html: false },
    binary: { runtimes: [] },
  };

  async function loadConfig() {
    try {
      const resp = await apiGet("/api/config");
      if (resp.success && resp.data) {
        Object.assign(currentConfig.format, resp.data.format || {});
        Object.assign(currentConfig.highlight, resp.data.highlight || {});
        if (resp.data.languages) {
          currentConfig.languages = resp.data.languages;
        }
        // 加载运行时环境配置
        if (resp.data.binary) {
          currentConfig.binary = resp.data.binary;
          if (!currentConfig.binary.runtimes) currentConfig.binary.runtimes = [];
        }
        applyConfigToUI();
        // 配置加载完成后刷新语言列表 (工作台下拉依赖配置)
        loadLanguages();
      }
    } catch (e) {
      console.warn("加载配置失败，使用默认值", e);
    }
  }

  async function saveConfig() {
    try {
      const result = await api("/api/config", currentConfig);
      if (result.success) {
        showToast("配置已保存", "success");
      } else {
        showToast(result.error || "保存失败", "error");
      }
    } catch (e) {
      showToast("保存失败: " + e.message, "error");
    }
  }

  function applyConfigToUI() {
    const cfs = $("configFontSize");
    const fsVal = currentConfig.highlight.font_size || 14;
    if (cfs) cfs.value = fsVal;
    const fsv = $("fontSizeValue");
    if (fsv) fsv.textContent = fsVal + "px";
    const cln = $("configLineNumbers");
    if (cln) cln.checked = currentConfig.highlight.line_numbers || false;
    const cch = $("configCompatHTML");
    if (cch) cch.checked = currentConfig.highlight.compat_html || false;
    const ss = $("styleSelect");
    if (ss && currentConfig.highlight.style) {
      ss.value = currentConfig.highlight.style;
      // 直接调用 updateThemePreview 刷新预览，不触发 change 事件以免误调 saveConfig
      updateThemePreview();
    }
    const qs = $("quickStyle");
    if (qs && currentConfig.highlight.style) {
      qs.value = currentConfig.highlight.style;
    }
    // 工作台快速字体大小: 默认取配置值，0 表示使用配置默认
    const qfs = $("quickFontSize");
    if (qfs) qfs.value = String(fsVal);
    // 快速选项和操作按钮根据当前选中语言初始化
    const lang = $("langSelect")?.value || "";
    updateQuickOptionsForLanguage(lang);
    updateButtonStates(lang);
  }

  // 根据语言更新快速选项 (缩进方式: -1=Tab, >0=空格宽度)
  // 优先级与数据处理一致: 语言级 indent 配置 > 全局 format 配置
  // 行号、兼容输出始终取高亮配置 (不随语言变化)
  function updateQuickOptionsForLanguage(lang) {
    let tabWidth = currentConfig.format.tab_width;
    if (tabWidth === undefined || tabWidth === 0) tabWidth = 2;

    // 语言级配置覆盖全局
    if (lang && lang !== "auto") {
      const langCfg = (currentConfig.languages || {})[lang];
      if (langCfg && langCfg.indent && langCfg.indent.tab_width !== undefined) {
        tabWidth = langCfg.indent.tab_width;
      }
    }

    const tw = $("tabWidth");
    if (tw) tw.value = String(tabWidth);
    const ln = $("lineNumbers");
    if (ln) ln.checked = currentConfig.highlight.line_numbers || false;
    const qch = $("quickCompatHTML");
    if (qch) qch.checked = currentConfig.highlight.compat_html || false;
  }

  // 根据语言能力更新操作按钮状态 (仅当支持时才可点击)
  // auto 语言: 全部启用 (语言未知); 具体语言: 按 formatter/compressor 配置决定
  function updateButtonStates(lang) {
    const hasFormatter =
      !lang || lang === "auto" || (currentConfig.languages || {})[lang]?.formatter;
    const hasCompressor =
      !lang || lang === "auto" || (currentConfig.languages || {})[lang]?.compressor;

    document.querySelectorAll(".action-btn").forEach((btn) => {
      const action = btn.dataset.action;
      let enabled = true;
      switch (action) {
        case "format":
          enabled = hasFormatter;
          break;
        case "compress":
          enabled = hasCompressor;
          break;
        case "highlight":
          enabled = true;
          break; // 所有语言均支持高亮
        case "run":
          enabled = hasFormatter;
          break; // run = 格式化 + 高亮
      }
      btn.disabled = !enabled;
    });
  }

  // 自动检测语言并更新 UI (语言下拉、快速选项、操作按钮状态)
  async function detectLanguage() {
    const code = $("inputCode").value;
    if (!code.trim()) {
      showToast("请输入代码后再检测", "error");
      return;
    }
    try {
      const result = await api("/api/detect", { code: code });
      if (result.success) {
        const lang = result.language || "";
        const langSelect = $("langSelect");
        const known = lang && [...langSelect.options].some((opt) => opt.value === lang);
        if (known) {
          langSelect.value = lang;
          // 触发 change 事件以同步快速选项和操作按钮状态
          langSelect.dispatchEvent(new Event("change"));
          showToast("已检测到语言: " + lang, "success");
        } else if (lang) {
          showToast("检测到语言: " + lang + " (当前未配置)", "warning");
        } else {
          showToast("未能识别代码语言", "warning");
        }
      } else {
        showToast("语言检测失败: " + (result.error || "未知错误"), "error");
      }
    } catch (e) {
      showToast("检测请求失败: " + e.message, "error");
    }
  }

  // ---- Highlight Config (in 配置管理 > 高亮配置 tab) ----
  function updateThemePreview() {
    const sel = $("styleSelect");
    if (!sel) return;
    const style = sel.value;
    const fontSize = parseInt($("configFontSize")?.value, 10) || 0;
    const lineNumbers = $("configLineNumbers")?.checked || false;
    const compatHTML = $("configCompatHTML")?.checked || false;
    const code = 'function hello() {\n  const x = "Hello World";\n  return x;\n}';
    api("/api/highlight", {
      language: "javascript",
      code: code,
      style: style,
      font_size: fontSize,
      line_numbers: lineNumbers,
      compat_html: compatHTML,
    })
      .then((result) => {
        if (result.success) {
          $("themePreview").innerHTML = result.result;
        }
      })
      .catch(() => {});
  }

  function initHighlightConfig() {
    const sel = $("styleSelect");
    if (!sel) return;
    sel.addEventListener("change", () => {
      currentConfig.highlight.style = sel.value;
      const qs = $("quickStyle");
      if (qs) qs.value = sel.value;
      updateThemePreview();
      saveConfig();
    });
    $("configLineNumbers")?.addEventListener("change", (e) => {
      currentConfig.highlight.line_numbers = e.target.checked;
      updateThemePreview();
      saveConfig();
    });
    $("configCompatHTML")?.addEventListener("change", (e) => {
      currentConfig.highlight.compat_html = e.target.checked;
      updateThemePreview();
      saveConfig();
    });
    $("configFontSize")?.addEventListener("input", (e) => {
      // 拖动滑块时实时更新数值显示
      const v = parseInt(e.target.value, 10);
      const fsv = $("fontSizeValue");
      if (fsv) fsv.textContent = (isNaN(v) ? 14 : v) + "px";
    });
    $("configFontSize")?.addEventListener("change", (e) => {
      // 释放滑块时保存配置并刷新预览
      const v = parseInt(e.target.value, 10);
      const fs = isNaN(v) ? 14 : v;
      currentConfig.highlight.font_size = fs;
      updateThemePreview();
      saveConfig();
    });
    $("applyStyleBtn")?.addEventListener("click", () => {
      showToast("主题已应用", "success");
    });
    $("resetStyleBtn")?.addEventListener("click", () => {
      const ss = $("styleSelect");
      if (ss) ss.value = "github";
      const cln = $("configLineNumbers");
      if (cln) cln.checked = false;
      const cfs = $("configFontSize");
      if (cfs) cfs.value = 14;
      const fsv = $("fontSizeValue");
      if (fsv) fsv.textContent = "14px";
      const cch = $("configCompatHTML");
      if (cch) cch.checked = false;
      currentConfig.highlight.style = "github";
      currentConfig.highlight.font_size = 14;
      currentConfig.highlight.line_numbers = false;
      currentConfig.highlight.compat_html = false;
      const qs = $("quickStyle");
      if (qs) qs.value = "github";
      updateThemePreview();
      saveConfig();
      showToast("已重置为默认主题", "success");
    });
  }

  // ---- Workbench Quick Style Dropdown ----
  // 工作台快速选项中的高亮主题下拉框，选项从配置页 styleSelect 克隆以避免重复维护。
  // 与 styleSelect 双向同步: 任一变更均更新 currentConfig 并持久化。
  function initQuickStyle() {
    const quick = $("quickStyle");
    const source = $("styleSelect");
    if (!quick) return;
    // 从 styleSelect 克隆选项 (主题列表唯一维护点)
    if (source) quick.innerHTML = source.innerHTML;
    // 初始值: 取配置中的高亮主题
    quick.value = currentConfig.highlight.style || "github";
    // 变更时同步到 styleSelect、currentConfig 并保存
    quick.addEventListener("change", () => {
      currentConfig.highlight.style = quick.value;
      if (source) source.value = quick.value;
      saveConfig();
    });
  }

  // ---- Manage Page: Tab Switching ----
  // manageTabsInited 防止重复绑定: initManageTabs 每次进入配置管理页都会调用，
  // 但 .manage-tab 元素是静态 HTML，只需绑定一次事件监听器
  let manageTabsInited = false;
  function initManageTabs() {
    if (manageTabsInited) return;
    manageTabsInited = true;
    document.querySelectorAll(".manage-tab").forEach((tab) => {
      tab.addEventListener("click", () => {
        const target = tab.dataset.tab;
        document
          .querySelectorAll(".manage-tab")
          .forEach((t) => t.classList.toggle("active", t === tab));
        document.querySelectorAll(".tab-panel").forEach((p) => {
          p.classList.toggle("active", p.dataset.tab === target);
        });
        if (target === "runtimes") loadRuntimeList();
        if (target === "tools") loadBinaries();
        if (target === "languages") loadLanguageList();
        if (target === "highlight") {
          // 切换到高亮配置 tab 时触发预览 (直接调用，不触发 change 事件以免误调 saveConfig)
          updateThemePreview();
        }
      });
    });
  }

  // ---- Modal Management ----
  let modalSaveHandler = null;

  function openModal(title, bodyHtml, onSave) {
    $("modalTitle").textContent = title;
    $("modalBody").innerHTML = bodyHtml;
    modalSaveHandler = onSave;
    $("modalOverlay").classList.add("show");
  }

  function closeModal() {
    $("modalOverlay").classList.remove("show");
    modalSaveHandler = null;
  }

  // ---- 通用 CRUD (运行时/工具/语言 共用保存与删除逻辑) ----
  // CRUD_CONFIG 映射各类型的 endpoint 基路径、显示标签和操作后的刷新回调
  const CRUD_CONFIG = {
    runtime: { base: "/api/runtimes", label: "运行时", reload: () => loadRuntimeList() },
    tool: { base: "/api/tools", label: "工具", reload: () => loadBinaries() },
    language: {
      base: "/api/languages",
      label: "语言",
      reload: () => {
        loadLanguageList();
        loadLanguages();
      },
    },
  };

  // saveCRUD 通用保存 (添加/更新)，type 见 CRUD_CONFIG
  async function saveCRUD(type, data, isEdit) {
    const cfg = CRUD_CONFIG[type];
    const endpoint = isEdit ? cfg.base + "/update" : cfg.base + "/add";
    try {
      const result = await api(endpoint, data);
      if (result.success) {
        showToast(
          result.result || (isEdit ? cfg.label + "已更新" : cfg.label + "已添加"),
          "success",
        );
        closeModal();
        await loadConfig();
        cfg.reload();
      } else {
        showToast(result.error || "操作失败", "error");
      }
    } catch (e) {
      showToast("操作失败: " + e.message, "error");
    }
  }

  // deleteCRUD 通用删除
  async function deleteCRUD(type, name) {
    const cfg = CRUD_CONFIG[type];
    const suffix = type === "language" ? " 及其格式化/压缩配置" : "";
    if (!(await confirmDialog("确定删除" + cfg.label + " " + name + suffix + "?"))) return;
    try {
      const result = await api(cfg.base + "/delete", { name: name });
      if (result.success) {
        showToast(cfg.label + "已删除", "success");
        await loadConfig();
        cfg.reload();
      } else {
        showToast(result.error || "删除失败", "error");
      }
    } catch (e) {
      showToast("删除失败: " + e.message, "error");
    }
  }

  // ---- Runtime CRUD ----

  async function loadRuntimeList() {
    const el = $("runtimeList");
    if (!el) return;
    el.textContent = "加载中...";

    // 确保配置已加载
    if (!currentConfig.binary || !currentConfig.binary.runtimes) {
      try {
        await loadConfig();
      } catch (e) {}
    }
    const runtimes = (currentConfig.binary && currentConfig.binary.runtimes) || [];

    // 获取运行时安装状态
    let statusMap = {};
    try {
      const statusResp = await apiGet("/api/status");
      if (statusResp.success && Array.isArray(statusResp.runtimes)) {
        statusResp.runtimes.forEach((rt) => {
          if (rt && rt.name) statusMap[rt.name] = rt;
        });
      }
    } catch (e) {}

    const countEl = $("runtimeCount");
    if (countEl) countEl.textContent = "共 " + runtimes.length + " 个";

    if (runtimes.length === 0) {
      el.innerHTML = '<div class="manage-empty">暂无运行时环境，点击「添加运行时」创建</div>';
      return;
    }

    let html =
      '<div class="cfg-table-wrap"><table class="cfg-table">' +
      "<thead><tr>" +
      '<th>名称</th><th>可执行文件</th><th>版本</th><th>路径</th><th>状态</th><th class="col-actions">操作</th>' +
      "</tr></thead><tbody>";
    runtimes.forEach((rt) => {
      const name = rt.name || "?";
      const exe = rt.exe || name;
      const path = rt.path || "";
      const status = statusMap[name] || {};
      const installed = status.installed;
      const version = status.version || "";
      const statusCls = installed ? "installed" : "missing";
      const statusText = installed ? "✓ 可用" : "✗ 未安装";
      const pathText = path ? escapeHtml(path) : '<span class="cell-muted">系统 PATH</span>';
      const verText = version ? escapeHtml(version) : '<span class="cell-muted">—</span>';
      const installBtn =
        !installed && status.installable
          ? '<button class="mini-btn primary rt-install" data-name="' +
            escapeHtml(name) +
            '">安装</button>'
          : "";

      html +=
        "<tr>" +
        '<td class="cell-name">' +
        escapeHtml(name) +
        "</td>" +
        '<td class="cell-mono">' +
        escapeHtml(exe) +
        "</td>" +
        '<td class="cell-muted">' +
        verText +
        "</td>" +
        '<td class="cell-mono" title="' +
        escapeHtml(path) +
        '" style="max-width:200px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">' +
        pathText +
        "</td>" +
        '<td><span class="status-badge ' +
        statusCls +
        '">' +
        statusText +
        "</span></td>" +
        '<td class="col-actions">' +
        installBtn +
        '<button class="mini-btn edit-btn" data-type="runtime" data-name="' +
        escapeHtml(name) +
        '">编辑</button>' +
        '<button class="mini-btn danger del-btn" data-type="runtime" data-name="' +
        escapeHtml(name) +
        '">删除</button>' +
        "</td>" +
        "</tr>";
    });
    html += "</tbody></table></div>";
    el.innerHTML = html;

    // 绑定按钮事件
    el.querySelectorAll(".rt-install").forEach((btn) => {
      btn.addEventListener("click", () => installRuntime(btn.dataset.name));
    });
    el.querySelectorAll('.edit-btn[data-type="runtime"]').forEach((btn) => {
      btn.addEventListener("click", () => openRuntimeModal(btn.dataset.name));
    });
    el.querySelectorAll('.del-btn[data-type="runtime"]').forEach((btn) => {
      btn.addEventListener("click", () => deleteCRUD("runtime", btn.dataset.name));
    });
  }

  // formatInstallCmds 将 install_cmds 对象格式化为 textarea 文本 (每行: 平台 命令)
  function formatInstallCmds(cmds) {
    if (!cmds) return "";
    return Object.entries(cmds)
      .map(([k, v]) => k + " " + (Array.isArray(v) ? v.join(" ") : v))
      .join("\n");
  }

  // parseInstallCmds 将 textarea 文本解析为 install_cmds 对象
  function parseInstallCmds(text) {
    if (!text.trim()) return null;
    const cmds = {};
    text.split("\n").forEach((line) => {
      const parts = line.trim().split(/\s+/);
      if (parts.length >= 2) cmds[parts[0]] = parts.slice(1);
    });
    return Object.keys(cmds).length > 0 ? cmds : null;
  }

  function openRuntimeModal(name) {
    const isEdit = !!name;
    const runtimes = (currentConfig.binary && currentConfig.binary.runtimes) || [];
    const rt = isEdit ? runtimes.find((r) => r.name === name) || {} : {};

    const html =
      '<div class="form-row"><label>名称</label><input type="text" id="rt_name" value="' +
      escapeHtml(rt.name || "") +
      '" placeholder="如 node / python"' +
      (isEdit ? " disabled" : "") +
      "></div>" +
      '<div class="form-row"><label>可执行文件</label><input type="text" id="rt_exe" value="' +
      escapeHtml(rt.exe || "") +
      '" placeholder="如 node / python3"></div>' +
      '<div class="form-row"><label>自定义路径</label><input type="text" id="rt_path" value="' +
      escapeHtml(rt.path || "") +
      '" placeholder="留空使用系统 PATH"></div>' +
      '<div class="form-row"><label>版本检测命令</label><input type="text" id="rt_version_cmd" value="' +
      escapeHtml((rt.version_cmd || []).join(" ")) +
      '" placeholder="如 --version"></div>' +
      '<div class="form-row"><label>备选可执行文件</label><input type="text" id="rt_detect_exes" value="' +
      escapeHtml((rt.detect_exes || []).join(", ")) +
      '" placeholder="逗号分隔，如 python3, python"></div>' +
      '<div class="form-row"><label>安装命令</label><textarea id="rt_install_cmds" rows="3" placeholder="每行一个平台命令，格式: 平台 命令 参数&#10;如: darwin brew install node&#10;linux apt-get install -y nodejs">' +
      escapeHtml(formatInstallCmds(rt.install_cmds)) +
      "</textarea></div>";

    openModal(isEdit ? "编辑运行时" : "添加运行时", html, async () => {
      const data = {
        name: $("rt_name").value.trim(),
        exe: $("rt_exe").value.trim(),
        path: $("rt_path").value.trim(),
      };
      const vcmd = $("rt_version_cmd").value.trim();
      if (vcmd) data.version_cmd = vcmd.split(/\s+/);
      const dexes = $("rt_detect_exes").value.trim();
      if (dexes) data.detect_exes = dexes.split(/,\s*/);
      const installCmds = parseInstallCmds($("rt_install_cmds").value);
      if (installCmds) data.install_cmds = installCmds;
      if (!data.name) {
        showToast("名称不能为空", "error");
        return;
      }
      if (!data.exe) data.exe = data.name;
      await saveCRUD("runtime", data, isEdit);
    });
  }

  // ---- Tool CRUD (Add/Edit/Delete) ----

  function openToolModal(name) {
    const isEdit = !!name;
    const tools = (currentConfig.binary && currentConfig.binary.tools) || [];
    const tool = isEdit ? tools.find((t) => t.name === name) || {} : {};

    let urlsText = "";
    if (tool.urls) {
      urlsText = Object.entries(tool.urls)
        .map(([k, v]) => k + " " + v)
        .join("\n");
    }

    const html =
      '<div class="form-row"><label>名称</label><input type="text" id="tl_name" value="' +
      escapeHtml(tool.name || "") +
      '" placeholder="如 oxfmt"' +
      (isEdit ? " disabled" : "") +
      "></div>" +
      '<div class="form-row"><label>版本</label><input type="text" id="tl_version" value="' +
      escapeHtml(tool.version || "") +
      '" placeholder="如 2.5.4"></div>' +
      '<div class="form-row"><label>支持语言</label><input type="text" id="tl_languages" value="' +
      escapeHtml((tool.languages || []).join(", ")) +
      '" placeholder="逗号分隔，如 javascript, typescript"></div>' +
      '<div class="form-row"><label>运行时</label><input type="text" id="tl_runtime" value="' +
      escapeHtml(tool.runtime || "") +
      '" placeholder="留空=独立二进制，或填 node/python/java/ruby"></div>' +
      '<div class="form-row"><label>可执行文件名</label><input type="text" id="tl_executable" value="' +
      escapeHtml(tool.executable || "") +
      '" placeholder="如 oxfmt"></div>' +
      '<div class="form-row"><label>来源</label><select id="tl_source"><option value="download"' +
      (tool.source === "download" || !tool.source ? " selected" : "") +
      '>download (下载)</option><option value="preset"' +
      (tool.source === "preset" ? " selected" : "") +
      '>preset (预置)</option><option value="install"' +
      (tool.source === "install" ? " selected" : "") +
      ">install (安装)</option></select></div>" +
      '<div class="form-row"><label>归档类型</label><select id="tl_archive"><option value="raw"' +
      (tool.archive === "raw" || !tool.archive ? " selected" : "") +
      '>raw</option><option value="tar.gz"' +
      (tool.archive === "tar.gz" ? " selected" : "") +
      '>tar.gz</option><option value="zip"' +
      (tool.archive === "zip" ? " selected" : "") +
      '>zip</option><option value="tar.xz"' +
      (tool.archive === "tar.xz" ? " selected" : "") +
      ">tar.xz</option></select></div>" +
      '<div class="form-row"><label>自定义路径</label><input type="text" id="tl_path" value="' +
      escapeHtml(tool.path || "") +
      '" placeholder="留空=自动下载/查找"></div>' +
      '<div class="form-row"><label>验证命令</label><input type="text" id="tl_verify_cmd" value="' +
      escapeHtml(tool.verify_cmd || "") +
      '" placeholder="如 {exe} --version"></div>' +
      '<div class="form-row"><label>运行命令</label><input type="text" id="tl_run_cmd" value="' +
      escapeHtml(tool.run_cmd || "") +
      '" placeholder="如 {runtime} -jar {exe}，留空=默认"></div>' +
      '<div class="form-row"><label>下载地址</label><textarea id="tl_urls" rows="4" placeholder="每行一个，格式: 平台 URL&#10;如: darwin/amd64 https://github.com/...&#10;支持 {version} 占位符">' +
      escapeHtml(urlsText) +
      "</textarea></div>" +
      '<div class="form-row"><label>安装命令</label><textarea id="tl_install_cmds" rows="3" placeholder="source=install 时使用，每行: 平台 命令 参数&#10;如: darwin gem install rubocop --version 1.75.0&#10;支持 {version} 占位符">' +
      escapeHtml(formatInstallCmds(tool.install_cmds)) +
      "</textarea></div>";

    openModal(isEdit ? "编辑工具" : "添加工具", html, async () => {
      const data = {
        name: $("tl_name").value.trim(),
        version: $("tl_version").value.trim(),
        runtime: $("tl_runtime").value.trim(),
        executable: $("tl_executable").value.trim(),
        source: $("tl_source").value,
        archive: $("tl_archive").value,
        path: $("tl_path").value.trim(),
        verify_cmd: $("tl_verify_cmd").value.trim(),
        run_cmd: $("tl_run_cmd").value.trim(),
      };
      const langs = $("tl_languages").value.trim();
      if (langs) data.languages = langs.split(/,\s*/);
      // 解析 URLs
      const urlsText2 = $("tl_urls").value.trim();
      if (urlsText2) {
        const urls = {};
        urlsText2.split("\n").forEach((line) => {
          const parts = line.trim().split(/\s+/);
          if (parts.length >= 2) urls[parts[0]] = parts.slice(1).join(" ");
        });
        if (Object.keys(urls).length > 0) data.urls = urls;
      }
      // 解析安装命令 (source=install 时生效)
      const installCmds = parseInstallCmds($("tl_install_cmds").value);
      if (installCmds) data.install_cmds = installCmds;
      if (!data.name) {
        showToast("名称不能为空", "error");
        return;
      }
      if (!data.executable) data.executable = data.name;
      await saveCRUD("tool", data, isEdit);
    });
  }

  // ---- Language CRUD ----

  async function loadLanguageList() {
    const el = $("languageList");
    if (!el) return;
    el.textContent = "加载中...";

    if (!currentConfig.languages) {
      try {
        await loadConfig();
      } catch (e) {}
    }
    const langs = currentConfig.languages || {};
    const langKeys = Object.keys(langs).sort();

    const countEl = $("languageCount");
    if (countEl) countEl.textContent = "共 " + langKeys.length + " 种";

    if (langKeys.length === 0) {
      el.innerHTML = '<div class="manage-empty">暂无语言配置，点击「添加语言」创建</div>';
      return;
    }

    // 预填充颜色
    langKeys.forEach((lang) => langColor(lang));

    let html =
      '<div class="cfg-table-wrap"><table class="cfg-table">' +
      "<thead><tr>" +
      '<th>语言</th><th>格式化器</th><th>压缩器</th><th>缩进</th><th>扩展名</th><th class="col-actions">操作</th>' +
      "</tr></thead><tbody>";
    langKeys.forEach((lang) => {
      const cfg = langs[lang] || {};
      const fmt = cfg.formatter || {};
      const cmp = cfg.compressor || null;
      const indent = cfg.indent || null;
      const detection = cfg.detection || null;
      const color = langColor(lang);
      const fmtStr = fmt.tool
        ? backendTag(fmt.backend) + " " + escapeHtml(fmt.tool)
        : '<span class="cell-muted">无</span>';
      const cmpStr =
        cmp && cmp.tool
          ? backendTag(cmp.backend) + " " + escapeHtml(cmp.tool)
          : '<span class="cell-muted">无</span>';
      const indentStr = indent
        ? (indent.tab_width === -1 ? "Tab" : "空格×" + indent.tab_width)
        : '<span class="cell-muted">默认</span>';
      const detExts =
        detection && detection.extensions && detection.extensions.length > 0
          ? detection.extensions.join(", ")
          : '<span class="cell-muted">无</span>';

      html +=
        "<tr>" +
        '<td><span class="lang-tag" style="background:' +
        color +
        '">' +
        escapeHtml(lang) +
        "</span></td>" +
        "<td>" +
        fmtStr +
        "</td>" +
        "<td>" +
        cmpStr +
        "</td>" +
        '<td class="cell-muted">' +
        indentStr +
        "</td>" +
        '<td class="cell-mono" title="' +
        escapeHtml(detExts) +
        '" style="max-width:180px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">' +
        detExts +
        "</td>" +
        '<td class="col-actions">' +
        '<button class="mini-btn edit-btn" data-type="language" data-name="' +
        escapeHtml(lang) +
        '">编辑</button>' +
        '<button class="mini-btn danger del-btn" data-type="language" data-name="' +
        escapeHtml(lang) +
        '">删除</button>' +
        "</td>" +
        "</tr>";
    });
    html += "</tbody></table></div>";
    el.innerHTML = html;

    el.querySelectorAll('.edit-btn[data-type="language"]').forEach((btn) => {
      btn.addEventListener("click", () => openLanguageModal(btn.dataset.name));
    });
    el.querySelectorAll('.del-btn[data-type="language"]').forEach((btn) => {
      btn.addEventListener("click", () => deleteCRUD("language", btn.dataset.name));
    });
  }

  function openLanguageModal(lang) {
    const isEdit = !!lang;
    const langs = currentConfig.languages || {};
    const langCfg = isEdit ? langs[lang] || {} : {};
    const fmt = langCfg.formatter || {};
    const cmp = langCfg.compressor || null;
    const indent = langCfg.indent || null;
    const detection = langCfg.detection || null;
    const hl = langCfg.highlighter || null;
    const hlLexer = hl && hl.options && hl.options.lexer ? hl.options.lexer : "";

    const html =
      '<div class="form-row"><label>语言名称</label><input type="text" id="lg_lang" value="' +
      escapeHtml(lang || "") +
      '" placeholder="如 javascript / python"' +
      (isEdit ? " disabled" : "") +
      "></div>" +
      '<div class="form-section"><h4>格式化器</h4>' +
      '<div class="form-row"><label>Backend</label><select id="lg_fmt_backend"><option value="native"' +
      (fmt.backend === "native" ? " selected" : "") +
      '>native</option><option value="tool"' +
      (fmt.backend === "tool" ? " selected" : "") +
      ">tool</option></select></div>" +
      '<div class="form-row"><label>工具</label><input type="text" id="lg_fmt_tool" value="' +
      escapeHtml(fmt.tool || "") +
      '" placeholder="如 oxfmt-wrapper / shfmt"></div>' +
      '<div class="form-row"><label>命令参数</label><textarea id="lg_fmt_cmd" rows="2" placeholder="每行一个参数，支持 {tab_width} 等占位符">' +
      escapeHtml((fmt.cmd || []).join("\n")) +
      "</textarea></div>" +
      "</div>" +
      '<div class="form-section"><h4>压缩器</h4>' +
      '<div class="form-row"><label>Backend</label><select id="lg_cmp_backend"><option value=""' +
      (!cmp ? " selected" : "") +
      '>无</option><option value="native"' +
      (cmp && cmp.backend === "native" ? " selected" : "") +
      '>native</option><option value="tool"' +
      (cmp && cmp.backend === "tool" ? " selected" : "") +
      ">tool</option></select></div>" +
      '<div class="form-row"><label>工具</label><input type="text" id="lg_cmp_tool" value="' +
      escapeHtml(cmp ? cmp.tool || "" : "") +
      '" placeholder="如 json / html"></div>' +
      '<div class="form-row"><label>命令参数</label><textarea id="lg_cmp_cmd" rows="2" placeholder="每行一个参数">' +
      escapeHtml(cmp ? (cmp.cmd || []).join("\n") : "") +
      "</textarea></div>" +
      "</div>" +
      '<div class="form-section"><h4>高亮配置</h4>' +
      '<div class="form-row"><label>Chroma Lexer</label><input type="text" id="lg_hl_lexer" value="' +
      escapeHtml(hlLexer) +
      '" placeholder="留空=使用语言名，如 python3 / bash"></div>' +
      "</div>" +
      '<div class="form-section"><h4>缩进配置</h4>' +
      '<div class="form-row"><label>缩进方式</label><select id="lg_indent_tab_width"><option value=""' +
      (!indent ? " selected" : "") +
      '>默认</option><option value="-1"' +
      (indent && indent.tab_width === -1 ? " selected" : "") +
      '>Tab</option><option value="2"' +
      (indent && indent.tab_width === 2 ? " selected" : "") +
      '>2 空格</option><option value="4"' +
      (indent && indent.tab_width === 4 ? " selected" : "") +
      '>4 空格</option><option value="8"' +
      (indent && indent.tab_width === 8 ? " selected" : "") +
      ">8 空格</option></select></div>" +
      "</div>" +
      '<div class="form-section"><h4>检测规则</h4>' +
      '<div class="form-row"><label>扩展名</label><input type="text" id="lg_det_exts" value="' +
      escapeHtml(((detection && detection.extensions) || []).join(", ")) +
      '" placeholder="逗号分隔，如 .go, .go2"></div>' +
      '<div class="form-row"><label>Shebang 正则</label><textarea id="lg_det_shebangs" rows="2" placeholder="每行一个 RE2 正则，匹配首行 #!&#10;如: ^#!.*\\bpython3?\\b">' +
      escapeHtml(((detection && detection.shebangs) || []).join("\n")) +
      "</textarea></div>" +
      '<div class="form-row"><label>内容正则</label><textarea id="lg_det_content" rows="2" placeholder="每行一个 RE2 正则，匹配内容前 2048 字节&#10;如: ^\\s*package\\s+main\\b">' +
      escapeHtml(((detection && detection.content) || []).join("\n")) +
      "</textarea></div>" +
      '<div class="form-row"><label>优先级</label><input type="number" id="lg_det_priority" value="' +
      (detection && detection.priority !== undefined && detection.priority !== null ? detection.priority : "") +
      '" placeholder="数值小者先匹配，默认 0"></div>' +
      "</div>";

    openModal(isEdit ? "编辑语言" : "添加语言", html, async () => {
      const data = {
        lang: $("lg_lang").value.trim(),
        formatter: { backend: $("lg_fmt_backend").value, tool: $("lg_fmt_tool").value.trim() },
      };
      const fmtCmd = $("lg_fmt_cmd").value.trim();
      if (fmtCmd)
        data.formatter.cmd = fmtCmd
          .split("\n")
          .map((s) => s.trim())
          .filter((s) => s);

      const cmpBackend = $("lg_cmp_backend").value;
      if (cmpBackend) {
        data.compressor = { backend: cmpBackend, tool: $("lg_cmp_tool").value.trim() };
        const cmpCmd = $("lg_cmp_cmd").value.trim();
        if (cmpCmd)
          data.compressor.cmd = cmpCmd
            .split("\n")
            .map((s) => s.trim())
            .filter((s) => s);
      }

      // 高亮配置: chroma lexer 覆盖
      const hlLexerVal = $("lg_hl_lexer").value.trim();
      if (hlLexerVal) {
        data.highlighter = { backend: "native", tool: "chroma", options: { lexer: hlLexerVal } };
      }

      // 缩进配置: tab_width 有值时才写入 (-1=Tab, >0=空格宽度)
      const indentTabWidth = $("lg_indent_tab_width").value.trim();
      if (indentTabWidth) {
        data.indent = { tab_width: parseInt(indentTabWidth) };
      }

      // 检测规则: 任一字段有值时写入
      const detExts = $("lg_det_exts").value.trim();
      const detShebangs = $("lg_det_shebangs").value.trim();
      const detContent = $("lg_det_content").value.trim();
      const detPriority = $("lg_det_priority").value.trim();
      if (detExts || detShebangs || detContent || detPriority) {
        data.detection = {};
        if (detExts) data.detection.extensions = detExts.split(/,\s*/).filter((s) => s);
        if (detShebangs)
          data.detection.shebangs = detShebangs
            .split("\n")
            .map((s) => s.trim())
            .filter((s) => s);
        if (detContent)
          data.detection.content = detContent
            .split("\n")
            .map((s) => s.trim())
            .filter((s) => s);
        if (detPriority) data.detection.priority = parseInt(detPriority) || 0;
      }

      if (!data.lang) {
        showToast("语言名称不能为空", "error");
        return;
      }
      await saveCRUD("language", data, isEdit);
    });
  }

  // ---- Event Bindings ----
  function bindEvents() {
    document.querySelectorAll(".action-btn").forEach((btn) => {
      btn.addEventListener("click", () => execute(btn.dataset.action));
    });

    $("clearBtn").addEventListener("click", () => {
      $("inputCode").value = "";
      setOutput("", false);
      showToast("已清空");
    });

    $("copyBtn").addEventListener("click", async () => {
      const out = $("outputCode");
      const text = out.innerText;
      if (!text || text === "结果将显示在这里...") {
        showToast("没有可复制的内容", "error");
        return;
      }
      try {
        if (outputIsHighlight) {
          // 复制富文本 (包含高亮样式 HTML + 纯文本回退)
          const html = out.innerHTML;
          const htmlBlob = new Blob([html], { type: "text/html" });
          const textBlob = new Blob([text], { type: "text/plain" });
          const item = new ClipboardItem({
            "text/html": htmlBlob,
            "text/plain": textBlob,
          });
          await navigator.clipboard.write([item]);
        } else {
          await navigator.clipboard.writeText(text);
        }
        showToast("已复制到剪贴板", "success");
      } catch (e) {
        // 富文本复制失败时回退到纯文本
        try {
          await navigator.clipboard.writeText(text);
          showToast("已复制到剪贴板", "success");
        } catch (e2) {
          showToast("复制失败", "error");
        }
      }
    });

    $("downloadBtn").addEventListener("click", () => {
      const text = $("outputCode").innerText;
      if (!text || text === "结果将显示在这里...") {
        showToast("没有可下载的内容", "error");
        return;
      }
      const lang = $("langSelect").value;
      const ext = langExtension(lang);
      const blob = new Blob([text], { type: "text/plain" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = "formatter-output." + ext;
      a.style.display = "none";
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    });

    $("fileInput").addEventListener("change", (e) => {
      const file = e.target.files[0];
      if (!file) return;
      const ext = "." + file.name.split(".").pop().toLowerCase();
      const langMap = buildLangMap();
      const detected = langMap[ext];
      if (detected) $("langSelect").value = detected;
      const reader = new FileReader();
      reader.onload = (ev) => {
        $("inputCode").value = ev.target.result;
        showToast("已加载文件: " + file.name, "success");
      };
      reader.readAsText(file);
      e.target.value = "";
    });

    $("refreshBinaries")?.addEventListener("click", loadBinaries);
    $("installAllBinaries")?.addEventListener("click", installAllBinaries);
    $("addRuntimeBtn")?.addEventListener("click", () => openRuntimeModal(null));
    $("addToolBtn")?.addEventListener("click", () => openToolModal(null));
    $("addLanguageBtn")?.addEventListener("click", () => openLanguageModal(null));

    // Modal events
    $("modalClose")?.addEventListener("click", closeModal);
    $("modalCancel")?.addEventListener("click", closeModal);
    $("modalSave")?.addEventListener("click", () => {
      if (!modalSaveHandler) return;
      // 捕获同步和异步错误，避免 save handler 异常时无反馈 (Wails webview 无控制台可见)
      Promise.resolve(modalSaveHandler()).catch((e) => {
        showToast("操作失败: " + (e && e.message ? e.message : String(e)), "error");
        console.error("Modal save error:", e);
      });
    });
    $("modalOverlay")?.addEventListener("click", (e) => {
      if (e.target === $("modalOverlay")) closeModal();
    });

    // 快速选项仅对当前会话生效，不保存到配置文件
    $("tabWidth")?.addEventListener("change", () => {});
    $("lineNumbers")?.addEventListener("change", () => {});

    // 语言切换时更新快速选项和操作按钮状态
    $("langSelect")?.addEventListener("change", (e) => {
      const lang = e.target.value;
      updateQuickOptionsForLanguage(lang);
      updateButtonStates(lang);
    });

    // 自动检测语言按钮
    $("detectLangBtn")?.addEventListener("click", detectLanguage);

    document.addEventListener("keydown", (e) => {
      if (e.ctrlKey || e.metaKey) {
        if (e.key === "Enter") {
          e.preventDefault();
          const tag = document.activeElement.tagName;
          if (tag !== "TEXTAREA" && tag !== "INPUT") execute("run");
        } else if (e.key === "s") {
          e.preventDefault();
          execute("format");
        }
      }
    });
  }

  // ---- Init ----
  loadConfig();
  initNavigation();
  bindEvents();
  initHighlightConfig();
  initQuickStyle();
  loadStatus();
})();
