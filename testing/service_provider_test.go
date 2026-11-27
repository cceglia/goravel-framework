package testing

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/goravel/framework/contracts/binding"
	contractsconsole "github.com/goravel/framework/contracts/console"
	"github.com/goravel/framework/contracts/foundation"
	"github.com/goravel/framework/errors"
	mocksconfig "github.com/goravel/framework/mocks/config"
	mocksconsole "github.com/goravel/framework/mocks/console"
	mocksorm "github.com/goravel/framework/mocks/database/orm"
	mocksfoundation "github.com/goravel/framework/mocks/foundation"
	mocksroute "github.com/goravel/framework/mocks/route"
	mockssession "github.com/goravel/framework/mocks/session"
	"github.com/goravel/framework/support/color"
)

func TestServiceProvider_Register(t *testing.T) {
	previous := registeredApp
	t.Cleanup(func() {
		registeredApp = previous
	})
	registeredApp = nil

	mockApp := mocksfoundation.NewApplication(t)
	mockApp.EXPECT().Singleton(binding.Testing, mock.Anything).Once()

	(&ServiceProvider{}).Register(mockApp)

	assert.Same(t, mockApp, registeredApp)
}

func TestServiceProvider_Register_Binding(t *testing.T) {
	tests := []struct {
		name      string
		hasOrm    bool
		expectOrm bool
	}{
		{name: "resolves the Orm on use when the Orm is set", hasOrm: true, expectOrm: true},
		{name: "builds without a resolver when the Orm is nil"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			previous := registeredApp
			t.Cleanup(func() {
				registeredApp = previous
			})

			mockApp := mocksfoundation.NewApplication(t)
			var factory func(foundation.Application) (any, error)
			mockApp.EXPECT().Singleton(binding.Testing, mock.Anything).Run(func(_ any, callback func(foundation.Application) (any, error)) {
				factory = callback
			}).Once()
			(&ServiceProvider{}).Register(mockApp)
			assert.NotNil(t, factory)

			builtOrm := mocksorm.NewOrm(t)
			resolvedOrm := mocksorm.NewOrm(t)
			mockApp.EXPECT().MakeConfig().Return(mocksconfig.NewConfig(t)).Once()
			mockApp.EXPECT().MakeArtisan().Return(nil).Once()
			mockApp.EXPECT().MakeCache().Return(nil).Once()
			mockApp.EXPECT().MakeProcess().Return(nil).Once()
			if tt.hasOrm {
				mockApp.EXPECT().MakeOrm().Return(builtOrm).Once()
			} else {
				mockApp.EXPECT().MakeOrm().Return(nil).Once()
			}

			instance, err := factory(mockApp)
			assert.NoError(t, err)
			application, ok := instance.(*Application)
			assert.True(t, ok)

			if !tt.expectOrm {
				assert.Nil(t, application.orm)
				assert.Nil(t, application.resolveOrm)
				return
			}

			assert.Same(t, builtOrm, application.orm)
			mockApp.EXPECT().MakeOrm().Return(resolvedOrm).Once()
			assert.Same(t, resolvedOrm, application.resolveOrm())
		})
	}
}

func TestServiceProvider_Boot(t *testing.T) {
	tests := []struct {
		name         string
		artisan      contractsconsole.Artisan
		hasRoute     bool
		hasSession   bool
		expectOutput []string
	}{
		{
			name:       "all facades are set",
			artisan:    mocksconsole.NewArtisan(t),
			hasRoute:   true,
			hasSession: true,
		},
		{
			name: "prints a message for each missing facade",
			expectOutput: []string{
				errors.ConsoleFacadeNotSet.SetModule(errors.ModuleTesting).Error(),
				errors.RouteFacadeNotSet.SetModule(errors.ModuleTesting).Error(),
				errors.SessionFacadeNotSet.SetModule(errors.ModuleTesting).Error(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockApp := mocksfoundation.NewApplication(t)
			mockApp.EXPECT().MakeArtisan().Return(tt.artisan).Once()
			if tt.hasRoute {
				mockApp.EXPECT().MakeRoute().Return(mocksroute.NewRoute(t)).Once()
			} else {
				mockApp.EXPECT().MakeRoute().Return(nil).Once()
			}
			if tt.hasSession {
				mockApp.EXPECT().MakeSession().Return(mockssession.NewManager(t)).Once()
			} else {
				mockApp.EXPECT().MakeSession().Return(nil).Once()
			}
			mockApp.EXPECT().GetJson().Return(nil).Once()

			output := color.CaptureOutput(func(_ io.Writer) {
				(&ServiceProvider{}).Boot(mockApp)
			})

			plain := color.StripColor(output)
			for _, message := range tt.expectOutput {
				assert.True(t, strings.Contains(plain, message), "missing %q in %q", message, plain)
			}
			if len(tt.expectOutput) == 0 {
				assert.Empty(t, plain)
			}
		})
	}
}

func TestServiceProvider_FacadesReturnNilWithoutApplication(t *testing.T) {
	previous := registeredApp
	t.Cleanup(func() {
		registeredApp = previous
	})
	registeredApp = nil

	assert.Nil(t, makeArtisan())
	assert.Nil(t, makeRoute())
	assert.Nil(t, makeSession())
}

func TestServiceProvider_FacadesResolveOnUse(t *testing.T) {
	mockApp := useApplication(t)
	firstArtisan := mocksconsole.NewArtisan(t)
	secondArtisan := mocksconsole.NewArtisan(t)
	firstRoute := mocksroute.NewRoute(t)
	secondRoute := mocksroute.NewRoute(t)
	firstSession := mockssession.NewManager(t)
	secondSession := mockssession.NewManager(t)

	mockApp.EXPECT().MakeArtisan().Return(firstArtisan).Once()
	mockApp.EXPECT().MakeArtisan().Return(secondArtisan).Once()
	mockApp.EXPECT().MakeRoute().Return(firstRoute).Once()
	mockApp.EXPECT().MakeRoute().Return(secondRoute).Once()
	mockApp.EXPECT().MakeSession().Return(firstSession).Once()
	mockApp.EXPECT().MakeSession().Return(secondSession).Once()

	assert.Same(t, firstArtisan, makeArtisan())
	assert.Same(t, secondArtisan, makeArtisan())
	assert.Same(t, firstRoute, makeRoute())
	assert.Same(t, secondRoute, makeRoute())
	assert.Same(t, firstSession, makeSession())
	assert.Same(t, secondSession, makeSession())
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
