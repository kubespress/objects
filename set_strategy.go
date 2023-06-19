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

package objects

import (
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SetStrategy is used by the set client to name the objects it creates. It
// is also used to determine which objects should be deleted as they do no
// conform to the strategy.
type SetStrategy interface {
	// DesiredReplicas is the number of desired replicas.
	DesiredReplicas() int
	// Less is used to sort the objects using the naming strategy. This will be
	// called before ShouldBeDeleted in order to allow the strategy to sort the
	// objects before deciding what should be deleted
	Less(i, j client.Object) bool
	// ShouldBeDeleted is called against each object that exists to determine
	// if the object should be deleted due to the nameing strategy or scale
	// down. The slice passed into this function will have been sorted by the
	// provided Sort method.
	ShouldBeDeleted(objects ObjectSliceAccessor, idx int) bool
	// SetName is used to set the name of the object, the existing objects are
	// also passed in. This function may be called multiple times per object
	// so must be idempotent, random names should be generated using
	// the GenerateName field in the Kubernetes object.
	SetName(objects ObjectSliceAccessor, current client.Object) error
}

// ObjectSliceAccessor is used to access items in a slice of client.Objects.
type ObjectSliceAccessor interface {
	Get(int) client.Object
	Len() int
}

type objectSliceAccessor[T client.Object] []T

func (o objectSliceAccessor[T]) Get(i int) client.Object {
	return o[i]
}

func (o objectSliceAccessor[T]) Len() int {
	return len(o)
}

type indexedSetStrategy struct {
	prefix   string
	names    sets.Set[string]
	replicas int
}

// IndexedNamingStrategy names objects in the format "prefix-<index>" where
// index is a zero indexed, incrementing numeric value. For example, this could
// generate the names example-0, example-1, etc. Using this strategy the highest
// index is deleted first during scale-down.
func IndexedSetStrategy(prefix string, replicas int) SetStrategy {
	// Return the strategy
	return &indexedSetStrategy{
		prefix:   prefix,
		replicas: replicas,
	}
}

func (s indexedSetStrategy) DesiredReplicas() int {
	return s.replicas
}

func (s indexedSetStrategy) Less(i, j client.Object) bool {
	// Get the names
	iName := i.GetName()
	jName := j.GetName()

	// Add the dash to the prefix if required
	prefix := s.prefix
	if !strings.HasSuffix(prefix, "-") {
		prefix += "-"
	}

	// Determine if the strings have the correct prefix
	iHasPrefix := strings.HasPrefix(iName, prefix)
	jHasPrefix := strings.HasPrefix(jName, prefix)

	// If the objects do not have the correct prefix, they should be pushed
	// to the end of the slice
	switch {
	case iHasPrefix && !jHasPrefix:
		return true
	case !iHasPrefix && jHasPrefix:
		return false
	case !iHasPrefix && !jHasPrefix:
		// Sort all pods without the prefix by creation timestamp as a
		// fallback
		return i.GetCreationTimestamp().After(j.GetCreationTimestamp().Time)
	}

	// Remove the prefix
	iName = strings.TrimPrefix(iName, prefix)
	jName = strings.TrimPrefix(jName, prefix)

	// Get the index from the name, if the name is invalid, push the object
	// to the right
	iValidOrd := true
	iOrd, err := strconv.Atoi(iName)
	if err != nil || iOrd < 0 {
		iValidOrd = false
	}

	// Get the index from the name, if the name is invalid, push the object
	// to the right
	jValidOrd := true
	jOrd, err := strconv.Atoi(jName)
	if err != nil || jOrd < 0 {
		jValidOrd = false
	}

	// If the objects do not have the correct prefix, they should be pushed
	// to the end of the slice
	switch {
	case iValidOrd && !jValidOrd:
		return true
	case !iValidOrd && jValidOrd:
		return false
	case !iValidOrd && !jValidOrd:
		// Sort all pods without the a valid name by the creation timestamp
		// as a fallback
		return i.GetCreationTimestamp().After(j.GetCreationTimestamp().Time)
	}

	// Compare the indexes
	return iOrd < jOrd
}

func (s *indexedSetStrategy) ShouldBeDeleted(objects ObjectSliceAccessor, idx int) bool {
	// Build up list of "desired" names
	if s.names.Len() != s.replicas {
		s.names = make(sets.Set[string], s.replicas)
		for i := 0; i < s.replicas; i++ {
			s.names.Insert(s.generateName(i))
		}
	}

	// Return if the object should be deleted
	return !s.names.Has(objects.Get(idx).GetName())
}

func (s indexedSetStrategy) SetName(objects ObjectSliceAccessor, obj client.Object) error {
	// Get current objects into a set of names
	current := make(sets.Set[string], objects.Len())
	for i := 0; i < objects.Len(); i++ {
		current.Insert(objects.Get(i).GetName())
	}

	// Loop over possible names, setting the first one that is not in use
	for i := 0; true; i++ {
		name := s.generateName(i)
		if current.Has(name) {
			continue
		}

		obj.SetName(name)
		return nil
	}

	// Since the above loop is infinite, this is unreachable
	return nil
}

func (s *indexedSetStrategy) generateName(i int) string {
	// Add the dash to the prefix if required
	prefix := s.prefix
	if !strings.HasSuffix(prefix, "-") {
		prefix += "-"
	}

	// Return the name
	return fmt.Sprintf("%s%d", prefix, i)
}

type generatedNameSetStrategy struct {
	prefix   string
	replicas int
}

// GeneratedNameSetStrategy uses the `metadata.generateName` field to set the
// name of an object. This gives each object a unique name in the format
// "prefix-<hash>". For example, this could generate the names example-d42cf,
// example-ce42r, etc. Using this strategy the oldest object is deleted first
// during scale-down.
func GeneratedNameSetStrategy(prefix string, replicas int) SetStrategy {
	// Return the strategy
	return generatedNameSetStrategy{
		prefix:   prefix,
		replicas: replicas,
	}
}

func (s generatedNameSetStrategy) DesiredReplicas() int {
	return s.replicas
}

func (s generatedNameSetStrategy) Less(i, j client.Object) bool {
	// Get the names
	iName := i.GetName()
	jName := j.GetName()

	// Add the dash to the prefix if required
	prefix := s.prefix
	if !strings.HasSuffix(prefix, "-") {
		prefix += "-"
	}

	// Determine if the strings have the correct prefix
	iHasPrefix := strings.HasPrefix(iName, prefix)
	jHasPrefix := strings.HasPrefix(jName, prefix)

	// If the objects do not have the correct prefix, they should be pushed
	// to the end of the slice
	switch {
	case iHasPrefix && !jHasPrefix:
		return true
	case !iHasPrefix && jHasPrefix:
		return false
	}

	// Since all the names are random, we sort by creation timestamp. This
	// should mean the oldest are deleted first
	return i.GetCreationTimestamp().After(j.GetCreationTimestamp().Time)
}

func (s generatedNameSetStrategy) ShouldBeDeleted(objects ObjectSliceAccessor, idx int) bool {
	// Add the dash to the prefix if required
	prefix := s.prefix
	if !strings.HasSuffix(prefix, "-") {
		prefix += "-"
	}

	// If the object has the incorrect prefix, it should be deleted
	if !strings.HasPrefix(objects.Get(idx).GetName(), prefix) {
		return true
	}

	return idx >= s.replicas
}

func (s generatedNameSetStrategy) SetName(objects ObjectSliceAccessor, obj client.Object) error {
	// Ensure prefix has hyphen separation
	if strings.HasSuffix(s.prefix, "-") {
		obj.SetGenerateName(s.prefix)
	} else {
		obj.SetGenerateName(s.prefix + "-")
	}

	// Return no error
	return nil
}

type fixedNamingStrategy struct {
	prefix string
	names  []string

	lookup sets.Set[string]
}

// FixedNameSetStrategy returns a strategy that defines a fixed number of
// pre-defined names. If a prefix is specified then the names will also be
// prefixed by this value, with a hyphen separator. Using this strategy the
// oldest object is deleted first during scale-down.
func FixedNameSetStrategy(prefix string, names []string) SetStrategy {
	// Return the strategy
	return &fixedNamingStrategy{
		prefix: prefix,
		names:  names,
	}
}

func (s *fixedNamingStrategy) DesiredReplicas() int {
	return len(s.names)
}

func (s *fixedNamingStrategy) Less(i, j client.Object) bool {
	// Build up list of "desired" names
	if s.lookup.Len() != len(s.names) {
		s.lookup = make(sets.Set[string], len(s.names))
		for _, suffix := range s.names {
			s.lookup.Insert(s.generateName(suffix))
		}
	}

	// Get the names
	iName := i.GetName()
	jName := j.GetName()

	// If prefix is defined, push objects with an invalid prefix to the end
	if s.prefix != "" {
		// Add the dash to the prefix if required
		prefix := s.prefix
		if !strings.HasSuffix(prefix, "-") {
			prefix += "-"
		}

		// Determine if the strings have the correct prefix
		iHasPrefix := strings.HasPrefix(iName, prefix)
		jHasPrefix := strings.HasPrefix(jName, prefix)

		// If the objects do not have the correct prefix, they should be pushed
		// to the end of the slice
		switch {
		case iHasPrefix && !jHasPrefix:
			return true
		case !iHasPrefix && jHasPrefix:
			return false
		case !iHasPrefix && !jHasPrefix:
			// Sort all pods without the prefix by creation timestamp as a
			// fallback
			return i.GetCreationTimestamp().After(j.GetCreationTimestamp().Time)
		}
	}

	// Determine if the names are valid
	iValid := s.lookup.Has(iName)
	jValid := s.lookup.Has(jName)

	// If the objects do not have a valid name, they should be pushed to the end
	// end of the slice
	switch {
	case iValid && !jValid:
		return true
	case !iValid && jValid:
		return false
	}

	// Sort by timestamp, order is not important with this strategy
	return i.GetCreationTimestamp().After(j.GetCreationTimestamp().Time)
}

func (s *fixedNamingStrategy) ShouldBeDeleted(objects ObjectSliceAccessor, idx int) bool {
	// Build up list of "desired" names
	if s.lookup.Len() != len(s.names) {
		s.lookup = make(sets.Set[string], len(s.names))
		for _, suffix := range s.names {
			s.lookup.Insert(s.generateName(suffix))
		}
	}

	// Return if the object should be deleted
	return !s.lookup.Has(objects.Get(idx).GetName()) || idx >= len(s.names)
}

func (s fixedNamingStrategy) SetName(objects ObjectSliceAccessor, obj client.Object) error {
	// Get current objects into a set of names
	current := make(sets.Set[string], objects.Len())
	for i := 0; i < objects.Len(); i++ {
		current.Insert(objects.Get(i).GetName())
	}

	// Loop over possible names, setting the first one that is not in use
	for _, suffix := range s.names {
		name := s.generateName(suffix)
		if current.Has(name) {
			continue
		}

		obj.SetName(name)
		return nil
	}

	// Name not set, all names have been used
	return fmt.Errorf("no names available in naming strategy")
}

func (s fixedNamingStrategy) generateName(suffix string) string {
	// If no prefix is specified, don't use one
	if s.prefix == "" {
		return suffix
	}

	// If the prefix ends with a dash, then just append the suffix
	if strings.HasSuffix(s.prefix, "-") {
		return s.prefix + suffix
	}

	// Join the prefix and suffix with a hyphen
	return fmt.Sprintf("%s-%s", s.prefix, suffix)
}

// ForEachSetStrategyLookup is a lookup returned by the ForEachSetStrategy
// function, it allows the source object to easily be looked up.
type ForEachSetStrategyLookup[T client.Object] map[string]T

// Source returns the source object when using the ForEachSetStrategy
func (f ForEachSetStrategyLookup[T]) Source(o client.Object) (T, bool) {
	source, ok := f[o.GetName()]
	return source, ok
}

// ForEachSetStrategy is a convenience method to generate a strategy that
// produces an object for each input object. It also returns a lookup that can
// be used to obtain the source object from the name.
func ForEachSetStrategy[T client.Object](prefix string, objects []T) (SetStrategy, ForEachSetStrategyLookup[T]) {
	// Store names
	names := make([]string, len(objects))
	lookup := make(ForEachSetStrategyLookup[T])

	for i, object := range objects {
		// Generate a short hash based on the owner namespace/name
		name := shortHash(object.GetNamespace() + "/" + object.GetName())

		// Generate full name
		fullName := name
		if prefix != "" {
			fullName = prefix + "-" + name
		}

		// Store name and lookup
		names[i] = name
		lookup[fullName] = object
	}

	// Return the strategy and the lookup
	return FixedNameSetStrategy(prefix, names), lookup
}

func shortHash(obj interface{}) string {
	hf := fnv.New32()

	// Create spew config
	printer := spew.ConfigState{
		Indent:         " ",
		SortKeys:       true,
		DisableMethods: true,
		SpewKeys:       true,
	}

	// Spew into hasher
	printer.Fprintf(hf, "%#v", obj)

	return rand.SafeEncodeString(fmt.Sprint(hf.Sum32()))
}
