package traffic

import (
	"fmt"
	"net"
	"sync"
	"time"
)

type Aggregator struct {
	mu      sync.Mutex
	from    net.IP
	to      net.IP
	buckets map[string]*Bucket
	bucket  time.Duration
}

func NewAggregator(netFrom, netTo string, bucket time.Duration) (*Aggregator, error) {
	from := net.ParseIP(netFrom).To4()
	to := net.ParseIP(netTo).To4()
	if from == nil || to == nil || bucket <= 0 {
		return nil, fmt.Errorf("invalid traffic aggregator configuration")
	}
	return &Aggregator{from: from, to: to, bucket: bucket, buckets: make(map[string]*Bucket)}, nil
}

func (a *Aggregator) Add(flow Flow) {
	a.mu.Lock()
	defer a.mu.Unlock()
	ts := flow.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	start := ts.UTC().Truncate(a.bucket)
	end := start.Add(a.bucket)
	add := func(ipStr string, in bool) {
		ip := net.ParseIP(ipStr).To4()
		if ip == nil || ip[3] == 255 || ip[3] == 0 {
			return
		}
		if !ipInRange(ip, a.from, a.to) && !ip.IsPrivate() {
			return
		}
		key := ipStr + "\x00" + start.Format(time.RFC3339Nano)
		b := a.buckets[key]
		if b == nil {
			b = &Bucket{IP: ipStr, BucketStart: start, BucketEnd: end}
			a.buckets[key] = b
		}
		if in {
			b.BytesIn += flow.Bytes
			b.PacketsIn += flow.Packets
		} else {
			b.BytesOut += flow.Bytes
			b.PacketsOut += flow.Packets
		}
		if b.LastActivityAt.Before(ts) {
			b.LastActivityAt = ts
		}
	}
	add(flow.SourceIP, false)
	add(flow.DestinationIP, true)
}

func (a *Aggregator) Drain() []Bucket {
	a.mu.Lock()
	defer a.mu.Unlock()
	result := make([]Bucket, 0, len(a.buckets))
	for _, bucket := range a.buckets {
		result = append(result, *bucket)
	}
	a.buckets = make(map[string]*Bucket)
	return result
}

func ipInRange(ip net.IP, from, to net.IP) bool {
	if ip == nil || from == nil || to == nil {
		return false
	}
	return bytesCompare(ip, from) >= 0 && bytesCompare(ip, to) <= 0
}

func bytesCompare(a, b net.IP) int {
	for i := range a {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}
