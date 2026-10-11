package pay

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	payutilsErrors "go.gh.ink/payutils/v3/errors"

	"refundtool/internal/config"
)

const fullSpace = `
myshop:
  prefix: "P"
  suffix: "S"
  alipay:
    app_id: "2021000000000001"
    app_cert_private_key: "app-private-key"
    app_cert: "app-cert"
    root_cert: "root-cert"
    public_cert: "public-cert"
    is_prod: true
  wechat_pay:
    app_id: "wx0001"
    app_secret: "wx-secret"
    merchant_id: "1600000000"
    merchant_api_v3_key: "api-v3-key"
    merchant_cert_serial_number: "serial-1"
    merchant_private_key: "merchant-private-key"
    public_key: "wechat-public-key"
    public_key_id: "PUB_KEY_ID_1"

log:
  file: "refund-tool.log"
`

func load(t *testing.T, body string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := config.Load(path); err != nil {
		t.Fatalf("load config: %v", err)
	}

	t.Cleanup(func() {
		config.Spaces = nil
		config.Debug = false
	})
}

func TestLoadSpacePassesDriverSpelledKeys(t *testing.T) {
	load(t, fullSpace)

	settings, err := loadSpace("myshop")
	if err != nil {
		t.Fatalf("loadSpace: %v", err)
	}

	if settings.prefix != "P" || settings.suffix != "S" {
		t.Errorf("prefix/suffix = %q/%q, want P/S", settings.prefix, settings.suffix)
	}

	got := make([]string, 0, len(settings.upstreams))
	for _, upstream := range settings.upstreams {
		got = append(got, upstream.Name)
	}
	if len(got) != 2 || got[0] != "alipay" || got[1] != "wechat" {
		t.Fatalf("upstreams = %v, want [alipay wechat]", got)
	}

	// Config is snake_case; the drivers ask for camelCase constants.
	alipay := settings.credentials["alipay"]
	for key, want := range map[string]string{
		"appID":             "2021000000000001",
		"appCertPrivateKey": "app-private-key",
		"appCert":           "app-cert",
		"rootCert":          "root-cert",
		"publicCert":        "public-cert",
		"isProd":            "true",
	} {
		if alipay[key] != want {
			t.Errorf("alipay[%q] = %q, want %q", key, alipay[key], want)
		}
	}

	wechat := settings.credentials["wechat"]
	for key, want := range map[string]string{
		"appID":            "wx0001",
		"mchID":            "1600000000",
		"apiV3Key":         "api-v3-key",
		"certSerialNumber": "serial-1",
		"privateKey":       "merchant-private-key",
		"publicKey":        "wechat-public-key",
		"publicKeyID":      "PUB_KEY_ID_1",
	} {
		if wechat[key] != want {
			t.Errorf("wechat[%q] = %q, want %q", key, wechat[key], want)
		}
	}

	// app_secret is config only: the v3 wechat driver has no such credential.
	if _, ok := wechat["appSecret"]; ok {
		t.Error("app_secret was forwarded to a driver that does not take it")
	}
	if len(wechat) != 7 {
		t.Errorf("wechat credentials = %v, want the 7 keys the driver declares", wechat)
	}
}

func TestLoadSpaceRequiresPresenceKey(t *testing.T) {
	load(t, `
partial:
  alipay:
    app_id: ""
    app_cert: "still-here"
  wechat_pay:
    app_id: "wx0001"
    merchant_id: "1600000000"
`)

	settings, err := loadSpace("partial")
	if err != nil {
		t.Fatalf("loadSpace: %v", err)
	}

	if len(settings.upstreams) != 1 || settings.upstreams[0].Name != "wechat" {
		t.Fatalf("upstreams = %v, want only wechat", settings.upstreams)
	}
	if _, ok := settings.credentials["alipay"]; ok {
		t.Error("alipay was configured despite an empty app_id")
	}
}

func TestLoadSpaceRejectsHalfSetSpaces(t *testing.T) {
	cases := []struct {
		name string
		body string
		want error
	}{
		{
			name: "no channel",
			body: "s:\n  prefix: \"P\"\n",
			want: ErrNoUpstream,
		},
		{
			name: "channel section without its presence key",
			body: "s:\n  alipay:\n    app_cert: \"cert\"\n",
			want: ErrNoUpstream,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			load(t, c.body)

			_, err := loadSpace("s")
			if !errors.Is(err, c.want) {
				t.Errorf("err = %v, want %v", err, c.want)
			}
		})
	}
}

func TestLoadSpaceUnknownName(t *testing.T) {
	load(t, fullSpace)

	if _, err := loadSpace("nosuch"); !errors.Is(err, ErrNoSpace) {
		t.Errorf("err = %v, want %v", err, ErrNoSpace)
	}
}

func TestSandboxedFollowsAlipayIsProd(t *testing.T) {
	client := &Client{credential: map[string]map[string]string{
		"alipay": {"isProd": "false"},
		"wechat": {"isProd": "false"},
	}}

	if !client.Sandboxed("alipay") {
		t.Error("alipay with isProd=false should report sandboxed")
	}
	if client.Sandboxed("wechat") {
		t.Error("wechat has no sandbox switch, it must not report sandboxed")
	}

	client.credential["alipay"]["isProd"] = "true"
	if client.Sandboxed("alipay") {
		t.Error("alipay with isProd=true should report the live environment")
	}
}

func TestRefundRejectsUnconfiguredChannel(t *testing.T) {
	load(t, fullSpace)

	settings, err := loadSpace("myshop")
	if err != nil {
		t.Fatalf("loadSpace: %v", err)
	}

	client := &Client{space: "myshop", upstreams: settings.upstreams, credential: settings.credentials}

	err = client.Refund("paypal", "trade", "CNY", "refund", 100, 100, "reason")
	if !errors.Is(err, payutilsErrors.ErrUpstreamNotFound) {
		t.Fatalf("err = %v, want upstream not found", err)
	}

	var payutilsError *payutilsErrors.PayutilsError
	if !errors.As(err, &payutilsError) || payutilsError.UpstreamName() != "paypal" {
		t.Errorf("err = %v, want it to name the rejected upstream", err)
	}
}
