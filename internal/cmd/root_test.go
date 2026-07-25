package cmd_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/futuretea/etcd-mcp-server/internal/cmd"
)

func TestRootHelpListsSubcommands(t *testing.T) {
	var out bytes.Buffer
	root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("help: %v", err)
	}
	text := out.String()
	for _, want := range []string{"mcp", "tools", "version", "completion"} {
		if !strings.Contains(text, want) {
			t.Fatalf("help missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(strings.ToLower(text), "template") || !strings.Contains(text, "etcd") {
		t.Fatalf("root help must present the etcd server identity:\n%s", text)
	}
}

func TestRootWithoutArgsShowsHelp(t *testing.T) {
	var out bytes.Buffer
	root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs([]string{})
	if err := root.Execute(); err != nil {
		t.Fatalf("root with no args: %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "Available Commands") {
		t.Fatalf("expected help output, got:\n%s", text)
	}
	if strings.Contains(text, "registered MCP tools") || strings.Contains(text, "starting HTTP MCP server") {
		t.Fatalf("root must not start the MCP server:\n%s", text)
	}
}

// Seam: CLI list is schema-only and reads the same approved catalogue as MCP;
// it must not expose the removed template tools.
func TestToolsListShowsApprovedEtcdCatalog(t *testing.T) {
	var out bytes.Buffer
	root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs([]string{"tools", "list", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("tools list: %v", err)
	}
	var listed []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(out.Bytes(), &listed); err != nil {
		t.Fatalf("decode tools list: %v\n%s", err, out.String())
	}
	want := approvedToolNames()
	if len(listed) != len(want) {
		t.Fatalf("tools list count = %d, want %d: %s", len(listed), len(want), out.String())
	}
	seen := make(map[string]bool, len(listed))
	for _, tool := range listed {
		seen[tool.Name] = true
	}
	for _, name := range want {
		if !seen[name] {
			t.Fatalf("tools list missing %q: %s", name, out.String())
		}
	}
}

// Seam: the CLI describes the same catalogue consumed by the MCP server. Each
// approved tool must expose a JSON input schema through that public entry point.
func TestToolsDescribeExposesSchemasForApprovedCatalog(t *testing.T) {
	for _, name := range approvedToolNames() {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
			root.SetArgs([]string{"tools", "describe", name, "--json"})
			if err := root.Execute(); err != nil {
				t.Fatalf("tools describe: %v", err)
			}
			var described struct {
				Name        string         `json:"name"`
				InputSchema map[string]any `json:"inputSchema"`
			}
			if err := json.Unmarshal(out.Bytes(), &described); err != nil {
				t.Fatalf("decode tools describe: %v\n%s", err, out.String())
			}
			if described.Name != name || described.InputSchema == nil {
				t.Fatalf("tools describe = %+v, want %q with input schema", described, name)
			}
		})
	}
}

func TestToolsCallRejectsTrailingJSONBeforeClientCreation(t *testing.T) {
	var out bytes.Buffer
	root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs([]string{"tools", "call", "etcd_lease_list", "--params", "{}{}"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "single JSON object") {
		t.Fatalf("trailing JSON error = %v, want parser rejection", err)
	}
}

func TestVersionCommand(t *testing.T) {
	var out bytes.Buffer
	root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs([]string{"version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("version: %v", err)
	}
	if !strings.Contains(out.String(), "etcd-mcp-server") {
		t.Fatalf("unexpected version output: %q", out.String())
	}
}

func TestToolsUnknownTool(t *testing.T) {
	var out bytes.Buffer
	root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs([]string{"tools", "describe", "missing-tool"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Fatalf("expected unknown tool error, got %v", err)
	}

	root = cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs([]string{"tools", "call", "missing-tool", "--params", "{}"})
	err = root.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Fatalf("expected unknown tool error on call, got %v", err)
	}
}

func TestToolsRejectsNullParams(t *testing.T) {
	var out bytes.Buffer
	root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs([]string{"tools", "call", "etcd_lease_list", "--params", "null"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "expected an object") {
		t.Fatalf("expected JSON object error, got %v", err)
	}
}

func TestCommandsReturnOutputErrors(t *testing.T) {
	writeErr := errors.New("write failed")
	tests := []struct {
		name string
		args []string
	}{
		{name: "completion script", args: []string{"completion", "bash"}},
		{name: "completion install", args: []string{"completion", "install"}},
		{name: "version", args: []string{"version"}},
		{name: "tools list", args: []string{"tools", "list"}},
		{name: "tools describe", args: []string{"tools", "describe", "etcd_get"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := cmd.NewRootCommand(cmd.IOStreams{
				In:     strings.NewReader(""),
				Out:    errorWriter{err: writeErr},
				ErrOut: io.Discard,
			})
			root.SetArgs(tt.args)
			if err := root.Execute(); !errors.Is(err, writeErr) {
				t.Fatalf("Execute() error = %v, want %v", err, writeErr)
			}
		})
	}
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func approvedToolNames() []string {
	return []string{
		"etcd_get", "etcd_lease_ttl", "etcd_lease_list", "etcd_member_list",
		"etcd_alarm_list", "etcd_status", "etcd_hash_kv", "etcd_auth_status",
		"etcd_user_get", "etcd_user_list", "etcd_role_get", "etcd_role_list",
	}
}
