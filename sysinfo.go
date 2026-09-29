package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type sysInfo struct {
	host     string
	hasBat   bool
	bat      int
	charging bool
	ssid     string
}

func readSysInfo() sysInfo {
	host, _ := os.Hostname()
	si := sysInfo{host: host}

	if bats, _ := filepath.Glob("/sys/class/power_supply/BAT*"); len(bats) > 0 {
		if b, err := os.ReadFile(bats[0] + "/capacity"); err == nil {
			if n, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
				si.bat, si.hasBat = n, true
			}
		}
		if b, err := os.ReadFile(bats[0] + "/status"); err == nil {
			si.charging = strings.TrimSpace(string(b)) == "Charging"
		}
	}

	si.ssid = wifiSSID()
	return si
}

// 先试 iwgetid（wireless_tools），再试 nmcli；都没有就不显示。
func wifiSSID() string {
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	if out, err := exec.CommandContext(ctx, "iwgetid", "-r").Output(); err == nil {
		if s := strings.TrimSpace(string(out)); s != "" {
			return s
		}
	}
	out, err := exec.CommandContext(ctx, "nmcli", "-t", "-f", "active,ssid", "dev", "wifi").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if ssid, ok := strings.CutPrefix(line, "yes:"); ok {
			return ssid
		}
	}
	return ""
}
