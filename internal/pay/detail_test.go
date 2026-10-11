package pay

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	payutilsErrors "go.gh.ink/payutils/v3/errors"
)

func TestDetailExpandsUpstreamMetadata(t *testing.T) {
	err := payutilsErrors.ErrTradeNotExist.
		WithUpstreamName("alipay").
		WithUpstreamCode("ACQ.TRADE_NOT_EXIST").
		WithUpstreamMessage("交易不存在").
		WithUpstreamResponse(map[string]string{"code": "40004", "msg": "Business Failed"})

	got := Detail(err)

	for _, want := range []string{
		"trade not exist",
		"upstream=alipay",
		"code=ACQ.TRADE_NOT_EXIST",
		"message=交易不存在",
		`response {"code":"40004"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Detail() = %q, want it to carry %q", got, want)
		}
	}
}

func TestDetailOmitsEmptyFields(t *testing.T) {
	got := Detail(payutilsErrors.ErrUnsupportedCurrency)

	if got != "unsupported currency" {
		t.Errorf("Detail() = %q, want just the sentinel text", got)
	}
	if strings.Contains(got, "upstream") || strings.Contains(got, "response") {
		t.Errorf("Detail() = %q, want no empty metadata lines", got)
	}
}

func TestDetailPassesThroughForeignErrors(t *testing.T) {
	native := errors.New("dial tcp: lookup failed")

	if got := Detail(native); got != "dial tcp: lookup failed" {
		t.Errorf("Detail() = %q, want the raw error text", got)
	}

	got := Detail(fmt.Errorf("查询：%.0w", payutilsErrors.ErrTradeNotExist.WithUpstreamName("wechat")))
	if !strings.Contains(got, "upstream=wechat") {
		t.Errorf("Detail() = %q, want metadata of a wrapped payutils error", got)
	}
}

func TestDetailTruncatesALongResponse(t *testing.T) {
	err := payutilsErrors.ErrTradeNotExist.WithUpstreamResponse(strings.Repeat("数", 900))

	got := Detail(err)

	if !strings.Contains(got, "…(截断)") {
		t.Errorf("Detail() = %q, want the truncation marker", got)
	}
	if strings.ContainsRune(got, utf8.RuneError) {
		t.Error("Detail() cut a multi-byte rune in half")
	}
	if lines := strings.Split(got, "\n"); len(lines) < 2 || len(lines[1]) > maxResponseLen+40 {
		t.Errorf("Detail() response line is %d bytes, want it capped near %d",
			len(lines[len(lines)-1]), maxResponseLen)
	}
}
