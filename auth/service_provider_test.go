package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	contractsauthaccess "github.com/goravel/framework/contracts/auth/access"
	contractsbinding "github.com/goravel/framework/contracts/binding"
	cachecontract "github.com/goravel/framework/contracts/cache"
	contractsconsole "github.com/goravel/framework/contracts/console"
	ormcontract "github.com/goravel/framework/contracts/database/orm"
	contractsfoundation "github.com/goravel/framework/contracts/foundation"
	frameworkerrors "github.com/goravel/framework/errors"
	mockscache "github.com/goravel/framework/mocks/cache"
	mocksconfig "github.com/goravel/framework/mocks/config"
	mocksorm "github.com/goravel/framework/mocks/database/orm"
	mocksfoundation "github.com/goravel/framework/mocks/foundation"
	mockshttp "github.com/goravel/framework/mocks/http"
	mockslog "github.com/goravel/framework/mocks/log"
	mockssession "github.com/goravel/framework/mocks/session"
	"github.com/goravel/framework/support/binding"
)

const expectedAuthCommandCount = 2

func TestAuthServiceProviderRelationship(t *testing.T) {
	provider := &ServiceProvider{}

	relationship := provider.Relationship()
	bindings := []string{contractsbinding.Auth, contractsbinding.Gate}

	assert.Equal(t, bindings, relationship.Bindings)
	assert.Equal(t, binding.Dependencies(bindings...), relationship.Dependencies)
	assert.Empty(t, relationship.ProvideFor)
}

func TestAuthServiceProviderRegister(t *testing.T) {
	provider := &ServiceProvider{}
	application := mocksfoundation.NewApplication(t)

	var authCallback func(contractsfoundation.Application, map[string]any) (any, error)
	var gateCallback func(contractsfoundation.Application) (any, error)
	application.EXPECT().BindWith(contractsbinding.Auth, mock.AnythingOfType("func(foundation.Application, map[string]interface {}) (interface {}, error)")).Run(func(_ any, callback func(contractsfoundation.Application, map[string]any) (any, error)) {
		authCallback = callback
	}).Once()
	application.EXPECT().Singleton(contractsbinding.Gate, mock.AnythingOfType("func(foundation.Application) (interface {}, error)")).Run(func(_ any, callback func(contractsfoundation.Application) (any, error)) {
		gateCallback = callback
	}).Once()

	previousApp := registeredApp
	t.Cleanup(func() {
		registeredApp = previousApp
	})

	provider.Register(application)
	assert.Same(t, application, registeredApp)
	assert.NotNil(t, authCallback)
	assert.NotNil(t, gateCallback)

	t.Run("returns error when config facade is nil", func(t *testing.T) {
		callbackApp := mocksfoundation.NewApplication(t)
		callbackApp.EXPECT().MakeConfig().Return(nil).Once()

		instance, err := authCallback(callbackApp, map[string]any{})

		assert.Nil(t, instance)
		assert.Error(t, err)
		assert.True(t, frameworkerrors.Is(err, frameworkerrors.ConfigFacadeNotSet))
	})

	t.Run("returns error when log facade is nil", func(t *testing.T) {
		callbackApp := mocksfoundation.NewApplication(t)
		config := mocksconfig.NewConfig(t)
		callbackApp.EXPECT().MakeConfig().Return(config).Once()
		callbackApp.EXPECT().MakeLog().Return(nil).Once()

		instance, err := authCallback(callbackApp, map[string]any{})

		assert.Nil(t, instance)
		assert.Error(t, err)
		assert.True(t, frameworkerrors.Is(err, frameworkerrors.LogFacadeNotSet))
	})

	t.Run("creates auth instance without context", func(t *testing.T) {
		callbackApp := mocksfoundation.NewApplication(t)
		config := mocksconfig.NewConfig(t)
		log := mockslog.NewLog(t)
		callbackApp.EXPECT().MakeConfig().Return(config).Once()
		callbackApp.EXPECT().MakeLog().Return(log).Once()
		config.EXPECT().GetString("auth.defaults.guard").Return("").Once()

		instance, err := authCallback(callbackApp, map[string]any{})

		assert.NoError(t, err)
		assert.IsType(t, &Auth{}, instance)
	})

	t.Run("creates auth instance with context parameter", func(t *testing.T) {
		callbackApp := mocksfoundation.NewApplication(t)
		config := mocksconfig.NewConfig(t)
		log := mockslog.NewLog(t)
		ctx := mockshttp.NewContext(t)
		request := mockshttp.NewContextRequest(t)
		session := mockssession.NewSession(t)
		useFacades(t, nil, mocksorm.NewOrm(t))

		callbackApp.EXPECT().MakeConfig().Return(config).Once()
		callbackApp.EXPECT().MakeLog().Return(log).Once()
		config.EXPECT().GetString("auth.defaults.guard").Return("web").Once()
		config.EXPECT().GetString("auth.guards.web.driver").Return("session").Once()
		config.EXPECT().GetString("auth.guards.web.provider").Return("users").Once()
		config.EXPECT().GetString("auth.providers.users.driver").Return("orm").Once()
		ctx.EXPECT().Request().Return(request).Once()
		request.EXPECT().Session().Return(session).Once()

		instance, err := authCallback(callbackApp, map[string]any{
			"ctx": ctx,
		})

		assert.NoError(t, err)
		auth, ok := instance.(*Auth)
		assert.True(t, ok)
		assert.Same(t, ctx, auth.ctx)
		assert.NotNil(t, auth.GuardDriver)
	})

	t.Run("registers gate singleton", func(t *testing.T) {
		instance, err := gateCallback(application)

		assert.NoError(t, err)
		assert.Implements(t, (*contractsauthaccess.Gate)(nil), instance)
	})
}

func TestAuthServiceProviderBoot(t *testing.T) {
	// This test mutates package-level facades and should not run in parallel.
	provider := &ServiceProvider{}
	app := mocksfoundation.NewApplication(t)
	config := mocksconfig.NewConfig(t)
	orm := mocksorm.NewOrm(t)

	app.EXPECT().MakeOrm().Return(orm).Once()
	app.EXPECT().MakeConfig().Return(config).Once()
	app.EXPECT().Commands(mock.MatchedBy(func(commands []contractsconsole.Command) bool {
		if len(commands) != expectedAuthCommandCount {
			t.Logf("expected %d auth commands, got %d", expectedAuthCommandCount, len(commands))
			return false
		}
		for i, command := range commands {
			if command == nil {
				t.Logf("auth command %d is nil", i)
				return false
			}
		}

		return true
	})).Once()

	provider.Boot(app)
}

func TestAuthServiceProviderFacadesReturnNilWithoutApplication(t *testing.T) {
	previousApp := registeredApp
	t.Cleanup(func() {
		registeredApp = previousApp
	})
	registeredApp = nil

	assert.Nil(t, makeCache())
	assert.Nil(t, makeOrm())
}

func TestAuthServiceProviderFacadesResolveOnUse(t *testing.T) {
	mockApp := useApplication(t)
	firstCache := mockscache.NewCache(t)
	secondCache := mockscache.NewCache(t)
	firstOrm := mocksorm.NewOrm(t)
	secondOrm := mocksorm.NewOrm(t)

	mockApp.EXPECT().MakeCache().Return(firstCache).Once()
	mockApp.EXPECT().MakeCache().Return(secondCache).Once()
	mockApp.EXPECT().MakeOrm().Return(firstOrm).Once()
	mockApp.EXPECT().MakeOrm().Return(secondOrm).Once()

	assert.Same(t, firstCache, makeCache())
	assert.Same(t, secondCache, makeCache())
	assert.Same(t, firstOrm, makeOrm())
	assert.Same(t, secondOrm, makeOrm())
}

func TestOrmUserProviderKeepsOrmResolvedWhenBuilt(t *testing.T) {
	mockApp := useApplication(t)
	firstOrm := mocksorm.NewOrm(t)
	secondOrm := mocksorm.NewOrm(t)
	mockApp.EXPECT().MakeOrm().Return(firstOrm).Once()
	mockApp.EXPECT().MakeOrm().Return(secondOrm).Once()

	first, err := NewOrmUserProvider(Background())
	assert.NoError(t, err)
	second, err := NewOrmUserProvider(Background())
	assert.NoError(t, err)

	assert.Same(t, firstOrm, first.(*OrmUserProvider).orm)
	assert.Same(t, secondOrm, second.(*OrmUserProvider).orm)
}

func TestNewJwtGuardKeepsCacheResolvedWhenBuilt(t *testing.T) {
	mockApp := useApplication(t)
	firstCache := mockscache.NewCache(t)
	secondCache := mockscache.NewCache(t)
	mockApp.EXPECT().MakeCache().Return(firstCache).Once()
	mockApp.EXPECT().MakeCache().Return(secondCache).Once()

	config := mocksconfig.NewConfig(t)
	config.EXPECT().GetString("auth.guards.user.secret").Return("a").Twice()
	config.EXPECT().GetInt("auth.guards.user.ttl").Return(2).Twice()
	config.EXPECT().GetInt("auth.guards.user.refresh_ttl").Return(2).Twice()
	previousConfig := configFacade
	t.Cleanup(func() {
		configFacade = previousConfig
	})
	configFacade = config

	first, err := NewJwtGuard(Background(), "user", nil)
	assert.NoError(t, err)
	second, err := NewJwtGuard(Background(), "user", nil)
	assert.NoError(t, err)

	assert.Same(t, firstCache, first.(*JwtGuard).cache)
	assert.Same(t, secondCache, second.(*JwtGuard).cache)
}

func useApplication(t *testing.T) *mocksfoundation.Application {
	t.Helper()

	previous := registeredApp
	mockApp := mocksfoundation.NewApplication(t)
	registeredApp = mockApp
	t.Cleanup(func() {
		registeredApp = previous
	})

	return mockApp
}

func useFacades(t *testing.T, cache cachecontract.Cache, orm ormcontract.Orm) {
	t.Helper()

	mockApp := useApplication(t)
	mockApp.EXPECT().MakeCache().Return(cache).Maybe()
	mockApp.EXPECT().MakeOrm().Return(orm).Maybe()
}
