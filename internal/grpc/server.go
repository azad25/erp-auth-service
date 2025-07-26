package grpc

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"erp-auth-service/internal/config"
	"erp-auth-service/internal/middleware"
	"erp-auth-service/internal/models"
	pb "erp-auth-service/proto/auth"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

type AuthGRPCServer struct {
	pb.UnimplementedAuthServiceServer
	db          *gorm.DB
	redisClient *redis.Client
	config      *config.Config
}

func NewAuthGRPCServer(db *gorm.DB, redisClient *redis.Client, config *config.Config) *AuthGRPCServer {
	return &AuthGRPCServer{
		db:          db,
		redisClient: redisClient,
		config:      config,
	}
}

func (s *AuthGRPCServer) Start(port string) error {
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return fmt.Errorf("failed to listen on port %s: %w", port, err)
	}

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(s.loggingInterceptor),
	)

	pb.RegisterAuthServiceServer(grpcServer, s)

	log.Printf("gRPC server starting on port %s", port)
	return grpcServer.Serve(lis)
}

func (s *AuthGRPCServer) loggingInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	start := time.Now()
	resp, err := handler(ctx, req)
	duration := time.Since(start)

	log.Printf("gRPC call: %s, duration: %v, error: %v", info.FullMethod, duration, err)
	return resp, err
}

func (s *AuthGRPCServer) ValidateToken(ctx context.Context, req *pb.ValidateTokenRequest) (*pb.ValidateTokenResponse, error) {
	if req.Token == "" {
		return &pb.ValidateTokenResponse{
			Valid: false,
			Error: "Token is required",
		}, nil
	}

	// Parse and validate JWT token
	claims := &middleware.Claims{}
	token, err := jwt.ParseWithClaims(req.Token, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(s.config.JWT.Secret), nil
	})

	if err != nil || !token.Valid {
		return &pb.ValidateTokenResponse{
			Valid: false,
			Error: "Invalid token",
		}, nil
	}

	// Check if token is blacklisted
	if s.redisClient.Get(ctx, "blacklist:"+req.Token).Err() == nil {
		return &pb.ValidateTokenResponse{
			Valid: false,
			Error: "Token is blacklisted",
		}, nil
	}

	// Verify user still exists and is active
	var user models.User
	if err := s.db.Where("id = ? AND is_active = true", claims.UserID).First(&user).Error; err != nil {
		return &pb.ValidateTokenResponse{
			Valid: false,
			Error: "User not found or inactive",
		}, nil
	}

	return &pb.ValidateTokenResponse{
		Valid:          true,
		UserId:         claims.UserID.String(),
		OrganizationId: claims.OrganizationID.String(),
		Email:          claims.Email,
		ExpiresAt:      timestamppb.New(claims.ExpiresAt.Time),
	}, nil
}

func (s *AuthGRPCServer) RefreshToken(ctx context.Context, req *pb.RefreshTokenRequest) (*pb.RefreshTokenResponse, error) {
	if req.RefreshToken == "" {
		return &pb.RefreshTokenResponse{
			Error: "Refresh token is required",
		}, nil
	}

	// Parse refresh token
	claims := &middleware.Claims{}
	token, err := jwt.ParseWithClaims(req.RefreshToken, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(s.config.JWT.Secret), nil
	})

	if err != nil || !token.Valid {
		return &pb.RefreshTokenResponse{
			Error: "Invalid refresh token",
		}, nil
	}

	// Check if refresh token is blacklisted
	if s.redisClient.Get(ctx, "blacklist:"+req.RefreshToken).Err() == nil {
		return &pb.RefreshTokenResponse{
			Error: "Refresh token is blacklisted",
		}, nil
	}

	// Find user
	var user models.User
	if err := s.db.Where("id = ? AND is_active = true", claims.UserID).First(&user).Error; err != nil {
		return &pb.RefreshTokenResponse{
			Error: "User not found or inactive",
		}, nil
	}

	// Generate new tokens
	accessToken, newRefreshToken, err := s.generateTokens(user)
	if err != nil {
		return &pb.RefreshTokenResponse{
			Error: "Failed to generate tokens",
		}, nil
	}

	// Blacklist old refresh token
	s.redisClient.Set(ctx, "blacklist:"+req.RefreshToken, "1", time.Duration(s.config.JWT.RefreshExpiry)*time.Second)

	return &pb.RefreshTokenResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		ExpiresIn:    int32(s.config.JWT.AccessExpiry),
	}, nil
}

func (s *AuthGRPCServer) RevokeToken(ctx context.Context, req *pb.RevokeTokenRequest) (*pb.RevokeTokenResponse, error) {
	if req.Token == "" {
		return &pb.RevokeTokenResponse{
			Success: false,
			Error:   "Token is required",
		}, nil
	}

	// Add token to blacklist
	var expiry time.Duration
	if req.TokenType == "refresh" {
		expiry = time.Duration(s.config.JWT.RefreshExpiry) * time.Second
	} else {
		expiry = time.Duration(s.config.JWT.AccessExpiry) * time.Second
	}

	err := s.redisClient.Set(ctx, "blacklist:"+req.Token, "1", expiry).Err()
	if err != nil {
		return &pb.RevokeTokenResponse{
			Success: false,
			Error:   "Failed to revoke token",
		}, nil
	}

	return &pb.RevokeTokenResponse{
		Success: true,
	}, nil
}

func (s *AuthGRPCServer) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.GetUserResponse, error) {
	if req.UserId == "" {
		return &pb.GetUserResponse{
			Error: "User ID is required",
		}, nil
	}

	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return &pb.GetUserResponse{
			Error: "Invalid user ID format",
		}, nil
	}

	var user models.User
	if err := s.db.Preload("Organization").Where("id = ?", userID).First(&user).Error; err != nil {
		return &pb.GetUserResponse{
			Error: "User not found",
		}, nil
	}

	pbUser := &pb.User{
		Id:             user.ID.String(),
		OrganizationId: user.OrganizationID.String(),
		Email:          user.Email,
		FirstName:      user.FirstName,
		LastName:       user.LastName,
		IsActive:       user.IsActive,
		IsVerified:     user.IsVerified,
		CreatedAt:      timestamppb.New(user.CreatedAt),
		UpdatedAt:      timestamppb.New(user.UpdatedAt),
	}

	if user.LastLoginAt != nil {
		pbUser.LastLoginAt = timestamppb.New(*user.LastLoginAt)
	}

	if user.Organization.ID != uuid.Nil {
		pbUser.Organization = &pb.Organization{
			Id:        user.Organization.ID.String(),
			Name:      user.Organization.Name,
			Domain:    user.Organization.Domain,
			IsActive:  user.Organization.IsActive,
			CreatedAt: timestamppb.New(user.Organization.CreatedAt),
			UpdatedAt: timestamppb.New(user.Organization.UpdatedAt),
		}
	}

	return &pb.GetUserResponse{
		User: pbUser,
	}, nil
}

func (s *AuthGRPCServer) CheckPermission(ctx context.Context, req *pb.CheckPermissionRequest) (*pb.CheckPermissionResponse, error) {
	if req.UserId == "" || req.Resource == "" || req.Action == "" {
		return &pb.CheckPermissionResponse{
			HasPermission: false,
			Error:         "User ID, resource, and action are required",
		}, nil
	}

	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return &pb.CheckPermissionResponse{
			HasPermission: false,
			Error:         "Invalid user ID format",
		}, nil
	}

	// Get user roles with permissions
	var userRoles []models.UserRole
	if err := s.db.Preload("Role.RolePermissions.Permission").Where("user_id = ?", userID).Find(&userRoles).Error; err != nil {
		return &pb.CheckPermissionResponse{
			HasPermission: false,
			Error:         "Failed to fetch user roles",
		}, nil
	}

	// Check if user has the required permission
	hasPermission := false
	for _, userRole := range userRoles {
		if !userRole.Role.IsActive {
			continue
		}
		for _, rolePermission := range userRole.Role.RolePermissions {
			if rolePermission.Permission.Resource == req.Resource && rolePermission.Permission.Action == req.Action {
				hasPermission = true
				break
			}
		}
		if hasPermission {
			break
		}
	}

	return &pb.CheckPermissionResponse{
		HasPermission: hasPermission,
	}, nil
}

func (s *AuthGRPCServer) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	return &pb.HealthCheckResponse{
		Status:    "healthy",
		Service:   "auth-service",
		Timestamp: timestamppb.New(time.Now()),
	}, nil
}

func (s *AuthGRPCServer) generateTokens(user models.User) (string, string, error) {
	// Access token claims
	accessClaims := middleware.Claims{
		UserID:         user.ID,
		OrganizationID: user.OrganizationID,
		Email:          user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(s.config.JWT.AccessExpiry) * time.Second)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   user.ID.String(),
		},
	}

	// Generate access token
	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessTokenString, err := accessToken.SignedString([]byte(s.config.JWT.Secret))
	if err != nil {
		return "", "", err
	}

	// Refresh token claims
	refreshClaims := middleware.Claims{
		UserID:         user.ID,
		OrganizationID: user.OrganizationID,
		Email:          user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(s.config.JWT.RefreshExpiry) * time.Second)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   user.ID.String(),
		},
	}

	// Generate refresh token
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshTokenString, err := refreshToken.SignedString([]byte(s.config.JWT.Secret))
	if err != nil {
		return "", "", err
	}

	return accessTokenString, refreshTokenString, nil
}