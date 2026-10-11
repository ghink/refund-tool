package pay

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	payutilsClient "go.gh.ink/payutils/v3/client"
	payutilsErrors "go.gh.ink/payutils/v3/errors"
	"go.gh.ink/payutils/v3/model"
)

// fakePayClient stands in for a driver.
type fakePayClient struct {
	status    model.ReturnStatus
	statusErr error

	refundErr  error
	refundArgs []any
}

func (f *fakePayClient) Create(context.Context, any) (bool, any, error) { return false, nil, nil }

func (f *fakePayClient) Callback(http.ResponseWriter, *http.Request) {}

func (f *fakePayClient) Status(string) (model.ReturnStatus, error) { return f.status, f.statusErr }

func (f *fakePayClient) Close(string) error { return nil }

func (f *fakePayClient) Refund(
	tradeID, currency, refundID string,
	totalAmount, refundAmount int64,
	reason string,
) error {
	f.refundArgs = []any{tradeID, currency, refundID, totalAmount, refundAmount, reason}
	return f.refundErr
}

func testClient(channels map[string]*fakePayClient) *Client {
	payClients := make(map[string]model.PayClient, len(channels))
	for name, fake := range channels {
		payClients[name] = fake
	}

	upstreams := make([]Upstream, 0, len(channels))
	for _, upstream := range Upstreams {
		if _, ok := channels[upstream.Name]; ok {
			upstreams = append(upstreams, upstream)
		}
	}

	return &Client{
		space:     "shop",
		upstreams: upstreams,
		inner:     &payutilsClient.Client{PayClient: payClients},
	}
}

func absent() error {
	return payutilsErrors.ErrTradeNotExist.WithUpstreamName("absent")
}

// errSignature stands for a channel-level failure.
var errSignature = errors.New("signature mismatch")

func TestDetectPicksTheChannelHoldingTheTrade(t *testing.T) {
	paid := model.ReturnStatus{
		TradeStatus: model.TradeStateSuccess,
		Upstream:    "wechat",
		Time:        time.Date(2026, 10, 11, 3, 0, 0, 0, time.UTC),
	}

	client := testClient(map[string]*fakePayClient{
		"alipay": {statusErr: absent()},
		"wechat": {status: paid},
	})

	found, err := client.Detect("T1")
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if found.Upstream.Name != "wechat" {
		t.Errorf("detected %q, want wechat", found.Upstream.Name)
	}
	if found.Status.TradeStatus != model.TradeStateSuccess {
		t.Errorf("status = %q, want SUCCESS", found.Status.TradeStatus)
	}
}

func TestDetectFailures(t *testing.T) {
	cases := []struct {
		name     string
		channels map[string]*fakePayClient
		want     error
	}{
		{
			name:     "nobody knows the trade",
			channels: map[string]*fakePayClient{"alipay": {statusErr: absent()}, "wechat": {statusErr: absent()}},
			want:     ErrTradeNotFound,
		},
		{
			name: "both channels hold it",
			channels: map[string]*fakePayClient{
				"alipay": {status: model.ReturnStatus{TradeStatus: model.TradeStateSuccess}},
				"wechat": {status: model.ReturnStatus{TradeStatus: model.TradeStateSuccess}},
			},
			want: ErrTradeAmbiguous,
		},
		{
			name: "a broken channel aborts detection",
			channels: map[string]*fakePayClient{
				"alipay": {statusErr: errSignature},
				"wechat": {status: model.ReturnStatus{TradeStatus: model.TradeStateSuccess}},
			},
			want: errSignature,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			client := testClient(c.channels)

			_, err := client.Detect("T1")
			if !errors.Is(err, c.want) {
				t.Errorf("Detect err = %v, want %v", err, c.want)
			}
		})
	}
}

func TestStatusReturnsTheChannelRow(t *testing.T) {
	client := testClient(map[string]*fakePayClient{
		"alipay": {status: model.ReturnStatus{TradeStatus: model.TradeStatePending, Upstream: "alipay"}},
	})

	status, err := client.Status("alipay", "T1")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.TradeStatus != model.TradeStatePending || status.Upstream != "alipay" {
		t.Errorf("status = %+v, want PENDING from alipay", status)
	}
}

func TestRefundForwardsEveryArgument(t *testing.T) {
	alipay := &fakePayClient{}
	client := testClient(map[string]*fakePayClient{"alipay": alipay, "wechat": {}})

	if err := client.Refund("alipay", "T1", "CNY", "R1", 10000, 1234, "主动退款"); err != nil {
		t.Fatalf("Refund: %v", err)
	}

	want := []any{"T1", "CNY", "R1", int64(10000), int64(1234), "主动退款"}
	if len(alipay.refundArgs) != len(want) {
		t.Fatalf("driver saw %v, want %v", alipay.refundArgs, want)
	}
	for i := range want {
		if alipay.refundArgs[i] != want[i] {
			t.Fatalf("driver arg %d = %v, want %v (full call %v)", i, alipay.refundArgs[i], want[i], alipay.refundArgs)
		}
	}

	wechat := client.inner.PayClient["wechat"].(*fakePayClient)
	if len(wechat.refundArgs) != 0 {
		t.Error("a refund for alipay must not reach the wechat driver")
	}
}

func TestRefundSurfacesDriverError(t *testing.T) {
	client := testClient(map[string]*fakePayClient{
		"alipay": {refundErr: errSignature},
	})

	if err := client.Refund("alipay", "T1", "CNY", "R1", 100, 100, "r"); !errors.Is(err, errSignature) {
		t.Errorf("err = %v, want the driver error", err)
	}
}
