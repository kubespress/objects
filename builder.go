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
	"fmt"
	"reflect"
	"strings"

	"github.com/kubespress/objects/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Action is passed to build functions to allow them to change behavior depending on if its a create or update action
// allowing for behaviors such as not updating immutable fields.
type Action uint8

const (
	// ActionCreate gets called when the object is being mutated prior to being created. At the point this is called,
	// the objects existence may not have been determined yet so this value does not guarantee that the eventual
	// operation will be a create.
	ActionCreate Action = iota + 1
	// ActionUpdate gets called when an object is getting mutated prior to being submitted as an update. When the
	// builder is called with this method the object has just been populated from Kubernetes.
	ActionUpdate
)

// BuildFn is a function used to build a given object. The action is passed in to allow the function to change behavior
// based on if it is creating a new instance of an object, or updating an existing object.
type BuildFn[T client.Object] func(T, Action) error

// Builder is used to construct Kubernetes objects using BuildFn functions.
type Builder[T client.Object] struct {
	// Object is an instance of the object, this is only required if the builder is operating on an unstructured object
	// as it is used to get the group/version/kind information required for interacting with Kubernetes.
	//
	// It is possible to use unstructured objects without specifying this field, however the build functions MUST set
	// the group/version/kind information.
	Object T
	// Builders are the build functions that are called to create or update a Kubernetes object.
	Builders []BuildFn[T]
	// UpdateStrategy defines the strategy used to determine if an object requires an update.
	UpdateStrategy UpdateStrategy
	// The Kubernetes client used by any constructed object/set clients.
	Client client.Client
	// Namer is used to name the object.
	Namer Namer[T]

	// The following fields are set by the With<N> methods below, by doing this instead of appending BuildFns to the
	// Builders field we can ensure that the fields are validated in the correct order.
	namespace  string
	owners     []client.Object
	controller client.Object
	hooks      hookSlice

	parent *FinalizedBuilder[T]
}

// New returns a builder for a new object.
func New[T client.Object](client client.Client) Builder[T] {
	return Builder[T]{Client: client}
}

// WithNamespace sets the namespace of the object, if the namespace is set using this method any changes made by build
// functions will be reverted.
func (b Builder[T]) WithNamespace(namespace string) Builder[T] {
	b.namespace = namespace
	return b
}

// WithHook adds a hook to the given builder, a hook does nothing to change the builder behavior, but can be used to
// log/observe the client.
func (b Builder[T]) WithHook(hook Hook) Builder[T] {
	b.hooks = append(b.hooks, hook)
	return b
}

// WithName sets the name of the object, if the name is set using this method any changes made by build functions will
// be reverted.
func (b Builder[T]) WithName(name string) Builder[T] {
	b.Namer = staticNamer[T](name)
	return b
}

// WithGenerateName sets the generated name of the object, if the name is set using this method any changes made by
// build functions will be reverted. If the prefix provided does not end in a hyphen, then an hyphen is appended.
func (b Builder[T]) WithGenerateName(prefix string) Builder[T] {
	b.Namer = generateNamer[T](prefix)
	return b
}

// WithNameFn registers a method that will be called to set the name of the object. If the name is set using this method
// a Builder function cannot change it.
func (b Builder[T]) WithNameFn(fn func(T) error) Builder[T] {
	b.Namer = funcNamer[T](fn)
	return b
}

// WithBuilder adds a build function to the builder. The function should not mutate the status of the object as these
// changes will be ignored.
func (b Builder[T]) WithBuilder(fn BuildFn[T]) Builder[T] {
	b.Builders = append(b.Builders, fn)
	return b
}

// WithUpdateStrategy sets the UpdateStrategy for the generated clients.
func (b Builder[T]) WithUpdateStrategy(strategy UpdateStrategy) Builder[T] {
	b.UpdateStrategy = strategy
	return b
}

// WithLabel adds the provided label to the object.
func (b Builder[T]) WithLabel(key, value string) Builder[T] {
	return b.WithBuilder(func(obj T, action Action) error {
		// Insert into the map
		mapInsert(obj.GetLabels, obj.SetLabels, key, value)

		// Return no error
		return nil
	})
}

// WithLabels adds the provided labels to the object.
func (b Builder[T]) WithLabels(new map[string]string) Builder[T] {
	return b.WithBuilder(func(obj T, action Action) error {
		// Merge into the map
		mapMerge(obj.GetLabels, obj.SetLabels, new)

		// Return no error
		return nil
	})
}

// WithAnnotation adds the provided annotation to the object.
func (b Builder[T]) WithAnnotation(key, value string) Builder[T] {
	return b.WithBuilder(func(obj T, action Action) error {
		// Insert into the map
		mapInsert(obj.GetAnnotations, obj.SetAnnotations, key, value)

		// Return no error
		return nil
	})
}

// WithAnnotations adds the provided annotations to the object.
func (b Builder[T]) WithAnnotations(new map[string]string) Builder[T] {
	return b.WithBuilder(func(obj T, action Action) error {
		// Merge into the map
		mapMerge(obj.GetAnnotations, obj.SetAnnotations, new)

		// Return no error
		return nil
	})
}

// WithFinalizer sets the finalizer if the provided method returns true, otherwise it removes it.
func (b Builder[T]) WithFinalizer(finalizer string, fn func(T) (bool, error)) Builder[T] {
	return b.WithBuilder(func(obj T, action Action) error {
		// Call the provided function
		ok, err := fn(obj)
		if err != nil {
			return err
		}

		// Add or remove finalizer
		if ok {
			controllerutil.AddFinalizer(obj, finalizer)
		} else {
			controllerutil.RemoveFinalizer(obj, finalizer)
		}

		// Return no error
		return nil
	})
}

// WithControllerReference sets controller reference of the object to the provided object. The namespaces of the owner
// and the object being built must match.
func (b Builder[T]) WithControllerReference(owner client.Object) Builder[T] {
	b.controller = owner
	return b
}

// WithControllerReference adds an owner reference of the object pointing to the provided object. The namespaces of the
// owner and the object being built must match.
func (b Builder[T]) WithOwnerReference(owner client.Object) Builder[T] {
	b.owners = append(b.owners, owner)
	return b
}

// Finalize returns a builder that is ready to be used by the client. The default values are set, and the internal
// slices are cloned to prevent updates to the builder instance the client is using.
func (b Builder[T]) Finalize() *FinalizedBuilder[T] {
	return &FinalizedBuilder[T]{
		parent:  b.parent,
		builder: b.defaultAndClone(),
	}
}

func (b Builder[T]) defaultAndClone() Builder[T] {
	// Default the update strategy
	if b.UpdateStrategy == nil {
		b.UpdateStrategy = DefaultUpdateStrategy
	}

	// Return cloned builder
	return Builder[T]{
		Object:         b.Object,
		Builders:       append([]BuildFn[T]{}, b.Builders...),
		UpdateStrategy: b.UpdateStrategy,
		Client:         b.Client,
		Namer:          b.Namer,
		namespace:      b.namespace,
		owners:         append([]client.Object{}, b.owners...),
		controller:     b.controller,
		hooks:          append(hookSlice{}, b.hooks...),
		parent:         b.parent,
	}
}

// Namer is used to name objects, it should mutate the `metadata.name` and `metadata.generateName` fields only.
type Namer[T client.Object] interface {
	SetObjectName(T) error
}

type staticNamer[T client.Object] string

func (n staticNamer[T]) SetObjectName(obj T) error {
	// Set the `metadata.name` field
	obj.SetName(string(n))

	// Return no error
	return nil
}

type generateNamer[T client.Object] string

func (n generateNamer[T]) SetObjectName(obj T) error {
	// Ensure the prefix ends in a "-"
	prefix := string(n)
	if !strings.HasSuffix(prefix, "-") {
		prefix += "-"
	}

	// Set the `metadata.generateName` field
	obj.SetGenerateName(prefix)

	// Return no error
	return nil
}

type funcNamer[T client.Object] func(T) error

func (n funcNamer[T]) SetObjectName(obj T) error {
	return n(obj)
}

type FinalizedBuilder[T client.Object] struct {
	parent   *FinalizedBuilder[T]
	builder  Builder[T]
	metadata *resource.ResourceMetadata
}

func (b *FinalizedBuilder[T]) child() Builder[T] {
	builder := b.builder.defaultAndClone()
	builder.parent = b
	return builder
}

// Apply runs the builder against the provided object. The operation is information to the build function, allowing it
// to change behavior based on if it is creating a new instance of an object, or updating an existing object.
func (b FinalizedBuilder[T]) Apply(obj T, action Action) error {
	// Set the namespace and name
	if action == ActionCreate {
		if err := b.setNamespaceName(obj); err != nil {
			return err
		}
	}

	// Apply the build functions
	for _, fn := range b.builder.Builders {
		if err := fn(obj, action); err != nil {
			return err
		}
	}

	// Set the namespace and name once again, to ensure the build functions did not alter it
	if action == ActionCreate {
		if err := b.setNamespaceName(obj); err != nil {
			return err
		}
	}

	// Add the owner references
	for _, owner := range b.builder.owners {
		if err := controllerutil.SetOwnerReference(owner, obj, b.builder.Client.Scheme()); err != nil {
			return err
		}
	}

	// Add the controller reference
	if b.builder.controller != nil {
		if err := controllerutil.SetControllerReference(b.builder.controller, obj, b.builder.Client.Scheme()); err != nil {
			return err
		}
	}

	// Return no error
	return nil
}

// Create returns a new instance of the object and runs all the specified build functions over it with ActionCreate
// passed in.
func (b FinalizedBuilder[T]) Create() (T, error) {
	// Create new instance of the object
	newObj, err := b.zero()
	if err != nil {
		return newObj, err
	}

	// Run the builders
	if err := b.Apply(newObj, ActionCreate); err != nil {
		return newObj, err
	}

	// Return the object
	return newObj, nil
}

// Zero returns a new "zero" instance of the object.
func (b FinalizedBuilder[T]) zero() (T, error) {
	// Get the object as an interface for type inference (cant do that with
	// generics)
	obj := client.Object(b.builder.Object)

	// Special case for handling unstructured objects
	if obj, ok := obj.(runtime.Unstructured); ok {
		return obj.NewEmptyInstance().(T), nil
	}

	// Get type of object
	typ := reflect.TypeOf(b.builder.Object)

	// If its not a pointer, return an error
	if kind := typ.Kind(); kind != reflect.Ptr {
		if kind == reflect.Invalid {
			return *new(T), fmt.Errorf("expected pointer, but got invalid kind")
		}
		return *new(T), fmt.Errorf("expected pointer, but got %v type", typ)
	}

	// Create new instance of the object
	return reflect.New(typ.Elem()).Interface().(T), nil
}

// ZeroList returns a list for the object type
func (b *FinalizedBuilder[T]) zeroList() (client.ObjectList, error) {
	// Get the object metadata
	metadata, err := b.ResourceMetadata()
	if err != nil {
		return nil, err
	}

	// Get GroupVersionKind from the context
	gvk := metadata.GroupVersionKind

	// Edit the GVK to get the list object
	gvk.Kind += "List"

	// If the set client uses an unstructured object, then we need an unstructured list
	if metadata.Unstructured {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(gvk)
		return list, nil
	}

	// Get the list object from the scheme
	list, err := b.builder.Client.Scheme().New(gvk)
	if err != nil {
		return nil, err
	}

	// Return the list object
	return list.(client.ObjectList), nil
}

// ResourceMetadata returns information on the objects type
func (b *FinalizedBuilder[T]) ResourceMetadata() (resource.ResourceMetadata, error) {
	// If the metadata has not yet been retrieved, retrieved it
	if b.metadata != nil {
		return *b.metadata, nil
	}

	// If this is not the root builder, recurse onto the parent. This should reduce the number of times we need to
	// get metadata for large sets.
	if b.parent != nil {
		return b.parent.ResourceMetadata()
	}

	// Get zero object
	obj, err := b.zero()
	if err != nil {
		return resource.ResourceMetadata{}, err
	}

	// Get metadata of the zero object
	metadata, err := resource.GetResourceMetadata(b.builder.Client, obj)
	if err != nil {
		return metadata, err
	}

	// Store the metadata for fast lookups
	b.metadata = &metadata

	// Return the metadata
	return metadata, err

}

// Client creates a client to manage the object that the builder will create.
func (b *FinalizedBuilder[T]) Client() *Client[T] {
	return newClient(b)
}

// ClientForExistingObject creates a client that will manage an existing object.
func (b *FinalizedBuilder[T]) ClientForExistingObject(obj T) *Client[T] {
	return newClientForExistingObject(b, obj)
}

// Selector is the label selector used to match existing objects
type Selector map[string]string

// ObjectClient returns a client the can be used to manage an object described
// by this builder.
func (b *FinalizedBuilder[T]) SetClient(strategy SetStrategy, selector Selector) *SetClient[T] {
	return &SetClient[T]{
		builder:   b.child().WithLabels(selector).Finalize(),
		labels:    selector,
		replicas:  strategy.DesiredReplicas(),
		naming:    strategy,
		namespace: b.builder.namespace,
	}
}

func (b *FinalizedBuilder[T]) setNamespaceName(obj T) error {
	// If namer exists, use it to set the name
	if b.builder.Namer != nil {
		if err := b.builder.Namer.SetObjectName(obj); err != nil {
			return err
		}
	}

	// Set the namespace of the object if provided. This is repeated to overide
	// any builders that may have changed the vlaue
	if b.builder.namespace != "" {
		obj.SetNamespace(b.builder.namespace)
	}

	// Return no error
	return nil
}

func mapInsert[K comparable, V any](get func() map[K]V, set func(map[K]V), key K, value V) {
	// Get the map
	dst := get()

	// If the map is nil, create it
	if dst == nil {
		dst = make(map[K]V)
	}

	// Insert the key/value into the map
	dst[key] = value

	// Set the updated map
	set(dst)
}

func mapMerge[K comparable, V any](get func() map[K]V, set func(map[K]V), src map[K]V) {
	// Get the map
	dst := get()

	// If the map is nil, create it
	if dst == nil {
		dst = make(map[K]V)
	}

	// Merge into the map
	for k, v := range src {
		dst[k] = v
	}

	// Set the updated map
	set(dst)
}
