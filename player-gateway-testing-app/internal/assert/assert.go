/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package assert

import (
	"errors"
	"strings"
)

type TestingT interface {
	Helper()
	Fatalf(format string, args ...any)
}

// Equal validates got and expected are equal and fails the test if they aren't equal.
//
// Parameters:
//   - t: testing instance
//   - got: actual value received
//   - expected: expected value to compare against
//   - messages: optional error messages to include in failure output
func Equal[E comparable](t TestingT, got, expected E, messages ...string) {
	t.Helper()
	if got != expected {
		t.Fatalf("Expected %v, got %v. %s", expected, got, strings.Join(messages, " "))
	}
}

// NotEqual validates got and notExpected are not equal and fails the test if they are equal.
//
// Parameters:
//   - t: testing instance
//   - got: actual value received
//   - notExpected: value that should not match the actual value
//   - messages: optional error messages to include in failure output
func NotEqual[E comparable](t TestingT, got, notExpected E, messages ...string) {
	t.Helper()
	if got == notExpected {
		t.Fatalf("Expected values to differ, but both were %v. %s", got, strings.Join(messages, " "))
	}
}

// Error validates that err is not nil and fails the test if no error is present.
//
// Parameters:
//   - t: testing instance
//   - err: error value to check
//   - messages: optional error messages to include in failure output
func Error(t TestingT, err error, messages ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("Expected error, got %v. %s", err, strings.Join(messages, " "))
	}
}

// NoError validates that err is nil and fails the test if an error is present.
//
// Parameters:
//   - t: testing instance
//   - err: error value to check
//   - messages: optional error messages to include in failure output
func NoError(t TestingT, err error, messages ...string) {
	t.Helper()
	if err != nil {
		t.Fatalf("Expected no error, got %v. %s", err, strings.Join(messages, " "))
	}
}

// Nil validates that got is nil and fails the test if it is not nil.
//
// Parameters:
//   - t: testing instance
//   - got: actual value to check for nil
//   - messages: optional error messages to include in failure output
func Nil(t TestingT, got any, messages ...string) {
	t.Helper()
	if got != nil {
		t.Fatalf("Expected nil, got %v. %s", got, strings.Join(messages, " "))
	}
}

// NotNil validates that got is not nil and fails the test if it is nil.
//
// Parameters:
//   - t: testing instance
//   - got: actual value to check for nil
//   - messages: optional error messages to include in failure output
func NotNil(t TestingT, got any, messages ...string) {
	t.Helper()
	if got == nil {
		t.Fatalf("Expected non-nil value. %s", strings.Join(messages, " "))
	}
}

// True validates that condition is true and fails the test if it is false.
//
// Parameters:
//   - t: testing instance
//   - condition: boolean condition to check
//   - messages: optional error messages to include in failure output
func True(t TestingT, condition bool, messages ...string) {
	t.Helper()
	if !condition {
		t.Fatalf("Expected true. %s", strings.Join(messages, " "))
	}
}

// False validates that condition is false and fails the test if it is true.
//
// Parameters:
//   - t: testing instance
//   - condition: boolean condition to check
//   - messages: optional error messages to include in failure output
func False(t TestingT, condition bool, messages ...string) {
	t.Helper()
	if condition {
		t.Fatalf("Expected false. %s", strings.Join(messages, " "))
	}
}

// ErrorIs validates that err matches target using errors.Is and fails the test if it doesn't match.
//
// Parameters:
//   - t: testing instance
//   - err: error value to check
//   - target: target error to match against
//   - messages: optional error messages to include in failure output
func ErrorIs(t TestingT, err, target error, messages ...string) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("Expected error to be %v, got %v. %s", target, err, strings.Join(messages, " "))
	}
}
