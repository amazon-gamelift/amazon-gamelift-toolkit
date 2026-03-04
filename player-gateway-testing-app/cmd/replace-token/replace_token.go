/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package replacetoken

import (
	"fmt"

	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/common"
	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/config"
	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/proxy"
	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/token"
	"github.com/spf13/cobra"
)

const (
	playerNumberFlag = "player-number"
	portFlag         = "port"
)

var (
	replaceTokenCfg = config.ReplaceTokenConfig{}
	ReplaceToken    = &cobra.Command{
		Use:   "replace-token",
		Short: "Generate and replace the token for a specific player",
		RunE:  runReplaceToken,
	}
)

func init() {
	ReplaceToken.Flags().IntVar(&replaceTokenCfg.PlayerNumber, playerNumberFlag, 0, "The player number (1-indexed)")
	ReplaceToken.Flags().IntVar(&replaceTokenCfg.Port, portFlag, 8000, "A port of the running testing app")
	ReplaceToken.MarkFlagRequired(playerNumberFlag)
}

func runReplaceToken(command *cobra.Command, args []string) error {
	if replaceTokenCfg.PlayerNumber < 1 {
		return fmt.Errorf("player-number must be at least 1")
	}

	if err := common.ValidateNumberWithinBounds(replaceTokenCfg.Port, common.MinPort, common.MaxPort); err != nil {
		return fmt.Errorf("port must be between %d and %d, got %d", common.MinPort, common.MaxPort, replaceTokenCfg.Port)
	}

	newToken := token.GenerateRandomToken()
	message := fmt.Sprintf("%s%s:%d:%s", proxy.ConfigCommandPrefix, proxy.ReplaceTokenCommand, replaceTokenCfg.PlayerNumber, newToken)
	return common.SendUDPMessage(replaceTokenCfg.Port, message)
}
