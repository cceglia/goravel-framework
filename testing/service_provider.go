package testing

import (
	"github.com/goravel/framework/contracts/binding"
	contractsconsole "github.com/goravel/framework/contracts/console"
	"github.com/goravel/framework/contracts/foundation"
	contractsroute "github.com/goravel/framework/contracts/route"
	contractsession "github.com/goravel/framework/contracts/session"
	"github.com/goravel/framework/errors"
	"github.com/goravel/framework/support/color"
)

var (
	json foundation.Json

	registeredApp foundation.Application
)

// makeArtisan resolves the artisan facade from the application, and returns nil
// when the provider never registered.
func makeArtisan() contractsconsole.Artisan {
	if registeredApp == nil {
		return nil
	}

	return registeredApp.MakeArtisan()
}

// makeRoute resolves the route facade from the application, and returns nil
// when the provider never registered.
func makeRoute() contractsroute.Route {
	if registeredApp == nil {
		return nil
	}

	return registeredApp.MakeRoute()
}

// makeSession resolves the session facade from the application, and returns nil
// when the provider never registered.
func makeSession() contractsession.Manager {
	if registeredApp == nil {
		return nil
	}

	return registeredApp.MakeSession()
}

type ServiceProvider struct {
}

func (r *ServiceProvider) Relationship() binding.Relationship {
	return binding.Relationship{
		Bindings: []string{
			binding.Testing,
		},
		Dependencies: binding.Bindings[binding.Testing].Dependencies,
		ProvideFor:   []string{},
	}
}

func (r *ServiceProvider) Register(app foundation.Application) {
	registeredApp = app

	app.Singleton(binding.Testing, func(app foundation.Application) (any, error) {
		config := app.MakeConfig()
		if config == nil {
			return nil, errors.ConfigFacadeNotSet.SetModule(errors.ModuleTesting)
		}

		// They are checked when using actually
		artisan := app.MakeArtisan()
		cache := app.MakeCache()
		orm := app.MakeOrm()
		process := app.MakeProcess()

		var options []Option
		if orm != nil {
			options = append(options, WithOrmResolver(app.MakeOrm))
		}

		return NewApplication(artisan, cache, config, orm, process, options...), nil
	})
}

func (r *ServiceProvider) Boot(app foundation.Application) {
	// The facades are resolved here only to print a message for a missing one.
	if app.MakeArtisan() == nil {
		color.Errorln(errors.ConsoleFacadeNotSet.SetModule(errors.ModuleTesting))
	}

	if app.MakeRoute() == nil {
		color.Errorln(errors.RouteFacadeNotSet.SetModule(errors.ModuleTesting))
	}

	if app.MakeSession() == nil {
		color.Errorln(errors.SessionFacadeNotSet.SetModule(errors.ModuleTesting))
	}

	json = app.GetJson()
}
