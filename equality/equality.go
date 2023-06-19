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

package equality

import (
	"reflect"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/conversion"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Clone semantic equality
var Semantic = Equalities{
	Equalities: equality.Semantic.Copy(),
}

// Equalities wraps the equality functionality provided by conversion.Equalities, providing methods to ignore or focus
// on the status field.
type Equalities struct {
	conversion.Equalities
}

// ObjectsEqualWithoutStatus returns true if the objects are equal ignoring the status field.
func (e Equalities) ObjectsEqualWithoutStatus(a1, a2 client.Object) bool {
	return e.DeepEqual(
		withoutStatusField(reflect.ValueOf(a1)).Interface(),
		withoutStatusField(reflect.ValueOf(a2)).Interface(),
	)
}

// ObjectsEqualOnlyStatus returns true if the objects status fields are equal.
func (e Equalities) ObjectsEqualOnlyStatus(a1, a2 client.Object) bool {
	return e.DeepEqual(
		statusFieldOnly(reflect.ValueOf(a1)).Interface(),
		statusFieldOnly(reflect.ValueOf(a2)).Interface(),
	)
}

// Copy shallow clones the Equalities object
func (e Equalities) Copy() Equalities {
	return Equalities{
		Equalities: e.Equalities.Copy(),
	}
}

func ObjectsEqual[T client.Object](equality Equalities, obj1, obj2 T) bool {
	return equality.DeepEqual(obj1, obj2)
}

func ObjectsEqualWithoutStatus[T client.Object](equality Equalities, obj1, obj2 T) bool {
	value1, value2 := withoutStatusField(reflect.ValueOf(obj1)), withoutStatusField(reflect.ValueOf(obj2))
	return equality.DeepEqual(value1.Interface(), value2.Interface())
}

func ObjectsStatusEqual[T client.Object](equality Equalities, obj1, obj2 T) bool {
	value1, value2 := statusFieldOnly(reflect.ValueOf(obj1)), statusFieldOnly(reflect.ValueOf(obj2))
	return equality.DeepEqual(value1.Interface(), value2.Interface())
}

func statusFieldOnly(value reflect.Value) reflect.Value {
	// Handle unstructured typ
	if value.Type() == reflect.TypeOf(&unstructured.Unstructured{}) {
		return reflect.ValueOf(&unstructured.Unstructured{
			Object: map[string]any{
				"status": value.Elem().FieldByName("Object").MapIndex(reflect.ValueOf("status")).Interface(),
			},
		})
	}

	// If the value is a struct, copy the status field into a new object
	if value.Kind() == reflect.Struct {
		newValue := reflect.New(value.Type()).Elem()
		newValue.FieldByName("Status").Set(value.FieldByName("Status"))
		return newValue

	}

	// If the value is a pointer, call again using the value
	if value.Kind() == reflect.Pointer {
		return statusFieldOnly(value.Elem())
	}

	// Return original object
	return value
}

func withoutStatusField(value reflect.Value) reflect.Value {
	// Handle unstructured typ
	if value.Type() == reflect.TypeOf(&unstructured.Unstructured{}) {
		// Create a new unstructured object
		newObj := &unstructured.Unstructured{Object: make(map[string]any)}

		// Copy the fields across, omitting status
		iter := value.Elem().FieldByName("Object").MapRange()
		for iter.Next() {
			k, v := iter.Key(), iter.Value()
			if keyStr := k.String(); keyStr != "status" {
				newObj.Object[keyStr] = v.Interface()
			}
		}

		// Return the value of the new object
		return reflect.ValueOf(newObj)
	}

	// If the value is a pointer, call again using the value
	if value.Kind() == reflect.Pointer {
		return withoutStatusField(value.Elem())
	}

	// If the value is not a struct, return the original value
	if value.Kind() != reflect.Struct {
		return value
	}

	// Get the type of the struct
	typ := value.Type()

	// Create a new instance of the struct
	newValue := reflect.New(typ).Elem()

	// Loop over fields
	for i := 0; i < typ.NumField(); i++ {
		// Get field at index
		field := typ.Field(i)

		// If the field is not a status field, copy it across
		if fieldValue := newValue.Field(i); field.Name != "Status" && newValue.CanSet() {
			fieldValue.Set(
				value.Field(i),
			)
		}
	}

	// Return the new struct
	return newValue
}
