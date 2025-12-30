package server

import (
	"testing"
	"time"
)

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				Port:           "8080",
				ReadTimeout:    10 * time.Second,
				WriteTimeout:   10 * time.Second,
				IdleTimeout:    60 * time.Second,
				MaxHeaderBytes: 1 << 20,
			},
			wantErr: false,
		},
		{
			name: "valid low port",
			config: &Config{
				Port: "1",
			},
			wantErr: false,
		},
		{
			name: "valid high port",
			config: &Config{
				Port: "65535",
			},
			wantErr: false,
		},
		{
			name: "invalid - non-numeric",
			config: &Config{
				Port: "abc",
			},
			wantErr: true,
		},
		{
			name: "invalid - port too low",
			config: &Config{
				Port: "0",
			},
			wantErr: true,
		},
		{
			name: "invalid - port too high",
			config: &Config{
				Port: "65536",
			},
			wantErr: true,
		},
		{
			name: "invalid - negative port",
			config: &Config{
				Port: "-1",
			},
			wantErr: true,
		},
		{
			name: "invalid - empty port",
			config: &Config{
				Port: "",
			},
			wantErr: true,
		},
		{
			name: "invalid - port with spaces",
			config: &Config{
				Port: "80 80",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
