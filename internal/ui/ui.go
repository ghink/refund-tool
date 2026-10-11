package ui

import (
	"errors"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"refundtool/internal/config"
	"refundtool/internal/money"
	"refundtool/internal/pay"
)

const (
	autoChannel     = "自动探测"
	defaultCurrency = "CNY"
	defaultReason   = "主动退款"
)

var errNoChannel = errors.New("请先选择支付渠道")

// App is the refund tool window.
type App struct {
	window fyne.Window
	log    *logPane

	configPath string

	space       *widget.Select
	channel     *widget.Select
	trade       *widget.Entry
	currency    *widget.Entry
	total       *widget.Entry
	totalCents  *widget.Label
	refundNo    *widget.Entry
	amount      *widget.Entry
	amountCents *widget.Label
	reason      *widget.Entry

	reloadButton *widget.Button
	statusButton *widget.Button
	refundButton *widget.Button
	clearButton  *widget.Button

	// post hands a closure to the UI thread.
	post func(fn func())

	client   *pay.Client
	channels []pay.Upstream
	running  bool
}

// Run opens the window and blocks until it closes.
func Run(configPath string, loadErr error) {
	a := newApp(configPath, app.New().NewWindow("退款工具"))

	a.startup(loadErr)
	a.window.Resize(fyne.NewSize(820, 900))
	a.window.ShowAndRun()
}

func newApp(configPath string, window fyne.Window) *App {
	a := &App{configPath: configPath, window: window, post: fyne.Do}
	a.build()

	return a
}

func (a *App) startup(loadErr error) {
	if loadErr != nil {
		a.log.Fail("配置加载失败：%v", loadErr)
		a.log.Dim("改好配置后点「重新加载配置」。")
		return
	}

	a.reload()
}

func (a *App) build() {
	a.log = newLogPane()

	a.space = widget.NewSelect(nil, a.useSpace)
	a.space.PlaceHolder = "选择 space"

	a.channel = widget.NewSelect([]string{autoChannel}, nil)
	a.channel.SetSelected(autoChannel)

	a.trade = widget.NewEntry()
	a.trade.PlaceHolder = "下单时的交易号，不含配置里的前后缀"

	a.currency = widget.NewEntry()
	a.currency.Text = defaultCurrency

	a.total = widget.NewEntry()
	a.total.PlaceHolder = "订单总额，例如 100.00"
	a.totalCents = widget.NewLabel("—")

	a.refundNo = widget.NewEntry()
	a.refundNo.PlaceHolder = "商户退款单号，重复提交会被渠道拒绝"

	a.amount = widget.NewEntry()
	a.amount.PlaceHolder = "本次退款，例如 12.34"
	a.amountCents = widget.NewLabel("—")

	a.reason = widget.NewEntry()
	a.reason.Text = defaultReason

	a.total.OnChanged = func(text string) { a.setCents(a.totalCents, text) }
	a.amount.OnChanged = func(text string) { a.setCents(a.amountCents, text) }

	a.reloadButton = widget.NewButton("重新加载配置", a.reload)
	a.statusButton = widget.NewButton("查询状态", a.queryStatus)
	a.refundButton = widget.NewButtonWithIcon("执行退款", theme.ConfirmIcon(), a.confirmRefund)
	a.refundButton.Importance = widget.HighImportance
	a.clearButton = widget.NewButton("清空日志", a.log.Clear)

	form := container.New(layout.NewFormLayout(),
		widget.NewLabel("Space"), a.space,
		widget.NewLabel("渠道"), a.channel,
		widget.NewLabel("交易号"), a.trade,
		widget.NewLabel("币种"), a.currency,
		widget.NewLabel("订单金额（元）"), container.NewBorder(nil, nil, nil, a.totalCents, a.total),
		widget.NewLabel("退款单号"), a.refundNo,
		widget.NewLabel("退款金额（元）"), container.NewBorder(nil, nil, nil, a.amountCents, a.amount),
		widget.NewLabel("退款原因"), a.reason,
	)

	body := container.NewVSplit(
		container.NewScroll(form),
		container.NewBorder(widget.NewLabel("日志"), nil, nil, nil, a.log.grid),
	)
	body.SetOffset(0.52)

	footer := container.NewHBox(
		a.reloadButton,
		a.clearButton,
		layout.NewSpacer(),
		a.statusButton,
		a.refundButton,
	)

	a.window.SetContent(container.NewBorder(nil, footer, nil, nil, body))
}

// reload rereads the config and repopulates the space list.
func (a *App) reload() {
	if err := config.Load(a.configPath); err != nil {
		a.forgetSpace()
		a.log.Fail("配置加载失败：%v", err)
		return
	}

	a.log.Info("配置文件：%s", config.Path())
	if config.Debug {
		a.log.Dim("config_debug.yaml 覆盖了主配置，调试模式开启，渠道 SDK 会输出详细日志。")
	}

	spaces := config.SpaceNames()
	if len(spaces) == 0 {
		a.forgetSpace()
		a.log.Fail("没找到任何 space：%s 里需要一个以 alipay 或 wechat_pay 为小节的顶层键。", config.Path())
		return
	}

	a.log.Dim("可选 space：%s", strings.Join(spaces, "、"))
	a.space.SetOptions(spaces)
	// SetSelected fires OnChanged, which loads the space.
	a.space.SetSelected(spaces[0])
}

// forgetSpace drops the client and both pickers.
func (a *App) forgetSpace() {
	a.forgetChannels()
	a.space.SetOptions(nil)
	a.space.Selected = ""
	a.space.Refresh()
}

// forgetChannels drops the client and the channel picker, keeping the space list.
func (a *App) forgetChannels() {
	a.client = nil
	a.channels = nil

	a.channel.SetOptions(nil)
	a.channel.Selected = ""
	a.channel.Refresh()
}

// useSpace rebuilds the payutils client for the picked space.
func (a *App) useSpace(space string) {
	if space == "" {
		return
	}

	client, err := pay.Build(space)
	if err != nil {
		a.forgetChannels()
		a.log.Fail("初始化 space %s 失败：%s", space, pay.Detail(err))
		return
	}

	a.client = client
	a.channels = client.Upstreams()

	options := make([]string, 0, len(a.channels)+1)
	options = append(options, autoChannel)
	for _, upstream := range a.channels {
		options = append(options, upstream.Display)
	}

	a.channel.SetOptions(options)
	a.channel.SetSelected(autoChannel)

	a.log.OK("已载入 space %s：%s", space, a.describe(client))
}

func (a *App) describe(client *pay.Client) string {
	marks := make([]string, 0, len(a.channels))
	for _, upstream := range client.Upstreams() {
		mark := upstream.Display
		if upstream.HasSandboxSwitch {
			if client.Sandboxed(upstream.Name) {
				mark += "·测试环境"
			} else {
				mark += "·生产"
			}
		}
		marks = append(marks, mark)
	}

	return strings.Join(marks, "、")
}

// selectedChannel reads the channel picker; auto is true for the auto-detect entry.
func (a *App) selectedChannel() (upstream pay.Upstream, auto bool, err error) {
	switch selected := a.channel.Selected; selected {
	case autoChannel:
		return pay.Upstream{}, true, nil
	case "":
		return pay.Upstream{}, false, errNoChannel
	default:
		for _, configured := range a.channels {
			if configured.Display == selected {
				return configured, false, nil
			}
		}
		return pay.Upstream{}, false, errNoChannel
	}
}

func (a *App) setCents(label *widget.Label, text string) {
	cents, err := money.ParseYuan(text)
	if err != nil {
		label.SetText(amountHint(err))
		return
	}
	label.SetText(formatCents(cents))
}

// amountHint words a parse failure for the hint column.
func amountHint(err error) string {
	switch {
	case errors.Is(err, money.ErrEmpty):
		return "—"
	case errors.Is(err, money.ErrTooPrecise):
		return "最多两位小数"
	case errors.Is(err, money.ErrOverflow):
		return "超出范围"
	default:
		return "不是金额"
	}
}

// task runs action on a background goroutine.
func (a *App) task(title string, action func(report func(string, ...any)) (string, error)) {
	if a.running {
		a.log.Dim("上一个操作还没结束。")
		return
	}

	a.running = true
	a.setRunning(true)

	go func() {
		message, err := action(a.report)

		a.post(func() {
			a.running = false
			a.setRunning(false)

			if err != nil {
				a.log.Fail("%s失败：%s", title, pay.Detail(err))
				return
			}
			if message != "" {
				a.log.OK("%s成功：%s", title, message)
			}
		})
	}()
}

// report is the progress writer handed to a task; safe from any goroutine.
func (a *App) report(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	a.post(func() { a.log.Info("%s", message) })
}

func (a *App) setRunning(running bool) {
	for _, object := range []fyne.CanvasObject{
		a.space, a.channel, a.reloadButton, a.statusButton, a.refundButton,
	} {
		disableable, ok := object.(fyne.Disableable)
		if !ok {
			continue
		}
		if running {
			disableable.Disable()
		} else {
			disableable.Enable()
		}
	}
}
