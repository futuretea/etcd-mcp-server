package cmd

import (
	"fmt"
	"io"

	"github.com/futuretea/etcd-mcp-server/pkg/core/config"
	"github.com/futuretea/etcd-mcp-server/pkg/etcdclient"
	"github.com/futuretea/etcd-mcp-server/pkg/toolset/etcd"
)

type runtimeClient struct {
	adapter etcd.Adapter
	closer  io.Closer
}

var newRuntimeClient = func(cfg *config.StaticConfig) (runtimeClient, error) {
	client, err := etcdclient.New(cfg)
	if err != nil {
		return runtimeClient{}, err
	}
	return runtimeClient{adapter: etcd.NewV3Adapter(client), closer: client}, nil
}

func (c runtimeClient) close() error {
	if c.closer == nil {
		return fmt.Errorf("runtime client closer is required")
	}
	return c.closer.Close()
}
