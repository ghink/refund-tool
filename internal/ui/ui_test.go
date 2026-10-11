package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	payutilsErrors "go.gh.ink/payutils/v3/errors"

	"refundtool/internal/config"
	"refundtool/internal/pay"
)

// alipaySpace names a real channel with unusable key material.
const alipaySpace = `
example:
  alipay:
    app_id: "2021000000000001"
    app_cert_private_key: "not-a-real-key"
    app_cert: "not-a-real-cert"
    root_cert: "not-a-real-root"
    public_cert: "not-a-real-cert"
    is_prod: false
`

func writeConfig(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	return path
}

func newTestApp(t *testing.T, body string) *App {
	t.Helper()

	test.NewApp()
	t.Cleanup(func() {
		config.Spaces = nil
		config.Debug = false
	})

	a := newApp(writeConfig(t, body), test.NewWindow(nil))
	a.startup(nil)

	return a
}

func logText(t *testing.T, a *App) string {
	t.Helper()

	return a.log.grid.Text()
}

func TestStartupListsSpaceAndSurvivesUnusableCredentials(t *testing.T) {
	a := newTestApp(t, alipaySpace)

	if len(a.space.Options) != 1 || a.space.Options[0] != "example" {
		t.Fatalf("space options = %v, want [example]", a.space.Options)
	}

	// The driver SDK rejects the fake key material.
	if a.client != nil {
		t.Error("client built from unusable credentials, expected it to stay nil")
	}
	if got := logText(t, a); !strings.Contains(got, "初始化 space example 失败") {
		t.Errorf("log = %q, want it to report the failed init", got)
	}
	if len(a.channel.Options) != 0 {
		t.Errorf("channel options = %v, want none when no channel wired up", a.channel.Options)
	}
	// The failed space stays named in the picker.
	if a.space.Selected != "example" {
		t.Errorf("space picker = %q, want example", a.space.Selected)
	}
}

func TestStartupWithoutAnySpace(t *testing.T) {
	a := newTestApp(t, "log:\n  file: \"refund-tool.log\"\n")

	if got := logText(t, a); !strings.Contains(got, "没找到任何 space") {
		t.Errorf("log = %q, want the missing-space hint", got)
	}

	test.Tap(a.refundButton)
	if got := logText(t, a); !strings.Contains(got, "还没有可用的渠道") {
		t.Errorf("log after refund = %q, want the no-channel refusal", got)
	}
}

func TestReloadButtonRecoversFromABrokenConfig(t *testing.T) {
	test.NewApp()
	t.Cleanup(func() {
		config.Spaces = nil
		config.Debug = false
	})

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	a := newApp(path, test.NewWindow(nil))
	a.startup(config.Load(path))

	if got := logText(t, a); !strings.Contains(got, "配置加载失败") {
		t.Fatalf("log = %q, want the load failure", got)
	}

	if err := os.WriteFile(path, []byte(alipaySpace), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	test.Tap(a.reloadButton)

	if len(a.space.Options) != 1 || a.space.Options[0] != "example" {
		t.Errorf("space options after reload = %v, want [example]", a.space.Options)
	}
	if got := logText(t, a); !strings.Contains(got, "配置文件") {
		t.Errorf("log after reload = %q, want the loaded file line", got)
	}
}

func TestCentsHintTracksTheYuanEntries(t *testing.T) {
	a := newTestApp(t, alipaySpace)

	cases := []struct {
		text string
		want string
	}{
		{"12.34", "1234 分"},
		{"100", "10000 分"},
		{"0.05", "5 分"},
		{"12.345", "最多两位小数"},
		{"abc", "不是金额"},
		{"92233720368547759", "超出范围"},
		{"", "—"},
	}

	for _, c := range cases {
		a.amount.SetText(c.text)
		if got := a.amountCents.Text; got != c.want {
			t.Errorf("refund hint for %q = %q, want %q", c.text, got, c.want)
		}

		a.total.SetText(c.text)
		if got := a.totalCents.Text; got != c.want {
			t.Errorf("total hint for %q = %q, want %q", c.text, got, c.want)
		}
	}
}

func TestActionButtonsRefuseWithoutAClient(t *testing.T) {
	a := newTestApp(t, alipaySpace)

	for _, button := range []*widget.Button{a.refundButton, a.statusButton} {
		test.Tap(button)
	}

	text := logText(t, a)
	if !strings.Contains(text, "不能提交") {
		t.Errorf("log = %q, want the refund refusal", text)
	}
	if !strings.Contains(text, "不能查询") && !strings.Contains(text, "还没有可用的渠道") {
		t.Errorf("log = %q, want the status refusal", text)
	}
	if strings.Contains(text, "退款成功") {
		t.Errorf("log = %q, a refund must not have run", text)
	}
}

// installPoster diverts UI-thread closures into a channel the test drains.
func installPoster(a *App) chan func() {
	queue := make(chan func(), 16)
	a.post = func(fn func()) { queue <- fn }

	return queue
}

func runPosted(t *testing.T, queue chan func(), count int) {
	t.Helper()

	for i := range count {
		select {
		case fn := <-queue:
			fn()
		case <-time.After(2 * time.Second):
			t.Fatalf("only %d of %d UI updates arrived", i, count)
		}
	}
}

func TestTaskWritesProgressAndResult(t *testing.T) {
	a := newTestApp(t, alipaySpace)
	queue := installPoster(a)

	a.task("退款", func(report func(string, ...any)) (string, error) {
		report("中间进度一行")
		return "最终结果一行", nil
	})

	if !a.space.Disabled() || !a.refundButton.Disabled() {
		t.Error("controls were not locked while the task ran")
	}

	runPosted(t, queue, 2)

	text := logText(t, a)
	if !strings.Contains(text, "中间进度一行") {
		t.Errorf("log = %q, want the progress line", text)
	}
	if !strings.Contains(text, "退款成功：最终结果一行") {
		t.Errorf("log = %q, want the success line", text)
	}
	if a.running {
		t.Error("running stayed true after the task")
	}
	if a.refundButton.Disabled() {
		t.Error("buttons stayed disabled after the task")
	}
}

func TestTaskReportsUpstreamDetailOnFailure(t *testing.T) {
	a := newTestApp(t, alipaySpace)
	queue := installPoster(a)

	a.task("退款", func(report func(string, ...any)) (string, error) {
		return "", payutilsErrors.ErrTradeNotExist.
			WithUpstreamName("alipay").
			WithUpstreamCode("ACQ.TRADE_NOT_EXIST").
			WithUpstreamMessage("交易不存在")
	})

	runPosted(t, queue, 1)

	text := logText(t, a)
	for _, want := range []string{"trade not exist", "upstream=alipay", "code=ACQ.TRADE_NOT_EXIST", "交易不存在"} {
		if !strings.Contains(text, want) {
			t.Errorf("log = %q, want it to carry %q", text, want)
		}
	}
	if strings.Contains(text, "退款成功") {
		t.Errorf("log = %q, a failed task must not report success", text)
	}
	if a.space.Disabled() {
		t.Error("controls stayed disabled after the task failed")
	}
}

func TestTaskRefusesToRunTwoAtOnce(t *testing.T) {
	a := newTestApp(t, alipaySpace)
	queue := installPoster(a)

	a.running = true
	ran := false
	a.task("测试", func(report func(string, ...any)) (string, error) {
		ran = true
		return "", nil
	})

	if ran {
		t.Error("a task started while another was running")
	}
	if got := logText(t, a); !strings.Contains(got, "上一个操作还没结束") {
		t.Errorf("log = %q, want the busy notice", got)
	}
	select {
	case <-queue:
		t.Error("a refused task still scheduled UI work")
	default:
	}
}

func TestRefundSummaryListsWhatIsAboutToHappen(t *testing.T) {
	request := refundRequest{
		upstream: pay.Upstreams[0],
		tradeID:  "GH20261011001",
		currency: "CNY",
		refundID: "RF-0001",
		reason:   "主动退款",
		total:    10000,
		amount:   1234,
	}

	text := request.summary("demo-shop", false)
	for _, want := range []string{
		"space：demo-shop",
		"渠道：支付宝",
		"交易号：GH20261011001",
		"¥12.34（1234 分）",
		"¥100.00（10000 分）",
		"退款单号：RF-0001",
		"原因：主动退款",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("summary = %q, want it to carry %q", text, want)
		}
	}

	if auto := request.summary("demo-shop", true); !strings.Contains(auto, autoChannel) {
		t.Errorf("auto summary = %q, want it to say the channel is being detected", auto)
	}
}
