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

package equality_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/kubespress/objects/equality"
)

var _ = Describe("Comparing two unstructured objects", func() {

	var original *unstructured.Unstructured

	BeforeEach(func() {
		original = &unstructured.Unstructured{}
		original.SetAPIVersion("v1")
		original.SetKind("Pod")
		original.SetNamespace("default")
		original.SetName("example")
		unstructured.SetNestedField(original.Object, "original", "spec", "nodeName")
		unstructured.SetNestedField(original.Object, "original", "status", "message")
	})

	When("the spec is the same", func() {

		var updated *unstructured.Unstructured

		BeforeEach(func() {
			updated = original.DeepCopy()
		})

		Context("when comparing without status", func() {
			It("should be equal", func() {
				fmt.Println("CATS:", original.Object["status"])
				fmt.Println("DOGS:", updated.Object["status"])
				result := equality.ObjectsEqualWithoutStatus(equality.Semantic, original, updated)
				Expect(result).To(BeTrue())
			})
		})

		Context("when comparing the status", func() {
			It("should be equal", func() {
				result := equality.ObjectsStatusEqual(equality.Semantic, original, updated)
				Expect(result).To(BeTrue())
			})
		})
	})

	When("the spec is different", func() {

		var updated *unstructured.Unstructured

		BeforeEach(func() {
			updated = original.DeepCopy()
			unstructured.SetNestedField(original.Object, "updated", "spec", "nodeName")
		})

		Context("when comparing without status", func() {
			It("should not be equal", func() {
				result := equality.ObjectsEqualWithoutStatus(equality.Semantic, original, updated)
				Expect(result).To(BeFalse())
			})
		})

		Context("when comparing the status", func() {
			It("should be equal", func() {
				result := equality.ObjectsStatusEqual(equality.Semantic, original, updated)
				Expect(result).To(BeTrue())
			})
		})
	})

	When("the status is the same", func() {

		var updated *unstructured.Unstructured

		BeforeEach(func() {
			updated = original.DeepCopy()
		})

		Context("when comparing without status", func() {
			It("should be equal", func() {
				result := equality.ObjectsEqualWithoutStatus(equality.Semantic, original, updated)
				Expect(result).To(BeTrue())
			})
		})

		Context("when comparing the status", func() {
			It("should be equal", func() {
				result := equality.ObjectsStatusEqual(equality.Semantic, original, updated)
				Expect(result).To(BeTrue())
			})
		})
	})

	When("the status is different", func() {

		var updated *unstructured.Unstructured

		BeforeEach(func() {
			updated = original.DeepCopy()
			unstructured.SetNestedField(original.Object, "updated", "status", "message")
		})

		Context("when comparing without status", func() {
			It("should be equal", func() {
				result := equality.ObjectsEqualWithoutStatus(equality.Semantic, original, updated)
				Expect(result).To(BeTrue())
			})
		})

		Context("when comparing the status", func() {
			It("should not be equal", func() {
				result := equality.ObjectsStatusEqual(equality.Semantic, original, updated)
				Expect(result).To(BeFalse())
			})
		})
	})
})

var _ = Describe("Comparing two objects", func() {

	// Create object
	var original *corev1.Pod

	BeforeEach(func() {
		original = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				NodeName: "original",
			},
			Status: corev1.PodStatus{
				Message: "original",
			},
		}
	})

	When("the spec is the same", func() {

		var updated *corev1.Pod

		BeforeEach(func() {
			updated = original.DeepCopy()
		})

		Context("when comparing without status", func() {
			It("should be equal", func() {
				result := equality.ObjectsEqualWithoutStatus(equality.Semantic, original, updated)
				Expect(result).To(BeTrue())
			})
		})

		Context("when comparing the status", func() {
			It("should be equal", func() {
				result := equality.ObjectsStatusEqual(equality.Semantic, original, updated)
				Expect(result).To(BeTrue())
			})
		})
	})

	When("the spec is different", func() {

		var updated *corev1.Pod

		BeforeEach(func() {
			updated = original.DeepCopy()
			updated.Spec.NodeName = "updated"
		})

		Context("when comparing without status", func() {
			It("should not be equal", func() {
				result := equality.ObjectsEqualWithoutStatus(equality.Semantic, original, updated)
				Expect(result).To(BeFalse())
			})
		})

		Context("when comparing the status", func() {
			It("should be equal", func() {
				result := equality.ObjectsStatusEqual(equality.Semantic, original, updated)
				Expect(result).To(BeTrue())
			})
		})
	})

	When("the status is the same", func() {

		var updated *corev1.Pod

		BeforeEach(func() {
			updated = original.DeepCopy()
		})

		Context("when comparing without status", func() {
			It("should be equal", func() {
				result := equality.ObjectsEqualWithoutStatus(equality.Semantic, original, updated)
				Expect(result).To(BeTrue())
			})
		})

		Context("when comparing the status", func() {
			It("should be equal", func() {
				result := equality.ObjectsStatusEqual(equality.Semantic, original, updated)
				Expect(result).To(BeTrue())
			})
		})
	})

	When("the status is different", func() {

		var updated *corev1.Pod

		BeforeEach(func() {
			updated = original.DeepCopy()
			updated.Status.Message = "updated"
		})

		Context("when comparing without status", func() {
			It("should be equal", func() {
				result := equality.ObjectsEqualWithoutStatus(equality.Semantic, original, updated)
				Expect(result).To(BeTrue())
			})
		})

		Context("when comparing the status", func() {
			It("should not be equal", func() {
				result := equality.ObjectsStatusEqual(equality.Semantic, original, updated)
				Expect(result).To(BeFalse())
			})
		})
	})
})
