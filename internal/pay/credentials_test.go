package pay

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
)

// liveCredentials builds key material the Alipay SDK accepts: the private key is
// the bare base64 body, which the driver wraps into a PEM block itself.
func liveCredentials(t *testing.T) (privateKey, certificate string) {
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

	privateKey = base64.StdEncoding.EncodeToString(x509.MarshalPKCS1PrivateKey(key))
	certificate = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))

	return privateKey, certificate
}

// yamlQuote puts text into a YAML double-quoted scalar.
func yamlQuote(text string) string {
	return `"` + strings.ReplaceAll(text, "\n", `\n`) + `"`
}

func TestBuildWiresTheRealAlipayDriver(t *testing.T) {
	privateKey, certificate := liveCredentials(t)

	load(t, "shop:\n"+
		"  alipay:\n"+
		"    app_id: \"2021000000000001\"\n"+
		"    app_cert_private_key: "+yamlQuote(privateKey)+"\n"+
		"    app_cert: "+yamlQuote(certificate)+"\n"+
		"    root_cert: "+yamlQuote(certificate)+"\n"+
		"    public_cert: "+yamlQuote(certificate)+"\n"+
		"    is_prod: true\n")

	client, err := Build("shop")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if got := client.Space(); got != "shop" {
		t.Errorf("Space() = %q", got)
	}

	upstreams := client.Upstreams()
	if len(upstreams) != 1 || upstreams[0].Name != "alipay" {
		t.Fatalf("Upstreams() = %v, want only alipay", upstreams)
	}

	// is_prod must reach the driver as "true" under its own key.
	if client.Sandboxed("alipay") {
		t.Error("isProd=true was not delivered to the driver")
	}
}

func TestBuildHonoursSandboxSwitch(t *testing.T) {
	privateKey, certificate := liveCredentials(t)

	load(t, "shop:\n"+
		"  alipay:\n"+
		"    app_id: \"2021000000000001\"\n"+
		"    app_cert_private_key: "+yamlQuote(privateKey)+"\n"+
		"    app_cert: "+yamlQuote(certificate)+"\n"+
		"    root_cert: "+yamlQuote(certificate)+"\n"+
		"    public_cert: "+yamlQuote(certificate)+"\n"+
		"    is_prod: false\n")

	client, err := Build("shop")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if !client.Sandboxed("alipay") {
		t.Error("isProd=false should report the test environment")
	}
}
