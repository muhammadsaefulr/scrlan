package repository

import (
	"context"
	"time"

	"github.com/muhammadsaeful/scrlan/internal/model"
	"github.com/muhammadsaeful/scrlan/internal/traffic"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type BandwidthMetric struct {
	IP             string     `json:"ip"`
	Hostname       string     `json:"hostname"`
	Status         string     `json:"status"`
	BytesIn        uint64     `json:"bytes_in"`
	BytesOut       uint64     `json:"bytes_out"`
	TotalBytes     uint64     `json:"total_bytes"`
	DownloadBPS    uint64     `json:"download_bps"`
	UploadBPS      uint64     `json:"upload_bps"`
	LastActivityAt *time.Time `json:"last_activity_at"`
	Active         bool       `json:"active"`
}

type SubnetBandwidthMetric struct {
	Subnet         string     `json:"subnet"`
	BytesIn        uint64     `json:"bytes_in"`
	BytesOut       uint64     `json:"bytes_out"`
	TotalBytes     uint64     `json:"total_bytes"`
	DownloadBPS    uint64     `json:"download_bps"`
	UploadBPS      uint64     `json:"upload_bps"`
	LastActivityAt *time.Time `json:"last_activity_at"`
	Active         bool       `json:"active"`
}

func (r *DeviceRepository) FlushTraffic(buckets []traffic.Bucket) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		for _, bucket := range buckets {
			var device model.Device
			if err := tx.Select("id").Where("ip = ?", bucket.IP).Limit(1).Find(&device).Error; err != nil {
				return err
			}
			if device.ID == 0 {
				continue
			}
			row := model.DeviceTraffic{
				DeviceID:       device.ID,
				IP:             bucket.IP,
				BucketStart:    bucket.BucketStart,
				BucketEnd:      bucket.BucketEnd,
				BytesIn:        bucket.BytesIn,
				BytesOut:       bucket.BytesOut,
				PacketsIn:      bucket.PacketsIn,
				PacketsOut:     bucket.PacketsOut,
				LastActivityAt: &bucket.LastActivityAt,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "device_id"}, {Name: "bucket_start"}},
				DoUpdates: clause.Assignments(map[string]any{
					"bytes_in":         gorm.Expr("device_traffics.bytes_in + EXCLUDED.bytes_in"),
					"bytes_out":        gorm.Expr("device_traffics.bytes_out + EXCLUDED.bytes_out"),
					"packets_in":       gorm.Expr("device_traffics.packets_in + EXCLUDED.packets_in"),
					"packets_out":      gorm.Expr("device_traffics.packets_out + EXCLUDED.packets_out"),
					"last_activity_at": gorm.Expr("GREATEST(device_traffics.last_activity_at, EXCLUDED.last_activity_at)"),
					"updated_at":       time.Now().UTC(),
				}),
			}).Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *DeviceRepository) BandwidthMetrics(ctx context.Context, window, activityWindow time.Duration, limit int, sortField, order string) ([]BandwidthMetric, error) {
	cutoff := time.Now().UTC().Add(-window)
	activityCutoff := time.Now().UTC().Add(-activityWindow)
	var rows []BandwidthMetric
	orderSQL := metricOrder(sortField, order, "d.ip")
	err := r.db.WithContext(ctx).Raw(`
		SELECT d.ip AS ip, d.hostname AS hostname, d.status AS status,
		       COALESCE(SUM(t.bytes_in), 0) AS bytes_in, COALESCE(SUM(t.bytes_out), 0) AS bytes_out,
		       MAX(t.last_activity_at) AS last_activity_at
		FROM devices d
		LEFT JOIN device_traffics t ON t.device_id = d.id AND t.bucket_start >= ?
		GROUP BY d.id, d.ip, d.hostname, d.status
		ORDER BY `+orderSQL+`
		LIMIT ?`, cutoff, limit).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].TotalBytes = rows[i].BytesIn + rows[i].BytesOut
		rows[i].DownloadBPS = rate(rows[i].BytesIn, window)
		rows[i].UploadBPS = rate(rows[i].BytesOut, window)
		rows[i].Active = rows[i].LastActivityAt != nil && !rows[i].LastActivityAt.Before(activityCutoff)
	}
	return rows, nil
}

func (r *DeviceRepository) SubnetBandwidthMetrics(ctx context.Context, window, activityWindow time.Duration, limit int, sortField, order string) ([]SubnetBandwidthMetric, error) {
	cutoff := time.Now().UTC().Add(-window)
	activityCutoff := time.Now().UTC().Add(-activityWindow)
	var rows []SubnetBandwidthMetric
	orderSQL := metricOrder(sortField, order, "1")
	err := r.db.WithContext(ctx).Raw(`
		SELECT network(set_masklen(d.ip::inet, 24))::text AS subnet,
		       COALESCE(SUM(t.bytes_in), 0) AS bytes_in, COALESCE(SUM(t.bytes_out), 0) AS bytes_out,
		       MAX(t.last_activity_at) AS last_activity_at
		FROM devices d
		LEFT JOIN device_traffics t ON t.device_id = d.id AND t.bucket_start >= ?
		GROUP BY network(set_masklen(d.ip::inet, 24))
		ORDER BY `+orderSQL+`
		LIMIT ?`, cutoff, limit).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].TotalBytes = rows[i].BytesIn + rows[i].BytesOut
		rows[i].DownloadBPS = rate(rows[i].BytesIn, window)
		rows[i].UploadBPS = rate(rows[i].BytesOut, window)
		rows[i].Active = rows[i].LastActivityAt != nil && !rows[i].LastActivityAt.Before(activityCutoff)
	}
	return rows, nil
}

func metricOrder(field, direction, tie string) string {
	column := "(COALESCE(SUM(t.bytes_in), 0) + COALESCE(SUM(t.bytes_out), 0))"
	switch field {
	case "bytes_in":
		column = "COALESCE(SUM(t.bytes_in), 0)"
	case "bytes_out":
		column = "COALESCE(SUM(t.bytes_out), 0)"
	}
	if direction != "asc" {
		direction = "desc"
	}
	return column + " " + direction + ", " + tie + " ASC"
}

func rate(bytes uint64, window time.Duration) uint64 {
	seconds := uint64(window / time.Second)
	if seconds == 0 {
		return 0
	}
	return bytes * 8 / seconds
}
