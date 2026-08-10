package iam

// PasswordHasher is a password hashing function. REST storages accept it
// as a pluggable function so tests can stub the bcrypt cost; the default
// wiring passes iam.HashPassword.
type PasswordHasher func(password string) (string, error)
