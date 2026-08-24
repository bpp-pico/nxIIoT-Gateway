package netconfig

import (
	"errors"
	"strings"
	"testing"
)

// fakeNmcli routes canned "nmcli <args>" output by matching on the args
// actually passed, so tests exercise the real parsing code in netconfig.go
// against realistic `nmcli -t ...` terse output, without needing nmcli
// installed (it never is, on this Windows dev host).
func fakeNmcli(t *testing.T, responses map[string]string) *nmcliController {
	t.Helper()
	return &nmcliController{
		lookPath: func(string) (string, error) { return "/usr/bin/nmcli", nil },
		run: func(args ...string) (string, error) {
			key := strings.Join(args, " ")
			if out, ok := responses[key]; ok {
				return out, nil
			}
			t.Fatalf("unexpected nmcli invocation: %v", args)
			return "", nil
		},
	}
}

func TestCurrentParsesRealisticNmcliOutput(t *testing.T) {
	c := fakeNmcli(t, map[string]string{
		"-t -f DEVICE,TYPE,STATE device status": "eth0:ethernet:connected\nwlan0:wifi:disconnected\nlo:loopback:unmanaged\n",
		"-t -f GENERAL.CONNECTION,IP4.ADDRESS,IP4.GATEWAY,IP4.DNS device show eth0": "GENERAL.CONNECTION:Wired connection 1\n" +
			"IP4.ADDRESS[1]:192.168.1.50/24\n" +
			"IP4.GATEWAY:192.168.1.1\n" +
			"IP4.DNS[1]:8.8.8.8\n" +
			"IP4.DNS[2]:1.1.1.1\n",
		`-t -f ipv4.method connection show Wired connection 1`: "ipv4.method:auto\n",
	})

	info, err := c.Current()
	if err != nil {
		t.Fatalf("Current: %v", err)
	}

	if info.Interface != "eth0" {
		t.Errorf("Interface = %q, want eth0", info.Interface)
	}
	if info.Address != "192.168.1.50" || info.Prefix != 24 {
		t.Errorf("Address/Prefix = %q/%d, want 192.168.1.50/24", info.Address, info.Prefix)
	}
	if info.Gateway != "192.168.1.1" {
		t.Errorf("Gateway = %q, want 192.168.1.1", info.Gateway)
	}
	if len(info.DNS) != 2 || info.DNS[0] != "8.8.8.8" || info.DNS[1] != "1.1.1.1" {
		t.Errorf("DNS = %v, want [8.8.8.8 1.1.1.1]", info.DNS)
	}
	if info.Method != "auto" {
		t.Errorf("Method = %q, want auto", info.Method)
	}
}

func TestCurrentSkipsDisconnectedAndUnmanagedInterfaces(t *testing.T) {
	c := fakeNmcli(t, map[string]string{
		"-t -f DEVICE,TYPE,STATE device status": "eth0:ethernet:disconnected\nwlan0:wifi:connected\nlo:loopback:unmanaged\n",
		"-t -f GENERAL.CONNECTION,IP4.ADDRESS,IP4.GATEWAY,IP4.DNS device show wlan0": "GENERAL.CONNECTION:Home Wifi\n" +
			"IP4.ADDRESS[1]:10.0.0.5/24\n" +
			"IP4.GATEWAY:10.0.0.1\n",
		"-t -f ipv4.method connection show Home Wifi": "ipv4.method:manual\n",
	})

	info, err := c.Current()
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if info.Interface != "wlan0" {
		t.Errorf("Interface = %q, want wlan0 (the connected one, not the disconnected eth0)", info.Interface)
	}
}

func TestCurrentReturnsErrUnsupportedWhenNmcliMissing(t *testing.T) {
	c := &nmcliController{
		lookPath: func(string) (string, error) { return "", errors.New("not found") },
		run: func(args ...string) (string, error) {
			t.Fatal("run should not be called when nmcli is unavailable")
			return "", nil
		},
	}

	_, err := c.Current()
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Current() error = %v, want ErrUnsupported", err)
	}
}

func TestApplyStaticSendsCorrectNmcliArgs(t *testing.T) {
	var modArgs, upArgs []string
	c := &nmcliController{
		lookPath: func(string) (string, error) { return "/usr/bin/nmcli", nil },
		run: func(args ...string) (string, error) {
			key := strings.Join(args, " ")
			switch {
			case key == "-t -f GENERAL.CONNECTION device show eth0":
				return "GENERAL.CONNECTION:Wired connection 1\n", nil
			case len(args) > 1 && args[0] == "con" && args[1] == "mod":
				modArgs = args
				return "", nil
			case len(args) > 1 && args[0] == "con" && args[1] == "up":
				upArgs = args
				return "", nil
			}
			t.Fatalf("unexpected nmcli invocation: %v", args)
			return "", nil
		},
	}

	err := c.ApplyStatic(StaticConfig{
		Interface: "eth0",
		Address:   "192.168.1.99",
		Prefix:    24,
		Gateway:   "192.168.1.1",
		DNS:       []string{"8.8.8.8"},
	})
	if err != nil {
		t.Fatalf("ApplyStatic: %v", err)
	}

	wantMod := "con mod Wired connection 1 ipv4.addresses 192.168.1.99/24 ipv4.gateway 192.168.1.1 ipv4.method manual ipv4.dns 8.8.8.8"
	if got := strings.Join(modArgs, " "); got != wantMod {
		t.Errorf("con mod args = %q, want %q", got, wantMod)
	}
	wantUp := "con up Wired connection 1"
	if got := strings.Join(upArgs, " "); got != wantUp {
		t.Errorf("con up args = %q, want %q", got, wantUp)
	}
}

func TestApplyStaticRejectsInvalidConfig(t *testing.T) {
	c := fakeNmcli(t, nil)

	cases := []StaticConfig{
		{Address: "1.2.3.4", Gateway: "1.2.3.1", Prefix: 24},                    // missing interface
		{Interface: "eth0", Gateway: "1.2.3.1", Prefix: 24},                     // missing address
		{Interface: "eth0", Address: "1.2.3.4", Prefix: 24},                     // missing gateway
		{Interface: "eth0", Address: "1.2.3.4", Gateway: "1.2.3.1"},             // missing prefix
		{Interface: "eth0", Address: "1.2.3.4", Gateway: "1.2.3.1", Prefix: 33}, // invalid prefix
	}
	for _, cfg := range cases {
		if err := c.ApplyStatic(cfg); err == nil {
			t.Errorf("ApplyStatic(%+v) succeeded, want a validation error", cfg)
		}
	}
}

func TestApplyDHCPSendsCorrectNmcliArgs(t *testing.T) {
	var modArgs, upArgs []string
	c := &nmcliController{
		lookPath: func(string) (string, error) { return "/usr/bin/nmcli", nil },
		run: func(args ...string) (string, error) {
			switch {
			case strings.Join(args, " ") == "-t -f GENERAL.CONNECTION device show eth0":
				return "GENERAL.CONNECTION:Wired connection 1\n", nil
			case len(args) > 1 && args[0] == "con" && args[1] == "mod":
				modArgs = args
				return "", nil
			case len(args) > 1 && args[0] == "con" && args[1] == "up":
				upArgs = args
				return "", nil
			}
			t.Fatalf("unexpected nmcli invocation: %v", args)
			return "", nil
		},
	}

	if err := c.ApplyDHCP("eth0"); err != nil {
		t.Fatalf("ApplyDHCP: %v", err)
	}

	wantMod := "con mod Wired connection 1 ipv4.method auto"
	if got := strings.Join(modArgs, " "); got != wantMod {
		t.Errorf("con mod args = %q, want %q", got, wantMod)
	}
	wantUp := "con up Wired connection 1"
	if got := strings.Join(upArgs, " "); got != wantUp {
		t.Errorf("con up args = %q, want %q", got, wantUp)
	}
}
