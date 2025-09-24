package main

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
