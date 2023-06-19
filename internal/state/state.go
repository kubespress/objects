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

package state

type State uint8

func (s State) Check(condition Condition) bool {
	return (s & condition.mask) == (condition.value & condition.mask)
}

func (s *State) Set(condition Condition) {
	*s = (*s & ^condition.mask) | (condition.value & condition.mask)
}

type Condition struct {
	mask  State
	value State
}

var (
	ClientUninitialized       = Condition{mask: 0b00000001, value: 0b00000000}
	ClientInitialized         = Condition{mask: 0b00000001, value: 0b00000001}
	ObjectExistenceUnknown    = Condition{mask: 0b00000110, value: 0b00000000}
	ObjectExists              = Condition{mask: 0b00000110, value: 0b00000110}
	ObjectDoesNotExist        = Condition{mask: 0b00000110, value: 0b00000010}
	ObjectNotPendingDeletion  = Condition{mask: 0b00001000, value: 0b00000000}
	ObjectPendingDeletion     = Condition{mask: 0b00001000, value: 0b00001000}
	ObjectUpdateStatusUnknown = Condition{mask: 0b00110000, value: 0b00000000}
	ObjectRequiresUpdate      = Condition{mask: 0b00110000, value: 0b00110000}
	ObjectUpToDate            = Condition{mask: 0b00110000, value: 0b00010000}
	SetActionCreate           = Condition{mask: 0b11000000, value: 0b01000000}
	SetActionKeep             = Condition{mask: 0b11000000, value: 0b11000000}
	SetActionDelete           = Condition{mask: 0b11000000, value: 0b10000000}
)

func Merge(conditions ...Condition) (merged Condition) {
	for _, condition := range conditions {
		merged.mask |= condition.mask
		merged.value |= condition.value
	}
	return
}
