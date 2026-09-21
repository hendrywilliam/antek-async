package kube

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync/atomic"
	"time"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

// Resource identifies a watched resource kind. The values are the strings the frontend sends
// back through SelectResource.
type Resource string

const (
	ResourceNodes        Resource = "nodes"
	ResourcePods         Resource = "pods"
	ResourceDeployments  Resource = "deployments"
	ResourceStatefulSets Resource = "statefulSets"
)

// Valid reports whether the value names a resource this app can watch.
func (r Resource) Valid() bool {
	switch r {
	case ResourceNodes, ResourcePods, ResourceDeployments, ResourceStatefulSets:
		return true
	default:
		return false
	}
}

const (
	probeTimeout    = 15 * time.Second
	requestTimeout  = 15 * time.Second
	flushInterval   = time.Second
	ageRefreshEvery = 10 * time.Second
)

// watchInformer streams one resource kind. Each kind calls it with its own informer, probe and
// converter, so the informer, flush and ticker plumbing lives in exactly one place instead of
// being copied per resource.
//
// The informer does the initial list plus a watch and reconnects on its own, and no namespace
// option means every namespace is watched. It returns when ctx is done, or when the probe or
// the first cache sync fails.
func watchInformer[T any](
	ctx context.Context,
	noun string,
	factory informers.SharedInformerFactory,
	informer cache.SharedIndexInformer,
	probe func(context.Context) error,
	convert func(cache.Store) []T,
	onSnapshot func([]T),
) error {
	// Probe first: an informer that fails to sync only reports a generic error, while this
	// surfaces the API server's own message (401, 403, DNS, timeout) and names the resource
	// the credentials cannot read. Limit keeps the probe cheap.
	probeCtx, cancelProbe := context.WithTimeout(ctx, probeTimeout)
	defer cancelProbe()

	if err := probe(probeCtx); err != nil {
		return fmt.Errorf("cannot read %s: %w", noun, err)
	}

	// Events only mark the cache dirty; the flush loop publishes one full list.
	var dirty atomic.Bool
	markDirty := func(any) { dirty.Store(true) }
	if _, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    markDirty,
		UpdateFunc: func(any, any) { markDirty(nil) },
		DeleteFunc: markDirty,
	}); err != nil {
		return err
	}

	factory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
		if ctx.Err() != nil {
			return nil
		}
		return errors.New("failed to sync " + noun + " cache")
	}

	onSnapshot(convert(informer.GetStore()))

	flush := time.NewTicker(flushInterval)
	defer flush.Stop()
	refreshAge := time.NewTicker(ageRefreshEvery)
	defer refreshAge.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-refreshAge.C:
			dirty.Store(true)
		case <-flush.C:
			if dirty.Swap(false) {
				onSnapshot(convert(informer.GetStore()))
			}
		}
	}
}

// sortByNamespaceAndName orders items the way the tables render them.
func sortByNamespaceAndName[T any](items []T, namespaceOf, nameOf func(T) string) {
	sort.Slice(items, func(i, j int) bool {
		if namespaceOf(items[i]) != namespaceOf(items[j]) {
			return namespaceOf(items[i]) < namespaceOf(items[j])
		}
		return nameOf(items[i]) < nameOf(items[j])
	})
}

// sortByName orders cluster-scoped items the way the table renders them.
func sortByName[T any](items []T, nameOf func(T) string) {
	sort.Slice(items, func(i, j int) bool {
		return nameOf(items[i]) < nameOf(items[j])
	})
}

// replicaCount formats the READY column shared by deployments and statefulsets. Replicas is a
// pointer that stays nil when the field is absent from the manifest, which kubectl reports as
// a desired count of zero.
func replicaCount(ready int32, desired *int32) string {
	total := int32(0)
	if desired != nil {
		total = *desired
	}

	return fmt.Sprintf("%d/%d", ready, total)
}
