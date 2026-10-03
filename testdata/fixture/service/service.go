package service

import "diffr/testdata/fixture/calc"

// ExecuteOperation calls calc.Add across package boundaries.
func ExecuteOperation(x, y int) int {
	return calc.Add(x, y)
}
