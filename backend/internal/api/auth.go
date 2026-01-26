package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"domain-risk-eval/backend/internal/auth"
)

// RequestOTPRequest is the request body for requesting an OTP
type RequestOTPRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// VerifyOTPRequest is the request body for verifying an OTP
type VerifyOTPRequest struct {
	Email string `json:"email" binding:"required,email"`
	Code  string `json:"code" binding:"required,len=6"`
}

// AuthUserResponse is the response for authenticated user info
type AuthUserResponse struct {
	ID    uint   `json:"id"`
	Email string `json:"email"`
}

// handleRequestOTP handles POST /api/auth/request-otp
func (s *Server) handleRequestOTP(c *gin.Context) {
	var req RequestOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.renderError(c, http.StatusBadRequest, err)
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))

	// Validate email domain
	if err := auth.ValidateEmail(email); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Check rate limit
	recentCount, err := s.db.CountRecentOTPRequests(email, auth.RateLimitTime())
	if err != nil {
		logrus.WithError(err).Error("count recent OTP requests")
		s.renderError(c, http.StatusInternalServerError, err)
		return
	}

	if recentCount >= auth.RateLimitMax {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error": auth.ErrRateLimitExceeded.Error(),
		})
		return
	}

	// Generate OTP
	code, err := auth.GenerateOTP()
	if err != nil {
		logrus.WithError(err).Error("generate OTP")
		s.renderError(c, http.StatusInternalServerError, err)
		return
	}

	// Store OTP
	_, err = s.db.CreateOTPCode(email, code, auth.OTPExpiryTime())
	if err != nil {
		logrus.WithError(err).Error("store OTP code")
		s.renderError(c, http.StatusInternalServerError, err)
		return
	}

	// Send OTP email
	if s.emailService != nil {
		if err := s.emailService.SendOTP(email, code); err != nil {
			logrus.WithError(err).WithField("email", email).Error("send OTP email")
			// Don't expose email sending errors to the user for security
			// The OTP is still created, but the user won't receive it
		}
	} else {
		logrus.WithFields(logrus.Fields{
			"email": email,
			"code":  code,
		}).Warn("email service not configured, OTP logged for development")
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "verification code sent",
	})
}

// handleVerifyOTP handles POST /api/auth/verify-otp
func (s *Server) handleVerifyOTP(c *gin.Context) {
	var req VerifyOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.renderError(c, http.StatusBadRequest, err)
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	code := strings.TrimSpace(req.Code)

	// Validate email domain
	if err := auth.ValidateEmail(email); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Get valid OTP
	otp, err := s.db.GetValidOTPCode(email, code)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": auth.ErrInvalidOTP.Error(),
		})
		return
	}

	// Increment attempts
	if err := s.db.IncrementOTPAttempts(otp.ID); err != nil {
		logrus.WithError(err).Error("increment OTP attempts")
	}

	// Mark OTP as used
	if err := s.db.MarkOTPUsed(otp.ID); err != nil {
		logrus.WithError(err).Error("mark OTP used")
		s.renderError(c, http.StatusInternalServerError, err)
		return
	}

	// Create or get user
	user, err := s.db.CreateUser(email)
	if err != nil {
		logrus.WithError(err).Error("create user")
		s.renderError(c, http.StatusInternalServerError, err)
		return
	}

	// Update last login
	if err := s.db.UpdateUserLastLogin(user.ID); err != nil {
		logrus.WithError(err).Error("update last login")
	}

	// Generate JWT
	token, expiresAt, err := s.authService.GenerateJWT(user.ID, user.Email)
	if err != nil {
		logrus.WithError(err).Error("generate JWT")
		s.renderError(c, http.StatusInternalServerError, err)
		return
	}

	// Set cookie
	auth.SetAuthCookie(c, token, expiresAt)

	logrus.WithFields(logrus.Fields{
		"user_id": user.ID,
		"email":   user.Email,
	}).Info("user authenticated")

	c.JSON(http.StatusOK, gin.H{
		"message": "authenticated",
		"user": AuthUserResponse{
			ID:    user.ID,
			Email: user.Email,
		},
	})
}

// handleLogout handles POST /api/auth/logout
func (s *Server) handleLogout(c *gin.Context) {
	auth.ClearAuthCookie(c)

	c.JSON(http.StatusOK, gin.H{
		"message": "logged out",
	})
}

// handleGetCurrentUser handles GET /api/auth/me
func (s *Server) handleGetCurrentUser(c *gin.Context) {
	userID, ok := auth.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "not authenticated",
		})
		return
	}

	user, err := s.db.GetUserByID(userID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "user not found",
		})
		return
	}

	c.JSON(http.StatusOK, AuthUserResponse{
		ID:    user.ID,
		Email: user.Email,
	})
}
