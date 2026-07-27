package plugin

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/local_plugin"
	"AgentSmith-HUB/logger"
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"reflect"
	regexpgo "regexp"
	"strings"
	"sync"
	"sync/atomic"

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

	// Whether this plugin instance is in test mode (skip statistics recording)
	IsTestMode bool `json:"is_test_mode"`

	// Status and error handling (consistent with other components)
	Status common.Status `json:"status"`
	Err    error         `json:"-"`

	// Plugin statistics (atomic counters)
	successTotal uint64 // Total successful invocations
	failureTotal uint64 // Total failed invocations

	// Statistics difference counters for increment calculation (like other components)
	lastReportedSuccessTotal uint64 // Last reported success total for increment calculation
	lastReportedFailureTotal uint64 // Last reported failure total for increment calculation
}

// PluginParameter represents a function parameter
type PluginParameter struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}

var Plugins = make(map[string]*Plugin)
var PluginsNew = make(map[string]string)
var PluginsMu sync.RWMutex
var pluginLifecycleMu sync.Mutex

const (
	pluginRedisImportPath  = "AgentSmith-HUB/pluginapi/redis"
	pluginRedisPackagePath = pluginRedisImportPath + "/redis"
)

var (
	pluginRedisGet              = common.RedisGet
	pluginRedisSet              = common.RedisSet
	pluginRedisSetNX            = common.RedisSetNX
	pluginRedisIncrBy           = common.RedisIncrby
	pluginRedisDel              = common.RedisDel
	pluginRedisExpire           = common.RedisExpire
	pluginRedisHSet             = common.RedisHSet
	pluginRedisHGet             = common.RedisHGet
	pluginRedisHGetAll          = common.RedisHGetAll
	pluginRedisHDel             = common.RedisHDel
	pluginRedisLPush            = common.RedisLPush
	pluginRedisLRange           = common.RedisLRange
	pluginRedisSAdd             = common.RedisSAdd
	pluginRedisSRem             = common.RedisSRem
	pluginRedisSMembers         = common.RedisSMembers
	pluginRedisZAdd             = common.RedisZAdd
	pluginRedisZRevRange        = common.RedisZRevRange
	pluginRedisZRemRangeByRank  = common.RedisZRemRangeByRank
	pluginRedisZRemRangeByScore = common.RedisZRemRangeByScore
)

// AgentToolParameters overrides Parameters for plugins used by the agent when the
// plugin has variadic Eval(args ...interface{}) so the LLM gets named args (e.g. rulesetId, ruleContent).
var AgentToolParameters = make(map[string][]PluginParameter)

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
			logger.PluginError("plugin_init error: plugin name conflict, already exists", "plugin", name)
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
			logger.PluginError("plugin_init error: plugin name conflict, already exists", "plugin", name)
		}
	}

	// Agent-facing local plugins use variadic Eval(...interface{}); give them explicit params for the LLM.
	AgentToolParameters["addRule"] = []PluginParameter{
		{Name: "rulesetId", Type: "string", Required: true},
		{Name: "ruleContent", Type: "string", Required: true},
		{Name: "autoApply", Type: "bool", Required: false},
	}
	AgentToolParameters["getConfig"] = []PluginParameter{
		{Name: "componentType", Type: "string", Required: true},
		{Name: "id", Type: "string", Required: false},
	}

	logger.Info("plugin_init", "plugins_count", len(Plugins))
}

// RegisterLLMCallIfConfigured registers the llmCall builtin plugin when config has llm_api_key set.
// Must be called after loadHubConfig (e.g. from main).
func RegisterLLMCallIfConfigured() {
	if !local_plugin.RegisterLLMCallIfConfigured() {
		return
	}
	f := local_plugin.LocalPluginInterfaceAndBoolRes["llmCall"]
	p := &Plugin{
		Name:       "llmCall",
		Type:       LOCAL_PLUGIN,
		Payload:    nil,
		f:          reflect.ValueOf(f),
		ReturnType: "interface{}",
	}
	p.parsePluginParameters()
	PluginsMu.Lock()
	Plugins["llmCall"] = p
	PluginsMu.Unlock()
	logger.Info("plugin_init", "llmCall registered (config llm_api_key set)")
}

func Verify(path string, raw string, name string) error {
	var content []byte
	var err error
	if raw != "" || path == "" {
		content = []byte(raw)
	} else {
		content, err = common.ReadContentFromPathOrRaw(path, "")
		if err != nil {
			return fmt.Errorf("failed to read plugin configuration: %w", err)
		}
	}

	if _, err = inspectPluginCode(string(content)); err != nil {
		return err
	}

	p := &Plugin{Name: name, Path: path, Payload: content, Type: YAEGI_PLUGIN}
	return p.yaegiCompile()
}

// Inspect validates a plugin without executing package initializers and returns
// the metadata needed by read-only API endpoints.
func Inspect(path string, raw string, name string) ([]PluginParameter, string, error) {
	content, err := common.ReadContentFromPathOrRaw(path, raw)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read plugin configuration: %w", err)
	}

	info, err := inspectPluginCode(string(content))
	if err != nil {
		return nil, "", err
	}

	p := &Plugin{Name: name, Path: path, Payload: content, Type: YAEGI_PLUGIN}
	if err := p.yaegiCompile(); err != nil {
		return nil, "", err
	}

	return info.Parameters, info.ReturnType, nil
}

func NewPlugin(path string, raw string, name string, pluginType int) error {
	return InstallPlugin(path, raw, name, pluginType, nil)
}

// InstallPlugin serializes plugin lifecycle changes. The optional commit callback
// runs after the candidate has initialized successfully and before it becomes
// visible in the registry.
func InstallPlugin(path string, raw string, name string, pluginType int, commit func() error) error {
	var content []byte
	var err error
	if raw != "" || path == "" {
		content = []byte(raw)
	} else {
		content, err = common.ReadContentFromPathOrRaw(path, "")
		if err != nil {
			return fmt.Errorf("failed to read plugin configuration: %w", err)
		}
	}

	pluginLifecycleMu.Lock()
	defer pluginLifecycleMu.Unlock()

	PluginsMu.RLock()
	current, exists := Plugins[name]
	PluginsMu.RUnlock()
	if exists && current != nil && current.Type == LOCAL_PLUGIN && pluginType != LOCAL_PLUGIN {
		return fmt.Errorf("plugin name %q is reserved by a built-in plugin", name)
	}

	// Cluster retries and duplicate deliveries must not execute package init
	// again when the same plugin version is already active.
	if exists && current != nil && current.Type == pluginType && current.Err == nil && bytes.Equal(current.Payload, content) {
		if commit != nil {
			return commit()
		}
		return nil
	}

	p := &Plugin{Path: path, Payload: content, Type: pluginType, Name: name}
	if err := p.yaegiLoad(); err != nil {
		return fmt.Errorf("plugin yaegi load err %s: %w", name, err)
	}
	if commit != nil {
		if err := commit(); err != nil {
			return err
		}
	}

	PluginsMu.Lock()
	Plugins[p.Name] = p
	PluginsMu.Unlock()
	return nil
}

// NewTestPlugin creates a plugin for testing without adding it to the global registry
func NewTestPlugin(path string, raw string, name string, pluginType int) (*Plugin, error) {
	content, err := common.ReadContentFromPathOrRaw(path, raw)
	if err != nil {
		return nil, fmt.Errorf("failed to read plugin configuration: %w", err)
	}

	p := &Plugin{Path: path, Payload: content, Type: pluginType, Name: name, IsTestMode: true}
	if err := p.yaegiLoad(); err != nil {
		return nil, fmt.Errorf("plugin yaegi load err %s: %w", name, err)
	}
	// For test plugins, do NOT add to global registry
	return p, nil
}

func (p *Plugin) pluginIdentifier() string {
	if p.Name != "" {
		return p.Name
	}
	if p.Path != "" {
		return p.Path
	}
	return "<inline plugin>"
}

func (p *Plugin) resetLoadState() {
	p.yaegiIntp = nil
	p.f = reflect.Value{}
	p.Parameters = nil
	p.ReturnType = ""
}

func (p *Plugin) recoverLoadPanic(err *error) {
	if r := recover(); r != nil {
		p.resetLoadState()
		pluginID := p.pluginIdentifier()
		logger.PluginError("plugin yaegi load panicked", "plugin", pluginID, "panic", r)
		*err = fmt.Errorf("plugin yaegi load panicked for %s: %v", pluginID, r)
	}
}

func (p *Plugin) newInterpreter() (*interp.Interpreter, error) {
	i := interp.New(interp.Options{})
	if err := i.Use(stdlib.Symbols); err != nil {
		return nil, err
	}
	if err := i.Use(p.redisSymbols()); err != nil {
		return nil, err
	}
	return i, nil
}

// yaegiCompile validates that Yaegi can compile the plugin without executing
// package variables or init functions.
func (p *Plugin) yaegiCompile() (err error) {
	defer p.recoverLoadPanic(&err)

	i, err := p.newInterpreter()
	if err != nil {
		return err
	}
	_, err = i.Compile(string(p.Payload))
	return err
}

func (p *Plugin) yaegiLoad() (err error) {
	defer p.recoverLoadPanic(&err)
	defer func() {
		if err != nil {
			p.resetLoadState()
		}
	}()

	if _, err := inspectPluginCode(string(p.Payload)); err != nil {
		return err
	}

	p.yaegiIntp, err = p.newInterpreter()
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

func IsBuiltinName(name string) bool {
	if name == "llmCall" {
		return true
	}
	if _, ok := local_plugin.LocalPluginBoolRes[name]; ok {
		return true
	}
	_, ok := local_plugin.LocalPluginInterfaceAndBoolRes[name]
	return ok
}

type pluginSourceInfo struct {
	Parameters []PluginParameter
	ReturnType string
}

func inspectPluginCode(source string) (*pluginSourceInfo, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", source, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse plugin code: %w", err)
	}

	if file.Name == nil || file.Name.Name != "plugin" {
		return nil, fmt.Errorf("plugin package must be 'plugin', found: %s", getPackageName(file))
	}

	for _, importSpec := range file.Imports {
		if importSpec.Path == nil {
			continue
		}
		importPath := strings.Trim(importSpec.Path.Value, `"`)
		if importPath == pluginRedisImportPath {
			continue
		}
		if !isStandardLibraryPackage(importPath) {
			return nil, fmt.Errorf("plugin can only import Go standard library packages, found external package: %s", importPath)
		}
	}

	var evalFunc *ast.FuncDecl
	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if ok && funcDecl.Recv == nil && funcDecl.Name.Name == "Eval" {
			evalFunc = funcDecl
			break
		}
	}
	if evalFunc == nil {
		return nil, fmt.Errorf("plugin must contain an 'Eval' function")
	}

	return inspectEvalSignature(fset, evalFunc)
}

func inspectEvalSignature(fset *token.FileSet, evalFunc *ast.FuncDecl) (*pluginSourceInfo, error) {
	resultTypes := flattenFieldTypes(evalFunc.Type.Results)
	if len(resultTypes) != 2 && len(resultTypes) != 3 {
		return nil, fmt.Errorf(
			"plugin Eval function must return 2 values (bool, error) or 3 values (interface{}, bool, error), but returns %d values",
			len(resultTypes),
		)
	}

	if len(resultTypes) == 2 {
		if isDefinitelyWrongBuiltin(resultTypes[0], "bool") {
			return nil, fmt.Errorf("plugin Eval function with 2 return values must have first return value as bool")
		}
		if isDefinitelyWrongBuiltin(resultTypes[1], "error") {
			return nil, fmt.Errorf("plugin Eval function second return value must be error")
		}
	} else {
		if isDefinitelyWrongBuiltin(resultTypes[1], "bool") {
			return nil, fmt.Errorf("plugin Eval function with 3 return values must have second return value as bool")
		}
		if isDefinitelyWrongBuiltin(resultTypes[2], "error") {
			return nil, fmt.Errorf("plugin Eval function third return value must be error")
		}
	}

	returnType := "interface{}"
	if len(resultTypes) == 2 {
		returnType = "bool"
	}

	return &pluginSourceInfo{
		Parameters: inspectParameters(fset, evalFunc.Type.Params),
		ReturnType: returnType,
	}, nil
}

func flattenFieldTypes(fields *ast.FieldList) []ast.Expr {
	if fields == nil {
		return nil
	}

	var result []ast.Expr
	for _, field := range fields.List {
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		for i := 0; i < count; i++ {
			result = append(result, field.Type)
		}
	}
	return result
}

func isDefinitelyWrongBuiltin(expr ast.Expr, expected string) bool {
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return false
	}

	builtins := map[string]bool{
		"any": true, "bool": true, "byte": true, "complex64": true, "complex128": true,
		"error": true, "float32": true, "float64": true, "int": true, "int8": true,
		"int16": true, "int32": true, "int64": true, "rune": true, "string": true,
		"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
		"uintptr": true,
	}
	return builtins[ident.Name] && ident.Name != expected
}

func inspectParameters(fset *token.FileSet, fields *ast.FieldList) []PluginParameter {
	if fields == nil {
		return []PluginParameter{}
	}

	parameters := make([]PluginParameter, 0, len(fields.List))
	argIndex := 1
	for _, field := range fields.List {
		typeName := formatASTType(fset, field.Type)
		_, variadic := field.Type.(*ast.Ellipsis)
		required := !variadic

		if len(field.Names) == 0 {
			parameters = append(parameters, PluginParameter{
				Name:     fmt.Sprintf("arg%d", argIndex),
				Type:     typeName,
				Required: required,
			})
			argIndex++
			continue
		}

		for _, name := range field.Names {
			parameters = append(parameters, PluginParameter{
				Name:     name.Name,
				Type:     typeName,
				Required: required,
			})
			argIndex++
		}
	}
	return parameters
}

func formatASTType(fset *token.FileSet, expr ast.Expr) string {
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, expr); err != nil {
		return "interface{}"
	}
	return buf.String()
}

func (p *Plugin) redisSymbols() interp.Exports {
	return interp.Exports{
		pluginRedisPackagePath: {
			"Get": reflect.ValueOf(func(key string) (string, error) {
				scopedKey, err := p.scopedRedisKey(key)
				if err != nil {
					return "", err
				}
				return pluginRedisGet(scopedKey)
			}),
			"Set": reflect.ValueOf(func(key string, value interface{}, expiration int) (string, error) {
				scopedKey, err := p.scopedRedisKey(key)
				if err != nil {
					return "", err
				}
				return pluginRedisSet(scopedKey, value, expiration)
			}),
			"SetNX": reflect.ValueOf(func(key string, value interface{}, expiration int) (bool, error) {
				scopedKey, err := p.scopedRedisKey(key)
				if err != nil {
					return false, err
				}
				return pluginRedisSetNX(scopedKey, value, expiration)
			}),
			"Incr": reflect.ValueOf(func(key string) (int64, error) {
				scopedKey, err := p.scopedRedisKey(key)
				if err != nil {
					return 0, err
				}
				return pluginRedisIncrBy(scopedKey, 1)
			}),
			"IncrBy": reflect.ValueOf(func(key string, value int64) (int64, error) {
				scopedKey, err := p.scopedRedisKey(key)
				if err != nil {
					return 0, err
				}
				return pluginRedisIncrBy(scopedKey, value)
			}),
			"Del": reflect.ValueOf(func(key string) error {
				scopedKey, err := p.scopedRedisKey(key)
				if err != nil {
					return err
				}
				return pluginRedisDel(scopedKey)
			}),
			"Expire": reflect.ValueOf(func(key string, expiration int) error {
				scopedKey, err := p.scopedRedisKey(key)
				if err != nil {
					return err
				}
				return pluginRedisExpire(scopedKey, expiration)
			}),
			"HSet": reflect.ValueOf(func(key string, field string, value interface{}) error {
				scopedKey, err := p.scopedRedisKey(key)
				if err != nil {
					return err
				}
				return pluginRedisHSet(scopedKey, field, value)
			}),
			"HGet": reflect.ValueOf(func(key string, field string) (string, error) {
				scopedKey, err := p.scopedRedisKey(key)
				if err != nil {
					return "", err
				}
				return pluginRedisHGet(scopedKey, field)
			}),
			"HGetAll": reflect.ValueOf(func(key string) (map[string]string, error) {
				scopedKey, err := p.scopedRedisKey(key)
				if err != nil {
					return nil, err
				}
				return pluginRedisHGetAll(scopedKey)
			}),
			"HDel": reflect.ValueOf(func(key string, field string) error {
				scopedKey, err := p.scopedRedisKey(key)
				if err != nil {
					return err
				}
				return pluginRedisHDel(scopedKey, field)
			}),
			"LPush": reflect.ValueOf(func(key string, value interface{}, maxLen int64) error {
				scopedKey, err := p.scopedRedisKey(key)
				if err != nil {
					return err
				}
				return pluginRedisLPush(scopedKey, value, maxLen)
			}),
			"LRange": reflect.ValueOf(func(key string, start int64, stop int64) ([]string, error) {
				scopedKey, err := p.scopedRedisKey(key)
				if err != nil {
					return nil, err
				}
				return pluginRedisLRange(scopedKey, start, stop)
			}),
			"SAdd": reflect.ValueOf(func(key string, member interface{}) (int64, error) {
				scopedKey, err := p.scopedRedisKey(key)
				if err != nil {
					return 0, err
				}
				return pluginRedisSAdd(scopedKey, member)
			}),
			"SRem": reflect.ValueOf(func(key string, member interface{}) (int64, error) {
				scopedKey, err := p.scopedRedisKey(key)
				if err != nil {
					return 0, err
				}
				return pluginRedisSRem(scopedKey, member)
			}),
			"SMembers": reflect.ValueOf(func(key string) ([]string, error) {
				scopedKey, err := p.scopedRedisKey(key)
				if err != nil {
					return nil, err
				}
				return pluginRedisSMembers(scopedKey)
			}),
			"ZAdd": reflect.ValueOf(func(key string, score float64, member interface{}) (int64, error) {
				scopedKey, err := p.scopedRedisKey(key)
				if err != nil {
					return 0, err
				}
				return pluginRedisZAdd(scopedKey, score, member)
			}),
			"ZRevRange": reflect.ValueOf(func(key string, start int64, stop int64) ([]string, error) {
				scopedKey, err := p.scopedRedisKey(key)
				if err != nil {
					return nil, err
				}
				return pluginRedisZRevRange(scopedKey, start, stop)
			}),
			"ZRemRangeByRank": reflect.ValueOf(func(key string, start int64, stop int64) (int64, error) {
				scopedKey, err := p.scopedRedisKey(key)
				if err != nil {
					return 0, err
				}
				return pluginRedisZRemRangeByRank(scopedKey, start, stop)
			}),
			"ZRemRangeByScore": reflect.ValueOf(func(key string, min string, max string) (int64, error) {
				scopedKey, err := p.scopedRedisKey(key)
				if err != nil {
					return 0, err
				}
				return pluginRedisZRemRangeByScore(scopedKey, min, max)
			}),
		},
	}
}

func (p *Plugin) scopedRedisKey(key string) (string, error) {
	if p.Name == "" {
		return "", fmt.Errorf("plugin redis key namespace requires plugin name")
	}
	if key == "" {
		return "", fmt.Errorf("plugin redis key cannot be empty")
	}
	return "plugin:" + p.Name + ":" + key, nil
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

			p.RecordInvocation(err == nil)
			return result, err
		} else {
			err := fmt.Errorf("local plugin not found: %s", p.Name)
			logger.PluginError("local plugin not found", "plugin", p.Name)
			p.RecordInvocation(false)
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

		p.RecordInvocation(err == nil)
		return result, err
	}
	p.RecordInvocation(false)
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
					logger.PluginError("Local plugin returned error", "plugin", p.Name, "error", err)
				}
			}()

			p.RecordInvocation(err == nil)
			return result, success, err
		} else {
			err := fmt.Errorf("local plugin not found: %s", p.Name)
			logger.PluginError("local plugin not found", "plugin", p.Name)
			p.RecordInvocation(false)
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

		p.RecordInvocation(err == nil)
		return result, success, err
	}
	p.RecordInvocation(false)
	return nil, false, fmt.Errorf("unknown plugin type")
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

// RecordInvocation increments the appropriate counter based on success/failure
// Skip recording if plugin is in test mode
func (p *Plugin) RecordInvocation(success bool) {
	// Skip statistics recording for test mode plugins
	if p.IsTestMode {
		return
	}

	if success {
		atomic.AddUint64(&p.successTotal, 1)
	} else {
		atomic.AddUint64(&p.failureTotal, 1)
	}
}

// GetSuccessIncrementAndUpdate returns the increment in success count since last call and updates the baseline
// This method is thread-safe and designed for statistics collection.
// Uses CAS operation to ensure atomicity.
func (p *Plugin) GetSuccessIncrementAndUpdate() uint64 {
	currentTotal := atomic.LoadUint64(&p.successTotal)
	lastReported := atomic.LoadUint64(&p.lastReportedSuccessTotal)

	// Use CAS to atomically update lastReportedSuccessTotal
	// If CAS fails, we simply return 0 - one missed stat collection is not critical
	if atomic.CompareAndSwapUint64(&p.lastReportedSuccessTotal, lastReported, currentTotal) {
		return currentTotal - lastReported
	}

	return 0
}

// GetFailureIncrementAndUpdate returns the increment in failure count since last call and updates the baseline
// This method is thread-safe and designed for statistics collection.
// Uses CAS operation to ensure atomicity.
func (p *Plugin) GetFailureIncrementAndUpdate() uint64 {
	currentTotal := atomic.LoadUint64(&p.failureTotal)
	lastReported := atomic.LoadUint64(&p.lastReportedFailureTotal)

	// Use CAS to atomically update lastReportedFailureTotal
	// If CAS fails, we simply return 0 - one missed stat collection is not critical
	if atomic.CompareAndSwapUint64(&p.lastReportedFailureTotal, lastReported, currentTotal) {
		return currentTotal - lastReported
	}

	return 0
} // ResetSuccessTotal resets the success counter and baseline (for restart scenarios)
func (p *Plugin) ResetSuccessTotal() {
	atomic.StoreUint64(&p.successTotal, 0)
	atomic.StoreUint64(&p.lastReportedSuccessTotal, 0)
}

// ResetFailureTotal resets the failure counter and baseline (for restart scenarios)
func (p *Plugin) ResetFailureTotal() {
	atomic.StoreUint64(&p.failureTotal, 0)
	atomic.StoreUint64(&p.lastReportedFailureTotal, 0)
}

// ResetAllStats resets all statistics counters (for restart scenarios)
func (p *Plugin) ResetAllStats() {
	p.ResetSuccessTotal()
	p.ResetFailureTotal()
}

// SafeDeletePlugin safely deletes a plugin with all necessary validations and locking.
func SafeDeletePlugin(id string) ([]string, error) {
	return SafeDeletePluginWithCommit(id, nil)
}

// SafeDeletePluginWithCommit performs persistence work before removing the
// runtime instance, while holding the lifecycle lock against concurrent installs.
func SafeDeletePluginWithCommit(id string, commit func() error) ([]string, error) {
	pluginLifecycleMu.Lock()
	defer pluginLifecycleMu.Unlock()

	// Check if component exists
	PluginsMu.RLock()
	pluginInstance, componentExists := Plugins[id]
	_, tempExists := PluginsNew[id]
	PluginsMu.RUnlock()
	if !componentExists {
		// Check if only exists in temporary storage
		if !tempExists {
			return nil, fmt.Errorf("plugin not found: %s", id)
		}
		if commit != nil {
			if err := commit(); err != nil {
				return nil, err
			}
		}
		// Only exists in temp, just remove from temp
		PluginsMu.Lock()
		delete(PluginsNew, id)
		PluginsMu.Unlock()
		common.DeleteRawConfig("plugin", id)
		return []string{}, nil
	}
	if pluginInstance == nil || pluginInstance.Type == LOCAL_PLUGIN {
		return nil, fmt.Errorf("built-in plugin %s cannot be deleted", id)
	}

	if commit != nil {
		if err := commit(); err != nil {
			return nil, err
		}
	}

	// Check if used by any ruleset (plugins are used in rulesets)
	affectedProjects := make([]string, 0)

	// For plugins, we need to check if they are referenced in any rulesets
	// This is more complex as plugins are referenced by name in XML content
	// But we don't want to parse all rulesets here for performance reasons
	// Instead, we'll just return an empty list and let the caller handle restarts

	// Reset statistics before deleting plugin
	pluginInstance.ResetAllStats()
	logger.Debug("Reset plugin statistics during deletion", "plugin", id)

	// Remove from global mappings
	PluginsMu.Lock()
	delete(Plugins, id)
	delete(PluginsNew, id)
	PluginsMu.Unlock()
	common.DeleteRawConfig("plugin", id)

	return affectedProjects, nil
}

// RestorePluginAfterFailedDelete restores the exact previous instance without
// executing its initializer again. It is only for rolling back a failed
// persistence/publication step immediately after SafeDeletePlugin.
func RestorePluginAfterFailedDelete(p *Plugin, commit func() error) error {
	if p == nil {
		return fmt.Errorf("cannot restore nil plugin")
	}
	if p.Type == LOCAL_PLUGIN {
		return fmt.Errorf("built-in plugins do not use delete rollback")
	}

	pluginLifecycleMu.Lock()
	defer pluginLifecycleMu.Unlock()

	PluginsMu.RLock()
	_, exists := Plugins[p.Name]
	PluginsMu.RUnlock()
	if exists {
		return fmt.Errorf("plugin %s was replaced while delete was being published", p.Name)
	}
	if commit != nil {
		if err := commit(); err != nil {
			return err
		}
	}

	PluginsMu.Lock()
	Plugins[p.Name] = p
	PluginsMu.Unlock()
	common.SetRawConfig("plugin", p.Name, string(p.Payload))
	return nil
}

// ResetManagedPluginsForResync drops only leader-managed runtime plugins while
// preserving built-in local plugins registered at process startup.
func ResetManagedPluginsForResync() {
	pluginLifecycleMu.Lock()
	defer pluginLifecycleMu.Unlock()
	PluginsMu.Lock()
	defer PluginsMu.Unlock()

	for id, p := range Plugins {
		if p == nil {
			delete(Plugins, id)
			continue
		}
		if p.Type != LOCAL_PLUGIN {
			delete(Plugins, id)
		}
	}

	PluginsNew = make(map[string]string)
}

// Safe accessor functions for PluginsNew map
func SetPluginNew(id, content string) {
	PluginsMu.Lock()
	defer PluginsMu.Unlock()
	PluginsNew[id] = content
}

func DeletePluginNew(id string) {
	PluginsMu.Lock()
	defer PluginsMu.Unlock()
	delete(PluginsNew, id)
}

func GetAllPluginsNew() map[string]string {
	PluginsMu.RLock()
	defer PluginsMu.RUnlock()

	result := make(map[string]string)
	for id, content := range PluginsNew {
		result[id] = content
	}
	return result
}

func GetPlugin(id string) (*Plugin, bool) {
	PluginsMu.RLock()
	defer PluginsMu.RUnlock()
	p, ok := Plugins[id]
	return p, ok
}

func GetPluginNew(id string) (string, bool) {
	PluginsMu.RLock()
	defer PluginsMu.RUnlock()
	content, ok := PluginsNew[id]
	return content, ok
}

func GetAllPlugins() map[string]*Plugin {
	PluginsMu.RLock()
	defer PluginsMu.RUnlock()

	result := make(map[string]*Plugin, len(Plugins))
	for id, p := range Plugins {
		result[id] = p
	}
	return result
}

// SetPluginErrorIfAbsent records startup load failures without replacing a
// successfully registered built-in or managed plugin.
func SetPluginErrorIfAbsent(id string, p *Plugin) bool {
	PluginsMu.Lock()
	defer PluginsMu.Unlock()
	if _, exists := Plugins[id]; exists {
		return false
	}
	Plugins[id] = p
	return true
}
