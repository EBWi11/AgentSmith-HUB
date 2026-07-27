package plugin

import (
	"fmt"
	"os"
	"sync"
	"testing"
)

func initCountingPlugin(marker string) string {
	return fmt.Sprintf(`package plugin

import "os"

func init() {
	f, err := os.OpenFile(%q, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err == nil {
		_, _ = f.WriteString("x")
		_ = f.Close()
	}
}

func Eval(value string) (bool, error) {
	return value != "", nil
}`, marker)
}

func assertPluginInitCount(t *testing.T, marker string, expected string) {
	t.Helper()

	content, err := os.ReadFile(marker)
	if expected == "" && os.IsNotExist(err) {
		return
	}
	if err != nil {
		t.Fatalf("read initialization marker: %v", err)
	}
	if got := string(content); got != expected {
		t.Fatalf("unexpected plugin initialization count: got marker %q, want %q", got, expected)
	}
}

func TestVerifyAndInspectDoNotInitializePlugin(t *testing.T) {
	marker := t.TempDir() + "/verify-init-count.txt"
	raw := initCountingPlugin(marker)

	if err := Verify("", raw, "verify_init_once"); err != nil {
		t.Fatalf("verify plugin: %v", err)
	}
	assertPluginInitCount(t, marker, "")

	parameters, returnType, err := Inspect("", raw, "inspect_init_once")
	if err != nil {
		t.Fatalf("inspect plugin: %v", err)
	}
	if len(parameters) != 1 || parameters[0].Name != "value" || parameters[0].Type != "string" {
		t.Fatalf("unexpected inspected parameters: %+v", parameters)
	}
	if returnType != "bool" {
		t.Fatalf("unexpected inspected return type: %q", returnType)
	}
	assertPluginInitCount(t, marker, "")
}

func TestNewPluginInitializesPluginOnce(t *testing.T) {
	const pluginName = "production_init_once"
	marker := t.TempDir() + "/production-init-count.txt"

	PluginsMu.Lock()
	previous, existed := Plugins[pluginName]
	delete(Plugins, pluginName)
	PluginsMu.Unlock()
	t.Cleanup(func() {
		PluginsMu.Lock()
		defer PluginsMu.Unlock()
		if existed {
			Plugins[pluginName] = previous
		} else {
			delete(Plugins, pluginName)
		}
	})

	if err := NewPlugin("", initCountingPlugin(marker), pluginName, YAEGI_PLUGIN); err != nil {
		t.Fatalf("load production plugin: %v", err)
	}
	assertPluginInitCount(t, marker, "x")
}

func TestNewTestPluginInitializesIsolatedPluginOnce(t *testing.T) {
	marker := t.TempDir() + "/test-init-count.txt"

	if _, err := NewTestPlugin("", initCountingPlugin(marker), "test_init_once", YAEGI_PLUGIN); err != nil {
		t.Fatalf("load test plugin: %v", err)
	}
	assertPluginInitCount(t, marker, "x")
}

func TestConcurrentIdenticalPluginInstallInitializesOnce(t *testing.T) {
	const pluginName = "concurrent_init_once"
	marker := t.TempDir() + "/concurrent-init-count.txt"
	raw := initCountingPlugin(marker)

	PluginsMu.Lock()
	previous, existed := Plugins[pluginName]
	delete(Plugins, pluginName)
	PluginsMu.Unlock()
	t.Cleanup(func() {
		PluginsMu.Lock()
		defer PluginsMu.Unlock()
		if existed {
			Plugins[pluginName] = previous
		} else {
			delete(Plugins, pluginName)
		}
	})

	const callers = 12
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- NewPlugin("", raw, pluginName, YAEGI_PLUGIN)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent install failed: %v", err)
		}
	}

	assertPluginInitCount(t, marker, "x")
}

func TestBuiltinPluginCannotBeReplaced(t *testing.T) {
	if err := NewPlugin("", "package plugin\nfunc Eval() (bool, error) { return true, nil }", "addRule", YAEGI_PLUGIN); err == nil {
		t.Fatal("expected built-in plugin replacement to be rejected")
	}
}

func TestDeleteRollbackRestoresInstanceWithoutInitializingAgain(t *testing.T) {
	const pluginName = "delete_rollback_init_once"
	marker := t.TempDir() + "/delete-rollback-init-count.txt"

	PluginsMu.Lock()
	previous, existed := Plugins[pluginName]
	delete(Plugins, pluginName)
	PluginsMu.Unlock()
	t.Cleanup(func() {
		PluginsMu.Lock()
		defer PluginsMu.Unlock()
		if existed {
			Plugins[pluginName] = previous
		} else {
			delete(Plugins, pluginName)
		}
	})

	if err := NewPlugin("", initCountingPlugin(marker), pluginName, YAEGI_PLUGIN); err != nil {
		t.Fatalf("load plugin: %v", err)
	}
	active, _ := GetPlugin(pluginName)
	if _, err := SafeDeletePlugin(pluginName); err != nil {
		t.Fatalf("delete plugin: %v", err)
	}
	if err := RestorePluginAfterFailedDelete(active, nil); err != nil {
		t.Fatalf("restore plugin: %v", err)
	}

	restored, _ := GetPlugin(pluginName)
	if restored != active {
		t.Fatal("delete rollback did not restore the exact plugin instance")
	}
	assertPluginInitCount(t, marker, "x")
}
