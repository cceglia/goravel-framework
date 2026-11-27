package session

import (
	"github.com/goravel/framework/contracts/binding"
	"github.com/goravel/framework/contracts/config"
	"github.com/goravel/framework/contracts/foundation"
	"github.com/goravel/framework/contracts/session"
	"github.com/goravel/framework/errors"
)

var (
	ConfigFacade config.Config

	// SessionFacade is the session facade assigned during Boot.
	//
	// Deprecated: Use App().MakeSession() instead.
	SessionFacade session.Manager

	registeredApp foundation.Application
)

var _ foundation.ServiceProviderWithFlushState = (*ServiceProvider)(nil)

// MakeSession returns the exported variable when it is set, so a mock keeps
// working. Otherwise it resolves the session facade from the application, and
// returns nil when the provider never registered.
func MakeSession() session.Manager {
	if SessionFacade != nil {
		return SessionFacade
	}

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
			binding.Session,
		},
		Dependencies: binding.Bindings[binding.Session].Dependencies,
		ProvideFor:   []string{},
	}
}

func (r *ServiceProvider) Register(app foundation.Application) {
	registeredApp = app

	app.Singleton(binding.Session, func(app foundation.Application) (any, error) {
		c := app.MakeConfig()
		if c == nil {
			return nil, errors.ConfigFacadeNotSet.SetModule(errors.ModuleSession)
		}

		j := app.GetJson()
		if j == nil {
			return nil, errors.JSONParserNotSet.SetModule(errors.ModuleSession)
		}

		return NewManager(c, j), nil
	})
}

func (r *ServiceProvider) Boot(app foundation.Application) {
	SessionFacade = app.MakeSession()
	ConfigFacade = app.MakeConfig()
}

// FlushState clears the deprecated session facade variable. Boot assigns it again.
func (r *ServiceProvider) FlushState(foundation.Application) {
	SessionFacade = nil
}
