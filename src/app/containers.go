package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

func (wh *WebhookHandler) recreateContainers(containers []string, newImage string) error {
	for _, containerName := range containers {
		log.Printf("Processing container for recreation: %s", containerName)

		data, err := dockerInspect(containerName)
		if err != nil {
			log.Printf("Failed to inspect container %s: %v", containerName, err)
			continue
		}

		labels := map[string]string{}
		if cfg, ok := data["Config"].(map[string]interface{}); ok {
			if l, ok := cfg["Labels"].(map[string]interface{}); ok {
				for k, v := range l {
					labels[k] = fmt.Sprint(v)
				}
			}
		}

		var envs []string
		if cfg, ok := data["Config"].(map[string]interface{}); ok {
			if ev, ok := cfg["Env"].([]interface{}); ok {
				for _, e := range ev {
					envs = append(envs, fmt.Sprint(e))
				}
			}
		}

		var binds []string
		var ports []string
		var restartPolicy string
		if hc, ok := data["HostConfig"].(map[string]interface{}); ok {
			if b, ok := hc["Binds"].([]interface{}); ok {
				for _, bi := range b {
					binds = append(binds, fmt.Sprint(bi))
				}
			}
			if rp, ok := hc["RestartPolicy"].(map[string]interface{}); ok {
				restartPolicy = fmt.Sprint(rp["Name"])
			}
			if pb, ok := hc["PortBindings"].(map[string]interface{}); ok {
				for containerPort, binding := range pb {
					if arr, ok := binding.([]interface{}); ok && len(arr) > 0 {
						if m, ok := arr[0].(map[string]interface{}); ok {
							hostPort := fmt.Sprint(m["HostPort"])
							parts := strings.Split(containerPort, "/")
							ports = append(ports, fmt.Sprintf("%s:%s", hostPort, parts[0]))
						}
					}
				}
			}
		}

		var networks []string
		if ns, ok := data["NetworkSettings"].(map[string]interface{}); ok {
			if nets, ok := ns["Networks"].(map[string]interface{}); ok {
				for netName := range nets {
					networks = append(networks, netName)
				}
			}
		}

		service := labels["com.docker.compose.service"]
		stack := labels["com.docker.stack.namespace"]

		var oldImage string
		if cfg, ok := data["Config"].(map[string]interface{}); ok {
			if img, ok := cfg["Image"]; ok {
				oldImage = fmt.Sprint(img)
			}
		}

		if err := wh.stopAndRemoveContainer(containerName); err != nil {
			log.Printf("Failed to stop/remove container %s: %v", containerName, err)
			continue
		}

		if stack != "" && service != "" {
			fullService := fmt.Sprintf("%s_%s", stack, service)
			log.Printf("Detected stack '%s', attempting service update for %s to image %s via API", stack, fullService, newImage)
			client := newDockerClient()
			svcURL := fmt.Sprintf("http://unix/v1.41/services/%s", fullService)
			resp, err := client.Get(svcURL)
			if err == nil {
				if resp.StatusCode == 200 {
					var svc map[string]interface{}
					if err := json.NewDecoder(resp.Body).Decode(&svc); err == nil {
						if resp.Body != nil {
							resp.Body.Close()
						}
						if spec, ok := svc["Spec"].(map[string]interface{}); ok {
							if task, ok := spec["TaskTemplate"].(map[string]interface{}); ok {
								if containerSpec, ok := task["ContainerSpec"].(map[string]interface{}); ok {
									containerSpec["Image"] = newImage
								}
							}
							version := 1
							if v, ok := svc["Version"].(map[string]interface{}); ok {
								if idx, ok := v["Index"].(float64); ok {
									version = int(idx)
								}
							}
							updateURL := fmt.Sprintf("http://unix/v1.41/services/%s/update?version=%d", fullService, version)
							bodyBytes, _ := json.Marshal(spec)
							upr, _ := http.NewRequest("POST", updateURL, strings.NewReader(string(bodyBytes)))
							upr.Header.Set("Content-Type", "application/json")
							upresp, err := client.Do(upr)
							if err == nil {
								if upresp.Body != nil {
									upresp.Body.Close()
								}
								if upresp.StatusCode < 400 {
									log.Printf("Requested service update for %s", fullService)
									if oldImage != "" {
										removeImageByAPI(oldImage)
									}
									continue
								}
								log.Printf("Service update API returned status %d for %s", upresp.StatusCode, fullService)
							} else {
								log.Printf("Service update API request failed for %s: %v", fullService, err)
							}
						}
					}
				}
				if resp.Body != nil {
					resp.Body.Close()
				}
			}
		}

		log.Printf("Falling back to API-based recreation for container %s", containerName)
		client := newDockerClient()

		createBody := map[string]interface{}{"Image": newImage}
		if len(envs) > 0 {
			createBody["Env"] = envs
		}
		hostConfig := map[string]interface{}{}
		if len(binds) > 0 {
			hostConfig["Binds"] = binds
		}
		if restartPolicy != "" {
			hostConfig["RestartPolicy"] = map[string]interface{}{"Name": restartPolicy}
		}
		if len(ports) > 0 {
			pb := map[string]interface{}{}
			for _, p := range ports {
				parts := strings.Split(p, ":")
				if len(parts) == 2 {
					host := parts[0]
					container := parts[1]
					pb[container+"/tcp"] = []map[string]string{{"HostPort": host}}
				}
			}
			hostConfig["PortBindings"] = pb
		}
		if len(hostConfig) > 0 {
			createBody["HostConfig"] = hostConfig
		}
		if len(labels) > 0 {
			createBody["Labels"] = labels
		}
		if len(networks) > 0 {
			netCfg := map[string]interface{}{"EndpointsConfig": map[string]interface{}{}}
			for _, n := range networks {
				netCfg["EndpointsConfig"].(map[string]interface{})[n] = map[string]interface{}{}
			}
			createBody["NetworkingConfig"] = netCfg
		}

		createURL := fmt.Sprintf("http://unix/v1.41/containers/create?name=%s", containerName)
		rb, _ := json.Marshal(createBody)
		req, _ := http.NewRequest("POST", createURL, strings.NewReader(string(rb)))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Failed to create container %s via API: %v", containerName, err)
		} else {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode >= 400 {
				log.Printf("Create container API error for %s: status=%d body=%s", containerName, resp.StatusCode, string(body))
			} else {
				var createResp map[string]interface{}
				if err := json.Unmarshal(body, &createResp); err == nil {
					if id, ok := createResp["Id"].(string); ok {
						startURL := fmt.Sprintf("http://unix/v1.41/containers/%s/start", id)
						sreq, _ := http.NewRequest("POST", startURL, nil)
						sresp, _ := client.Do(sreq)
						if sresp != nil {
							sresp.Body.Close()
						}
						if sresp != nil && sresp.StatusCode >= 400 {
							log.Printf("Failed to start container %s via API: status=%d", containerName, sresp.StatusCode)
						} else {
							log.Printf("Created and started container %s via API", containerName)
						}
					}
				}
			}
		}

		if oldImage != "" {
			removeImageByAPI(oldImage)
		}
	}

	return nil
}

func (wh *WebhookHandler) restartContainers(containers []string) error {
	client := newDockerClient()
	for _, containerName := range containers {
		log.Printf("Restarting container: %s", containerName)
		url := fmt.Sprintf("http://unix/v1.41/containers/%s/restart?t=10", containerName)
		req, _ := http.NewRequest("POST", url, nil)
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Failed to restart container %s: %v", containerName, err)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			log.Printf("Failed to restart container %s: status=%d body=%s", containerName, resp.StatusCode, string(body))
			continue
		}
		log.Printf("Successfully restarted container: %s", containerName)
	}
	return nil
}

func (wh *WebhookHandler) stopAndRemoveContainer(containerName string) error {
	client := newDockerClient()
	stopURL := fmt.Sprintf("http://unix/v1.41/containers/%s/stop?t=10", containerName)
	req, _ := http.NewRequest("POST", stopURL, nil)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}
	resp.Body.Close()

	removeURL := fmt.Sprintf("http://unix/v1.41/containers/%s?force=1&v=1", containerName)
	req, _ = http.NewRequest("DELETE", removeURL, nil)
	resp, err = client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to remove container: %w", err)
	}
	resp.Body.Close()
	return nil
}
