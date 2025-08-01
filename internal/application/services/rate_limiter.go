package services

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go.uber.org/zap"

	"erp-auth-service/internal/cache"
	"erp-auth-service/internal/config"
)

// RateLimiter implements rate limiting and brute force protection
type RateLimiter struct {
	config       *config.Config
	logger       *zap.Logger
	cacheManager cache.CacheManager
	
	// Rate limiting configuration
	maxAttemptsPerEmail    int
	maxAttemptsPerIP       int
	emailWindowDuration    time.Duration
	ipWindowDuration       time.Duration
	lockoutDuration        time.Duration
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	MaxAttemptsPerEmail int
	MaxAttemptsPerIP    int
	EmailWindowMinutes  int
	IPWindowMinutes     int
	LockoutMinutes      int
}

// NewRateLimiter creates a new rate limiter instance
func NewRateLimiter(config *config.Config, logger *zap.Logger, cacheManager cache.CacheManager) *RateLimiter {
	// Default configuration - should be configurable
	return &RateLimiter{
		config:                 config,
		logger:                logger.With(zap.String("component", "rate_limiter")),
		cacheManager:          cacheManager,
		maxAttemptsPerEmail:   5,  // 5 attempts per email
		maxAttemptsPerIP:      20, // 20 attempts per IP
		emailWindowDuration:   15 * time.Minute,
		ipWindowDuration:      15 * time.Minute,
		lockoutDuration:       15 * time.Minute,
	}
}

// IsBlocked checks if an email or IP address is currently blocked
func (rl *RateLimiter) IsBlocked(ctx context.Context, email, ipAddress string) (bool, error) {
	// Check email-based blocking
	emailBlocked, err := rl.isEmailBlocked(ctx, email)
	if err != nil {
		return false, fmt.Errorf("failed to check email blocking: %w", err)
	}
	
	if emailBlocked {
		rl.logger.Debug("Email is blocked", zap.String("email", email))
		return true, nil
	}
	
	// Check IP-based blocking
	ipBlocked, err := rl.isIPBlocked(ctx, ipAddress)
	if err != nil {
		return false, fmt.Errorf("failed to check IP blocking: %w", err)
	}
	
	if ipBlocked {
		rl.logger.Debug("IP is blocked", zap.String("ip", ipAddress))
		return true, nil
	}
	
	return false, nil
}

// RecordAttempt records an authentication attempt
func (rl *RateLimiter) RecordAttempt(ctx context.Context, email, ipAddress string, success bool) error {
	now := time.Now()
	
	// Record email attempt
	if err := rl.recordEmailAttempt(ctx, email, success, now); err != nil {
		rl.logger.Error("Failed to record email attempt", zap.Error(err))
		return fmt.Errorf("failed to record email attempt: %w", err)
	}
	
	// Record IP attempt
	if err := rl.recordIPAttempt(ctx, ipAddress, success, now); err != nil {
		rl.logger.Error("Failed to record IP attempt", zap.Error(err))
		return fmt.Errorf("failed to record IP attempt: %w", err)
	}
	
	return nil
}

// ClearAttempts clears all recorded attempts for an email (used after successful login)
func (rl *RateLimiter) ClearAttempts(ctx context.Context, email string) error {
	emailKey := rl.getEmailAttemptsKey(email)
	
	if err := rl.cacheManager.Delete(ctx, emailKey); err != nil {
		return fmt.Errorf("failed to clear email attempts: %w", err)
	}
	
	rl.logger.Debug("Cleared attempts for email", zap.String("email", email))
	return nil
}

// isEmailBlocked checks if an email is currently blocked
func (rl *RateLimiter) isEmailBlocked(ctx context.Context, email string) (bool, error) {
	attemptsKey := rl.getEmailAttemptsKey(email)
	lockKey := rl.getEmailLockKey(email)
	
	// Check if explicitly locked
	if exists, err := rl.cacheManager.Exists(ctx, lockKey); err != nil {
		return false, err
	} else if exists {
		return true, nil
	}
	
	// Check attempt count
	attempts, err := rl.getAttemptCount(ctx, attemptsKey)
	if err != nil {
		return false, err
	}
	
	if attempts >= rl.maxAttemptsPerEmail {
		// Lock the email
		if err := rl.lockEmail(ctx, email); err != nil {
			rl.logger.Error("Failed to lock email", zap.Error(err))
		}
		return true, nil
	}
	
	return false, nil
}

// isIPBlocked checks if an IP address is currently blocked
func (rl *RateLimiter) isIPBlocked(ctx context.Context, ipAddress string) (bool, error) {
	attemptsKey := rl.getIPAttemptsKey(ipAddress)
	lockKey := rl.getIPLockKey(ipAddress)
	
	// Check if explicitly locked
	if exists, err := rl.cacheManager.Exists(ctx, lockKey); err != nil {
		return false, err
	} else if exists {
		return true, nil
	}
	
	// Check attempt count
	attempts, err := rl.getAttemptCount(ctx, attemptsKey)
	if err != nil {
		return false, err
	}
	
	if attempts >= rl.maxAttemptsPerIP {
		// Lock the IP
		if err := rl.lockIP(ctx, ipAddress); err != nil {
			rl.logger.Error("Failed to lock IP", zap.Error(err))
		}
		return true, nil
	}
	
	return false, nil
}

// recordEmailAttempt records an authentication attempt for an email
func (rl *RateLimiter) recordEmailAttempt(ctx context.Context, email string, success bool, timestamp time.Time) error {
	if success {
		// Clear attempts on successful login
		return rl.ClearAttempts(ctx, email)
	}
	
	attemptsKey := rl.getEmailAttemptsKey(email)
	
	// Increment attempt counter
	if err := rl.incrementAttempts(ctx, attemptsKey, rl.emailWindowDuration); err != nil {
		return err
	}
	
	rl.logger.Debug("Recorded failed email attempt", 
		zap.String("email", email),
		zap.Time("timestamp", timestamp))
	
	return nil
}

// recordIPAttempt records an authentication attempt for an IP address
func (rl *RateLimiter) recordIPAttempt(ctx context.Context, ipAddress string, success bool, timestamp time.Time) error {
	attemptsKey := rl.getIPAttemptsKey(ipAddress)
	
	// Always record IP attempts (even successful ones for monitoring)
	if err := rl.incrementAttempts(ctx, attemptsKey, rl.ipWindowDuration); err != nil {
		return err
	}
	
	rl.logger.Debug("Recorded IP attempt", 
		zap.String("ip", ipAddress),
		zap.Bool("success", success),
		zap.Time("timestamp", timestamp))
	
	return nil
}

// lockEmail locks an email address for the configured duration
func (rl *RateLimiter) lockEmail(ctx context.Context, email string) error {
	lockKey := rl.getEmailLockKey(email)
	
	if err := rl.cacheManager.Set(ctx, lockKey, []byte("locked"), rl.lockoutDuration); err != nil {
		return fmt.Errorf("failed to lock email: %w", err)
	}
	
	rl.logger.Info("Email locked due to too many failed attempts", 
		zap.String("email", email),
		zap.Duration("duration", rl.lockoutDuration))
	
	return nil
}

// lockIP locks an IP address for the configured duration
func (rl *RateLimiter) lockIP(ctx context.Context, ipAddress string) error {
	lockKey := rl.getIPLockKey(ipAddress)
	
	if err := rl.cacheManager.Set(ctx, lockKey, []byte("locked"), rl.lockoutDuration); err != nil {
		return fmt.Errorf("failed to lock IP: %w", err)
	}
	
	rl.logger.Info("IP locked due to too many failed attempts", 
		zap.String("ip", ipAddress),
		zap.Duration("duration", rl.lockoutDuration))
	
	return nil
}

// getAttemptCount gets the current attempt count for a key
func (rl *RateLimiter) getAttemptCount(ctx context.Context, key string) (int, error) {
	data, err := rl.cacheManager.Get(ctx, key)
	if err != nil {
		// Key doesn't exist, return 0
		return 0, nil
	}
	
	count, err := strconv.Atoi(string(data))
	if err != nil {
		rl.logger.Error("Failed to parse attempt count", zap.Error(err))
		return 0, nil
	}
	
	return count, nil
}

// incrementAttempts increments the attempt counter for a key
func (rl *RateLimiter) incrementAttempts(ctx context.Context, key string, ttl time.Duration) error {
	// Get current count
	currentCount, err := rl.getAttemptCount(ctx, key)
	if err != nil {
		return err
	}
	
	// Increment and store
	newCount := currentCount + 1
	countData := []byte(strconv.Itoa(newCount))
	
	if err := rl.cacheManager.Set(ctx, key, countData, ttl); err != nil {
		return fmt.Errorf("failed to increment attempts: %w", err)
	}
	
	return nil
}

// Key generation methods

func (rl *RateLimiter) getEmailAttemptsKey(email string) string {
	return fmt.Sprintf("ratelimit:email:attempts:%s", email)
}

func (rl *RateLimiter) getEmailLockKey(email string) string {
	return fmt.Sprintf("ratelimit:email:lock:%s", email)
}

func (rl *RateLimiter) getIPAttemptsKey(ipAddress string) string {
	return fmt.Sprintf("ratelimit:ip:attempts:%s", ipAddress)
}

func (rl *RateLimiter) getIPLockKey(ipAddress string) string {
	return fmt.Sprintf("ratelimit:ip:lock:%s", ipAddress)
}

// GetStats returns rate limiting statistics
func (rl *RateLimiter) GetStats(ctx context.Context) (map[string]interface{}, error) {
	// This would return statistics about rate limiting
	// Implementation depends on requirements
	return map[string]interface{}{
		"max_attempts_per_email": rl.maxAttemptsPerEmail,
		"max_attempts_per_ip":    rl.maxAttemptsPerIP,
		"email_window_minutes":   rl.emailWindowDuration.Minutes(),
		"ip_window_minutes":      rl.ipWindowDuration.Minutes(),
		"lockout_minutes":        rl.lockoutDuration.Minutes(),
	}, nil
}

// UpdateConfig updates the rate limiter configuration
func (rl *RateLimiter) UpdateConfig(config RateLimitConfig) {
	rl.maxAttemptsPerEmail = config.MaxAttemptsPerEmail
	rl.maxAttemptsPerIP = config.MaxAttemptsPerIP
	rl.emailWindowDuration = time.Duration(config.EmailWindowMinutes) * time.Minute
	rl.ipWindowDuration = time.Duration(config.IPWindowMinutes) * time.Minute
	rl.lockoutDuration = time.Duration(config.LockoutMinutes) * time.Minute
	
	rl.logger.Info("Rate limiter configuration updated", 
		zap.Int("max_email_attempts", rl.maxAttemptsPerEmail),
		zap.Int("max_ip_attempts", rl.maxAttemptsPerIP))
}