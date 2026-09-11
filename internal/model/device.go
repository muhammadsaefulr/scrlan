package model

import "time"

type Device struct {
	ID uint `gorm:"primaryKey"`

	IP           string `gorm:"type:inet;uniqueIndex;not null"`
	MAC          string `gorm:"type:varchar(17)"`
	Hostname     string `gorm:"type:varchar(255)"`
	Manufacturer string `gorm:"type:varchar(255)"`
	Status       string `gorm:"type:varchar(20);not null;default:offline"`
	FirstSeen    time.Time
	LastSeen     time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

const (
	DeviceStatusOnline  = "online"
	DeviceStatusOffline = "offline"
)
