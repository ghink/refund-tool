package pay

import (
	"errors"
	"fmt"
	"strconv"

	payalipay "go.gh.ink/payutils/pay/alipay/v3"
	paywechat "go.gh.ink/payutils/pay/wechat/v3"
	payutilsClient "go.gh.ink/payutils/v3/client"
	payutilsErrors "go.gh.ink/payutils/v3/errors"
	"go.gh.ink/payutils/v3/model"

	"refundtool/internal/config"
)

// Upstream binds a payutils driver to the space fields that feed it.
type Upstream struct {
	// Name is the payutils driver name, and the key of model.Credentials.
	Name string
	// Display is the channel label shown to the operator.
	Display string
	// HasSandboxSwitch marks channels whose config selects live or test.
	HasSandboxSwitch bool

	// present reports whether a space carries this channel.
	present func(*config.Space) bool
	// credentials maps the space onto the driver's credential keys.
	credentials func(*config.Space) map[string]string
}

var (
	alipay = Upstream{
		Name:             payalipay.Name,
		Display:          "支付宝",
		HasSandboxSwitch: true,
		present:          func(s *config.Space) bool { return s.Alipay.AppID != "" },
		credentials: func(s *config.Space) map[string]string {
			return map[string]string{
				payalipay.AppID:             s.Alipay.AppID,
				payalipay.AppCertPrivateKey: s.Alipay.AppCertPrivateKey,
				payalipay.AppCert:           s.Alipay.AppCert,
				payalipay.RootCert:          s.Alipay.RootCert,
				payalipay.PublicCert:        s.Alipay.PublicCert,
				payalipay.IsProd:            strconv.FormatBool(s.Alipay.IsProd),
			}
		},
	}

	wechat = Upstream{
		Name:    paywechat.Name,
		Display: "微信",
		present: func(s *config.Space) bool { return s.WeChatPay.MerchantID != "" },
		credentials: func(s *config.Space) map[string]string {
			return map[string]string{
				paywechat.AppID:                    s.WeChatPay.AppID,
				paywechat.MerchantID:               s.WeChatPay.MerchantID,
				paywechat.MerchantAPIv3Key:         s.WeChatPay.MerchantAPIv3Key,
				paywechat.MerchantCertSerialNumber: s.WeChatPay.MerchantCertSerialNumber,
				paywechat.MerchantPrivateKey:       s.WeChatPay.MerchantPrivateKey,
				paywechat.PublicKey:                s.WeChatPay.PublicKey,
				paywechat.PublicKeyID:              s.WeChatPay.PublicKeyID,
			}
		},
	}

	// Upstreams is every channel this tool can wire.
	Upstreams = []Upstream{alipay, wechat}
)

var (
	// ErrNoSpace is returned when the config does not define the space.
	ErrNoSpace = errors.New("space not found")
	// ErrNoUpstream is returned when a space configures no payment channel.
	ErrNoUpstream = errors.New("no payment channel configured")
	// ErrTradeNotFound is returned when no configured channel knows the trade.
	ErrTradeNotFound = errors.New("trade not found on any configured channel")
	// ErrTradeAmbiguous is returned when more than one channel knows the trade.
	ErrTradeAmbiguous = errors.New("trade found on more than one channel")
)

// endpoint is the base payutils puts in notify URLs: WeChat refunds carry "/wechat/callback".
const endpoint = "https://www.ghink.net"

// TradeAbsent reports whether err means the channel has no such trade.
func TradeAbsent(err error) bool {
	return errors.Is(err, payutilsErrors.ErrTradeNotExist)
}

// Client is a payutils client bound to one space.
type Client struct {
	space      string
	upstreams  []Upstream
	credential map[string]map[string]string
	inner      *payutilsClient.Client
}

// spaceSettings is one space as read from config.
type spaceSettings struct {
	prefix      string
	suffix      string
	upstreams   []Upstream
	credentials model.C
}

// loadSpace picks up the channels a space configured.
func loadSpace(name string) (*spaceSettings, error) {
	space, ok := config.Spaces[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNoSpace, name)
	}

	settings := &spaceSettings{
		prefix:      space.Prefix,
		suffix:      space.Suffix,
		credentials: model.C{},
	}

	for _, upstream := range Upstreams {
		if !upstream.present(space) {
			continue
		}

		settings.credentials[upstream.Name] = upstream.credentials(space)
		settings.upstreams = append(settings.upstreams, upstream)
	}

	if len(settings.upstreams) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrNoUpstream, name)
	}

	return settings, nil
}

// Build wires a payutils client for the named space, without callback routes.
func Build(space string) (*Client, error) {
	settings, err := loadSpace(space)
	if err != nil {
		return nil, err
	}

	inner, err := payutilsClient.NewClient(model.Config{
		Debug:         config.Debug,
		Endpoint:      endpoint,
		TradeIDPrefix: settings.prefix,
		TradeIDSuffix: settings.suffix,
		Credentials:   settings.credentials,
	})
	if err != nil {
		return nil, err
	}

	return &Client{
		space:      space,
		upstreams:  settings.upstreams,
		credential: settings.credentials,
		inner:      inner,
	}, nil
}

// Space is the configured space this client was built from.
func (c *Client) Space() string { return c.space }

// Upstreams are the channels this space configured.
func (c *Client) Upstreams() []Upstream { return c.upstreams }

// Sandboxed reports whether a channel points at a test environment.
func (c *Client) Sandboxed(upstream string) bool {
	if upstream != payalipay.Name {
		return false
	}
	return c.credential[upstream][payalipay.IsProd] != payalipay.True
}

// Status queries one channel for a trade.
func (c *Client) Status(upstream, tradeID string) (model.ReturnStatus, error) {
	result, err := c.inner.Status([]string{upstream}, tradeID)
	if err != nil {
		return model.ReturnStatus{}, err
	}
	return result[upstream], nil
}

// Detection is the single channel that knows a trade.
type Detection struct {
	Upstream Upstream
	Status   model.ReturnStatus
}

// Detect finds which configured channel owns tradeID. Any channel error other
// than "trade not exist" aborts the detection.
func (c *Client) Detect(tradeID string) (Detection, error) {
	var found []Detection

	for _, upstream := range c.upstreams {
		status, err := c.Status(upstream.Name, tradeID)
		switch {
		case err == nil:
			found = append(found, Detection{Upstream: upstream, Status: status})
		case errors.Is(err, payutilsErrors.ErrTradeNotExist):
		default:
			return Detection{}, err
		}
	}

	switch len(found) {
	case 0:
		return Detection{}, fmt.Errorf("%w: %s", ErrTradeNotFound, tradeID)
	case 1:
		return found[0], nil
	default:
		return Detection{}, fmt.Errorf("%w: %s", ErrTradeAmbiguous, tradeID)
	}
}

// Refund refunds tradeID on one channel.
func (c *Client) Refund(
	upstream, tradeID, currency, refundID string,
	totalAmount, refundAmount int64,
	reason string,
) error {
	for _, configured := range c.upstreams {
		if configured.Name == upstream {
			return c.inner.Refund(
				[]string{upstream}, tradeID,
				currency, refundID,
				totalAmount, refundAmount,
				reason,
			)
		}
	}

	return payutilsErrors.ErrUpstreamNotFound.WithUpstreamName(upstream)
}
