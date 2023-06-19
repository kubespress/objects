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
	"context"

	"github.com/kubespress/objects"
	"github.com/kubespress/objects/internal/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta/testrestmapper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

var _ = Describe("Client", func() {
	var (
		ctx    context.Context
		client client.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		client = fake.NewClientBuilder().WithRESTMapper(testrestmapper.TestOnlyStaticRESTMapper(scheme.Scheme)).Build()
	})

	Context("For namespaced object", func() {
		var (
			builder objects.Builder[*corev1.Pod]
		)

		BeforeEach(func() {
			ctx = context.Background()
			client = fake.NewClientBuilder().WithRESTMapper(testrestmapper.TestOnlyStaticRESTMapper(scheme.Scheme)).Build()
			builder = objects.New[*corev1.Pod](client)
		})

		Context("With object name specified", func() {

			BeforeEach(func() {
				builder = builder.WithName("example")
			})

			Context("With object namespaces specified", func() {
				BeforeEach(func() {
					builder = builder.WithNamespace("default")
				})

				Context("For existing object", func() {
					var (
						pod          *corev1.Pod
						objectClient *objects.Client[*corev1.Pod]
					)

					BeforeEach(func() {
						pod = &corev1.Pod{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "default",
								Name:      "another-name",
							},
							Spec: corev1.PodSpec{
								NodeName: "original",
							},
						}

						err := client.Create(ctx, pod)
						Expect(err).ToNot(HaveOccurred())

						objectClient = builder.Finalize().ClientForExistingObject(pod)
					})

					DescribeTable("On the constructed object client",
						func(fn func(*objects.Client[*corev1.Pod], context.Context) error, validate func(error, *corev1.Pod)) {
							err := fn(objectClient, ctx)
							validate(err, pod)
						},
						Entry("When Get method is called, no error should be returned", Get[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).ToNot(HaveOccurred())
						}),
						Entry("When Create method is called, already exists error should be returned", Create[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).To(matchers.BeAlreadyExistsError())
						}),
						Entry("When Update method is called, not found error should be returned", Update[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).ToNot(HaveOccurred())
						}),
						Entry("When Delete method is called, object should be deleted", Delete[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).ToNot(HaveOccurred())
							Expect(pod).ToNot(matchers.Exists(client))
						}),
						Entry("When CreateIfNotExists method is called, no error should be returned", CreateIfNotExists[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).ToNot(HaveOccurred())
							Expect(pod).To(matchers.Exists(client))
						}),
						Entry("When DeleteIfExists method is called, object should be deleted", DeleteIfExists[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).ToNot(HaveOccurred())
							Expect(pod).ToNot(matchers.Exists(client))
						}),
						Entry("When CreateOrUpdate method is called, no error should be returned", CreateOrUpdate[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).ToNot(HaveOccurred())
						}),
						Entry("When UpdateIfRequired method is called, no error should be returned", UpdateIfRequired[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).ToNot(HaveOccurred())
						}),
						Entry("When DeleteIfUpdateRequired method is called, no error should be returned", DeleteIfUpdateRequired[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).ToNot(HaveOccurred())
							Expect(pod).To(matchers.Exists(client))
						}),
					)
				})

				Context("Object requires update", func() {
					var (
						pod          *corev1.Pod
						objectClient *objects.Client[*corev1.Pod]
					)

					BeforeEach(func() {
						builder = builder.WithBuilder(func(pod *corev1.Pod, action objects.Action) error {
							pod.Spec.NodeName = "updated"
							return nil
						})

						pod = &corev1.Pod{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "default",
								Name:      "example",
							},
							Spec: corev1.PodSpec{
								NodeName: "original",
							},
						}

						err := client.Create(ctx, pod)
						Expect(err).ToNot(HaveOccurred())

						objectClient = builder.Finalize().Client()
					})

					DescribeTable("On the constructed object client",
						func(fn func(*objects.Client[*corev1.Pod], context.Context) error, validate func(error, *corev1.Pod)) {
							err := fn(objectClient, ctx)
							validate(err, pod)
						},
						Entry("When Get method is called, no error should be returned", Get[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).ToNot(HaveOccurred())
						}),
						Entry("When Create method is called, already exists error should be returned", Create[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).To(matchers.BeAlreadyExistsError())
						}),
						Entry("When Update method is called, not found error should be returned", Update[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).ToNot(HaveOccurred())
							Expect(pod).To(matchers.Match(client, HaveField("Spec.NodeName", "updated")))
						}),
						Entry("When Delete method is called, object should be deleted", Delete[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).ToNot(HaveOccurred())
							Expect(pod).ToNot(matchers.Exists(client))
						}),
						Entry("When CreateIfNotExists method is called, no error should be returned", CreateIfNotExists[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).ToNot(HaveOccurred())
							Expect(pod).To(matchers.Exists(client))
						}),
						Entry("When DeleteIfExists method is called, object should be deleted", DeleteIfExists[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).ToNot(HaveOccurred())
							Expect(pod).ToNot(matchers.Exists(client))
						}),
						Entry("When CreateOrUpdate method is called, object should be updated", CreateOrUpdate[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).ToNot(HaveOccurred())
							Expect(pod).To(matchers.Match(client, HaveField("Spec.NodeName", "updated")))
						}),
						Entry("When UpdateIfRequired method is called, object should be updated", UpdateIfRequired[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).ToNot(HaveOccurred())
							Expect(pod).To(matchers.Match(client, HaveField("Spec.NodeName", "updated")))
						}),
						Entry("When DeleteIfUpdateRequired method is called, object should be deleted", DeleteIfUpdateRequired[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).ToNot(HaveOccurred())
							Expect(pod).ToNot(matchers.Exists(client))
						}),
					)
				})

				Context("Object already exists", func() {
					var (
						pod          *corev1.Pod
						objectClient *objects.Client[*corev1.Pod]
					)

					BeforeEach(func() {
						pod = &corev1.Pod{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "default",
								Name:      "example",
							},
						}

						err := client.Create(ctx, pod)
						Expect(err).ToNot(HaveOccurred())

						objectClient = builder.Finalize().Client()
					})

					DescribeTable("On the constructed object client",
						func(fn func(*objects.Client[*corev1.Pod], context.Context) error, validate func(error, *corev1.Pod)) {
							err := fn(objectClient, ctx)
							validate(err, pod)
						},
						Entry("When Get method is called, no error should be returned", Get[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).ToNot(HaveOccurred())
						}),
						Entry("When Create method is called, already exists error should be returned", Create[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).To(matchers.BeAlreadyExistsError())
						}),
						Entry("When Update method is called, not found error should be returned", Update[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).ToNot(HaveOccurred())
						}),
						Entry("When Delete method is called, object should be deleted", Delete[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).ToNot(HaveOccurred())
							Expect(pod).ToNot(matchers.Exists(client))
						}),
						Entry("When CreateIfNotExists method is called, no error should be returned", CreateIfNotExists[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).ToNot(HaveOccurred())
							Expect(pod).To(matchers.Exists(client))
						}),
						Entry("When DeleteIfExists method is called, object should be deleted", DeleteIfExists[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).ToNot(HaveOccurred())
							Expect(pod).ToNot(matchers.Exists(client))
						}),
						Entry("When CreateOrUpdate method is called, no error should be returned", CreateOrUpdate[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).ToNot(HaveOccurred())
						}),
						Entry("When UpdateIfRequired method is called, no error should be returned", UpdateIfRequired[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).ToNot(HaveOccurred())
						}),
						Entry("When DeleteIfUpdateRequired method is called, no error should be returned", DeleteIfUpdateRequired[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).ToNot(HaveOccurred())
							Expect(pod).To(matchers.Exists(client))
						}),
					)
				})

				Context("Object does not exist", func() {
					var (
						pod          *corev1.Pod
						objectClient *objects.Client[*corev1.Pod]
					)

					BeforeEach(func() {
						pod = &corev1.Pod{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "default",
								Name:      "example",
							},
						}

						objectClient = builder.Finalize().Client()
					})

					DescribeTable("On the constructed object client",
						func(fn func(*objects.Client[*corev1.Pod], context.Context) error, validate func(error, *corev1.Pod)) {
							err := fn(objectClient, ctx)
							validate(err, pod)
						},
						Entry("When Get method is called, not found error should be returned", Get[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).To(matchers.BeNotFoundError())
						}),
						Entry("When Create method is called, object should be created", Create[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).ToNot(HaveOccurred())
							Expect(pod).To(matchers.Exists(client))
						}),
						Entry("When Update method is called, not found error should be returned", Update[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).To(matchers.BeNotFoundError())
							Expect(pod).ToNot(matchers.Exists(client))
						}),
						Entry("When Delete method is called, not found error should be returned", Delete[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).To(matchers.BeNotFoundError())
							Expect(pod).ToNot(matchers.Exists(client))
						}),
						Entry("When CreateIfNotExists method is called, object should be created", CreateIfNotExists[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).ToNot(HaveOccurred())
							Expect(pod).To(matchers.Exists(client))
						}),
						Entry("When DeleteIfExists method is called, no error should be returned", DeleteIfExists[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).ToNot(HaveOccurred())
							Expect(pod).ToNot(matchers.Exists(client))
						}),
						Entry("When CreateOrUpdate method is called, object should be created", CreateOrUpdate[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).ToNot(HaveOccurred())
							Expect(pod).To(matchers.Exists(client))
						}),
						Entry("When UpdateIfRequired method is called, not found error should be returned", UpdateIfRequired[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).To(matchers.BeNotFoundError())
							Expect(pod).ToNot(matchers.Exists(client))
						}),
						Entry("When DeleteIfUpdateRequired method is called, no error should be returned", DeleteIfUpdateRequired[*corev1.Pod], func(err error, pod *corev1.Pod) {
							Expect(err).ToNot(HaveOccurred())
							Expect(pod).ToNot(matchers.Exists(client))
						}),
					)
				})
			})
		})
	})
})

func Get[T client.Object](client *objects.Client[T], ctx context.Context) error {
	_, err := client.Get(ctx)
	return err
}

func Create[T client.Object](client *objects.Client[T], ctx context.Context) error {
	return client.Create(ctx)
}

func Update[T client.Object](client *objects.Client[T], ctx context.Context) error {
	return client.Update(ctx)
}

func Delete[T client.Object](client *objects.Client[T], ctx context.Context) error {
	return client.Delete(ctx)
}

func CreateIfNotExists[T client.Object](client *objects.Client[T], ctx context.Context) error {
	return client.CreateIfNotExists(ctx)
}

func DeleteIfExists[T client.Object](client *objects.Client[T], ctx context.Context) error {
	return client.DeleteIfExists(ctx)
}

func CreateOrUpdate[T client.Object](client *objects.Client[T], ctx context.Context) error {
	return client.CreateOrUpdate(ctx)
}

func UpdateIfRequired[T client.Object](client *objects.Client[T], ctx context.Context) error {
	return client.UpdateIfRequired(ctx)
}

func DeleteIfUpdateRequired[T client.Object](client *objects.Client[T], ctx context.Context) error {
	return client.DeleteIfUpdateRequired(ctx)
}
