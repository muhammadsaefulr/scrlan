package scanner

import (
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"time"
)

const (
	netBIOSPort       = 137
	netBIOSNameLength = 15
	netBIOSAnswerSize = 1 + netBIOSNameLength + 1 + 2
)

func lookupNetBIOS(ip string, timeout time.Duration) (string, error) {
	address := net.JoinHostPort(ip, fmt.Sprint(netBIOSPort))
	connection, err := net.DialTimeout("udp", address, timeout)
	if err != nil {
		return "", err
	}
	defer connection.Close()

	if err := connection.SetDeadline(time.Now().Add(timeout)); err != nil {
		return "", err
	}
	if _, err := connection.Write(netBIOSQuery()); err != nil {
		return "", err
	}

	response := make([]byte, 1500)
	length, err := connection.Read(response)
	if err != nil {
		return "", err
	}
	return parseNetBIOSResponse(response[:length])
}

func netBIOSQuery() []byte {
	query := make([]byte, 50)
	binary.BigEndian.PutUint16(query[0:2], uint16(time.Now().UnixNano()))
	binary.BigEndian.PutUint16(query[4:6], 1)
	query[12] = 0x20
	copy(query[13:45], encodeNetBIOSName("*"))
	query[45] = 0
	binary.BigEndian.PutUint16(query[46:48], 0x0021)
	binary.BigEndian.PutUint16(query[48:50], 0x0001)
	return query
}

func encodeNetBIOSName(name string) []byte {
	nameBytes := make([]byte, 16)
	copy(nameBytes, strings.ToUpper(name))
	for index := len(name); index < 15; index++ {
		nameBytes[index] = ' '
	}
	encoded := make([]byte, 32)
	for index, value := range nameBytes {
		encoded[index*2] = 'A' + (value >> 4)
		encoded[index*2+1] = 'A' + (value & 0x0f)
	}
	return encoded
}

func parseNetBIOSResponse(packet []byte) (string, error) {
	if len(packet) < 12 {
		return "", fmt.Errorf("NetBIOS response header is incomplete")
	}
	if binary.BigEndian.Uint16(packet[2:4])&0x8000 == 0 {
		return "", fmt.Errorf("NetBIOS response flag is missing")
	}
	if binary.BigEndian.Uint16(packet[6:8]) == 0 {
		return "", fmt.Errorf("NetBIOS response has no answers")
	}

	offset, err := skipNetBIOSName(packet, 12)
	if err != nil || offset+4 > len(packet) {
		return "", fmt.Errorf("NetBIOS question is incomplete")
	}
	offset += 4
	answers := int(binary.BigEndian.Uint16(packet[6:8]))
	for answer := 0; answer < answers; answer++ {
		offset, err = skipNetBIOSName(packet, offset)
		if err != nil || offset+10 > len(packet) {
			return "", fmt.Errorf("NetBIOS answer is incomplete")
		}
		recordType := binary.BigEndian.Uint16(packet[offset : offset+2])
		dataLength := int(binary.BigEndian.Uint16(packet[offset+8 : offset+10]))
		offset += 10
		if offset+dataLength > len(packet) {
			return "", fmt.Errorf("NetBIOS answer data is incomplete")
		}
		if recordType == 0x0020 && dataLength >= netBIOSAnswerSize {
			nameStart := offset + 1
			nameEnd := nameStart + netBIOSNameLength
			name := strings.TrimSpace(string(packet[nameStart:nameEnd]))
			if name != "" {
				return name, nil
			}
		}
		offset += dataLength
	}
	return "", fmt.Errorf("NetBIOS computer name not found")
}

func skipNetBIOSName(packet []byte, offset int) (int, error) {
	if offset >= len(packet) {
		return 0, fmt.Errorf("NetBIOS name is missing")
	}
	if packet[offset]&0xc0 == 0xc0 {
		if offset+2 > len(packet) {
			return 0, fmt.Errorf("NetBIOS name pointer is incomplete")
		}
		return offset + 2, nil
	}
	for {
		if offset >= len(packet) {
			return 0, fmt.Errorf("NetBIOS name is incomplete")
		}
		length := int(packet[offset])
		offset++
		if length == 0 {
			return offset, nil
		}
		if offset+length > len(packet) {
			return 0, fmt.Errorf("NetBIOS name label is incomplete")
		}
		offset += length
	}
}
