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
	"context"

	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Hook is an interface that is called by the client when specific actions take place.
type Hook interface {
	PreCreate(ctx context.Context, obj client.Object)
	PostCreate(ctx context.Context, obj client.Object)
	CreateError(ctx context.Context, obj client.Object, err error)
	PreUpdate(ctx context.Context, old, new client.Object)
	PostUpdate(ctx context.Context, old, new client.Object)
	UpdateError(ctx context.Context, old, new client.Object, err error)
	PreDelete(ctx context.Context, obj client.Object)
	PostDelete(ctx context.Context, obj client.Object)
	DeleteError(ctx context.Context, obj client.Object, err error)
}

// hookSlice is a slice of hooks, it calls all hooks in the slice when invoked.
type hookSlice []Hook

func (h hookSlice) PreCreate(ctx context.Context, obj client.Object) {
	for _, hook := range h {
		hook.PreCreate(ctx, obj)
	}
}

func (h hookSlice) PostCreate(ctx context.Context, obj client.Object) {
	for _, hook := range h {
		hook.PostCreate(ctx, obj)
	}
}

func (h hookSlice) CreateError(ctx context.Context, obj client.Object, err error) {
	for _, hook := range h {
		hook.CreateError(ctx, obj, err)
	}
}

func (h hookSlice) PreUpdate(ctx context.Context, old, new client.Object) {
	for _, hook := range h {
		hook.PreUpdate(ctx, old, new)
	}
}

func (h hookSlice) PostUpdate(ctx context.Context, old, new client.Object) {
	for _, hook := range h {
		hook.PostUpdate(ctx, old, new)
	}
}

func (h hookSlice) UpdateError(ctx context.Context, old, new client.Object, err error) {
	for _, hook := range h {
		hook.UpdateError(ctx, old, new, err)
	}
}

func (h hookSlice) PreDelete(ctx context.Context, obj client.Object) {
	for _, hook := range h {
		hook.PreDelete(ctx, obj)
	}
}

func (h hookSlice) PostDelete(ctx context.Context, obj client.Object) {
	for _, hook := range h {
		hook.PostDelete(ctx, obj)
	}
}

func (h hookSlice) DeleteError(ctx context.Context, obj client.Object, err error) {
	for _, hook := range h {
		hook.DeleteError(ctx, obj, err)
	}
}

// NullHook is a hook that performs no action. It can be embedded in other hooks to ensure non implemented methods have
// a null implementation
type NullHook struct{}

// Compile time validation that NullHook implements Hook
var _ Hook = NullHook{}

func (NullHook) PreCreate(ctx context.Context, obj client.Object)                   {}
func (NullHook) PostCreate(ctx context.Context, obj client.Object)                  {}
func (NullHook) CreateError(ctx context.Context, obj client.Object, err error)      {}
func (NullHook) PreUpdate(ctx context.Context, old, new client.Object)              {}
func (NullHook) PostUpdate(ctx context.Context, old, new client.Object)             {}
func (NullHook) UpdateError(ctx context.Context, old, new client.Object, err error) {}
func (NullHook) PreDelete(ctx context.Context, obj client.Object)                   {}
func (NullHook) PostDelete(ctx context.Context, obj client.Object)                  {}
func (NullHook) DeleteError(ctx context.Context, obj client.Object, err error)      {}

type eventEmitterHook struct {
	NullHook
	Object   client.Object
	Recorder record.EventRecorder
}

// Compile time validation that eventEmitterHook implements Hook
var _ Hook = eventEmitterHook{}

// EventEmitterHook returns a hook that emits an event on the provided object when an object is created, updated and
// deleted.
func EventEmitterHook(recorder record.EventRecorder, object client.Object) Hook {
	return eventEmitterHook{
		Object:   object,
		Recorder: recorder,
	}
}

func (e eventEmitterHook) PostCreate(ctx context.Context, obj client.Object) {
	e.Recorder.Eventf(e.Object, "Normal", obj.GetObjectKind().GroupVersionKind().Kind+"Created", "created %q", obj.GetName())
}

func (e eventEmitterHook) CreateError(ctx context.Context, obj client.Object, err error) {
	e.Recorder.Eventf(e.Object, "Warning", "FailedToCreate"+obj.GetObjectKind().GroupVersionKind().Kind, "failed to create object: %s", err.Error())
}

func (e eventEmitterHook) PostUpdate(ctx context.Context, old, new client.Object) {
	e.Recorder.Eventf(e.Object, "Normal", old.GetObjectKind().GroupVersionKind().Kind+"Updated", "updated %q", old.GetName())
}

func (e eventEmitterHook) UpdateError(ctx context.Context, old, new client.Object, err error) {
	e.Recorder.Eventf(e.Object, "Warning", "FailedToUpdate"+old.GetObjectKind().GroupVersionKind().Kind, "failed to update object %q: %s", old.GetName(), err.Error())
}

func (e eventEmitterHook) PostDelete(ctx context.Context, obj client.Object) {
	e.Recorder.Eventf(e.Object, "Normal", obj.GetObjectKind().GroupVersionKind().Kind+"Deleted", "deleted %q", obj.GetName())
}

func (e eventEmitterHook) DeleteError(ctx context.Context, obj client.Object, err error) {
	e.Recorder.Eventf(e.Object, "Warning", "FailedToDelete"+obj.GetObjectKind().GroupVersionKind().Kind, "failed to delete object %q: %s", obj.GetName(), err.Error())
}
