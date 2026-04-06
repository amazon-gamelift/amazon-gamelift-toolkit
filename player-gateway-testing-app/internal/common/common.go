/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package common

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	MinPort = 1
	MaxPort = 65535
)

var portRangeRegex = regexp.MustCompile(`^\d{1,5}-\d{1,5}$`)

// ParsePortRange parses a port range string in the format "start-end" and returns
// the start and end port numbers as integers.
//
// Parameters:
//   - portRange: the port range to parse
//
// Returns:
//   - startPort, endPort, and any error that occurred
func ParsePortRange(portRange string) (int, int, error) {
	if !portRangeRegex.MatchString(portRange) {
		return 0, 0, fmt.Errorf("invalid port range format: %s", portRange)
	}

	parts := strings.Split(portRange, "-")
	startPort, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("start port not parsable: %s, Expected port as an integer.", parts[0])
	}
	endPort, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("end port not parsable: %s, Expected port as an integer.", parts[1])
	}

	return startPort, endPort, nil
}

// FindAvailablePortRange finds a range of consecutive available ports starting from a given port.
//
// Parameters:
//   - startSearchPort: the port to start searching from
//   - count: the number of consecutive ports needed
//   - maxSearchPort: the maximum port to search up to
//
// Returns:
//   - startPort: the first port in the available range
//   - error: nil on success, error if no suitable range is found
func FindAvailablePortRange(startSearchPort, count, maxSearchPort int) (int, error) {
	for startPort := startSearchPort; startPort+count-1 <= maxSearchPort; startPort++ {
		allAvailable := true

		for i := range count {
			port := startPort + i
			addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", port))
			if err != nil {
				allAvailable = false
				break
			}

			conn, err := net.ListenUDP("udp", addr)
			if err != nil {
				allAvailable = false
				break
			}
			conn.Close()
		}

		if allAvailable {
			return startPort, nil
		}
	}

	return 0, fmt.Errorf("no available port range of %d consecutive ports found between %d and %d",
		count, startSearchPort, maxSearchPort)
}

// SendUDPMessage sends a UDP message to the specified port on localhost.
//
// Parameters:
//   - port: the port to send the message to
//   - message: the message to send
//
// Returns:
//   - error: nil on success, error if connection or write fails
func SendUDPMessage(port int, message string) error {
	conn, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = conn.Write([]byte(message))
	if err != nil {
		return err
	}

	return nil
}

// SendUDPMessageWithResponse sends a UDP message and waits for a response.
//
// Parameters:
//   - port: the port to send the message to
//   - message: the message to send
//
// Returns:
//   - string: the response received
//   - error: nil on success, error if connection, write, or read fails
func SendUDPMessageWithResponse(port int, message string) (string, error) {
	const responseTimeout = 5 * time.Second

	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return "", err
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(responseTimeout))

	if _, err = conn.Write([]byte(message)); err != nil {
		return "", err
	}

	buf := make([]byte, 65535)
	n, err := conn.Read(buf)
	if err != nil {
		return "", err
	}

	return string(buf[:n]), nil
}

// ValidateNumberWithinBounds validates that a value is within the specified bounds.
//
// Parameters:
//   - value: the value to validate
//   - minimum: the minimum allowed value
//   - maximum: the maximum allowed value
//
// Returns:
//   - error: nil if valid, error with details if out of bounds
func ValidateNumberWithinBounds(value, minimum, maximum int) error {
	if value < minimum || value > maximum {
		return fmt.Errorf("value %d must be between %d and %d", value, minimum, maximum)
	}
	return nil
}
