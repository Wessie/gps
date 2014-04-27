package gps

import (
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"reflect"
)

type Function func([]reflect.Value) []reflect.Value

func NewController() *Controller {
	c := &Controller{
		ValueCodec:        DefaultValueCodec,
		StreamCodec:       DefaultStreamCodec,
		TypeRegistry:      make(map[string]reflect.Type),
		InterfaceRegistry: make(map[reflect.Type]reflect.Type),
		FuncRegistry:      make(map[string]Function),
	}

	return c
}

func NewPluginController(in io.Reader, out io.Writer) *Controller {
	c := NewController()
	c.Plugin = true

	c.plugin = newPlugin(c)
	c.plugin.In = in
	c.plugin.Out = out

	c.Serve()

	return c
}

type Controller struct {
	// Indicates if we are the plugin or master
	Plugin bool
	// Dummy plugin of the plugin, nil for master
	plugin *Plugin
	// ValueCodec is used for the encoding and decoding of values to
	// bytes.
	ValueCodec Codec
	// StreamCodec is used for encoding and decoding the protocol
	// before it is send across a connection.
	StreamCodec Codec
	// TypeRegistry is a registry of all the types that are valid
	// to be passed between the two sides.
	TypeRegistry map[string]reflect.Type
	// InterfaceRegistry is a mapping of interface types to
	// their mock implementation type
	//
	// Only interfaces registered in here are allowed to be passed
	// between sides.
	InterfaceRegistry map[reflect.Type]reflect.Type
	// FuncRegistry is a mapping of function identifiers to
	// functions `reflect.Value`s
	//
	// Only functions registered in here are allowed to be passed
	// and or called between sides.
	FuncRegistry map[string]Function
}

func (c *Controller) LoadPlugin(path string) (*Plugin, error) {
	abspath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(abspath)
	r, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	w, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	p := newPlugin(c)
	p.In = r
	p.Out = w
	p.Cmd = cmd

	return p, nil
}

func (c *Controller) Serve() error {
	if !c.Plugin {
		return nil
	}

	return c.plugin.Run()
}

func (c *Controller) CallFunction(identifier string, args ...interface{}) []interface{} {
	_, ok := c.FuncRegistry[identifier]
	if !ok {
		panic("Invalid identifier: " + identifier)
	}

	m := &Message{
		FuncID: identifier,
	}

	for _, arg := range args {
		v, err := c.EncodeValue(arg)
		if err != nil {
			panic(err)
		}

		m.Values = append(m.Values, v)
	}

	return <-c.plugin.sendMessage(m)
}

func (c *Controller) callFunction(identifier string, args ...interface{}) []interface{} {
	f, ok := c.FuncRegistry[identifier]
	if !ok {
		panic("Invalid identifier: " + identifier)
	}

	in := make([]reflect.Value, len(args))
	for i, arg := range args {
		in[i] = reflect.ValueOf(arg)
	}

	out := f(in)
	res := make([]interface{}, len(out))
	for i, r := range out {
		res[i] = r.Interface()
	}

	return res
}

func (c *Controller) registerType(t reflect.Type) {
	switch t.Kind() {
	case reflect.Interface:
		panic("Unsupported interface arguments")
	case reflect.Func:
		panic("Unsupported function arguments")
	default:
		c.TypeRegistry[t.String()] = t
	}
}

// RegisterInterface registers an interface and its mock implementation.
//
// It is impossible to pass interfaces that have
// not been registered by this method. Input `e` is
// expected to be a struct with a single field with
// the type of the interface you want to register, and have
// as value the mocked implementation of this interface.
//
// A shortcut to do the above is to pass in an anonymous struct:
// 	`struct{Interface}{MockImplementation}`
//
// Passing in a non-struct will panic
//
// See the package documentation on how to implement a mock of your interface
func (c *Controller) RegisterInterface(e interface{}) {
	v := reflect.ValueOf(e)
	t := v.Type()

	// Check if what we got is actually usable
	if t.Kind() != reflect.Struct {
		panic("Expected struct, found '" + t.String() + "' instead.")
	}
	if t.NumField() < 1 || t.NumField() > 1 {
		msg := fmt.Sprintf("Expected single-field struct, got %d fields instead.", t.NumField())
		panic(msg)
	}

	it := t.Field(0).Type
	elem := v.Field(0).Elem()

	if !elem.IsValid() {
		msg := fmt.Sprintf("Expected mock implementation, found nil instead")
		panic(msg)
	}
	mt := elem.Type()

	c.InterfaceRegistry[it] = mt

	// Now we will farm the interface methods for extra types
	// we should register
	for i := 0; i < it.NumMethod(); i++ {
		m := it.Method(i)

		for j := 0; j < m.Type.NumIn(); j++ {
			t := m.Type.In(j)
			c.registerType(t)
		}

		for j := 0; j < m.Type.NumOut(); j++ {
			t := m.Type.Out(j)
			c.registerType(t)
		}
	}
}

// RegisterFunc registers a function that can be called from another
// process. All functions to be called need to be registered and
// mocked in the other process.
//
// The `identifier` is for naming function `e`, since function types
// (and values) do not have a name, it has to be given one.
//
// See the package documentation on how to implement a mock of your function
func (c *Controller) RegisterFunc(identifier string, e interface{}) {
	v := reflect.ValueOf(e)
	t := v.Type()

	if t.Kind() != reflect.Func {
		panic("Expected function, found '" + t.String() + "' instead.")
	}

	// Pre-select the choice of Call or CallSlice so we don't have to
	// check it every time in other code
	var f Function
	if t.IsVariadic() {
		f = v.CallSlice
	} else {
		f = v.Call
	}

	c.FuncRegistry[identifier] = f

	for i := 0; i < t.NumIn(); i++ {
		arg := t.In(i)
		c.registerType(arg)
	}

	for i := 0; i < t.NumOut(); i++ {
		arg := t.Out(i)
		c.registerType(arg)
	}
}

func (c *Controller) EncodeValue(value interface{}) (*Value, error) {
	var v Value

	t := reflect.TypeOf(value)
	v.Type = t.String()

	b, err := c.ValueCodec.Encode(value)
	if err != nil {
		return nil, err
	}

	v.Value = b
	return &v, nil
}

func (c *Controller) DecodeValue(v *Value) interface{} {
	t, ok := c.TypeRegistry[v.Type]
	if !ok {
		return nil
	}

	value := reflect.New(t)

	if err := c.ValueCodec.Decode(v.Value, value.Interface()); err != nil {
		return nil
	}

	return value.Elem().Interface()
}

type Value struct {
	Type  string
	Value []byte
}
