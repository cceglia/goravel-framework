package orm

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	gormtests "gorm.io/gorm/utils/tests"

	"github.com/goravel/framework/contracts/binding"
	"github.com/goravel/framework/contracts/database"
	databasedriver "github.com/goravel/framework/database/driver"
	mocksconfig "github.com/goravel/framework/mocks/config"
)

func TestOrm_Fresh(t *testing.T) {
	tests := []struct {
		name         string
		fresh        func(t *testing.T, events *[]string, pool **sql.DB) func(key ...any)
		expectEvents []string
	}{
		{
			name: "closes the pools before the callback receives the Orm and DB keys",
			fresh: func(t *testing.T, events *[]string, pool **sql.DB) func(key ...any) {
				return func(key ...any) {
					*events = append(*events, "fresh")
					assert.Equal(t, []any{binding.Orm, binding.DB}, key)
					assert.ErrorContains(t, (*pool).Ping(), "database is closed")
				}
			},
			expectEvents: []string{"fresh"},
		},
		{
			name: "a plain drop callback keeps its behavior apart from the close",
			fresh: func(t *testing.T, events *[]string, pool **sql.DB) func(key ...any) {
				dropped := map[any]bool{binding.Orm: true, binding.DB: true, "other": true}

				return func(key ...any) {
					for _, k := range key {
						delete(dropped, k)
					}
					assert.Equal(t, map[any]bool{"other": true}, dropped)
					*events = append(*events, "drop")
				}
			},
			expectEvents: []string{"drop"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(databasedriver.CloseConnections)

			instance, _, err := databasedriver.BuildGorm(stubGormConfig(t), gormlogger.Discard, stubPool(), "primary", nil)
			require.NoError(t, err)
			sqlDB, err := instance.DB()
			require.NoError(t, err)

			var events []string
			orm := NewOrm(context.Background(), nil, "primary", database.Config{}, nil, NewQueriesCache(nil, nil), nil, nil, tt.fresh(t, &events, &sqlDB), nil)

			orm.Fresh()

			assert.Equal(t, tt.expectEvents, events)
			assert.ErrorContains(t, sqlDB.Ping(), "database is closed")

			// The next build creates a new connection instead of returning the closed one.
			second, _, err := databasedriver.BuildGorm(stubGormConfig(t), gormlogger.Discard, stubPool(), "primary", nil)
			require.NoError(t, err)
			assert.NotSame(t, instance, second)
		})
	}
}

type stubConnector struct{}

func (stubConnector) Connect(context.Context) (driver.Conn, error) { return nil, driver.ErrBadConn }
func (stubConnector) Driver() driver.Driver                        { return stubDriver{} }

type stubDriver struct{}

func (stubDriver) Open(string) (driver.Conn, error) { return nil, driver.ErrBadConn }

type stubDialector struct {
	gormtests.DummyDialector
}

func (stubDialector) Initialize(db *gorm.DB) error {
	if err := (gormtests.DummyDialector{}).Initialize(db); err != nil {
		return err
	}

	db.ConnPool = sql.OpenDB(stubConnector{})

	return nil
}

func stubPool() database.Pool {
	return database.Pool{
		Writers: []database.Config{
			{Driver: "postgres", Database: "app", Dialector: stubDialector{}},
		},
	}
}

func stubGormConfig(t *testing.T) *mocksconfig.Config {
	mockConfig := mocksconfig.NewConfig(t)
	mockConfig.EXPECT().GetInt("database.pool.max_idle_conns", 10).Return(10).Once()
	mockConfig.EXPECT().GetInt("database.pool.max_open_conns", 100).Return(100).Once()
	mockConfig.EXPECT().GetDuration("database.pool.conn_max_idletime", time.Duration(3600)).Return(time.Duration(3600)).Once()
	mockConfig.EXPECT().GetDuration("database.pool.conn_max_lifetime", time.Duration(3600)).Return(time.Duration(3600)).Once()

	return mockConfig
}
