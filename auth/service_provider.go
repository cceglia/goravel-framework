package auth

import (
	"context"

	"github.com/goravel/framework/auth/access"
	"github.com/goravel/framework/auth/console"
	contractsbinding "github.com/goravel/framework/contracts/binding"
	"github.com/goravel/framework/contracts/cache"
	"github.com/goravel/framework/contracts/config"
	contractconsole "github.com/goravel/framework/contracts/console"
	"github.com/goravel/framework/contracts/database/orm"
	"github.com/goravel/framework/contracts/foundation"
	"github.com/goravel/framework/contracts/http"
	"github.com/goravel/framework/errors"
	"github.com/goravel/framework/support/binding"
)

var (
	configFacade config.Config

	registeredApp foundation.Application
)

// makeCache resolves the cache facade from the application, and returns nil
// when the provider never registered.
func makeCache() cache.Cache {
	if registeredApp == nil {
		return nil
	}

	return registeredApp.MakeCache()
}

// makeOrm resolves the ORM facade from the application, and returns nil when
// the provider never registered. A guard or user provider keeps the facade it
// resolves for its own lifetime, so a caller must not keep one across Orm.Fresh.
func makeOrm() orm.Orm {
	if registeredApp == nil {
		return nil
	}

	return registeredApp.MakeOrm()
}

type ServiceProvider struct {
}

func (r *ServiceProvider) Relationship() contractsbinding.Relationship {
	bindings := []string{
		contractsbinding.Auth,
		contractsbinding.Gate,
	}

	return contractsbinding.Relationship{
		Bindings:     bindings,
		Dependencies: binding.Dependencies(bindings...),
		ProvideFor:   []string{},
	}
}

func (r *ServiceProvider) Register(app foundation.Application) {
	registeredApp = app

	app.BindWith(contractsbinding.Auth, func(app foundation.Application, parameters map[string]any) (any, error) {
		configFacade = app.MakeConfig()
		if configFacade == nil {
			return nil, errors.ConfigFacadeNotSet.SetModule(errors.ModuleAuth)
		}

		log := app.MakeLog()
		if log == nil {
			return nil, errors.LogFacadeNotSet.SetModule(errors.ModuleAuth)
		}

		ctx, ok := parameters["ctx"]
		if ok {
			return NewAuth(ctx.(http.Context), configFacade, log)
		}

		// ctx is optional when calling facades.Auth().Extend()
		return NewAuth(nil, configFacade, log)
	})
	app.Singleton(contractsbinding.Gate, func(app foundation.Application) (any, error) {
		return access.NewGate(context.Background()), nil
	})
}

func (r *ServiceProvider) Boot(app foundation.Application) {
	// Resolving the ORM builds it at boot, so its connection warning keeps its timing.
	_ = app.MakeOrm()

	r.registerCommands(app)
}

func (r *ServiceProvider) registerCommands(app foundation.Application) {
	app.Commands([]contractconsole.Command{
		console.NewJwtSecretCommand(app.MakeConfig()),
		console.NewPolicyMakeCommand(),
	})
}
