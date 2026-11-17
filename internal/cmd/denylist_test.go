package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/futuretea/etcd-mcp-server/pkg/core/config"
)

func TestToolsListRejectsNonemptyAllBlankDenyLists(t *testing.T) {
	yamlTool := "disabled_tools:\n  - etcd_get\n"
	yamlDomain := "disabled_domains:\n  - etcd\n"

	tests := []struct {
		name    string
		yaml    string
		args    []string
		envKey  string
		envVal  string
		wantErr string
	}{
		{
			name:    "yaml tools",
			yaml:    "disabled_tools: [\" \", \"\"]\n",
			wantErr: "disabled_tools",
		},
		{
			name:    "yaml domains",
			yaml:    "disabled_domains:\n  - \" \"\n",
			wantErr: "disabled_domains",
		},
		{
			name:    "blank env overrides yaml tool denial",
			yaml:    yamlTool,
			envKey:  "MCP_DISABLED_TOOLS",
			envVal:  " ",
			wantErr: "disabled_tools",
		},
		{
			name:    "comma env overrides yaml tool denial",
			yaml:    yamlTool,
			envKey:  "MCP_DISABLED_TOOLS",
			envVal:  ",",
			wantErr: "disabled_tools",
		},
		{
			name:    "blank flag overrides yaml tool denial",
			yaml:    yamlTool,
			args:    []string{"--disabled-tools", " "},
			wantErr: "disabled_tools",
		},
		{
			name:    "comma flag overrides yaml tool denial",
			yaml:    yamlTool,
			args:    []string{"--disabled-tools", ","},
			wantErr: "disabled_tools",
		},
		{
			name:    "blank env overrides yaml domain denial",
			yaml:    yamlDomain,
			envKey:  "MCP_DISABLED_DOMAINS",
			envVal:  " ",
			wantErr: "disabled_domains",
		},
		{
			name:    "blank flag overrides yaml domain denial",
			yaml:    yamlDomain,
			args:    []string{"--disable-domains", " "},
			wantErr: "disabled_domains",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envKey != "" {
				t.Setenv(tt.envKey, tt.envVal)
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
			if strings.Contains(out.String(), "etcd_get") {
				t.Fatalf("all-blank deny-list exposed etcd_get:\n%s", out.String())
			}
		})
	}
}

func TestToolsListPreservesRealDenyLists(t *testing.T) {
	yamlTool := "disabled_tools:\n  - etcd_get\n"
	yamlDomain := "disabled_domains:\n  - etcd\n"

	tests := []struct {
		name      string
		yaml      string
		args      []string
		envKey    string
		envVal    string
		wantGet   bool
		wantOther bool
	}{
		{
			name:      "yaml tool denial hides etcd_get",
			yaml:      yamlTool,
			wantOther: true,
		},
		{
			name:      "empty deny list hides nothing",
			yaml:      "disabled_tools: []\n",
			wantGet:   true,
			wantOther: true,
		},
		{
			name:      "empty env keeps yaml tool denial",
			yaml:      yamlTool,
			envKey:    "MCP_DISABLED_TOOLS",
			wantOther: true,
		},
		{
			name:      "empty flag clears yaml tool denial",
			yaml:      yamlTool,
			args:      []string{"--disabled-tools", ""},
			wantGet:   true,
			wantOther: true,
		},
		{
			name:      "blank token beside a denied name keeps the denial",
			yaml:      "disabled_tools:\n  - \" \"\n  - etcd_get\n",
			wantOther: true,
		},
		{
			name: "yaml domain denial hides every tool",
			yaml: yamlDomain,
		},
		{
			name:   "empty env keeps yaml domain denial",
			yaml:   yamlDomain,
			envKey: "MCP_DISABLED_DOMAINS",
		},
		{
			name:      "empty flag clears yaml domain denial",
			yaml:      yamlDomain,
			args:      []string{"--disable-domains", ""},
			wantGet:   true,
			wantOther: true,
		},
		{
			name: "blank token beside a denied domain keeps the denial",
			yaml: "disabled_domains:\n  - \" \"\n  - etcd\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envKey != "" {
				t.Setenv(tt.envKey, tt.envVal)
			}
			args := append([]string{}, tt.args...)
			if tt.yaml != "" {
				args = append([]string{"--config", writeConfig(t, tt.yaml)}, args...)
			}
			args = append(args, "tools", "list", "--json")
			names := listedToolNames(t, args)
			got := map[string]bool{}
			for _, name := range names {
				got[name] = true
			}
			if got["etcd_get"] != tt.wantGet || got["etcd_lease_list"] != tt.wantOther {
				t.Fatalf("etcd_get=%v etcd_lease_list=%v, want get=%v other=%v, tools=%v", got["etcd_get"], got["etcd_lease_list"], tt.wantGet, tt.wantOther, names)
			}
		})
	}
}

func TestMCPAllBlankDenyListFailsBeforeRuntimeClient(t *testing.T) {
	original := newRuntimeClient
	clients := 0
	newRuntimeClient = func(*config.StaticConfig) (runtimeClient, error) {
		clients++
		return runtimeClient{}, errors.New("client created")
	}
	t.Cleanup(func() { newRuntimeClient = original })

	var out bytes.Buffer
	root := NewRootCommand(IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs([]string{"--config", writeConfig(t, "disabled_tools:\n  - etcd_get\n"), "--disabled-tools", " ", "mcp"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "disabled_tools") || clients != 0 {
		t.Fatalf("Execute() error = %v, clients = %d, want deny-list rejection before a client", err, clients)
	}
}
