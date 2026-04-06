/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package getplayerconnectiondetails

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/common"
	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/proxy"
	"github.com/spf13/cobra"
)

const (
	playerNumbersFlag = "player-numbers"
	portFlag          = "port"
)

var (
	playerNumbersStr string
	port             int

	GetPlayerConnectionDetails = &cobra.Command{
		Use:   "get-player-connection-details",
		Short: "Get connection details for specified players",
		RunE:  runGetPlayerConnectionDetails,
	}
)

func init() {
	GetPlayerConnectionDetails.Flags().StringVar(&playerNumbersStr, playerNumbersFlag, "", "Comma-separated list of player numbers (e.g., --player-numbers 1,2,3)")
	GetPlayerConnectionDetails.Flags().IntVar(&port, portFlag, 8000, "A UDP port of the running testing app")
	GetPlayerConnectionDetails.MarkFlagRequired(playerNumbersFlag)
}

func runGetPlayerConnectionDetails(command *cobra.Command, args []string) error {
	if err := common.ValidateNumberWithinBounds(port, common.MinPort, common.MaxPort); err != nil {
		return fmt.Errorf("port must be between %d and %d", common.MinPort, common.MaxPort)
	}

	playerNumbers := strings.Split(playerNumbersStr, ",")
	if len(playerNumbers) == 1 && playerNumbers[0] == "" {
		return fmt.Errorf("at least one player number is required")
	}

	for _, num := range playerNumbers {
		if _, err := strconv.Atoi(strings.TrimSpace(num)); err != nil {
			return fmt.Errorf("invalid player number: %s", num)
		}
	}

	message := fmt.Sprintf("%s%s:%s", proxy.ConfigCommandPrefix, proxy.GetPlayerConnectionDetailsCommand, playerNumbersStr)
	response, err := common.SendUDPMessageWithResponse(port, message)
	if err != nil {
		return fmt.Errorf("failed to get response from testing app: %w", err)
	}

	var result proxy.PlayerConnectionDetailsResponse
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	output, err := json.MarshalIndent(result.PlayerConnectionDetails, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format response: %w", err)
	}

	fmt.Println(string(output))
	return nil
}
