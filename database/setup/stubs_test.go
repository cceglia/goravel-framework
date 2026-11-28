package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStubs_Config(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		expected    Config
		expectedPre string
	}{
		{
			name: "declares the prepared statement switch after the pool",
			key:  "prepare_stmt",
			expected: Config{
				Key:   "prepare_stmt",
				Value: `config.Env("DB_PREPARE_STMT", false)`,
				Annotations: []string{
					"Caches prepared statements per connection. Disable it when the driver or a proxy in front of the database does not support prepared statements.",
				},
			},
			expectedPre: "pool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configs := Stubs{}.Config()

			var got Config
			var gotPre string
			for i, config := range configs {
				if config.Key == tt.key {
					got = config
					if i > 0 {
						gotPre = configs[i-1].Key
					}
				}
			}

			assert.Equal(t, tt.expected, got)
			assert.Equal(t, tt.expectedPre, gotPre)
		})
	}
}
