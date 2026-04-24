package internal

import "time"

type Config struct {
	APIKey     string
	APIURL     string
	DeviceName string
	CLIVersion string

	HeartbeatInterval time.Duration
	RegisterTimeout   time.Duration
}

func (c *Config) withDefaults() {
	if c.HeartbeatInterval == 0 {
		c.HeartbeatInterval = 15 * time.Second
	}
	if c.RegisterTimeout == 0 {
		c.RegisterTimeout = 30 * time.Second
	}
}
