/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package proxy

import "net"

// TrafficHandler handles different behavior for routing traffic in client/server proxies depending on the direction of the traffic.
type TrafficHandler interface {
	// PreprocessServerBoundTraffic processes traffic going from client to server.
	// Returns the player number, modified packet data (with token processing applied), and any error.
	// The modified data should be used for forwarding instead of the original data.
	PreprocessServerBoundTraffic(data []byte, sourceAddr *net.UDPAddr) (PreprocessServerBoundTrafficResult, error)
	// HandleClientBoundTraffic processes traffic going from server to client through the proxy.
	HandleClientBoundTraffic(data ClientBoundData, ccPool *ClientConnectionPool, socket *net.UDPConn) error
}

type PreprocessServerBoundTrafficResult struct {
	PlayerNumber  int
	Data          []byte
	ConfigCommand ConfigCommandResult
}

// ConfigCommandResult represents a successfully processed configuration command
type ConfigCommandResult interface {
	// ConfigCommandResult is a marker method that identifies types implementing the ConfigCommandResult interface
	ConfigCommandResult()
}

// DegradationResult represents a successful degradation change
type DegradationResult struct {
	Percentage int // The degradation percentage that was set
}

func (d DegradationResult) ConfigCommandResult() {}

// GenericCommandResult represents a successful command execution with no specific result data
type GenericCommandResult struct{}

func (g GenericCommandResult) ConfigCommandResult() {}
