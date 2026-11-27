package validation

import (
	"context"

	"github.com/goravel/framework/contracts/binding"
	consolecontract "github.com/goravel/framework/contracts/console"
	"github.com/goravel/framework/contracts/database/orm"
	"github.com/goravel/framework/contracts/foundation"
	contractstranslation "github.com/goravel/framework/contracts/translation"
	"github.com/goravel/framework/validation/console"
)

var (
	langFacade = func(ctx context.Context) contractstranslation.Translator { return nil }

	registeredApp foundation.Application
)

// makeOrm resolves the ORM facade from the application, and returns nil when the
// provider never registered.
func makeOrm() orm.Orm {
	if registeredApp == nil {
		return nil
	}

	return registeredApp.MakeOrm()
}

type ServiceProvider struct {
}

func (r *ServiceProvider) Relationship() binding.Relationship {
	return binding.Relationship{
		Bindings: []string{
			binding.Validation,
		},
		Dependencies: binding.Bindings[binding.Validation].Dependencies,
		ProvideFor:   []string{},
	}
}

func (r *ServiceProvider) Register(app foundation.Application) {
	registeredApp = app

	app.Singleton(binding.Validation, func(app foundation.Application) (any, error) {
		return NewValidation(), nil
	})
}

func (r *ServiceProvider) Boot(app foundation.Application) {
	// Resolving the ORM builds it at boot, so its connection warning keeps its timing.
	_ = app.MakeOrm()
	langFacade = app.MakeLang

	app.Commands([]consolecontract.Command{
		console.NewLangPublishCommand(app.LangPath(), LangFS),
		&console.RuleMakeCommand{},
		&console.FilterMakeCommand{},
	})
}
