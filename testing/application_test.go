package testing

import (
	"testing"

	"github.com/stretchr/testify/assert"

	contractsdriver "github.com/goravel/framework/contracts/database/driver"
	contractsorm "github.com/goravel/framework/contracts/database/orm"
	"github.com/goravel/framework/errors"
	mocksconfig "github.com/goravel/framework/mocks/config"
	mocksconsole "github.com/goravel/framework/mocks/console"
	mocksdriver "github.com/goravel/framework/mocks/database/driver"
	mocksorm "github.com/goravel/framework/mocks/database/orm"
	mocksdocker "github.com/goravel/framework/mocks/testing/docker"
)

func TestApplication_Docker(t *testing.T) {
	tests := []struct {
		name        string
		buildOrm    func(t *testing.T) contractsorm.Orm
		resolveOrm  func(t *testing.T) func() contractsorm.Orm
		expectError error
	}{
		{
			name:     "without a resolver the Docker helper uses the build Orm",
			buildOrm: func(t *testing.T) contractsorm.Orm { return mocksorm.NewOrm(t) },
		},
		{
			name:        "without a resolver and without a build Orm the Docker helper reports the missing Orm",
			buildOrm:    func(t *testing.T) contractsorm.Orm { return nil },
			expectError: errors.OrmFacadeNotSet.SetModule(errors.ModuleTesting),
		},
		{
			name:     "a nil resolver function is treated as no resolver",
			buildOrm: func(t *testing.T) contractsorm.Orm { return mocksorm.NewOrm(t) },
			resolveOrm: func(t *testing.T) func() contractsorm.Orm {
				return nil
			},
		},
		{
			name:     "a resolver supplies the Orm instead of the build Orm",
			buildOrm: func(t *testing.T) contractsorm.Orm { return mocksorm.NewOrm(t) },
			resolveOrm: func(t *testing.T) func() contractsorm.Orm {
				return func() contractsorm.Orm { return mocksorm.NewOrm(t) }
			},
		},
		{
			name:     "a resolver that returns nil gives the nil Orm behavior",
			buildOrm: func(t *testing.T) contractsorm.Orm { return mocksorm.NewOrm(t) },
			resolveOrm: func(t *testing.T) func() contractsorm.Orm {
				return func() contractsorm.Orm { return nil }
			},
			expectError: errors.OrmFacadeNotSet.SetModule(errors.ModuleTesting),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buildOrm := tt.buildOrm(t)

			var options []Option
			if tt.resolveOrm != nil {
				options = append(options, WithOrmResolver(tt.resolveOrm(t)))
			}

			application := NewApplication(mocksconsole.NewArtisan(t), nil, databaseConfig(t), buildOrm, nil, options...)

			database, err := application.Docker().Database()

			assert.Equal(t, tt.expectError, err)
			if tt.expectError != nil {
				assert.Nil(t, database)
			} else {
				assert.NotNil(t, database)
			}
		})
	}
}

func TestApplication_Docker_UsesOrmResolvedAfterFresh(t *testing.T) {
	oldOrm := mocksorm.NewOrm(t)
	newOrm := mocksorm.NewOrm(t)
	current := contractsorm.Orm(oldOrm)

	application := NewApplication(
		mocksconsole.NewArtisan(t),
		nil,
		databaseConfig(t),
		oldOrm,
		nil,
		WithOrmResolver(func() contractsorm.Orm { return current }),
	)

	// The application is not rebuilt: only the Orm that the resolver returns changes.
	current = newOrm
	newOrm.EXPECT().Fresh().Once()

	database, err := application.Docker().Database()
	assert.NoError(t, err)
	assert.NoError(t, database.Ready())
}

func databaseConfig(t *testing.T) *mocksconfig.Config {
	t.Helper()

	databaseDocker := mocksdocker.NewDatabaseDriver(t)
	databaseDocker.EXPECT().Ready().Return(nil).Maybe()

	driver := mocksdriver.NewDriver(t)
	driver.EXPECT().Docker().Return(databaseDocker, nil).Maybe()

	config := mocksconfig.NewConfig(t)
	config.EXPECT().GetString("database.default").Return("primary").Maybe()
	config.EXPECT().Get("database.connections.primary.via").Return(func() (contractsdriver.Driver, error) {
		return driver, nil
	}).Maybe()

	return config
}
