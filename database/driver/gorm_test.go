package driver

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

	"github.com/goravel/framework/contracts/database"
	contractstelemetry "github.com/goravel/framework/contracts/telemetry"
	mocksconfig "github.com/goravel/framework/mocks/config"
	instrumentationdatabase "github.com/goravel/framework/telemetry/instrumentation/database"
)

func TestBuildGorm_TelemetryPlugin(t *testing.T) {
	t.Cleanup(ResetConnections)

	resolver := func() contractstelemetry.Telemetry { return nil }
	instance, _, err := BuildGorm(stubGormConfig(t), gormlogger.Discard, stubPool(), "primary", resolver)
	assert.NoError(t, err)
	assert.NotNil(t, instance)
	t.Cleanup(func() {
		if db, err := instance.DB(); err == nil {
			_ = db.Close()
		}
	})

	_, registered := instance.Plugins[instrumentationdatabase.PluginName]
	assert.True(t, registered)
}

func TestBuildGorm_PrepareStmt(t *testing.T) {
	t.Cleanup(CloseConnections)

	tests := []struct {
		name             string
		connection       string
		prepareStmt      bool
		expectedConnPool gorm.ConnPool
	}{
		{
			name:             "wraps the pool when enabled",
			connection:       "prepare_stmt_enabled",
			prepareStmt:      true,
			expectedConnPool: &gorm.PreparedStmtDB{},
		},
		{
			name:             "keeps the raw pool when disabled",
			connection:       "prepare_stmt_disabled",
			prepareStmt:      false,
			expectedConnPool: &sql.DB{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := mocksconfig.NewConfig(t)
			mockConfig.EXPECT().GetBool("database.prepare_stmt", false).Return(tt.prepareStmt).Once()
			mockConfig.EXPECT().GetInt("database.pool.max_idle_conns", 10).Return(10).Once()
			mockConfig.EXPECT().GetInt("database.pool.max_open_conns", 100).Return(100).Once()
			mockConfig.EXPECT().GetDuration("database.pool.conn_max_idletime", time.Duration(3600)).Return(time.Duration(3600)).Once()
			mockConfig.EXPECT().GetDuration("database.pool.conn_max_lifetime", time.Duration(3600)).Return(time.Duration(3600)).Once()

			instance, _, err := BuildGorm(mockConfig, gormlogger.Discard, stubPool(), tt.connection, nil)
			require.NoError(t, err)

			assert.Equal(t, tt.prepareStmt, instance.PrepareStmt)
			assert.IsType(t, tt.expectedConnPool, instance.ConnPool)
		})
	}
}

func TestCloseConnections_ClosesPool(t *testing.T) {
	t.Cleanup(CloseConnections)

	resolver := func() contractstelemetry.Telemetry { return nil }
	instance, _, err := BuildGorm(stubGormConfig(t), gormlogger.Discard, stubPool(), "primary", resolver)
	assert.NoError(t, err)
	sqlDB, err := instance.DB()
	assert.NoError(t, err)

	CloseConnections()

	assert.ErrorContains(t, sqlDB.Ping(), "database is closed")
}

func TestResetConnections_KeepsPoolOpen(t *testing.T) {
	t.Cleanup(CloseConnections)

	resolver := func() contractstelemetry.Telemetry { return nil }
	first, _, err := BuildGorm(stubGormConfig(t), gormlogger.Discard, stubPool(), "primary", resolver)
	assert.NoError(t, err)
	sqlDB, err := first.DB()
	assert.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	ResetConnections()

	err = sqlDB.Ping()
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "database is closed")

	second, _, err := BuildGorm(stubGormConfig(t), gormlogger.Discard, stubPool(), "primary", resolver)
	assert.NoError(t, err)
	assert.NotSame(t, first, second)
}

func TestResetConnections_NilInstrument(t *testing.T) {
	t.Cleanup(CloseConnections)

	config := mocksconfig.NewConfig(t)
	config.EXPECT().GetBool("database.prepare_stmt", false).Return(false).Once()
	config.EXPECT().GetInt("database.pool.max_idle_conns", 10).Return(10).Once()
	config.EXPECT().GetInt("database.pool.max_open_conns", 100).Return(100).Once()
	config.EXPECT().GetDuration("database.pool.conn_max_idletime", time.Duration(3600)).Return(time.Duration(3600)).Once()
	config.EXPECT().GetDuration("database.pool.conn_max_lifetime", time.Duration(3600)).Return(time.Duration(3600)).Once()

	_, instrument, err := BuildGorm(config, gormlogger.Discard, stubPool(), "primary", nil)
	assert.NoError(t, err)
	assert.Nil(t, instrument)

	assert.NotPanics(t, ResetConnections)
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
	mockConfig.EXPECT().GetBool("telemetry.instrumentation.database.enabled", true).Return(true).Once()
	mockConfig.EXPECT().GetBool("database.prepare_stmt", false).Return(false).Once()
	mockConfig.EXPECT().GetInt("database.pool.max_idle_conns", 10).Return(10).Once()
	mockConfig.EXPECT().GetInt("database.pool.max_open_conns", 100).Return(100).Once()
	mockConfig.EXPECT().GetDuration("database.pool.conn_max_idletime", time.Duration(3600)).Return(time.Duration(3600)).Once()
	mockConfig.EXPECT().GetDuration("database.pool.conn_max_lifetime", time.Duration(3600)).Return(time.Duration(3600)).Once()

	return mockConfig
}
