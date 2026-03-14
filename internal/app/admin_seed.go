package app

import (
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/simp-lee/logger"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/simp-lee/gobase/internal/config"
	"github.com/simp-lee/gobase/internal/domain"
)

const (
	seedAdminPasswordEnv    = "APP__AUTH__SEED_ADMIN_PASSWORD"
	seedAdminEmailEnv       = "APP__AUTH__SEED_ADMIN_EMAIL"
	defaultSeedAdminEmail   = "admin@example.com"
	defaultSeedAdminPassord = "password123"
)

func ensureSeedAdmin(cfg *config.Config, db *gorm.DB, log *logger.Logger) error {
	if cfg == nil || db == nil || !cfg.Auth.Enabled || cfg.Server.Mode == gin.TestMode {
		return nil
	}

	adminEmail, err := resolveSeedAdminEmail()
	if err != nil {
		return err
	}

	adminPassword, syncPassword, err := resolveSeedAdminPassword(cfg.Server.Mode)
	if err != nil {
		return err
	}

	var admin domain.User
	err = db.Where("LOWER(email) = ?", adminEmail).First(&admin).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("query seed admin: %w", err)
		}

		hash, hashErr := bcrypt.GenerateFromPassword([]byte(adminPassword), bcrypt.DefaultCost)
		if hashErr != nil {
			return fmt.Errorf("hash seed admin password: %w", hashErr)
		}

		admin = domain.User{
			Username:     "Admin",
			Email:        adminEmail,
			PasswordHash: string(hash),
			Role:         domain.RoleAdmin,
			Status:       domain.StatusActive,
		}
		if createErr := db.Create(&admin).Error; createErr != nil {
			return fmt.Errorf("create seed admin: %w", createErr)
		}
		logSeedAdmin(log, "seed admin created", slog.String("email", adminEmail))
		return nil
	}

	updates := map[string]any{}
	if admin.Email != adminEmail {
		updates["email"] = adminEmail
	}
	if admin.Role != domain.RoleAdmin {
		updates["role"] = domain.RoleAdmin
	}
	if admin.Status != domain.StatusActive {
		updates["status"] = domain.StatusActive
	}
	if syncPassword && bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(adminPassword)) != nil {
		hash, hashErr := bcrypt.GenerateFromPassword([]byte(adminPassword), bcrypt.DefaultCost)
		if hashErr != nil {
			return fmt.Errorf("hash seed admin password: %w", hashErr)
		}
		updates["password_hash"] = string(hash)
	}

	if len(updates) == 0 {
		return nil
	}

	if err := db.Model(&admin).Updates(updates).Error; err != nil {
		return fmt.Errorf("update seed admin: %w", err)
	}
	logSeedAdmin(log, "seed admin reconciled", slog.String("email", adminEmail))
	return nil
}

func resolveSeedAdminEmail() (string, error) {
	email := strings.ToLower(strings.TrimSpace(os.Getenv(seedAdminEmailEnv)))
	if email == "" {
		email = defaultSeedAdminEmail
	}

	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Name != "" || addr.Address != email {
		return "", fmt.Errorf("invalid %s: must be a bare email address", seedAdminEmailEnv)
	}

	return email, nil
}

func resolveSeedAdminPassword(mode string) (password string, syncPassword bool, err error) {
	password = strings.TrimSpace(os.Getenv(seedAdminPasswordEnv))
	if password != "" {
		return password, true, nil
	}

	if mode == gin.ReleaseMode {
		return "", false, fmt.Errorf("%s is required when auth is enabled in release mode", seedAdminPasswordEnv)
	}

	if err := os.Setenv(seedAdminPasswordEnv, defaultSeedAdminPassord); err != nil {
		return "", false, fmt.Errorf("set default %s: %w", seedAdminPasswordEnv, err)
	}

	return defaultSeedAdminPassord, true, nil
}

func logSeedAdmin(log *logger.Logger, message string, args ...any) {
	if log != nil {
		log.Info(message, args...)
		return
	}
	slog.Info(message, args...)
}
