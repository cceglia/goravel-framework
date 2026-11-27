package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/goravel/framework/contracts/binding"
	contractsconfig "github.com/goravel/framework/contracts/config"
	contractsconsole "github.com/goravel/framework/contracts/console"
	contractsdatabase "github.com/goravel/framework/contracts/database"
	contractsdb "github.com/goravel/framework/contracts/database/db"
	contractsdriver "github.com/goravel/framework/contracts/database/driver"
	contractsorm "github.com/goravel/framework/contracts/database/orm"
	contractsschema "github.com/goravel/framework/contracts/database/schema"
	contractsseeder "github.com/goravel/framework/contracts/database/seeder"
	"github.com/goravel/framework/contracts/foundation"
	contractslog "github.com/goravel/framework/contracts/log"
	databaseorm "github.com/goravel/framework/database/orm"
	mocksconfig "github.com/goravel/framework/mocks/config"
	mocksconsole "github.com/goravel/framework/mocks/console"
	mocksdb "github.com/goravel/framework/mocks/database/db"
	mocksdriver "github.com/goravel/framework/mocks/database/driver"
	mocksorm "github.com/goravel/framework/mocks/database/orm"
	mocksschema "github.com/goravel/framework/mocks/database/schema"
	mocksseeder "github.com/goravel/framework/mocks/database/seeder"
	mocksfoundation "github.com/goravel/framework/mocks/foundation"
	mockslog "github.com/goravel/framework/mocks/log"
)

func TestServiceProviderBoot(t *testing.T) {
	t.Run("registers commands through app.Commands", func(t *testing.T) {
		provider := &ServiceProvider{}
		app := mocksfoundation.NewApplication(t)
		config := mocksconfig.NewConfig(t)

		app.EXPECT().MakeArtisan().Return(mocksconsole.NewArtisan(t)).Once()
		app.EXPECT().MakeConfig().Return(config).Once()
		app.EXPECT().MakeLog().Return(mockslog.NewLog(t)).Once()
		app.EXPECT().MakeSchema().Return(mocksschema.NewSchema(t)).Once()
		app.EXPECT().MakeSeeder().Return(mocksseeder.NewFacade(t)).Once()
		config.EXPECT().GetString("database.migrations.table").Return("migrations").Once()

		expectedSignatures := []string{
			"make:migration",
			"migrate",
			"migrate:rollback",
			"migrate:reset",
			"migrate:refresh",
			"migrate:fresh",
			"migrate:status",
			"make:model",
			"make:observer",
			"db:seed",
			"make:seeder",
			"make:factory",
			"db:table",
			"db:show",
			"db:wipe",
		}
		app.EXPECT().Commands(mock.MatchedBy(func(commands []contractsconsole.Command) bool {
			signatures := make([]string, len(commands))
			for i, command := range commands {
				signatures[i] = command.Signature()
			}

			return assert.ObjectsAreEqual(expectedSignatures, signatures)
		})).Once()

		provider.Boot(app)
	})

	tests := []struct {
		name    string
		artisan func(t *testing.T) contractsconsole.Artisan
		config  func(t *testing.T) contractsconfig.Config
		log     func(t *testing.T) contractslog.Log
		schema  func(t *testing.T) contractsschema.Schema
		seeder  func(t *testing.T) contractsseeder.Facade
	}{
		{name: "artisan is nil", artisan: nilFactory[contractsconsole.Artisan]},
		{name: "config is nil", config: nilFactory[contractsconfig.Config]},
		{name: "log is nil", log: nilFactory[contractslog.Log]},
		{name: "schema is nil", schema: nilFactory[contractsschema.Schema]},
		{name: "seeder is nil", seeder: nilFactory[contractsseeder.Facade]},
	}

	for _, tt := range tests {
		t.Run("skips registration when "+tt.name, func(t *testing.T) {
			provider := &ServiceProvider{}
			app := mocksfoundation.NewApplication(t)

			app.EXPECT().MakeArtisan().Return(orDefault(t, tt.artisan, func(t *testing.T) contractsconsole.Artisan { return mocksconsole.NewArtisan(t) })).Once()
			app.EXPECT().MakeConfig().Return(orDefault(t, tt.config, func(t *testing.T) contractsconfig.Config { return mocksconfig.NewConfig(t) })).Once()
			app.EXPECT().MakeLog().Return(orDefault(t, tt.log, func(t *testing.T) contractslog.Log { return mockslog.NewLog(t) })).Once()
			app.EXPECT().MakeSchema().Return(orDefault(t, tt.schema, func(t *testing.T) contractsschema.Schema { return mocksschema.NewSchema(t) })).Once()
			app.EXPECT().MakeSeeder().Return(orDefault(t, tt.seeder, func(t *testing.T) contractsseeder.Facade { return mocksseeder.NewFacade(t) })).Once()

			// app.Commands is not expected; the mock fails the test if it is called.
			provider.Boot(app)
		})
	}
}

func TestServiceProviderRegister_OrmFresh(t *testing.T) {
	tests := []struct {
		name         string
		makeOrm      contractsorm.Orm
		makeDB       contractsdb.DB
		expectEvents []string
	}{
		{
			name:         "forgets the Orm and DB bindings then resolves both",
			makeOrm:      mocksorm.NewOrm(t),
			makeDB:       mocksdb.NewDB(t),
			expectEvents: []string{"fresh", "makeOrm", "makeDB"},
		},
		{
			name:         "returns when both factories fail",
			expectEvents: []string{"fresh", "makeOrm", "makeDB"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &ServiceProvider{}
			app := mocksfoundation.NewApplication(t)
			config := mocksconfig.NewConfig(t)

			var ormFactory func(foundation.Application) (any, error)
			app.EXPECT().Singleton(binding.Orm, mock.Anything).Run(func(_ any, callback func(foundation.Application) (any, error)) {
				ormFactory = callback
			}).Once()
			app.EXPECT().Singleton(binding.DB, mock.Anything).Once()
			app.EXPECT().Singleton(binding.Schema, mock.Anything).Once()
			app.EXPECT().Singleton(binding.Seeder, mock.Anything).Once()
			provider.Register(app)
			require.NotNil(t, ormFactory)

			app.EXPECT().MakeConfig().Return(config).Once()
			app.EXPECT().MakeLog().Return(mockslog.NewLog(t)).Once()
			config.EXPECT().GetString("database.default").Return("primary").Once()
			config.EXPECT().Get("database.connections.primary.via").Return(nil).Once()

			instance, err := ormFactory(app)
			require.NoError(t, err)
			orm, ok := instance.(*databaseorm.Orm)
			require.True(t, ok)

			var events []string
			app.EXPECT().Fresh(binding.Orm, binding.DB).Run(func(...any) {
				events = append(events, "fresh")
			}).Once()
			app.EXPECT().MakeOrm().RunAndReturn(func() contractsorm.Orm {
				events = append(events, "makeOrm")
				return tt.makeOrm
			}).Once()
			app.EXPECT().MakeDB().RunAndReturn(func() contractsdb.DB {
				events = append(events, "makeDB")
				return tt.makeDB
			}).Once()

			orm.Fresh()

			assert.Equal(t, tt.expectEvents, events)
		})
	}
}

func TestServiceProviderRegister_SchemaResolvesOrmOnUse(t *testing.T) {
	provider := &ServiceProvider{}
	app := mocksfoundation.NewApplication(t)

	var factory func(foundation.Application) (any, error)
	app.EXPECT().Singleton(binding.Orm, mock.Anything).Once()
	app.EXPECT().Singleton(binding.DB, mock.Anything).Once()
	app.EXPECT().Singleton(binding.Schema, mock.Anything).Run(func(_ any, callback func(foundation.Application) (any, error)) {
		factory = callback
	}).Once()
	app.EXPECT().Singleton(binding.Seeder, mock.Anything).Once()
	provider.Register(app)
	require.NotNil(t, factory)

	builtOrm := mocksorm.NewOrm(t)
	builtOrm.EXPECT().Name().Return("primary").Twice()
	resolvedOrm := mocksorm.NewOrm(t)

	mockDriver := mocksdriver.NewDriver(t)
	mockDriver.EXPECT().Pool().Return(contractsdatabase.Pool{Writers: []contractsdatabase.Config{{}}}).Once()
	mockDriver.EXPECT().Grammar().Return(nil).Once()
	mockDriver.EXPECT().Processor().Return(nil).Once()

	config := mocksconfig.NewConfig(t)
	config.EXPECT().Get("database.connections.primary.via").Return(func() (contractsdriver.Driver, error) {
		return mockDriver, nil
	}).Once()

	app.EXPECT().MakeConfig().Return(config).Once()
	app.EXPECT().MakeLog().Return(mockslog.NewLog(t)).Once()
	app.EXPECT().MakeOrm().Return(builtOrm).Once()

	instance, err := factory(app)
	require.NoError(t, err)
	schema, ok := instance.(contractsschema.Schema)
	require.True(t, ok)

	app.EXPECT().MakeOrm().Return(resolvedOrm).Once()

	assert.Same(t, resolvedOrm, schema.Orm())
}

func nilFactory[T any](*testing.T) T {
	var zero T
	return zero
}

func orDefault[T any](t *testing.T, override, fallback func(*testing.T) T) T {
	if override != nil {
		return override(t)
	}

	return fallback(t)
}
