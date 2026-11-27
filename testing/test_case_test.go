package testing

import (
	"io"
	nethttp "net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	mocksconsole "github.com/goravel/framework/mocks/console"
	mocksroute "github.com/goravel/framework/mocks/route"
	mockssession "github.com/goravel/framework/mocks/session"
)

type TestCaseSuite struct {
	suite.Suite
	mockArtisan *mocksconsole.Artisan
	testCase    *TestCase
}

func TestTestCaseSuite(t *testing.T) {
	suite.Run(t, new(TestCaseSuite))
}

// SetupTest will run before each test in the suite.
func (s *TestCaseSuite) SetupTest() {
	s.mockArtisan = mocksconsole.NewArtisan(s.T())
	s.testCase = &TestCase{}
	useApplication(s.T()).EXPECT().MakeArtisan().Return(s.mockArtisan).Maybe()
}

func (s *TestCaseSuite) TestHttp() {
	mockRoute := mocksroute.NewRoute(s.T())
	mockSession := mockssession.NewManager(s.T())
	mockApp := useApplication(s.T())
	mockApp.EXPECT().MakeRoute().Return(mockRoute).Once()
	mockApp.EXPECT().MakeSession().Return(mockSession).Once()

	var gotRequest *nethttp.Request
	mockRoute.EXPECT().Test(mock.Anything).RunAndReturn(func(request *nethttp.Request) (*nethttp.Response, error) {
		gotRequest = request

		return &nethttp.Response{StatusCode: nethttp.StatusOK, Body: io.NopCloser(strings.NewReader("pong"))}, nil
	}).Once()

	response, err := s.testCase.Http(s.T()).Get("/ping")

	s.Require().NoError(err)
	s.Equal(nethttp.MethodGet, gotRequest.Method)
	s.Equal("/ping", gotRequest.URL.Path)
	content, err := response.Content()
	s.Require().NoError(err)
	s.Equal("pong", content)
}

func (s *TestCaseSuite) TestSeed() {
	s.mockArtisan.On("Call", "--no-ansi db:seed").Return(nil).Once()
	s.testCase.Seed()

	s.mockArtisan.On("Call", "--no-ansi db:seed --seeder mock").Return(nil).Once()
	s.testCase.Seed(&MockSeeder{})

	s.Panics(func() {
		s.mockArtisan.On("Call", "--no-ansi db:seed").Return(assert.AnError).Once()
		s.testCase.Seed()
	})
}

func (s *TestCaseSuite) TestRefreshDatabase() {
	s.mockArtisan.On("Call", "--no-ansi migrate:refresh").Return(nil).Once()
	s.testCase.RefreshDatabase()

	s.Panics(func() {
		s.mockArtisan.On("Call", "--no-ansi migrate:refresh").Return(assert.AnError).Once()
		s.testCase.RefreshDatabase()
	})
}

type MockSeeder struct{}

func (m *MockSeeder) Signature() string {
	return "mock"
}

func (m *MockSeeder) Run() error {
	return nil
}
