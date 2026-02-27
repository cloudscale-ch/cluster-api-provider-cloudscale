package controller

import (
	"context"
	"fmt"

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v6"

	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/scope"
)

// getListService is satisfied by all cloudscale SDK resource services.
type getListService[T any] interface {
	Get(ctx context.Context, id string) (*T, error)
	List(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]T, error)
}

// ensureResource checks if a resource exists by its status ID, or adopts one found by tags.
// Returns the resolved ID and nil on success. An empty ID means the caller should create the resource.
func ensureResource[T any](
	ctx context.Context,
	clusterScope *scope.ClusterScope,
	currentID string,
	resourceName string,
	svc getListService[T],
	extractUUID func(T) string,
	tags cloudscalesdk.TagMap,
) (string, error) {
	if currentID != "" {
		_, err := svc.Get(ctx, currentID)
		if err == nil {
			return currentID, nil
		}
		if !cloudscale.IsNotFound(err) {
			return "", fmt.Errorf("getting %s: %w", resourceName, err)
		}
		// Resource was deleted externally, fall through to list/recreate
	}

	items, err := svc.List(ctx, cloudscalesdk.WithTagFilter(tags))
	if err != nil {
		return "", fmt.Errorf("listing %ss: %w", resourceName, err)
	}
	if len(items) > 1 {
		return "", fmt.Errorf("found %d %ss matching tag filter, expected at most 1", len(items), resourceName)
	}
	if len(items) == 1 {
		uuid := extractUUID(items[0])
		clusterScope.Info("Found existing "+resourceName+" by tag", "id", uuid)
		return uuid, nil
	}

	return "", nil
}
