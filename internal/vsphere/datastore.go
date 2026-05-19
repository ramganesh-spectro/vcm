package vsphere

import (
	"context"
	"fmt"
	"strings"

	"github.com/vmware/govmomi/vim25/mo"
)

func (c *Client) ListDatastores(ctx context.Context) ([]Datastore, error) {
	stores, err := c.finder.DatastoreList(ctx, "*")
	if err != nil {
		return nil, err
	}

	result := make([]Datastore, 0, len(stores))
	for _, store := range stores {
		var entity mo.Datastore
		if err := c.client.RetrieveOne(ctx, store.Reference(), []string{"summary"}, &entity); err != nil {
			return nil, err
		}
		result = append(result, Datastore{
			Name:          entity.Summary.Name,
			Type:          entity.Summary.Type,
			URL:           entity.Summary.Url,
			CapacityBytes: entity.Summary.Capacity,
			FreeBytes:     entity.Summary.FreeSpace,
		})
	}

	return result, nil
}

func (c *Client) UploadToDatastore(ctx context.Context, localFile string, datastorePath string) error {
	localFile = strings.TrimSpace(localFile)
	if localFile == "" {
		return fmt.Errorf("local file is required")
	}

	datastoreName, remotePath, err := ParseDatastorePath(datastorePath)
	if err != nil {
		return err
	}

	store, err := c.finder.Datastore(ctx, datastoreName)
	if err != nil {
		return err
	}
	if err := store.FindInventoryPath(ctx); err != nil {
		return err
	}

	return store.UploadFile(ctx, localFile, remotePath, nil)
}

func ParseDatastorePath(value string) (string, string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", fmt.Errorf("datastore path is required")
	}

	if strings.HasPrefix(value, "[") {
		closing := strings.Index(value, "]")
		if closing < 0 {
			return "", "", fmt.Errorf("datastore path %q is missing closing bracket", value)
		}
		name := strings.TrimSpace(value[1:closing])
		path := strings.Trim(strings.TrimSpace(value[closing+1:]), "/")
		return validateDatastorePath(name, path)
	}

	name, path, ok := strings.Cut(value, ":")
	if !ok {
		return "", "", fmt.Errorf("datastore path must be in \"[datastore] path/file\" or \"datastore:path/file\" form")
	}
	return validateDatastorePath(strings.TrimSpace(name), strings.Trim(strings.TrimSpace(path), "/"))
}

func validateDatastorePath(name string, path string) (string, string, error) {
	if name == "" {
		return "", "", fmt.Errorf("datastore name is required")
	}
	if path == "" {
		return "", "", fmt.Errorf("remote datastore file path is required")
	}
	return name, path, nil
}
