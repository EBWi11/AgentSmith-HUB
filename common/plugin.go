package common

import (
	"errors"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

const PluginsPath = "plugins/"
const PluginEnding = "_plugin.go"

var Plugins = make(map[string]*Plugin)

func init() {
	_ = filepath.WalkDir(PluginsPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			panic(err)
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), PluginEnding) {
			p, err := NewPlugin(path, "Yaegi")
			if err != nil {
				panic(err)
			}
			p.setName(d.Name()[:len(d.Name())-len(PluginEnding)])
			Plugins[p.Name] = p
		}
		return nil
	})
}

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

func (p *Plugin) FuncEval(funcArgs []interface{}) []interface{} {
	var out []reflect.Value

	switch p.Type {
	case "Yaegi":
		var realArgs []reflect.Value
		var result []interface{}

		for _, v := range funcArgs {
			realArgs = append(realArgs, reflect.ValueOf(v))
		}

		if len(realArgs) == 0 {
			out = p.YaegiFunc.Call(nil)
		} else {
			out = p.YaegiFunc.Call(realArgs)
		}

		for _, v := range out {
			result = append(result, v.Interface())
		}

		return result
	default:
		return nil
	}
}

func (p *Plugin) setName(name string) {
	p.Name = name
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
