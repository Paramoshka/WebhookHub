package hmacsig

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestSignHeaderVerifySuccess(t *testing.T) {
	now := time.Unix(1710000000, 0)
	payload := []byte(`{"id":"evt_123"}`)
	header := SignHeader("whsec_test", payload, now)

	err := VerifyHeader("whsec_test", header, payload, now.Add(10*time.Second), 5*time.Minute)
	if err != nil {
		t.Fatalf("VerifyHeader returned error: %v", err)
	}
}

func TestVerifyHeaderRejectsWrongSecret(t *testing.T) {
	now := time.Unix(1710000000, 0)
	payload := []byte(`{"id":"evt_123"}`)
	header := SignHeader("whsec_test", payload, now)

	err := VerifyHeader("wrong_secret", header, payload, now, 5*time.Minute)
	if !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature, got %v", err)
	}
}

func TestVerifyHeaderRejectsExpiredTimestamp(t *testing.T) {
	now := time.Unix(1710000000, 0)
	payload := []byte(`{"id":"evt_123"}`)
	header := SignHeader("whsec_test", payload, now)

	err := VerifyHeader("whsec_test", header, payload, now.Add(10*time.Minute), 5*time.Minute)
	if !errors.Is(err, ErrTimestampExpired) {
		t.Fatalf("expected ErrTimestampExpired, got %v", err)
	}
}

func TestVerifyHeaderAcceptsAnyMatchingV1Signature(t *testing.T) {
	now := time.Unix(1710000000, 0)
	payload := []byte(`{"id":"evt_123"}`)
	valid := SignHeader("whsec_test", payload, now)
	header := fmt.Sprintf("%s, v1=deadbeef", valid)

	err := VerifyHeader("whsec_test", header, payload, now, 5*time.Minute)
	if err != nil {
		t.Fatalf("VerifyHeader returned error: %v", err)
	}
}
