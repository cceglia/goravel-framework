package tests

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/goravel/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormio "gorm.io/gorm"

	"github.com/goravel/framework/auth"
	"github.com/goravel/framework/contracts/config"
	"github.com/goravel/framework/contracts/database/driver"
	"github.com/goravel/framework/contracts/database/schema"
	"github.com/goravel/framework/contracts/database/seeder"
	contractsfoundation "github.com/goravel/framework/contracts/foundation"
	"github.com/goravel/framework/contracts/testing/docker"
	"github.com/goravel/framework/database"
	databasedriver "github.com/goravel/framework/database/driver"
	"github.com/goravel/framework/foundation"
	"github.com/goravel/framework/http"
	"github.com/goravel/framework/log"
	"github.com/goravel/framework/support"
	fwtesting "github.com/goravel/framework/testing"
	"github.com/goravel/framework/validation"
)

const helperConnection = "helper"

type helperUser struct {
	ID    uint `gorm:"primaryKey"`
	Name  string
	Email string
}

func (helperUser) TableName() string {
	return "users"
}

// helperMigration creates the tables that the helper's migrate step must create in the new database only.
type helperMigration struct{}

func (helperMigration) Signature() string {
	return "20260101000000_create_users_and_widgets_tables"
}

func (helperMigration) Up() error {
	schemaFacade := foundation.App.MakeSchema()
	if err := schemaFacade.Create("users", func(table schema.Blueprint) {
		table.ID()
		table.String("name")
		table.String("email")
	}); err != nil {
		return err
	}

	return schemaFacade.Create("widgets", func(table schema.Blueprint) {
		table.ID()
		table.String("name")
	})
}

func (helperMigration) Down() error {
	schemaFacade := foundation.App.MakeSchema()
	if err := schemaFacade.DropIfExists("widgets"); err != nil {
		return err
	}

	return schemaFacade.DropIfExists("users")
}

// helperSeeder resolves the ORM when it runs, after the helper's ready step. A seeder that keeps an
// ORM or a query builder it resolved before the ready step holds a closed pool afterwards: it must
// resolve them in Run, as this seeder does, or be built after the ready step.
type helperSeeder struct{}

func (helperSeeder) Signature() string {
	return "helper_seeder"
}

func (helperSeeder) Run() error {
	return foundation.App.MakeOrm().Query().Table("users").Create(map[string]any{
		"id":    1,
		"name":  "new-user",
		"email": "new@example.com",
	})
}

// helperDriver wraps the SQLite driver, and makes its Docker helper change the configured database
// as the container based drivers do when they report ready.
type helperDriver struct {
	driver.Driver
	config config.Config
	target string
}

func (r helperDriver) Docker() (docker.DatabaseDriver, error) {
	return &helperDocker{
		DatabaseDriver: sqlite.NewDocker(r.target),
		config:         r.config,
		target:         r.target,
	}, nil
}

type helperDocker struct {
	docker.DatabaseDriver
	config config.Config
	target string
}

func (r *helperDocker) Ready() error {
	if err := r.DatabaseDriver.Ready(); err != nil {
		return err
	}

	r.config.Add("database.connections."+helperConnection+".database", r.target)

	return nil
}

func TestDatabaseHelper_SwitchesDatabaseEndToEnd(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.db")
	newPath := filepath.Join(dir, "new.db")

	dontVerifyAppKey := support.DontVerifyAppKey
	support.DontVerifyAppKey = true
	t.Cleanup(func() {
		support.DontVerifyAppKey = dontVerifyAppKey
		databasedriver.CloseConnections()
	})

	app := foundation.Setup().
		WithProviders(func() []contractsfoundation.ServiceProvider {
			return []contractsfoundation.ServiceProvider{
				&log.ServiceProvider{},
				&database.ServiceProvider{},
				&auth.ServiceProvider{},
				&validation.ServiceProvider{},
				&fwtesting.ServiceProvider{},
			}
		}).
		WithConfig(func() {
			configFacade := foundation.App.MakeConfig()
			configFacade.Add("database", map[string]any{
				"default": helperConnection,
				"migrations": map[string]any{
					"table": "migrations",
				},
				"connections": map[string]any{
					helperConnection: map[string]any{
						"driver":   sqlite.Name,
						"database": oldPath,
						"via": func() (driver.Driver, error) {
							return helperDriver{
								Driver: sqlite.NewSqlite(configFacade, foundation.App.MakeLog(), helperConnection),
								config: configFacade,
								target: newPath,
							}, nil
						},
					},
				},
			})
		}).
		WithMigrations(func() []schema.Migration {
			return []schema.Migration{helperMigration{}}
		}).
		WithSeeders(func() []seeder.Seeder {
			return []seeder.Seeder{helperSeeder{}}
		}).
		Create()

	// The old database holds two users and a table the new database never gets.
	schemaBefore := app.MakeSchema()
	ormBefore := app.MakeOrm()
	require.NoError(t, schemaBefore.Create("users", func(table schema.Blueprint) {
		table.ID()
		table.String("name")
		table.String("email")
	}))
	require.NoError(t, schemaBefore.Create("old_only", func(table schema.Blueprint) {
		table.ID()
	}))
	require.NoError(t, ormBefore.Query().Table("users").Create([]map[string]any{
		{"id": 1, "name": "old-user", "email": "old@example.com"},
		{"id": 2, "name": "old-only-user", "email": "old-only@example.com"},
	}))

	user := &helperUser{}
	provider, err := auth.NewOrmUserProvider(http.Background())
	require.NoError(t, err)
	require.NoError(t, provider.RetriveByID(user, 1))
	assert.Equal(t, "old-user", user.Name)
	assert.True(t, validationPasses(t, app, "old@example.com", "exists:users,email"))

	// Change the database through the helper, and run its ready, migrate, and seed steps.
	helper, err := app.MakeTesting().Docker().Database()
	require.NoError(t, err)
	require.NoError(t, helper.Ready())
	require.NoError(t, helper.Migrate())
	require.NoError(t, helper.Seed(helperSeeder{}))

	assert.NotSame(t, ormBefore, app.MakeOrm())

	// Auth looks users up in the new database.
	user = &helperUser{}
	provider, err = auth.NewOrmUserProvider(http.Background())
	require.NoError(t, err)
	require.NoError(t, provider.RetriveByID(user, 1))
	assert.Equal(t, helperUser{ID: 1, Name: "new-user", Email: "new@example.com"}, *user)
	assert.Error(t, provider.RetriveByID(&helperUser{}, 2))

	// The exists and unique rules query the new database.
	assert.True(t, validationPasses(t, app, "new@example.com", "exists:users,email"))
	assert.False(t, validationPasses(t, app, "old@example.com", "exists:users,email"))
	assert.False(t, validationPasses(t, app, "new@example.com", "unique:users,email"))
	assert.True(t, validationPasses(t, app, "old@example.com", "unique:users,email"))

	// The schema that existed before the switch operates on the new database, and the migrate step ran there.
	assert.True(t, schemaBefore.HasTable("widgets"))
	assert.True(t, schemaBefore.HasTable("migrations"))
	assert.False(t, schemaBefore.HasTable("old_only"))
	assert.True(t, app.MakeSchema().HasTable("widgets"))

	// The old database stays as it was.
	oldDB, err := gormio.Open(sqlite.Open(oldPath))
	require.NoError(t, err)
	t.Cleanup(func() {
		sqlDB, err := oldDB.DB()
		if assert.NoError(t, err) {
			assert.NoError(t, sqlDB.Close())
		}
	})
	assert.True(t, sqliteHasTable(t, oldDB, "old_only"))
	assert.False(t, sqliteHasTable(t, oldDB, "widgets"))
	assert.False(t, sqliteHasTable(t, oldDB, "migrations"))

	var oldUsers []helperUser
	require.NoError(t, oldDB.Order("id").Find(&oldUsers).Error)
	assert.Equal(t, []helperUser{
		{ID: 1, Name: "old-user", Email: "old@example.com"},
		{ID: 2, Name: "old-only-user", Email: "old-only@example.com"},
	}, oldUsers)
}

func validationPasses(t *testing.T, app contractsfoundation.Application, email, rule string) bool {
	t.Helper()

	validator, err := app.MakeValidation().Make(context.Background(), map[string]any{"email": email}, map[string]any{"email": rule})
	require.NoError(t, err)

	return !validator.Fails()
}

func sqliteHasTable(t *testing.T, db *gormio.DB, name string) bool {
	t.Helper()

	var count int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", name).Scan(&count).Error)

	return count > 0
}
