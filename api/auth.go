package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func StartDeviceLogin() (*DeviceCodeResponse, error) {
	// Initiate device flow
	resp, err := client.MakeRequest("POST", "/auth/device/code", nil, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var deviceResp DeviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&deviceResp); err != nil {
		return nil, err
	}

	return &deviceResp, nil
}

// PollForToken polls the server for the authorized token.
func PollForToken(deviceCode string, hostname string) (*TokenResponse, bool, error) {
	reqBody, _ := json.Marshal(map[string]string{
		"device_code": deviceCode,
		"hostname":    hostname,
	})

	resp, err := client.MakeRequest("POST", "/auth/device/token", bytes.NewBuffer(reqBody), "application/json")
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusOK {
		var tokenResp TokenResponse
		if err := json.Unmarshal(body, &tokenResp); err != nil {
			return nil, false, err
		}
		return &tokenResp, false, nil
	}

	var errResp ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		return nil, false, err
	}

	switch errResp.Error {
	case "authorization_pending":
		return nil, true, nil // Keep polling
	case "access_denied":
		return nil, false, fmt.Errorf("authorization denied by user")
	case "expired_token":
		return nil, false, fmt.Errorf("device code expired")
	default:
		return nil, false, fmt.Errorf("error: %s", errResp.Description)
	}
}
