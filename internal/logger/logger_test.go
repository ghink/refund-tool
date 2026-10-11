package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"

	"refundtool/internal/config"
)

func TestInitWithoutAnyConfig(t *testing.T) {
	Init()

	if L == nil {
		t.Fatal("Init left the logger nil")
	}

	// Init works with no config loaded.
	L.Info("stdout only")
}

func TestInitOpensFileSinkFromConfig(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "refund-tool.log")

	path := filepath.Join(dir, "config.yaml")
	body := "log:\n  file: " + logPath + "\n  max_size: 1\n  max_backups: 2\n  compress: false\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := config.Load(path); err != nil {
		t.Fatalf("load config: %v", err)
	}
	t.Cleanup(func() { config.Spaces = nil; config.Log = config.FileLog{} })

	Init()
	L.Error("落盘一行", zap.String("trade_id", "T1"))
	_ = L.Sync()

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	text := string(data)

	for _, want := range []string{`"msg":"落盘一行"`, `"trade_id":"T1"`, `"level":"ERROR"`} {
		if !strings.Contains(text, want) {
			t.Errorf("log file = %s, want it to contain %s", text, want)
		}
	}
}
