package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type ClientConfig struct {
	ServerAddress   string
	Token           string
	Room            string
	DeviceID        string
	DeviceName      string
	PollInterval    time.Duration
	EnableText      bool
	EnableImage     bool
	ImageHelperPath string
	TLSEnabled      bool
	TLSCertFile     string
	TLSSkipVerify   bool
}

func LoadClientConfig(path string) (ClientConfig, error) {
	cfg := ClientConfig{
		PollInterval: 1 * time.Second,
		EnableText:   true,
		EnableImage:  true,
		TLSEnabled:   true,
	}

	file, err := os.Open(path)
	if err != nil {
		return cfg, err
	}
	defer file.Close()

	section := ""
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		switch section {
		case "server":
			switch key {
			case "address":
				cfg.ServerAddress = value
			case "token":
				cfg.Token = value
			case "room":
				cfg.Room = value
			}
		case "device":
			switch key {
			case "id":
				cfg.DeviceID = value
			case "name":
				cfg.DeviceName = value
			}
		case "sync":
			switch key {
			case "poll_interval":
				seconds, err := strconv.Atoi(value)
				if err != nil || seconds <= 0 {
					return cfg, fmt.Errorf("invalid sync.poll_interval: %q", value)
				}
				cfg.PollInterval = time.Duration(seconds) * time.Second
			case "enable_text":
				parsed, err := parseBool(value)
				if err != nil {
					return cfg, fmt.Errorf("invalid sync.enable_text: %w", err)
				}
				cfg.EnableText = parsed
			case "enable_image":
				parsed, err := parseBool(value)
				if err != nil {
					return cfg, fmt.Errorf("invalid sync.enable_image: %w", err)
				}
				cfg.EnableImage = parsed
			}
		case "clipboard":
			if key == "image_helper_path" {
				cfg.ImageHelperPath = value
			}
		case "tls":
			switch key {
			case "enabled":
				parsed, err := parseBool(value)
				if err != nil {
					return cfg, fmt.Errorf("invalid tls.enabled: %w", err)
				}
				cfg.TLSEnabled = parsed
			case "cert_file":
				cfg.TLSCertFile = value
			case "skip_verify":
				parsed, err := parseBool(value)
				if err != nil {
					return cfg, fmt.Errorf("invalid tls.skip_verify: %w", err)
				}
				cfg.TLSSkipVerify = parsed
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (c ClientConfig) Validate() error {
	if c.ServerAddress == "" {
		return fmt.Errorf("server.address is required")
	}
	if c.Room == "" {
		return fmt.Errorf("server.room is required")
	}
	if c.DeviceID == "" {
		return fmt.Errorf("device.id is required")
	}
	if c.Token == "" {
		return fmt.Errorf("server.token is required")
	}
	return nil
}

func parseBool(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("unsupported boolean value %q", value)
	}
}
