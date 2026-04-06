package model

import "errors"

var (
	ErrNotFound        = errors.New("resource not found")
	ErrAlreadyExists   = errors.New("resource already exists")
	ErrConflict        = errors.New("resource conflict")
	ErrAccountDisabled    = errors.New("account is disabled")
	ErrCannotDisableAdmin = errors.New("cannot disable admin account")
)
