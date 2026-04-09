package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/yedou37/dbd/internal/model"
	"github.com/yedou37/dbd/internal/service"
)

func NewHandler(queryService *service.QueryService) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/sql", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, model.SQLResponse{Success: false, Error: "method not allowed"})
			return
		}

		var request model.SQLRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, model.SQLResponse{Success: false, Error: err.Error()})
			return
		}

		result, err := queryService.Execute(r.Context(), request.SQL)
		if err != nil {
			var redirectError *service.LeaderRedirectError
			if errors.As(err, &redirectError) {
				writeJSON(w, http.StatusConflict, model.SQLResponse{Success: false, Error: err.Error(), Leader: redirectError.Leader})
				return
			}
			writeJSON(w, http.StatusBadRequest, model.SQLResponse{Success: false, Error: err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, model.SQLResponse{Success: true, Result: result})
	})
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		result, err := queryService.Status(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
	mux.HandleFunc("/leader", func(w http.ResponseWriter, r *http.Request) {
		result, err := queryService.Leader(r.Context())
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
	mux.HandleFunc("/members", func(w http.ResponseWriter, r *http.Request) {
		result, err := queryService.Members(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
	mux.HandleFunc("/tables", func(w http.ResponseWriter, r *http.Request) {
		result, err := queryService.Execute(r.Context(), "SHOW TABLES")
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, model.SQLResponse{Success: false, Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, model.SQLResponse{Success: true, Result: result})
	})
	mux.HandleFunc("/join", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		var request model.JoinRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		if err := queryService.Join(r.Context(), request); err != nil {
			var redirectError *service.LeaderRedirectError
			if errors.As(err, &redirectError) {
				writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error(), "leader": redirectError.Leader})
				return
			}
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "joined"})
	})

	return mux
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
