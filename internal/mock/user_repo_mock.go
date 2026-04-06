package mock

import (
	"context"
	"fmt"

	"github.com/steven/vaultflix/internal/model"
)

type UserRepository struct {
	CreateFunc         func(ctx context.Context, user *model.User) error
	GetByUsernameFunc  func(ctx context.Context, username string) (*model.User, error)
	GetByIDFunc        func(ctx context.Context, id string) (*model.User, error)
	CountUsersFunc     func(ctx context.Context) (int64, error)
	ListFunc           func(ctx context.Context) ([]model.User, error)
	DisableUserFunc    func(ctx context.Context, id string) error
	EnableUserFunc     func(ctx context.Context, id string) error
	UpdatePasswordFunc func(ctx context.Context, id string, passwordHash string) error
}

func (m *UserRepository) Create(ctx context.Context, user *model.User) error {
	if m.CreateFunc == nil {
		return fmt.Errorf("mock: CreateFunc not set")
	}
	return m.CreateFunc(ctx, user)
}

func (m *UserRepository) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	if m.GetByUsernameFunc == nil {
		return nil, fmt.Errorf("mock: GetByUsernameFunc not set")
	}
	return m.GetByUsernameFunc(ctx, username)
}

func (m *UserRepository) GetByID(ctx context.Context, id string) (*model.User, error) {
	if m.GetByIDFunc == nil {
		return nil, fmt.Errorf("mock: GetByIDFunc not set")
	}
	return m.GetByIDFunc(ctx, id)
}

func (m *UserRepository) CountUsers(ctx context.Context) (int64, error) {
	if m.CountUsersFunc == nil {
		return 0, fmt.Errorf("mock: CountUsersFunc not set")
	}
	return m.CountUsersFunc(ctx)
}

func (m *UserRepository) List(ctx context.Context) ([]model.User, error) {
	if m.ListFunc == nil {
		return nil, fmt.Errorf("mock: ListFunc not set")
	}
	return m.ListFunc(ctx)
}

func (m *UserRepository) DisableUser(ctx context.Context, id string) error {
	if m.DisableUserFunc == nil {
		return fmt.Errorf("mock: DisableUserFunc not set")
	}
	return m.DisableUserFunc(ctx, id)
}

func (m *UserRepository) EnableUser(ctx context.Context, id string) error {
	if m.EnableUserFunc == nil {
		return fmt.Errorf("mock: EnableUserFunc not set")
	}
	return m.EnableUserFunc(ctx, id)
}

func (m *UserRepository) UpdatePassword(ctx context.Context, id string, passwordHash string) error {
	if m.UpdatePasswordFunc == nil {
		return fmt.Errorf("mock: UpdatePasswordFunc not set")
	}
	return m.UpdatePasswordFunc(ctx, id, passwordHash)
}
