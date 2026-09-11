package repository

import (
	"time"

	"github.com/muhammadsaeful/scrlan/internal/model"
	"github.com/muhammadsaeful/scrlan/internal/scanner"
	"gorm.io/gorm"
)

type DeviceRepository struct {
	db *gorm.DB
}

func NewDeviceRepository(db *gorm.DB) *DeviceRepository {
	return &DeviceRepository{db: db}
}

func (r *DeviceRepository) Sync(devices []scanner.Device, seenAt time.Time) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		seenIPs := make([]string, 0, len(devices))
		for _, device := range devices {
			seenIPs = append(seenIPs, device.IP)
			var current model.Device
			err := tx.Where("ip = ?", device.IP).First(&current).Error
			if err == gorm.ErrRecordNotFound {
				current = model.Device{IP: device.IP, FirstSeen: seenAt}
			} else if err != nil {
				return err
			}

			current.MAC = device.MAC
			current.Hostname = device.Hostname
			current.Manufacturer = device.Manufacturer
			current.Status = model.DeviceStatusOnline
			current.LastSeen = seenAt
			if err := tx.Save(&current).Error; err != nil {
				return err
			}

			history := model.DeviceHistory{
				DeviceID:     current.ID,
				IP:           device.IP,
				MAC:          device.MAC,
				Hostname:     device.Hostname,
				Manufacturer: device.Manufacturer,
				FirstSeen:    seenAt,
				LastSeen:     seenAt,
			}
			if err := tx.Create(&history).Error; err != nil {
				return err
			}
		}

		query := tx.Model(&model.Device{}).Where("status = ?", model.DeviceStatusOnline)
		if len(seenIPs) > 0 {
			query = query.Where("ip NOT IN ?", seenIPs)
		}
		return query.Update("status", model.DeviceStatusOffline).Error
	})
}

func (r *DeviceRepository) List() ([]model.Device, error) {
	var devices []model.Device
	err := r.db.Order("ip ASC").Find(&devices).Error
	return devices, err
}

func (r *DeviceRepository) Counts() (map[string]int64, error) {
	counts := make(map[string]int64)
	rows, err := r.db.Model(&model.Device{}).Select("status, count(*) as total").Group("status").Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var total int64
		if err := rows.Scan(&status, &total); err != nil {
			return nil, err
		}
		counts[status] = total
	}
	return counts, rows.Err()
}
