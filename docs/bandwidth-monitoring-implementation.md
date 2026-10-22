# Bandwidth Monitoring Implementation

## Goal

Add bandwidth monitoring endpoint without changing existing discovery logic, existing endpoints, or existing response contracts.

New endpoint:

```http
GET /api/metrics/bandwidth
```

Existing endpoints must remain unchanged:

```text
GET /healthz
GET /api/devices
GET /api/scan
GET /api/metrics/devices
```

## Final Decisions

- Traffic source: MikroTik Traffic Flow using NetFlow v9 or IPFIX.
- Scrlan receives flow data through UDP, default port `2055`.
- Scrlan does not need MikroTik login to receive flow data.
- MikroTik requires one-time administrative configuration.
- Default endpoint window: `1h`.
- Devices without traffic remain visible with zero counters.
- Default feature state: disabled with `TRAFFIC_ENABLED=false`.
- Existing discovery worker remains responsible for ARP discovery and online/offline status.
- Traffic collector runs separately from discovery worker.
- Traffic collector failure must not stop existing discovery or HTTP API.
- Database changes must be additive only.
- Do not store one database row per packet.

## Response Contract

Successful response:

```json
{
  "success": true,
  "data": [
    {
      "ip": "192.168.1.20",
      "hostname": "OFFICE-PC",
      "status": "online",
      "bytes_in": 891289600,
      "bytes_out": 125829120,
      "total_bytes": 1017118720,
      "download_bps": 5242880,
      "upload_bps": 1048576,
      "last_activity_at": "2026-10-18T10:15:20Z",
      "active": true
    }
  ]
}
```

Field definitions:

- `ip`: identity from `devices.ip`.
- `hostname`: identity from `devices.hostname`.
- `status`: existing discovery status, not traffic status.
- `bytes_in`: bytes received by device.
- `bytes_out`: bytes sent by device.
- `total_bytes`: `bytes_in + bytes_out`.
- `download_bps`: current download rate in bits per second.
- `upload_bps`: current upload rate in bits per second.
- `last_activity_at`: latest flow timestamp for device.
- `active`: true when activity is within configured activity window.

Empty response:

```json
{
  "success": true,
  "data": []
}
```

Database error response:

```json
{
  "success": false,
  "message": "unable to retrieve bandwidth metrics"
}
```

## Optional Query Parameters

Support these parameters without changing default behavior:

```http
GET /api/metrics/bandwidth?window=1h
GET /api/metrics/bandwidth?window=24h
GET /api/metrics/bandwidth?sort=total_bytes
GET /api/metrics/bandwidth?limit=20
GET /api/metrics/bandwidth?by_subnet=true
```

Defaults:

```text
window=1h
sort=total_bytes
order=desc
limit=50
```

Validation rules:

- Invalid `window` returns HTTP `400`.
- Invalid `limit` returns HTTP `400`.
- Limit must have a reasonable maximum.
- `by_subnet=true` returns one summary row per available `/24` subnet derived from `devices.ip`.
- Subnet summary keeps `bytes_in`, `bytes_out`, `total_bytes`, rate, activity, and latest activity fields.
- `sort` accepts `total_bytes`, `bytes_in`, or `bytes_out`; `order` accepts `asc` or `desc`.
- Without `by_subnet=true`, response remains one row per device.
- Existing endpoints keep current validation and behavior.

## Data Flow

```text
MikroTik Traffic Flow
  -> UDP NetFlow/IPFIX packet
  -> Scrlan traffic collector
  -> flow parser
  -> in-memory aggregator
  -> periodic batch flush
  -> PostgreSQL traffic buckets
  -> join with devices.ip
  -> GET /api/metrics/bandwidth
```

Identity comes from `devices`. Traffic comes from flow records. The endpoint must not read traffic directly from collector memory.

## New Package

Create isolated package:

```text
internal/traffic/
├── collector.go
├── netflow.go
├── aggregator.go
├── repository.go
└── contract.go
```

Responsibilities:

- `collector.go`: listen on UDP and receive packets.
- `netflow.go`: parse NetFlow v9 or IPFIX records.
- `aggregator.go`: normalize direction and aggregate counters.
- `repository.go`: persist and query traffic data.
- `contract.go`: internal traffic types.

Do not place traffic logic in `internal/scanner`.

## Traffic Model

Add a new model, for example:

```go
type DeviceTraffic struct {
    ID             uint
    DeviceID       uint
    IP             string
    BucketStart    time.Time
    BucketEnd      time.Time
    BytesIn        uint64
    BytesOut       uint64
    PacketsIn      uint64
    PacketsOut     uint64
    LastActivityAt time.Time
    CreatedAt      time.Time
    UpdatedAt      time.Time
}
```

Recommended indexes:

```text
(device_id, bucket_start)
(ip, bucket_start)
(last_activity_at)
```

Recommended unique key:

```text
device_id + bucket_start
```

Storage rules:

- Migration is additive.
- Do not modify or rename existing device tables.
- Do not modify `device_history`.
- Do not insert one row per packet.
- Use aggregation buckets.
- Initial bucket size: `10` seconds or `1` minute.
- Recommended detail retention: `24` hours.

## Traffic Direction

Identify local IPs from configured scan range or interface subnet.

```text
source IP local      -> bytes_out for source device
destination IP local -> bytes_in for destination device
```

Example upload:

```text
source: 192.168.1.20
destination: 8.8.8.8
bytes: 1000

192.168.1.20.bytes_out += 1000
```

Example download:

```text
source: 8.8.8.8
destination: 192.168.1.20
bytes: 2000

192.168.1.20.bytes_in += 2000
```

For LAN-to-LAN traffic, update source `bytes_out` and destination `bytes_in`.

Ignore flows where neither endpoint is in the monitored local range.

## Rate Calculation

```text
download_bps = bytes_in_delta * 8 / elapsed_seconds
upload_bps   = bytes_out_delta * 8 / elapsed_seconds
```

Keep database counters as `uint64`. Handle safely:

- counter reset
- duplicate flow
- timestamp moving backwards
- zero elapsed time
- malformed packet
- missing bytes field

Return zero rate when there is not enough data to calculate a rate.

## Activity

Configuration:

```env
TRAFFIC_ACTIVITY_WINDOW_SECONDS=60
NETBIOS_ENABLED=true
NETBIOS_TIMEOUT_MS=500
```

Logic:

```text
active = last_activity_at >= now - activity_window
```

When no traffic exists:

```json
{
  "last_activity_at": null,
  "active": false
}
```

Do not replace existing `online` or `offline` status with `active`.

## Repository

Add a separate query method, for example:

```go
func (r *DeviceRepository) BandwidthMetrics(
    ctx context.Context,
    window time.Duration,
    limit int,
) ([]BandwidthMetric, error)
```

Query rules:

1. Use `devices` as the identity source.
2. Left join traffic aggregation by device ID or IP.
3. Keep devices without traffic with zero counters.
4. Filter by requested time window.
5. Sort by `total_bytes DESC` by default.
6. Apply validated limit.
7. Calculate `active` from `last_activity_at`.

Do not make the HTTP handler call MikroTik or read collector memory.

## Collector Lifecycle

Add optional collector startup in `cmd/worker/main.go`:

```text
database migration
  -> existing discovery worker
  -> optional traffic collector
  -> HTTP API
```

Configuration:

```env
TRAFFIC_ENABLED=false
NETFLOW_LISTEN_ADDR=:2055
TRAFFIC_BUCKET_SECONDS=10
TRAFFIC_FLUSH_SECONDS=5
TRAFFIC_ACTIVITY_WINDOW_SECONDS=60
```

Lifecycle rules:

- Use existing shutdown context.
- Collector must not block HTTP API startup.
- Collector must not call `scanner.Scan`.
- Collector errors are logged and isolated.
- UDP bind failure must not stop discovery or API.
- Avoid goroutine leaks on shutdown.

## Existing Logic Protection

Do not change:

- `scanner.Scan` contract.
- ARP request logic.
- ARP response parsing.
- Hostname cache behavior.
- `repository.Sync` behavior.
- Existing online/offline status logic.
- Existing discovery interval.
- Existing endpoint response schemas.
- Existing authentication middleware.

Allowed additive changes:

- new traffic model
- additive migration
- new traffic package
- new repository methods
- new route and handler
- optional traffic configuration
- optional collector goroutine
- UDP Docker/firewall configuration

## Tests

### Parser tests

- valid NetFlow v9 record
- valid IPFIX template and record
- short packet
- unsupported version
- missing template
- missing bytes field
- invalid source or destination IP

### Aggregator tests

- local source becomes upload
- local destination becomes download
- LAN-to-LAN direction
- external-to-external flow ignored
- multiple flows combine in one bucket
- counter reset handled
- duplicate flow handled
- bucket boundaries handled

### Repository tests

- device without traffic remains visible
- device with traffic includes hostname
- join uses device identity correctly
- sorting by total bytes descending
- window filtering
- limit enforcement

### API tests

- `GET /api/metrics/bandwidth`
- expected sample schema
- empty data
- invalid `window`
- invalid `limit`
- database error
- authentication behavior
- existing endpoints unchanged

### Concurrency tests

Run:

```bash
go test -race ./...
```

Cover:

- concurrent flow ingestion
- flush concurrent with ingestion
- context shutdown
- no data race
- no goroutine leak

## MikroTik Configuration

One-time MikroTik configuration example:

```routeros
/ip traffic-flow
set enabled=yes interfaces=all

/ip traffic-flow target
add dst-address=SCRLAN_IP port=2055 version=9
```

Traffic Flow limitations:

- near-realtime, not packet-by-packet realtime
- hardware-offloaded bridged traffic may not appear
- router must process the traffic flow
- UDP flow data is not authenticated or encrypted by default

Restrict UDP `2055` to the MikroTik source IP at host firewall level.

## Implementation Order

1. Add compatibility tests for existing endpoint contracts.
2. Add internal traffic contracts.
3. Implement NetFlow v9 parser.
4. Add parser unit tests.
5. Implement in-memory aggregator.
6. Add aggregator tests.
7. Add `DeviceTraffic` model.
8. Add additive migration.
9. Add traffic repository query and batch persistence.
10. Add UDP collector.
11. Add periodic flush.
12. Start collector optionally from `main`.
13. Add `GET /api/metrics/bandwidth`.
14. Add API tests.
15. Add Docker UDP configuration if required by deployment.
16. Test with real MikroTik flow data.
17. Run full tests and race detector.
18. Audit diff for unchanged existing logic and responses.

## Acceptance Criteria

- `/api/metrics/bandwidth` exists.
- Response matches documented schema.
- Default ranking is `total_bytes DESC`.
- Identity comes from `devices`.
- Traffic comes from NetFlow/IPFIX.
- Existing hostname values remain unchanged.
- Existing status values remain discovery-driven.
- `active` uses `last_activity_at`.
- Devices without traffic appear with zero counters.
- Existing endpoint responses do not change.
- `TRAFFIC_ENABLED=false` preserves old behavior.
- Collector does not block API startup.
- Collector failure does not stop discovery.
- No row per packet.
- No data race.
- ARP scan logic remains unchanged.
