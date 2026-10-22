package traffic

import "time"

type Flow struct {
	SourceIP      string
	DestinationIP string
	Bytes         uint64
	Packets       uint64
	Timestamp     time.Time
}

type Bucket struct {
	IP             string
	BucketStart    time.Time
	BucketEnd      time.Time
	BytesIn        uint64
	BytesOut       uint64
	PacketsIn      uint64
	PacketsOut     uint64
	LastActivityAt time.Time
}
