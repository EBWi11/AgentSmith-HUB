package plugin

import (
	"AgentSmith-HUB/local_plugin"
	"AgentSmith-HUB/logger"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

type Plugin struct {
	Name    string
	Path    string
	Payload []byte

	yaegiIntp *interp.Interpreter
	f         reflect.Value

	// 0 local
	// 1 yaegi
	Type int
}

const PluginEnding = ".go"

var Plugins = make(map[string]*Plugin)

func PluginInit(PluginsPath string) error {
	// load yaegi plugins
	_ = filepath.WalkDir(PluginsPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() && strings.HasSuffix(d.Name(), PluginEnding) {
			p, err := NewPlugin(path, "Yaegi")
			if err != nil {
				return err
			}
			p.setName(d.Name()[:len(d.Name())-len(PluginEnding)])
			Plugins[p.Name] = p
		}
		return nil
	})

	for name, f := range local_plugin.LocalPluginBoolRes {
		if _, ok := Plugins[name]; !ok {
			p := &Plugin{
				Name:    name,
				Type:    0, // local plugin
				Payload: nil,
				f:       reflect.ValueOf(f),
			}
			Plugins[name] = p
		} else {
			return fmt.Errorf("plugin name conflict: %s already exists", name)
		}
	}

	for name, f := range local_plugin.LocalPluginInterfaceAndBoolRes {
		if _, ok := Plugins[name]; !ok {
			p := &Plugin{
				Name:    name,
				Type:    0, // local plugin
				Payload: nil,
				f:       reflect.ValueOf(f),
			}
			Plugins[name] = p
		} else {
			return fmt.Errorf("plugin name conflict: %s already exists", name)
		}
	}

	logger.Info("plugin_init", "plugins_count", len(Plugins))

	return nil
}

func NewPlugin(path string, pluginType string) (*Plugin, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("plugin file not found at path: %s", path)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read plugin file: %w", err)
	}

	p := &Plugin{Path: path, Payload: content, Type: 1}

	switch pluginType {
	case "Yaegi":
		p.Type = 1
	default:
		return nil, fmt.Errorf("unsupported plugin type: %s, only Yaegi is supported", pluginType)
	}

	err = p.yaegiLoad()
	return p, err
}

func (p *Plugin) yaegiLoad() error {
	p.yaegiIntp = interp.New(interp.Options{})
	err := p.yaegiIntp.Use(stdlib.Symbols)

	if err != nil {
		return err
	}

	_, err = p.yaegiIntp.Eval(string(p.Payload))
	if err != nil {
		return err
	}

	v, err := p.yaegiIntp.Eval("plugin.Eval")
	if err != nil {
		return err
	}

	p.f = reflect.ValueOf(v.Interface())
	return nil
}

func (p *Plugin) setName(name string) {
	p.Name = name
}

func (p *Plugin) FuncEvalCheckNode(funcArgs ...interface{}) bool {
	var realArgs []reflect.Value

	switch p.Type {
	case 0: // local plugin
		if f, ok := local_plugin.LocalPluginBoolRes[p.Name]; ok {
			res, err := f(funcArgs...)
			if err != nil {
				logger.Error("local plugin returned error:", "plugin", p.Name, "error", err)
			}
			return res
		} else {
			logger.Error("local plugin not found", "plugin", p.Name)
			return false
		}
	case 1: // yaegi plugin
		var res1 bool
		var res2 error
		var ok bool
		var out []reflect.Value

		for _, v := range funcArgs {
			realArgs = append(realArgs, reflect.ValueOf(v))
		}

		if len(realArgs) == 0 {
			out = p.f.Call(nil)
		} else {
			out = p.f.Call(realArgs)
		}

		if len(out) != 2 {
			logger.Error("plugin returned unexpected number of results", "name", p.Name, "len of out", len(out))
			return false
		}

		if res1, ok = out[0].Interface().(bool); !ok {
			logger.Error("plugin returned unexpected type", "plugin", p.Name, "type", reflect.TypeOf(res1))
			return false
		}

		if res2, ok = out[1].Interface().(error); ok {
			logger.Error("plugin returned error", "plugin", p.Name, "error", res2)
		}

		return res1
	}
	return false
}

func (p *Plugin) FuncEvalOther(funcArgs ...interface{}) (interface{}, bool) {
	var realArgs []reflect.Value

	switch p.Type {
	case 0: // local plugin
		if f, ok := local_plugin.LocalPluginInterfaceAndBoolRes[p.Name]; ok {
			res1, res2, err := f(funcArgs...)
			if err != nil {
				logger.Error("local plugin %s returned error:", "plugin", p.Name, "error", err)
				return nil, false
			}
			return res1, res2
		} else {
			logger.Error("local plugin not found", "plugin", p.Name)
			return nil, false
		}
	case 1: // yaegi plugin
		var out []reflect.Value
		var res2 bool
		var res3 error
		var ok bool

		for _, v := range funcArgs {
			realArgs = append(realArgs, reflect.ValueOf(v))
		}

		if len(realArgs) == 0 {
			out = p.f.Call(nil)
		} else {
			out = p.f.Call(realArgs)
		}

		if len(out) != 3 {
			logger.Error("plugin returned unexpected number of results", "plugin", p.Name, "len of out", len(out))
		}

		if res2, ok = out[1].Interface().(bool); !ok {
			logger.Error("plugin returned unexpected type for first result", "plugin", p.Name, "type", reflect.TypeOf(out[2].Interface()))
			return nil, false
		}

		if res3, ok = out[2].Interface().(error); ok {
			logger.Error("plugin returned error", "name", p.Name, "error", res3)
		}

		return out[0].Interface(), res2
	}
	return nil, false
}
