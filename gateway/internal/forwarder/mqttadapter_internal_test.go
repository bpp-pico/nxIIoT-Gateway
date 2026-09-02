package forwarder

import (
	"errors"
	"testing"
)

// TestIsPahoAlreadyReconnectingErr locks in the string match against paho's
// real error text (status.go's errStatusMustBeDisconnected) so a future
// paho.mqtt.golang upgrade that changes the wording fails loudly here
// instead of silently reviving the 2026-09-02 incident (watchdog
// misclassifying "paho is already reconnecting on its own" as a forced-
// reconnect failure and fighting it every tick — see MEMORY.md).
func TestIsPahoAlreadyReconnectingErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"paho's real state-transition error", errors.New("status can only transition to connecting from disconnected"), true},
		{"unrelated network error", errors.New("network Error : dial tcp: connection refused"), false},
		{"paho's already-connected/reconnecting error", errors.New("status is already connected or reconnecting"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isPahoAlreadyReconnectingErr(tc.err); got != tc.want {
				t.Errorf("isPahoAlreadyReconnectingErr(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
