package live

import "testing"

func TestNewWithoutConfig(t *testing.T) {
	r := New("", "", "", "")
	if r != nil {
		t.Fatalf("New() = %v; want nil", r)
	}
	// A nil Rooms writes nothing (Firebase not configured).
	if err := r.Update(t.Context(), "g1", Room{Status: "waiting", Members: 1}); err != nil {
		t.Fatal(err)
	}
}
