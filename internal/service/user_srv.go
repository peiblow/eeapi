package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/peiblow/eeapi/internal/database/postgres"
	"github.com/peiblow/eeapi/internal/repository"
	"github.com/peiblow/eeapi/internal/schema"
	"golang.org/x/crypto/bcrypt"
)

var ErrInvalidCredentials = errors.New("invalid credentials")

type UserService struct {
	db repository.UserRepository
}

func NewUserService(db *postgres.DB) *UserService {
	return &UserService{
		db: repository.NewPsqlUserRepository(db),
	}
}

func (s *UserService) Authenticate(ctx context.Context, email, password string) (*schema.User, error) {
	user, err := s.db.GetUserByEmail(email)
	if err != nil {
		return nil, err
	}

	if err := checkPassword(password, user.Password); err != nil {
		return nil, ErrInvalidCredentials
	}

	return user, nil
}

func (s *UserService) Register(ctx context.Context, email, password string) (*schema.User, error) {
	if _, err := s.db.GetUserByEmail(email); err == nil {
		return nil, errors.New("email already in use")
	}

	hash, err := hashPassword(password)
	if err != nil {
		return nil, err
	}

	user := &schema.User{
		ID:        uuid.New().String(),
		Email:     email,
		Password:  hash,
		CreatedAt: time.Now().UnixMilli(),
	}

	slog.Info("Registering new user", "email", email)
	return user, s.db.CreateUser(ctx, user)
}

func checkPassword(provided, stored string) error {
	return bcrypt.CompareHashAndPassword([]byte(stored), []byte(provided))
}

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}
