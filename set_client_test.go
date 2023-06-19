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

var _ = Describe("SetClient", func() {
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
			builder            objects.Builder[*corev1.ConfigMap]
			existingConfigMaps *corev1.ConfigMapList
			outdatedConfigMaps *corev1.ConfigMapList
			unwantedConfigMaps *corev1.ConfigMapList
		)

		BeforeEach(func() {
			existingConfigMaps = &corev1.ConfigMapList{
				Items: []corev1.ConfigMap{
					*ConfigMap("default", "example-0", map[string]string{"outdated": "outdated"}),
					*ConfigMap("default", "example-1", map[string]string{"up-to-date": "up-to-date"}),
					*ConfigMap("default", "example-3", map[string]string{"outdated": "outdated"}),
					*ConfigMap("default", "example-4", map[string]string{"up-to-date": "up-to-date"}),
					*ConfigMap("default", "example-6", map[string]string{"up-to-date": "up-to-date"}), // Should be deleted
					*ConfigMap("default", "example-7", map[string]string{"up-to-date": "up-to-date"}), // Should be deleted
				},
			}

			ctx = context.Background()
			client = fake.NewClientBuilder().WithRESTMapper(testrestmapper.TestOnlyStaticRESTMapper(scheme.Scheme)).WithLists(existingConfigMaps).Build()
			builder = objects.New[*corev1.ConfigMap](client)

			outdatedConfigMaps = &corev1.ConfigMapList{
				Items: []corev1.ConfigMap{
					existingConfigMaps.Items[0],
					existingConfigMaps.Items[2],
				},
			}

			unwantedConfigMaps = &corev1.ConfigMapList{
				Items: []corev1.ConfigMap{
					existingConfigMaps.Items[5],
					existingConfigMaps.Items[4],
				},
			}
		})

		Context("With object name specified", func() {

			BeforeEach(func() {
				builder = builder.WithName("example")
			})

			Context("With object namespaces specified", func() {
				BeforeEach(func() {
					builder = builder.WithNamespace("default")
				})

				Context("With set client", func() {
					var (
						setClient *objects.SetClient[*corev1.ConfigMap]
					)

					BeforeEach(func() {
						builder = builder.WithBuilder(func(configMap *corev1.ConfigMap, action objects.Action) error {
							configMap.Data = map[string]string{
								"up-to-date": "up-to-date",
							}

							return nil
						})

						setClient = builder.Finalize().SetClient(objects.IndexedSetStrategy("example", 6), objects.Selector{
							"example.kubebuilder.com/app": "example",
						})
					})

					Context("With Existing object set", func() {
						var (
							existingObjectSet objects.ExistingObjectsSet[*corev1.ConfigMap]
						)

						BeforeEach(func() {
							existingObjectSet = setClient.Existing()
						})

						When("Each is called", func() {
							var (
								returned []corev1.ConfigMap
							)

							BeforeEach(func() {
								err := existingObjectSet.Each(ctx, func(ctx context.Context, client *objects.Client[*corev1.ConfigMap]) error {
									configMap, err := client.Get(ctx)
									if err != nil {
										return err
									}

									returned = append(returned, *configMap)
									return nil
								})

								Expect(err).ToNot(HaveOccurred())
							})

							It("Should call the callback for each existing object", func() {
								Expect(returned).To(HaveLen(6))
								Expect(returned).To(Equal(existingConfigMaps.Items))
							})
						})

						When("Count is called", func() {
							var (
								count int
							)

							BeforeEach(func() {
								var err error
								count, err = existingObjectSet.Count(ctx)
								Expect(err).ToNot(HaveOccurred())
							})

							It("Should return the number of objects", func() {
								Expect(count).To(Equal(6))
							})
						})

						When("Filter is called", func() {
							var (
								count int
							)

							BeforeEach(func() {
								var err error
								count, err = existingObjectSet.Filter(func(cm *corev1.ConfigMap) bool {
									return cm.Name == "example-0" || cm.Name == "example-7"
								}).Count(ctx)
								Expect(err).ToNot(HaveOccurred())
							})

							It("Should return set with filtered values", func() {
								Expect(count).To(Equal(2))
							})
						})

						When("DeleteOne is called", func() {
							var (
								deleted bool
							)

							BeforeEach(func() {
								var err error
								deleted, err = existingObjectSet.DeleteOne(ctx)
								Expect(err).ToNot(HaveOccurred())
							})

							It("Should delete a single object", func() {
								Expect(deleted).To(BeTrue())
								Expect(ConfigMap("default", "example-0", nil)).ToNot(matchers.Exists(client))
								Expect(ConfigMap("default", "example-1", nil)).To(matchers.Exists(client))
								Expect(ConfigMap("default", "example-3", nil)).To(matchers.Exists(client))
								Expect(ConfigMap("default", "example-4", nil)).To(matchers.Exists(client))
								Expect(ConfigMap("default", "example-6", nil)).To(matchers.Exists(client))
								Expect(ConfigMap("default", "example-7", nil)).To(matchers.Exists(client))
							})
						})

						When("DeleteAll is called", func() {
							var (
								deleted int
							)

							BeforeEach(func() {
								var err error
								deleted, err = existingObjectSet.DeleteAll(ctx)
								Expect(err).ToNot(HaveOccurred())
							})

							It("Should delete all objects object", func() {
								Expect(deleted).To(Equal(6))
								Expect(ConfigMap("default", "example-0", nil)).ToNot(matchers.Exists(client))
								Expect(ConfigMap("default", "example-1", nil)).ToNot(matchers.Exists(client))
								Expect(ConfigMap("default", "example-3", nil)).ToNot(matchers.Exists(client))
								Expect(ConfigMap("default", "example-4", nil)).ToNot(matchers.Exists(client))
								Expect(ConfigMap("default", "example-6", nil)).ToNot(matchers.Exists(client))
								Expect(ConfigMap("default", "example-7", nil)).ToNot(matchers.Exists(client))
							})
						})

					})

					Context("With ObjectsToCreate set", func() {
						var (
							objectsToCreateSet objects.ObjectsToCreateSet[*corev1.ConfigMap]
						)

						BeforeEach(func() {
							objectsToCreateSet = setClient.ObjectsToCreate()
						})

						When("Each is called", func() {
							var (
								calls int
							)

							BeforeEach(func() {
								err := objectsToCreateSet.Each(ctx, func(ctx context.Context, client *objects.Client[*corev1.ConfigMap]) error {
									calls++

									exists, err := client.Exists(ctx)
									if err != nil {
										return err
									}

									Expect(exists).To(BeFalse())

									return nil
								})

								Expect(err).ToNot(HaveOccurred())
							})

							It("Should call the callback for each object that should be created", func() {
								Expect(calls).To(Equal(2))
							})
						})

						When("Count is called", func() {
							var (
								count int
							)

							BeforeEach(func() {
								var err error
								count, err = objectsToCreateSet.Count(ctx)
								Expect(err).ToNot(HaveOccurred())
							})

							It("Should return the number of objects", func() {
								Expect(count).To(Equal(2))
							})
						})

						When("Filter is called", func() {
							var (
								count int
							)

							BeforeEach(func() {
								var err error
								count, err = objectsToCreateSet.Filter(func(cm *corev1.ConfigMap) bool {
									return cm.Name == "example-2"
								}).Count(ctx)
								Expect(err).ToNot(HaveOccurred())
							})

							It("Should return set with filtered values", func() {
								Expect(count).To(Equal(1))
							})
						})

						When("CreateOne is called", func() {
							var (
								created bool
							)

							BeforeEach(func() {
								var err error
								created, err = objectsToCreateSet.CreateOne(ctx)
								Expect(err).ToNot(HaveOccurred())
							})

							It("Should create one object", func() {
								Expect(created).To(BeTrue())
								Expect(ConfigMap("default", "example-2", nil)).To(matchers.Exists(client))
								Expect(ConfigMap("default", "example-5", nil)).ToNot(matchers.Exists(client))
							})
						})

						When("CreateAll is called", func() {
							var (
								created int
							)

							BeforeEach(func() {
								var err error
								created, err = objectsToCreateSet.CreateAll(ctx)
								Expect(err).ToNot(HaveOccurred())
							})

							It("Should create all missing object", func() {
								Expect(created).To(Equal(2))
								Expect(ConfigMap("default", "example-2", nil)).To(matchers.Exists(client))
								Expect(ConfigMap("default", "example-5", nil)).To(matchers.Exists(client))
							})
						})
					})

					Context("With ObjectsToUpdate set", func() {
						var (
							objectsToUpdateSet objects.ObjectsToUpdateSet[*corev1.ConfigMap]
						)

						BeforeEach(func() {
							objectsToUpdateSet = setClient.ObjectsToUpdate()
						})

						When("Each is called", func() {
							var (
								returned []corev1.ConfigMap
							)

							BeforeEach(func() {
								err := objectsToUpdateSet.Each(ctx, func(ctx context.Context, client *objects.Client[*corev1.ConfigMap]) error {
									configMap, err := client.Get(ctx)
									if err != nil {
										return err
									}

									returned = append(returned, *configMap)
									return nil
								})

								Expect(err).ToNot(HaveOccurred())
							})

							It("Should call the callback for each existing object", func() {
								Expect(returned).To(HaveLen(2))
								Expect(returned).To(Equal(outdatedConfigMaps.Items))
							})
						})
						When("Count is called", func() {
							var (
								count int
							)

							BeforeEach(func() {
								var err error
								count, err = objectsToUpdateSet.Count(ctx)
								Expect(err).ToNot(HaveOccurred())
							})

							It("Should return the number of objects", func() {
								Expect(count).To(Equal(2))
							})
						})

						When("Filter is called", func() {
							var (
								count int
							)

							BeforeEach(func() {
								var err error
								count, err = objectsToUpdateSet.Filter(func(cm *corev1.ConfigMap) bool {
									return cm.Name == "example-0"
								}).Count(ctx)
								Expect(err).ToNot(HaveOccurred())
							})

							It("Should return set with filtered values", func() {
								Expect(count).To(Equal(1))
							})
						})

						When("UpdateOne is called", func() {
							var (
								updated bool
							)

							BeforeEach(func() {
								var err error
								updated, err = objectsToUpdateSet.UpdateOne(ctx)
								Expect(err).ToNot(HaveOccurred())
							})

							It("Should update one object", func() {
								Expect(updated).To(BeTrue())
								Expect(ConfigMap("default", "example-0", nil)).To(matchers.Match(client, HaveField("Data", map[string]string{"up-to-date": "up-to-date"})))
								Expect(ConfigMap("default", "example-3", nil)).To(matchers.Match(client, HaveField("Data", map[string]string{"outdated": "outdated"})))
							})
						})

						When("UpdateAll is called", func() {
							var (
								updated int
							)

							BeforeEach(func() {
								var err error
								updated, err = objectsToUpdateSet.UpdateAll(ctx)
								Expect(err).ToNot(HaveOccurred())
							})

							It("Should update all objects", func() {
								Expect(updated).To(Equal(2))
								Expect(ConfigMap("default", "example-0", nil)).To(matchers.Match(client, HaveField("Data", map[string]string{"up-to-date": "up-to-date"})))
								Expect(ConfigMap("default", "example-3", nil)).To(matchers.Match(client, HaveField("Data", map[string]string{"up-to-date": "up-to-date"})))
							})
						})

						When("DeleteOne is called", func() {
							var (
								deleted bool
							)

							BeforeEach(func() {
								var err error
								deleted, err = objectsToUpdateSet.DeleteOne(ctx)
								Expect(err).ToNot(HaveOccurred())
							})

							It("Should delete one object that requires an update", func() {
								Expect(deleted).To(BeTrue())
								Expect(ConfigMap("default", "example-0", nil)).ToNot(matchers.Exists(client))
								Expect(ConfigMap("default", "example-3", nil)).To(matchers.Exists(client))
							})
						})

						When("DeleteAll is called", func() {
							var (
								deleted int
							)

							BeforeEach(func() {
								var err error
								deleted, err = objectsToUpdateSet.DeleteAll(ctx)
								Expect(err).ToNot(HaveOccurred())
							})

							It("Should delete all objects that require an update", func() {
								Expect(deleted).To(Equal(2))
								Expect(ConfigMap("default", "example-0", nil)).ToNot(matchers.Exists(client))
								Expect(ConfigMap("default", "example-3", nil)).ToNot(matchers.Exists(client))
							})
						})
					})

					Context("With ObjectsToDelete set", func() {
						var (
							objectsToDeleteSet objects.ObjectsToDeleteSet[*corev1.ConfigMap]
						)

						BeforeEach(func() {
							objectsToDeleteSet = setClient.ObjectsToDelete()
						})

						When("Each is called", func() {
							var (
								returned []corev1.ConfigMap
							)

							BeforeEach(func() {
								err := objectsToDeleteSet.Each(ctx, func(ctx context.Context, client *objects.Client[*corev1.ConfigMap]) error {
									configMap, err := client.Get(ctx)
									if err != nil {
										return err
									}

									returned = append(returned, *configMap)
									return nil
								})

								Expect(err).ToNot(HaveOccurred())
							})

							It("Should call the callback for each existing object", func() {
								Expect(returned).To(HaveLen(2))
								Expect(returned).To(Equal(unwantedConfigMaps.Items))
							})
						})
						When("Count is called", func() {
							var (
								count int
							)

							BeforeEach(func() {
								var err error
								count, err = objectsToDeleteSet.Count(ctx)
								Expect(err).ToNot(HaveOccurred())
							})

							It("Should return the number of objects", func() {
								Expect(count).To(Equal(2))
							})
						})

						When("Filter is called", func() {
							var (
								count int
							)

							BeforeEach(func() {
								var err error
								count, err = objectsToDeleteSet.Filter(func(cm *corev1.ConfigMap) bool {
									return cm.Name == "example-7"
								}).Count(ctx)
								Expect(err).ToNot(HaveOccurred())
							})

							It("Should return set with filtered values", func() {
								Expect(count).To(Equal(1))
							})
						})

						When("DeleteOne is called", func() {
							var (
								deleted bool
							)

							BeforeEach(func() {
								var err error
								deleted, err = objectsToDeleteSet.DeleteOne(ctx)
								Expect(err).ToNot(HaveOccurred())
							})

							It("Should delete one object that requires an update", func() {
								Expect(deleted).To(BeTrue())
								// Deletes happen in reverse order
								Expect(ConfigMap("default", "example-6", nil)).To(matchers.Exists(client))
								Expect(ConfigMap("default", "example-7", nil)).ToNot(matchers.Exists(client))
							})
						})

						When("DeleteAll is called", func() {
							var (
								deleted int
							)

							BeforeEach(func() {
								var err error
								deleted, err = objectsToDeleteSet.DeleteAll(ctx)
								Expect(err).ToNot(HaveOccurred())
							})

							It("Should delete all objects that require an update", func() {
								Expect(deleted).To(Equal(2))
								Expect(ConfigMap("default", "example-6", nil)).ToNot(matchers.Exists(client))
								Expect(ConfigMap("default", "example-7", nil)).ToNot(matchers.Exists(client))
							})
						})
					})
				})
			})
		})
	})
})

func ConfigMap(namespace, name string, data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			Labels: map[string]string{
				"example.kubebuilder.com/app": "example",
			},
		},
		Data: data,
	}
}
