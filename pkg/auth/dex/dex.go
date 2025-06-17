package auth

import (
	"context"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type Config struct {
	BaseURL      string // Authorization base url
	ClientID     string // client id oauth
	RedirectURL  string // valid redirect url
	ClientSecret string // dex client secret
	Realm        string // dex realm
}

// Client struct holds all components needed for authentication
type Client struct {
	Provider *oidc.Provider        // Handles OIDC protocol operations with dex
	OIDC     *oidc.IDTokenVerifier // Verifies JWT tokens from dex
	Oauth    oauth2.Config         // Manages OAuth2 flow (authorization codes, tokens)
}

func New(ctx context.Context, config *Config) (*Client, error) {
	// Construct the provider URL using dex realm
	// providerURL := fmt.Sprintf("%s/realms/%s", config.BaseURL, config.Realm)
	providerURL := "http://127.0.0.1:5556/dex" // FIXME not hardcode this

	provider, err := oidc.NewProvider(ctx, providerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider: %v", err)
	}

	// Create ID token verifier
	verifier := provider.Verifier(&oidc.Config{
		ClientID: config.ClientID,
	})

	// Configure an OpenID Connect aware OAuth2 client with specific scopes:
	// - oidc.ScopeOpenID: Required for OpenID Connect authentication, provides subject ID (sub)
	// - "roles": dex-specific scope to get user roles in the token
	oauth2Config := oauth2.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		RedirectURL:  config.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes: []string{
			oidc.ScopeOpenID, // Required for OIDC authentication
		},
	}

	// Return initialized client with all required components
	return &Client{
		// oauth2Config: Used for OAuth2 operations like:
		// - Generating login URL (AuthCodeURL)
		// - Exchanging auth code for tokens (Exchange)
		// - Managing token refresh
		Oauth: oauth2Config,

		// verifier: Used to validate tokens:
		// - Verifies JWT signature
		// - Validates token claims (exp, iss, aud)
		// - Extracts user information
		OIDC: verifier,

		// provider: dex OIDC provider that:
		// - Provides endpoint URLs (auth, token)
		// - Handles OIDC protocol details
		// - Manages provider metadata
		Provider: provider,
	}, nil
}

// AuthCodeURL generates the login URL for OAuth2 authorization code flow.
// It returns a URL that the user should be redirected to for authentication.
// The state parameter is a random string that will be validated in the callback
// to prevent CSRF attacks.
func (c *Client) AuthCodeURL(state string) string {
	return c.Oauth.AuthCodeURL(state)
}
