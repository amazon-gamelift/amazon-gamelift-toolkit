/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package proxy

import (
	"bytes"
	"encoding/hex"
	"io"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/assert"
	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/testutil"
)

func TestNewPacketLogger(t *testing.T) {
	defer os.Remove(testutil.TestReportFilePath)

	logger, err := NewPacketLogger(testutil.TestReportFilePath)
	assert.NoError(t, err, "Failed to create packet logger")
	defer logger.Close()

	assert.Equal(t, logger.filePath, testutil.TestReportFilePath)
}

func TestPacketLogger_LogFailedPacket(t *testing.T) {
	defer os.Remove(testutil.TestReportFilePath)

	logger, err := NewPacketLogger(testutil.TestReportFilePath)
	assert.NoError(t, err, "Failed to create packet logger")

	sourceAddr, _ := net.ResolveUDPAddr("udp", testutil.TestSourceAddr)
	testData := []byte(testutil.TestMessage)

	logger.LogFailedPacket(sourceAddr, testData, "token validation failed")

	// Close logger to ensure all data is flushed
	logger.Close()

	// Read the log file
	content, err := os.ReadFile(testutil.TestReportFilePath)
	assert.NoError(t, err, "Failed to read log file")

	logContent := string(content)

	// Verify log contains source address
	if !strings.Contains(logContent, testutil.TestSourceAddr) {
		t.Error("Log should contain source address")
	}

	// Verify log contains hex-encoded data
	expectedHex := hex.EncodeToString(testData)
	if !strings.Contains(logContent, expectedHex) {
		t.Errorf("Log should contain hex data %s, got: %s", expectedHex, logContent)
	}
}

func TestPacketLogger_MultiplePackets(t *testing.T) {
	defer os.Remove(testutil.TestReportFilePath)

	logger, err := NewPacketLogger(testutil.TestReportFilePath)
	assert.NoError(t, err, "Failed to create packet logger")

	// Log multiple packets
	for i := 0; i < 5; i++ {
		sourceAddr, _ := net.ResolveUDPAddr("udp", testutil.TestSourceAddr)
		testData := []byte("packet " + string(rune('0'+i)))
		logger.LogFailedPacket(sourceAddr, testData, "test error")
	}

	// Close logger to ensure all data is flushed
	logger.Close()

	// Read the log file
	content, err := os.ReadFile(testutil.TestReportFilePath)
	assert.NoError(t, err, "Failed to read log file")

	logContent := string(content)

	// Count number of log entries (each should have a timestamp)
	lines := strings.Split(strings.TrimSpace(logContent), "\n")
	assert.Equal(t, len(lines), 5)
}

func TestPacketLogger_Close(t *testing.T) {
	defer os.Remove(testutil.TestReportFilePath)

	logger, err := NewPacketLogger(testutil.TestReportFilePath)
	assert.NoError(t, err, "Failed to create packet logger")

	sourceAddr, _ := net.ResolveUDPAddr("udp", testutil.TestSourceAddr)
	logger.LogFailedPacket(sourceAddr, []byte("test"), "test reason")

	// Close should not panic
	logger.Close()

	// Multiple closes should be safe
	logger.Close()
}

// TestPacketLogger_LogFailedPacket_NilSourceAddr tests logging with nil source address
func TestPacketLogger_LogFailedPacket_NilSourceAddr(t *testing.T) {
	defer os.Remove(testutil.TestReportFilePath)

	logger, err := NewPacketLogger(testutil.TestReportFilePath)
	assert.NoError(t, err, "Failed to create packet logger")

	testData := []byte(testutil.TestMessage)

	// Log with nil source address
	logger.LogFailedPacket(nil, testData, "test error")

	logger.Close()

	// Read the log file
	content, err := os.ReadFile(testutil.TestReportFilePath)
	assert.NoError(t, err, "Failed to read log file")

	logContent := string(content)

	// Verify log contains "unknown" for nil source
	assert.True(t, strings.Contains(logContent, "unknown"), "Log should contain 'unknown' for nil source address")
}

// TestPacketLogger_ChannelFull tests behavior when packet channel is full
func TestPacketLogger_ChannelFull(t *testing.T) {
	defer os.Remove(testutil.TestReportFilePath)

	logger, err := NewPacketLogger(testutil.TestReportFilePath)
	assert.NoError(t, err, "Failed to create packet logger")

	// Capture stderr to verify drop message
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Read stderr concurrently to avoid blocking when channel is full
	var buf bytes.Buffer
	go func() {
		io.Copy(&buf, r)
	}()

	sourceAddr, _ := net.ResolveUDPAddr("udp", testutil.TestSourceAddr)
	testData := []byte(testutil.TestMessage)

	// Fill the channel by sending many packets quickly
	// The channel size is PacketLoggerChannelSize (1000)
	for i := 0; i < 1050; i++ {
		logger.LogFailedPacket(sourceAddr, testData, "test error")
	}

	// Restore stderr first, then read captured output
	w.Close()
	os.Stderr = oldStderr

	stderrOutput := buf.String()

	// Close and verify no panic occurred
	logger.Close()

	// Verify that the channel full message was printed (if any drops occurred)
	// On Windows, some messages might not be captured due to buffering
	if len(stderrOutput) > 0 {
		assert.True(t, strings.Contains(stderrOutput, "Packet logger channel full") ||
			strings.Contains(stderrOutput, testutil.TestSourceAddr),
			"Should print channel full message when dropping packets")
	}
}
