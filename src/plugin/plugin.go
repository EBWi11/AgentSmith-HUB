package plugin

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/local_plugin"
	"AgentSmith-HUB/logger"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	regexpgo "regexp"
	"strings"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

const (
	LOCAL_PLUGIN = 0
	YAEGI_PLUGIN = 1
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

	// Function parameter information for autocomplete
	Parameters []PluginParameter `json:"parameters"`

	// Return type information for validation and filtering
	ReturnType string `json:"return_type"` // "bool" or "interface{}"

	// Whether the plugin result should be negated (for ! prefix)
	IsNegated bool `json:"is_negated"`
}

// PluginParameter represents a function parameter
type PluginParameter struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}

var Plugins = make(map[string]*Plugin)
var PluginsNew = make(map[string]string)

func init() {
	for name, f := range local_plugin.LocalPluginBoolRes {
		if _, ok := Plugins[name]; !ok {
			p := &Plugin{
				Name:       name,
				Type:       LOCAL_PLUGIN,
				Payload:    nil,
				f:          reflect.ValueOf(f),
				ReturnType: "bool", // These plugins return bool for checknode
			}
			p.parsePluginParameters()
			Plugins[name] = p
		} else {
			logger.PluginError("plugin_init error", "plugin name conflict: %s already exists", name)
		}
	}

	for name, f := range local_plugin.LocalPluginInterfaceAndBoolRes {
		if _, ok := Plugins[name]; !ok {
			p := &Plugin{
				Name:       name,
				Type:       LOCAL_PLUGIN,
				Payload:    nil,
				f:          reflect.ValueOf(f),
				ReturnType: "interface{}", // These plugins return interface{} for other uses
			}
			p.parsePluginParameters()
			Plugins[name] = p
		} else {
			logger.PluginError("plugin_init error", "plugin name conflict: %s already exists", name)
		}
	}

	logger.Info("plugin_init", "plugins_count", len(Plugins))
}

func Verify(path string, raw string, name string) error {
	// Skip name conflict check during verification - this is only for syntax and structure validation
	// Name conflict will be checked during actual save/create operations

	// Use common file reading function
	content, err := common.ReadContentFromPathOrRaw(path, raw)
	if err != nil {
		return fmt.Errorf("failed to read plugin configuration: %w", err)
	}

	// Validate plugin code requirements
	err = validatePluginCode(string(content))
	if err != nil {
		return err
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

	err = p.yaegiLoad()
	if err != nil {
		return fmt.Errorf("plugin yaegi load err %s: %w", name, err)
	}

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

	// Validate function signature
	err = p.validateFunctionSignature()
	if err != nil {
		return err
	}

	// Parse plugin parameters for autocomplete
	p.parsePluginParameters()

	return nil
}

// validateFunctionSignature checks if the plugin Eval function has the correct signature
func (p *Plugin) validateFunctionSignature() error {
	if !p.f.IsValid() {
		return fmt.Errorf("plugin function is not valid")
	}

	funcType := p.f.Type()
	if funcType.Kind() != reflect.Func {
		return fmt.Errorf("plugin Eval is not a function")
	}

	// Check number of return values
	numOut := funcType.NumOut()
	if numOut != 2 && numOut != 3 {
		return fmt.Errorf("plugin Eval function must return 2 values (bool, error) or 3 values (interface{}, bool, error), but returns %d values", numOut)
	}

	errorInterface := reflect.TypeOf((*error)(nil)).Elem()

	if numOut == 2 {
		// Two return values: (bool, error) - for checknode plugins
		firstReturnType := funcType.Out(0)
		if firstReturnType.Kind() != reflect.Bool {
			return fmt.Errorf("plugin Eval function with 2 return values must have first return value as bool, but is %s", firstReturnType.String())
		}

		secondReturnType := funcType.Out(1)
		if !secondReturnType.Implements(errorInterface) {
			return fmt.Errorf("plugin Eval function second return value must be error, but is %s", secondReturnType.String())
		}

		p.ReturnType = "bool"
	} else if numOut == 3 {
		// Three return values: (interface{}, bool, error) - for other plugins
		secondReturnType := funcType.Out(1)
		if secondReturnType.Kind() != reflect.Bool {
			return fmt.Errorf("plugin Eval function with 3 return values must have second return value as bool, but is %s", secondReturnType.String())
		}

		thirdReturnType := funcType.Out(2)
		if !thirdReturnType.Implements(errorInterface) {
			return fmt.Errorf("plugin Eval function third return value must be error, but is %s", thirdReturnType.String())
		}

		p.ReturnType = "interface{}"
	}

	return nil
}

// parsePluginParameters extracts parameter information from the plugin function
func (p *Plugin) parsePluginParameters() {
	if !p.f.IsValid() {
		return
	}

	funcType := p.f.Type()
	if funcType.Kind() != reflect.Func {
		return
	}

	numIn := funcType.NumIn()
	p.Parameters = make([]PluginParameter, 0, numIn)

	for i := 0; i < numIn; i++ {
		paramType := funcType.In(i)
		paramName := fmt.Sprintf("arg%d", i+1)

		// Try to get better parameter names from function signature
		if p.Type == YAEGI_PLUGIN {
			// For Yaegi plugins, we can try to extract parameter names from the source code
			if paramNames := p.extractParameterNamesFromSource(); len(paramNames) > i {
				paramName = paramNames[i]
			}
		}

		// Convert Go type to readable string
		typeStr := p.formatTypeString(paramType)

		// All parameters are required except for variadic parameters
		isRequired := true
		if paramType.Kind() == reflect.Slice && i == numIn-1 {
			// Check if this is a variadic parameter (like ...interface{})
			if paramType.Elem().Kind() == reflect.Interface {
				isRequired = false
				typeStr = "..." + typeStr[2:] // Remove "[]" and add "..."
			}
		}

		p.Parameters = append(p.Parameters, PluginParameter{
			Name:     paramName,
			Type:     typeStr,
			Required: isRequired,
		})
	}
}

// extractParameterNamesFromSource tries to extract parameter names from plugin source code
func (p *Plugin) extractParameterNamesFromSource() []string {
	if p.Type != YAEGI_PLUGIN {
		return nil
	}

	source := string(p.Payload)

	// Look for function Eval definition
	funcPattern := `func\s+Eval\s*\(\s*([^)]*)\s*\)`
	re := regexpgo.MustCompile(funcPattern)
	matches := re.FindStringSubmatch(source)

	if len(matches) < 2 {
		return nil
	}

	paramStr := strings.TrimSpace(matches[1])
	if paramStr == "" {
		return nil
	}

	// Parse parameter list
	params := strings.Split(paramStr, ",")
	names := make([]string, 0, len(params))

	for _, param := range params {
		param = strings.TrimSpace(param)
		if param == "" {
			continue
		}

		// Handle variadic parameters (args ...interface{})
		if strings.Contains(param, "...") {
			parts := strings.Fields(param)
			if len(parts) > 0 {
				names = append(names, parts[0])
			}
			continue
		}

		// Handle normal parameters (name type)
		parts := strings.Fields(param)
		if len(parts) >= 2 {
			names = append(names, parts[0])
		} else if len(parts) == 1 {
			// Handle cases like "string" without parameter name
			names = append(names, fmt.Sprintf("arg%d", len(names)+1))
		}
	}

	return names
}

// formatTypeString converts reflect.Type to a readable string
func (p *Plugin) formatTypeString(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "int"
	case reflect.Float32, reflect.Float64:
		return "float"
	case reflect.Bool:
		return "bool"
	case reflect.Slice:
		elemType := p.formatTypeString(t.Elem())
		return "[]" + elemType
	case reflect.Interface:
		return "interface{}"
	default:
		return t.String()
	}
}

func (p *Plugin) FuncEvalCheckNode(funcArgs ...interface{}) (bool, error) {
	var realArgs []reflect.Value

	switch p.Type {
	case 0: // local plugin
		if f, ok := local_plugin.LocalPluginBoolRes[p.Name]; ok {
			// Execute with panic recovery
			var result bool
			var err error

			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.PluginError("local plugin execution panicked", "plugin", p.Name, "panic", r)
						result = false
						err = fmt.Errorf("local plugin execution panicked: %v", r)
					}
				}()

				result, err = f(funcArgs...)
				if err != nil {
					logger.PluginError("local plugin returned error:", "plugin", p.Name, "error", err)
				}
			}()

			common.RecordPluginInvoke(p.Name, err == nil)
			return result, err
		} else {
			err := fmt.Errorf("local plugin not found: %s", p.Name)
			logger.PluginError("local plugin not found", "plugin", p.Name)
			common.RecordPluginInvoke(p.Name, false)
			return false, err
		}
	case 1: // yaegi plugin
		// Execute with panic recovery
		var result bool
		var err error

		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.PluginError("plugin execution panicked", "plugin", p.Name, "panic", r)
					result = false
					err = fmt.Errorf("plugin execution panicked: %v", r)
				}
			}()

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
				err = fmt.Errorf("plugin returned unexpected number of results: %d", len(out))
				logger.PluginError("plugin returned unexpected number of results", "name", p.Name, "len of out", len(out))
				result = false
				return
			}

			if res1, ok = out[0].Interface().(bool); !ok {
				err = fmt.Errorf("plugin returned unexpected type: %v", reflect.TypeOf(out[0].Interface()))
				logger.PluginError("plugin returned unexpected type", "plugin", p.Name, "type", reflect.TypeOf(out[0].Interface()))
				result = false
				return
			}

			if res2, ok = out[1].Interface().(error); ok && res2 != nil {
				logger.PluginError("plugin returned error", "plugin", p.Name, "error", res2)
				result = res1
				err = res2
				return
			}

			result = res1
			err = nil
		}()

		common.RecordPluginInvoke(p.Name, err == nil)
		return result, err
	}
	common.RecordPluginInvoke(p.Name, false)
	return false, fmt.Errorf("unknown plugin type")
}

func (p *Plugin) FuncEvalOther(funcArgs ...interface{}) (interface{}, bool, error) {
	var realArgs []reflect.Value

	switch p.Type {
	case 0: // local plugin
		if f, ok := local_plugin.LocalPluginInterfaceAndBoolRes[p.Name]; ok {
			// Execute with panic recovery
			var result interface{}
			var success bool
			var err error

			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.PluginError("local plugin execution panicked", "plugin", p.Name, "panic", r)
						result = nil
						success = false
						err = fmt.Errorf("local plugin execution panicked: %v", r)
					}
				}()

				result, success, err = f(funcArgs...)
				if err != nil {
					logger.PluginError("local plugin %s returned error:", "plugin", p.Name, "error", err)
				}
			}()

			common.RecordPluginInvoke(p.Name, err == nil)
			return result, success, err
		} else {
			err := fmt.Errorf("local plugin not found: %s", p.Name)
			logger.PluginError("local plugin not found", "plugin", p.Name)
			common.RecordPluginInvoke(p.Name, false)
			return nil, false, err
		}
	case 1: // yaegi plugin
		// Execute with panic recovery
		var result interface{}
		var success bool
		var err error

		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.PluginError("plugin execution panicked", "plugin", p.Name, "panic", r)
					result = nil
					success = false
					err = fmt.Errorf("plugin execution panicked: %v", r)
				}
			}()

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
				err = fmt.Errorf("plugin returned unexpected number of results: %d", len(out))
				logger.PluginError("plugin returned unexpected number of results", "plugin", p.Name, "len of out", len(out))
				result = nil
				success = false
				return
			}

			if res2, ok = out[1].Interface().(bool); !ok {
				err = fmt.Errorf("plugin returned unexpected type for second result: %v", reflect.TypeOf(out[1].Interface()))
				logger.PluginError("plugin returned unexpected type for second result", "plugin", p.Name, "type", reflect.TypeOf(out[1].Interface()))
				result = nil
				success = false
				return
			}

			if res3, ok = out[2].Interface().(error); ok && res3 != nil {
				logger.PluginError("plugin returned error", "name", p.Name, "error", res3)
				result = out[0].Interface()
				success = res2
				err = res3
				return
			}

			result = out[0].Interface()
			success = res2
			err = nil
		}()

		common.RecordPluginInvoke(p.Name, err == nil)
		return result, success, err
	}
	common.RecordPluginInvoke(p.Name, false)
	return nil, false, fmt.Errorf("unknown plugin type")
}

// YaegiLoad is a public wrapper for yaegiLoad to allow external access
func (p *Plugin) YaegiLoad() error {
	return p.yaegiLoad()
}

// validatePluginCode validates that the plugin code meets the requirements
func validatePluginCode(source string) error {
	// Parse the Go source code
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", source, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("failed to parse plugin code: %w", err)
	}

	// 1. Check package declaration
	if file.Name == nil || file.Name.Name != "plugin" {
		return fmt.Errorf("plugin package must be 'plugin', found: %s", getPackageName(file))
	}

	// 2. Check for Eval function
	hasEvalFunc := false
	for _, decl := range file.Decls {
		if funcDecl, ok := decl.(*ast.FuncDecl); ok {
			if funcDecl.Name.Name == "Eval" {
				hasEvalFunc = true
				break
			}
		}
	}
	if !hasEvalFunc {
		return fmt.Errorf("plugin must contain an 'Eval' function")
	}

	// 3. Check imports - only allow Go standard library
	for _, importSpec := range file.Imports {
		if importSpec.Path != nil {
			importPath := strings.Trim(importSpec.Path.Value, `"`)
			if !isStandardLibraryPackage(importPath) {
				return fmt.Errorf("plugin can only import Go standard library packages, found external package: %s", importPath)
			}
		}
	}

	return nil
}

// getPackageName safely extracts the package name
func getPackageName(file *ast.File) string {
	if file.Name == nil {
		return "<unknown>"
	}
	return file.Name.Name
}

// isStandardLibraryPackage checks if a package is part of Go standard library
func isStandardLibraryPackage(pkg string) bool {
	// List of allowed Go standard library packages
	stdLibPackages := map[string]bool{
		// Basic packages
		"fmt":     true,
		"errors":  true,
		"strings": true,
		"strconv": true,
		"sort":    true,
		"reflect": true,

		// Math packages
		"math":      true,
		"math/big":  true,
		"math/rand": true,

		// Time packages
		"time": true,

		// I/O packages
		"io":        true,
		"io/fs":     true,
		"io/ioutil": true,
		"bufio":     true,
		"bytes":     true,

		// Encoding packages
		"encoding/json":   true,
		"encoding/xml":    true,
		"encoding/base64": true,
		"encoding/hex":    true,
		"encoding/csv":    true,

		// Crypto packages
		"crypto":        true,
		"crypto/md5":    true,
		"crypto/sha1":   true,
		"crypto/sha256": true,
		"crypto/sha512": true,
		"crypto/rand":   true,
		"crypto/aes":    true,
		"crypto/des":    true,
		"crypto/hmac":   true,

		// Compression packages
		"compress/gzip":  true,
		"compress/zlib":  true,
		"compress/flate": true,

		// Regular expressions
		"regexp": true,

		// Net packages
		"net":      true,
		"net/url":  true,
		"net/http": true,

		// Path packages
		"path":          true,
		"path/filepath": true,

		// Container packages
		"container/heap": true,
		"container/list": true,
		"container/ring": true,

		// Unicode packages
		"unicode":       true,
		"unicode/utf8":  true,
		"unicode/utf16": true,

		// Context
		"context": true,

		// Sync packages
		"sync":        true,
		"sync/atomic": true,

		// Archive packages
		"archive/tar": true,
		"archive/zip": true,

		// OS packages
		"os":      true,
		"os/exec": true,
		"os/user": true,

		// Log packages
		"log": true,

		// Flag packages
		"flag": true,

		// Template packages
		"text/template":  true,
		"html/template":  true,
		"text/scanner":   true,
		"text/tabwriter": true,

		// Hash packages
		"hash":       true,
		"hash/crc32": true,
		"hash/crc64": true,
		"hash/fnv":   true,

		// Image packages
		"image":       true,
		"image/color": true,
		"image/draw":  true,
		"image/gif":   true,
		"image/jpeg":  true,
		"image/png":   true,

		// Database packages
		"database/sql":        true,
		"database/sql/driver": true,

		// Plugin packages
		"plugin": true,

		// Runtime packages
		"runtime":       true,
		"runtime/debug": true,

		// Unsafe (though should be used carefully)
		"unsafe": true,
	}

	return stdLibPackages[pkg]
}
