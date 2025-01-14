package healthcheck

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// Config represents the JSON input configuration
type Config struct {
	Token   string `json:"token"`
	AppName string `json:"app_name"`
}

type App struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// IsHealthy checks if the JWT has read access to all required resources
func IsHealthy(token, appName string) error {
	var app App

	url := fmt.Sprintf("https://api.machines.dev/v1/apps/%s", appName)

	client := http.DefaultClient

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request execution failed: %w", err)
	}
	defer resp.Body.Close()

	if err = json.NewDecoder(resp.Body).Decode(&app); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}

func FlyIntegrationHealthcheck(cfg Config) (bool, error) {
	// Check for the token
	if cfg.Token == "" {
		return false, errors.New("token must be configured")
	}

	if cfg.AppName == "" {
		return false, errors.New("app name must be configured")
	}

	err := IsHealthy(cfg.Token, cfg.AppName)
	if err != nil {
		return false, err
	}

	return true, nil
}
