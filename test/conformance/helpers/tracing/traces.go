/*
Copyright 2019 The Knative Authors

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

package tracing

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/openzipkin/zipkin-go/model"
)

// PrettyPrintTrace pretty prints a Trace.
func PrettyPrintTrace(trace []model.SpanModel) string {
	b, _ := json.Marshal(trace)
	return string(b)
}

// SpanTree is the tree of Spans representation of a Trace.
type SpanTree struct {
	Span     model.SpanModel
	Children []SpanTree
}

// TestSpanTree is the expected version of SpanTree used for assertions in testing.
type TestSpanTree struct {
	Root                     bool
	Kind                     model.Kind
	LocalEndpointServiceName string
	Tags                     map[string]string

	Children []TestSpanTree
}

// GetTraceTree converts a set slice of spans into a SpanTree.
func GetTraceTree(t *testing.T, trace []model.SpanModel) SpanTree {
	var roots []model.SpanModel
	parents := map[model.ID][]model.SpanModel{}
	for _, span := range trace {
		if span.ParentID != nil {
			parents[*span.ParentID] = append(parents[*span.ParentID], span)
		} else {
			roots = append(roots, span)
		}
	}

	children, err := getChildren(parents, roots)
	if err != nil {
		t.Fatalf("Could not create span tree for %v: %v", PrettyPrintTrace(trace), err)
	}

	tree := SpanTree{
		Children: children,
	}
	if len(parents) != 0 {
		t.Fatalf("Left over spans after generating the SpanTree: %v. Original: %v", parents, PrettyPrintTrace(trace))
	}
	return tree
}

func getChildren(parents map[model.ID][]model.SpanModel, current []model.SpanModel) ([]SpanTree, error) {
	var children []SpanTree
	for _, span := range current {
		grandchildren, err := getChildren(parents, parents[span.ID])
		if err != nil {
			return children, err
		}
		children = append(children, SpanTree{
			Span:     span,
			Children: grandchildren,
		})
		delete(parents, span.ID)
	}
	return children, nil
}

// SpanCount gets the count of spans in this tree.
func (t TestSpanTree) SpanCount() int {
	spans := 1
	if t.Root {
		// The root span is artificial. It exits solely so we can easily pass around the tree.
		spans = 0
	}
	for _, child := range t.Children {
		spans += child.SpanCount()
	}
	return spans
}

// Matches checks to see if this TestSpanTree matches an actual SpanTree. It is intended to be used
// for assertions while testing.
func (t TestSpanTree) Matches(actual SpanTree) error {
	return traceTreeMatches(".", t, actual)
}

func traceTreeMatches(pos string, want TestSpanTree, got SpanTree) error {
	if g, w := got.Span.Kind, want.Kind; g != w {
		return fmt.Errorf("unexpected kind at %q: got %q, want %q", pos, g, w)
	}
	gotLocalEndpointServiceName := ""
	if got.Span.LocalEndpoint != nil {
		gotLocalEndpointServiceName = got.Span.LocalEndpoint.ServiceName
	}
	if w := want.LocalEndpointServiceName; w != "" && gotLocalEndpointServiceName != w {
		return fmt.Errorf("unexpected localEndpoint.ServiceName at %q: got %q, want %q", pos, gotLocalEndpointServiceName, w)
	}
	for k, w := range want.Tags {
		if g := got.Span.Tags[k]; g != w {
			return fmt.Errorf("unexpected tag[%s] value at %q: got %q, want %q", k, pos, g, w)
		}
	}
	if g, w := len(got.Children), len(want.Children); g != w {
		return fmt.Errorf("unexpected number of children at %q: got %v, want %v", pos, g, w)
	}
	// TODO: Children are actually unordered, assert them in an unordered fashion.
	for i := range want.Children {
		if err := traceTreeMatches(fmt.Sprintf("%s%d.", pos, i), want.Children[i], got.Children[i]); err != nil {
			return err
		}
	}
	return nil
}
