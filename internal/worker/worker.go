package worker

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/muhammadsaeful/scrlan/internal/repository"
	"github.com/muhammadsaeful/scrlan/internal/scanner"
)

type Worker struct {
	scanner    scanner.Scanner
	repository *repository.DeviceRepository
	netFrom    string
	netTo      string
	interval   time.Duration
}

func New(s scanner.Scanner, repo *repository.DeviceRepository, netFrom string, netTo string, interval time.Duration) (*Worker, error) {
	if s == nil || repo == nil {
		return nil, fmt.Errorf("scanner and repository are required")
	}
	if _, _, err := scanner.ParseIPRange(netFrom, netTo); err != nil {
		return nil, err
	}
	if interval <= 0 {
		return nil, fmt.Errorf("scan interval must be positive")
	}
	return &Worker{scanner: s, repository: repo, netFrom: netFrom, netTo: netTo, interval: interval}, nil
}

func (w *Worker) RunOnce(ctx context.Context) error {
	devices, err := w.scanner.Scan(w.netFrom, w.netTo)
	if err != nil {
		return fmt.Errorf("scan range %s-%s: %w", w.netFrom, w.netTo, err)
	}
	if err := w.repository.Sync(devices, time.Now().UTC()); err != nil {
		return fmt.Errorf("sync scanned devices: %w", err)
	}
	return nil
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		if err := w.RunOnce(ctx); err != nil {
			log.Printf("scan failed: %v", err)
		} else {
			log.Printf("scan completed for range %s-%s", w.netFrom, w.netTo)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
