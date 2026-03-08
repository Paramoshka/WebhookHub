package hmacsig

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultIncomingHeader   = "Stripe-Signature"
	DefaultToleranceSeconds = 300
	OutgoingHeader          = "X-WebhookHub-Signature"
)

var (
	ErrMissingTimestamp = errors.New("missing timestamp")
	ErrMissingSignature = errors.New("missing v1 signature")
	ErrInvalidSignature = errors.New("invalid signature")
	ErrTimestampExpired = errors.New("timestamp outside tolerance")
)

func SignHeader(secret string, payload []byte, now time.Time) string {
	timestamp := now.Unix()
	return fmt.Sprintf("t=%d, v1=%s", timestamp, signature(secret, timestamp, payload))
}

func VerifyHeader(secret, header string, payload []byte, now time.Time, tolerance time.Duration) error {
	timestamp, candidates, err := parseHeader(header)
	if err != nil {
		return err
	}

	if tolerance > 0 {
		delta := now.Unix() - timestamp
		if delta < 0 {
			delta = -delta
		}
		if time.Duration(delta)*time.Second > tolerance {
			return ErrTimestampExpired
		}
	}

	expected, err := hex.DecodeString(signature(secret, timestamp, payload))
	if err != nil {
		return err
	}

	for _, candidate := range candidates {
		decoded, err := hex.DecodeString(candidate)
		if err != nil {
			continue
		}
		if hmac.Equal(decoded, expected) {
			return nil
		}
	}

	return ErrInvalidSignature
}

func signature(secret string, timestamp int64, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(timestamp, 10)))
	mac.Write([]byte("."))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func parseHeader(header string) (int64, []string, error) {
	var (
		timestamp int64
		foundTime bool
		values    []string
	)

	for _, part := range strings.Split(header, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}

		switch strings.TrimSpace(key) {
		case "t":
			parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
			if err != nil {
				return 0, nil, ErrMissingTimestamp
			}
			timestamp = parsed
			foundTime = true
		case "v1":
			value = strings.TrimSpace(value)
			if value != "" {
				values = append(values, value)
			}
		}
	}

	if !foundTime {
		return 0, nil, ErrMissingTimestamp
	}
	if len(values) == 0 {
		return 0, nil, ErrMissingSignature
	}

	return timestamp, values, nil
}
