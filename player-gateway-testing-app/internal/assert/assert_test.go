/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package assert

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

type mockT struct {
	failed bool
	msg    string
}

func (m *mockT) Helper() {}
func (m *mockT) Fatalf(format string, args ...any) {
	m.failed = true
	m.msg = fmt.Sprintf(format, args...)
}

func TestEqual(t *testing.T) {
	mock := &mockT{}
	Equal(mock, 5, 5)
	if mock.failed {
		t.Error("Equal failed on equal values")
	}

	mock = &mockT{}
	Equal(mock, 5, 10, "custom message")
	if !mock.failed || !strings.Contains(mock.msg, "custom message") {
		t.Error("Equal should fail on unequal values")
	}
}

func TestNotEqual(t *testing.T) {
	mock := &mockT{}
	NotEqual(mock, 5, 10)
	if mock.failed {
		t.Error("NotEqual failed on different values")
	}

	mock = &mockT{}
	NotEqual(mock, 5, 5)
	if !mock.failed {
		t.Error("NotEqual should fail on equal values")
	}
}

func TestError(t *testing.T) {
	mock := &mockT{}
	Error(mock, errors.New("test"))
	if mock.failed {
		t.Error("Error failed when error present")
	}

	mock = &mockT{}
	Error(mock, nil)
	if !mock.failed {
		t.Error("Error should fail when no error")
	}
}

func TestNoError(t *testing.T) {
	mock := &mockT{}
	NoError(mock, nil)
	if mock.failed {
		t.Error("NoError failed when no error")
	}

	mock = &mockT{}
	NoError(mock, errors.New("test"))
	if !mock.failed {
		t.Error("NoError should fail when error present")
	}
}

func TestNil(t *testing.T) {
	mock := &mockT{}
	Nil(mock, nil)
	if mock.failed {
		t.Error("Nil failed on nil value")
	}

	mock = &mockT{}
	Nil(mock, "not nil")
	if !mock.failed {
		t.Error("Nil should fail on non-nil value")
	}
}

func TestNotNil(t *testing.T) {
	mock := &mockT{}
	NotNil(mock, "value")
	if mock.failed {
		t.Error("NotNil failed on non-nil value")
	}

	mock = &mockT{}
	NotNil(mock, nil)
	if !mock.failed {
		t.Error("NotNil should fail on nil value")
	}
}

func TestTrue(t *testing.T) {
	mock := &mockT{}
	True(mock, true)
	if mock.failed {
		t.Error("True failed on true condition")
	}

	mock = &mockT{}
	True(mock, false, "custom message")
	if !mock.failed {
		t.Error("True should fail on false condition")
	}
}

func TestFalse(t *testing.T) {
	mock := &mockT{}
	False(mock, false)
	if mock.failed {
		t.Error("False failed on false condition")
	}

	mock = &mockT{}
	False(mock, true, "custom message")
	if !mock.failed {
		t.Error("False should fail on true condition")
	}
}
