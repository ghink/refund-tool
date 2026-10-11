package money

import (
	"testing"
)

func TestParseYuan(t *testing.T) {
	cases := []struct {
		name string
		text string
		want int64
	}{
		{"whole yuan", "12", 1200},
		{"two decimals", "12.34", 1234},
		{"one decimal", "12.3", 1230},
		{"trailing dot", "12.", 1200},
		{"leading dot", ".5", 50},
		{"cents only", "0.05", 5},
		{"zero", "0", 0},
		{"zero with decimals", "0.00", 0},
		{"surrounding spaces", "  8.80  ", 880},
		{"tab", "\t1.05\n", 105},
		{"explicit plus", "+2.5", 250},
		{"explicit minus", "-1.00", -100},
		{"minus below a yuan", "-0.99", -99},
		{"large", "1234567890123.45", 123456789012345},
		{"max int64 cents", "92233720368547758.07", 9223372036854775807},
		{"many leading zeros", "0000012.30", 1230},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseYuan(c.text)
			if err != nil {
				t.Fatalf("ParseYuan(%q) returned error: %v", c.text, err)
			}
			if got != c.want {
				t.Errorf("ParseYuan(%q) = %d, want %d", c.text, got, c.want)
			}
		})
	}
}

func TestParseYuanErrors(t *testing.T) {
	cases := []struct {
		name string
		text string
		want error
	}{
		{"empty", "", ErrEmpty},
		{"blank", "   ", ErrEmpty},
		{"sign only", "-", ErrInvalid},
		{"plus only", "+", ErrInvalid},
		{"dot only", ".", ErrInvalid},
		{"dots only", "..", ErrInvalid},
		{"letters", "abc", ErrInvalid},
		{"word number", "十二", ErrInvalid},
		{"thousands separator", "1,234.56", ErrInvalid},
		{"space in middle", "1 000", ErrInvalid},
		{"currency symbol", "¥12.00", ErrInvalid},
		{"exponent", "1e3", ErrInvalid},
		{"two dots", "1.2.3", ErrInvalid},
		{"inner dot in fraction", "1..5", ErrInvalid},
		{"scientific", "1.5E2", ErrInvalid},
		{"three decimals", "12.345", ErrTooPrecise},
		{"four decimals", "0.0001", ErrTooPrecise},
		{"overflow whole", "92233720368547759", ErrOverflow},
		{"overflow by scaling", "92233720368547758.08", ErrOverflow},
		{"overflow way past", "99999999999999999999999999", ErrOverflow},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseYuan(c.text)
			if err != c.want {
				t.Errorf("ParseYuan(%q) = (%d, %v), want error %v", c.text, got, err, c.want)
			}
			if got != 0 && err != nil {
				t.Errorf("ParseYuan(%q) returned %d alongside error %v, want 0", c.text, got, err)
			}
		})
	}
}

func TestParseYuanNeverRounds(t *testing.T) {
	// Amounts that cannot be represented exactly in cents are rejected, not rounded.
	for _, text := range []string{"0.001", "1.999", "12.349"} {
		if got, err := ParseYuan(text); err != ErrTooPrecise {
			t.Errorf("ParseYuan(%q) = (%d, %v), want ErrTooPrecise", text, got, err)
		}
	}
}
