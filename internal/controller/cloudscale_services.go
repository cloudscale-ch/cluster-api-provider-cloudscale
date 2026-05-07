package controller

import (
	"context"
	"fmt"
	"time"

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v8"

	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/scope"
)

const (
	// CreateTimeoutRequeueInterval is the requeue interval after an HTTP timeout
	// on a Create call. This MUST be longer than cloudscale.WriteTimeout (2m) so the
	// original request has time to complete before we retry.
	CreateTimeoutRequeueInterval = 150 * time.Second
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
