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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta/testrestmapper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
)

var _ = Describe("Builder", func() {
	var (
		builder objects.Builder[*corev1.Pod]
		client  client.Client
	)

	BeforeEach(func() {
		client = fake.NewClientBuilder().WithRESTMapper(testrestmapper.TestOnlyStaticRESTMapper(scheme.Scheme)).Build()
		builder = objects.New[*corev1.Pod](client)
	})

	When("Calling WithName", func() {
		BeforeEach(func() {
			builder = builder.WithName("example")
		})

		It("Should set the object name on created objects", func() {
			pod, err := builder.Finalize().Create()
			Expect(err).NotTo(HaveOccurred())
			Expect(pod.Name).To(Equal("example"))
		})
	})

	When("Calling WithGeneratedName", func() {
		BeforeEach(func() {
			builder = builder.WithGenerateName("example")
		})

		It("Should set the objects generated name on created objects", func() {
			pod, err := builder.Finalize().Create()
			Expect(err).NotTo(HaveOccurred())
			Expect(pod.GenerateName).To(Equal("example-"))
		})
	})

	When("Calling WithNameFn", func() {
		BeforeEach(func() {
			builder = builder.WithNameFn(func(pod *corev1.Pod) error {
				pod.Name = "example"
				return nil
			})
		})

		It("Should set the objects name on created objects", func() {
			pod, err := builder.Finalize().Create()
			Expect(err).NotTo(HaveOccurred())
			Expect(pod.Name).To(Equal("example"))
		})
	})

	When("Calling WithBuilder", func() {
		BeforeEach(func() {
			builder = builder.WithBuilder(func(pod *corev1.Pod, action objects.Action) error {
				pod.Spec.NodeName = "example"
				return nil
			})
		})

		It("Should use the builder to mutate the created objects", func() {
			pod, err := builder.Finalize().Create()
			Expect(err).NotTo(HaveOccurred())
			Expect(pod.Spec.NodeName).To(Equal("example"))
		})
	})

	When("Calling WithLabel", func() {
		BeforeEach(func() {
			builder = builder.WithLabel("kubespress.com/example", "example")
		})

		It("Should add the provided label on created objects", func() {
			pod, err := builder.Finalize().Create()
			Expect(err).NotTo(HaveOccurred())
			Expect(pod.Labels).To(HaveKeyWithValue("kubespress.com/example", "example"))
		})
	})

	When("Calling WithLabels", func() {
		BeforeEach(func() {
			builder = builder.WithLabels(map[string]string{"kubespress.com/example": "example"})
		})

		It("Should add the provided labels on created objects", func() {
			pod, err := builder.Finalize().Create()
			Expect(err).NotTo(HaveOccurred())
			Expect(pod.Labels).To(HaveKeyWithValue("kubespress.com/example", "example"))
		})
	})

	When("Calling WithAnnotation", func() {
		BeforeEach(func() {
			builder = builder.WithAnnotation("kubespress.com/example", "example")
		})

		It("Should add the provided annotation on created objects", func() {
			pod, err := builder.Finalize().Create()
			Expect(err).NotTo(HaveOccurred())
			Expect(pod.Annotations).To(HaveKeyWithValue("kubespress.com/example", "example"))
		})
	})

	When("Calling WithAnnotations", func() {
		BeforeEach(func() {
			builder = builder.WithAnnotations(map[string]string{"kubespress.com/example": "example"})
		})

		It("Should add the provided annotations on created objects", func() {
			pod, err := builder.Finalize().Create()
			Expect(err).NotTo(HaveOccurred())
			Expect(pod.Annotations).To(HaveKeyWithValue("kubespress.com/example", "example"))
		})
	})

	When("Calling WithFinalizer", func() {
		BeforeEach(func() {
			builder = builder.WithFinalizer("kubespress.com/example", func(pod *corev1.Pod) (bool, error) {
				return true, nil
			})
		})

		It("Should add the provided finalizers on created objects", func() {
			pod, err := builder.Finalize().Create()
			Expect(err).NotTo(HaveOccurred())
			Expect(pod.Finalizers).To(ContainElement("kubespress.com/example"))
		})
	})

	When("Calling WithControllerReference", func() {
		BeforeEach(func() {
			builder = builder.WithControllerReference(&corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "example-node",
					UID:  "2378494b-2508-4495-844b-cef0c63701f3",
				},
			})
		})

		It("Should add the provided object as a controller owner reference on created object", func() {
			pod, err := builder.Finalize().Create()
			Expect(err).NotTo(HaveOccurred())
			Expect(pod.OwnerReferences).To(ContainElement(metav1.OwnerReference{
				APIVersion:         "v1",
				Kind:               "Node",
				Name:               "example-node",
				UID:                "2378494b-2508-4495-844b-cef0c63701f3",
				Controller:         pointer.Bool(true),
				BlockOwnerDeletion: pointer.Bool(true),
			}))
		})
	})

	Context("Calling WithOwnerReference", func() {
		BeforeEach(func() {
			builder = builder.WithOwnerReference(&corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "example-node",
					UID:  "2378494b-2508-4495-844b-cef0c63701f3",
				},
			})
		})

		It("Should add the provided object as a owner reference on created objects", func() {
			pod, err := builder.Finalize().Create()
			Expect(err).NotTo(HaveOccurred())
			Expect(pod.OwnerReferences).To(ContainElement(metav1.OwnerReference{
				APIVersion: "v1",
				Kind:       "Node",
				Name:       "example-node",
				UID:        "2378494b-2508-4495-844b-cef0c63701f3",
			}))
		})
	})

	Context("Calling ResourceMetadata", func() {
		It("Should return the resource metadata", func() {
			metadata, err := builder.Finalize().ResourceMetadata()
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata.GroupVersionKind).To(Equal(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}))
			Expect(metadata.GroupVersionResource).To(Equal(schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}))
			Expect(metadata.Namespaced).To(BeTrue())
			Expect(metadata.Unstructured).To(BeFalse())
			Expect(metadata.IsList).To(BeFalse())
		})
	})
})
