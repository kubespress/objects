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

package resource

import (
	"github.com/kubespress/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// ResourceMetadata contains information about a given object, including the
// Group/Version/Kind and the Group/Version/Resource as well as if the object
// is namespaced
type ResourceMetadata struct {
	schema.GroupVersionResource
	schema.GroupVersionKind
	Namespaced   bool
	Unstructured bool
	IsList       bool
}

// Empty returns true if the object has not yet been populated
func (o ResourceMetadata) Empty() bool {
	return o.GroupVersionResource.Empty() && o.GroupVersionKind.Empty()
}

// GetResourceMetadata returns the ResourceMetadata using the Scheme and RESTMapper embedded in the provided client.
func GetResourceMetadata(client client.Client, object client.Object) (ResourceMetadata, error) {
	// Get GroupVersionKind
	gvk, err := apiutil.GVKForObject(object, client.Scheme())
	if err != nil {
		return ResourceMetadata{}, errors.Enrich(err, errors.Wrap("could not get group/version/kind"))
	}

	// Determine if the object is unstructured
	_, unstructured := object.(runtime.Unstructured)

	// Get REST mapping
	mapping, err := client.RESTMapper().RESTMapping(gvk.GroupKind())
	if err != nil {
		return ResourceMetadata{}, errors.Enrich(err, errors.Wrap("could not get rest mapping"))
	}

	// Return metadata
	return ResourceMetadata{
		GroupVersionKind:     gvk,
		GroupVersionResource: mapping.Resource,
		Namespaced:           mapping.Scope == meta.RESTScopeNamespace,
		Unstructured:         unstructured,
		IsList:               meta.IsListType(object),
	}, nil
}
