package database

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

	contractsdatabase "github.com/goravel/framework/contracts/database"
	contractsfoundation "github.com/goravel/framework/contracts/foundation"
	databasedriver "github.com/goravel/framework/database/driver"
	mocksconfig "github.com/goravel/framework/mocks/config"
	mocksfoundation "github.com/goravel/framework/mocks/foundation"
)

var _ contractsfoundation.ServiceProviderWithFlushState = (*ServiceProvider)(nil)

func TestFlushState(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T) (assertAfter func())
	}{
		{
			name: "closes the cached connections and builds a new one afterwards",
			setup: func(t *testing.T) func() {
				first, sqlDB := buildFlushGorm(t)

				return func() {
					assert.ErrorContains(t, sqlDB.Ping(), "database is closed")

					second, _ := buildFlushGorm(t)
					assert.NotSame(t, first, second)
				}
			},
		},
		{
			name:  "is safe on empty state",
			setup: func(*testing.T) func() { return func() {} },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			databasedriver.CloseConnections()
			t.Cleanup(databasedriver.CloseConnections)

			assertAfter := tt.setup(t)
			provider := &ServiceProvider{}
			app := mocksfoundation.NewApplication(t)

			assert.NotPanics(t, func() {
				provider.FlushState(app)
				provider.FlushState(app)
			})
			assertAfter()
		})
	}
}

func buildFlushGorm(t *testing.T) (*gorm.DB, *sql.DB) {
	t.Helper()

	config := mocksconfig.NewConfig(t)
	config.EXPECT().GetInt("database.pool.max_idle_conns", 10).Return(10).Once()
	config.EXPECT().GetInt("database.pool.max_open_conns", 100).Return(100).Once()
	config.EXPECT().GetDuration("database.pool.conn_max_idletime", time.Duration(3600)).Return(time.Duration(3600)).Once()
	config.EXPECT().GetDuration("database.pool.conn_max_lifetime", time.Duration(3600)).Return(time.Duration(3600)).Once()

	pool := contractsdatabase.Pool{
		Writers: []contractsdatabase.Config{
			{Driver: "postgres", Database: "app", Dialector: flushDialector{}},
		},
	}
	instance, _, err := databasedriver.BuildGorm(config, gormlogger.Discard, pool, "flush", nil)
	require.NoError(t, err)
	sqlDB, err := instance.DB()
	require.NoError(t, err)

	return instance, sqlDB
}

type flushConnector struct{}

func (flushConnector) Connect(context.Context) (driver.Conn, error) { return nil, driver.ErrBadConn }
func (flushConnector) Driver() driver.Driver                        { return flushDriver{} }

type flushDriver struct{}

func (flushDriver) Open(string) (driver.Conn, error) { return nil, driver.ErrBadConn }

type flushDialector struct {
	gormtests.DummyDialector
}

func (flushDialector) Initialize(db *gorm.DB) error {
	if err := (gormtests.DummyDialector{}).Initialize(db); err != nil {
		return err
	}

	db.ConnPool = sql.OpenDB(flushConnector{})

	return nil
}
