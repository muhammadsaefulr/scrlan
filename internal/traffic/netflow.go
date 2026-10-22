package traffic

import (
	"encoding/binary"
	"fmt"
	"net"
	"time"
)

type field struct{ typ, length uint16 }
type template struct{ fields []field }

type Parser struct{ templates map[uint16]template }

func NewParser() *Parser { return &Parser{templates: make(map[uint16]template)} }

func (p *Parser) Parse(packet []byte, received time.Time) ([]Flow, error) {
	if len(packet) < 2 {
		return nil, fmt.Errorf("short flow packet")
	}
	version := binary.BigEndian.Uint16(packet)
	switch version {
	case 9:
		return p.parseV9(packet, received)
	default:
		return nil, fmt.Errorf("unsupported flow version %d", version)
	}
}

func (p *Parser) parseV9(packet []byte, received time.Time) ([]Flow, error) {
	if len(packet) < 20 {
		return nil, fmt.Errorf("short netflow v9 packet")
	}
	count := int(binary.BigEndian.Uint16(packet[2:4]))
	unix := int64(binary.BigEndian.Uint32(packet[8:12]))
	base := time.Unix(unix, 0).UTC()
	offset := 20
	flows := make([]Flow, 0)
	for i := 0; i < count && offset+4 <= len(packet); i++ {
		setID := binary.BigEndian.Uint16(packet[offset:])
		length := int(binary.BigEndian.Uint16(packet[offset+2:]))
		if length < 4 || offset+length > len(packet) {
			return nil, fmt.Errorf("invalid netflow set length")
		}
		payload := packet[offset+4 : offset+length]
		if setID == 0 {
			p.parseTemplateSet(payload)
		} else if setID >= 256 {
			if t, ok := p.templates[setID]; ok {
				flows = append(flows, parseRecords(payload, t, base, received)...)
			}
		}
		offset += length
	}
	return flows, nil
}

func (p *Parser) parseTemplateSet(payload []byte) {
	for len(payload) >= 4 {
		id, count := binary.BigEndian.Uint16(payload), int(binary.BigEndian.Uint16(payload[2:]))
		payload = payload[4:]
		if count <= 0 || len(payload) < count*4 {
			return
		}
		fields := make([]field, count)
		for i := range fields {
			fields[i] = field{binary.BigEndian.Uint16(payload), binary.BigEndian.Uint16(payload[2:])}
			payload = payload[4:]
		}
		p.templates[id] = template{fields: fields}
	}
}

func parseRecords(payload []byte, t template, base, received time.Time) []Flow {
	recordSize := 0
	for _, f := range t.fields {
		recordSize += int(f.length)
	}
	if recordSize == 0 {
		return nil
	}
	flows := make([]Flow, 0, len(payload)/recordSize)
	for len(payload) >= recordSize {
		record := payload[:recordSize]
		payload = payload[recordSize:]
		var source, destination net.IP
		var bytes, packets uint64
		ts := base
		pos := 0
		for _, f := range t.fields {
			value := record[pos : pos+int(f.length)]
			pos += int(f.length)
			switch f.typ {
			case 8:
				if len(value) == 4 {
					source = net.IP(value).To4()
				}
			case 12:
				if len(value) == 4 {
					destination = net.IP(value).To4()
				}
			case 1:
				bytes = number(value)
			case 2:
				packets = number(value)
			case 21, 22:
				if len(value) == 4 {
					ts = time.UnixMilli(int64(binary.BigEndian.Uint32(value))).UTC()
				}
			}
		}
		if source != nil && destination != nil && bytes > 0 {
			flows = append(flows, Flow{SourceIP: source.String(), DestinationIP: destination.String(), Bytes: bytes, Packets: packets, Timestamp: ts})
		}
	}
	if len(flows) == 0 && received.IsZero() {
		return nil
	}
	return flows
}

func number(value []byte) uint64 {
	var result uint64
	for _, b := range value {
		result = result<<8 | uint64(b)
	}
	return result
}
