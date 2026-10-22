package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/muhammadsaeful/scrlan/internal/repository"
	"github.com/muhammadsaeful/scrlan/internal/scanner"
)

type Handler struct {
	devices *repository.DeviceRepository
	scanner scanner.Scanner
	token   string
}

func NewHandler(devices *repository.DeviceRepository, scan scanner.Scanner, token string) http.Handler {
	h := &Handler{devices: devices, scanner: scan, token: token}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", h.health)
	mux.HandleFunc("/api/devices", h.devicesList)
	mux.HandleFunc("/api/scan", h.scan)
	mux.HandleFunc("/api/metrics/devices", h.deviceMetrics)
	mux.HandleFunc("/api/metrics/bandwidth", h.bandwidthMetrics)
	return h.auth(mux)
}

func (h *Handler) scan(w http.ResponseWriter, r *http.Request) {
	netFrom := r.URL.Query().Get("net_from")
	netTo := r.URL.Query().Get("net_to")
	if _, _, err := scanner.ParseIPRange(netFrom, netTo); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	scannedDevices, err := h.scanner.Scan(netFrom, netTo)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.devices.Sync(scannedDevices, time.Now().UTC()); err != nil {
		writeError(w, http.StatusInternalServerError, "unable to sync scanned devices")
		return
	}
	devices, err := h.devices.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to retrieve devices")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "net_from": netFrom, "net_to": netTo, "data": devices})
}

func (h *Handler) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.token != "" {
			value := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if value != h.token {
				writeError(w, http.StatusUnauthorized, "authentication required")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "status": "ok"})
}

func (h *Handler) devicesList(w http.ResponseWriter, r *http.Request) {
	devices, err := h.devices.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to retrieve devices")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": devices})
}

func (h *Handler) deviceMetrics(w http.ResponseWriter, r *http.Request) {
	counts, err := h.devices.Counts()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to retrieve device metrics")
		return
	}
	data := []map[string]any{
		{"metric": "devices_online", "value": counts["online"]},
		{"metric": "devices_offline", "value": counts["offline"]},
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": data})
}

func (h *Handler) bandwidthMetrics(w http.ResponseWriter, r *http.Request) {
	window, err := parseWindow(r.URL.Query().Get("window"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid window")
		return
	}
	limit := 50
	if value := r.URL.Query().Get("limit"); value != "" {
		limit, err = strconv.Atoi(value)
		if err != nil || limit <= 0 || limit > 500 {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
	}
	sortField := r.URL.Query().Get("sort")
	if sortField == "" {
		sortField = "total_bytes"
	}
	if sortField != "total_bytes" && sortField != "bytes_in" && sortField != "bytes_out" {
		writeError(w, http.StatusBadRequest, "invalid sort")
		return
	}
	order := r.URL.Query().Get("order")
	if order == "" {
		order = "desc"
	}
	if order != "asc" && order != "desc" {
		writeError(w, http.StatusBadRequest, "invalid order")
		return
	}
	activityWindow := 60 * time.Second
	if value := os.Getenv("TRAFFIC_ACTIVITY_WINDOW_SECONDS"); value != "" {
		seconds, parseErr := strconv.Atoi(value)
		if parseErr != nil || seconds <= 0 {
			writeError(w, http.StatusInternalServerError, "invalid traffic activity configuration")
			return
		}
		activityWindow = time.Duration(seconds) * time.Second
	}
	if r.URL.Query().Get("by_subnet") == "true" {
		data, queryErr := h.devices.SubnetBandwidthMetrics(r.Context(), window, activityWindow, limit, sortField, order)
		if queryErr != nil {
			writeError(w, http.StatusInternalServerError, "unable to retrieve bandwidth metrics")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": data})
		return
	}
	data, queryErr := h.devices.BandwidthMetrics(r.Context(), window, activityWindow, limit, sortField, order)
	if queryErr != nil {
		writeError(w, http.StatusInternalServerError, "unable to retrieve bandwidth metrics")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": data})
}

func parseWindow(value string) (time.Duration, error) {
	if value == "" {
		return time.Hour, nil
	}
	switch value {
	case "1h":
		return time.Hour, nil
	case "24h":
		return 24 * time.Hour, nil
	case "6h":
		return 6 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unsupported window")
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"success": false, "message": message})
}
