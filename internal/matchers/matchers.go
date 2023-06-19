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

package matchers

import (
	"context"
	"fmt"
	"reflect"

	"github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type objectExistsMatcher struct {
	client client.Client
}

func Exists(client client.Client) gomega.OmegaMatcher {
	return &objectExistsMatcher{
		client: client,
	}
}

func (matcher *objectExistsMatcher) Match(actual interface{}) (success bool, err error) {
	// Check object is the correct type
	obj, isCorrectType := actual.(client.Object)
	if !isCorrectType {
		return false, fmt.Errorf("Exists matcher expects a client.Object")
	}

	// Check the object exists
	key := client.ObjectKeyFromObject(obj)
	err = matcher.client.Get(context.Background(), key, obj.DeepCopyObject().(client.Object))

	// Object exists
	if err == nil {
		return true, nil
	}

	// Object does not exist
	if apierrors.IsNotFound(err) {
		return false, nil
	}

	// Different error
	return false, err
}

func (matcher *objectExistsMatcher) FailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected\n\t%s\nto exist within the cluster", actual)
}

func (matcher *objectExistsMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected\n\t%s\nnot to exist within the cluster", actual)
}

type objNotFoundError struct{}

func BeNotFoundError() gomega.OmegaMatcher {
	return &objNotFoundError{}
}

func (matcher *objNotFoundError) Match(actual interface{}) (success bool, err error) {
	// Check object is the correct type
	errToMatch, isCorrectType := actual.(error)
	if !isCorrectType {
		return false, fmt.Errorf("BeNotFoundError matcher expects an error")
	}

	// Check if the error is not found
	if apierrors.IsNotFound(errToMatch) {
		return true, nil
	}

	// Error is not "not found"
	return false, nil
}

func (matcher *objNotFoundError) FailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected\n\t%s\nto be a not found error", actual)
}

func (matcher *objNotFoundError) NegatedFailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected\n\t%s\nnot to be a not found error", actual)
}

type objAlreadyExistsError struct{}

func BeAlreadyExistsError() gomega.OmegaMatcher {
	return &objAlreadyExistsError{}
}

func (matcher *objAlreadyExistsError) Match(actual interface{}) (success bool, err error) {
	// Check object is the correct type
	errToMatch, isCorrectType := actual.(error)
	if !isCorrectType {
		return false, fmt.Errorf("BeAlreadyExistsError matcher expects an error")
	}

	// Check if the error is not found
	if apierrors.IsAlreadyExists(errToMatch) {
		return true, nil
	}

	// Error is not "not found"
	return false, nil
}

func (matcher *objAlreadyExistsError) FailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected\n\t%s\nto be a already exists error", actual)
}

func (matcher *objAlreadyExistsError) NegatedFailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected\n\t%s\nnot to be a already exists error", actual)
}

type objMatcher struct {
	client  client.Client
	matcher gomega.OmegaMatcher
}

func Match(client client.Client, matcher gomega.OmegaMatcher) gomega.OmegaMatcher {
	return &objMatcher{
		client:  client,
		matcher: matcher,
	}
}

func (matcher *objMatcher) getObject(actual interface{}) (client.Object, error) {
	// Check object is the correct type
	obj, isCorrectType := actual.(client.Object)
	if !isCorrectType {
		return nil, fmt.Errorf("Exists matcher expects a client.Object")
	}

	// Check the object exists
	key := client.ObjectKeyFromObject(obj)
	typ := reflect.TypeOf(actual)
	newObj := reflect.New(typ.Elem()).Interface().(client.Object)
	err := matcher.client.Get(context.Background(), key, newObj)
	if err != nil {
		return nil, err
	}

	// Return the object
	return newObj, nil
}

func (matcher *objMatcher) Match(actual interface{}) (success bool, err error) {
	// Get the object
	obj, err := matcher.getObject(actual)
	if err != nil {
		return false, err
	}

	// Call the child matcher
	return matcher.matcher.Match(obj)
}

func (matcher *objMatcher) FailureMessage(actual interface{}) (message string) {
	obj, _ := matcher.getObject(actual)
	return matcher.matcher.FailureMessage(obj)
}

func (matcher *objMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	obj, _ := matcher.getObject(actual)
	return matcher.matcher.NegatedFailureMessage(obj)
}
