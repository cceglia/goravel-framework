package queue

import (
	"testing"

	"github.com/stretchr/testify/suite"

	contractsqueue "github.com/goravel/framework/contracts/queue"
	mocksdb "github.com/goravel/framework/mocks/database/db"
	mocksqueue "github.com/goravel/framework/mocks/queue"
)

type ApplicationTestSuite struct {
	suite.Suite
	mockConfig *mocksqueue.Config
}

func TestApplicationTestSuite(t *testing.T) {
	suite.Run(t, new(ApplicationTestSuite))
}

func (s *ApplicationTestSuite) SetupTest() {
	s.mockConfig = mocksqueue.NewConfig(s.T())
}

func (s *ApplicationTestSuite) TestWorkerUsesConfiguredQueueListForDefaultConnection() {
	s.mockConfig.EXPECT().DefaultConnection().Return("sync").Once()
	s.mockConfig.EXPECT().DefaultConcurrent().Return(2).Once()
	s.mockConfig.EXPECT().GetString("queue.connections.sync.queue", "default").Return("high,default").Once()
	s.mockConfig.EXPECT().Driver("sync").Return(contractsqueue.DriverSync).Once()
	s.mockConfig.EXPECT().Timeout("sync").Return(defaultReceiveTimeout).Once()
	s.mockConfig.EXPECT().Debug().Return(false).Once()

	worker := NewApplication(s.mockConfig, nil, nil, nil, nil, nil).Worker().(*Worker)

	s.Equal("high,default", worker.queue)
	s.Equal(2, worker.concurrent)
}

func (s *ApplicationTestSuite) TestWorkerUsesConfiguredQueueListForSelectedConnection() {
	s.mockConfig.EXPECT().DefaultConnection().Return("sync").Once()
	s.mockConfig.EXPECT().DefaultConcurrent().Return(1).Once()
	s.mockConfig.EXPECT().GetString("queue.connections.redis.queue", "default").Return("high,default").Once()
	s.mockConfig.EXPECT().GetInt("queue.connections.redis.concurrent", 1).Return(3).Once()
	s.mockConfig.EXPECT().Driver("redis").Return(contractsqueue.DriverSync).Once()
	s.mockConfig.EXPECT().Timeout("redis").Return(defaultReceiveTimeout).Once()
	s.mockConfig.EXPECT().Debug().Return(false).Once()

	worker := NewApplication(s.mockConfig, nil, nil, nil, nil, nil).Worker(contractsqueue.Args{
		Connection: "redis",
	}).(*Worker)

	s.Equal("high,default", worker.queue)
	s.Equal(3, worker.concurrent)
}

func (s *ApplicationTestSuite) TestFailerReadsFailedJobsFromConfiguredTable() {
	mockDB := mocksdb.NewDB(s.T())
	mockQuery := mocksdb.NewQuery(s.T())
	s.mockConfig.EXPECT().GetString("queue.failed.database").Return("mysql").Once()
	s.mockConfig.EXPECT().GetString("queue.failed.table").Return("failed_jobs").Once()
	mockDB.EXPECT().Connection("mysql").Return(mockDB).Once()
	mockDB.EXPECT().Table("failed_jobs").Return(mockQuery).Once()

	app := NewApplication(s.mockConfig, nil, mockDB, nil, nil, nil)
	failer, ok := app.Failer().(*Failer)

	s.Require().True(ok)
	s.Same(mockQuery, failer.query)
	s.Same(app, failer.queue)
}
