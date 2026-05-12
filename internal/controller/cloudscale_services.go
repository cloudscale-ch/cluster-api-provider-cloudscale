package controller

import (
	"context"
	"fmt"

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v8"

	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/scope"
)

// getListService is satisfied by all cloudscale SDK resource services.
type getListService[T any] interface {
	Get(ctx context.Context, id string) (*T, error)
	List(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]T, error)
}

// ensureResource checks if a resource exists by its status ID, or adopts one found by tags.
// Returns the resource object and its ID on success. A nil resource and empty ID means
// the caller should create the resource.
func ensureResource[T any](
	ctx context.Context,
	clusterScope *scope.ClusterScope,
	currentID string,
	resourceName string,
	svc getListService[T],
	extractUUID func(T) string,
	tags cloudscalesdk.TagMap,
) (*T, string, error) {
	if currentID != "" {
		getCtx, cancel := context.WithTimeout(ctx, cloudscale.ReadTimeout)
		defer cancel()
		resource, err := svc.Get(getCtx, currentID)
		if err == nil {
			return resource, currentID, nil
		}
		if !cloudscale.IsNotFound(err) {
			return nil, "", fmt.Errorf("getting %s: %w", resourceName, err)
		}
		// Resource was deleted externally, fall through to list/recreate
	}

	// tag-based lookup can happen after the resource has been created just before due to the caching behavior of
	// controller-runtime. It is therefore expected to have the following in the logs:
	// Reconcile 1: Create & Patch Status to have the ID recorded
	// Reconcile 2: Get 404, List 200 (found existing ...)
	// Reconcile 3: Get 200 -> here the cache is up-to-date

	listCtx, cancel := context.WithTimeout(ctx, cloudscale.ReadTimeout)
	defer cancel()
	items, err := svc.List(listCtx, cloudscalesdk.WithTagFilter(tags))
	if err != nil {
		return nil, "", fmt.Errorf("listing %ss: %w", resourceName, err)
	}
	if len(items) > 1 {
		return nil, "", fmt.Errorf("found %d %ss matching tag filter, expected at most 1", len(items), resourceName)
	}
	if len(items) == 1 {
		item := items[0]
		uuid := extractUUID(item)
		clusterScope.Info("Found existing "+resourceName+" by tag", "id", uuid)
		return &item, uuid, nil
	}

	return nil, "", nil
}
