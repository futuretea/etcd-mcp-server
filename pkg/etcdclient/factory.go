// Package etcdclient owns construction of the configured etcd v3 client.
package etcdclient

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/futuretea/etcd-mcp-server/pkg/core/config"
)

const (
	usernameEnvironment = "MCP_ETCD_USERNAME"
	passwordEnvironment = "MCP_ETCD_PASSWORD"
)

// New creates the single runtime etcd client and rejects incomplete startup inputs.
func New(cfg *config.StaticConfig) (*clientv3.Client, error) {
	clientConfig, err := clientConfig(cfg)
	if err != nil {
		return nil, err
	}
	return clientv3.New(clientConfig)
}

func clientConfig(cfg *config.StaticConfig) (clientv3.Config, error) {
	if cfg == nil {
		return clientv3.Config{}, fmt.Errorf("etcd client configuration is required")
	}
	if err := cfg.Validate(); err != nil {
		return clientv3.Config{}, fmt.Errorf("validate etcd configuration: %w", err)
	}
	if len(cfg.EtcdEndpoints) == 0 {
		return clientv3.Config{}, fmt.Errorf("etcd_endpoints must be configured for a runtime client")
	}

	tlsConfig, err := loadTLSConfig(cfg)
	if err != nil {
		return clientv3.Config{}, err
	}
	username, password, err := credentialsFromEnvironment()
	if err != nil {
		return clientv3.Config{}, err
	}

	return clientv3.Config{
		Endpoints:          cfg.EtcdEndpoints,
		DialTimeout:        cfg.EtcdDialTimeout,
		TLS:                tlsConfig,
		Username:           username,
		Password:           password,
		MaxCallRecvMsgSize: cfg.EtcdMaxResponseBytes,
		MaxUnaryRetries:    1,
	}, nil
}

func credentialsFromEnvironment() (string, string, error) {
	username, hasUsername := os.LookupEnv(usernameEnvironment)
	password, hasPassword := os.LookupEnv(passwordEnvironment)
	if hasUsername != hasPassword {
		return "", "", fmt.Errorf("%s and %s must be set together", usernameEnvironment, passwordEnvironment)
	}
	if !hasUsername {
		return "", "", nil
	}
	return username, password, nil
}

func loadTLSConfig(cfg *config.StaticConfig) (*tls.Config, error) {
	if cfg.EtcdCAFile == "" && cfg.EtcdCertFile == "" {
		return nil, nil
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if cfg.EtcdCAFile != "" {
		pem, err := os.ReadFile(cfg.EtcdCAFile)
		if err != nil {
			return nil, fmt.Errorf("read etcd_ca_file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("parse etcd_ca_file %q", cfg.EtcdCAFile)
		}
		tlsConfig.RootCAs = pool
	}
	if cfg.EtcdCertFile != "" {
		certificate, err := tls.LoadX509KeyPair(cfg.EtcdCertFile, cfg.EtcdKeyFile)
		if err != nil {
			return nil, fmt.Errorf("load etcd client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{certificate}
	}
	return tlsConfig, nil
}
