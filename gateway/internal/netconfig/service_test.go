package netconfig_test

import (
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"nxiiot-gateway/internal/netconfig"
)

// fakeController is an in-memory Controller so Service's confirm/revert
// timer logic can be tested deterministically without nmcli.
type fakeController struct {
	mu      sync.Mutex
	current netconfig.Info
	applies []netconfig.StaticConfig
}

func (c *fakeController) Current() (netconfig.Info, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.current, nil
}

func (c *fakeController) ApplyStatic(cfg netconfig.StaticConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.applies = append(c.applies, cfg)
	c.current = netconfig.Info{
		Interface: cfg.Interface, Method: "manual",
		Address: cfg.Address, Prefix: cfg.Prefix, Gateway: cfg.Gateway, DNS: cfg.DNS,
	}
	return nil
}

func (c *fakeController) ApplyDHCP(iface string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.current = netconfig.Info{Interface: iface, Method: "auto"}
	return nil
}

func (c *fakeController) applyCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.applies)
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestServiceRevertsUnconfirmedChangeAfterWindow(t *testing.T) {
	ctrl := &fakeController{current: netconfig.Info{Interface: "eth0", Method: "auto"}}
	svc := netconfig.NewService(ctrl, testLogger())

	err := svc.Apply(netconfig.StaticConfig{Interface: "eth0", Address: "192.168.1.99", Prefix: 24, Gateway: "192.168.1.1"}, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	got, _ := ctrl.Current()
	if got.Address != "192.168.1.99" {
		t.Fatalf("expected the static config applied immediately, got %+v", got)
	}
	if !svc.Pending() {
		t.Fatal("expected a pending revert immediately after Apply")
	}

	time.Sleep(250 * time.Millisecond)

	got, _ = ctrl.Current()
	if got.Method != "auto" {
		t.Fatalf("expected revert to DHCP after the confirm window elapsed, got %+v", got)
	}
	if svc.Pending() {
		t.Fatal("expected no pending revert after it already fired")
	}
}

func TestServiceKeepsChangeWhenConfirmedInTime(t *testing.T) {
	ctrl := &fakeController{current: netconfig.Info{Interface: "eth0", Method: "auto"}}
	svc := netconfig.NewService(ctrl, testLogger())

	if err := svc.Apply(netconfig.StaticConfig{Interface: "eth0", Address: "192.168.1.99", Prefix: 24, Gateway: "192.168.1.1"}, 150*time.Millisecond); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if err := svc.Confirm(); err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if svc.Pending() {
		t.Fatal("expected no pending revert right after Confirm")
	}

	time.Sleep(250 * time.Millisecond) // past the original confirm window

	got, _ := ctrl.Current()
	if got.Address != "192.168.1.99" {
		t.Fatalf("expected the confirmed static config to survive, got %+v", got)
	}
	if applyCount := ctrl.applyCount(); applyCount != 1 {
		t.Fatalf("expected exactly 1 apply (no revert), got %d", applyCount)
	}
}

func TestServiceConfirmFailsWhenNothingPending(t *testing.T) {
	ctrl := &fakeController{current: netconfig.Info{Interface: "eth0", Method: "auto"}}
	svc := netconfig.NewService(ctrl, testLogger())

	if err := svc.Confirm(); err == nil {
		t.Fatal("expected Confirm to fail when there is nothing to confirm")
	}
}

func TestServiceApplyDHCPSwitchesImmediatelyAndArmsRevert(t *testing.T) {
	ctrl := &fakeController{current: netconfig.Info{
		Interface: "eth0", Method: "manual", Address: "192.168.1.10", Prefix: 24, Gateway: "192.168.1.1",
	}}
	svc := netconfig.NewService(ctrl, testLogger())

	if err := svc.ApplyDHCP("eth0", 150*time.Millisecond); err != nil {
		t.Fatalf("ApplyDHCP: %v", err)
	}

	got, _ := ctrl.Current()
	if got.Method != "auto" {
		t.Fatalf("expected DHCP applied immediately, got %+v", got)
	}
	if !svc.Pending() {
		t.Fatal("expected a pending revert immediately after ApplyDHCP")
	}
}

func TestServiceRevertsUnconfirmedDHCPChangeBackToPreviousStatic(t *testing.T) {
	// Switching to DHCP carries the same lockout risk as a bad static
	// apply (losing a fixed address the device depends on) — an
	// unconfirmed switch must revert back to the prior static config.
	ctrl := &fakeController{current: netconfig.Info{
		Interface: "eth0", Method: "manual", Address: "192.168.1.10", Prefix: 24, Gateway: "192.168.1.1",
	}}
	svc := netconfig.NewService(ctrl, testLogger())

	if err := svc.ApplyDHCP("eth0", 80*time.Millisecond); err != nil {
		t.Fatalf("ApplyDHCP: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	got, _ := ctrl.Current()
	if got.Method != "manual" || got.Address != "192.168.1.10" {
		t.Fatalf("expected revert to the original static config 192.168.1.10, got %+v", got)
	}
	if svc.Pending() {
		t.Fatal("expected no pending revert after it already fired")
	}
}

func TestServiceKeepsDHCPWhenConfirmedInTime(t *testing.T) {
	ctrl := &fakeController{current: netconfig.Info{
		Interface: "eth0", Method: "manual", Address: "192.168.1.10", Prefix: 24, Gateway: "192.168.1.1",
	}}
	svc := netconfig.NewService(ctrl, testLogger())

	if err := svc.ApplyDHCP("eth0", 150*time.Millisecond); err != nil {
		t.Fatalf("ApplyDHCP: %v", err)
	}
	if err := svc.Confirm(); err != nil {
		t.Fatalf("Confirm: %v", err)
	}

	time.Sleep(250 * time.Millisecond) // past the original confirm window

	got, _ := ctrl.Current()
	if got.Method != "auto" {
		t.Fatalf("expected the confirmed DHCP switch to survive, got %+v", got)
	}
}

func TestServiceRevertsToPreviousStaticConfigNotJustDHCP(t *testing.T) {
	// Starting from an existing static config (not DHCP), an unconfirmed
	// change must revert back to that exact prior static config — not
	// fall back to DHCP, which would be a different, unrequested change.
	ctrl := &fakeController{current: netconfig.Info{
		Interface: "eth0", Method: "manual", Address: "192.168.1.10", Prefix: 24, Gateway: "192.168.1.1",
	}}
	svc := netconfig.NewService(ctrl, testLogger())

	if err := svc.Apply(netconfig.StaticConfig{Interface: "eth0", Address: "192.168.1.99", Prefix: 24, Gateway: "192.168.1.1"}, 80*time.Millisecond); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	got, _ := ctrl.Current()
	if got.Address != "192.168.1.10" {
		t.Fatalf("expected revert to the original static address 192.168.1.10, got %+v", got)
	}
}
