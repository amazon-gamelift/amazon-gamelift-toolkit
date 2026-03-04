/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package testutil

import "time"

// Shared test constants
const (
	// Network addresses
	TestDestinationAddr = "127.0.0.1:9000" // Used as destination address
	TestProxyAddr       = "127.0.0.1"      // Used as proxy bind address
	TestSourceAddr      = "127.0.0.1:8001" // Used as client source address

	// Test data
	TestMessage  = "test message"
	TestResponse = "response"

	// Configuration values
	TestTimeout        = 100 * time.Millisecond
	TestChannelSize    = 10
	TestReportFilePath = "test-report.txt"

	// Port and player constants
	TestReturnTrafficPort = 8080
	TestPlayerNumber      = 1
	TestPlayerCount       = 1

	// Token - TestTokenHash is base64-encoded, TestTokenHashDecoded is the decoded value (28 bytes)
	TestTokenHash        = "dG9rZW4xMTExMTExMTExMTExMTExMTExMTExMQ==" // base64("token11111111111111111111111")
	TestTokenHashDecoded = "token11111111111111111111111"              // 28 bytes
)
