/*
Copyright 2023 Kubespress Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package objects

import (
	"context"
	"sort"

	"github.com/kubespress/errors"
	"github.com/kubespress/objects/internal/state"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SetClient is used to manage a "set" of items. It is constructed by the Builder object.
type SetClient[T client.Object] struct {
	clients   []*Client[T]
	objects   []T
	builder   *FinalizedBuilder[T]
	labels    map[string]string
	replicas  int
	naming    SetStrategy
	namespace string
}

// SetCommonActions contains common methods that exists in all object sets.
type SetCommonActions[T1 client.Object, T2 any] interface {
	// Each runs the provided function for each item within the set. The client
	// passed to the function should not be used outside of the function call.
	// For each invocation the object originally passed to the SetBuilder will
	// be populated with the contents of the object.
	Each(context.Context, func(context.Context, *Client[T1]) error) error
	// Count returns the number of objects within the set.
	Count(context.Context) (int, error)
	// Filter returns a filtered version of the set
	Filter(func(T1) bool) T2
}

// SetCreateActions contains methods used to create sets of objects within
// Kubernetes.
type SetCreateActions interface {
	// CreateOne will create a single object within Kubernetes, if the set is
	// empty no action will be performed and false will be returned.
	CreateOne(context.Context) (bool, error)
	// CreateAll will create all objects in the set within Kubernetes. The
	// number of created objects is returned.
	CreateAll(context.Context) (int, error)
}

// SetUpdateActions contains methods used to update sets of objects within
// Kubernetes.
type SetUpdateActions interface {
	// UpdateOne will update a single object within Kubernetes, if the set is
	// empty no action will be performed and false will be returned.
	UpdateOne(context.Context) (bool, error)
	// UpdateAll will update all objects in the set within Kubernetes. The
	// number of updated objects is returned.
	UpdateAll(context.Context) (int, error)
}

// SetDeleteActions contains methods used to delete sets of objects within
// Kubernetes.
type SetDeleteActions interface {
	// DeleteOne will delete a single object within Kubernetes, if the set is
	// empty no action will be performed and false will be returned.
	DeleteOne(context.Context) (bool, error)
	// DeleteAll will delete all objects in the set within Kubernetes. The
	// number of deleted objects is returned.
	DeleteAll(context.Context) (int, error)
}

// ExistingObjectsSet is the set of existing objects.
type ExistingObjectsSet[T client.Object] interface {
	SetCommonActions[T, ExistingObjectsSet[T]]
	SetDeleteActions
}

// ObjectsToCreateSet is the set of objects to be created.
type ObjectsToCreateSet[T client.Object] interface {
	SetCommonActions[T, ObjectsToCreateSet[T]]
	SetCreateActions
}

// ObjectsToUpdateSet is the set of objects that require an update.
type ObjectsToUpdateSet[T client.Object] interface {
	SetCommonActions[T, ObjectsToUpdateSet[T]]
	SetUpdateActions
	SetDeleteActions
}

// ObjectsToUpdateSet is the set of objects to be deleted.
type ObjectsToDeleteSet[T client.Object] interface {
	SetCommonActions[T, ObjectsToDeleteSet[T]]
	SetDeleteActions
}

// Ensure will perform all create/update/delete operations required to make the objects in Kubernetes match the defined set.
func (c *SetClient[T]) Ensure(ctx context.Context) error {
	// Perform create
	if _, err := c.ObjectsToCreate().CreateAll(ctx); err != nil {
		return err
	}

	// Perform update
	if _, err := c.ObjectsToUpdate().UpdateAll(ctx); err != nil {
		return err
	}

	// Perform delete
	if _, err := c.ObjectsToDelete().DeleteAll(ctx); err != nil {
		return err
	}

	// Return no error
	return nil
}

// Existing returns a set of objects that reprents all existing objects in Kubernetes, regardless of if they are desired
// by the set.
func (c *SetClient[T]) Existing() ExistingObjectsSet[T] {
	return existingSetIterator[T]{
		setIterator: setIterator[T]{
			client:    c,
			condition: state.ObjectExists,
		},
	}
}

// Existing returns a set of objects that reprents all existing objects in Kubernetes, regardless of if they are desired
// by the set.
func (c *SetClient[T]) ObjectsToCreate() ObjectsToCreateSet[T] {
	return objectsToCreateIterator[T]{
		setIterator: setIterator[T]{
			client:    c,
			condition: state.Merge(state.ObjectDoesNotExist, state.SetActionCreate),
		},
	}
}

// ObjectsToUpdate returns a set of objects that require an update within Kubernetes. Objects that are due to be deleted
// are not included in this set.
func (c *SetClient[T]) ObjectsToUpdate() ObjectsToUpdateSet[T] {
	return objectsToUpdateIterator[T]{
		setIterator: setIterator[T]{
			client:    c,
			condition: state.Merge(state.ObjectExists, state.ObjectRequiresUpdate, state.SetActionKeep),
		},
	}
}

// ObjectsToDelete returns a set of objects that should bd deleted in order
// to conform the the replica count and naming strategy of the set.
func (c *SetClient[T]) ObjectsToDelete() ObjectsToDeleteSet[T] {
	return objectsToDeleteIterator[T]{
		setIterator: setIterator[T]{
			client:    c,
			condition: state.Merge(state.ObjectExists, state.SetActionDelete),
			reverse:   true,
		},
	}
}

func (c *SetClient[T]) prepare(ctx context.Context) error {
	// Get object metadata
	metadata, err := c.builder.ResourceMetadata()
	if err != nil {
		return err
	}

	// Validate the client exists
	if c.builder.builder.Client == nil {
		return errors.Errorf("no client provided")
	}

	// Validate the labels
	if len(c.labels) == 0 {
		return errors.Errorf("no labels specified for set")
	}

	// If the type is a list, return an error
	if metadata.IsList {
		return errors.Errorf("cannot use %s in client: type must not be list", metadata.GroupKind())
	}

	// Ensure that namespace is set for namespaced resources
	if metadata.Namespaced && c.namespace == "" {
		return errors.Errorf("namespace must be specified")
	}

	// If the objects slice is nil we need to perform the initial list, note
	// that a non nil slice with a length of zero is perfectly valid
	if c.objects == nil {
		// List the objects
		objects, err := c.list(ctx, c.listOptions()...)
		if err != nil {
			return err
		}

		// Store the obtained objects
		c.objects = objects

		// Sort the obtained objects
		sort.Slice(c.objects, func(i, j int) bool {
			return c.naming.Less(c.objects[i], c.objects[j])
		})

		// Rebuild the clients
		c.clients = c.clients[:0]
		for i, obj := range c.objects {
			// Create client for the current object
			client := c.builder.ClientForExistingObject(obj)
			client.state.Set(state.SetActionKeep)

			// If the object should be deleted as determined by the naming
			// strategy, then store the delete action in the state
			if c.naming.ShouldBeDeleted(objectSliceAccessor[T](c.objects), i) {
				client.state.Set(state.SetActionDelete)
			}

			// Append the new client
			c.clients = append(c.clients, client)
		}
	}

	// Track "desired" objects to compare to the replica count
	desired := 0

	// Create slice to hold clients, this is used when we need to drop clients
	clone := make([]*Client[T], 0, cap(c.clients))

	// Loop over all clients
	for _, client := range c.clients {
		// Run prepare on the client to ensure the state is populated
		if err := client.prepare(ctx); err != nil {
			return err
		}

		// If the object does not exist, and its not one that should be created, skip over it.
		if client.state.Check(state.ObjectDoesNotExist) && !client.state.Check(state.SetActionCreate) {
			continue
		}

		// If this object should be kept or created (should not be deleted) it is a "desired" object. Increment the desired object count.
		if client.state.Check(state.SetActionCreate) || client.state.Check(state.SetActionKeep) {
			desired++
		}

		// Append the client to the clone
		clone = append(clone, client)
	}

	// If the new slice of clients and old slice of clients have different
	// lengths, then clients have been dropped (due to the object no longer
	// existing in Kubernetes).
	//
	// In this case we need to store the new set of clients and rebuild the
	// object slice
	if len(clone) != len(c.clients) {
		// Create new slice to hold objects
		objects := make([]T, 0, len(clone))
		for _, client := range clone {
			objects = append(objects, client.getExistingOrCreateObject())
		}

		// Update the slices
		c.clients = clone
		c.objects = objects
	}

	// Determine if we need to create new clients by comparing the count of
	// "desired" object with the replica count
	switch {
	case desired < c.replicas:
		// Create new embedded clients for new objects
		for i := 0; i < c.replicas-desired; i++ {
			// Create child builder that uses our custom name function
			builder := c.builder.child().WithNameFn(func(o T) error {
				return c.naming.SetName(objectSliceAccessor[T](c.objects), o)
			}).Finalize()

			// Create client for the new objects
			client := newClient(builder)

			// Update the client state to mark the object as one that needs
			// creating
			client.state.Set(state.SetActionCreate)
			client.state.Set(state.ObjectDoesNotExist)

			// Run the prepare method to populate the client state
			if err := client.prepare(ctx); err != nil {
				return err
			}

			// Append the new client and the new object
			c.clients = append(c.clients, client)
			c.objects = append(c.objects, client.objectToCreate)
		}

		// Sort the newly generated clients and objects
		c.sortClientAndObjects()

	case desired > c.replicas:
		// We have too many objects, this should not happen so return an error
		return errors.Errorf("set desired object miscalculation desired=%d replicas=%d", desired, c.replicas)
	}

	// Return no error
	return nil
}

func (c *SetClient[T]) sortClientAndObjects() {
	sort.Slice(c.clients, func(i, j int) bool {
		return c.naming.Less(c.clients[i].getExistingOrCreateObject(), c.clients[j].getExistingOrCreateObject())
	})

	sort.Slice(c.objects, func(i, j int) bool {
		return c.naming.Less(c.objects[i], c.objects[j])
	})
}

func (c *SetClient[T]) listOptions() (opts []client.ListOption) {
	opts = append(opts, client.MatchingLabels(c.labels))
	if c.namespace != "" {
		opts = append(opts, client.InNamespace(c.namespace))
	}
	return opts
}

func (c *SetClient[T]) list(ctx context.Context, opts ...client.ListOption) ([]T, error) {
	// Get list object
	list, err := c.builder.zeroList()
	if err != nil {
		return nil, err
	}

	// Perform list using client
	if err := c.builder.builder.Client.List(ctx, list, opts...); err != nil {
		return nil, err
	}

	// Extract the objects
	return c.extractList(list)
}

// extractList returns obj's Items element as an array of runtime.Objects. Returns an error if obj is not a List type
// (does not have an Items member).
func (c *SetClient[T]) extractList(obj runtime.Object) ([]T, error) {
	// Get pointer to the "Items" field in the slice
	itemsPtr, err := meta.GetItemsPtr(obj)
	if err != nil {
		return nil, err
	}

	// Get reflect value of the item pointer
	items, err := conversion.EnforcePtr(itemsPtr)
	if err != nil {
		return nil, err
	}

	// Create slice to hold result
	list := make([]T, items.Len())

	// Loop over items
	for i := range list {
		// Get item at index
		raw := items.Index(i)

		// Switch on item type
		switch item := raw.Interface().(type) {
		case T:
			list[i] = item
		default:
			var found bool
			if list[i], found = raw.Addr().Interface().(T); !found {
				return nil, errors.Errorf("%v: item[%v]: Expected object, got %#v(%s)", obj, i, raw.Interface(), raw.Kind())
			}
		}
	}

	// Return built list
	return list, nil
}

type setIterator[T client.Object] struct {
	client    *SetClient[T]
	condition state.Condition
	filters   []func(T) bool
	reverse   bool
}

func (sa setIterator[T]) Each(ctx context.Context, fn func(context.Context, *Client[T]) error) error {
	_, err := sa.iterate(ctx, func(ctx context.Context, client *Client[T]) (bool, error) {
		return true, fn(ctx, client)
	})
	return err
}

func (sa setIterator[T]) Count(ctx context.Context) (int, error) {
	return sa.iterate(ctx, nil)
}

func (sa setIterator[T]) CreateOne(ctx context.Context) (bool, error) {
	count, err := sa.iterate(ctx, func(ctx context.Context, client *Client[T]) (bool, error) {
		return false, client.Create(ctx)
	})
	return count != 0, err
}

func (sa setIterator[T]) CreateAll(ctx context.Context) (int, error) {
	return sa.iterate(ctx, func(ctx context.Context, client *Client[T]) (bool, error) {
		return true, client.Create(ctx)
	})
}

func (sa setIterator[T]) UpdateOne(ctx context.Context) (bool, error) {
	count, err := sa.iterate(ctx, func(ctx context.Context, client *Client[T]) (bool, error) {
		return false, client.Update(ctx)
	})
	return count != 0, err
}

func (sa setIterator[T]) UpdateAll(ctx context.Context) (int, error) {
	return sa.iterate(ctx, func(ctx context.Context, client *Client[T]) (bool, error) {
		return true, client.Update(ctx)
	})
}

func (sa setIterator[T]) DeleteOne(ctx context.Context) (bool, error) {
	count, err := sa.iterate(ctx, func(ctx context.Context, client *Client[T]) (bool, error) {
		// If object is already being deleted, we are waiting for its deletion
		if client.state.Check(state.ObjectPendingDeletion) {
			return false, nil
		}

		return false, client.Delete(ctx)
	})
	return count != 0, err
}

func (sa setIterator[T]) DeleteAll(ctx context.Context) (int, error) {
	return sa.iterate(ctx, func(ctx context.Context, client *Client[T]) (bool, error) {
		// If object is already being deleted, we are waiting for its deletion
		if client.state.Check(state.ObjectPendingDeletion) {
			return true, nil
		}

		return true, client.Delete(ctx)
	})
}

func (si setIterator[T]) withFilter(fn func(T) bool) setIterator[T] {
	si.filters = append(si.filters, fn)
	return si
}

func (si setIterator[T]) iterate(ctx context.Context, fn func(context.Context, *Client[T]) (bool, error)) (int, error) {
	// Call prepare on the client
	if err := si.client.prepare(ctx); err != nil {
		return 0, err
	}

	// Track object count
	count := 0

	// Loop start depends on mode
	i := 0
	if si.reverse {
		i = len(si.client.clients) - 1
	}

	// Iterate over clients
	for {
		// Break out the loop at the end of the array
		if (!si.reverse && i >= len(si.client.clients)) || (si.reverse && i < 0) {
			break
		}

		// Get the client
		client := si.client.clients[i]

		// Ignore clients that don't match criteria
		if !client.state.Check(si.condition) {
			goto next
		}

		// Apply user defined filters
		if len(si.filters) != 0 {
			// Get object
			obj := client.getExistingOrCreateObject().DeepCopyObject().(T)

			// Apply filters
			for _, fn := range si.filters {
				if !fn(obj) {
					goto next
				}
			}
		}

		// Increment the count
		count++

		// Run the function
		if fn != nil {
			if cont, err := fn(ctx, client); err != nil || !cont {
				return count, err
			}
		}

	next:
		if si.reverse {
			i--
		} else {
			i++
		}
	}

	// Return the count
	return count, nil
}

type existingSetIterator[T client.Object] struct {
	setIterator[T]
}

func (sa existingSetIterator[T]) Filter(fn func(T) bool) ExistingObjectsSet[T] {
	return existingSetIterator[T]{
		setIterator: sa.setIterator.withFilter(fn),
	}
}

type objectsToCreateIterator[T client.Object] struct {
	setIterator[T]
}

func (sa objectsToCreateIterator[T]) Filter(fn func(T) bool) ObjectsToCreateSet[T] {
	return objectsToCreateIterator[T]{
		setIterator: sa.setIterator.withFilter(fn),
	}
}

type objectsToUpdateIterator[T client.Object] struct {
	setIterator[T]
}

func (sa objectsToUpdateIterator[T]) Filter(fn func(T) bool) ObjectsToUpdateSet[T] {
	return objectsToUpdateIterator[T]{
		setIterator: sa.setIterator.withFilter(fn),
	}
}

type objectsToDeleteIterator[T client.Object] struct {
	setIterator[T]
}

func (sa objectsToDeleteIterator[T]) Filter(fn func(T) bool) ObjectsToDeleteSet[T] {
	return objectsToDeleteIterator[T]{
		setIterator: sa.setIterator.withFilter(fn),
	}
}
