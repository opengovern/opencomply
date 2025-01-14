package discovery

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// Config represents the JSON input configuration
type Config struct {
	Token string `json:"token"`
}

type App struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type Response struct {
	Apps []App `json:"apps"`
}

// Discover retrieves fly user info
func Discover(token string) ([]App, error) {
	var response Response

	url := "https://api.machines.dev/v1/apps?org_slug=personal"

	client := http.DefaultClient

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request execution failed: %w", err)
	}
	defer resp.Body.Close()

	if err = json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return response.Apps, nil
}

func FlyIntegrationDiscovery(cfg Config) ([]App, error) {
	// Check for the token
	if cfg.Token == "" {
		return nil, errors.New("token must be configured")
	}

	return Discover(cfg.Token)
}
