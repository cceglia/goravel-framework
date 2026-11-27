package mail

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	contractsfoundation "github.com/goravel/framework/contracts/foundation"
	"github.com/goravel/framework/mail/template"
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
			name: "builds the engine again from the current configuration",
			setup: func(t *testing.T) func() {
				first := mocksconfig.NewConfig(t)
				first.EXPECT().GetString("mail.template.default", "html").Return("html").Once()
				first.EXPECT().GetString("mail.template.engines.html.driver", "html").Return("html").Once()
				first.EXPECT().GetString("mail.template.engines.html.path", "resources/views/mail").Return("/first").Once()
				engine, err := template.Get(first)
				require.NoError(t, err)

				return func() {
					second := mocksconfig.NewConfig(t)
					second.EXPECT().GetString("mail.template.default", "html").Return("html").Once()
					second.EXPECT().GetString("mail.template.engines.html.driver", "html").Return("html").Once()
					second.EXPECT().GetString("mail.template.engines.html.path", "resources/views/mail").Return("/second").Once()
					rebuilt, rebuiltErr := template.Get(second)
					require.NoError(t, rebuiltErr)
					assert.NotSame(t, engine, rebuilt)
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
			template.FlushEngines()
			t.Cleanup(template.FlushEngines)

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
