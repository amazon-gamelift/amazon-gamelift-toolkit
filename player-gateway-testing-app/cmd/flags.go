/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package cmd

import (
	"fmt"
	"net"

	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/common"
)

const (
	IPAddressFlag        = "testing-app-ip"
	ToolPortRangeFlag    = "tool-port-range"
	ServerIPAddressFlag  = "game-server-ip"
	ServerPortFlag       = "game-server-port"
	PlayerCountFlag      = "player-count"
	UDPEndpointCountFlag = "udp-endpoint-count"
	ReportFilePathFlag   = "report-file-path"
	HeadlessModeFlag     = "headless"
)

func bindFlags() {
	const localHost = "127.0.0.1"

	rootCmd.Flags().StringVarP(&cfg.IPAddress, IPAddressFlag, "i", localHost, "The IP address to listen to.")
	rootCmd.Flags().StringVarP(&cfg.PortRange, ToolPortRangeFlag, "r", "", "The port range that the testing app is allowed to allocate ports from: <number>-<number>.")
	rootCmd.Flags().StringVarP(&cfg.ServerIPAddress, ServerIPAddressFlag, "s", "", "The IP address or hostname where the game session will be run (Managed EC2 or Anywhere).")
	rootCmd.Flags().IntVarP(&cfg.ServerPort, ServerPortFlag, "p", 0, "The port the game session will be run on.")
	rootCmd.Flags().IntVarP(&cfg.PlayerCount, PlayerCountFlag, "n", 1, "The number of players (generates one token per player).")
	rootCmd.Flags().IntVarP(&cfg.UDPEndpointCount, UDPEndpointCountFlag, "u", 3, "The number of UDP endpoints to initialize.")
	rootCmd.Flags().StringVarP(&cfg.ReportFilePath, ReportFilePathFlag, "f", "./report.txt", "Path to a file to report invalid packets to.")
	rootCmd.Flags().BoolVarP(&cfg.Headless, HeadlessModeFlag, "H", false, "Run in headless mode without UI rendering.")

	rootCmd.MarkFlagRequired(ServerIPAddressFlag)
	rootCmd.MarkFlagRequired(ServerPortFlag)
}

func validateIPAddress(ipAddress string) error {
	if ip := net.ParseIP(ipAddress); ip == nil {
		return fmt.Errorf("failed to parse IP address %s", ipAddress)
	}
	return nil
}

func validatePortRange(portRange string, endpointCount int) error {
	if portRange == "" {
		return nil
	}

	startPort, endPort, err := common.ParsePortRange(portRange)
	if err != nil {
		return err
	}

	if startPort > endPort {
		return fmt.Errorf("start port %d should be less than end port %d", startPort, endPort)
	}

	if err := common.ValidateNumberWithinBounds(startPort, common.MinPort, common.MaxPort); err != nil {
		return fmt.Errorf("start port of %s out of bounds: %w", ToolPortRangeFlag, err)
	}

	if err := common.ValidateNumberWithinBounds(endPort, common.MinPort, common.MaxPort); err != nil {
		return fmt.Errorf("end port of %s out of bounds: %w", ToolPortRangeFlag, err)
	}

	availablePorts := endPort - startPort + 1
	if availablePorts < endpointCount {
		return fmt.Errorf("insufficient ports in range %s for %d UDP endpoints (need %d, have %d)",
			portRange, endpointCount, endpointCount, availablePorts)
	}

	return nil
}

func validateFlags() error {
	if err := validateIPAddress(cfg.IPAddress); err != nil {
		return fmt.Errorf("%s is invalid: %w", IPAddressFlag, err)
	}

	if err := validateIPAddress(cfg.ServerIPAddress); err != nil {
		return fmt.Errorf("%s is invalid: %w", ServerIPAddressFlag, err)
	}

	const minEndpointCount, maxEndpointCount = 1, 10
	if err := common.ValidateNumberWithinBounds(cfg.UDPEndpointCount, minEndpointCount, maxEndpointCount); err != nil {
		return fmt.Errorf("%s out of bounds: %w", UDPEndpointCountFlag, err)
	}

	if err := common.ValidateNumberWithinBounds(cfg.ServerPort, common.MinPort, common.MaxPort); err != nil {
		return fmt.Errorf("%s out of bounds: %w", ServerPortFlag, err)
	}

	if err := validatePortRange(cfg.PortRange, cfg.UDPEndpointCount); err != nil {
		return err
	}

	return nil
}
