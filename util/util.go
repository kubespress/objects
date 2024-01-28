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

package util

import (
	"context"
	"reflect"

	"github.com/kubespress/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// GetObjects returns the objects of the selected type that are "owners" of the specified object. It will skip objects
// marked as "controllers"
func GetOwners[T client.Object](ctx context.Context, c client.Client, scheme *runtime.Scheme, obj client.Object) ([]T, error) {
	// Get new object
	parent, err := NewObject[T]()
	if err != nil {
		return nil, err
	}

	// Get the GVK
	gvk, err := apiutil.GVKForObject(parent, scheme)
	if err != nil {
		return nil, err
	}

	// Create slice to hold results
	results := make([]T, 0)

	// Look for parent
	for _, ref := range obj.GetOwnerReferences() {
		// Skip all controller references
		if pointer.BoolDeref(ref.Controller, false) {
			continue
		}

		// Skip invalid kinds
		if ref.Kind != gvk.Kind {
			continue
		}

		// Parse group version of reference
		gv, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil {
			return nil, errors.Enrich(err,
				errors.Wrap("could not get apiversion of object from schema"),
				errors.WithStack(),
			)
		}

		// Skip invalid groups
		if gv.Group != gvk.Group {
			continue
		}

		// Create key for the object
		key := client.ObjectKey{
			Namespace: obj.GetNamespace(),
			Name:      ref.Name,
		}

		// Clone the object
		parent := parent.DeepCopyObject().(T)

		// Get the object
		if err := c.Get(ctx, key, parent); err != nil {
			return nil, err
		}

		// Append to results
		results = append(results, parent)
	}

	// Return found objects
	return results, nil
}

// GetController returns the object that controls the resource. If the object has no controller of the specified type
// then nil is returned.
func GetController[T client.Object](ctx context.Context, c client.Client, scheme *runtime.Scheme, obj client.Object) (T, error) {
	// Get new object
	parent, err := NewObject[T]()
	if err != nil {
		return *new(T), err
	}

	// Get the GVK
	gvk, err := apiutil.GVKForObject(parent, scheme)
	if err != nil {
		return *new(T), err
	}

	// Look for parent
	for _, ref := range obj.GetOwnerReferences() {
		// Skip all non controller reference
		if !pointer.BoolDeref(ref.Controller, false) {
			continue
		}

		// Skip invalid kind
		if ref.Kind != gvk.Kind {
			continue
		}

		// Parse group version of reference
		gv, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil {
			return parent, errors.Enrich(err,
				errors.Wrap("could not get apiversion of object from schema"),
				errors.WithStack(),
			)
		}

		// Skip invalid groups
		if gv.Group != gvk.Group {
			continue
		}

		// Get key of object
		key := client.ObjectKey{
			Namespace: obj.GetNamespace(),
			Name:      ref.Name,
		}

		// Get the object
		if err := c.Get(ctx, key, parent); err != nil {
			return *new(T), err
		}

		// Return no error
		return parent, nil
	}

	return *new(T), nil
}

func NewObject[T client.Object]() (T, error) {
	// Create zero object
	var zeroObj T

	// Get the object as an interface for type inference (cant do that with
	// generics)
	obj := client.Object(zeroObj)

	// Special case for handling unstructured objects
	if _, ok := obj.(runtime.Unstructured); ok {
		return zeroObj, errors.Enrich(
			errors.New("cannot construct unstructured object"),
			errors.WithStack(),
		)
	}

	// Get type of object
	typ := reflect.TypeOf(zeroObj)

	// If its not a pointer, return an error
	if kind := typ.Kind(); kind != reflect.Ptr {
		if kind == reflect.Invalid {
			return *new(T), errors.Errorf("expected pointer, but got invalid kind")
		}
		return *new(T), errors.Errorf("expected pointer, but got %v type", typ)
	}

	// Create new instance of the object
	return reflect.New(typ.Elem()).Interface().(T), nil
}

func newListObject[T client.Object](kubeclient client.Client) (client.ObjectList, error) {
	// Create a normal object
	object, err := NewObject[T]()
	if err != nil {
		return nil, err
	}

	// Get GroupVersionKind of object
	gvk, err := apiutil.GVKForObject(object, kubeclient.Scheme())
	if err != nil {
		return nil, err
	}

	// Edit the GVK to get the list object
	gvk.Kind += "List"

	// Get the list object from the scheme
	list, err := kubeclient.Scheme().New(gvk)
	if err != nil {
		return nil, err
	}

	// Return the list object
	return list.(client.ObjectList), nil
}

func List[T client.Object](ctx context.Context, client client.Client, opts ...client.ListOption) ([]T, error) {
	list, err := newListObject[T](client)
	if err != nil {
		return nil, err
	}

	if err := client.List(ctx, list, opts...); err != nil {
		return nil, err
	}

	itemsPtr, err := meta.GetItemsPtr(list)
	if err != nil {
		return nil, err
	}

	if typed, ok := itemsPtr.(*[]T); ok {
		return *typed, nil
	}

	return nil, errors.Errorf("expected %T got %T", &([]T{}), itemsPtr)
}
