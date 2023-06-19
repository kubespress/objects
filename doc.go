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

// Package objects contains utilities for managing Kubernetes objects, either individually or as a larger set. It is
// designed for use within Kubebuilder and reuses many of the interfaces from the controller-runtime project.
//
// # Builders
//
// Builders and the New method are the main entrypoint of this library, they are used to define and construct Kubernetes
// objects using utility methods. These methods allow common fields to be set such as the namespace, name and owners
// references while also allowing for custom build functions to be included in order to define more bespoke object
// customizations.
//
//	objects.New[*corev1.ServiceAccount](k8sClient).
//		WithNamespace("default").
//		WithName("my-awesome-application").
//		WithControllerReference(parentObject)
//
// # Client
//
// An Client is a type of client the builder can create. It manages asingle Kubernetes object and provides utility
// methods to create, update and delete it. The namespace and name of the object are defined by the builder, the client
// takes this information and can determine if the object exists, and if it is up to date. Using this information
// utility methods like CreateIfNotExists and UpdateIfRequired are provided. The CreateOrUpdate method will be the
// standard method used by most reconcilers as it ensures the object exists and is up to date with a single call, this
// coupled with a controller reference handles most required functionality.
//
//	objects.New[*corev1.ServiceAccount](k8sClient).
//		WithNamespace("default").
//		WithName("my-awesome-application").
//		WithControllerReference(parentObject).
//		Finalize().Client()
//
// # SetClient
//
// An SetClient is a type of client the builder can create. It manages "sets" of objects. When constructing a SetClient
// a desired replica count, SetStrategy and a label selector are provided. This allows the client to discover
// existing objects as well as determine if objects need to be created, updated or deleted. Using this strategy the name
// provided by the builder is ignored as the SetStrategy takes over this function.
//
//	objects.New[*corev1.ServiceAccount](k8sClient).
//		WithNamespace("default").
//		WithControllerReference(parentObject).
//		Finalize().
//		SetClient(objects.GeneratedNameSetStrategy("my-awesome-application", 5), map[string]string{"owner": parentObject.GetName()})
//
// # Update strategies
//
// An update strategy is what is used by the package to determine if an object is out of date and requires and update.
// See the implementations below for more information about included strategies.
//
// # Set strategies
//
// When dealing with sets of objects, objects can name the object using many different strategies. The naming strategy
// also defines what objects are deleted first during scale-down. See the implementations below for more information
// about included strategies.
//
// # Hooks
//
// Hooks can be provided to the builder to allow functions to be called under certain events. This is designed to add a
// point that observability can be injected into the clients. For example a hook to emit a Kubernetes event when an
// object is create/updated/deleted is included in the library.
//
//	objects.New[*corev1.ServiceAccount](k8sClient).
//		WithNamespace("default").
//		WithName("my-awesome-application").
//		WithControllerReference(parentObject).
//		WithHook(EventEmitterHook(eventRecorder, parentObject))
//
// # Usage
//
// Inlining all the builders into a Reconcile method can make it very verbose, so instead it is recommended to break out
// the construction of the client into a separate method that be invoked by the client
//
//	func (r *ExampleReconciler) ServiceAccount(ex *examplev1.Example) *objects.Client[*corev1.ServiceAccount] {
//	    // ServiceAccounts don't have any fields we want to set, so its just an object
//	    objects.New[*corev1.ServiceAccount](k8sClient).
//	        WithNamespace(ex.Namespace).
//	        WithName(ex.Name).
//	        WithControllerReference(ex).
//	        Finalize().Client()
//	}
//
//	func (r *ExampleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
//	    ...
//	    // The most basic usage is using the CreateOrUpdate method to ensure the object is up to date,
//	    // deletion is handled by the OwnerReference.
//	    if err := r.ServiceAccount(&example).CreateOrUpdate(ctx); err != nil {
//	        return ctrl.Result{}, err
//	    }
//	    ...
//	}
package objects
