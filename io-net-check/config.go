package main

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type config struct {
	ConfigFileName string        `json:"config_file_name" yaml:"configFileName" env:"CONFIG_FILE_NAME" envDefault:"config.yaml"`
	Address        string        `json:"address" yaml:"address" env:"ADDRESS"`
	Port           int           `json:"port" yaml:"port" env:"PORT" envDefault:"8080"`
	Interval       time.Duration `json:"interval" yaml:"interval" env:"INTERVAL" envDefault:"1m"`
	DialTimeout    time.Duration `json:"dial_timeout" yaml:"dialTimeout" env:"DIAL_TIMEOUT" envDefault:"3s"`
	MaxConcurrency int           `json:"max_concurrency" yaml:"maxConcurrency" env:"MAX_CONCURRENCY" envDefault:"16"`
	Targets        []Target      `json:"targets" yaml:"targets"`
}

type Target struct {
	Endpoint string `json:"endpoint" yaml:"endpoint"`
}

type ProbeTarget struct {
	Endpoint string
	Host     string
	Port     int
}

func (c *config) Validate() ([]ProbeTarget, error) {
	if c.Interval <= 0 {
		return nil, fmt.Errorf("interval must be greater than 0")
	}
	if c.DialTimeout <= 0 {
		return nil, fmt.Errorf("dialTimeout must be greater than 0")
	}
	if c.MaxConcurrency <= 0 {
		return nil, fmt.Errorf("maxConcurrency must be greater than 0")
	}
	if c.Port <= 0 || c.Port > 65535 {
		return nil, fmt.Errorf("port must be between 1 and 65535")
	}
	if len(c.Targets) == 0 {
		return nil, fmt.Errorf("targets cannot be empty")
	}

	targets := make([]ProbeTarget, 0, len(c.Targets))
	for _, target := range c.Targets {
		parsed, err := ParseProbeTarget(target.Endpoint)
		if err != nil {
			return nil, err
		}
		targets = append(targets, parsed)
	}
	return targets, nil
}

func ParseProbeTarget(endpoint string) (ProbeTarget, error) {
	if strings.TrimSpace(endpoint) == "" {
		return ProbeTarget{}, fmt.Errorf("endpoint cannot be empty")
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return ProbeTarget{}, fmt.Errorf("parse endpoint %q failed: %w", endpoint, err)
	}

	switch u.Scheme {
	case "http":
	case "https":
	default:
		return ProbeTarget{}, fmt.Errorf("endpoint %q uses unsupported scheme %q", endpoint, u.Scheme)
	}

	host := u.Hostname()
	if host == "" {
		return ProbeTarget{}, fmt.Errorf("endpoint %q has empty host", endpoint)
	}

	port := defaultPortByScheme(u.Scheme)
	if rawPort := u.Port(); rawPort != "" {
		parsedPort, err := strconv.Atoi(rawPort)
		if err != nil || parsedPort <= 0 || parsedPort > 65535 {
			return ProbeTarget{}, fmt.Errorf("endpoint %q has invalid port %q", endpoint, rawPort)
		}
		port = parsedPort
	}

	return ProbeTarget{
		Endpoint: endpoint,
		Host:     host,
		Port:     port,
	}, nil
}

func defaultPortByScheme(scheme string) int {
	switch scheme {
	case "http":
		return 80
	case "https":
		return 443
	default:
		return 0
	}
}
