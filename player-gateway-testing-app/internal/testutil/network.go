/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package testutil

import (
	"net"
	"testing"
	"time"
)

// CreateTestUDPSocket creates a UDP socket for testing on a random port
func CreateTestUDPSocket(t *testing.T) *net.UDPConn {
	socket, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("Failed to create test socket: %v", err)
	}
	return socket
}

// CreateTestUDPSocketWithPort creates a UDP socket for testing on a specific port
func CreateTestUDPSocketWithPort(t *testing.T, port int) *net.UDPConn {
	socket, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		t.Fatalf("Failed to create test socket on port %d: %v", port, err)
	}
	return socket
}

// ReadUDPMessage reads a message from socket with timeout, returns message and source address
func ReadUDPMessage(t *testing.T, socket *net.UDPConn) (string, *net.UDPAddr) {
	buf := make([]byte, 1024)
	socket.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, sourceAddr, err := socket.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("Failed to read from socket: %v", err)
	}
	return string(buf[:n]), sourceAddr
}

// SendUDPMessage sends a UDP message and handles errors
func SendUDPMessage(t *testing.T, conn *net.UDPConn, addr *net.UDPAddr, data []byte) {
	_, err := conn.WriteToUDP(data, addr)
	if err != nil {
		t.Fatalf("Failed to send UDP message: %v", err)
	}
}

// WaitForUDPMessage waits for a UDP message with custom timeout
func WaitForUDPMessage(t *testing.T, conn *net.UDPConn, timeout time.Duration) ([]byte, *net.UDPAddr, error) {
	buf := make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(timeout))
	n, addr, err := conn.ReadFromUDP(buf)
	if err != nil {
		return nil, nil, err
	}
	return buf[:n], addr, nil
}
