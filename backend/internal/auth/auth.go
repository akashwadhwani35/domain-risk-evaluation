package auth

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Constants for authentication configuration
const (
	OTPExpiry         = 5 * time.Minute
	JWTExpiry         = 24 * time.Hour
	JWTRefreshWindow  = 4 * time.Hour
	RateLimitWindow   = 15 * time.Minute
	RateLimitMax      = 3
	MaxOTPAttempts    = 3
	AllowedDomain     = "@radix.email"
)

var (
	ErrInvalidEmail     = errors.New("email must end with @radix.email")
	ErrRateLimitExceeded = errors.New("too many OTP requests, please wait 15 minutes")
	ErrInvalidOTP       = errors.New("invalid or expired OTP code")
	ErrMaxAttempts      = errors.New("maximum verification attempts exceeded")
	ErrInvalidToken     = errors.New("invalid or expired token")
)

// Claims represents JWT token claims
type Claims struct {
	UserID uint   `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

// Service provides authentication operations
type Service struct {
	jwtSecret []byte
}

// NewService creates a new auth service
func NewService(jwtSecret string) (*Service, error) {
	if len(jwtSecret) < 32 {
		return nil, errors.New("JWT_SECRET must be at least 32 characters")
	}
	return &Service{
		jwtSecret: []byte(jwtSecret),
	}, nil
}

// ValidateEmail checks if the email ends with the default @radix.email domain.
func ValidateEmail(email string) error {
	return ValidateEmailFor(email, AllowedDomain)
}

// ValidateEmailFor checks an email against a configurable allowed domain.
// An empty allowedDomain or "*" disables the restriction and accepts any
// syntactically non-empty email address. A leading "@" on allowedDomain is
// optional (both "radix.email" and "@radix.email" work).
func ValidateEmailFor(email, allowedDomain string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return errors.New("email is required")
	}

	allowedDomain = strings.ToLower(strings.TrimSpace(allowedDomain))
	if allowedDomain == "" || allowedDomain == "*" {
		// No domain restriction: accept any address that looks like an email.
		if !strings.Contains(email, "@") || strings.HasPrefix(email, "@") || strings.HasSuffix(email, "@") {
			return errors.New("a valid email address is required")
		}
		return nil
	}

	if !strings.HasPrefix(allowedDomain, "@") {
		allowedDomain = "@" + allowedDomain
	}
	if !strings.HasSuffix(email, allowedDomain) {
		return fmt.Errorf("email must end with %s", allowedDomain)
	}
	return nil
}

// GenerateOTP creates a cryptographically secure 6-digit OTP code
func GenerateOTP() (string, error) {
	// Generate a 6-digit code (100000-999999)
	max := big.NewInt(900000)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", fmt.Errorf("generate OTP: %w", err)
	}
	code := n.Int64() + 100000
	return fmt.Sprintf("%06d", code), nil
}

// GenerateJWT creates a new JWT token for the user
func (s *Service) GenerateJWT(userID uint, email string) (string, time.Time, error) {
	expiresAt := time.Now().Add(JWTExpiry)
	claims := Claims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "domain-risk-eval",
			Subject:   fmt.Sprintf("%d", userID),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign token: %w", err)
	}

	return tokenString, expiresAt, nil
}

// ValidateJWT validates a JWT token and returns the claims
func (s *Service) ValidateJWT(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.jwtSecret, nil
	})

	if err != nil {
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// ShouldRefreshToken checks if the token should be refreshed (less than 4 hours remaining)
func (s *Service) ShouldRefreshToken(claims *Claims) bool {
	if claims.ExpiresAt == nil {
		return false
	}
	return time.Until(claims.ExpiresAt.Time) < JWTRefreshWindow
}

// OTPExpiryTime returns the expiry time for a new OTP
func OTPExpiryTime() time.Time {
	return time.Now().Add(OTPExpiry)
}

// RateLimitTime returns the time window for rate limiting
func RateLimitTime() time.Time {
	return time.Now().Add(-RateLimitWindow)
}
