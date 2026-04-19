package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	appsettings "github.com/florian/vinculum/apps/operator/internal/settings"
)

func registerSettingsHandlers(mux *http.ServeMux, store *appsettings.Store) {
	mux.HandleFunc("/api/settings/models", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		cfg, err := store.Load(r.Context())
		if err != nil || cfg.OpenCodeAPIKey == "" {
			// Return empty list — caller will show a manual input instead.
			jsonOK(w, map[string]any{"models": []string{}})
			return
		}

		url := cfg.OpenCodeBaseURL + "/models"
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, url, nil)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, fmt.Sprintf("build request: %v", err))
			return
		}
		req.Header.Set("Authorization", "Bearer "+cfg.OpenCodeAPIKey)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			jsonError(w, http.StatusBadGateway, fmt.Sprintf("fetch models: %v", err))
			return
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)

		if resp.StatusCode != http.StatusOK {
			jsonError(w, http.StatusBadGateway, fmt.Sprintf("models API returned %d", resp.StatusCode))
			return
		}

		var result struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &result); err != nil {
			jsonError(w, http.StatusBadGateway, "models response not JSON")
			return
		}

		ids := make([]string, 0, len(result.Data))
		for _, m := range result.Data {
			// Filter to chat-capable models only (skip embedding/tts/whisper/dall-e etc.)
			id := m.ID
			if strings.HasPrefix(id, "gpt-") || strings.HasPrefix(id, "o1") || strings.HasPrefix(id, "o3") || strings.HasPrefix(id, "o4") {
				ids = append(ids, id)
			}
		}
		sort.Strings(ids)
		jsonOK(w, map[string]any{"models": ids})
	})

	mux.HandleFunc("/api/settings", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			cfg, err := store.Load(r.Context())
			if err != nil {
				jsonError(w, http.StatusInternalServerError, err.Error())
				return
			}
			// Mask the key in the GET response — return only whether it is set.
			jsonOK(w, map[string]any{
				"openCodeApiKeySet": cfg.OpenCodeAPIKey != "",
				"openCodeModel":     cfg.OpenCodeModel,
				"openCodeBaseURL":   cfg.OpenCodeBaseURL,
			})

		case http.MethodPut:
			var body struct {
				OpenCodeAPIKey  string `json:"openCodeApiKey"`
				OpenCodeModel   string `json:"openCodeModel"`
				OpenCodeBaseURL string `json:"openCodeBaseURL"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				jsonError(w, http.StatusBadRequest, "invalid JSON")
				return
			}
			existing, _ := store.Load(r.Context())
			// Allow a PUT that only updates model/URL (empty key = keep existing).
			if body.OpenCodeAPIKey == "" {
				body.OpenCodeAPIKey = existing.OpenCodeAPIKey
			}
			if body.OpenCodeBaseURL == "" {
				body.OpenCodeBaseURL = existing.OpenCodeBaseURL
			}
			if err := store.Save(r.Context(), appsettings.Settings{
				OpenCodeAPIKey:  body.OpenCodeAPIKey,
				OpenCodeModel:   body.OpenCodeModel,
				OpenCodeBaseURL: body.OpenCodeBaseURL,
			}); err != nil {
				jsonError(w, http.StatusInternalServerError, err.Error())
				return
			}
			jsonOK(w, map[string]any{
				"openCodeApiKeySet": body.OpenCodeAPIKey != "",
				"openCodeModel":     body.OpenCodeModel,
				"openCodeBaseURL":   body.OpenCodeBaseURL,
			})

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}
