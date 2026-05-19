package vsphere

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"

	"vcm/internal/config"
)

type Client struct {
	client     *govmomi.Client
	finder     *find.Finder
	datacenter *object.Datacenter
	baseURL    string
}

func NewClient(ctx context.Context, cfg config.Config) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	normalized, err := config.NormalizeURL(cfg.URL)
	if err != nil {
		return nil, err
	}
	endpoint, err := url.Parse(normalized)
	if err != nil {
		return nil, err
	}
	endpoint.User = url.UserPassword(cfg.Username, cfg.Password)

	govmomiClient, err := govmomi.NewClient(ctx, endpoint, cfg.Insecure)
	if err != nil {
		return nil, err
	}

	finder := find.NewFinder(govmomiClient.Client, true)
	vc := &Client{
		client:  govmomiClient,
		finder:  finder,
		baseURL: webBaseURL(endpoint),
	}

	if cfg.Datacenter != "" {
		dc, err := finder.Datacenter(ctx, cfg.Datacenter)
		if err != nil {
			_ = govmomiClient.Logout(ctx)
			return nil, fmt.Errorf("find datacenter %q: %w", cfg.Datacenter, err)
		}
		vc.datacenter = dc
		finder.SetDatacenter(dc)
		return vc, nil
	}

	if dc, err := finder.DefaultDatacenter(ctx); err == nil {
		vc.datacenter = dc
		finder.SetDatacenter(dc)
	}

	return vc, nil
}

func (c *Client) Close(ctx context.Context) error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Logout(ctx)
}

func webBaseURL(endpoint *url.URL) string {
	u := *endpoint
	u.User = nil
	u.Path = ""
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/")
}
