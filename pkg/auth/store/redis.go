package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// SessionData represents the data we'll store for each session
type SessionData struct {
	AccessToken string    `json:"access_token"`
	UserInfo    UserInfo  `json:"user_info"`
	CreatedAt   time.Time `json:"created_at"`
}

// UserInfo contains the essential user information we want to cache
type UserInfo struct {
	Username string   `json:"username"`
	Email    string   `json:"email"`
	Groups   []string `json:"groups"`
	// Add other user fields you need
}

// SessionStore defines the contract for session management
type SessionStore interface {
	Set(ctx context.Context, sessionID string, data SessionData) error
	Get(ctx context.Context, sessionID string) (*SessionData, error)
	Delete(ctx context.Context, sessionID string) error
}

type RedisSessionManager struct {
	client      *redis.Client
	PrefixState string
	defaultTTL  time.Duration
}

type AuthData struct {
}

type AuthStore interface {
	SetState(ctx context.Context, state string) error
	GetState(
		ctx context.Context,
		state string,
	) (string, error)
	DeleteState(
		ctx context.Context,
		state string,
	) error
}

type RedisAuthManager struct {
	client      *redis.Client
	PrefixState string
	defaultTTL  time.Duration
}

func NewAuthRedisManager(rds *redis.Client) *RedisAuthManager {
	return &RedisAuthManager{
		client:      rds,
		PrefixState: "stateauth",
		defaultTTL:  2 * time.Minute,
	}
}

func NewSessionRedisManager(rds *redis.Client) *RedisSessionManager {
	return &RedisSessionManager{
		client:      rds,
		PrefixState: "session",
		defaultTTL:  5 * time.Minute,
	}
}
func (r *RedisSessionManager) buildKeyState(session string) string {
	return fmt.Sprintf("%s:%s", r.PrefixState, session)
}

func (r *RedisAuthManager) buildKeyState(state string) string {
	return fmt.Sprintf("%s:%s", r.PrefixState, state)
}

// Set stores session data in Redis
func (r *RedisSessionManager) Set(ctx context.Context, sessionID string, data SessionData) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal session data: %w", err)
	}

	key := r.buildKeyState(sessionID)
	return r.client.Set(ctx, key, jsonData, r.defaultTTL).Err()
}

// Get retrieves session data from Redis
func (r *RedisSessionManager) Get(ctx context.Context, sessionID string) (*SessionData, error) {
	key := r.buildKeyState(sessionID)
	data, err := r.client.Get(ctx, key).Result()

	if err == redis.Nil {
		return nil, fmt.Errorf("session not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	var sessionData SessionData
	if err := json.Unmarshal([]byte(data), &sessionData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session data: %w", err)
	}

	return &sessionData, nil
}

// Delete removes a session from Redis
func (r *RedisSessionManager) Delete(ctx context.Context, sessionID string) error {
	key := r.buildKeyState(sessionID)
	return r.client.Del(ctx, key).Err()
}

func (r *RedisAuthManager) SetState(ctx context.Context, state string) error {
	key := r.buildKeyState(state)
	expiration := r.defaultTTL

	err := r.client.Set(ctx, key, state, expiration).Err()
	if err != nil {
		return fmt.Errorf("failed to set session in Redis: %w", err)
	}
	return nil
}

func (r *RedisAuthManager) GetState(
	ctx context.Context,
	state string,
) (string, error) {
	key := r.buildKeyState(state)
	stateData, err := r.client.Get(ctx, key).Result()
	if err != nil {
		return "", fmt.Errorf("failed to get state data from Redis: %w", err)
	}
	return stateData, nil
}

func (r *RedisAuthManager) DeleteState(
	ctx context.Context,
	state string,
) error {
	key := r.buildKeyState(state)
	if err := r.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("failed to remove state data from Redis: %w", err)
	}
	return nil
}
