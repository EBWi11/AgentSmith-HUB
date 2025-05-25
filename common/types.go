package common

import (
	"github.com/traefik/yaegi/interp"
	"reflect"
)

// CheckCoreCache for rule engine
type CheckCoreCache struct {
	Exist bool
	Data  string
}
type Plugin struct {
	Name      string
	Path      string
	Payload   []byte
	YaegiIntp *interp.Interpreter
	YaegiFunc reflect.Value
	Type      string
}
