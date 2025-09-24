package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

func NewWebhookHandler() *WebhookHandler {
	config := &Config{
		Registry:       getEnv("REGISTRY", "127.0.0.1:5000"),
		WebhookPort:    getEnvInt("WEBHOOK_PORT", 5001),
		UpdateStrategy: getEnv("UPDATE_STRATEGY", "recreate"),
	}

	return &WebhookHandler{
		config: config,
	}
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting Hooky Registry webhook receiver...")

	handler := NewWebhookHandler()

	router := mux.NewRouter()
	router.HandleFunc("/webhook", handler.handleWebhook).Methods("POST")
	router.HandleFunc("/health", handler.healthCheck).Methods("GET")
	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"service": "hooky-registry",
			"version": "2.0.0-go",
			"status":  "running",
		})
	}).Methods("GET")

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", handler.config.WebhookPort),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	log.Printf("Webhook receiver listening on port %d", handler.config.WebhookPort)
	log.Printf("Registry: %s", handler.config.Registry)
	log.Printf("Update strategy: %s", handler.config.UpdateStrategy)

	log.Fatal(server.ListenAndServe())
}
