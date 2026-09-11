package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/muhammadsaeful/scrlan/internal/api"
	"github.com/muhammadsaeful/scrlan/internal/model"
	"github.com/muhammadsaeful/scrlan/internal/repository"
	"github.com/muhammadsaeful/scrlan/internal/scanner"
	"github.com/muhammadsaeful/scrlan/internal/worker"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("warning: .env file not found")
	}

	db, err := repository.NewPostgres()
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	err = db.AutoMigrate(
		&model.Device{},
		&model.DeviceHistory{},
	)
	if err != nil {
		log.Fatalf("failed to migrate database: %v", err)
	}

	log.Println("database migration completed")

	s, err := scanner.NewScanner()
	if err != nil {
		log.Fatalf("failed to initialize scanner: %v", err)
	}
	interval := 60 * time.Second
	if value := os.Getenv("SCAN_INTERVAL_SECONDS"); value != "" {
		seconds, parseErr := strconv.Atoi(value)
		if parseErr != nil || seconds <= 0 {
			log.Fatalf("invalid SCAN_INTERVAL_SECONDS: %q", value)
		}
		interval = time.Duration(seconds) * time.Second
	}
	netFrom := os.Getenv("SCAN_NET_FROM")
	netTo := os.Getenv("SCAN_NET_TO")
	if netFrom == "" || netTo == "" {
		var rangeErr error
		netFrom, netTo, rangeErr = scanner.RangeFromSubnet(os.Getenv("SCAN_SUBNET"))
		if rangeErr != nil {
			log.Fatalf("scan range configuration is invalid: %v", rangeErr)
		}
	}

	repo := repository.NewDeviceRepository(db)
	job, err := worker.New(s, repo, netFrom, netTo, interval)
	if err != nil {
		log.Fatalf("failed to initialize worker: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go job.Run(ctx)

	addr := os.Getenv("API_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	server := &http.Server{Addr: addr, Handler: api.NewHandler(repo, s, os.Getenv("API_TOKEN"))}
	go func() {
		log.Printf("metrics API listening on %s", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("metrics API failed: %v", err)
		}
	}()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
}
