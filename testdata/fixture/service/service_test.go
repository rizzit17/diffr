package service

import "testing"

func TestExecuteOperation(t *testing.T) {
	res := ExecuteOperation(10, 20)
	if res != 30 {
		t.Errorf("ExecuteOperation(10, 20) = %d, want 30", res)
	}
}
