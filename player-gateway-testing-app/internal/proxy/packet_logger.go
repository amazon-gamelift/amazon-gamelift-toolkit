/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package proxy

import (
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"sync"
	"time"
)

const (
	// ReportFilePermissions defines the file permissions for the report log file (rw-------)
	ReportFilePermissions = 0600
	// UnknownSourceAddr is used when the source address is nil or unavailable
	UnknownSourceAddr = "unknown"
)

// FailedPacket represents a packet that failed validation.
type FailedPacket struct {
	Timestamp    time.Time    // Time when the packet was received
	SourceAddr   *net.UDPAddr // UDP address of the packet sender
	Data         []byte       // Raw packet data that failed validation
	ErrorMessage string       // Error message explaining why the packet failed validation
}

// PacketLogger handles asynchronous logging of failed packets to a file.
type PacketLogger struct {
	filePath   string            // Path to the log file
	packetChan chan FailedPacket // Buffered channel for queuing packets to log
	file       *os.File          // Open file handle for writing logs
	wg         sync.WaitGroup    // Wait group for graceful shutdown
	stopOnce   sync.Once         // Ensures Close() runs exactly once
}

// NewPacketLogger creates a new packet logger that writes to the specified file.
//
// Parameters:
//   - filePath: path to the file where failed packets will be logged
//
// Returns:
//   - *PacketLogger: configured packet logger ready to start
//   - error: nil on success, error if file cannot be opened
func NewPacketLogger(filePath string) (*PacketLogger, error) {
	// Open file in append mode, create if it doesn't exist
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, ReportFilePermissions)
	if err != nil {
		return nil, fmt.Errorf("failed to open report file: %w", err)
	}

	logger := &PacketLogger{
		filePath:   filePath,
		packetChan: make(chan FailedPacket, 1000),
		file:       file,
	}

	logger.wg.Add(1)
	go logger.run()

	return logger, nil
}

// run is the main goroutine that processes failed packets and writes them to the file.
// This method runs continuously until the packet channel is closed.
func (pl *PacketLogger) run() {
	defer pl.wg.Done()
	defer pl.file.Close()

	for packet := range pl.packetChan {
		pl.writePacket(packet)
	}
}

// writePacket writes a single failed packet to the log file.
// Logs include timestamp, source address, error message, packet length, and hex-encoded data.
// If writing fails, an error message is written to stderr.
//
// Parameters:
//   - packet: the failed packet to log
func (pl *PacketLogger) writePacket(packet FailedPacket) {
	timestamp := packet.Timestamp.Format(time.RFC3339)
	hexData := hex.EncodeToString(packet.Data)

	sourceAddr := UnknownSourceAddr
	if packet.SourceAddr != nil {
		sourceAddr = packet.SourceAddr.String()
	}

	logEntry := fmt.Sprintf("[%s] Source: %s | Error: %s | Length: %d bytes | Data (hex): %s\n",
		timestamp,
		sourceAddr,
		packet.ErrorMessage,
		len(packet.Data),
		hexData,
	)

	if _, err := pl.file.WriteString(logEntry); err != nil {
		// If we can't write to the log file, write to stderr as a fallback
		fmt.Fprintf(os.Stderr, "Failed to write to report file: %v\n", err)
	}
}

// LogFailedPacket queues a failed packet for logging.
// This method is non-blocking and safe to call from multiple goroutines.
//
// Parameters:
//   - sourceAddr: source address of the failed packet
//   - data: raw packet data
//   - errorMessage: error message explaining why the packet failed validation
func (pl *PacketLogger) LogFailedPacket(sourceAddr *net.UDPAddr, data []byte, errorMessage string) {
	packet := FailedPacket{
		Timestamp:    time.Now(),
		SourceAddr:   sourceAddr,
		Data:         data,
		ErrorMessage: errorMessage,
	}

	select {
	case pl.packetChan <- packet:
	default:
		addr := UnknownSourceAddr
		if sourceAddr != nil {
			addr = sourceAddr.String()
		}
		fmt.Fprintf(os.Stderr, "Packet logger channel full, dropping packet from %s\n", addr)
	}
}

// Close stops the packet logger and waits for all queued packets to be written.
// This method is safe to call multiple times.
func (pl *PacketLogger) Close() {
	pl.stopOnce.Do(func() {
		close(pl.packetChan)
		pl.wg.Wait()
	})
}
