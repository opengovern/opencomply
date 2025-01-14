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

type User struct {
	ID        string `json:"id"`
	LoginName string `json:"loginName"`
	Role      string `json:"role"`
	Status    string `json:"status"`
}

type Response struct {
	Users []User `json:"users"`
}

// Discover retrieves tailscale user info
func Discover(token string) (*User, error) {
	var response Response

	url := "https://api.tailscale.com/api/v2/tailnet/-/users"

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

	for _, user := range response.Users {
		if user.Role == "owner" {
			return &user, nil
		}
	}

	return nil, nil
}

func TailScaleIntegrationDiscovery(cfg Config) (*User, error) {
	// Check for the token
	if cfg.Token == "" {
		return nil, errors.New("token must be configured")
	}

	return Discover(cfg.Token)
}
