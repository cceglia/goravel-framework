package filesystem

import (
	"github.com/goravel/framework/contracts/binding"
	configcontract "github.com/goravel/framework/contracts/config"
	filesystemcontract "github.com/goravel/framework/contracts/filesystem"
	"github.com/goravel/framework/contracts/foundation"
	"github.com/goravel/framework/errors"
)

var (
	ConfigFacade configcontract.Config

	// StorageFacade is the storage facade assigned during Boot.
	//
	// Deprecated: Use App().MakeStorage() instead.
	StorageFacade filesystemcontract.Storage

	registeredApp foundation.Application
)

var _ foundation.ServiceProviderWithFlushState = (*ServiceProvider)(nil)

// storageFacade returns the exported variable when it is set, so a mock keeps
// working. Otherwise it resolves the storage facade from the application, and
// returns nil when the provider never registered.
func storageFacade() filesystemcontract.Storage {
	if StorageFacade != nil {
		return StorageFacade
	}

	if registeredApp == nil {
		return nil
	}

	return registeredApp.MakeStorage()
}

type ServiceProvider struct {
}

func (r *ServiceProvider) Relationship() binding.Relationship {
	return binding.Relationship{
		Bindings: []string{
			binding.Storage,
		},
		Dependencies: binding.Bindings[binding.Storage].Dependencies,
		ProvideFor:   []string{},
	}
}

func (r *ServiceProvider) Register(app foundation.Application) {
	registeredApp = app

	app.Singleton(binding.Storage, func(app foundation.Application) (any, error) {
		config := app.MakeConfig()
		if config == nil {
			return nil, errors.ConfigFacadeNotSet.SetModule(errors.ModuleFilesystem)
		}

		return NewStorage(config)
	})
}

func (r *ServiceProvider) Boot(app foundation.Application) {
	ConfigFacade = app.MakeConfig()
	StorageFacade = app.MakeStorage()
}

// FlushState clears the deprecated storage facade variable. Boot assigns it again.
func (r *ServiceProvider) FlushState(foundation.Application) {
	StorageFacade = nil
}
