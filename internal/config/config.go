// Package config reads the single configuration file used by the service.
package config

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type Config struct {
	HTTP      HTTP     `json:"http"`
	Logging   Logging  `json:"logging"`
	Plugs     []Device `json:"plugs"`
	LEDStrips []Device `json:"lights"`
}

type HTTP struct {
	Listen                string `json:"listen"`
	RequestTimeoutSeconds int    `json:"request_timeout_seconds"`
	Token                 string `json:"token"`
}

type Logging struct {
	Level string `json:"level"`
	OTel  struct {
		Enabled     bool   `json:"enabled"`
		Endpoint    string `json:"endpoint"`
		ServiceName string `json:"service_name"`
	} `json:"otel"`
}

type Device struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Address  string `json:"address"`
	DeviceID string `json:"device_id"`
	LocalKey string `json:"local_key"`
	Protocol string `json:"protocol"`
	Profile  string `json:"profile"`
}

func Load(path string) (Config, error) {
	var c Config
	f, err := os.Open(path)
	if err != nil {
		return c, fmt.Errorf("open configuration: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.Size() > 1024*1024 {
		return c, fmt.Errorf("configuration must be a readable file no larger than 1 MiB")
	}
	decoder := json.NewDecoder(io.LimitReader(f, 1024*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&c); err != nil {
		// Do not echo decoder errors: they may include a secret field value.
		return c, fmt.Errorf("configuration must be valid JSON with known fields")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return c, fmt.Errorf("configuration must contain one JSON object")
	}
	if c.Plugs == nil {
		return c, fmt.Errorf("configuration must include a plugs array (use [] for no devices)")
	}
	return c, c.validate()
}

func (c *Config) validate() error {
	if c.HTTP.Listen == "" {
		c.HTTP.Listen = "127.0.0.1:8080"
	}
	if _, port, err := net.SplitHostPort(c.HTTP.Listen); err != nil {
		return fmt.Errorf("http.listen must be host:port")
	} else if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("http.listen must contain a valid port")
	}
	if c.HTTP.RequestTimeoutSeconds == 0 {
		c.HTTP.RequestTimeoutSeconds = 15
	}
	if c.HTTP.RequestTimeoutSeconds < 1 || c.HTTP.RequestTimeoutSeconds > 120 {
		return fmt.Errorf("http.request_timeout_seconds must be between 1 and 120")
	}
	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
	switch c.Logging.Level {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("logging.level must be debug, info, warn, or error")
	}
	if c.Logging.OTel.ServiceName == "" {
		c.Logging.OTel.ServiceName = "plug-api"
	}
	if c.Logging.OTel.Enabled {
		u, err := url.Parse(c.Logging.OTel.Endpoint)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
			return fmt.Errorf("logging.otel.endpoint must be an HTTP(S) origin without a path or credentials")
		}
	}
	ids := make(map[string]bool)
	devices := make(map[string]bool)
	addresses := make(map[string]bool)
	idPattern := regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)
	for _, group := range []struct {
		name    string
		profile string
		devices []Device
	}{
		{"plugs", "lsc-3202087-2", c.Plugs},
		{"lights", "lsc-neon-strip", c.LEDStrips},
	} {
		for i, p := range group.devices {
			label := fmt.Sprintf("%s[%d]", group.name, i)
			if !idPattern.MatchString(p.ID) || ids[p.ID] {
				return fmt.Errorf("%s: id must be unique and contain 1..64 letters, digits, underscores, or hyphens", label)
			}
			if strings.TrimSpace(p.DeviceID) == "" || devices[p.DeviceID] {
				return fmt.Errorf("%s: device_id must be nonempty and unique", label)
			}
			ip := net.ParseIP(p.Address)
			if ip == nil || ip.IsUnspecified() || ip.IsMulticast() || addresses[ip.String()] {
				return fmt.Errorf("%s: address must be a unique unicast IP address", label)
			}
			if len(p.LocalKey) != 16 {
				return fmt.Errorf("%s: local_key must contain exactly 16 bytes", label)
			}
			if p.Protocol != "3.5" || p.Profile != group.profile {
				return fmt.Errorf("%s: supported protocol is 3.5 and profile is %s", label, group.profile)
			}
			ids[p.ID], devices[p.DeviceID], addresses[ip.String()] = true, true, true
		}
	}
	return nil
}
