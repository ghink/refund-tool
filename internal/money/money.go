package money

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

var (
	// ErrEmpty is returned for an empty amount.
	ErrEmpty = errors.New("amount is empty")
	// ErrInvalid is returned for anything that is not a plain decimal amount.
	ErrInvalid = errors.New("amount is not a valid decimal")
	// ErrTooPrecise is returned when more than 2 decimal places are given.
	ErrTooPrecise = errors.New("amount has more than 2 decimal places")
	// ErrOverflow is returned when the amount does not fit into int64 cents.
	ErrOverflow = errors.New("amount is out of range")
)

// ParseYuan converts a decimal yuan amount into cents.
func ParseYuan(text string) (int64, error) {
	amount := strings.TrimSpace(text)
	if amount == "" {
		return 0, ErrEmpty
	}

	negative := false
	switch amount[0] {
	case '-':
		negative = true
		amount = amount[1:]
	case '+':
		amount = amount[1:]
	}
	if amount == "" {
		return 0, ErrInvalid
	}

	whole, fraction, _ := strings.Cut(amount, ".")
	if whole == "" && fraction == "" {
		return 0, ErrInvalid
	}
	if !isDigits(whole) || !isDigits(fraction) {
		return 0, ErrInvalid
	}
	if len(fraction) > 2 {
		return 0, ErrTooPrecise
	}

	// ".5" is a valid shorthand for "0.5".
	var wholeValue int64
	if whole != "" {
		parsed, err := strconv.ParseInt(whole, 10, 64)
		if err != nil {
			return 0, ErrOverflow
		}
		wholeValue = parsed
	}

	fractionValue, err := strconv.ParseInt(fraction+"00"[len(fraction):], 10, 64)
	if err != nil {
		return 0, ErrOverflow
	}

	if wholeValue > (math.MaxInt64-fractionValue)/100 {
		return 0, ErrOverflow
	}

	if wholeValue > (math.MaxInt64-fractionValue)/100 {
		return 0, ErrOverflow
	}

	cents := wholeValue*100 + fractionValue
	if negative {
		cents = -cents
	}

	return cents, nil
}

func isDigits(text string) bool {
	for _, r := range text {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
