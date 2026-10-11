# refund-tool

A desktop refund tool built on payutils v3.

## Why

Refunding one order through the provider's own console means digging through
several menus and retyping a handful of identifiers every time. This collapses
it into one window: pick a space, fill in the identifiers and the amount, confirm.

## Build and run

Needs cgo and OpenGL (it is a Fyne desktop app). On macOS, the Xcode Command
Line Tools are enough:

```bash
go build -o refund-tool .
./refund-tool                 # reads config.yaml next to the binary, or in the working directory
./refund-tool -config /path/to/config.yaml
```

## Configuration

Every top-level key in `config.yaml` is a space (one merchant configuration), and
that is what the picker lists. Real credentials belong in `config_debug.yaml`
beside it: same shape, merged over the main config, and its presence is what
switches the tool into debug mode. It is git-ignored — do not commit it.

Keys inside a space are snake_case and are unmarshalled into structs in one pass
by `internal/config`. `log` is a reserved top-level section name, so do not name
a space that. The camelCase credential keys the drivers ask for are translated
one by one in `internal/pay` — see `pay.Upstreams`.

| Channel | Section | Keys |
| --- | --- | --- |
| Alipay | `alipay` | `app_id` `app_cert_private_key` `app_cert` `root_cert` `public_cert` `is_prod` |
| WeChat Pay | `wechat_pay` | `app_id` `app_secret` `merchant_id` `merchant_api_v3_key` `merchant_cert_serial_number` `merchant_private_key` `public_key` `public_key_id` |

Three things worth knowing:

- `app_cert_private_key` takes the bare base64 body from the Alipay console, without
  the `-----BEGIN/END-----` markers. The driver wraps it into a PEM block itself, so
  passing a PEM through corrupts it.
- WeChat Pay's `public_key` and `public_key_id` are required; without them the client
  cannot be built at all.
- `app_secret` is parsed and kept in the config struct only. The WeChat Pay driver
  takes no such credential, so it is never sent.

## Behaviour

- **Channel detection**: with the auto-detect entry selected in the channel picker,
  each configured channel is queried first and only the one that knows the trade is
  refunded. If none knows it, the tool stops; if both do, it asks for an explicit
  choice, so nothing is refunded twice.
- **Notify URL**: WeChat Pay refund requests carry `notify_url = <base> +
  "/wechat/callback"`, with the base hard-coded in `internal/pay` rather than read
  from the config. The tool does not listen for callbacks and reads the outcome
  from the synchronous response.
- **Amounts**: Alipay only supports `CNY`, and the refund amount may not exceed the
  order total. The confirmation dialog shows yuan and cents side by side.
- **Logging**: the log pane at the bottom is the operator stream. When `log.file` is
  set, the same events also go to that file through zap (JSON, size-rotated).

## Tests

```bash
go test ./...
```

Covered: the yuan-to-cents parser (mutation-checked), config loading and space
enumeration, credential wiring (against the real Alipay driver using generated
certificates), form validation, and the asynchronous task callbacks. The refund
call itself is not covered — that needs live merchant credentials.
