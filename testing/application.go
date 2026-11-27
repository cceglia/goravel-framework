package testing

import (
	contractscache "github.com/goravel/framework/contracts/cache"
	contractsconfig "github.com/goravel/framework/contracts/config"
	contractsconsole "github.com/goravel/framework/contracts/console"
	contractsorm "github.com/goravel/framework/contracts/database/orm"
	contractsprocess "github.com/goravel/framework/contracts/process"
	"github.com/goravel/framework/contracts/testing"
	"github.com/goravel/framework/testing/docker"
)

type Application struct {
	artisan    contractsconsole.Artisan
	cache      contractscache.Cache
	config     contractsconfig.Config
	orm        contractsorm.Orm
	process    contractsprocess.Process
	resolveOrm func() contractsorm.Orm
}

// Option configures an Application.
type Option func(*Application)

// WithOrmResolver makes the Application resolve its Orm each time the Docker helper is
// requested, instead of using the Orm it was built with. A nil resolve function is ignored.
func WithOrmResolver(resolve func() contractsorm.Orm) Option {
	return func(r *Application) {
		r.resolveOrm = resolve
	}
}

func NewApplication(
	artisan contractsconsole.Artisan,
	cache contractscache.Cache,
	config contractsconfig.Config,
	orm contractsorm.Orm,
	process contractsprocess.Process,
	options ...Option,
) *Application {
	application := &Application{
		artisan: artisan,
		cache:   cache,
		config:  config,
		orm:     orm,
		process: process,
	}
	for _, option := range options {
		option(application)
	}

	return application
}

func (r *Application) Docker() testing.Docker {
	orm := r.orm
	if r.resolveOrm != nil {
		orm = r.resolveOrm()
	}

	return docker.NewDocker(r.artisan, r.cache, r.config, orm, r.process)
}
