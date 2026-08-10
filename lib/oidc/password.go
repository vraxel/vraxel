package oidc

import "golang.org/x/crypto/bcrypt"

// PasswordService handles password hashing and verification.
type PasswordService struct {
	cost int
}

// NewPasswordService creates a new password service with the given bcrypt cost.
func NewPasswordService(cost int) *PasswordService {
	if cost == 0 {
		cost = 10
	}
	return &PasswordService{cost: cost}
}

// Hash generates a bcrypt hash of the password.
func (ps *PasswordService) Hash(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), ps.cost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// Verify compares a password against a bcrypt hash.
func (ps *PasswordService) Verify(password, hash string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
