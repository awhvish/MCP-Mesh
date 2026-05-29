package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type MQTTConfig struct {
	Host      string `yaml:"host"`
	Port      int    `yaml:"port"`
	Username  string `yaml:"username"`
	Password  string `yaml:"password"`
	TLS       bool   `yaml:"tls"`
	Keepalive int    `yaml:"keepalive"`
}

type AgentConfig struct {
	Host             string   `yaml:"host"`
	Port             int      `yaml:"port"`
	TransferPort     int      `yaml:"transfer_port"`
	AllowedPaths     []string `yaml:"allowed_paths"`
	AllowRunCommand  bool     `yaml:"allow_run_command"`
	AllowDestructive bool     `yaml:"allow_destructive"`
}

type Config struct {
	DeviceName string      `yaml:"device_name"`
	DeviceType string      `yaml:"device_type"`
	Secret     string      `yaml:"secret"`
	LogLevel   string      `yaml:"log_level"`
	LogFormat  string      `yaml:"log_format"`
	MQTT       MQTTConfig  `yaml:"mqtt"`
	Agent      AgentConfig `yaml:"agent"`
}

func defaults() *Config {
	return &Config{
		DeviceName: "",
		DeviceType: "pc",
		LogLevel:   "INFO",
		LogFormat:  "pretty",
		MQTT: MQTTConfig{
			Host:      "localhost",
			Port:      1883,
			Username:  "meshuser",
			Keepalive: 60,
		},
		Agent: AgentConfig{
			Host:             "0.0.0.0",
			Port:             7800,
			TransferPort:     7801,
			AllowDestructive: true,
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := defaults()

	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("reading config file %q: %w", path, err)
	}
	if len(data) > 0 {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing config file %q: %w", path, err)
		}
	}

	// Environment variable overrides
	if v := os.Getenv("MESH_DEVICE_NAME"); v != "" {
		cfg.DeviceName = v
	}
	if v := os.Getenv("MQTT_PASSWORD"); v != "" {
		cfg.MQTT.Password = v
	}
	if v := os.Getenv("MESH_SECRET"); v != "" {
		cfg.Secret = v
	}

	return cfg, cfg.validate()
}

func (c *Config) validate() error {
	var errs []string

	if c.DeviceName == "" {
		errs = append(errs, "device_name is required — set a unique name (no spaces)")
	}
	if c.MQTT.Password == "" {
		errs = append(errs, "mqtt.password is required — set it in config or MQTT_PASSWORD env var")
	}
	if c.Secret == "" {
		errs = append(errs, "secret is required — must match the hub's secret")
	}
	if c.Agent.Port < 1024 || c.Agent.Port > 65535 {
		errs = append(errs, fmt.Sprintf("agent.port %d is out of range [1024–65535]", c.Agent.Port))
	}
	if c.Agent.TransferPort < 1024 || c.Agent.TransferPort > 65535 {
		errs = append(errs, fmt.Sprintf("agent.transfer_port %d is out of range [1024–65535]", c.Agent.TransferPort))
	}
	if c.Agent.Port == c.Agent.TransferPort {
		errs = append(errs, "agent.port and agent.transfer_port must be different")
	}

	if len(errs) > 0 {
		msg := "configuration errors:"
		for _, e := range errs {
			msg += "\n  • " + e
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}
