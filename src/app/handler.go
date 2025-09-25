package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Request deduplication to prevent processing duplicate webhook events
var (
	processedEvents = make(map[string]time.Time)
	eventMutex      sync.RWMutex
)

// isEventProcessed checks if this event was recently processed to avoid duplicates
func isEventProcessed(imageName string) bool {
	eventMutex.RLock()
	lastProcessed, exists := processedEvents[imageName]
	eventMutex.RUnlock()

	if !exists {
		return false
	}

	// Consider event as duplicate if processed within last 10 seconds
	return time.Since(lastProcessed) < 10*time.Second
}

// markEventProcessed marks an event as processed
func markEventProcessed(imageName string) {
	eventMutex.Lock()
	processedEvents[imageName] = time.Now()

	// Clean up old entries (older than 1 minute)
	for event, timestamp := range processedEvents {
		if time.Since(timestamp) > time.Minute {
			delete(processedEvents, event)
		}
	}
	eventMutex.Unlock()
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

			// Check for duplicate events to prevent multiple processing
			if isEventProcessed(imageName) {
				log.Printf("Skipping duplicate push event for image: %s", imageName)
				continue
			}

			log.Printf("Processing push event for image: %s", imageName)
			markEventProcessed(imageName)

			// Process in background to avoid timeout
			go func(img string) {
				if err := wh.handleImagePush(img); err != nil {
					log.Printf("Failed to handle image push: %v", err)
				}
			}(imageName)
		}
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

func (wh *WebhookHandler) handleImagePush(imageName string) error {
	log.Printf("Pulling image: %s", imageName)
	if err := wh.pullImage(imageName); err != nil {
		return fmt.Errorf("failed to pull image %s: %w", imageName, err)
	}

	containers, err := wh.findContainersUsingImage(imageName)
	if err != nil {
		return fmt.Errorf("failed to find containers: %w", err)
	}

	if len(containers) == 0 {
		log.Printf("No containers found using image: %s", imageName)
		return nil
	}

	log.Printf("Found %d containers using image %s", len(containers), imageName)

	// Wait 5 seconds before updating containers
	log.Printf("Waiting 5 seconds before updating containers...")
	time.Sleep(5 * time.Second)

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
	client := newDockerClient()
	url := fmt.Sprintf("http://unix/v1.41/images/create?fromImage=%s", imageName)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("docker pull failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("docker pull api error: status=%d body=%s", resp.StatusCode, string(body))
	}
	io.Copy(io.Discard, resp.Body)
	log.Printf("Successfully pulled image: %s", imageName)
	return nil
}

func (wh *WebhookHandler) findContainersUsingImage(imageName string) ([]string, error) {
	client := newDockerClient()
	url := "http://unix/v1.41/containers/json?all=1"
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("docker api error: status=%d body=%s", resp.StatusCode, string(body))
	}

	var list []struct {
		ID    string   `json:"Id"`
		Names []string `json:"Names"`
		Image string   `json:"Image"`
		State string   `json:"State"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, err
	}

	var containers []string
	targetBaseImage := extractOriginalImageName(imageName)

	for _, c := range list {
		if len(c.Names) == 0 {
			continue
		}

		name := strings.TrimPrefix(c.Names[0], "/")
		containerBaseImage := extractOriginalImageName(c.Image)

		// Only match containers that use the exact same base image (without registry prefix)
		// This prevents cross-contamination between different images
		if containerBaseImage == targetBaseImage {
			log.Printf("Found container %s using image %s (matches %s, state: %s)", name, c.Image, imageName, c.State)
			containers = append(containers, name)
		}
	}

	log.Printf("Found %d containers using base image %s", len(containers), targetBaseImage)
	return containers, nil
}
