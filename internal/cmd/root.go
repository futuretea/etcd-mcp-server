package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/futuretea/etcd-mcp-server/internal/toolcatalog"
	"github.com/futuretea/etcd-mcp-server/pkg/core/config"
	"github.com/futuretea/etcd-mcp-server/pkg/core/version"
	"github.com/futuretea/etcd-mcp-server/pkg/toolset"
	"github.com/futuretea/etcd-mcp-server/pkg/toolset/etcd"
)

// IOStreams groups the streams used by the CLI.
type IOStreams struct {
	In     io.Reader
	Out    io.Writer
	ErrOut io.Writer
}

// NewRootCommand creates the CLI root. The MCP server is started via the `mcp` subcommand.
func NewRootCommand(streams IOStreams) *cobra.Command {
	var cfgFile string
	v := viper.New()

	command := &cobra.Command{
		Use:           version.BinaryName,
		Short:         "Read-only etcd Model Context Protocol server",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	command.SetIn(streams.In)
	command.SetOut(streams.Out)
	command.SetErr(streams.ErrOut)

	addCommonFlags(command, &cfgFile)
	addEtcdFlags(command)
	command.AddCommand(newMCPCommand(streams, &cfgFile, v))
	command.AddCommand(newToolsCommand(streams, &cfgFile, v))
	command.AddCommand(newVersionCommand(streams))
	command.AddCommand(newCompletionCommand(streams))
	return command
}

func addCommonFlags(command *cobra.Command, cfgFile *string) {
	flags := command.PersistentFlags()
	flags.StringVar(cfgFile, "config", "", "config file path (YAML)")
	flags.StringSlice("enabled-tools", []string{}, "comma-separated list of tool names to enable")
	flags.StringSlice("disabled-tools", []string{}, "comma-separated list of tool names to disable")
	flags.StringSlice("enable-domains", []string{}, "comma-separated list of tool domains to enable")
	flags.StringSlice("disable-domains", []string{}, "comma-separated list of tool domains to disable")
}

func addEtcdFlags(command *cobra.Command) {
	flags := command.PersistentFlags()
	flags.StringSlice("etcd-endpoints", []string{}, "configured etcd endpoints")
	flags.Duration("etcd-dial-timeout", 5e9, "etcd client dial timeout")
	flags.Duration("etcd-request-timeout", 10e9, "per-tool etcd request timeout")
	flags.String("etcd-ca-file", "", "etcd CA certificate path")
	flags.String("etcd-cert-file", "", "etcd client certificate path")
	flags.String("etcd-key-file", "", "etcd client private key path")
	flags.Int("etcd-max-items", 100, "maximum etcd_get items")
	flags.Int("etcd-max-response-bytes", 1<<20, "maximum tool JSON response bytes")
}

func bindCommonFlags(v *viper.Viper, cmd *cobra.Command) error {
	flags := cmd.Root().PersistentFlags()
	bindings := map[string]string{
		"enabled_tools":           "enabled-tools",
		"disabled_tools":          "disabled-tools",
		"enabled_domains":         "enable-domains",
		"disabled_domains":        "disable-domains",
		"etcd_endpoints":          "etcd-endpoints",
		"etcd_dial_timeout":       "etcd-dial-timeout",
		"etcd_request_timeout":    "etcd-request-timeout",
		"etcd_ca_file":            "etcd-ca-file",
		"etcd_cert_file":          "etcd-cert-file",
		"etcd_key_file":           "etcd-key-file",
		"etcd_max_items":          "etcd-max-items",
		"etcd_max_response_bytes": "etcd-max-response-bytes",
	}
	for key, flag := range bindings {
		if err := v.BindPFlag(key, flags.Lookup(flag)); err != nil {
			return fmt.Errorf("bind flag %s: %w", flag, err)
		}
	}
	return nil
}

func loadCLIConfig(cmd *cobra.Command, cfgFile string, v *viper.Viper) (*config.StaticConfig, error) {
	if err := bindCommonFlags(v, cmd); err != nil {
		return nil, err
	}
	cfg, err := config.LoadConfig(cfgFile, v)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	return cfg, nil
}

func defaultToolsets(cfg *config.StaticConfig, adapter etcd.Adapter) []toolset.Toolset {
	return []toolset.Toolset{etcd.NewToolset(adapter, etcd.Config{
		Endpoints: cfg.EtcdEndpoints, RequestTimeout: cfg.EtcdRequestTimeout,
		MaxItems: cfg.EtcdMaxItems, MaxResponseBytes: cfg.EtcdMaxResponseBytes,
	})}
}

func buildToolCatalog(cfg *config.StaticConfig, adapter etcd.Adapter) ([]toolset.ServerTool, error) {
	return toolcatalog.Build(defaultToolsets(cfg, adapter), toolset.FilterOptions{
		EnabledTools:    cfg.EnabledTools,
		DisabledTools:   cfg.DisabledTools,
		EnabledDomains:  cfg.EnabledDomains,
		DisabledDomains: cfg.DisabledDomains,
	})
}

func newVersionCommand(streams IOStreams) *cobra.Command {
	command := &cobra.Command{
		Use:   "version",
		Short: "print version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintln(streams.Out, version.GetVersionInfo())
			return err
		},
	}
	command.SetOut(streams.Out)
	command.SetErr(streams.ErrOut)
	return command
}
