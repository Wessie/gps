package main

import (
	"fmt"
	"log"

	"github.com/Wessie/gps"
)

func HelloWorld(s S) {
	fmt.Println(s)
}

type S struct {
	A int
	B string
}

func main() {
	c := gps.NewController()
	c.RegisterFunc("hello", HelloWorld)

	p, err := c.LoadPlugin("pluggie")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(p)
	fmt.Println(p.Run())
	fmt.Println("ran")

	p.Wait()
}
