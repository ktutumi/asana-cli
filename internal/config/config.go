package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type TokenData struct {
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
	ExpiresAt    string `json:"expires_at,omitempty"`
}

type StoredConfig struct {
	ClientID    string     `json:"clientId,omitempty"`
	RedirectURI string     `json:"redirectUri,omitempty"`
	Token       *TokenData `json:"token,omitempty"`
}

func DefaultPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "asana-cli", "credentials.json")
	}
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		dir = "."
	}
	return filepath.Join(dir, "asana-cli", "credentials.json")
}

func LoadConfig(path string) (StoredConfig, error) {
	if path == "" {
		path = DefaultPath()
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return StoredConfig{}, nil
		}
		return StoredConfig{}, err
	}
	if len(b) == 0 {
		return StoredConfig{}, nil
	}
	var cfg StoredConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return StoredConfig{}, err
	}
	return cfg, nil
}

func SaveConfig(path string, next StoredConfig) error {
	if path == "" {
		path = DefaultPath()
	}
	cur, err := LoadConfig(path)
	if err != nil {
		return err
	}
	merged := merge(cur, next)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	_ = os.Chmod(filepath.Dir(path), 0o700)
	b, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func merge(a, b StoredConfig) StoredConfig {
	out := a
	if b.ClientID != "" {
		out.ClientID = b.ClientID
	}
	if b.RedirectURI != "" {
		out.RedirectURI = b.RedirectURI
	}
	if b.Token != nil {
		if out.Token == nil {
			out.Token = &TokenData{}
		}
		if b.Token.AccessToken != "" {
			out.Token.AccessToken = b.Token.AccessToken
		}
		if b.Token.RefreshToken != "" {
			out.Token.RefreshToken = b.Token.RefreshToken
		}
		if b.Token.TokenType != "" {
			out.Token.TokenType = b.Token.TokenType
		}
		if b.Token.ExpiresIn != 0 {
			out.Token.ExpiresIn = b.Token.ExpiresIn
		}
		if b.Token.ExpiresAt != "" {
			out.Token.ExpiresAt = b.Token.ExpiresAt
		}
	}
	return out
}
