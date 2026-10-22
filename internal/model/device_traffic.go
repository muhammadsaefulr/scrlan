package model

import "time"

type DeviceTraffic struct {
	ID             uint      `gorm:"primaryKey"`
	DeviceID       uint      `gorm:"uniqueIndex:idx_device_traffic_bucket;not null"`
	IP             string    `gorm:"type:inet;index:idx_ip_traffic_bucket;not null"`
	BucketStart    time.Time `gorm:"uniqueIndex:idx_device_traffic_bucket;index:idx_ip_traffic_bucket;not null"`
	BucketEnd      time.Time
	BytesIn        uint64
	BytesOut       uint64
	PacketsIn      uint64
	PacketsOut     uint64
	LastActivityAt *time.Time `gorm:"index"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
