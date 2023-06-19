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

package objects_test

import (
	"github.com/kubespress/objects"
	corev1 "k8s.io/api/core/v1"
)

// In this example we create a `PersistentVolumeClaim`. We also update the
// `PersistentVolumeClaim` if the parent object is updated in order to expand
// volume automatically. The only field that will be updated is the requested
// volume size, and this will only ever get larger.
func Example_conditionalUpdates() {
	// Create client to manage persistent volume claims
	pvcClient := objects.New[*corev1.PersistentVolumeClaim](k8sClient).
		WithNamespace(parentObject.GetNamespace()). // Set the namespace of the PVC
		WithName(parentObject.GetName()).           // Set the name of the PVC
		WithControllerReference(parentObject).
		WithBuilder(func(pvc *corev1.PersistentVolumeClaim, action objects.Action) error {
			// If this is a create operation, just copy the template
			if action == objects.ActionCreate {
				// Copy the template into the PVC object
				*pvc = volumeClaimTemplate
			}

			// If this is an update we only want to update the requested size, even then we only want to expand the
			// volume. If a user wants more drastic action then they should update the PVC themselves.
			desiredStorage := volumeClaimTemplate.Spec.Resources.Requests.Storage()
			if action == objects.ActionUpdate && pvc.Spec.Resources.Requests.Storage().Cmp(*desiredStorage) == -1 {
				pvc.Spec.Resources.Requests[corev1.ResourceStorage] = *desiredStorage
			}

			// Return no error
			return nil
		}).
		Finalize().Client()

	// Ensure pvc exists and is up to date
	if err := pvcClient.CreateOrUpdate(ctx); err != nil {
		// Handle error
	}
}

// This example shows how a single pod can be managed by the builder, since pods are mostly immutable they cannot use
// the CreateOrUpdate method, instead out of date pods are deleted.
//
// Pods are determined to be out of date using the "HashUpdateStrategy", a strategy that uses an annotation on the Pod
// to determine if the Pod needs an update.
func Example_createAPod() {
	// Create client to manage pod
	podClient := objects.New[*corev1.Pod](k8sClient).
		WithNamespace("my-namespace").                                      // Set the namespace of the pod
		WithName("pod-name").                                               // Set the name of the pod
		WithControllerReference(parentObject).                              // Add a controller reference to the pod
		WithUpdateStrategy(objects.HashUpdateStrategy("example.com/hash")). // Use the "Hash" strategy to determine if the pod needs an update
		WithBuilder(func(pod *corev1.Pod, _ objects.Action) error {         // Do custom build operations
			// Add containers to pod
			pod.Spec.Containers = []corev1.Container{
				// ...
			}

			// Return no error
			return nil
		}).
		Finalize().Client()

	// Create new pod if non exists
	if err := podClient.CreateIfNotExists(ctx); err != nil {
		// Handle error
	}

	// Since pods are largely immutable, we delete the pod if it is out of date,
	// the next reconcile will recreate the pod.
	if err := podClient.DeleteIfUpdateRequired(ctx); err != nil {
		// Handle error
	}
}

// This example manages a set of 10 pods, the pods are named using an indexed naming strategy, giving the pods a name of
// my-cache-0 to my-cache-9.
//
// Provided labels are used to discover existing Pods and will be automatically injected into Pods that are created.
func Example_createASetOfPods() {
	// Create client to manage a set of Pods.
	podClient := objects.New[*corev1.Pod](k8sClient).
		WithNamespace("my-namespace").                                      // Set the namespace of the pod
		WithControllerReference(parentObject).                              // Add a controller reference to the pod
		WithUpdateStrategy(objects.HashUpdateStrategy("example.com/hash")). // Use the "Hash" strategy to determine if the pod needs an update
		WithBuilder(func(pod *corev1.Pod, _ objects.Action) error {         // Do custom build operations
			// Add containers to pod
			pod.Spec.Containers = []corev1.Container{
				// ...
			}

			// Return no error
			return nil
		}).
		Finalize().SetClient(objects.IndexedSetStrategy("my-cache", 10), map[string]string{"example.com/owner": parentObject.GetName()})

	// Create any pods that are missing.
	if _, err := podClient.ObjectsToCreate().CreateAll(ctx); err != nil {
		// Handle error
	}

	// Delete out of date pods, since pods are largely immutable the pods will be re-created next reconcile.
	if _, err := podClient.ObjectsToUpdate().DeleteAll(ctx); err != nil {
		// Handle error
	}

	// Delete un-needed pods (due to scale down).
	if _, err := podClient.ObjectsToDelete().DeleteAll(ctx); err != nil {
		// Handle error
	}
}

var setOfConfigMaps []*corev1.ConfigMap

// The ForEachStrategy can be used to create an object for each of an input
// object.
func Example_usingForEachStrategy() {
	// Create set strategy
	strategy, lookup := objects.ForEachSetStrategy("example-prefix", setOfConfigMaps)

	// Create set client that creates a Pod for every ConfigMap
	podsClient := objects.New[*corev1.Pod](k8sClient).
		WithNamespace(parentObject.GetNamespace()).
		WithName(parentObject.GetName()).
		WithControllerReference(parentObject).
		WithBuilder(func(pod *corev1.Pod, _ objects.Action) error {
			// Get the ConfigMap for this pod
			source, _ := lookup.Source(pod)

			// Add the ConfigMap to the pod spec
			pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
				Name: "configmap",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: source.Name,
						},
					},
				},
			})
			return nil
		}).
		Finalize().
		SetClient(strategy, map[string]string{"example.com/owner": parentObject.GetName()})

	// Ensure pods exists and is up to date
	if err := podsClient.Ensure(ctx); err != nil {
		// Handle error
	}
}
