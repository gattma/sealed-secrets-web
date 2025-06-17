package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gattma/sealed-secrets-web/pkg/auth/dex"
	"github.com/gattma/sealed-secrets-web/pkg/auth/store"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
)

type AuthHandler struct {
	authClient   *auth.Client
	authStore    store.AuthStore
	sessionStore store.SessionStore
}

func NewAuthHandler(
	authClient *auth.Client,
	authStore store.AuthStore,
	sessionStore store.SessionStore,
) *AuthHandler {
	return &AuthHandler{
		authClient:   authClient,
		authStore:    authStore,
		sessionStore: sessionStore,
	}
}

// generateRandomSecureString creates a random secure string
func generateRandomSecureString() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// LoginHandler initiates the OAuth2 authorization code flow with dex.
// It generates a secure state parameter to prevent CSRF attacks and stores it
// in Redis for later verification during the callback phase.
//
// Returns:
// - 302: Redirects to dex login page
// - 500: Internal Server Error if state generation or storage fails
func (a *AuthHandler) LoginHandler(c *gin.Context) {
	state, err := generateRandomSecureString()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate state"})
		return
	}

	// Store state in session for later verification
	if err = a.authStore.SetState(c, state); err != nil {
		log.Printf("failed to set state in Redis: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save session"})
		return
	}
	// Build authentication URL
	authURL := a.authClient.Oauth.AuthCodeURL(
		state,
		oauth2.SetAuthURLParam("response_type", "code"),
		oauth2.SetAuthURLParam("scope", "openid profile email groups"),
	)

	// Redirect to dex login page
	c.Redirect(http.StatusTemporaryRedirect, authURL)
}

func (a *AuthHandler) CallbackHandler(c *gin.Context) {
	if err := a.validateStateSession(c); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to validate state session"})
		return
	}
	oauthToken, err := a.tokenExchange(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to exchange token"})
		return
	}
	userInfo, err := a.validateAndGetClaimsIDToken(c, oauthToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError,
			gin.H{"error": "Failed to validate and get claims id token"})
		return
	}
	sessionID, err := generateRandomSecureString()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate session ID"})
		return
	}

	log.Println(userInfo)

	// Create session data
	sessionData := store.SessionData{
		AccessToken: oauthToken.AccessToken,
		UserInfo: store.UserInfo{
			Username: userInfo.Username,
			Email:    userInfo.Email,
			Groups:   userInfo.Groups,
		},
		CreatedAt: time.Now(),
	}
	// Store session
	if err := a.sessionStore.Set(c, sessionID, sessionData); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store session"})
		return
	}
	// Set SameSite attribute
	// Note: Gin handles SameSite through the Config struct
	//c.SetSameSite(http.SameSiteStrictMode)
	// Set secure session cookie using Gin's methods
	c.SetCookie(
		"session_id", // name
		sessionID,    // value
		3600,         // maxAge in seconds
		"/",          // path
		"",           // domain (empty means default to current domain)
		false,        // secure (HTTPS only)
		true,         // httpOnly (prevents JavaScript access)
	)

	// Redirect to dashboard using Gin's redirect method
	log.Printf("User %s logged in successfully", userInfo.Username)
	c.Redirect(http.StatusTemporaryRedirect, "/dashboard")
}

type oidcClaims struct {
	Email    string   `json:"email"`
	Username string   `json:"profile"`
	Groups   []string `json:"groups"`
}

// ValidateIDToken verifies the id token from the oauth2token
func (a *AuthHandler) validateAndGetClaimsIDToken(
	c *gin.Context, oauth2Token *oauth2.Token) (*oidcClaims, error) {
	// Get and validate the ID token - this proves the user's identity
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return nil, errors.New("No ID token found")
	}
	// Verify the ID token
	idToken, err := a.authClient.OIDC.Verify(c.Request.Context(), rawIDToken)
	if err != nil {
		return nil, errors.New("Failed to verify ID token")
	}
	claims := oidcClaims{}
	log.Println("IdToken: ", idToken)
	if err := idToken.Claims(&claims); err != nil {
		return nil, errors.New("Failed to get user info")
	}
	return &claims, nil
}

func (a *AuthHandler) tokenExchange(c *gin.Context) (*oauth2.Token, error) {
	authorizationCode := c.Query("code")
	if authorizationCode == "" {
		return nil, errors.New("authorizationCode is required")
	}
	opts := []oauth2.AuthCodeOption{
		oauth2.SetAuthURLParam("grant_type", "authorization_code"),
	}
	oauth2Token, err := a.authClient.Oauth.Exchange(c, authorizationCode, opts...)
	if err != nil {
		return nil, err
	}
	return oauth2Token, nil
}

func (a *AuthHandler) validateStateSession(c *gin.Context) error {
	// Get state from callback parameters
	stateParam := c.Query("state")
	if stateParam == "" {
		return errors.New("missing state parameter in callback")
	}

	// Retrieve stored state from Redis
	storedState, err := a.authStore.GetState(c, stateParam)
	if err != nil {
		return fmt.Errorf("failed to retrieve stored state: %w", err)
	}

	// Validate state match
	if storedState != stateParam {
		return errors.New("state parameter mismatch")
	}

	// Clean up used state from store
	if err = a.authStore.DeleteState(c, storedState); err != nil {
		log.Printf("Warning: failed to delete used state: %v", err)
	}

	return nil
}
