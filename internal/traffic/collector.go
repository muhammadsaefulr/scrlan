package traffic

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"
)

type Flusher interface{ FlushTraffic([]Bucket) error }

func RunCollector(ctx context.Context, addr string, aggregator *Aggregator, repository Flusher, flushEvery time.Duration) error {
	conn, err := net.ListenPacket("udp", addr)
	if err != nil {
		return fmt.Errorf("listen traffic collector: %w", err)
	}
	defer conn.Close()
	parser := NewParser()
	flush := func() {
		if buckets := aggregator.Drain(); len(buckets) > 0 {
			if err := repository.FlushTraffic(buckets); err != nil {
				log.Printf("traffic flush failed: %v", err)
			}
		}
	}
	ticker := time.NewTicker(flushEvery)
	defer ticker.Stop()
	for {
		_ = conn.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
		buffer := make([]byte, 65535)
		n, _, readErr := conn.ReadFrom(buffer)
		if readErr == nil {
			flows, parseErr := parser.Parse(buffer[:n], time.Now().UTC())
			if parseErr != nil {
				log.Printf("traffic packet ignored: %v", parseErr)
			} else {
				for _, flow := range flows {
					aggregator.Add(flow)
				}
			}
		}
		select {
		case <-ctx.Done():
			flush()
			return nil
		case <-ticker.C:
			flush()
		default:
		}
	}
}
