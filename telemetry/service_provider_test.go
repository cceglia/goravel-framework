package telemetry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/goravel/framework/contracts/binding"
	"github.com/goravel/framework/contracts/foundation"
	"github.com/goravel/framework/errors"
	mocksconfig "github.com/goravel/framework/mocks/config"
	mocksfoundation "github.com/goravel/framework/mocks/foundation"
	mockstelemetry "github.com/goravel/framework/mocks/telemetry"
)

func TestMakeTelemetry(t *testing.T) {
	restore := func(t *testing.T) {
		originApp := registeredApp
		originFacade := Facade
		t.Cleanup(func() {
			registeredApp = originApp
			Facade = originFacade
		})
	}

	t.Run("returns the exported variable first", func(t *testing.T) {
		restore(t)

		mock := mockstelemetry.NewTelemetry(t)
		registeredApp = nil
		Facade = mock

		assert.Same(t, mock, MakeTelemetry())
	})

	t.Run("returns nil when the provider never registered", func(t *testing.T) {
		restore(t)

		registeredApp = nil
		Facade = nil

		assert.Nil(t, MakeTelemetry())
	})

	t.Run("resolves from the application", func(t *testing.T) {
		restore(t)

		mock := mockstelemetry.NewTelemetry(t)
		mockApp := mocksfoundation.NewApplication(t)
		mockApp.EXPECT().MakeTelemetry().Return(mock).Once()
		registeredApp = mockApp
		Facade = nil

		assert.Same(t, mock, MakeTelemetry())
	})
}

func TestServiceProvider_Register(t *testing.T) {
	originApp := registeredApp
	t.Cleanup(func() { registeredApp = originApp })
	registeredApp = nil

	mockApp := mocksfoundation.NewApplication(t)
	mockApp.EXPECT().Singleton(binding.Telemetry, mock.Anything).Once()

	(&ServiceProvider{}).Register(mockApp)

	assert.Same(t, mockApp, registeredApp)
}

func TestServiceProvider_Register_Binding(t *testing.T) {
	originApp := registeredApp
	t.Cleanup(func() { registeredApp = originApp })

	tests := []struct {
		name        string
		config      func(t *testing.T) *mocksconfig.Config
		expectError error
	}{
		{
			name: "returns the config facade error",
			config: func(t *testing.T) *mocksconfig.Config {
				return nil
			},
			expectError: errors.ConfigFacadeNotSet.SetModule(errors.ModuleTelemetry),
		},
		{
			name: "returns the unmarshal error",
			config: func(t *testing.T) *mocksconfig.Config {
				cfg := mocksconfig.NewConfig(t)
				cfg.EXPECT().UnmarshalKey("telemetry", mock.Anything).Return(assert.AnError).Once()
				return cfg
			},
			expectError: assert.AnError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var factory func(foundation.Application) (any, error)
			mockApp := mocksfoundation.NewApplication(t)
			mockApp.EXPECT().Singleton(binding.Telemetry, mock.Anything).Run(func(_ any, callback func(foundation.Application) (any, error)) {
				factory = callback
			}).Once()
			if cfg := tt.config(t); cfg != nil {
				mockApp.EXPECT().MakeConfig().Return(cfg).Once()
			} else {
				mockApp.EXPECT().MakeConfig().Return(nil).Once()
			}

			(&ServiceProvider{}).Register(mockApp)
			require.NotNil(t, factory)

			instance, err := factory(mockApp)

			assert.Nil(t, instance)
			assert.Equal(t, tt.expectError, err)
		})
	}
}

func TestServiceProvider_FlushState(t *testing.T) {
	provider := &ServiceProvider{}
	origin := Facade
	t.Cleanup(func() { Facade = origin })

	Facade = mockstelemetry.NewTelemetry(t)
	provider.FlushState(mocksfoundation.NewApplication(t))
	assert.Nil(t, Facade)

	assert.NotPanics(t, func() {
		provider.FlushState(mocksfoundation.NewApplication(t))
	})
}
