package queue

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/goravel/framework/contracts/binding"
	contractsconsole "github.com/goravel/framework/contracts/console"
	"github.com/goravel/framework/contracts/foundation"
	mocksconfig "github.com/goravel/framework/mocks/config"
	mocksdb "github.com/goravel/framework/mocks/database/db"
	mocksfoundation "github.com/goravel/framework/mocks/foundation"
	mockslog "github.com/goravel/framework/mocks/log"
	mocksqueue "github.com/goravel/framework/mocks/queue"
)

func TestServiceProviderRegister_ResolvesDBOnUse(t *testing.T) {
	provider := &ServiceProvider{}
	app := mocksfoundation.NewApplication(t)

	var factory func(foundation.Application) (any, error)
	app.EXPECT().Singleton(binding.Queue, mock.Anything).Run(func(_ any, callback func(foundation.Application) (any, error)) {
		factory = callback
	}).Once()
	provider.Register(app)
	require.NotNil(t, factory)

	config := mocksconfig.NewConfig(t)
	config.EXPECT().GetString("queue.default").Return("database").Once()
	config.EXPECT().GetString("queue.connections.database.queue", "default").Return("default").Once()
	config.EXPECT().GetInt("queue.connections.database.concurrent", 1).Return(1).Once()
	config.EXPECT().GetString("app.name", "goravel").Return("goravel").Once()
	config.EXPECT().GetBool("app.debug").Return(false).Once()
	config.EXPECT().GetString("queue.failed.database").Return("").Once()
	config.EXPECT().GetString("queue.failed.table").Return("").Once()

	builtDB := mocksdb.NewDB(t)
	resolvedDB := mocksdb.NewDB(t)
	app.EXPECT().MakeConfig().Return(config).Once()
	app.EXPECT().MakeLog().Return(mockslog.NewLog(t)).Once()
	app.EXPECT().MakeCache().Return(nil).Once()
	app.EXPECT().MakeDB().Return(builtDB).Once()
	app.EXPECT().Json().Return(nil).Once()

	instance, err := factory(app)
	require.NoError(t, err)
	queue, ok := instance.(*Application)
	require.True(t, ok)

	app.EXPECT().MakeDB().Return(resolvedDB).Once()

	assert.Same(t, resolvedDB, pendingDB(t, queue.Job(&TestJobOne{}).(*PendingJob)))
}

func TestServiceProviderBoot(t *testing.T) {
	provider := &ServiceProvider{}
	app := mocksfoundation.NewApplication(t)
	mockQueue := mocksqueue.NewQueue(t)

	app.EXPECT().MakeQueue().Return(mockQueue).Twice()
	app.EXPECT().GetJson().Return(nil).Once()
	app.EXPECT().Commands(mock.MatchedBy(func(commands []contractsconsole.Command) bool {
		if len(commands) != 3 {
			return false
		}

		return commands[0].Signature() == "make:job" &&
			commands[1].Signature() == "queue:retry" &&
			commands[2].Signature() == "queue:failed"
	})).Once()

	provider.Boot(app)
}
