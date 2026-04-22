package plugin

import (
	"reflect"
	"strings"
	"testing"

	"github.com/traefik/yaegi/interp"
)

func TestRecoverLoadPanicConvertsPanicToError(t *testing.T) {
	p := &Plugin{
		Name:       "panic-plugin",
		Path:       "/tmp/panic-plugin.go",
		ReturnType: "bool",
		Parameters: []PluginParameter{
			{Name: "arg1", Type: "string", Required: true},
		},
		yaegiIntp: interp.New(interp.Options{}),
		f:         reflect.ValueOf(func() {}),
	}

	var err error

	func() {
		defer p.recoverLoadPanic(&err)
		panic("boom")
	}()

	if err == nil {
		t.Fatal("expected panic to be converted to an error")
	}
	if !strings.Contains(err.Error(), "panic-plugin") {
		t.Fatalf("expected error to include plugin name, got %v", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected error to include panic value, got %v", err)
	}
	if p.yaegiIntp != nil {
		t.Fatal("expected yaegi interpreter to be cleared after panic")
	}
	if p.f.IsValid() {
		t.Fatal("expected function handle to be cleared after panic")
	}
	if p.ReturnType != "" {
		t.Fatalf("expected return type to be cleared after panic, got %q", p.ReturnType)
	}
	if p.Parameters != nil {
		t.Fatalf("expected parameters to be cleared after panic, got %+v", p.Parameters)
	}
}
