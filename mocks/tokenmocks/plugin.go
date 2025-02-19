// Code generated by mockery v1.0.0. DO NOT EDIT.

package tokenmocks

import (
	context "context"

	config "github.com/hyperledger/firefly-common/pkg/config"

	core "github.com/hyperledger/firefly/pkg/core"

	mock "github.com/stretchr/testify/mock"

	tokens "github.com/hyperledger/firefly/pkg/tokens"
)

// Plugin is an autogenerated mock type for the Plugin type
type Plugin struct {
	mock.Mock
}

// ActivateTokenPool provides a mock function with given fields: ctx, nsOpID, pool
func (_m *Plugin) ActivateTokenPool(ctx context.Context, nsOpID string, pool *core.TokenPool) (bool, error) {
	ret := _m.Called(ctx, nsOpID, pool)

	var r0 bool
	if rf, ok := ret.Get(0).(func(context.Context, string, *core.TokenPool) bool); ok {
		r0 = rf(ctx, nsOpID, pool)
	} else {
		r0 = ret.Get(0).(bool)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, *core.TokenPool) error); ok {
		r1 = rf(ctx, nsOpID, pool)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// BurnTokens provides a mock function with given fields: ctx, nsOpID, poolLocator, burn
func (_m *Plugin) BurnTokens(ctx context.Context, nsOpID string, poolLocator string, burn *core.TokenTransfer) error {
	ret := _m.Called(ctx, nsOpID, poolLocator, burn)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string, *core.TokenTransfer) error); ok {
		r0 = rf(ctx, nsOpID, poolLocator, burn)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Capabilities provides a mock function with given fields:
func (_m *Plugin) Capabilities() *tokens.Capabilities {
	ret := _m.Called()

	var r0 *tokens.Capabilities
	if rf, ok := ret.Get(0).(func() *tokens.Capabilities); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*tokens.Capabilities)
		}
	}

	return r0
}

// CreateTokenPool provides a mock function with given fields: ctx, nsOpID, pool
func (_m *Plugin) CreateTokenPool(ctx context.Context, nsOpID string, pool *core.TokenPool) (bool, error) {
	ret := _m.Called(ctx, nsOpID, pool)

	var r0 bool
	if rf, ok := ret.Get(0).(func(context.Context, string, *core.TokenPool) bool); ok {
		r0 = rf(ctx, nsOpID, pool)
	} else {
		r0 = ret.Get(0).(bool)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, *core.TokenPool) error); ok {
		r1 = rf(ctx, nsOpID, pool)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Init provides a mock function with given fields: ctx, name, _a2
func (_m *Plugin) Init(ctx context.Context, name string, _a2 config.Section) error {
	ret := _m.Called(ctx, name, _a2)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, config.Section) error); ok {
		r0 = rf(ctx, name, _a2)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// InitConfig provides a mock function with given fields: _a0
func (_m *Plugin) InitConfig(_a0 config.KeySet) {
	_m.Called(_a0)
}

// MintTokens provides a mock function with given fields: ctx, nsOpID, poolLocator, mint
func (_m *Plugin) MintTokens(ctx context.Context, nsOpID string, poolLocator string, mint *core.TokenTransfer) error {
	ret := _m.Called(ctx, nsOpID, poolLocator, mint)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string, *core.TokenTransfer) error); ok {
		r0 = rf(ctx, nsOpID, poolLocator, mint)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Name provides a mock function with given fields:
func (_m *Plugin) Name() string {
	ret := _m.Called()

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// RegisterListener provides a mock function with given fields: listener
func (_m *Plugin) RegisterListener(listener tokens.Callbacks) {
	_m.Called(listener)
}

// Start provides a mock function with given fields:
func (_m *Plugin) Start() error {
	ret := _m.Called()

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// TokensApproval provides a mock function with given fields: ctx, nsOpID, poolLocator, approval
func (_m *Plugin) TokensApproval(ctx context.Context, nsOpID string, poolLocator string, approval *core.TokenApproval) error {
	ret := _m.Called(ctx, nsOpID, poolLocator, approval)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string, *core.TokenApproval) error); ok {
		r0 = rf(ctx, nsOpID, poolLocator, approval)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// TransferTokens provides a mock function with given fields: ctx, nsOpID, poolLocator, transfer
func (_m *Plugin) TransferTokens(ctx context.Context, nsOpID string, poolLocator string, transfer *core.TokenTransfer) error {
	ret := _m.Called(ctx, nsOpID, poolLocator, transfer)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string, *core.TokenTransfer) error); ok {
		r0 = rf(ctx, nsOpID, poolLocator, transfer)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}
