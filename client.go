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

	"github.com/kubespress/errors"
	"github.com/kubespress/objects/internal/state"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Client is used to create/update/delete a Kubernetes object. It is created using the a Builder.
type Client[T client.Object] struct {
	builder *FinalizedBuilder[T]

	objectName, objectNamespace                      string
	state                                            state.State
	objectToCreate, existingObject, calculatedUpdate T
}

func newClient[T client.Object](builder *FinalizedBuilder[T]) *Client[T] {
	return &Client[T]{
		builder: builder,
	}
}

func newClientForExistingObject[T client.Object](builder *FinalizedBuilder[T], obj T) *Client[T] {
	// Create state
	var s state.State

	// Object is known to exist
	s.Set(state.ObjectExists)

	// Check if object is pending deletion
	if obj.GetDeletionTimestamp() != nil {
		s.Set(state.ObjectPendingDeletion)
	}

	// Return the client
	return &Client[T]{
		builder:         builder,
		objectName:      obj.GetName(),
		objectNamespace: obj.GetNamespace(),
		state:           s,
		existingObject:  obj,
	}
}

// Get returns the current object from Kubernetes. This may not do a request if the object has already been retrieved by
// this client. If the object does not exist in Kubernetes an error is returned.
func (c *Client[T]) Get(ctx context.Context) (T, error) {
	// Setup the client
	if err := c.prepare(ctx); err != nil {
		return *new(T), err
	}

	// Return error if object does not exist
	if c.state.Check(state.ObjectDoesNotExist) {
		return *new(T), c.errNotFound()
	}

	// Return no error
	return c.existingObject.DeepCopyObject().(T), nil
}

// Create attempts to create the object in Kubernetes. It will return an error if the object already exists.
func (c *Client[T]) Create(ctx context.Context) error {
	// Setup the client
	if err := c.prepare(ctx); err != nil {
		return err
	}

	// Clone the create object
	obj := c.objectToCreate.DeepCopyObject().(T)

	// Call the pre-create hook
	c.builder.builder.hooks.PreCreate(ctx, obj)

	// Return error if object already exists
	if c.state.Check(state.ObjectExists) {
		err := c.errAlreadyExists()
		c.builder.builder.hooks.CreateError(ctx, obj, err)
		return err
	}

	// Perform the create
	if err := c.builder.builder.Client.Create(ctx, obj); err != nil {
		c.builder.builder.hooks.CreateError(ctx, obj, err)
		return err
	}

	// Call the post-create hook
	c.builder.builder.hooks.PostCreate(ctx, obj)

	// Update the state
	c.state.Set(state.ObjectExists)
	c.existingObject = obj

	// If the object has an action of "create", it should transition into having and action of "keep"
	if c.state.Check(state.SetActionCreate) {
		c.state.Set(state.SetActionKeep)
	}

	// If we are using `metadata.generateName` then we need to track the generated name of the object. If we are not
	// using `metadata.generateName` then this update will be a no-op.
	c.objectName = obj.GetName()
	c.objectNamespace = obj.GetNamespace()

	// Return no error
	return nil
}

// Update will update the object within Kubernetes. It will return an error if the object does not exist.
func (c *Client[T]) Update(ctx context.Context) error {
	// Setup the client
	if err := c.prepare(ctx); err != nil {
		return err
	}

	// Return error if object does not exist
	if c.state.Check(state.ObjectDoesNotExist) {
		// Simulate call by calling PreUpdate, but use the c.create object as
		// it's not nil
		c.builder.builder.hooks.PreUpdate(ctx, c.existingObject, c.objectToCreate)

		// Create a NotFound error
		err := c.errNotFound()

		// Call the UpdateError hook
		c.builder.builder.hooks.UpdateError(ctx, c.existingObject, c.objectToCreate, err)

		// Return the error
		return err
	}

	// Clone the update object
	obj := c.calculatedUpdate.DeepCopyObject().(T)

	// Call the pre-update hook
	c.builder.builder.hooks.PreUpdate(ctx, c.existingObject, obj)

	// Perform the update
	if err := c.builder.builder.Client.Update(ctx, obj); err != nil {
		c.builder.builder.hooks.UpdateError(ctx, c.existingObject, obj, err)
		return err
	}

	// Call the post-update hook
	c.builder.builder.hooks.PostUpdate(ctx, c.existingObject, obj)

	// Mark the object as up to date, the next prepare call will recalculate this anyway since the resource versions
	// will no longer match.
	c.state.Set(state.ObjectUpToDate)

	// Store the new object
	c.existingObject = obj

	// Return no error
	return nil
}

// Delete will delete the object within Kubernetes,  It will return an error if the object does not exist.
func (c *Client[T]) Delete(ctx context.Context) error {
	// Setup the client
	if err := c.prepare(ctx); err != nil {
		return err
	}

	// Return error if object does not exist
	if c.state.Check(state.ObjectDoesNotExist) {
		// Simulate call by calling PreDelete, but use the c.create object as
		// it's not nil
		c.builder.builder.hooks.PreDelete(ctx, c.objectToCreate)

		// Create a NotFound error
		err := c.errNotFound()

		// Call the DeleteError hook
		c.builder.builder.hooks.DeleteError(ctx, c.objectToCreate, err)

		// Return the error
		return err
	}

	// Call the pre-delete hook
	c.builder.builder.hooks.PreDelete(ctx, c.existingObject)

	// Perform the delete
	if err := c.builder.builder.Client.Delete(ctx, c.existingObject); err != nil {
		c.builder.builder.hooks.DeleteError(ctx, c.existingObject, err)
		return err
	}

	// Call the post-delete hook
	c.builder.builder.hooks.PostDelete(ctx, c.existingObject)

	// Mark the object as pending deletion, we don't mark the item as fully deleted as we cannot confirm this. However
	// subsequent calls to prepare will check if the object still exists.
	c.state.Set(state.ObjectPendingDeletion)

	// Return no error
	return nil
}

// CreateIfNotExists creates the object if it does not exist.
func (c *Client[T]) CreateIfNotExists(ctx context.Context) error {
	// Setup the client
	if err := c.prepare(ctx); err != nil {
		return err
	}

	// If the object exists, do nothing
	if c.state.Check(state.ObjectExists) {
		return nil
	}

	// Perform create
	if err := c.Create(ctx); err != nil {
		// Since this object does not fail if the object already exists we don't return an error if the Create returns
		// an IsAlreadyExists
		// error.
		if !apierrors.IsAlreadyExists(err) {
			return err
		}

		// Set the existence to unknown to force the next call to refresh the object from Kubernetes
		c.state.Set(state.ObjectExistenceUnknown)

		// Return no error
		return nil
	}

	// Return no error
	return nil
}

// DeleteIfExists deletes the object from Kubernetes if it exists.
func (c *Client[T]) DeleteIfExists(ctx context.Context) error {
	// Setup the client
	if err := c.prepare(ctx); err != nil {
		return err
	}

	// If the object does not exist, do nothing
	if c.state.Check(state.ObjectDoesNotExist) {
		return nil
	}

	// If pending delete, do nothing
	if c.state.Check(state.ObjectPendingDeletion) {
		return nil
	}

	// Perform create
	if err := c.Delete(ctx); err != nil {
		// Since this method only deletes the object if it exists, we ignore the "NotFound" error
		if apierrors.IsNotFound(err) {
			return nil
		}

		// Return the error
		return err
	}

	// Return no error
	return nil
}

// CreateOrUpdate will create the object within Kubernetes if it does not exist and will update the object within
// Kubernetes if it does. This method can be used to ensure the object is what we expect it to be.
func (c *Client[T]) CreateOrUpdate(ctx context.Context) error {
	// Setup the client
	if err := c.prepare(ctx); err != nil {
		return err
	}

	// If object exists, update it
	if c.state.Check(state.ObjectExists) {
		return c.UpdateIfRequired(ctx)
	}

	// If object does not exist, create it.
	if err := c.Create(ctx); err != nil {
		// If the object already exists then we should retry this flow to see
		// if it needs updating
		if !apierrors.IsAlreadyExists(err) {
			return err
		}

		// Force the object to be re-obtained by setting the state to unknown
		c.state.Set(state.ObjectExistenceUnknown)

		// Retry the call
		return c.CreateOrUpdate(ctx)
	}

	// Return no error
	return nil
}

// UpdateIfRequired will update the object within Kubernetes if the UpdateStrategy has determined an update is required.
// If the object does not exist an error is returned. On conflict the method will retry the entire process, including:
// - Obtaining the current object from Kubernetes
// - Running the build functions
// - Determining if an update is required using the UpdateStrategy
// - Possibly performing an update, if the UpdateStrategy determined it is necessary
func (c *Client[T]) UpdateIfRequired(ctx context.Context) error {
	// Setup the client
	if err := c.prepare(ctx); err != nil {
		return err
	}

	// Return error if object does not exist
	if c.state.Check(state.ObjectDoesNotExist) {
		return c.errNotFound()
	}

	// If no update is required, do nothing
	if c.state.Check(state.ObjectUpToDate) {
		return nil
	}

	// Perform update
	if err := c.Update(ctx); err != nil {
		// If the update is a conflict, we can re-obtain the current object and retry. All other errors should be
		// returned.
		if !apierrors.IsConflict(err) {
			return err
		}

		// Force the object to be re-obtained by setting the state to unknown
		c.state.Set(state.ObjectExistenceUnknown)

		// Retry the update
		return c.UpdateIfRequired(ctx)
	}

	// Return no error
	return nil
}

func (c *Client[T]) DeleteIfUpdateRequired(ctx context.Context) error {
	// Setup the client
	if err := c.prepare(ctx); err != nil {
		return err
	}

	// If no object exists, do nothing
	if c.state.Check(state.ObjectDoesNotExist) {
		return nil
	}

	// If no update is required, do nothing
	if c.state.Check(state.ObjectUpToDate) {
		return nil
	}

	// If pending delete, do nothing
	if c.state.Check(state.ObjectPendingDeletion) {
		return nil
	}

	// Delete the object
	return c.Delete(ctx)
}

// Exists returns true if the object exists
func (c *Client[T]) Exists(ctx context.Context) (bool, error) {
	// Setup the client
	if err := c.prepare(ctx); err != nil {
		return false, err
	}

	// Return true if update is required
	return c.state.Check(state.ObjectExists), nil
}

// UpdateRequired returns true if the object exists and an update is required
func (c *Client[T]) UpdateRequired(ctx context.Context) (bool, error) {
	// Setup the client
	if err := c.prepare(ctx); err != nil {
		return false, err
	}

	// Return true if update is required
	return c.state.Check(state.ObjectRequiresUpdate), nil
}

// getExistingOrCreateObject returns the internal object, if the object exists in Kubernetes, that object will be
// returned, if not then the object that would be created will be returned.
func (c *Client[T]) getExistingOrCreateObject() T {
	if c.state.Check(state.ObjectExists) {
		return c.existingObject
	}

	return c.objectToCreate
}

func (c *Client[T]) prepare(ctx context.Context) error {
	// Validate the client exists
	if c.builder.builder.Client == nil {
		return errors.Errorf("no client provided")
	}

	// Get resource metadata
	metadata, err := c.builder.ResourceMetadata()
	if err != nil {
		return err
	}

	// Client does not work on lists
	if metadata.IsList {
		return errors.Errorf("cannot use %s in client: type must not be list", metadata.GroupKind())
	}

	// If the create object is not set, we should build it. This should only need to happen once per client as the
	// builder cannot be updated once it belongs to a client.
	if c.state.Check(state.ClientUninitialized) {
		// Create the object using the builder
		obj, err := c.builder.Create()
		if err != nil {
			return err
		}

		// Ensure the GroupVersionKind information is set on the object. This is not necessary for anything to work, but
		// may be useful information for the hooks.
		obj.GetObjectKind().SetGroupVersionKind(metadata.GroupVersionKind)

		// Run the PreCreate method on the update strategy
		c.builder.builder.UpdateStrategy.PreCreate(obj)

		// Validate the builder sets the namespace if the object is namespaced
		if metadata.Namespaced && obj.GetNamespace() == "" {
			return errors.Errorf("builder did not set the namespace of the object")
		}

		// Validate the builder set the name
		if obj.GetName() == "" && obj.GetGenerateName() == "" {
			return errors.Errorf("builder did not set the name of the object")
		}

		// Cache the name of the object, this allows us to use the cached name when doing get operations. We also update
		// the cache once the object is created, allowing the builder to use `metadata.generateName` if desired.
		if c.objectName == "" {
			c.objectName = obj.GetName()
		}

		// Also cache the namespace
		if c.objectNamespace == "" {
			c.objectNamespace = obj.GetNamespace()
		}

		// Store the result
		c.objectToCreate = obj

		// Update the state
		c.state.Set(state.ClientInitialized)
	}

	// If we don't know if the object exists, or if it is pending deletion, then we want to get the object
	if c.state.Check(state.ObjectExistenceUnknown) || c.state.Check(state.ObjectPendingDeletion) {
		// If the name is not set, this is a builder using `metadata.generateName`. In this case we cant get the
		// existing object as we are guaranteed to create a new one.
		if c.objectName == "" {
			// Object does not exist, update the state
			c.state.Set(state.ObjectDoesNotExist)

			// Return early, we cant check if an object that does not exist
			// requires an update
			return nil
		}

		// Build key of object we want to retrieve
		key := client.ObjectKey{Namespace: c.objectNamespace, Name: c.objectName}

		// Get zero object
		obj, err := c.builder.zero()
		if err != nil {
			return err
		}

		// Get the object from Kubernetes into the object we created above
		if err := c.builder.builder.Client.Get(ctx, key, obj); err != nil {
			// IsNotFound is not an error case for us, return any other errors.
			if !apierrors.IsNotFound(err) {
				return err
			}

			// Object does not exist, update the state
			c.state.Set(state.ObjectDoesNotExist)

			// Reset the name to the one from the "create" object. This means that if we are using
			// `metadata.generateName` this will be reset to empty and populated if a new object is created.
			c.objectName = c.objectToCreate.GetName()
			c.objectNamespace = c.objectToCreate.GetNamespace()

			// Return early, we cant check if an object that does not exist
			// requires an update
			return nil
		}

		// Store the retrieved object for later use
		c.existingObject = obj
		c.state.Set(state.ObjectExists)

		// Store if the object is pending deletion
		if obj.GetDeletionTimestamp() != nil {
			c.state.Set(state.ObjectPendingDeletion)
		} else {
			c.state.Set(state.ObjectNotPendingDeletion)
		}
	}

	// If the object does not exist, we cannot check if it needs an update
	if c.state.Check(state.ObjectDoesNotExist) {
		return nil
	}

	// Determine if an update is required, this is recalculated if the resource version of the existing object no longer
	// matches that of the built update object as this means the object in Kubernetes has changed.
	if c.state.Check(state.ObjectUpdateStatusUnknown) || c.existingObject.GetResourceVersion() != c.calculatedUpdate.GetResourceVersion() {
		// Clone the existing object
		obj := c.existingObject.DeepCopyObject().(T)

		// Run the builders over it
		if err := c.builder.Apply(obj, ActionUpdate); err != nil {
			return err
		}

		// If we are getting an existing object we need the name to be consistent
		if obj.GetName() != c.objectName {
			obj.SetName(c.objectName)
		}

		// If we are getting an existing object we need the namespace to be consistent
		if obj.GetNamespace() != c.objectNamespace {
			obj.SetNamespace(c.objectNamespace)
		}

		// Determine if the object requires an update
		if c.builder.builder.UpdateStrategy.RequiresUpdate(c.existingObject, c.objectToCreate, obj) {
			c.state.Set(state.ObjectRequiresUpdate)
		} else {
			c.state.Set(state.ObjectUpToDate)
		}

		// Store the updated object
		c.calculatedUpdate = obj
	}

	// Return no error
	return nil
}

func (c *Client[T]) errNotFound() error {
	metadata, _ := c.builder.ResourceMetadata()
	return apierrors.NewNotFound(metadata.GroupResource(), c.objectName)
}

func (c *Client[T]) errAlreadyExists() error {
	metadata, _ := c.builder.ResourceMetadata()
	return apierrors.NewAlreadyExists(metadata.GroupResource(), c.objectName)
}
