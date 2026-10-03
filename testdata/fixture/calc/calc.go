package calc

// Calculator performs basic arithmetic based on a configurable factor.
type Calculator struct {
	Base int
}

// Add sums two integers.
func Add(a, b int) int {
	return a + b
}

// Multiply computes the product of two integers.
func Multiply(a, b int) int {
	return a * b
}

// Compute adds x and y and scales the result by Base.
func (c *Calculator) Compute(x, y int) int {
	sum := Add(x, y)
	return Multiply(sum, c.Base)
}

// Unused is an independent function for isolation testing.
func Unused(x int) int {
	return x * 2
}
