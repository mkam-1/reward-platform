package domain_test

import (
	"errors"
	"testing"

	"reward-platform/pkg/domain"
)

func TestStoreGetPutList(t *testing.T) {
	s := domain.NewStore[string]()

	if _, err := s.Get("missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Get(missing) error = %v, want ErrNotFound", err)
	}

	s.Put("a", "alpha")
	s.Put("b", "beta")

	got, err := s.Get("a")
	if err != nil {
		t.Fatalf("Get(a): unexpected error: %v", err)
	}
	if got != "alpha" {
		t.Fatalf("Get(a) = %q, want %q", got, "alpha")
	}

	list := s.List()
	if len(list) != 2 {
		t.Fatalf("List() len = %d, want 2", len(list))
	}
}
