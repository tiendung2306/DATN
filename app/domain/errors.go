package domain

import "errors"

var (
	ErrNotFound       = errors.New("domain: not found")
	ErrUnauthorized   = errors.New("domain: unauthorized")
	ErrInvalidInput   = errors.New("domain: invalid input")
	ErrConflict       = errors.New("domain: conflict")
	ErrCryptoRequired = errors.New("domain: crypto engine required")
)
