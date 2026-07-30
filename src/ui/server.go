package ui

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"time"

	"github.com/formatter/formatter/src/appcommon"
	iocore "github.com/formatter/formatter/src/io"
	"github.com/formatter/formatter/src/pipeline"
)

//go:embed static
var staticFS embed.FS

// 共享类型别名
type (
	RuntimeInfo  = appcommon.RuntimeInfo
	BinInfo      = appcommon.BinInfo
	LanguageInfo = appcommon.LanguageInfo
	FormatRequest = appcommon.FormatRequest
	RunRequest    = appcommon.RunRequest
	StatusResult  = appcommon.StatusResult
)

type Server struct {
	*appcommon.Core
	addr       string
	nativeMode bool // true = Chrome app 模式, false = 默认浏览器
	noOpen     bool // true = 不自动打开窗口
}

// SetNoOpen 设置是否跳过自动打开窗口
func (s *Server) SetNoOpen(v bool) { s.noOpen = v }

// ---- HTTP 响应类型 ----

type APIResponse struct {
	Success  bool        `json:"success"`
	Error    string      `json:"error,omitempty"`
	Result   string      `json:"result,omitempty"`
	Language string      `json:"language,omitempty"`
	Data     interface{} `json:"data,omitempty"`
}

type LanguagesResponse struct {
	Success bool           `json:"success"`
	Data    []LanguageInfo `json:"data"`
}

type BinListResponse struct {
	Success bool      `json:"success"`
	Data    []BinInfo `json:"data"`
}

type BinActionRequest struct {
	Name string `json:"name"`
}

type BinActionResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	Result  string `json:"result,omitempty"`
}

func NewServer(addr string, browserMode ...bool) (*Server, error) {
	core := appcommon.NewCore("")

	if addr == "" {
		port := core.Cfg.Server.Port
		if port == 0 {
			port = 7890
		}
		addr = fmt.Sprintf("127.0.0.1:%d", port)
	}

	native := true
	if len(browserMode) > 0 && browserMode[0] {
		native = false
	}

	return &Server{
		Core:       core,
		addr:       addr,
		nativeMode: native,
	}, nil
}

func (s *Server) Start() error {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/format", s.handleFormat)
	mux.HandleFunc("/api/compress", s.handleCompress)
	mux.HandleFunc("/api/highlight", s.handleHighlight)
	mux.HandleFunc("/api/run", s.handleRun)
	mux.HandleFunc("/api/languages", s.handleLanguages)
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/binaries", s.handleBinariesList)
	mux.HandleFunc("/api/binaries/install", s.handleBinariesInstall)
	mux.HandleFunc("/api/binaries/uninstall", s.handleBinariesUninstall)
	mux.HandleFunc("/api/binaries/verify", s.handleBinariesVerify)
	mux.HandleFunc("/api/runtime/install", s.handleRuntimeInstall)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/detect", s.handleDetect)

	// CRUD endpoints for runtimes / tools / languages
	mux.HandleFunc("/api/runtimes/add", s.handleRuntimeAdd)
	mux.HandleFunc("/api/runtimes/update", s.handleRuntimeUpdate)
	mux.HandleFunc("/api/runtimes/delete", s.handleRuntimeDelete)
	mux.HandleFunc("/api/tools/add", s.handleToolAdd)
	mux.HandleFunc("/api/tools/update", s.handleToolUpdate)
	mux.HandleFunc("/api/tools/delete", s.handleToolDelete)
	mux.HandleFunc("/api/languages/add", s.handleLanguageAdd)
	mux.HandleFunc("/api/languages/update", s.handleLanguageUpdate)
	mux.HandleFunc("/api/languages/delete", s.handleLanguageDelete)

	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		return err
	}
	mux.Handle("/", http.FileServer(http.FS(sub)))

	// Try to find an available port if auto_port is enabled
	addr := s.addr
	if s.Cfg.Server.AutoPort {
		newAddr, found := findAvailablePort(addr)
		if found {
			addr = newAddr
		}
	}

	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	if !s.noOpen {
		go func() {
			time.Sleep(400 * time.Millisecond)
			url := fmt.Sprintf("http://%s", addr)
			var err error
			if s.nativeMode {
				err = openNativeWindow(url)
			} else {
				err = openBrowserWindow(url)
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "[ui] 无法打开窗口: %v\n", err)
				fmt.Fprintf(os.Stderr, "[ui] 请手动访问: %s\n", url)
			}
		}()
	}

	fmt.Fprintf(os.Stdout, "[ui] Formatter UI 已启动: http://%s\n", addr)
	return server.ListenAndServe()
}

// findAvailablePort tries the given address; if the port is in use, increments and retries
func findAvailablePort(addr string) (string, bool) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return addr, false
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return addr, false
	}
	for i := 0; i < 20; i++ {
		testAddr := fmt.Sprintf("%s:%d", host, port+i)
		ln, err := net.Listen("tcp", testAddr)
		if err == nil {
			ln.Close()
			return testAddr, true
		}
	}
	return addr, false
}

func openNativeWindow(url string) error {
	if runtime.GOOS == "darwin" {
		if path, err := exec.LookPath("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"); err == nil {
			cmd := exec.Command(path, "--app="+url)
			return cmd.Start()
		}
		return exec.Command("open", url).Start()
	}
	return openBrowserWindow(url)
}

func openBrowserWindow(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

// ---- HTTP Handlers ----

func (s *Server) handleFormat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.handlePipeline(w, r, func(req *FormatRequest) appcommon.PipelineOptions {
		return appcommon.PipelineOptions{
			Language:           req.Language,
			TabWidth:           req.TabWidth,
			FormatterBackend:   req.FormatterBackend,
			CompressorBackend:  req.CompressorBackend,
			HighlighterBackend: req.HighlighterBackend,
		}
	}, s.FormatParams)
}

func (s *Server) handleCompress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.handlePipeline(w, r, func(req *FormatRequest) appcommon.PipelineOptions {
		return appcommon.PipelineOptions{
			Language:           req.Language,
			FormatterBackend:   req.FormatterBackend,
			CompressorBackend:  req.CompressorBackend,
			HighlighterBackend: req.HighlighterBackend,
		}
	}, s.CompressParams)
}

func (s *Server) handleHighlight(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.handlePipeline(w, r, func(req *FormatRequest) appcommon.PipelineOptions {
		return appcommon.PipelineOptions{
			Language:           req.Language,
			Style:              req.Style,
			FontSize:           req.FontSize,
			LineNumbers:        req.LineNumbers,
			CompatHTML:         req.CompatHTML,
			FormatterBackend:   req.FormatterBackend,
			CompressorBackend:  req.CompressorBackend,
			HighlighterBackend: req.HighlighterBackend,
		}
	}, s.HighlightParams)
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, APIResponse{Success: false, Error: err.Error()})
		return
	}

	params := s.RunParams(appcommon.PipelineOptions{
		Language:           req.Language,
		TabWidth:           req.TabWidth,
		Style:              req.Style,
		FontSize:           req.FontSize,
		LineNumbers:        req.LineNumbers,
		CompatHTML:         req.CompatHTML,
		NoFormat:           req.NoFormat,
		NoCompress:         req.NoCompress,
		NoHighlight:        req.NoHighlight,
		FormatterBackend:   req.FormatterBackend,
		CompressorBackend:  req.CompressorBackend,
		HighlighterBackend: req.HighlighterBackend,
	})

	result, err := s.ExecutePipeline(r.Context(), req.Code, params)
	if err != nil {
		writeJSON(w, APIResponse{Success: false, Error: err.Error()})
		return
	}

	writeJSON(w, APIResponse{
		Success:  true,
		Result:   result,
		Language: req.Language,
	})
}

// paramBuilder 从 FormatRequest 构建 PipelineOptions
type paramBuilder func(*FormatRequest) appcommon.PipelineOptions

// paramsBuilder 从 PipelineOptions 构建 pipeline.ExecuteParams
type paramsBuilder func(appcommon.PipelineOptions) pipeline.ExecuteParams

func (s *Server) handlePipeline(w http.ResponseWriter, r *http.Request, buildOpts paramBuilder, buildParams paramsBuilder) {
	var req FormatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, APIResponse{Success: false, Error: err.Error()})
		return
	}

	opts := buildOpts(&req)
	// TabWidth/Style 默认值已由 Params 方法处理
	params := buildParams(opts)
	result, err := s.ExecutePipeline(r.Context(), req.Code, params)
	if err != nil {
		writeJSON(w, APIResponse{Success: false, Error: err.Error()})
		return
	}

	writeJSON(w, APIResponse{
		Success:  true,
		Result:   result,
		Language: req.Language,
	})
}

func (s *Server) handleLanguages(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, LanguagesResponse{Success: true, Data: s.ListLanguages()})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.GetStatus())
}

func (s *Server) handleBinariesList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, BinListResponse{Success: true, Data: s.ListBinaries()})
}

func (s *Server) handleBinariesInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req BinActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, BinActionResponse{Success: false, Error: err.Error()})
		return
	}

	result := s.Core.InstallBinary(req.Name)
	writeJSON(w, BinActionResponse{Success: result.Success, Error: result.Error, Result: result.Result})
}

func (s *Server) handleBinariesUninstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req BinActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, BinActionResponse{Success: false, Error: err.Error()})
		return
	}

	result := s.Core.UninstallBinary(req.Name)
	writeJSON(w, BinActionResponse{Success: result.Success, Error: result.Error, Result: result.Result})
}

func (s *Server) handleBinariesVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req BinActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, BinActionResponse{Success: false, Error: err.Error()})
		return
	}

	result := s.Core.VerifyBinary(req.Name)
	writeJSON(w, BinActionResponse{Success: result.Success, Error: result.Error, Result: result.Result})
}

func (s *Server) handleRuntimeInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req BinActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, BinActionResponse{Success: false, Error: err.Error()})
		return
	}

	result := s.Core.InstallRuntime(req.Name)
	writeJSON(w, BinActionResponse{Success: result.Success, Error: result.Error, Result: result.Result})
}

// handleDetect 根据代码内容检测编程语言
func (s *Server) handleDetect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, APIResponse{Success: false, Error: "无效的请求: " + err.Error()})
		return
	}
	lang := iocore.DetectLanguage("", []byte(req.Code))
	writeJSON(w, APIResponse{Success: true, Language: lang})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, APIResponse{Success: true, Data: appcommon.ConfigToMap(s.Cfg)})

	case http.MethodPost:
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, APIResponse{Success: false, Error: err.Error()})
			return
		}
		if err := appcommon.UpdateConfigFromMap(s.Cfg, req); err != nil {
			writeJSON(w, APIResponse{Success: false, Error: err.Error()})
			return
		}
		writeJSON(w, APIResponse{Success: true, Result: "配置已更新"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}

// ---- CRUD Handlers (runtimes / tools / languages) ----
// 这些端点与 Wails 绑定方法一一对应，供 HTTP server 模式使用。

// decodeCRUDBody 解析请求体为 map; 仅允许 POST 方法。
func decodeCRUDBody(w http.ResponseWriter, r *http.Request) (map[string]interface{}, bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return nil, false
	}
	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, APIResponse{Success: false, Error: err.Error()})
		return nil, false
	}
	return req, true
}

// handleRuntimeAdd 添加运行时环境。
func (s *Server) handleRuntimeAdd(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeCRUDBody(w, r)
	if !ok {
		return
	}
	if err := s.Core.AddRuntime(req); err != nil {
		writeJSON(w, APIResponse{Success: false, Error: err.Error()})
		return
	}
	writeJSON(w, APIResponse{Success: true, Result: "运行时已添加"})
}

// handleRuntimeUpdate 更新运行时环境。
func (s *Server) handleRuntimeUpdate(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeCRUDBody(w, r)
	if !ok {
		return
	}
	if err := s.Core.UpdateRuntime(req); err != nil {
		writeJSON(w, APIResponse{Success: false, Error: err.Error()})
		return
	}
	writeJSON(w, APIResponse{Success: true, Result: "运行时已更新"})
}

// handleRuntimeDelete 删除运行时环境。
func (s *Server) handleRuntimeDelete(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeCRUDBody(w, r)
	if !ok {
		return
	}
	name, _ := req["name"].(string)
	if err := s.Core.DeleteRuntime(name); err != nil {
		writeJSON(w, APIResponse{Success: false, Error: err.Error()})
		return
	}
	writeJSON(w, APIResponse{Success: true, Result: "运行时已删除"})
}

// handleToolAdd 添加二进制工具。
func (s *Server) handleToolAdd(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeCRUDBody(w, r)
	if !ok {
		return
	}
	if err := s.Core.AddTool(req); err != nil {
		writeJSON(w, APIResponse{Success: false, Error: err.Error()})
		return
	}
	writeJSON(w, APIResponse{Success: true, Result: "工具已添加"})
}

// handleToolUpdate 更新二进制工具。
func (s *Server) handleToolUpdate(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeCRUDBody(w, r)
	if !ok {
		return
	}
	if err := s.Core.UpdateTool(req); err != nil {
		writeJSON(w, APIResponse{Success: false, Error: err.Error()})
		return
	}
	writeJSON(w, APIResponse{Success: true, Result: "工具已更新"})
}

// handleToolDelete 删除二进制工具。
func (s *Server) handleToolDelete(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeCRUDBody(w, r)
	if !ok {
		return
	}
	name, _ := req["name"].(string)
	if err := s.Core.DeleteTool(name); err != nil {
		writeJSON(w, APIResponse{Success: false, Error: err.Error()})
		return
	}
	writeJSON(w, APIResponse{Success: true, Result: "工具已删除"})
}

// handleLanguageAdd 添加编程语言配置。
func (s *Server) handleLanguageAdd(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeCRUDBody(w, r)
	if !ok {
		return
	}
	if err := s.Core.AddLanguage(req); err != nil {
		writeJSON(w, APIResponse{Success: false, Error: err.Error()})
		return
	}
	writeJSON(w, APIResponse{Success: true, Result: "语言已添加"})
}

// handleLanguageUpdate 更新编程语言配置。
func (s *Server) handleLanguageUpdate(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeCRUDBody(w, r)
	if !ok {
		return
	}
	if err := s.Core.UpdateLanguage(req); err != nil {
		writeJSON(w, APIResponse{Success: false, Error: err.Error()})
		return
	}
	writeJSON(w, APIResponse{Success: true, Result: "语言已更新"})
}

// handleLanguageDelete 删除编程语言配置。
func (s *Server) handleLanguageDelete(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeCRUDBody(w, r)
	if !ok {
		return
	}
	name, _ := req["name"].(string)
	if err := s.Core.DeleteLanguage(name); err != nil {
		writeJSON(w, APIResponse{Success: false, Error: err.Error()})
		return
	}
	writeJSON(w, APIResponse{Success: true, Result: "语言已删除"})
}
