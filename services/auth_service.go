package services

import (
	"errors"
	"strings"
	"time"

	"navmesh-go/domains"

	"github.com/google/uuid"
	commonConfigs "github.com/wfu-work/nav-common-go-lib/configs"
	"github.com/wfu-work/nav-common-go-lib/global"
	commonUtils "github.com/wfu-work/nav-common-go-lib/utils"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type AuthService struct{}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type ChangePasswordRequest struct {
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
}

type LoginResult struct {
	Token     string       `json:"token"`
	ExpiresAt int64        `json:"expiresAt"`
	User      domains.User `json:"user"`
}

func (s AuthService) EnsureDefaultAdmin(username, password string) error {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	if username == "" {
		username = "admin"
	}
	if password == "" {
		password = "navmesh@2020"
	}
	var count int64
	if err := global.NAV_DB.Model(&domains.User{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	now := domains.NowMilli()
	return global.NAV_DB.Create(&domains.User{
		Username:     username,
		PasswordHash: string(hash),
		Status:       int(domains.StatusEnabled),
		CreateTime:   now,
		UpdateTime:   now,
	}).Error
}

func (s AuthService) Login(req LoginRequest) (*LoginResult, error) {
	username := strings.TrimSpace(req.Username)
	password := strings.TrimSpace(req.Password)
	if username == "" || password == "" {
		return nil, errors.New("username and password required")
	}
	var user domains.User
	if err := global.NAV_DB.Where("username = ? AND status = ?", username, int(domains.StatusEnabled)).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invalid username or password")
		}
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, errors.New("invalid username or password")
	}
	j := commonUtils.NewJWT()
	claims := j.CreateClaims(commonConfigs.BaseClaims{
		UserGuid:  uuid.NewString(),
		Username:  user.Username,
		RoleCodes: "SUPER_ADMIN",
	})
	token, err := j.CreateToken(claims)
	if err != nil {
		return nil, err
	}
	expiresAt := time.Now().Add(24 * time.Hour).Unix()
	if claims.ExpiresAt != nil {
		expiresAt = claims.ExpiresAt.Unix()
	}
	return &LoginResult{Token: token, ExpiresAt: expiresAt, User: user}, nil
}

func (s AuthService) Profile(username string) (*domains.User, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, errors.New("username required")
	}
	var user domains.User
	if err := global.NAV_DB.Where("username = ?", username).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (s AuthService) ChangePassword(username string, req ChangePasswordRequest) error {
	username = strings.TrimSpace(username)
	oldPassword := strings.TrimSpace(req.OldPassword)
	newPassword := strings.TrimSpace(req.NewPassword)
	if username == "" {
		return errors.New("username required")
	}
	if oldPassword == "" || newPassword == "" {
		return errors.New("oldPassword and newPassword required")
	}
	if len(newPassword) < 8 {
		return errors.New("newPassword must be at least 8 characters")
	}
	var user domains.User
	if err := global.NAV_DB.Where("username = ? AND status = ?", username, int(domains.StatusEnabled)).First(&user).Error; err != nil {
		return err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(oldPassword)); err != nil {
		return errors.New("invalid oldPassword")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return global.NAV_DB.Model(&domains.User{}).Where("id = ?", user.ID).Updates(map[string]any{
		"password_hash": string(hash),
		"update_time":   domains.NowMilli(),
	}).Error
}
