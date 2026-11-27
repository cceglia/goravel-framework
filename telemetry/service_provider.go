package telemetry

import (
	"github.com/goravel/framework/contracts/binding"
	"github.com/goravel/framework/contracts/config"
	"github.com/goravel/framework/contracts/foundation"
	"github.com/goravel/framework/contracts/telemetry"
	"github.com/goravel/framework/errors"
)

var (
	ConfigFacade config.Config

	// Facade is the telemetry facade assigned during Boot.
	//
	// Deprecated: Use App().MakeTelemetry() instead.
	Facade telemetry.Telemetry

	registeredApp foundation.Application
)

var _ foundation.ServiceProviderWithFlushState = (*ServiceProvider)(nil)

// MakeTelemetry returns the exported variable when it is set, so a mock keeps
// working. Otherwise it resolves the telemetry facade from the application, and
// returns nil when the provider never registered.
func MakeTelemetry() telemetry.Telemetry {
	if Facade != nil {
		return Facade
	}

	if registeredApp == nil {
		return nil
	}

	return registeredApp.MakeTelemetry()
}

type ServiceProvider struct {
}

func (r *ServiceProvider) Relationship() binding.Relationship {
	return binding.Relationship{
		Bindings: []string{
			binding.Telemetry,
		},
		Dependencies: binding.Bindings[binding.Telemetry].Dependencies,
		ProvideFor:   []string{},
	}
}

func (r *ServiceProvider) Register(app foundation.Application) {
	registeredApp = app

	app.Singleton(binding.Telemetry, func(app foundation.Application) (any, error) {
		cfg := app.MakeConfig()
		if cfg == nil {
			return nil, errors.ConfigFacadeNotSet.SetModule(errors.ModuleTelemetry)
		}

		var telemetryCfg Config
		if err := cfg.UnmarshalKey("telemetry", &telemetryCfg); err != nil {
			return nil, err
		}

		return NewApplication(telemetryCfg)
	})
}

func (r *ServiceProvider) Boot(app foundation.Application) {
	Facade = app.MakeTelemetry()
	ConfigFacade = app.MakeConfig()
}

func (r *ServiceProvider) Runners(app foundation.Application) []foundation.Runner {
	return []foundation.Runner{NewTelemetryRunner(app.MakeConfig(), app.MakeTelemetry())}
}

// FlushState clears the deprecated telemetry facade variable. Boot assigns it again.
func (r *ServiceProvider) FlushState(foundation.Application) {
	Facade = nil
}
