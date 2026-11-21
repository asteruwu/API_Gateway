package lb

import (
	"errors"
	"testing"

	gconst "API_Gateway/pkg/constant"
	gerrors "API_Gateway/pkg/errors"
)

func TestNewUnknownPolicy(t *testing.T) {
	_, err := New("unknown")
	if !errors.Is(err, gerrors.ErrInvalidConfig) {
		t.Errorf("err = %v, want %v", err, gerrors.ErrInvalidConfig)
	}
}

func TestNewCreatesDistinctRoundRobin(t *testing.T) {
	first, err := New(gconst.LBPolicyRoundRobin)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := New(gconst.LBPolicyRoundRobin)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if first == second {
		t.Error("factory returned the same round robin instance for two callers")
	}
}
