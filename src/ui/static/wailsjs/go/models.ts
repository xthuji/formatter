export namespace wailsapp {
  export class BinActionResult {
    success: boolean;
    error?: string;
    result?: string;

    static createFrom(source: any = {}) {
      return new BinActionResult(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.success = source["success"];
      this.error = source["error"];
      this.result = source["result"];
    }
  }
  export class BinInfo {
    name: string;
    version: string;
    language: string;
    installed: boolean;
    path?: string;
    runtime: string;
    runtime_ready: boolean;
    run_cmd?: string;
    size?: number;
    source: string;
    system_path?: string;

    static createFrom(source: any = {}) {
      return new BinInfo(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.name = source["name"];
      this.version = source["version"];
      this.language = source["language"];
      this.installed = source["installed"];
      this.path = source["path"];
      this.runtime = source["runtime"];
      this.runtime_ready = source["runtime_ready"];
      this.run_cmd = source["run_cmd"];
      this.size = source["size"];
      this.source = source["source"];
      this.system_path = source["system_path"];
    }
  }
  export class BinListResult {
    success: boolean;
    data: BinInfo[];

    static createFrom(source: any = {}) {
      return new BinListResult(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.success = source["success"];
      this.data = this.convertValues(source["data"], BinInfo);
    }

    convertValues(a: any, classs: any, asMap: boolean = false): any {
      if (!a) {
        return a;
      }
      if (a.slice && a.map) {
        return (a as any[]).map((elem) => this.convertValues(elem, classs));
      } else if ("object" === typeof a) {
        if (asMap) {
          for (const key of Object.keys(a)) {
            a[key] = new classs(a[key]);
          }
          return a;
        }
        return new classs(a);
      }
      return a;
    }
  }
  export class ConfigResult {
    success: boolean;
    data?: Record<string, any>;
    error?: string;
    result?: string;

    static createFrom(source: any = {}) {
      return new ConfigResult(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.success = source["success"];
      this.data = source["data"];
      this.error = source["error"];
      this.result = source["result"];
    }
  }
  export class DetectResult {
    success: boolean;
    error?: string;
    language?: string;

    static createFrom(source: any = {}) {
      return new DetectResult(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.success = source["success"];
      this.error = source["error"];
      this.language = source["language"];
    }
  }
  export class FormatRequest {
    language: string;
    code: string;
    tab_width: number;
    style: string;
    font_size: number;
    line_numbers: boolean;
    compat_html: boolean;
    formatter_backend: string;
    compressor_backend: string;
    highlighter_backend: string;

    static createFrom(source: any = {}) {
      return new FormatRequest(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.language = source["language"];
      this.code = source["code"];
      this.tab_width = source["tab_width"];
      this.style = source["style"];
      this.font_size = source["font_size"];
      this.line_numbers = source["line_numbers"];
      this.compat_html = source["compat_html"];
      this.formatter_backend = source["formatter_backend"];
      this.compressor_backend = source["compressor_backend"];
      this.highlighter_backend = source["highlighter_backend"];
    }
  }
  export class FormatResult {
    success: boolean;
    error?: string;
    result?: string;
    language?: string;

    static createFrom(source: any = {}) {
      return new FormatResult(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.success = source["success"];
      this.error = source["error"];
      this.result = source["result"];
      this.language = source["language"];
    }
  }
  export class LanguageInfo {
    lang: string;
    formatter: string;
    compressor: string;
    highlighter: string;

    static createFrom(source: any = {}) {
      return new LanguageInfo(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.lang = source["lang"];
      this.formatter = source["formatter"];
      this.compressor = source["compressor"];
      this.highlighter = source["highlighter"];
    }
  }
  export class LanguagesResult {
    success: boolean;
    data: LanguageInfo[];

    static createFrom(source: any = {}) {
      return new LanguagesResult(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.success = source["success"];
      this.data = this.convertValues(source["data"], LanguageInfo);
    }

    convertValues(a: any, classs: any, asMap: boolean = false): any {
      if (!a) {
        return a;
      }
      if (a.slice && a.map) {
        return (a as any[]).map((elem) => this.convertValues(elem, classs));
      } else if ("object" === typeof a) {
        if (asMap) {
          for (const key of Object.keys(a)) {
            a[key] = new classs(a[key]);
          }
          return a;
        }
        return new classs(a);
      }
      return a;
    }
  }
  export class RunRequest {
    language: string;
    code: string;
    tab_width: number;
    style: string;
    font_size: number;
    line_numbers: boolean;
    compat_html: boolean;
    no_format: boolean;
    no_compress: boolean;
    no_highlight: boolean;
    formatter_backend: string;
    compressor_backend: string;
    highlighter_backend: string;

    static createFrom(source: any = {}) {
      return new RunRequest(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.language = source["language"];
      this.code = source["code"];
      this.tab_width = source["tab_width"];
      this.style = source["style"];
      this.font_size = source["font_size"];
      this.line_numbers = source["line_numbers"];
      this.compat_html = source["compat_html"];
      this.no_format = source["no_format"];
      this.no_compress = source["no_compress"];
      this.no_highlight = source["no_highlight"];
      this.formatter_backend = source["formatter_backend"];
      this.compressor_backend = source["compressor_backend"];
      this.highlighter_backend = source["highlighter_backend"];
    }
  }
  export class RuntimeInfo {
    name: string;
    exe: string;
    installed: boolean;
    version?: string;
    path?: string;

    static createFrom(source: any = {}) {
      return new RuntimeInfo(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.name = source["name"];
      this.exe = source["exe"];
      this.installed = source["installed"];
      this.version = source["version"];
      this.path = source["path"];
    }
  }
  export class StatusResult {
    success: boolean;
    version: string;
    go_version: string;
    platform: string;
    install_dir: string;
    installed_bins: string[];
    missing_bins: string[];
    runtimes: RuntimeInfo[];

    static createFrom(source: any = {}) {
      return new StatusResult(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.success = source["success"];
      this.version = source["version"];
      this.go_version = source["go_version"];
      this.platform = source["platform"];
      this.install_dir = source["install_dir"];
      this.installed_bins = source["installed_bins"];
      this.missing_bins = source["missing_bins"];
      this.runtimes = this.convertValues(source["runtimes"], RuntimeInfo);
    }

    convertValues(a: any, classs: any, asMap: boolean = false): any {
      if (!a) {
        return a;
      }
      if (a.slice && a.map) {
        return (a as any[]).map((elem) => this.convertValues(elem, classs));
      } else if ("object" === typeof a) {
        if (asMap) {
          for (const key of Object.keys(a)) {
            a[key] = new classs(a[key]);
          }
          return a;
        }
        return new classs(a);
      }
      return a;
    }
  }
}
