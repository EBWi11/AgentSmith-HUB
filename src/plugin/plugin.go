package plugin

import (
	"AgentSmith-HUB/local_plugin"
	"AgentSmith-HUB/logger"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

const (
	LOCAL_PLUGIN = 0
	YAEGI_PLUGIN = 1
)

// Directory where yaegi plugins are stored
const PluginDir = "config/plugin"

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

var Plugins = make(map[string]*Plugin)
var PluginsNew = make(map[string]string)

func init() {
	for name, f := range local_plugin.LocalPluginBoolRes {
		if _, ok := Plugins[name]; !ok {
			p := &Plugin{
				Name:    name,
				Type:    LOCAL_PLUGIN,
				Payload: nil,
				f:       reflect.ValueOf(f),
			}
			Plugins[name] = p
		} else {
			logger.Error("plugin_init error", "plugin name conflict: %s already exists", name)
		}
	}

	for name, f := range local_plugin.LocalPluginInterfaceAndBoolRes {
		if _, ok := Plugins[name]; !ok {
			p := &Plugin{
				Name:    name,
				Type:    LOCAL_PLUGIN,
				Payload: nil,
				f:       reflect.ValueOf(f),
			}
			Plugins[name] = p
		} else {
			logger.Error("plugin_init error", "plugin name conflict: %s already exists", name)
		}
	}

	logger.Info("plugin_init", "plugins_count", len(Plugins))
}

func Verify(path string, raw string, name string) error {
	var err error
	var content []byte

	if _, ok := Plugins[name]; ok {
		return fmt.Errorf("plugin name conflict: %s already exists", name)
	}

	if path != "" {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return fmt.Errorf("plugin file not found at path: %s", path)
		}
		content, err = os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read plugin file: %w", err)
		}
	} else {
		content = []byte(raw)
	}

	p := &Plugin{Path: path, Payload: content}
	err = p.yaegiLoad()
	p = nil
	return err
}

func NewPlugin(path string, raw string, name string, pluginType int) error {
	var err error
	var content []byte

	err = Verify(path, raw, name)
	if err != nil {
		return fmt.Errorf("plugin verify err %s %s", name, err.Error())
	}

	if path != "" {
		content, _ = os.ReadFile(path)
	} else {
		content = []byte(raw)
	}

	p := &Plugin{Path: path, Payload: content, Type: pluginType, Name: name}

	_ = p.yaegiLoad()
	Plugins[p.Name] = p
	return nil
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

// LoadPlugin loads a yaegi plugin from the given path
func LoadPlugin(path string) (*Plugin, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	name := filepath.Base(path)
	if len(name) > 3 && name[len(name)-3:] == ".go" {
		name = name[:len(name)-3]
	}
	p := &Plugin{
		Name:    name,
		Path:    path,
		Payload: content,
		Type:    YAEGI_PLUGIN,
	}
	if err := p.yaegiLoad(); err != nil {
		return nil, err
	}
	return p, nil
}
