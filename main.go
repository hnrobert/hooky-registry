package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

type Config struct {
	Registry       string
	WebhookPort    int
	UpdateStrategy string
}

type WebhookHandler struct {
	config *Config
}

type RegistryWebhook struct {
	Events []struct {
		ID        string `json:"id"`
		Timestamp string `json:"timestamp"`
		Action    string `json:"action"`
		Target    struct {
			MediaType  string `json:"mediaType"`
			Digest     string `json:"digest"`
			Repository string `json:"repository"`
			Tag        string `json:"tag"`
		} `json:"target"`
		Request struct {
			ID        string `json:"id"`
			Addr      string `json:"addr"`
			Host      string `json:"host"`
			Method    string `json:"method"`
			UserAgent string `json:"useragent"`
		} `json:"request"`
		Actor struct {
			Name string `json:"name"`
		} `json:"actor"`
		Source struct {
			Addr       string `json:"addr"`
			InstanceID string `json:"instanceID"`
		} `json:"source"`
	} `json:"events"`
}

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

func (wh *WebhookHandler) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var webhook RegistryWebhook
	if err := json.NewDecoder(r.Body).Decode(&webhook); err != nil {
		log.Printf("Failed to decode webhook payload: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	for _, event := range webhook.Events {
		if event.Action == "push" {
			imageName := fmt.Sprintf("%s/%s:%s", wh.config.Registry, event.Target.Repository, event.Target.Tag)
			log.Printf("Processing push event for image: %s", imageName)

			if err := wh.handleImagePush(imageName); err != nil {
				log.Printf("Failed to handle image push: %v", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
		}
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

func (wh *WebhookHandler) handleImagePush(imageName string) error {
	// Pull the latest image
	log.Printf("Pulling image: %s", imageName)
	if err := wh.pullImage(imageName); err != nil {
		return fmt.Errorf("failed to pull image %s: %w", imageName, err)
	}

	// Find containers using this image
	containers, err := wh.findContainersUsingImage(imageName)
	if err != nil {
		return fmt.Errorf("failed to find containers: %w", err)
	}

	if len(containers) == 0 {
		log.Printf("No containers found using image: %s", imageName)
		return nil
	}

	log.Printf("Found %d containers using image %s", len(containers), imageName)

	switch wh.config.UpdateStrategy {
	case "recreate":
		return wh.recreateContainers(containers, imageName)
	case "restart":
		return wh.restartContainers(containers)
	default:
		return fmt.Errorf("unknown update strategy: %s", wh.config.UpdateStrategy)
	}
}

func (wh *WebhookHandler) pullImage(imageName string) error {
	cmd := exec.Command("docker", "pull", imageName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker pull failed: %v, output: %s", err, string(output))
	}
	log.Printf("Successfully pulled image: %s", imageName)
	return nil
}

func (wh *WebhookHandler) findContainersUsingImage(imageName string) ([]string, error) {
	// Get all containers (running and stopped)
	cmd := exec.Command("docker", "ps", "-a", "--format", "{{.Names}}:{{.Image}}")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %v", err)
	}

	var containers []string
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Split(line, ":")
		if len(parts) != 2 {
			continue
		}

		containerName := parts[0]
		containerImage := parts[1]

		// Check if container uses the same image
		if containerImage == imageName || strings.HasPrefix(containerImage, strings.Split(imageName, ":")[0]+":") {
			containers = append(containers, containerName)
		}
	}

	return containers, nil
}

func (wh *WebhookHandler) recreateContainers(containers []string, newImage string) error {
	for _, containerName := range containers {
		log.Printf("Recreating container: %s", containerName)

		// Get container configuration
		cmd := exec.Command("docker", "inspect", containerName)
		_, err := cmd.Output()
		if err != nil {
			log.Printf("Failed to inspect container %s: %v", containerName, err)
			continue
		}

		// Stop and remove the old container
		if err := wh.stopAndRemoveContainer(containerName); err != nil {
			log.Printf("Failed to stop/remove container %s: %v", containerName, err)
			continue
		}

		// Extract run command from inspect output (simplified approach)
		// In a real implementation, you'd parse the JSON and reconstruct the run command
		log.Printf("Container %s stopped and removed. Manual recreation required.", containerName)
		log.Printf("Please recreate container %s with new image %s", containerName, newImage)
	}

	return nil
}

func (wh *WebhookHandler) restartContainers(containers []string) error {
	for _, containerName := range containers {
		log.Printf("Restarting container: %s", containerName)

		cmd := exec.Command("docker", "restart", containerName)
		if err := cmd.Run(); err != nil {
			log.Printf("Failed to restart container %s: %v", containerName, err)
			continue
		}

		log.Printf("Successfully restarted container: %s", containerName)
	}

	return nil
}

func (wh *WebhookHandler) stopAndRemoveContainer(containerName string) error {
	// Stop container
	stopCmd := exec.Command("docker", "stop", containerName)
	if err := stopCmd.Run(); err != nil {
		return fmt.Errorf("failed to stop container: %v", err)
	}

	// Remove container
	removeCmd := exec.Command("docker", "rm", containerName)
	if err := removeCmd.Run(); err != nil {
		return fmt.Errorf("failed to remove container: %v", err)
	}

	return nil
}

func (wh *WebhookHandler) healthCheck(w http.ResponseWriter, r *http.Request) {
	// Simple health check - just check if service is running
	// We can test Docker access by checking socket existence
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
