//go:build live

package edistribucion

import (
	"strings"
	"testing"
)

func TestLiveLoginCanary(t *testing.T) {
	client := NewClient("go-edistribucion-canary.invalid", "not-a-real-password")

	err := client.Login()
	if err == nil {
		t.Fatal("login with canary credentials unexpectedly succeeded")
	}
	if !strings.HasPrefix(err.Error(), "login failed:") {
		t.Fatalf("live login protocol failed before credential rejection: %v", err)
	}
}
