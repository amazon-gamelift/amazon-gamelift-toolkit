/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package cmd

import (
	"testing"

	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/assert"
	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/testutil"
)

func TestValidateIPAddress(t *testing.T) {
	tests := []struct {
		name      string
		ipAddress string
		wantErr   bool
	}{
		{"valid IPv4", "192.168.1.1", false},
		{"valid localhost", testutil.TestProxyAddr, false},
		{"valid IPv6", "::1", false},
		{"invalid IP", "256.256.256.256", true},
		{"invalid format", "not.an.ip", true},
		{"empty string", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateIPAddress(tt.ipAddress)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateFlags(t *testing.T) {
	originalCfg := cfg

	tests := []struct {
		name    string
		setup   func()
		wantErr bool
	}{
		{
			name: "valid config",
			setup: func() {
				cfg.IPAddress = testutil.TestProxyAddr
				cfg.ServerIPAddress = "192.168.1.1"
				cfg.UDPEndpointCount = 5
				cfg.ServerPort = testutil.TestReturnTrafficPort
				cfg.PortRange = "8000-9000"
			},
			wantErr: false,
		},
		{
			name: "invalid IP address",
			setup: func() {
				cfg.IPAddress = "invalid.ip"
				cfg.ServerIPAddress = testutil.TestProxyAddr
				cfg.UDPEndpointCount = 5
				cfg.ServerPort = testutil.TestReturnTrafficPort
				cfg.PortRange = ""
			},
			wantErr: true,
		},
		{
			name: "invalid server IP address",
			setup: func() {
				cfg.IPAddress = testutil.TestProxyAddr
				cfg.ServerIPAddress = "invalid.ip"
				cfg.UDPEndpointCount = 5
				cfg.ServerPort = testutil.TestReturnTrafficPort
				cfg.PortRange = ""
			},
			wantErr: true,
		},
		{
			name: "UDP endpoint count out of bounds",
			setup: func() {
				cfg.IPAddress = testutil.TestProxyAddr
				cfg.ServerIPAddress = testutil.TestProxyAddr
				cfg.UDPEndpointCount = 15
				cfg.ServerPort = testutil.TestReturnTrafficPort
				cfg.PortRange = ""
			},
			wantErr: true,
		},
		{
			name: "server port out of bounds",
			setup: func() {
				cfg.IPAddress = testutil.TestProxyAddr
				cfg.ServerIPAddress = testutil.TestProxyAddr
				cfg.UDPEndpointCount = 5
				cfg.ServerPort = 70000
				cfg.PortRange = ""
			},
			wantErr: true,
		},
		{
			name: "invalid port range format",
			setup: func() {
				cfg.IPAddress = testutil.TestProxyAddr
				cfg.ServerIPAddress = testutil.TestProxyAddr
				cfg.UDPEndpointCount = 5
				cfg.ServerPort = testutil.TestReturnTrafficPort
				cfg.PortRange = "invalid-range"
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			err := validateFlags()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}

	cfg = originalCfg
}

func TestValidatePortRange(t *testing.T) {
	tests := []struct {
		name             string
		portRange        string
		udpEndpointCount int
		wantErr          bool
	}{
		{"valid range", "8000-9000", 3, false},
		{"empty string", "", 3, false},
		{"invalid format", "invalid-range", 3, true},
		{"start port too high", "70000-80000", 3, true},
		{"end port too high", "8000-70000", 3, true},
		{"start greater than end", "9000-8000", 3, true},
		{"range too small", "9000-9001", 10, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePortRange(tt.portRange, tt.udpEndpointCount)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
