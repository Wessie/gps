package gps

import "testing"

type testStruct struct {
	A bool
	B int
	C int8
	D int16
	E int32
	F int64
	G uint
	H uint8
	I uint16
	J uint32
	K uint64
	L rune // alias of int32
	M byte // alias of uint8
	N string
	O []byte
	P [16]byte
	Q float32
	R float64
	S complex64
	T complex128
	/*	U
		V
		W
		X
		Y
		Z*/
}

// Small interface with various types for testing base-behaviour
type interfaceMethodA interface {
	MethodA(a, b string, c int) testStruct
}

// Mock implementation of testMethodA
type mockMethodA rune

func (t mockMethodA) MethodA(a string, b string, c int) testStruct {
	return testStruct{}
}

func newController() *Controller {
	return NewController()

}

func TestRegisterInterface(t *testing.T) {
	c := newController()

	c.RegisterInterface(struct{ interfaceMethodA }{mockMethodA('A')})

	t.Log(c.InterfaceRegistry, c.TypeRegistry)
}

func TestRegisterFunc(t *testing.T) {
	c := newController()

	f := func(i int) int {
		return i + 50
	}

	c.RegisterFunc("hello", f)

	t.Log(c.FuncRegistry, c.TypeRegistry)

	result := c.CallFunction("hello", 50)
	if result[0].(int) != 100 {
		t.Error("Wrongdoing")
	}
}

// BenchmarkCallFunction benchmarks a simple function call through
// Controller.CallFunction.
func BenchmarkCallFunction(b *testing.B) {
	f := func(i int) int {
		return i + 50
	}
	c := newController()
	c.RegisterFunc("bench", f)

	for i := 0; i < b.N; i++ {
		res := c.CallFunction("bench", 50)
		_ = res[0].(int)
	}
}

// BenchmarkNormalFunction benchmarks the same simple function call
// as BenchmarkCallFunction but does so without reflection
func BenchmarkNormalFunction(b *testing.B) {
	f := func(i int) int {
		return i + 50
	}

	for i := 0; i < b.N; i++ {
		_ = f(50)
	}
}
