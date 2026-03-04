/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	replacetoken "github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/cmd/replace-token"
	setdegradation "github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/cmd/set-degradation"
	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/app"
	"github.com/amazon-gamelift/amazon-gamelift-toolkit/player-gateway-testing-app/internal/config"

	"github.com/spf13/cobra"
)

var (
	cfg     = config.Config{}
	rootCmd = &cobra.Command{
		Use:   "./player-gateway-testing-app",
		Short: "Test integration with Amazon GameLift Servers player gateway",
		RunE:  run,
	}
)

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	bindFlags()

	rootCmd.AddCommand(setdegradation.SetDegradation)
	rootCmd.AddCommand(replacetoken.ReplaceToken)
}

func run(cmd *cobra.Command, args []string) error {
	if err := validateFlags(); err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer cancel()

	a := app.New()
	return a.Run(ctx, cancel, cfg)
}
