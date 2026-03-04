/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package testutil

import "github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/config"

// ConfigOption allows customization of test configs
type ConfigOption func(*config.Config)

// CreateTestConfig creates a valid test configuration with optional customizations
func CreateTestConfig(opts ...ConfigOption) config.Config {
	// Default test configuration
	cfg := config.Config{
		IPAddress:        TestProxyAddr,
		PortRange:        "10000-10010",
		ServerIPAddress:  "127.0.0.1",
		ServerPort:       9000,
		PlayerCount:      1,
		UDPEndpointCount: 1,
		ReportFilePath:   TestReportFilePath,
	}

	// Apply options
	for _, opt := range opts {
		opt(&cfg)
	}

	return cfg
}

// WithIPAddress sets the IP address
func WithIPAddress(ip string) ConfigOption {
	return func(c *config.Config) {
		c.IPAddress = ip
	}
}

// WithServerIPAddress sets the server IP address
func WithServerIPAddress(ip string) ConfigOption {
	return func(c *config.Config) {
		c.ServerIPAddress = ip
	}
}

// WithPortRange sets the port range
func WithPortRange(portRange string) ConfigOption {
	return func(c *config.Config) {
		c.PortRange = portRange
	}
}

// WithServerPort sets the server port
func WithServerPort(port int) ConfigOption {
	return func(c *config.Config) {
		c.ServerPort = port
	}
}

// WithPlayerCount sets the player count
func WithPlayerCount(count int) ConfigOption {
	return func(c *config.Config) {
		c.PlayerCount = count
	}
}

// WithUDPEndpointCount sets the UDP endpoint count
func WithUDPEndpointCount(count int) ConfigOption {
	return func(c *config.Config) {
		c.UDPEndpointCount = count
	}
}

// WithReportFilePath sets the report file path
func WithReportFilePath(path string) ConfigOption {
	return func(c *config.Config) {
		c.ReportFilePath = path
	}
}

// WithHeadless sets the headless mode to true
func WithHeadless() ConfigOption {
	return func(c *config.Config) {
		c.Headless = true
	}
}
