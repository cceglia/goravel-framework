package validation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/goravel/framework/contracts/binding"
	contractsconsole "github.com/goravel/framework/contracts/console"
	contractsorm "github.com/goravel/framework/contracts/database/orm"
	mocksorm "github.com/goravel/framework/mocks/database/orm"
	mocksfoundation "github.com/goravel/framework/mocks/foundation"
)

func TestServiceProvider_Register(t *testing.T) {
	previous := registeredApp
	t.Cleanup(func() {
		registeredApp = previous
	})
	registeredApp = nil

	mockApp := mocksfoundation.NewApplication(t)
	mockApp.EXPECT().Singleton(binding.Validation, mock.Anything).Once()

	(&ServiceProvider{}).Register(mockApp)

	assert.Same(t, mockApp, registeredApp)
}

func TestServiceProvider_Boot(t *testing.T) {
	previousLangFacade := langFacade
	t.Cleanup(func() {
		langFacade = previousLangFacade
	})

	mockApp := mocksfoundation.NewApplication(t)
	mockApp.EXPECT().MakeOrm().Return(mocksorm.NewOrm(t)).Once()
	mockApp.EXPECT().LangPath().Return("lang").Once()
	mockApp.EXPECT().Commands(mock.MatchedBy(func(commands []contractsconsole.Command) bool {
		return len(commands) == 3
	})).Once()

	(&ServiceProvider{}).Boot(mockApp)
}

func TestMakeOrm(t *testing.T) {
	t.Run("returns nil without an application", func(t *testing.T) {
		previous := registeredApp
		t.Cleanup(func() {
			registeredApp = previous
		})
		registeredApp = nil

		assert.Nil(t, makeOrm())
	})

	t.Run("resolves the facade on every use", func(t *testing.T) {
		mockApp := useApplication(t)
		first := mocksorm.NewOrm(t)
		second := mocksorm.NewOrm(t)
		mockApp.EXPECT().MakeOrm().Return(first).Once()
		mockApp.EXPECT().MakeOrm().Return(second).Once()

		assert.Same(t, first, makeOrm())
		assert.Same(t, second, makeOrm())
	})
}

func TestDatabaseRulesUseCurrentOrm(t *testing.T) {
	mockApp := useApplication(t)
	firstOrm := mocksorm.NewOrm(t)
	secondOrm := mocksorm.NewOrm(t)
	secondQuery := mocksorm.NewQuery(t)
	mockApp.EXPECT().MakeOrm().Return(firstOrm).Once()
	mockApp.EXPECT().MakeOrm().Return(secondOrm).Once()

	// The first resolution fails the rule before it reaches the ORM.
	assert.False(t, ruleExists(&RuleContext{Ctx: context.Background(), Attribute: "email", Value: "a"}))

	secondOrm.EXPECT().WithContext(mock.Anything).Return(secondOrm).Once()
	secondOrm.EXPECT().Query().Return(secondQuery).Once()
	secondQuery.EXPECT().Table("users").Return(secondQuery).Once()
	secondQuery.EXPECT().Where("email", "a").Return(secondQuery).Once()
	secondQuery.EXPECT().Exists().Return(true, nil).Once()

	assert.True(t, ruleExists(&RuleContext{
		Ctx:        context.Background(),
		Attribute:  "email",
		Value:      "a",
		Parameters: []string{"users"},
	}))
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

func useOrmFacade(t *testing.T, orm contractsorm.Orm) {
	t.Helper()
	useApplication(t).EXPECT().MakeOrm().Return(orm).Maybe()
}
