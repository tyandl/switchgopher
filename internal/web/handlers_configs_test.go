package web

import (
	"reflect"
	"testing"

	"github.com/tyandl/switchgopher/internal/config"
)

func TestDiffLines(t *testing.T) {
	from := []string{"a", "b", "c"}
	to := []string{"a", "x", "c"}

	got := diffLines(from, to)
	want := []diffLine{
		{"same", "a"},
		{"remove", "b"},
		{"add", "x"},
		{"same", "c"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("diffLines(%v, %v) = %v, want %v", from, to, got, want)
	}
}

func TestDiffLinesIdentical(t *testing.T) {
	lines := []string{"a", "b", "c"}
	got := diffLines(lines, lines)
	for _, l := range got {
		if l.Kind != "same" {
			t.Errorf("identical input produced a %q line: %+v", l.Kind, l)
		}
	}
}

func TestSafeJoinBlocksTraversal(t *testing.T) {
	got := safeJoin("/base", "../../etc/passwd")
	want := "/base/etc/passwd"
	if got != want {
		t.Errorf("safeJoin traversal: got %q, want %q", got, want)
	}
}

func TestSafeJoinAllowsSubdirectory(t *testing.T) {
	got := safeJoin("/base", "Category/Device.txt")
	want := "/base/Category/Device.txt"
	if got != want {
		t.Errorf("safeJoin subdirectory: got %q, want %q", got, want)
	}
}

func TestConfigFolderAllowlist(t *testing.T) {
	config.Preferences = map[string]interface{}{
		"config_dir":        "resources/configs/",
		"dated_config_dir":  "resources/configs/dated/",
		"dns_domain_suffix": "/etc",
	}
	for _, key := range []string{"config_dir", "dated_config_dir"} {
		if _, ok := configFolder(key); !ok {
			t.Errorf("configFolder(%q) should be allowed", key)
		}
	}
	for _, key := range []string{"dns_domain_suffix", "", "nope"} {
		if _, ok := configFolder(key); ok {
			t.Errorf("configFolder(%q) should be refused", key)
		}
	}
}
