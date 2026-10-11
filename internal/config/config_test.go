package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir, name, body string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}

	return path
}

func setup(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	t.Cleanup(func() {
		Spaces = map[string]*Space{}
		Log = FileLog{}
		Debug = false
		path = ""
	})

	return dir
}

const twoSpaces = `
bravo:
  alipay:
    app_id: "2021B"
alpha:
  wechat_pay:
    merchant_id: "1600A"
log:
  file: "refund-tool.log"
`

func TestTopLevelKeysBecomeSpaces(t *testing.T) {
	dir := setup(t)

	if err := Load(write(t, dir, "config.yaml", twoSpaces)); err != nil {
		t.Fatalf("Load: %v", err)
	}

	got := SpaceNames()
	if len(got) != 2 || got[0] != "alpha" || got[1] != "bravo" {
		t.Errorf("SpaceNames() = %v, want [alpha bravo] sorted", got)
	}

	// The log section is not a space.
	if _, ok := Spaces["log"]; ok {
		t.Error("log was parsed as a space")
	}
	if Log.File != "refund-tool.log" {
		t.Errorf("Log.File = %q", Log.File)
	}
	if Path() == "" {
		t.Error("Path() is empty after a successful load")
	}
}

func TestSpaceFieldsAreTyped(t *testing.T) {
	dir := setup(t)

	path := write(t, dir, "config.yaml", `
shop:
  prefix: "GH"
  suffix: "X"
  alipay:
    app_id: 2021000000000001
    app_cert_private_key: "private"
    app_cert: "app"
    root_cert: "root"
    public_cert: "public"
    is_prod: true
  wechat_pay:
    app_id: "wx0001"
    app_secret: "wx-secret"
    merchant_id: "1600000000"
    merchant_api_v3_key: "v3"
    merchant_cert_serial_number: "sn"
    merchant_private_key: "pk"
    public_key: "pub"
    public_key_id: "PUB_KEY_ID_1"
log:
  max_size: 12
  max_backups: 4
  max_age: 9
  compress: true
`)

	if err := Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}

	space, ok := Spaces["shop"]
	if !ok {
		t.Fatalf("shop missing from %v", SpaceNames())
	}

	if space.Prefix != "GH" || space.Suffix != "X" {
		t.Errorf("space = %+v, want prefix/suffix carried over", space)
	}

	// An unquoted numeric app id still lands in the string field.
	if space.Alipay.AppID != "2021000000000001" {
		t.Errorf("Alipay.AppID = %q, want the digits as text", space.Alipay.AppID)
	}
	if !space.Alipay.IsProd {
		t.Error("Alipay.IsProd = false, want true")
	}
	if space.Alipay.AppCertPrivateKey != "private" || space.Alipay.RootCert != "root" {
		t.Errorf("alipay = %+v, want the snake_case keys decoded", space.Alipay)
	}
	if space.WeChatPay.MerchantID != "1600000000" || space.WeChatPay.MerchantAPIv3Key != "v3" {
		t.Errorf("wechat = %+v, want the snake_case keys decoded", space.WeChatPay)
	}
	if space.WeChatPay.PublicKeyID != "PUB_KEY_ID_1" {
		t.Errorf("PublicKeyID = %q", space.WeChatPay.PublicKeyID)
	}
	if space.WeChatPay.AppSecret != "wx-secret" {
		t.Errorf("AppSecret = %q, want wx-secret", space.WeChatPay.AppSecret)
	}

	if Log.MaxSize != 12 || Log.MaxBackups != 4 || Log.MaxAge != 9 || !Log.Compress {
		t.Errorf("Log = %+v, want the sizing fields decoded", Log)
	}
}

func TestDebugOverlayMergesOverMainConfig(t *testing.T) {
	dir := setup(t)

	write(t, dir, "config.yaml", `
shop:
  prefix: "GH"
  alipay:
    app_id: "live-app"
    is_prod: true
`)
	write(t, dir, "config_debug.yaml", `
shop:
  alipay:
    app_id: "sandbox-app"
    is_prod: false
sandboxonly:
  alipay:
    app_id: "sandbox-app-2"
`)

	if err := Load(filepath.Join(dir, "config.yaml")); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !Debug {
		t.Error("Debug = false, want the overlay to mark debug mode")
	}

	shop := Spaces["shop"]
	if shop == nil {
		t.Fatal("shop missing")
	}
	if shop.Alipay.AppID != "sandbox-app" {
		t.Errorf("app_id = %q, want the overlay value", shop.Alipay.AppID)
	}
	if shop.Alipay.IsProd {
		t.Error("is_prod = true, want the overlay's false")
	}
	// Keys the overlay does not mention survive the merge.
	if shop.Prefix != "GH" {
		t.Errorf("prefix = %q, want the main config value", shop.Prefix)
	}
	if len(SpaceNames()) != 2 {
		t.Errorf("spaces = %v, want the overlay's extra space too", SpaceNames())
	}
}

func TestBrokenSpaceIsNamed(t *testing.T) {
	dir := setup(t)

	body := "shop:\n  prefix: \"GH\"\nstray: \"not a space\"\n"

	err := Load(write(t, dir, "config.yaml", body))
	if err == nil {
		t.Fatal("a scalar where a space belongs loaded anyway")
	}
	if !strings.Contains(err.Error(), "stray") {
		t.Errorf("err = %v, want it to name the broken space", err)
	}
}

func TestLoadFailureKeepsPreviousConfig(t *testing.T) {
	dir := setup(t)

	if err := Load(write(t, dir, "config.yaml", twoSpaces)); err != nil {
		t.Fatalf("Load: %v", err)
	}

	before := Spaces
	if err := Load(filepath.Join(dir, "absent.yaml")); err == nil {
		t.Fatal("Load of a missing file returned no error")
	}
	if len(Spaces) != 2 || Spaces["alpha"] != before["alpha"] {
		t.Errorf("spaces = %v, want the previously loaded config kept", SpaceNames())
	}
	if Log.File != "refund-tool.log" {
		t.Errorf("Log.File = %q, want it kept too", Log.File)
	}
}
