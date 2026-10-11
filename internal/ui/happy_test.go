package ui

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
)

// workingSpace builds a space whose credentials the Alipay SDK accepts.
func workingSpace(t *testing.T, isProd bool) string {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "refund-tool test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	certificate := strings.ReplaceAll(
		string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), "\n", `\n`,
	)
	prod := "false"
	if isProd {
		prod = "true"
	}

	return `
demo-shop:
  alipay:
    app_id: "2021000000000001"
    app_cert_private_key: "` + base64.StdEncoding.EncodeToString(x509.MarshalPKCS1PrivateKey(key)) + `"
    app_cert: "` + certificate + `"
    root_cert: "` + certificate + `"
    public_cert: "` + certificate + `"
    is_prod: ` + prod + `
`
}

func TestUseSpaceWiresTheChannelAndOffersIt(t *testing.T) {
	a := newTestApp(t, workingSpace(t, true))

	if a.client == nil {
		t.Fatalf("client is nil, log was:\n%s", logText(t, a))
	}

	if got := a.channel.Options; len(got) != 2 || got[0] != autoChannel || got[1] != "支付宝" {
		t.Errorf("channel options = %v, want [自动探测 支付宝]", got)
	}

	// The picker shows which space is live.
	if a.space.Selected != "demo-shop" {
		t.Errorf("space picker = %q, want demo-shop", a.space.Selected)
	}
	if a.channel.Selected != autoChannel {
		t.Errorf("channel picker = %q, want %q", a.channel.Selected, autoChannel)
	}

	text := logText(t, a)
	if !strings.Contains(text, "已载入 space demo-shop") {
		t.Errorf("log = %q, want the loaded line", text)
	}
	if !strings.Contains(text, "支付宝·生产") {
		t.Errorf("log = %q, want the environment marked as live", text)
	}
}

func TestUseSpaceMarksTheTestEnvironment(t *testing.T) {
	a := newTestApp(t, workingSpace(t, false))

	if got := logText(t, a); !strings.Contains(got, "支付宝·测试环境") {
		t.Errorf("log = %q, want the sandbox warning", got)
	}
}

func TestRefundFormIsValidatedBeforeAnythingIsSent(t *testing.T) {
	a := newTestApp(t, workingSpace(t, true))

	cases := []struct {
		name  string
		fill  func()
		want  string
		opens bool
	}{
		{
			name: "missing trade id",
			fill: func() {},
			want: "交易号不能为空",
		},
		{
			name: "missing refund id",
			fill: func() { a.trade.SetText("T1"); a.total.SetText("100") },
			want: "退款单号不能为空",
		},
		{
			name: "refund above total",
			fill: func() {
				a.trade.SetText("T1")
				a.refundNo.SetText("R1")
				a.total.SetText("100")
				a.amount.SetText("100.01")
			},
			want: "退款金额不能大于订单金额",
		},
		{
			name: "zero amount",
			fill: func() {
				a.trade.SetText("T1")
				a.refundNo.SetText("R1")
				a.total.SetText("0")
				a.amount.SetText("0")
			},
			want: "金额必须大于 0",
		},
		{
			name: "valid",
			fill: func() {
				a.trade.SetText("T1")
				a.refundNo.SetText("R1")
				a.total.SetText("100.00")
				a.amount.SetText("12.34")
			},
			opens: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a.trade.SetText("")
			a.refundNo.SetText("")
			a.total.SetText("")
			a.amount.SetText("")
			before := len(a.window.Canvas().Overlays().List())

			c.fill()
			test.Tap(a.refundButton)

			opened := len(a.window.Canvas().Overlays().List()) > before
			if opened != c.opens {
				t.Errorf("dialog opened = %v, want %v", opened, c.opens)
			}

			text := logText(t, a)
			if c.want != "" && !strings.Contains(text, c.want) {
				t.Errorf("log = %q, want it to mention %q", text, c.want)
			}
			if strings.Contains(text, "退款成功") {
				t.Errorf("log = %q, no refund may have been sent", text)
			}
		})
	}
}
