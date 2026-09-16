package oauth

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"time"

	"9router/proxy/internal/providers"
)

func init() {
	Register("cline", RefreshCline)
	Register("clinepass", RefreshCline)
}

// RefreshCline refreshes tokens using Cline's extension JSON contract.
func RefreshCline(ctx context.Context, p *Params) (*TokenResult, error) {
	prov := p.Provider
	if prov == "" {
		prov = "cline"
	}
	if p.RefreshToken == "" {
		return nil, fmt.Errorf("%s: refresh_token is required", prov)
	}

	tokenURL := "https://api.cline.bot/api/v1/auth/refresh"
	if cfg, ok := providers.KnownOAuthConfigs[prov]; ok && cfg.TokenURL != "" {
		tokenURL = cfg.TokenURL
	} else if cfg, ok := providers.KnownOAuthConfigs["cline"]; ok && cfg.TokenURL != "" {
		tokenURL = cfg.TokenURL
	}

	reqBody, err := json.Marshal(map[string]string{
		"refreshToken": p.RefreshToken,
		"grantType":    "refresh_token",
		"clientType":   "extension",
	})
	if err != nil {
		return nil, fmt.Errorf("%s: marshal request: %w", prov, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("%s: create request: %w", prov, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Cline/3.0.61")
	req.Header.Set("X-CLIENT-TYPE", "cline-cli")
	req.Header.Set("X-CLIENT-VERSION", "3.0.61")
	req.Header.Set("X-CORE-VERSION", "3.0.61")
	req.Header.Set("X-PLATFORM", "cli")
	req.Header.Set("X-PLATFORM-VERSION", "3.0.61")

	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: request failed: %w", prov, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("%s: read response: %w", prov, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: refresh returned status %d: %s", prov, resp.StatusCode, string(body))
	}

	var parsed struct {
		Success bool `json:"success"`
		Error   any  `json:"error"`
		Data    struct {
			AccessToken  string `json:"accessToken"`
			RefreshToken string `json:"refreshToken"`
			ExpiresAt    string `json:"expiresAt"`
			ExpiresIn    int    `json:"expiresIn"`
		} `json:"data"`
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresAt    string `json:"expiresAt"`
		ExpiresIn    int    `json:"expiresIn"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("%s: parse response: %w", prov, err)
	}

	if !parsed.Success && parsed.Error != nil && fmt.Sprint(parsed.Error) != "" && fmt.Sprint(parsed.Error) != "<nil>" {
		return nil, fmt.Errorf("%s: refresh failed: %v", prov, parsed.Error)
	}

	accToken := parsed.Data.AccessToken
	if accToken == "" {
		accToken = parsed.AccessToken
	}
	if accToken == "" {
		return nil, fmt.Errorf("%s: no accessToken found in response", prov)
	}

	newRefreshToken := parsed.Data.RefreshToken
	if newRefreshToken == "" {
		newRefreshToken = parsed.RefreshToken
	}
	if newRefreshToken == "" {
		newRefreshToken = p.RefreshToken
	}

	expiresIn := 3600
	expiresAtStr := parsed.Data.ExpiresAt
	if expiresAtStr == "" {
		expiresAtStr = parsed.ExpiresAt
	}
	if expiresAtStr != "" {
		if t, err := time.Parse(time.RFC3339, expiresAtStr); err == nil {
			sec := int(time.Until(t).Seconds())
			if sec > 0 {
				expiresIn = sec
			}
		}
	} else if parsed.Data.ExpiresIn > 0 {
		expiresIn = parsed.Data.ExpiresIn
	} else if parsed.ExpiresIn > 0 {
		expiresIn = parsed.ExpiresIn
	}

	return &TokenResult{
		AccessToken:  accToken,
		RefreshToken: newRefreshToken,
		ExpiresIn:    expiresIn,
	}, nil
}
