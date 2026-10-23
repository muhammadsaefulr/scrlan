package traffic

import (
	"encoding/binary"
	"testing"
	"time"
)

func TestParserRejectsShortPacket(t *testing.T) {
	if _, err := NewParser().Parse([]byte{9}, time.Now()); err == nil {
		t.Fatal("short packet accepted")
	}
}

func TestParserNetFlowV9TemplateAndRecord(t *testing.T) {
	parser := NewParser()
	packet := make([]byte, 20)
	binary.BigEndian.PutUint16(packet[0:], 9)
	binary.BigEndian.PutUint16(packet[2:], 2)
	binary.BigEndian.PutUint32(packet[8:], uint32(time.Now().Unix()))
	templateSet := make([]byte, 4+4*4)
	binary.BigEndian.PutUint16(templateSet[0:], 256)
	binary.BigEndian.PutUint16(templateSet[2:], 4)
	for i, value := range [][2]uint16{{8, 4}, {12, 4}, {1, 4}, {2, 4}} {
		binary.BigEndian.PutUint16(templateSet[4+i*4:], value[0])
		binary.BigEndian.PutUint16(templateSet[6+i*4:], value[1])
	}
	packet = append(packet, set(templateSet, 0)...)
	record := make([]byte, 16)
	copy(record[0:4], []byte{192, 168, 1, 20})
	copy(record[4:8], []byte{8, 8, 8, 8})
	binary.BigEndian.PutUint32(record[8:12], 1000)
	binary.BigEndian.PutUint32(record[12:16], 2)
	packet = append(packet, set(record, 256)...)
	flows, err := parser.Parse(packet, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(flows) != 1 || flows[0].SourceIP != "192.168.1.20" || flows[0].Bytes != 1000 {
		t.Fatalf("unexpected flows: %+v", flows)
	}
	if flows[0].Timestamp.Year() < 2020 {
		t.Fatalf("flow timestamp year is %d, expected current year", flows[0].Timestamp.Year())
	}
}

func set(payload []byte, id uint16) []byte {
	result := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint16(result, id)
	binary.BigEndian.PutUint16(result[2:], uint16(len(result)))
	copy(result[4:], payload)
	return result
}
