package scanner

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/muhammadsaeful/scrlan/internal/scanner/contract"
)

const (
	defaultScanTimeout = 2 * time.Second
	packetReadTimeout  = 200 * time.Millisecond
)

type packetScanner struct {
	interfaceName  string
	timeout        time.Duration
	manufacturers  map[string]string
	dnsConcurrency int
	dnsTimeout     time.Duration
}

func newPacketScanner() *packetScanner {
	return &packetScanner{
		interfaceName:  os.Getenv("INET_INTERFACE"),
		timeout:        defaultScanTimeout,
		manufacturers:  loadManufacturers(os.Getenv("SCAN_OUI_FILE")),
		dnsConcurrency: envInt("SCAN_DNS_CONCURRENCY", 32),
		dnsTimeout:     time.Duration(envInt("SCAN_DNS_TIMEOUT_MS", 150)) * time.Millisecond,
	}
}

func (s *packetScanner) Scan(netFrom string, netTo string) ([]contract.Device, error) {
	from, to, err := ParseIPRange(netFrom, netTo)
	if err != nil {
		return nil, err
	}

	device, err := s.selectDevice(from, to)
	if err != nil {
		return nil, err
	}
	iface, localIP, err := findLocalInterface(device, from, to)
	if err != nil {
		return nil, err
	}

	handle, err := pcap.OpenLive(device.Name, 65535, true, packetReadTimeout)
	if err != nil {
		return nil, fmt.Errorf("open capture on %q: %w", device.Name, err)
	}
	defer handle.Close()
	if err := handle.SetBPFFilter("arp and arp[6:2] = 2"); err != nil {
		return nil, fmt.Errorf("set ARP capture filter: %w", err)
	}

	if len(iface.HardwareAddr) != 6 {
		return nil, fmt.Errorf("interface %q has no usable MAC address", device.Name)
	}
	for _, targetIP := range rangeIPs(from, to) {
		if err := sendARPRequest(handle, iface.HardwareAddr, localIP, targetIP); err != nil {
			return nil, err
		}
	}

	devices := readARPReplies(handle, from, to, s.timeout, s.manufacturers)
	return enrichDevices(devices, s.dnsConcurrency, s.dnsTimeout), nil
}

func (s *packetScanner) selectDevice(from net.IP, to net.IP) (pcap.Interface, error) {
	devices, err := pcap.FindAllDevs()
	if err != nil {
		return pcap.Interface{}, fmt.Errorf("list capture interfaces: %w", err)
	}
	if s.interfaceName != "" {
		for _, device := range devices {
			if device.Name == s.interfaceName || device.Description == s.interfaceName {
				return device, nil
			}
		}
		return pcap.Interface{}, fmt.Errorf("capture interface %q not found", s.interfaceName)
	}
	for _, device := range devices {
		for _, address := range device.Addresses {
			if address.IP != nil && ipInRange(address.IP, from, to) {
				return device, nil
			}
		}
	}
	return pcap.Interface{}, fmt.Errorf("no capture interface found for range %s-%s", from, to)
}

func findLocalInterface(device pcap.Interface, from net.IP, to net.IP) (*net.Interface, net.IP, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, nil, fmt.Errorf("list network interfaces: %w", err)
	}

	for index := range interfaces {
		iface := &interfaces[index]
		if iface.Name != device.Name {
			continue
		}
		localAddresses, _ := iface.Addrs()
		for _, localAddress := range localAddresses {
			localIP := addressIP(localAddress)
			if localIP == nil {
				continue
			}
			return iface, localIP, nil
		}
	}

	return nil, nil, fmt.Errorf("interface %q has no local IPv4 address for scan range %s-%s", device.Name, from, to)
}

func addressIP(address net.Addr) net.IP {
	switch value := address.(type) {
	case *net.IPNet:
		return value.IP.To4()
	case *net.IPAddr:
		return value.IP.To4()
	default:
		return nil
	}
}

func rangeIPs(start net.IP, end net.IP) []net.IP {
	hosts := make([]net.IP, 0)
	for current := append(net.IP(nil), start...); ; incrementIP(current) {
		hosts = append(hosts, append(net.IP(nil), current...))
		if current.Equal(end) {
			break
		}
	}
	return hosts
}

func ParseIPRange(netFrom string, netTo string) (net.IP, net.IP, error) {
	from := net.ParseIP(strings.TrimSpace(netFrom)).To4()
	to := net.ParseIP(strings.TrimSpace(netTo)).To4()
	if from == nil || to == nil {
		return nil, nil, fmt.Errorf("invalid IPv4 range %q-%q", netFrom, netTo)
	}
	if !ipInRange(from, from, to) {
		return nil, nil, fmt.Errorf("net_from must not be greater than net_to")
	}
	return from, to, nil
}

func RangeFromSubnet(subnet string) (string, string, error) {
	_, network, err := net.ParseCIDR(subnet)
	if err != nil || network.IP.To4() == nil {
		return "", "", fmt.Errorf("invalid IPv4 subnet %q", subnet)
	}
	from := network.IP.To4()
	to := append(net.IP(nil), from...)
	for index := range to {
		to[index] |= ^network.Mask[index]
	}
	return from.String(), to.String(), nil
}

func RangeFromInterface(interfaceName string) (string, string, error) {
	interfaceName = strings.TrimSpace(interfaceName)
	if interfaceName == "" {
		return "", "", fmt.Errorf("INET_INTERFACE is required")
	}
	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		return "", "", fmt.Errorf("find interface %q: %w", interfaceName, err)
	}
	addresses, err := iface.Addrs()
	if err != nil {
		return "", "", fmt.Errorf("list addresses for interface %q: %w", interfaceName, err)
	}
	for _, address := range addresses {
		network, ok := address.(*net.IPNet)
		if !ok || network.IP.To4() == nil {
			continue
		}
		ipv4 := network.IP.To4()
		mask := network.Mask
		from := ipv4.Mask(mask).To4()
		to := make(net.IP, net.IPv4len)
		for index := range to {
			to[index] = from[index] | ^mask[index]
		}
		return from.String(), to.String(), nil
	}
	return "", "", fmt.Errorf("interface %q has no IPv4 address", interfaceName)
}

func ipInRange(ip net.IP, from net.IP, to net.IP) bool {
	ip = ip.To4()
	from = from.To4()
	to = to.To4()
	return ip != nil && from != nil && to != nil && bytesCompare(ip, from) >= 0 && bytesCompare(ip, to) <= 0
}

func bytesCompare(left net.IP, right net.IP) int {
	for index := 0; index < 4; index++ {
		if left[index] < right[index] {
			return -1
		}
		if left[index] > right[index] {
			return 1
		}
	}
	return 0
}

func incrementIP(ip net.IP) {
	for index := len(ip) - 1; index >= 0; index-- {
		ip[index]++
		if ip[index] != 0 {
			return
		}
	}
}

func sendARPRequest(handle *pcap.Handle, sourceMAC net.HardwareAddr, sourceIP, targetIP net.IP) error {
	packet := gopacket.NewSerializeBuffer()
	options := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	ethernet := &layers.Ethernet{
		SrcMAC:       sourceMAC,
		DstMAC:       net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		EthernetType: layers.EthernetTypeARP,
	}
	arppacket := &layers.ARP{
		AddrType:          layers.LinkTypeEthernet,
		Protocol:          layers.EthernetTypeIPv4,
		HwAddressSize:     6,
		ProtAddressSize:   4,
		Operation:         layers.ARPRequest,
		SourceHwAddress:   []byte(sourceMAC),
		SourceProtAddress: []byte(sourceIP.To4()),
		DstHwAddress:      []byte{0, 0, 0, 0, 0, 0},
		DstProtAddress:    []byte(targetIP.To4()),
	}
	if err := gopacket.SerializeLayers(packet, options, ethernet, arppacket); err != nil {
		return fmt.Errorf("serialize ARP request: %w", err)
	}
	if err := handle.WritePacketData(packet.Bytes()); err != nil {
		return fmt.Errorf("send ARP request to %s: %w", targetIP, err)
	}
	return nil
}

func readARPReplies(handle *pcap.Handle, from net.IP, to net.IP, timeout time.Duration, manufacturers map[string]string) []contract.Device {
	deadline := time.Now().Add(timeout)
	seen := make(map[string]struct{})
	devices := make([]contract.Device, 0)
	for time.Now().Before(deadline) {
		data, _, err := handle.ReadPacketData()
		if err != nil {
			if strings.Contains(err.Error(), "timeout") {
				continue
			}
			continue
		}
		packet := gopacket.NewPacket(data, layers.LayerTypeEthernet, gopacket.NoCopy)
		layer := packet.Layer(layers.LayerTypeARP)
		if layer == nil {
			continue
		}
		arpLayer := layer.(*layers.ARP)
		ip := net.IP(arpLayer.SourceProtAddress).To4()
		if arpLayer.Operation != layers.ARPReply || ip == nil || !ipInRange(ip, from, to) {
			continue
		}
		key := ip.String()
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		mac := net.HardwareAddr(arpLayer.SourceHwAddress).String()
		devices = append(devices, contract.Device{IP: key, MAC: mac, Manufacturer: manufacturers[oui(mac)]})
	}
	return devices
}

func enrichDevices(devices []contract.Device, concurrency int, timeout time.Duration) []contract.Device {
	if concurrency < 1 {
		concurrency = 1
	}
	if timeout <= 0 {
		timeout = 150 * time.Millisecond
	}
	jobs := make(chan int)
	var wait sync.WaitGroup
	for worker := 0; worker < concurrency; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for index := range jobs {
				ctx, cancel := context.WithTimeout(context.Background(), timeout)
				hostnames, err := net.DefaultResolver.LookupAddr(ctx, devices[index].IP)
				cancel()
				if err == nil && len(hostnames) > 0 {
					devices[index].Hostname = strings.TrimSuffix(hostnames[0], ".")
				}
			}
		}()
	}
	for index := range devices {
		jobs <- index
	}
	close(jobs)
	wait.Wait()
	return devices
}

func envInt(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil || value < 1 {
		return fallback
	}
	return value
}

func oui(mac string) string {
	value := strings.ToUpper(strings.NewReplacer(":", "", "-", "", ".", "", " ", "").Replace(mac))
	if len(value) < 6 {
		return ""
	}
	for _, character := range value[:6] {
		if !strings.ContainsRune("0123456789ABCDEF", character) {
			return ""
		}
	}
	return value[0:2] + ":" + value[2:4] + ":" + value[4:6]
}

func loadManufacturers(file string) map[string]string {
	manufacturers := map[string]string{
		"00:50:56": "VMware",
		"08:00:27": "Oracle VirtualBox",
		"00:15:5D": "Microsoft Hyper-V",
		"DC:A6:32": "Raspberry Pi",
		"B8:27:EB": "Raspberry Pi",
		"3C:5A:B4": "Google",
		"F4:F5:D8": "TP-Link",
	}
	if file != "" {
		if !filepath.IsAbs(file) {
			file, _ = filepath.Abs(file)
		}
		if handle, err := os.Open(file); err == nil {
			defer handle.Close()
			parseOUI(handle, manufacturers)
			return manufacturers
		}
	}

	request, err := http.NewRequest(http.MethodGet, "https://standards-oui.ieee.org/oui/oui.txt", nil)
	if err != nil {
		return manufacturers
	}
	request.Header.Set("User-Agent", "scrlan-ipscan/1.0")
	client := http.Client{Timeout: 10 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return manufacturers
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return manufacturers
	}
	parseOUI(io.LimitReader(response.Body, 16*1024*1024), manufacturers)
	return manufacturers
}

var ieeeOUI = regexp.MustCompile(`^([0-9A-Fa-f]{2}-[0-9A-Fa-f]{2}-[0-9A-Fa-f]{2})\s+\(hex\)\s+(.+)$`)

func parseOUI(reader io.Reader, manufacturers map[string]string) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		parts := strings.SplitN(line, "|", 2)
		if len(parts) == 2 {
			prefix := oui(strings.ReplaceAll(strings.TrimSpace(parts[0]), "-", ":"))
			if prefix != "" {
				manufacturers[prefix] = strings.TrimSpace(parts[1])
			}
			continue
		}
		match := ieeeOUI.FindStringSubmatch(line)
		if len(match) == 3 {
			prefix := oui(strings.ReplaceAll(match[1], "-", ":"))
			manufacturers[prefix] = strings.TrimSpace(match[2])
		}
	}
}
