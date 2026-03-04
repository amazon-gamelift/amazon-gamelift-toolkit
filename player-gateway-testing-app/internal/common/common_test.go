/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package common

import (
	"fmt"
	"net"
	"testing"

	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/assert"
)

func TestParsePortRange_ValidInputs(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectStart int
		expectEnd   int
	}{
		{"standard range", "8000-8010", 8000, 8010},
		{"full port range", "1-65535", 1, 65535},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			start, end, err := ParsePortRange(test.input)
			assert.NoError(t, err)
			assert.Equal(t, start, test.expectStart)
			assert.Equal(t, end, test.expectEnd)
		})
	}
}

func TestParsePortRange_InvalidInputs(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"non-numeric input", "invalid"},
		{"single port", "8000"},
		{"empty string", ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			start, end, err := ParsePortRange(test.input)
			assert.Error(t, err)
			assert.Equal(t, start, 0)
			assert.Equal(t, end, 0)
		})
	}
}
func TestFindAvailablePortRange_Success(t *testing.T) {
	tests := []struct {
		name            string
		startSearchPort int
		count           int
		maxSearchPort   int
	}{
		{
			name:            "single port available",
			startSearchPort: 8000,
			count:           1,
			maxSearchPort:   8010,
		},
		{
			name:            "multiple consecutive ports",
			startSearchPort: 8020,
			count:           3,
			maxSearchPort:   8030,
		},
		{
			name:            "larger port range",
			startSearchPort: 8040,
			count:           10,
			maxSearchPort:   8060,
		},
		{
			name:            "start port equals max port with count 1",
			startSearchPort: 8010,
			count:           1,
			maxSearchPort:   8010,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			startPort, err := FindAvailablePortRange(tt.startSearchPort, tt.count, tt.maxSearchPort)
			assert.NoError(t, err)
			assert.True(t, startPort >= tt.startSearchPort, "Returned startPort should be >= startSearchPort")
			assert.True(t, startPort+tt.count-1 <= tt.maxSearchPort, "Returned range should not exceed maxSearchPort")
		})
	}
}

func TestFindAvailablePortRange_Error(t *testing.T) {
	tests := []struct {
		name            string
		startSearchPort int
		count           int
		maxSearchPort   int
	}{
		{
			name:            "start port greater than max port",
			startSearchPort: 8010,
			count:           1,
			maxSearchPort:   8000,
		},
		{
			name:            "port range too small",
			startSearchPort: 8000,
			count:           10,
			maxSearchPort:   8005,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			startPort, err := FindAvailablePortRange(tt.startSearchPort, tt.count, tt.maxSearchPort)
			assert.Error(t, err)
			assert.Equal(t, startPort, 0, "Expected startPort to be 0 on error")
		})
	}
}

func TestFindAvailablePortRange_WithOccupiedPorts(t *testing.T) {
	// Find an available port first
	startPort, err := FindAvailablePortRange(51000, 1, 51010)
	assert.NoError(t, err, "Failed to find initial available port")

	// Bind to that port to make it unavailable
	addr := fmt.Sprintf(":%d", startPort)
	listener, err := net.ListenPacket("udp", addr)
	assert.NoError(t, err, "Failed to bind to port")
	defer listener.Close()

	// Now try to find a range that includes the occupied port
	// This should skip the occupied port and find the next available range
	foundPort, err := FindAvailablePortRange(startPort, 2, startPort+10)
	assert.NoError(t, err, "Failed to find available port range")

	// The found port should be different from the occupied port
	assert.NotEqual(t, foundPort, startPort, "Function should have skipped occupied port")

	// Verify the found range is actually available
	for i := 0; i < 2; i++ {
		testAddr := fmt.Sprintf(":%d", foundPort+i)
		testListener, err := net.ListenPacket("udp", testAddr)
		assert.NoError(t, err, "Port should be available")
		if testListener != nil {
			testListener.Close()
		}
	}
}

// TestFindAvailablePortRange_InvalidPort tests behavior with invalid port numbers
func TestFindAvailablePortRange_InvalidPort(t *testing.T) {
	tests := []struct {
		name            string
		startSearchPort int
		count           int
		maxSearchPort   int
	}{
		{
			name:            "port exceeds maximum",
			startSearchPort: 65536,
			count:           1,
			maxSearchPort:   65540,
		},
		{
			name:            "range exceeds maximum port",
			startSearchPort: 65535,
			count:           2,
			maxSearchPort:   65536,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			startPort, err := FindAvailablePortRange(tt.startSearchPort, tt.count, tt.maxSearchPort)
			assert.Error(t, err, "Should fail with invalid port numbers")
			assert.Equal(t, 0, startPort, "Should return 0 on error")
		})
	}
}

func TestValidateNumberWithinBounds(t *testing.T) {
	tests := []struct {
		name    string
		value   int
		min     int
		max     int
		wantErr bool
	}{
		{"within bounds", 5, 1, 10, false},
		{"at minimum", 1, 1, 10, false},
		{"at maximum", 10, 1, 10, false},
		{"below minimum", 0, 1, 10, true},
		{"above maximum", 11, 1, 10, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNumberWithinBounds(tt.value, tt.min, tt.max)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
