/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package config

// Config holds all configuration parameters for the proxy application.
type Config struct {
	IPAddress        string // IP address for the proxy to bind to
	PortRange        string // Port range for client proxy allocation (format: "start-end")
	ServerIPAddress  string // IP address of the game server to forward traffic to
	ServerPort       int    // Port number of the game server
	PlayerCount      int    // Number of players (generates one token per player)
	UDPEndpointCount int    // Number of UDP endpoints to initialize (1-10)
	ReportFilePath   string // Path to file for reporting invalid packets
	Headless         bool   // If the application should run in headless mode or not
}

// SetDegradationConfig holds configuration for setting endpoint degradation on a specific port.
type SetDegradationConfig struct {
	Port                  int // Port number on which to apply degradation
	DegradationPercentage int // Percentage of packets to degrade (0-100)
}

// ReplaceTokenConfig holds configuration for replacing a player's token.
type ReplaceTokenConfig struct {
	Port         int // Any active port for a clientside endpoint to which the request will be sent to
	PlayerNumber int // Player number (1-indexed)
}
