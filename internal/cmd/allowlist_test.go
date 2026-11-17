package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/futuretea/etcd-mcp-server/pkg/core/config"
)

func TestToolsListRejectsNonemptyAllBlankAllowLists(t *testing.T) {
	yamlTools := "enabled_tools: [\" \", \"\"]\n"
	yamlDomains := "enabled_domains:\n  - \" \"\n"
	yamlRestricted := "enabled_tools:\n  - etcd_lease_list\n"

	tests := []struct {
		name    string
		yaml    string
		args    []string
		env     string
		setEnv  bool
		wantErr string
	}{
		{
			name:    "yaml tools",
			yaml:    yamlTools,
			wantErr: "enabled_tools",
		},
		{
			name:    "yaml domains",
			yaml:    yamlDomains,
			wantErr: "enabled_domains",
		},
		{
			name:    "flag tools space",
			args:    []string{"--enabled-tools", " "},
			wantErr: "enabled_tools",
		},
		{
			name:    "flag tools comma",
			args:    []string{"--enabled-tools", ","},
			wantErr: "enabled_tools",
		},
		{
			name:    "flag domains space",
			args:    []string{"--enable-domains", " "},
			wantErr: "enabled_domains",
		},
		{
			name:    "env tools space",
			setEnv:  true,
			env:     " ",
			wantErr: "enabled_tools",
		},
		{
			name:    "env tools comma",
			setEnv:  true,
			env:     ",",
			wantErr: "enabled_tools",
		},
		{
			name:    "blank env overrides yaml allow-list",
			yaml:    yamlRestricted,
			setEnv:  true,
			env:     " ",
			wantErr: "enabled_tools",
		},
		{
			name:    "blank flag overrides yaml allow-list",
			yaml:    yamlRestricted,
			args:    []string{"--enabled-tools", " "},
			wantErr: "enabled_tools",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				t.Setenv("MCP_ENABLED_TOOLS", tt.env)
			}
			args := append([]string{}, tt.args...)
			if tt.yaml != "" {
				args = append([]string{"--config", writeConfig(t, tt.yaml)}, args...)
			}
			args = append(args, "tools", "list", "--json")

			var out bytes.Buffer
			root := NewRootCommand(IOStreams{Out: &out, ErrOut: &out})
			root.SetArgs(args)
			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), "non-blank") {
				t.Fatalf("Execute() error = %v, want %s non-blank rejection", err, tt.wantErr)
			}
			if strings.Contains(out.String(), "etcd_get") || strings.Contains(out.String(), "etcd_lease_list") {
				t.Fatalf("all-blank allow-list listed tools:\n%s", out.String())
			}
		})
	}
}

func TestToolsListPreservesEmptyAllowListAndRealNames(t *testing.T) {
	restricted := "enabled_tools:\n  - etcd_lease_list\n"

	tests := []struct {
		name   string
		yaml   string
		args   []string
		env    string
		setEnv bool
		want   []string
	}{
		{
			name: "omitted yaml lists every tool",
			yaml: "log_level: info\n",
			want: approvedCatalog(),
		},
		{
			name: "empty yaml list lists every tool",
			yaml: "enabled_tools: []\n",
			want: approvedCatalog(),
		},
		{
			name:   "empty env keeps yaml allow-list",
			yaml:   restricted,
			setEnv: true,
			env:    "",
			want:   []string{"etcd_lease_list"},
		},
		{
			name: "empty flag clears yaml allow-list",
			yaml: restricted,
			args: []string{"--enabled-tools", ""},
			want: approvedCatalog(),
		},
		{
			name: "blank token beside a real name keeps the name",
			yaml: "enabled_tools:\n  - \" \"\n  - etcd_lease_list\n",
			want: []string{"etcd_lease_list"},
		},
		{
			name: "blank endpoints still list tools",
			yaml: "etcd_endpoints: [\" \", \"\"]\n",
			want: approvedCatalog(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				t.Setenv("MCP_ENABLED_TOOLS", tt.env)
			}
			args := append([]string{}, tt.args...)
			if tt.yaml != "" {
				args = append([]string{"--config", writeConfig(t, tt.yaml)}, args...)
			}
			args = append(args, "tools", "list", "--json")

			names := listedToolNames(t, args)
			if strings.Join(names, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("tools = %v, want %v", names, tt.want)
			}
		})
	}
}

func TestMCPAllBlankAllowListFailsBeforeRuntimeClient(t *testing.T) {
	original := newRuntimeClient
	clients := 0
	newRuntimeClient = func(*config.StaticConfig) (runtimeClient, error) {
		clients++
		return runtimeClient{}, errors.New("client created")
	}
	t.Cleanup(func() { newRuntimeClient = original })

	var out bytes.Buffer
	root := NewRootCommand(IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs([]string{"--enabled-tools", " ", "mcp"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "enabled_tools") || clients != 0 {
		t.Fatalf("Execute() error = %v, clients = %d, want allow-list rejection before a client", err, clients)
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func listedToolNames(t *testing.T, args []string) []string {
	t.Helper()
	var out bytes.Buffer
	root := NewRootCommand(IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(): %v\n%s", err, out.String())
	}
	text := out.String()
	var names []string
	for _, name := range approvedCatalog() {
		if strings.Contains(text, name) {
			names = append(names, name)
		}
	}
	return names
}

func approvedCatalog() []string {
	return []string{
		"etcd_alarm_list",
		"etcd_auth_status",
		"etcd_get",
		"etcd_hash_kv",
		"etcd_lease_list",
		"etcd_lease_ttl",
		"etcd_member_list",
		"etcd_role_get",
		"etcd_role_list",
		"etcd_status",
		"etcd_user_get",
		"etcd_user_list",
	}
}
