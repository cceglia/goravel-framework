package queue

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	contractsdb "github.com/goravel/framework/contracts/database/db"
	contractsqueue "github.com/goravel/framework/contracts/queue"
	"github.com/goravel/framework/errors"
	mocksdb "github.com/goravel/framework/mocks/database/db"
	mocksqueue "github.com/goravel/framework/mocks/queue"
)

func newResolverTestConfig(t *testing.T) *mocksqueue.Config {
	config := mocksqueue.NewConfig(t)
	config.EXPECT().DefaultConnection().Return("database").Maybe()
	config.EXPECT().DefaultQueue().Return("default").Maybe()
	config.EXPECT().Driver("database").Return(contractsqueue.DriverDatabase).Maybe()
	config.EXPECT().GetString("queue.connections.database.connection").Return("main").Maybe()
	config.EXPECT().GetString("queue.connections.database.table", "jobs").Return("jobs").Maybe()
	config.EXPECT().GetInt("queue.connections.database.retry_after", 60).Return(60).Maybe()

	return config
}

func pendingDB(t *testing.T, job *PendingJob) contractsdb.DB {
	creator, ok := job.driverCreator.(*DriverCreator)
	require.True(t, ok)

	return creator.db
}

func TestWithDBResolver_ResolvesOnUse(t *testing.T) {
	var current contractsdb.DB
	app := NewApplication(newResolverTestConfig(t), nil, nil, NewJobStorer(), nil, nil, WithDBResolver(func() contractsdb.DB {
		return current
	}))

	// A query builder that was nil at build time stays nil until the resolver returns one.
	_, err := app.Connection("database")
	assert.Equal(t, errors.QueueInvalidDatabaseConnection.Args("database"), err)
	assert.Nil(t, pendingDB(t, app.Job(&TestJobOne{}).(*PendingJob)))

	first := mocksdb.NewDB(t)
	first.EXPECT().Connection("main").Return(mocksdb.NewDB(t)).Once()
	current = first

	driver, err := app.Connection("database")
	require.NoError(t, err)
	assert.Equal(t, contractsqueue.DriverDatabase, driver.Driver())
	assert.Same(t, first, pendingDB(t, app.Job(&TestJobOne{}).(*PendingJob)))
	assert.Same(t, first, pendingDB(t, app.Chain([]contractsqueue.ChainJob{{Job: &TestJobOne{}}}).(*PendingJob)))

	second := mocksdb.NewDB(t)
	current = second

	assert.Same(t, second, pendingDB(t, app.Job(&TestJobOne{}).(*PendingJob)))
	assert.Same(t, second, pendingDB(t, app.Chain([]contractsqueue.ChainJob{{Job: &TestJobOne{}}}).(*PendingJob)))
}

func TestWithDBResolver_NilKeepsBuildTimeDB(t *testing.T) {
	built := mocksdb.NewDB(t)
	app := NewApplication(newResolverTestConfig(t), nil, built, NewJobStorer(), nil, nil, WithDBResolver(nil))

	assert.Same(t, built, pendingDB(t, app.Job(&TestJobOne{}).(*PendingJob)))
}

func TestApplication_WithoutOptionUsesBuildTimeDB(t *testing.T) {
	app := NewApplication(newResolverTestConfig(t), nil, nil, NewJobStorer(), nil, nil)

	assert.Nil(t, pendingDB(t, app.Job(&TestJobOne{}).(*PendingJob)))

	built := mocksdb.NewDB(t)
	app = NewApplication(newResolverTestConfig(t), nil, built, NewJobStorer(), nil, nil)

	assert.Same(t, built, pendingDB(t, app.Job(&TestJobOne{}).(*PendingJob)))
}
