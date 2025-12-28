package flake

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestDetector_detectFlakePattern_FailThenPass(t *testing.T) {
	d := &Detector{}
	msg := "boom"

	attempts := []testAttempt{
		{TestCaseID: uuid.New(), AttemptNumber: 1, Status: "failed", FailureMsg: &msg},
		{TestCaseID: uuid.New(), AttemptNumber: 2, Status: "passed"},
	}

	detected, failedAttempt, passedAttempt, failureMsg := d.detectFlakePattern(attempts)
	require.True(t, detected)
	require.Equal(t, 1, failedAttempt)
	require.Equal(t, 2, passedAttempt)
	require.NotNil(t, failureMsg)
	require.Equal(t, msg, *failureMsg)
}

func TestDetector_detectFlakePattern_FailFailPassUsesEarliestFailure(t *testing.T) {
	d := &Detector{}
	msg := "boom"

	attempts := []testAttempt{
		{TestCaseID: uuid.New(), AttemptNumber: 1, Status: "failed", FailureMsg: &msg},
		{TestCaseID: uuid.New(), AttemptNumber: 2, Status: "failed"},
		{TestCaseID: uuid.New(), AttemptNumber: 3, Status: "passed"},
	}

	detected, failedAttempt, passedAttempt, failureMsg := d.detectFlakePattern(attempts)
	require.True(t, detected)
	require.Equal(t, 1, failedAttempt)
	require.Equal(t, 3, passedAttempt)
	require.NotNil(t, failureMsg)
	require.Equal(t, msg, *failureMsg)
}

func TestDetector_detectFlakePattern_PassThenPassNoFlake(t *testing.T) {
	d := &Detector{}

	attempts := []testAttempt{
		{TestCaseID: uuid.New(), AttemptNumber: 1, Status: "passed"},
		{TestCaseID: uuid.New(), AttemptNumber: 2, Status: "passed"},
	}

	detected, _, _, _ := d.detectFlakePattern(attempts)
	require.False(t, detected)
}

func TestDetector_detectFlakePattern_PassThenFailNoFlake(t *testing.T) {
	d := &Detector{}

	attempts := []testAttempt{
		{TestCaseID: uuid.New(), AttemptNumber: 1, Status: "passed"},
		{TestCaseID: uuid.New(), AttemptNumber: 2, Status: "failed"},
	}

	detected, _, _, _ := d.detectFlakePattern(attempts)
	require.False(t, detected)
}
