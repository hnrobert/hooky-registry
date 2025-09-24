package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
)

// newDockerClient returns an http.Client that talks to the docker unix socket
func newDockerClient() *http.Client {
	tr := &http.Transport{}
	tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return net.Dial("unix", "/var/run/docker.sock")
	}
	return &http.Client{Transport: tr}
}

// helper to inspect container
func dockerInspect(containerName string) (map[string]interface{}, error) {
	client := newDockerClient()
	url := fmt.Sprintf("http://unix/v1.41/containers/%s/json", containerName)
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("inspect api error: status=%d body=%s", resp.StatusCode, string(body))
	}
	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data, nil
}

// removeImageByAPI removes image by name or id via Docker Engine API
func removeImageByAPI(image string) error {
	client := newDockerClient()
	url := fmt.Sprintf("http://unix/v1.41/images/%s?force=1&noprune=0", image)
	req, _ := http.NewRequest("DELETE", url, nil)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to remove image: status=%d body=%s", resp.StatusCode, string(body))
	}
	return nil
}
