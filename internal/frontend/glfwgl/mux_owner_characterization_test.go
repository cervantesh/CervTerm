//go:build glfw

package glfwgl

import (
	"reflect"
	"testing"

	termmux "cervterm/internal/mux"
)

// TestKnownDefect_L3_02_ProcessServicesExposeConcreteMux expires Slice 3.1.
func TestKnownDefect_L3_02_ProcessServicesExposeConcreteMux(t *testing.T) {
	field, ok := reflect.TypeOf(processServices{}).FieldByName("mux")
	if !ok {
		t.Fatal("process services mux field missing")
	}
	if got, want := field.Type, reflect.TypeOf((*termmux.Mux)(nil)); got != want {
		t.Fatalf("process services mux type=%v want known-defect %v", got, want)
	}
}
