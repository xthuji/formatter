package tests

import (
	"os"
	"testing"

	"github.com/formatter/formatter/src/appcommon"
)

// setupCRUDTest 备份 data/config.json，在 CRUD 测试结束后恢复原文件并重载 testCore，
// 避免测试修改影响到其他测试用例。
func setupCRUDTest(t *testing.T) {
	t.Helper()
	origConfig, err := os.ReadFile("data/config.json")
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	t.Cleanup(func() {
		_ = os.WriteFile("data/config.json", origConfig, 0644)
		testCore = appcommon.NewCore(os.Getenv("FORMATTER_BIN_HOME"))
	})
}

// ---- Runtime CRUD ----

func TestCRUD_AddRuntime_Success(t *testing.T) {
	setupCRUDTest(t)
	err := testCore.AddRuntime(map[string]interface{}{
		"name":        "test-runtime",
		"exe":         "test-rt",
		"path":        "/usr/local/bin/test-rt",
		"detect_exes": []interface{}{"test-rt", "test-rt2"},
	})
	if err != nil {
		t.Fatalf("AddRuntime failed: %v", err)
	}
	// 验证已添加
	found := false
	for _, rt := range testCore.Cfg.Binary.Runtimes {
		if rt.Name == "test-runtime" {
			found = true
			if rt.Exe != "test-rt" {
				t.Errorf("expected exe='test-rt', got %q", rt.Exe)
			}
		}
	}
	if !found {
		t.Error("test-runtime not found after AddRuntime")
	}
}

func TestCRUD_AddRuntime_Duplicate(t *testing.T) {
	setupCRUDTest(t)
	// 使用已存在的运行时名 (config.json 中应已有 node/java 等)
	existing := testCore.Cfg.Binary.Runtimes[0].Name
	err := testCore.AddRuntime(map[string]interface{}{
		"name": existing,
		"exe":  "dup",
	})
	if err == nil {
		t.Error("expected error for duplicate runtime name")
	}
}

func TestCRUD_AddRuntime_EmptyName(t *testing.T) {
	setupCRUDTest(t)
	err := testCore.AddRuntime(map[string]interface{}{
		"exe": "no-name",
	})
	if err == nil {
		t.Error("expected error for empty runtime name")
	}
}

func TestCRUD_UpdateRuntime_Success(t *testing.T) {
	setupCRUDTest(t)
	existing := testCore.Cfg.Binary.Runtimes[0].Name
	err := testCore.UpdateRuntime(map[string]interface{}{
		"name": existing,
		"exe":  "updated-exe",
		"path": "/new/path",
	})
	if err != nil {
		t.Fatalf("UpdateRuntime failed: %v", err)
	}
	if testCore.Cfg.Binary.Runtimes[0].Exe != "updated-exe" {
		t.Errorf("expected exe='updated-exe', got %q", testCore.Cfg.Binary.Runtimes[0].Exe)
	}
}

func TestCRUD_UpdateRuntime_NotFound(t *testing.T) {
	setupCRUDTest(t)
	err := testCore.UpdateRuntime(map[string]interface{}{
		"name": "nonexistent-rt",
		"exe":  "x",
	})
	if err == nil {
		t.Error("expected error for nonexistent runtime")
	}
}

func TestCRUD_DeleteRuntime_Success(t *testing.T) {
	setupCRUDTest(t)
	// 先添加再删除
	_ = testCore.AddRuntime(map[string]interface{}{
		"name": "to-delete",
		"exe":  "td",
	})
	err := testCore.DeleteRuntime("to-delete")
	if err != nil {
		t.Fatalf("DeleteRuntime failed: %v", err)
	}
	for _, rt := range testCore.Cfg.Binary.Runtimes {
		if rt.Name == "to-delete" {
			t.Error("runtime still exists after delete")
		}
	}
}

func TestCRUD_DeleteRuntime_NotFound(t *testing.T) {
	setupCRUDTest(t)
	err := testCore.DeleteRuntime("nonexistent-rt")
	if err == nil {
		t.Error("expected error for deleting nonexistent runtime")
	}
}

// ---- Tool CRUD ----

func TestCRUD_AddTool_Success(t *testing.T) {
	setupCRUDTest(t)
	err := testCore.AddTool(map[string]interface{}{
		"name":       "test-tool-new",
		"version":    "1.0.0",
		"executable": "test-tool-bin",
		"languages":  []interface{}{"text"},
		"source":     "download",
	})
	if err != nil {
		t.Fatalf("AddTool failed: %v", err)
	}
	found := false
	for _, tool := range testCore.Cfg.Binary.Tools {
		if tool.Name == "test-tool-new" {
			found = true
			if tool.Version != "1.0.0" {
				t.Errorf("expected version='1.0.0', got %q", tool.Version)
			}
		}
	}
	if !found {
		t.Error("test-tool-new not found after AddTool")
	}
}

func TestCRUD_AddTool_Duplicate(t *testing.T) {
	setupCRUDTest(t)
	existing := testCore.Cfg.Binary.Tools[0].Name
	err := testCore.AddTool(map[string]interface{}{
		"name":       existing,
		"executable": "dup",
	})
	if err == nil {
		t.Error("expected error for duplicate tool name")
	}
}

func TestCRUD_AddTool_EmptyName(t *testing.T) {
	setupCRUDTest(t)
	err := testCore.AddTool(map[string]interface{}{
		"executable": "no-name",
	})
	if err == nil {
		t.Error("expected error for empty tool name")
	}
}

func TestCRUD_UpdateTool_Success(t *testing.T) {
	setupCRUDTest(t)
	existing := testCore.Cfg.Binary.Tools[0].Name
	err := testCore.UpdateTool(map[string]interface{}{
		"name":       existing,
		"version":    "9.9.9",
		"executable": "updated-exe",
		"source":     "download",
	})
	if err != nil {
		t.Fatalf("UpdateTool failed: %v", err)
	}
	if testCore.Cfg.Binary.Tools[0].Version != "9.9.9" {
		t.Errorf("expected version='9.9.9', got %q", testCore.Cfg.Binary.Tools[0].Version)
	}
}

func TestCRUD_UpdateTool_NotFound(t *testing.T) {
	setupCRUDTest(t)
	err := testCore.UpdateTool(map[string]interface{}{
		"name":       "nonexistent-tool",
		"executable": "x",
	})
	if err == nil {
		t.Error("expected error for updating nonexistent tool")
	}
}

func TestCRUD_DeleteTool_Success(t *testing.T) {
	setupCRUDTest(t)
	_ = testCore.AddTool(map[string]interface{}{
		"name":       "tool-to-delete",
		"executable": "ttd",
		"source":     "download",
	})
	err := testCore.DeleteTool("tool-to-delete")
	if err != nil {
		t.Fatalf("DeleteTool failed: %v", err)
	}
	for _, tool := range testCore.Cfg.Binary.Tools {
		if tool.Name == "tool-to-delete" {
			t.Error("tool still exists after delete")
		}
	}
}

func TestCRUD_DeleteTool_NotFound(t *testing.T) {
	setupCRUDTest(t)
	err := testCore.DeleteTool("nonexistent-tool")
	if err == nil {
		t.Error("expected error for deleting nonexistent tool")
	}
}

// ---- Language CRUD ----

func TestCRUD_AddLanguage_Success(t *testing.T) {
	setupCRUDTest(t)
	err := testCore.AddLanguage(map[string]interface{}{
		"lang": "testlang",
		"formatter": map[string]interface{}{
			"backend": "native",
			"tool":    "go",
		},
		"detection": map[string]interface{}{
			"extensions": []interface{}{".tl"},
			"priority":   float64(5),
		},
		"indent": map[string]interface{}{
			"tab_width": float64(-1),
		},
	})
	if err != nil {
		t.Fatalf("AddLanguage failed: %v", err)
	}
	lc, ok := testCore.Cfg.Languages["testlang"]
	if !ok {
		t.Fatal("testlang not found after AddLanguage")
	}
	if lc.Formatter == nil || lc.Formatter.Backend != "native" {
		t.Errorf("unexpected formatter: %+v", lc.Formatter)
	}
	if lc.Detection == nil || len(lc.Detection.Extensions) != 1 || lc.Detection.Extensions[0] != ".tl" {
		t.Errorf("unexpected detection: %+v", lc.Detection)
	}
	if lc.Detection.Priority != 5 {
		t.Errorf("expected priority=5, got %d", lc.Detection.Priority)
	}
	if lc.Indent == nil || lc.Indent.TabWidth != -1 {
		t.Errorf("unexpected indent: %+v", lc.Indent)
	}
}

func TestCRUD_AddLanguage_Duplicate(t *testing.T) {
	setupCRUDTest(t)
	existing := ""
	for lang := range testCore.Cfg.Languages {
		existing = lang
		break
	}
	err := testCore.AddLanguage(map[string]interface{}{
		"lang": existing,
	})
	if err == nil {
		t.Error("expected error for duplicate language")
	}
}

func TestCRUD_AddLanguage_EmptyName(t *testing.T) {
	setupCRUDTest(t)
	err := testCore.AddLanguage(map[string]interface{}{})
	if err == nil {
		t.Error("expected error for empty language name")
	}
}

func TestCRUD_UpdateLanguage_Success(t *testing.T) {
	setupCRUDTest(t)
	// 先添加一个语言
	_ = testCore.AddLanguage(map[string]interface{}{
		"lang": "updatelang",
		"formatter": map[string]interface{}{
			"backend": "native",
			"tool":    "go",
		},
		"indent": map[string]interface{}{
			"tab_width": float64(2),
		},
	})
	// 更新语言 (只改 formatter, 不传 indent → 应保留原 indent)
	err := testCore.UpdateLanguage(map[string]interface{}{
		"lang": "updatelang",
		"formatter": map[string]interface{}{
			"backend": "tool",
			"tool":    "oxfmt-wrapper",
		},
	})
	if err != nil {
		t.Fatalf("UpdateLanguage failed: %v", err)
	}
	lc := testCore.Cfg.Languages["updatelang"]
	if lc.Formatter.Backend != "tool" || lc.Formatter.Tool != "oxfmt-wrapper" {
		t.Errorf("formatter not updated: %+v", lc.Formatter)
	}
	// indent 应保留 (parseLangConfig 在 data 未提供 indent 时保留 existing)
	if lc.Indent == nil || lc.Indent.TabWidth != 2 {
		t.Errorf("indent should be preserved: %+v", lc.Indent)
	}
}

func TestCRUD_DeleteLanguage_Success(t *testing.T) {
	setupCRUDTest(t)
	_ = testCore.AddLanguage(map[string]interface{}{
		"lang": "deleteme",
	})
	err := testCore.DeleteLanguage("deleteme")
	if err != nil {
		t.Fatalf("DeleteLanguage failed: %v", err)
	}
	if _, ok := testCore.Cfg.Languages["deleteme"]; ok {
		t.Error("language still exists after delete")
	}
}

func TestCRUD_DeleteLanguage_NotFound(t *testing.T) {
	setupCRUDTest(t)
	err := testCore.DeleteLanguage("nonexistent-lang")
	if err == nil {
		t.Error("expected error for deleting nonexistent language")
	}
}

// ---- parseLangConfig 边界场景 (通过 AddLanguage 间接测试) ----

func TestCRUD_AddLanguage_WithCompressor(t *testing.T) {
	setupCRUDTest(t)
	err := testCore.AddLanguage(map[string]interface{}{
		"lang": "compresstest",
		"formatter": map[string]interface{}{
			"backend": "native",
			"tool":    "jq",
		},
		"compressor": map[string]interface{}{
			"backend": "native",
			"tool":    "jq",
		},
	})
	if err != nil {
		t.Fatalf("AddLanguage with compressor failed: %v", err)
	}
	lc := testCore.Cfg.Languages["compresstest"]
	if lc.Compressor == nil || lc.Compressor.Tool != "jq" {
		t.Errorf("compressor not set correctly: %+v", lc.Compressor)
	}
}

func TestCRUD_AddLanguage_WithHighlighter(t *testing.T) {
	setupCRUDTest(t)
	err := testCore.AddLanguage(map[string]interface{}{
		"lang": "hltest",
		"highlighter": map[string]interface{}{
			"backend": "chroma",
			"tool":    "chroma",
			"options": map[string]interface{}{"theme": "monokai"},
		},
	})
	if err != nil {
		t.Fatalf("AddLanguage with highlighter failed: %v", err)
	}
	lc := testCore.Cfg.Languages["hltest"]
	if lc.Highlighter == nil || lc.Highlighter.Backend != "chroma" {
		t.Errorf("highlighter not set correctly: %+v", lc.Highlighter)
	}
}

func TestCRUD_AddLanguage_DetectionWithShebangs(t *testing.T) {
	setupCRUDTest(t)
	err := testCore.AddLanguage(map[string]interface{}{
		"lang": "shtest",
		"detection": map[string]interface{}{
			"extensions": []interface{}{".sht"},
			"shebangs":   []interface{}{"#!/usr/bin/sht"},
			"content":    []interface{}{"keyword"},
		},
	})
	if err != nil {
		t.Fatalf("AddLanguage failed: %v", err)
	}
	lc := testCore.Cfg.Languages["shtest"]
	if lc.Detection == nil {
		t.Fatal("detection is nil")
	}
	if len(lc.Detection.Shebangs) != 1 || lc.Detection.Shebangs[0] != "#!/usr/bin/sht" {
		t.Errorf("shebangs not set: %v", lc.Detection.Shebangs)
	}
	if len(lc.Detection.Content) != 1 || lc.Detection.Content[0] != "keyword" {
		t.Errorf("content not set: %v", lc.Detection.Content)
	}
}
