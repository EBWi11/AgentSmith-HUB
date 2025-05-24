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

type YaegiRes struct {
	Flag             bool
	Err              error
	ReturnOriData    map[string]interface{}
	ReturnSingleData interface{}
}

type YaegiArgs struct {
	OriData map[string]interface{}
	Args    map[int]interface{}
}

type Plugin struct {
	Path      string
	Payload   []byte
	YaegiIntp *interp.Interpreter
	YaegiFunc reflect.Value
	Type      string
}
