package auth

import (
	ghAuth "github.com/cli/go-gh/v2/pkg/auth"
)

// ResolveToken tries to find a GitHub token for the given host.
// It checks in order: env vars (GH_TOKEN/GITHUB_TOKEN), gh CLI config, jip config.
// Returns the token and a human-readable source description.
func ResolveToken(host string) (token, source string) {
	// 1. Environment variables and gh CLI config (go-gh handles both)
	token, tokenSource := ghAuth.TokenForHost(host)
	if token != "" {
		switch tokenSource {
		case "GH_TOKEN", "GITHUB_TOKEN":
			return token, tokenSource
		default:
			return token, "gh CLI config"
		}
	}

	// 2. jip's own config file
	cfg, err := LoadConfig()
	if err == nil {
		if hostCfg, ok := cfg[host]; ok && hostCfg.OAuthToken != "" {
			return hostCfg.OAuthToken, "jip config"
		}
	}

	return "", ""
}
