/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package setdegradation

import (
	"fmt"
	"net"
	"testing"

	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/assert"
)

func TestRunSetDegradation_ValidInput(t *testing.T) {
	tests := []struct {
		name                  string
		degradationPercentage int
	}{
		{"minimum degradation", 0},
		{"maximum degradation", 100},
		{"mid-range degradation", 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup UDP listener to receive the message
			addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
			assert.NoError(t, err)

			conn, err := net.ListenUDP("udp", addr)
			assert.NoError(t, err, "Failed to listen on udp address")
			defer conn.Close()

			connAddr, ok := conn.LocalAddr().(*net.UDPAddr)
			assert.True(t, ok, "Failed to get UDP address from connection")

			// Set config values
			setDegradationCfg.DegradationPercentage = tt.degradationPercentage
			setDegradationCfg.Port = connAddr.Port

			errChan := make(chan error, 1)
			go func() {
				errChan <- runSetDegradation(nil, nil)
			}()

			// Read the message
			buffer := make([]byte, 1024)
			n, _, err := conn.ReadFromUDP(buffer)
			assert.NoError(t, err)

			// Verify the message format
			expectedMessage := fmt.Sprintf("PlayerGateway:SetDegradation:%d", tt.degradationPercentage)
			actualMessage := string(buffer[:n])
			assert.Equal(t, expectedMessage, actualMessage)

			// Check command result
			err = <-errChan
			assert.NoError(t, err)
		})
	}
}

func TestRunSetDegradation_InvalidDegradationPercentage(t *testing.T) {
	tests := []struct {
		name                  string
		degradationPercentage int
		expectedError         string
	}{
		{"negative degradation", -1, "degradation-percentage must be between 0 and 100, got -1"},
		{"degradation too high", 101, "degradation-percentage must be between 0 and 100, got 101"},
		{"large negative value", -100, "degradation-percentage must be between 0 and 100, got -100"},
		{"large positive value", 200, "degradation-percentage must be between 0 and 100, got 200"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setDegradationCfg.DegradationPercentage = tt.degradationPercentage
			setDegradationCfg.Port = 8000

			err := runSetDegradation(nil, nil)
			assert.Error(t, err)
			assert.Equal(t, tt.expectedError, err.Error())
		})
	}
}

func TestRunSetDegradation_InvalidPort(t *testing.T) {
	tests := []struct {
		name          string
		port          int
		expectedError string
	}{
		{"zero port", 0, "port must be between 1 and 65535, got 0"},
		{"negative port", -1, "port must be between 1 and 65535, got -1"},
		{"port too high", 65536, "port must be between 1 and 65535, got 65536"},
		{"large negative port", -1000, "port must be between 1 and 65535, got -1000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setDegradationCfg.DegradationPercentage = 50
			setDegradationCfg.Port = tt.port

			err := runSetDegradation(nil, nil)
			assert.Error(t, err)
			assert.Equal(t, tt.expectedError, err.Error())
		})
	}
}

func TestSetDegradationCommand_FlagsInitialization(t *testing.T) {
	// Verify the command is properly initialized
	assert.NotNil(t, SetDegradation)
	assert.Equal(t, "set-degradation", SetDegradation.Use)
	assert.NotNil(t, SetDegradation.RunE)

	// Verify flags are registered
	degradationFlag := SetDegradation.Flags().Lookup("degradation-percentage")
	assert.NotNil(t, degradationFlag)
	assert.Equal(t, "0", degradationFlag.DefValue)

	portFlag := SetDegradation.Flags().Lookup("port")
	assert.NotNil(t, portFlag)
	assert.Equal(t, "0", portFlag.DefValue)
}
