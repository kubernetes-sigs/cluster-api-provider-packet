/*
Copyright 2021 The Kubernetes Authors.
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

package util

import (
	"bytes"
	"sync"
)

type ErrorCollection struct {
	mu       sync.Mutex
	contents map[string]error
}

func NewErrorCollection(size int) *ErrorCollection {
	return &ErrorCollection{ //nolint:exhaustivestruct
		contents: make(map[string]error, size),
	}
}

func (e *ErrorCollection) Store(key string, value error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.contents == nil {
		e.contents = make(map[string]error)
	}

	e.contents[key] = value
}

func (e *ErrorCollection) Load(key string) (error, bool) { //nolint:revive,golint,stylecheck
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.contents == nil {
		return nil, false
	}

	value, ok := e.contents[key]

	return value, ok
}

func (e *ErrorCollection) Get(key string) error {
	value, _ := e.Load(key)

	return value
}

type OutputBuffers struct {
	mu       sync.Mutex
	contents map[string]*bytes.Buffer
}

func NewOutputBuffers(size int) *OutputBuffers {
	return &OutputBuffers{ //nolint:exhaustivestruct
		contents: make(map[string]*bytes.Buffer, size),
	}
}

func (o *OutputBuffers) Store(key string, value *bytes.Buffer) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.contents == nil {
		o.contents = make(map[string]*bytes.Buffer)
	}

	o.contents[key] = value
}

func (o *OutputBuffers) Load(key string) (*bytes.Buffer, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.contents == nil {
		return nil, false
	}

	value, ok := o.contents[key]

	return value, ok
}

func (o *OutputBuffers) Get(key string) *bytes.Buffer {
	value, _ := o.Load(key)

	return value
}

type OutputCollection struct {
	mu       sync.Mutex
	contents map[string]string
}

func NewOutputCollection(size int) *OutputCollection {
	return &OutputCollection{ //nolint:exhaustivestruct
		contents: make(map[string]string, size),
	}
}

func (o *OutputCollection) Append(key string, value string) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.contents == nil {
		o.contents = make(map[string]string)
	}

	o.contents[key] += value
}

func (o *OutputCollection) Store(key string, value string) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.contents == nil {
		o.contents = make(map[string]string)
	}

	o.contents[key] = value
}

func (o *OutputCollection) Load(key string) (string, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.contents == nil {
		return "", false
	}

	value, ok := o.contents[key]

	return value, ok
}

func (o *OutputCollection) Get(key string) string {
	value, _ := o.Load(key)

	return value
}
