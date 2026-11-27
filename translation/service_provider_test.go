package translation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	contractsfoundation "github.com/goravel/framework/contracts/foundation"
	mocksfoundation "github.com/goravel/framework/mocks/foundation"
	mockstranslation "github.com/goravel/framework/mocks/translation"
)

var _ contractsfoundation.ServiceProviderWithFlushState = (*ServiceProvider)(nil)

func TestFlushState(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T) (assertAfter func())
	}{
		{
			name: "discards the loaded groups so a changed loader is used",
			setup: func(t *testing.T) func() {
				oldLoader := mockstranslation.NewLoader(t)
				oldLoader.EXPECT().Load("en", "test").Return(map[string]any{"key": "old"}, nil).Once()
				lines, err := NewTranslator(t.Context(), nil, oldLoader, "en", "en", nil).load("en", "test")
				require.NoError(t, err)
				assert.Equal(t, map[string]any{"key": "old"}, lines)

				return func() {
					newLoader := mockstranslation.NewLoader(t)
					newLoader.EXPECT().Load("en", "test").Return(map[string]any{"key": "new"}, nil).Once()
					reloaded, reloadErr := NewTranslator(t.Context(), nil, newLoader, "en", "en", nil).load("en", "test")
					require.NoError(t, reloadErr)
					assert.Equal(t, map[string]any{"key": "new"}, reloaded)
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
			loaded.Clear()
			t.Cleanup(loaded.Clear)

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
