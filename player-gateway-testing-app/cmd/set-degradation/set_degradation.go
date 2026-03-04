/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package setdegradation

import (
	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/common"
	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/config"
	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/proxy"
	"fmt"

	"github.com/spf13/cobra"
)

const (
	degradationPercentageFlag = "degradation-percentage"
	portFlag                  = "port"
)

var (
	setDegradationCfg = config.SetDegradationConfig{}
	SetDegradation    = &cobra.Command{
		Use:  "set-degradation",
		RunE: runSetDegradation,
	}
)

func init() {
	SetDegradation.Flags().IntVar(&setDegradationCfg.DegradationPercentage, degradationPercentageFlag, 0, "The desired degradation percentage")
	SetDegradation.MarkFlagRequired(degradationPercentageFlag)
	SetDegradation.Flags().IntVar(&setDegradationCfg.Port, portFlag, 0, "The port to configure with the desired degradation percentage")
	SetDegradation.MarkFlagRequired(portFlag)
}

func runSetDegradation(command *cobra.Command, args []string) error {
	if err := common.ValidateNumberWithinBounds(setDegradationCfg.DegradationPercentage, proxy.MinimumDegradationPercentage, proxy.MaximumDegradationPercentage); err != nil {
		return fmt.Errorf("degradation-percentage must be between %d and %d, got %d", proxy.MinimumDegradationPercentage, proxy.MaximumDegradationPercentage, setDegradationCfg.DegradationPercentage)
	}

	if err := common.ValidateNumberWithinBounds(setDegradationCfg.Port, common.MinPort, common.MaxPort); err != nil {
		return fmt.Errorf("port must be between %d and %d, got %d", common.MinPort, common.MaxPort, setDegradationCfg.Port)
	}

	message := fmt.Sprintf("%s%s:%d", proxy.ConfigCommandPrefix, proxy.SetDegradationCommand, setDegradationCfg.DegradationPercentage)
	return common.SendUDPMessage(setDegradationCfg.Port, message)
}
