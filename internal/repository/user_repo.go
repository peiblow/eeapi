package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/peiblow/eeapi/internal/database/postgres"
	"github.com/peiblow/eeapi/internal/schema"
)

var ErrUserNotFound = errors.New("User not found")

type UserRepository interface {
	GetUserByEmail(email string) (*schema.User, error)
	CreateUser(ctx context.Context, user *schema.User) error
}

type PsqlUserRepository struct {
	db *postgres.DB
}

func NewPsqlUserRepository(db *postgres.DB) UserRepository {
	return &PsqlUserRepository{db: db}
}

func (r *PsqlUserRepository) GetUserByEmail(email string) (*schema.User, error) {
	query := `SELECT id, username, email FROM users WHERE id = $1`
	row := r.db.QueryRow(query, email)

	var user schema.User
	if err := row.Scan(&user.ID, &user.ID, &user.Email, &user.Password, &user.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	return &user, nil
}

func (r *PsqlUserRepository) CreateUser(ctx context.Context, user *schema.User) error {
	query := `INSERT INTO users (id, email, password_hash, created_at) VALUES ($1, $2, $3, $4)`
	_, err := r.db.ExecContext(ctx, query, user.ID, user.Email, user.Password, user.CreatedAt)
	return err
}
