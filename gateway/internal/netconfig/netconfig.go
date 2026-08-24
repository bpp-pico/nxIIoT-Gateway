// Package netconfig configures the gateway host's own network interface
// (static IP/gateway/DNS) via NetworkManager's nmcli CLI — the default
// network stack on Raspberry Pi OS (Bookworm+) and most modern Debian/
// Ubuntu systems, the deployment targets in §27.
//
// This only makes sense when the gateway binary runs directly on the
// host (the systemd deployment path in §19), not inside this project's
// Docker Compose dev setup — a container's network namespace is not the
// host's real NIC, so nmcli inside a container (even if installed) would
// at best reconfigure the container's own virtual interface, not the
// physical LAN-facing adapter an operator actually cares about. On any
// host without nmcli (this dev environment's Windows host, the Docker
// dev containers, macOS, ...) every method returns ErrUnsupported —
// there is no fallback network stack implemented, by design: silently
// editing the wrong thing (e.g. dhcpcd.conf on a NetworkManager system,
// or vice versa) is worse than clearly doing nothing.
package netconfig

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

var ErrUnsupported = errors.New("netconfig: host network configuration is not available (nmcli not found — only Linux hosts with NetworkManager are supported)")

// Info is the current network configuration of one interface.
type Info struct {
	Interface string
	Method    string // "auto" (DHCP) or "manual" (static), per nmcli's ipv4.method
	Address   string
	Prefix    int // CIDR prefix length, e.g. 24 for a /24
	Gateway   string
	DNS       []string
}

// StaticConfig is a static IP assignment to apply.
type StaticConfig struct {
	Interface string
	Address   string
	Prefix    int
	Gateway   string
	DNS       []string
}

func (cfg StaticConfig) validate() error {
	if cfg.Interface == "" {
		return fmt.Errorf("interface is required")
	}
	if cfg.Address == "" {
		return fmt.Errorf("address is required")
	}
	if cfg.Gateway == "" {
		return fmt.Errorf("gateway is required")
	}
	if cfg.Prefix <= 0 || cfg.Prefix > 32 {
		return fmt.Errorf("prefix must be between 1 and 32")
	}
	return nil
}

// Controller applies host network configuration. New returns the only
// implementation (nmcli-backed); every method degrades to ErrUnsupported
// at runtime when nmcli isn't on PATH, rather than failing to compile on
// platforms that will never have it — see the package doc comment.
type Controller interface {
	Current() (Info, error)
	ApplyStatic(cfg StaticConfig) error
	ApplyDHCP(iface string) error
}

type nmcliController struct {
	run      func(args ...string) (string, error)
	lookPath func(string) (string, error)
}

func New() Controller {
	return &nmcliController{
		run: func(args ...string) (string, error) {
			out, err := exec.Command("nmcli", args...).CombinedOutput()
			return string(out), err
		},
		lookPath: exec.LookPath,
	}
}

func (c *nmcliController) available() bool {
	_, err := c.lookPath("nmcli")
	return err == nil
}

// primaryInterface picks the first connected ethernet/wifi device from
// `nmcli device status` — the interface an operator almost certainly
// means by "the gateway's IP" on a single-NIC device like a Raspberry Pi.
// A host with more than one active interface needs the caller to name it
// explicitly in ApplyStatic/ApplyDHCP; Current() is a convenience default.
func (c *nmcliController) primaryInterface() (string, error) {
	out, err := c.run("-t", "-f", "DEVICE,TYPE,STATE", "device", "status")
	if err != nil {
		return "", fmt.Errorf("nmcli device status: %w", err)
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Split(line, ":")
		if len(fields) < 3 {
			continue
		}
		device, devType, state := fields[0], fields[1], fields[2]
		if state == "connected" && (devType == "ethernet" || devType == "wifi") {
			return device, nil
		}
	}
	return "", fmt.Errorf("no connected ethernet/wifi interface found")
}

// connectionForInterface resolves the active NetworkManager connection
// profile name for iface — nmcli edits apply to a connection profile
// (`nmcli con mod`), not a device directly.
func (c *nmcliController) connectionForInterface(iface string) (string, error) {
	out, err := c.run("-t", "-f", "GENERAL.CONNECTION", "device", "show", iface)
	if err != nil {
		return "", fmt.Errorf("nmcli device show %s: %w", iface, err)
	}
	for _, line := range strings.Split(out, "\n") {
		key, val, ok := strings.Cut(line, ":")
		if !ok || key != "GENERAL.CONNECTION" {
			continue
		}
		val = strings.TrimSpace(val)
		if val == "" || val == "--" {
			return "", fmt.Errorf("interface %s has no active connection profile", iface)
		}
		return val, nil
	}
	return "", fmt.Errorf("could not determine connection profile for interface %s", iface)
}

func (c *nmcliController) Current() (Info, error) {
	if !c.available() {
		return Info{}, ErrUnsupported
	}

	iface, err := c.primaryInterface()
	if err != nil {
		return Info{}, err
	}

	out, err := c.run("-t", "-f", "GENERAL.CONNECTION,IP4.ADDRESS,IP4.GATEWAY,IP4.DNS", "device", "show", iface)
	if err != nil {
		return Info{}, fmt.Errorf("nmcli device show %s: %w", iface, err)
	}

	info := Info{Interface: iface}
	var conn string
	for _, line := range strings.Split(out, "\n") {
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch {
		case key == "GENERAL.CONNECTION":
			conn = val
		case key == "IP4.ADDRESS[1]":
			addr, prefixStr, _ := strings.Cut(val, "/")
			info.Address = addr
			info.Prefix, _ = strconv.Atoi(prefixStr)
		case key == "IP4.GATEWAY":
			info.Gateway = val
		case strings.HasPrefix(key, "IP4.DNS["):
			if val != "" {
				info.DNS = append(info.DNS, val)
			}
		}
	}

	if conn != "" && conn != "--" {
		if methodOut, err := c.run("-t", "-f", "ipv4.method", "connection", "show", conn); err == nil {
			for _, line := range strings.Split(methodOut, "\n") {
				if key, val, ok := strings.Cut(line, ":"); ok && key == "ipv4.method" {
					info.Method = strings.TrimSpace(val)
				}
			}
		}
	}

	return info, nil
}

func (c *nmcliController) ApplyStatic(cfg StaticConfig) error {
	if !c.available() {
		return ErrUnsupported
	}
	if err := cfg.validate(); err != nil {
		return err
	}

	conn, err := c.connectionForInterface(cfg.Interface)
	if err != nil {
		return err
	}

	args := []string{
		"con", "mod", conn,
		"ipv4.addresses", fmt.Sprintf("%s/%d", cfg.Address, cfg.Prefix),
		"ipv4.gateway", cfg.Gateway,
		"ipv4.method", "manual",
	}
	if len(cfg.DNS) > 0 {
		args = append(args, "ipv4.dns", strings.Join(cfg.DNS, ","))
	}
	if _, err := c.run(args...); err != nil {
		return fmt.Errorf("nmcli con mod: %w", err)
	}
	if _, err := c.run("con", "up", conn); err != nil {
		return fmt.Errorf("nmcli con up: %w", err)
	}
	return nil
}

func (c *nmcliController) ApplyDHCP(iface string) error {
	if !c.available() {
		return ErrUnsupported
	}
	if iface == "" {
		return fmt.Errorf("interface is required")
	}

	conn, err := c.connectionForInterface(iface)
	if err != nil {
		return err
	}

	if _, err := c.run("con", "mod", conn, "ipv4.method", "auto"); err != nil {
		return fmt.Errorf("nmcli con mod (dhcp): %w", err)
	}
	if _, err := c.run("con", "up", conn); err != nil {
		return fmt.Errorf("nmcli con up: %w", err)
	}
	return nil
}
