package pay

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	payutilsErrors "go.gh.ink/payutils/v3/errors"
)

// maxResponseLen caps the echoed upstream payload.
const maxResponseLen = 1000

// Detail renders err with the upstream metadata payutils errors carry.
func Detail(err error) string {
	var payutilsError *payutilsErrors.PayutilsError
	if !errors.As(err, &payutilsError) {
		return err.Error()
	}

	details := make([]string, 0, 4)
	fields := make([]string, 0, 4)

	appendField(&fields, "upstream", payutilsError.UpstreamName())
	appendField(&fields, "code", payutilsError.UpstreamCode())
	appendField(&fields, "message", payutilsError.UpstreamMessage())

	if len(fields) > 0 {
		details = append(details, strings.Join(fields, " "))
	}

	if response := payutilsError.UpstreamResponse(); response != nil {
		details = append(details, "response "+compactJSON(response))
	}

	if len(details) == 0 {
		return err.Error()
	}

	return err.Error() + "\n  " + strings.Join(details, "\n  ")
}

func appendField(fields *[]string, name, value string) {
	if value != "" {
		*fields = append(*fields, name+"="+value)
	}
}

func compactJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		encoded = []byte(fmt.Sprintf("%v", value))
	}

	text := strings.ReplaceAll(string(encoded), "\n", " ")
	if len(text) > maxResponseLen {
		// Truncate on a rune boundary.
		return strings.ToValidUTF8(text[:maxResponseLen], "") + " …(截断)"
	}

	return text
}
