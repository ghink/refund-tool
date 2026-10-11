package ui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2/dialog"
	payCurrency "go.gh.ink/payutils/v3/utils/currency"
	"go.uber.org/zap"

	"refundtool/internal/logger"
	"refundtool/internal/money"
	"refundtool/internal/pay"
)

var (
	errNoClient    = errors.New("还没有可用的渠道，先选对 space 或修正配置")
	errNoTradeID   = errors.New("交易号不能为空")
	errNoRefundID  = errors.New("退款单号不能为空")
	errAmountRange = errors.New("金额必须大于 0")
	errOverTotal   = errors.New("退款金额不能大于订单金额")
)

// refundRequest is a validated refund from the form.
type refundRequest struct {
	upstream pay.Upstream
	tradeID  string
	currency string
	refundID string
	reason   string
	total    int64
	amount   int64
}

// queryStatus reports where a trade stands on the picked channels.
func (a *App) queryStatus() {
	if a.client == nil {
		a.log.Fail("%s", errNoClient)
		return
	}

	tradeID := strings.TrimSpace(a.trade.Text)
	if tradeID == "" {
		a.log.Fail("%s", errNoTradeID)
		return
	}

	client := a.client

	upstream, auto, err := a.selectedChannel()
	if err != nil {
		a.log.Fail("不能查询：%s", err)
		return
	}

	targets := client.Upstreams()
	if !auto {
		targets = []pay.Upstream{upstream}
	}

	a.task("查询", func(report func(string, ...any)) (string, error) {
		found := make([]string, 0, len(targets))
		for _, target := range targets {
			status, err := client.Status(target.Name, tradeID)
			if err != nil {
				if pay.TradeAbsent(err) {
					report("%s：查无此交易", target.Display)
					continue
				}
				return "", fmt.Errorf("%s 查询：%w", target.Display, err)
			}

			report("%s：%s，成交时间 %s", target.Display, status.TradeStatus, formatTime(status.Time))
			found = append(found, target.Display+"="+status.TradeStatus)
		}

		if len(found) == 0 {
			return "", fmt.Errorf("所有已配置渠道都查不到交易 %s", tradeID)
		}

		return strings.Join(found, " "), nil
	})
}

// confirmRefund validates the form and asks for approval.
func (a *App) confirmRefund() {
	request, auto, err := a.collectRefund()
	if err != nil {
		a.log.Fail("不能提交：%s", err)
		return
	}

	dialog.NewConfirm("确认退款", request.summary(a.client.Space(), auto), func(accepted bool) {
		if !accepted {
			a.log.Dim("已取消，没有发出请求。")
			return
		}
		a.submitRefund(*request, auto)
	}, a.window).Show()
}

func (a *App) collectRefund() (*refundRequest, bool, error) {
	if a.client == nil {
		return nil, false, errNoClient
	}

	upstream, auto, err := a.selectedChannel()
	if err != nil {
		return nil, false, err
	}

	tradeID := strings.TrimSpace(a.trade.Text)
	if tradeID == "" {
		return nil, false, errNoTradeID
	}

	refundID := strings.TrimSpace(a.refundNo.Text)
	if refundID == "" {
		return nil, false, errNoRefundID
	}

	total, err := money.ParseYuan(a.total.Text)
	if err != nil {
		return nil, false, fmt.Errorf("订单金额：%s", amountHint(err))
	}

	amount, err := money.ParseYuan(a.amount.Text)
	if err != nil {
		return nil, false, fmt.Errorf("退款金额：%s", amountHint(err))
	}

	switch {
	case total <= 0, amount <= 0:
		return nil, false, errAmountRange
	case amount > total:
		return nil, false, errOverTotal
	}

	currency := strings.TrimSpace(a.currency.Text)
	if currency == "" {
		currency = defaultCurrency
	}

	reason := strings.TrimSpace(a.reason.Text)
	if reason == "" {
		reason = defaultReason
	}

	return &refundRequest{
		upstream: upstream,
		tradeID:  tradeID,
		currency: currency,
		refundID: refundID,
		reason:   reason,
		total:    total,
		amount:   amount,
	}, auto, nil
}

// summary renders what the operator is about to approve.
func (r *refundRequest) summary(space string, auto bool) string {
	channel := r.upstream.Display
	if auto {
		channel = autoChannel + "：先查交易归属，只退给认得这笔交易的那一个"
	}

	return strings.Join([]string{
		"space：" + space,
		"渠道：" + channel,
		"交易号：" + r.tradeID,
		fmt.Sprintf("退款：¥%s（%s），订单总额：¥%s（%s）",
			r.yuan(r.amount), formatCents(r.amount), r.yuan(r.total), formatCents(r.total)),
		"退款单号：" + r.refundID,
		"原因：" + r.reason,
	}, "\n")
}

func (r *refundRequest) submit(client *pay.Client, auto bool, report func(string, ...any)) (string, error) {
	upstream := r.upstream

	if auto {
		detection, err := client.Detect(r.tradeID)
		if err != nil {
			return "", err
		}

		upstream = detection.Upstream
		report("交易在 %s，当前状态 %s", upstream.Display, detection.Status.TradeStatus)
	}

	fields := []zap.Field{
		zap.String("space", client.Space()),
		zap.String("upstream", upstream.Name),
		zap.String("trade_id", r.tradeID),
		zap.String("refund_id", r.refundID),
		zap.String("currency", r.currency),
		zap.Int64("total_amount", r.total),
		zap.Int64("refund_amount", r.amount),
		zap.String("reason", r.reason),
	}

	logger.L.Info("refund requested", fields...)

	if err := client.Refund(
		upstream.Name, r.tradeID, r.currency, r.refundID,
		r.total, r.amount, r.reason,
	); err != nil {
		logger.L.Error("refund failed", append(fields, zap.Error(err))...)
		return "", err
	}

	return fmt.Sprintf("%s 已受理：%s 退款 ¥%s（%s），退款单 %s",
		upstream.Display, r.tradeID, r.yuan(r.amount), formatCents(r.amount), r.refundID), nil
}

func (a *App) submitRefund(request refundRequest, auto bool) {
	client := a.client

	a.task("退款", func(report func(string, ...any)) (string, error) {
		return request.submit(client, auto, report)
	})
}

func (r *refundRequest) yuan(cents int64) string {
	return payCurrency.CentsToYuan(cents)
}

func formatCents(cents int64) string {
	return fmt.Sprintf("%d 分", cents)
}

func formatTime(moment time.Time) string {
	if moment.IsZero() {
		return "未知"
	}
	return moment.Format("2006-01-02 15:04:05")
}
