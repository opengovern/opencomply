package healthcheck

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

type User struct {
	ID        string `json:"id"`
	LoginName string `json:"loginName"`
	Role      string `json:"role"`
	Status    string `json:"status"`
}

type Response struct {
	Users []User `json:"users"`
}

// IsHealthy checks if the JWT has read access to all required resources
func IsHealthy(token string) error {
	var response Response

	url := "https://api.tailscale.com/api/v2/tailnet/-/users"

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

	if err = json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if len(response.Users) == 0 {
		return errors.New("no user defined on this token")
	}

	return nil
}

func TailScaleIntegrationHealthcheck(cfg Config) (bool, error) {
	// Check for the token
	if cfg.Token == "" {
		return false, errors.New("token must be configured")
	}

	err := IsHealthy(cfg.Token)
	if err != nil {
		return false, err
	}

	return true, nil
}
