package scanner

import (
	"encoding/binary"
	"testing"
)

func TestEncodeNetBIOSName(t *testing.T) {
	encoded := encodeNetBIOSName("*")
	if len(encoded) != 32 {
		t.Fatalf("encoded name length = %d, want 32", len(encoded))
	}
	if string(encoded[:2]) != "CK" {
		t.Fatalf("encoded wildcard prefix = %q, want CK", encoded[:2])
	}
}

func TestParseNetBIOSResponse(t *testing.T) {
	packet := make([]byte, 12+1+4+2+10+netBIOSAnswerSize)
	binary.BigEndian.PutUint16(packet[2:4], 0x8500)
	binary.BigEndian.PutUint16(packet[6:8], 1)
	packet[12] = 0
	binary.BigEndian.PutUint16(packet[13:15], 0x0020)
	binary.BigEndian.PutUint16(packet[15:17], 0x0001)
	packet[17] = 0xc0
	packet[18] = 0x0c
	binary.BigEndian.PutUint16(packet[19:21], 0x0020)
	binary.BigEndian.PutUint16(packet[21:23], 0x0001)
	binary.BigEndian.PutUint16(packet[27:29], netBIOSAnswerSize)
	packet[29] = 1
	copy(packet[30:45], "OFFICE-PC      ")

	hostname, err := parseNetBIOSResponse(packet)
	if err != nil {
		t.Fatalf("parseNetBIOSResponse() error = %v", err)
	}
	if hostname != "OFFICE-PC" {
		t.Fatalf("hostname = %q, want OFFICE-PC", hostname)
	}
}

func TestParseNetBIOSResponseRejectsIncompletePacket(t *testing.T) {
	_, err := parseNetBIOSResponse([]byte{0x00, 0x01})
	if err == nil {
		t.Fatal("parseNetBIOSResponse() error = nil, want error")
	}
}
