package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

// Alipay credentials of one space, in config spelling.
type Alipay struct {
	AppID             string `mapstructure:"app_id"`
	AppCertPrivateKey string `mapstructure:"app_cert_private_key"`
	AppCert           string `mapstructure:"app_cert"`
	RootCert          string `mapstructure:"root_cert"`
	PublicCert        string `mapstructure:"public_cert"`
	IsProd            bool   `mapstructure:"is_prod"`
}

// WeChatPay credentials of one space, in config spelling.
type WeChatPay struct {
	AppID                    string `mapstructure:"app_id"`
	AppSecret                string `mapstructure:"app_secret"`
	MerchantID               string `mapstructure:"merchant_id"`
	MerchantAPIv3Key         string `mapstructure:"merchant_api_v3_key"`
	MerchantCertSerialNumber string `mapstructure:"merchant_cert_serial_number"`
	MerchantPrivateKey       string `mapstructure:"merchant_private_key"`
	PublicKey                string `mapstructure:"public_key"`
	PublicKeyID              string `mapstructure:"public_key_id"`
}

// Space is one merchant configuration.
type Space struct {
	Prefix    string    `mapstructure:"prefix"`
	Suffix    string    `mapstructure:"suffix"`
	Alipay    Alipay    `mapstructure:"alipay"`
	WeChatPay WeChatPay `mapstructure:"wechat_pay"`
}

// FileLog is the optional rolling file sink section.
type FileLog struct {
	File       string `mapstructure:"file"`
	MaxSize    int    `mapstructure:"max_size"`
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAge     int    `mapstructure:"max_age"`
	Compress   bool   `mapstructure:"compress"`
}

var (
	// Spaces holds every configured space, keyed by name.
	Spaces = map[string]*Space{}

	// Log is the file sink section.
	Log FileLog

	// Debug is set when config_debug.yaml was merged over the main config.
	Debug = false

	path = ""
)

// doc decodes the named sections; any other top-level key is a space.
type doc struct {
	Log    FileLog        `mapstructure:"log"`
	Spaces map[string]any `mapstructure:",remain"`
}

// Load reads the config file, merging config_debug.yaml from the same directory.
// explicitPath may point at a specific file instead.
func Load(explicitPath string) error {
	v := viper.New()
	v.SetConfigType("yaml")

	if explicitPath != "" {
		v.SetConfigFile(explicitPath)
	} else {
		v.SetConfigName("config")
		v.AddConfigPath(executableDir())
		v.AddConfigPath(".")
	}

	if err := v.ReadInConfig(); err != nil {
		return err
	}

	Debug = false
	if overlay, err := os.Open(debugPathOf(v)); err == nil {
		defer overlay.Close()
		if err = v.MergeConfig(overlay); err != nil {
			return err
		}
		Debug = true
	}

	parsed := doc{}
	if err := v.Unmarshal(&parsed, weaklyTyped); err != nil {
		return fmt.Errorf("parse %s: %w", v.ConfigFileUsed(), err)
	}

	spaces, err := parseSpaces(parsed.Spaces)
	if err != nil {
		return err
	}

	Log = parsed.Log
	Spaces = spaces
	path = v.ConfigFileUsed()

	return nil
}

// Path is the config file that was read.
func Path() string { return path }

// SpaceNames lists the configured spaces, sorted.
func SpaceNames() []string {
	names := make([]string, 0, len(Spaces))
	for name := range Spaces {
		names = append(names, name)
	}
	sort.Strings(names)

	return names
}

func parseSpaces(raw map[string]any) (map[string]*Space, error) {
	spaces := make(map[string]*Space, len(raw))

	for name, entry := range raw {
		space := &Space{}
		if err := decode(entry, space); err != nil {
			return nil, fmt.Errorf("space %q: %w", name, err)
		}
		spaces[name] = space
	}

	return spaces, nil
}

// weaklyTyped accepts an unquoted numeric app id into a string field.
func weaklyTyped(dc *mapstructure.DecoderConfig) {
	dc.WeaklyTypedInput = true
}

func decode(raw any, target any) error {
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           target,
		WeaklyTypedInput: true,
	})
	if err != nil {
		return err
	}

	return decoder.Decode(raw)
}

func debugPathOf(v *viper.Viper) string {
	return filepath.Join(filepath.Dir(v.ConfigFileUsed()), "config_debug.yaml")
}

func executableDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	if evaluated, err := filepath.EvalSymlinks(exe); err == nil {
		exe = evaluated
	}
	return filepath.Dir(exe)
}
