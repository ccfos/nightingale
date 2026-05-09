package es

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"sync"

	elasticsearch7 "github.com/elastic/go-elasticsearch/v7"
	elasticsearch8 "github.com/elastic/go-elasticsearch/v8"
	elasticsearch9 "github.com/elastic/go-elasticsearch/v9"
)

var (
	clientCacheV7 sync.Map // map[*Elasticsearch]*elasticsearch7.Client
	clientCacheV8 sync.Map // map[*Elasticsearch]*elasticsearch8.Client
	clientCacheV9 sync.Map // map[*Elasticsearch]*elasticsearch9.Client
)

func officialClientV7(escli *Elasticsearch) (*elasticsearch7.Client, error) {
	if cached, ok := clientCacheV7.Load(escli); ok {
		return cached.(*elasticsearch7.Client), nil
	}

	cfg := elasticsearch7.Config{
		Addresses: escli.Nodes,
	}

	if escli.Basic.Enable {
		cfg.Username = escli.Basic.Username
		cfg.Password = escli.Basic.Password
	}

	if len(escli.Headers) > 0 {
		cfg.Header = make(http.Header)
		for k, v := range escli.Headers {
			cfg.Header.Set(k, v)
		}
	}

	if escli.TLS.SkipTlsVerify {
		cfg.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, //nolint:gosec
			},
		}
	}

	client, err := elasticsearch7.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create elasticsearch v7 client: %w", err)
	}

	clientCacheV7.Store(escli, client)
	return client, nil
}

func officialClientV8(escli *Elasticsearch) (*elasticsearch8.Client, error) {
	if cached, ok := clientCacheV8.Load(escli); ok {
		return cached.(*elasticsearch8.Client), nil
	}

	cfg := elasticsearch8.Config{
		Addresses: escli.Nodes,
	}

	if escli.Basic.Enable {
		cfg.Username = escli.Basic.Username
		cfg.Password = escli.Basic.Password
	}

	if len(escli.Headers) > 0 {
		cfg.Header = make(http.Header)
		for k, v := range escli.Headers {
			cfg.Header.Set(k, v)
		}
	}

	if escli.TLS.SkipTlsVerify {
		cfg.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, //nolint:gosec
			},
		}
	}

	client, err := elasticsearch8.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create elasticsearch v8 client: %w", err)
	}

	clientCacheV8.Store(escli, client)
	return client, nil
}

func officialClientV9(escli *Elasticsearch) (*elasticsearch9.Client, error) {
	if cached, ok := clientCacheV9.Load(escli); ok {
		return cached.(*elasticsearch9.Client), nil
	}

	cfg := elasticsearch9.Config{
		Addresses: escli.Nodes,
	}

	if escli.Basic.Enable {
		cfg.Username = escli.Basic.Username
		cfg.Password = escli.Basic.Password
	}

	if len(escli.Headers) > 0 {
		cfg.Header = make(http.Header)
		for k, v := range escli.Headers {
			cfg.Header.Set(k, v)
		}
	}

	if escli.TLS.SkipTlsVerify {
		cfg.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, //nolint:gosec
			},
		}
	}

	client, err := elasticsearch9.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create elasticsearch v9 client: %w", err)
	}

	clientCacheV9.Store(escli, client)
	return client, nil
}
