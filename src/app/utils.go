package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"time"
)

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func (wh *WebhookHandler) healthCheck(w http.ResponseWriter, r *http.Request) {
	dockerSocketExists := false
	if _, err := os.Stat("/var/run/docker.sock"); err == nil {
		dockerSocketExists = true
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":                  "healthy",
		"timestamp":               time.Now().UTC().Format(time.RFC3339),
		"docker_socket_available": dockerSocketExists,
		"config": map[string]interface{}{
			"registry":        wh.config.Registry,
			"webhook_port":    wh.config.WebhookPort,
			"update_strategy": wh.config.UpdateStrategy,
		},
	})
}
