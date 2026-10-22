package traffic

import (
	"testing"
	"time"
)

func TestAggregatorTracksTrafficDirection(t *testing.T) {
	aggregator, err := NewAggregator("192.168.1.1", "192.168.1.254", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	when := time.Date(2026, 10, 22, 10, 1, 15, 0, time.UTC)
	aggregator.Add(Flow{SourceIP: "192.168.1.20", DestinationIP: "8.8.8.8", Bytes: 1000, Packets: 2, Timestamp: when})
	aggregator.Add(Flow{SourceIP: "8.8.8.8", DestinationIP: "192.168.1.20", Bytes: 2000, Packets: 3, Timestamp: when})
	buckets := aggregator.Drain()
	if len(buckets) != 1 {
		t.Fatalf("got %d buckets, want 1", len(buckets))
	}
	if buckets[0].BytesIn != 2000 || buckets[0].BytesOut != 1000 {
		t.Fatalf("unexpected counters: %+v", buckets[0])
	}
	if buckets[0].PacketsIn != 3 || buckets[0].PacketsOut != 2 {
		t.Fatalf("unexpected packets: %+v", buckets[0])
	}
}

func TestAggregatorIgnoresExternalFlow(t *testing.T) {
	aggregator, err := NewAggregator("192.168.1.1", "192.168.1.254", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	aggregator.Add(Flow{SourceIP: "8.8.8.8", DestinationIP: "1.1.1.1", Bytes: 1000, Timestamp: time.Now()})
	if buckets := aggregator.Drain(); len(buckets) != 0 {
		t.Fatalf("got external flow: %+v", buckets)
	}
}
