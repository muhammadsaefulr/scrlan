package api

import (
	"encoding/json"
	"net/http"
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
	return h.auth(mux)
}

func (h *Handler) scan(w http.ResponseWriter, r *http.Request) {
	netFrom := r.URL.Query().Get("net_from")
	netTo := r.URL.Query().Get("net_to")
	if _, _, err := scanner.ParseIPRange(netFrom, netTo); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	devices, err := h.scanner.Scan(netFrom, netTo)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.devices.Sync(devices, time.Now().UTC()); err != nil {
		writeError(w, http.StatusInternalServerError, "unable to sync scanned devices")
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

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"success": false, "message": message})
}
