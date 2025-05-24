package common

import (
	"errors"
	"io/ioutil"
	"os"
	"reflect"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

func (p *Plugin) yaegiLoad() error {
	p.YaegiIntp = interp.New(interp.Options{})
	err := p.YaegiIntp.Use(stdlib.Symbols)

	if err != nil {
		return err
	}

	_, err = p.YaegiIntp.Eval(string(p.Payload))
	if err != nil {
		return err
	}

	v, err := p.YaegiIntp.Eval("plugin.Eval")
	if err != nil {
		return err
	}

	p.YaegiFunc = reflect.ValueOf(v.Interface())
	return nil
}

func (p *Plugin) FuncEval(funcArgs map[int]interface{}) []interface{} {
	switch p.Type {
	case "Yaegi":
		var realArgs []reflect.Value
		var result []interface{}

		for _, v := range funcArgs {
			realArgs = append(realArgs, reflect.ValueOf(v))
		}

		out := p.YaegiFunc.Call(realArgs)
		for _, v := range out {
			result = append(result, v.Interface())
		}

		return result
	default:
		return nil
	}
}

func NewPlugin(path string, pluginType string) (*Plugin, error) {
	var err error

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, errors.New("PLUGIN FILE IS NOT EXIST")
	}

	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	p := &Plugin{Path: path, Payload: content}

	switch pluginType {
	case "Yaegi":
		p.Type = pluginType
	default:
		return nil, errors.New("PLUGIN TYP ONLY ALLOW Yaegi")
	}

	err = p.yaegiLoad()
	return p, err
}
