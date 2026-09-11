package parser

import (
	"bufio"
	"net"
	"regexp"
	"strings"

	"github.com/muhammadsaeful/scrlan/internal/scanner/contract"
)

var (
	windowsEntry = regexp.MustCompile(`^\s*(\d{1,3}(?:\.\d{1,3}){3})\s+([0-9a-fA-F-]{17})\s+\S+\s*$`)
	linuxEntry   = regexp.MustCompile(`\((\d{1,3}(?:\.\d{1,3}){3})\)\s+at\s+([0-9a-fA-F:]{17})`)
)

func ParseARPOutput(output string, subnet string) ([]contract.Device, error) {
	_, network, err := net.ParseCIDR(subnet)
	if err != nil {
		return nil, err
	}

	devices := make([]contract.Device, 0)
	seen := make(map[string]struct{})
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		match := windowsEntry.FindStringSubmatch(line)
		if match == nil {
			match = linuxEntry.FindStringSubmatch(line)
		}
		if match == nil {
			continue
		}

		ip := net.ParseIP(match[1])
		if ip == nil || !network.Contains(ip) {
			continue
		}
		mac := strings.ToLower(strings.ReplaceAll(match[2], "-", ":"))
		if _, exists := seen[ip.String()]; exists {
			continue
		}
		seen[ip.String()] = struct{}{}
		devices = append(devices, contract.Device{IP: ip.String(), MAC: mac})
	}

	return devices, scanner.Err()
}
