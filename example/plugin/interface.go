// +build plugin
package main

import (
	"os"

	"github.com/Wessie/gps"
)

var c *gps.Controller

func init() {
	c = gps.NewPluginController(os.Stdin, os.Stdout)
	c.RegisterFunc("hello", HelloWorld)
}

func HelloWorld(s S) {
	c.CallFunction("hello", s)
}

type S struct {
	A int
	B string
}
