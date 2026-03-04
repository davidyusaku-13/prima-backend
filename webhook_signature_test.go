package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"strings"
	"testing"
	"time"
)

func buildSvixSignature(secret string, body []byte, svixID string, ts int64) string {
	split := strings.SplitN(secret, "_", 2)
	key, err := base64.StdEncoding.DecodeString(split[1])
	if err != nil {
		panic(err)
	}

	msg := svixID + "." + strconv.FormatInt(ts, 10) + "." + string(body)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(msg))
	return "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func TestVerifySvixSignature_AcceptsFreshSignature(t *testing.T) {
	t.Parallel()

	secret := "whsec_" + base64.StdEncoding.EncodeToString([]byte("test-secret-key"))
	body := []byte(`{"type":"session.created"}`)
	now := time.Unix(1_730_000_000, 0).UTC()
	timestamp := now.Unix()
	signature := buildSvixSignature(secret, body, "msg_1", timestamp)

	if !verifySvixSignature(body, secret, "msg_1", strconv.FormatInt(timestamp, 10), signature, now) {
		t.Fatal("expected valid fresh signature")
	}
}

func TestVerifySvixSignature_RejectsStaleTimestamp(t *testing.T) {
	t.Parallel()

	secret := "whsec_" + base64.StdEncoding.EncodeToString([]byte("test-secret-key"))
	body := []byte(`{"type":"session.created"}`)
	now := time.Unix(1_730_000_000, 0).UTC()
	timestamp := now.Add(-10 * time.Minute).Unix()
	signature := buildSvixSignature(secret, body, "msg_1", timestamp)

	if verifySvixSignature(body, secret, "msg_1", strconv.FormatInt(timestamp, 10), signature, now) {
		t.Fatal("expected stale timestamp to be rejected")
	}
}

func TestVerifySvixSignature_RejectsInvalidSignature(t *testing.T) {
	t.Parallel()

	secret := "whsec_" + base64.StdEncoding.EncodeToString([]byte("test-secret-key"))
	body := []byte(`{"type":"session.created"}`)
	now := time.Unix(1_730_000_000, 0).UTC()
	timestamp := now.Unix()

	if verifySvixSignature(body, secret, "msg_1", strconv.FormatInt(timestamp, 10), "v1,invalid", now) {
		t.Fatal("expected invalid signature to be rejected")
	}
}
