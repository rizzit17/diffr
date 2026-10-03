package calc

import "testing"

func TestAdd(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Errorf("Add(2, 3) failed")
	}
}

func TestCompute(t *testing.T) {
	c := &Calculator{Base: 10}
	if c.Compute(2, 3) != 50 {
		t.Errorf("Compute(2, 3) failed")
	}
}

func TestUnusedDirect(t *testing.T) {
	if Unused(5) != 10 {
		t.Errorf("Unused(5) failed")
	}
}
