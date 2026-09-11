package model

import "time"

type DeviceHistory struct {
	ID uint `gorm:"primaryKey"`

	DeviceID uint `gorm:"index;not null"`

	IP           string `gorm:"type:inet;not null"`
	MAC          string `gorm:"type:varchar(17);not null"`
	Hostname     string `gorm:"type:varchar(255)"`
	Manufacturer string `gorm:"type:varchar(255)"`

	FirstSeen time.Time
	LastSeen  time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}
