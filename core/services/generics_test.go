// Package services_test provides comprehensive tests for generic ServiceContainer functions.
package services_test

import (
	"errors"
	"testing"

	"github.com/wangling-miao/aroute/core/services"
)

// TestGenericProvide tests the Provide[T] generic function
func TestGenericProvide_Get(t *testing.T) {
	c := services.NewContainer()

	// Test Provide[T]
	err := services.Provide(c, func() (*MockDatabase, error) {
		return &MockDatabase{Name: "test-db"}, nil
	})
	if err != nil {
		t.Fatalf("Provide[T] failed: %v", err)
	}

	// Test Get[T]
	db, err := services.Get[*MockDatabase](c)
	if err != nil {
		t.Fatalf("Get[T] failed: %v", err)
	}
	if db == nil {
		t.Fatal("Get[T] returned nil")
	}
	if db.Name != "test-db" {
		t.Errorf("expected Name=%q, got %q", "test-db", db.Name)
	}

	// Test singleton behavior (same instance on second Get)
	db2, err := services.Get[*MockDatabase](c)
	if err != nil {
		t.Fatalf("second Get[T] failed: %v", err)
	}
	if db != db2 {
		t.Error("Get[T] should return same instance (singleton)")
	}
}

// TestGenericProvide_NilProvider tests Provide[T] with nil provider
func TestGenericProvide_NilProvider(t *testing.T) {
	c := services.NewContainer()

	err := services.Provide[*MockDatabase](c, nil)
	if err == nil {
		t.Error("Provide[T](nil) should return error")
	}
}

// TestGenericProvide_Override tests that re-registration replaces previous provider
func TestGenericProvide_Override(t *testing.T) {
	c := services.NewContainer()

	// First registration
	err := services.Provide(c, func() (*MockDatabase, error) {
		return &MockDatabase{Name: "db1"}, nil
	})
	if err != nil {
		t.Fatalf("first Provide failed: %v", err)
	}

	// Get first instance
	db1, err := services.Get[*MockDatabase](c)
	if err != nil {
		t.Fatalf("first Get failed: %v", err)
	}
	if db1.Name != "db1" {
		t.Errorf("expected Name=%q, got %q", "db1", db1.Name)
	}

	// Override registration - should succeed (spec requires override support)
	err = services.Provide(c, func() (*MockDatabase, error) {
		return &MockDatabase{Name: "db2"}, nil
	})
	if err != nil {
		t.Fatalf("override Provide should succeed: %v", err)
	}

	// Get should return new instance
	db2, err := services.Get[*MockDatabase](c)
	if err != nil {
		t.Fatalf("second Get failed: %v", err)
	}
	if db2.Name != "db2" {
		t.Errorf("expected Name=%q after override, got %q", "db2", db2.Name)
	}

	// Should be different instance (cached instance should have been discarded)
	if db1 == db2 {
		t.Error("override should discard cached instance")
	}
}

// TestGenericGet_NotFound tests Get[T] for unregistered type
func TestGenericGet_NotFound(t *testing.T) {
	c := services.NewContainer()

	_, err := services.Get[*MockDatabase](c)
	if err == nil {
		t.Error("Get[T] should fail for unregistered type")
	}
}

// TestGenericGet_ProviderError tests Get[T] when provider returns error
func TestGenericGet_ProviderError(t *testing.T) {
	c := services.NewContainer()

	err := services.Provide(c, func() (*MockDatabase, error) {
		return nil, errors.New("provider failed")
	})
	if err != nil {
		t.Fatalf("Provide failed: %v", err)
	}

	_, err = services.Get[*MockDatabase](c)
	if err == nil {
		t.Error("Get[T] should fail when provider returns error")
	}
}

// TestGenericMustGet tests MustGet[T]
func TestGenericMustGet(t *testing.T) {
	c := services.NewContainer()

	err := services.Provide(c, func() (*MockDatabase, error) {
		return &MockDatabase{Name: "test-db"}, nil
	})
	if err != nil {
		t.Fatalf("Provide failed: %v", err)
	}

	// Should not panic
	db := services.MustGet[*MockDatabase](c)
	if db == nil {
		t.Fatal("MustGet[T] returned nil")
	}
	if db.Name != "test-db" {
		t.Errorf("expected Name=%q, got %q", "test-db", db.Name)
	}
}

// TestGenericMustGet_Panic tests MustGet[T] panics on error
func TestGenericMustGet_Panic(t *testing.T) {
	c := services.NewContainer()

	defer func() {
		if r := recover(); r == nil {
			t.Error("MustGet[T] should panic for unregistered type")
		}
	}()

	services.MustGet[*MockDatabase](c)
	t.Error("MustGet[T] should have panicked")
}

// TestGenericProvideNamed_GetNamed tests named service registration
func TestGenericProvideNamed_GetNamed(t *testing.T) {
	c := services.NewContainer()

	// Register two instances of same type with different names
	err := services.ProvideNamed(c, "primary", func() (*MockDatabase, error) {
		return &MockDatabase{Name: "primary-db"}, nil
	})
	if err != nil {
		t.Fatalf("ProvideNamed failed: %v", err)
	}

	err = services.ProvideNamed(c, "secondary", func() (*MockDatabase, error) {
		return &MockDatabase{Name: "secondary-db"}, nil
	})
	if err != nil {
		t.Fatalf("ProvideNamed failed: %v", err)
	}

	// Get primary
	primary, err := services.GetNamed[*MockDatabase](c, "primary")
	if err != nil {
		t.Fatalf("GetNamed failed: %v", err)
	}
	if primary.Name != "primary-db" {
		t.Errorf("expected Name=%q, got %q", "primary-db", primary.Name)
	}

	// Get secondary
	secondary, err := services.GetNamed[*MockDatabase](c, "secondary")
	if err != nil {
		t.Fatalf("GetNamed failed: %v", err)
	}
	if secondary.Name != "secondary-db" {
		t.Errorf("expected Name=%q, got %q", "secondary-db", secondary.Name)
	}

	// They should be different instances
	if primary == secondary {
		t.Error("named instances should be different")
	}
}

// TestGenericProvideNamed_EmptyName tests empty name validation
func TestGenericProvideNamed_EmptyName(t *testing.T) {
	c := services.NewContainer()

	err := services.ProvideNamed[*MockDatabase](c, "", func() (*MockDatabase, error) {
		return nil, nil
	})
	if err == nil {
		t.Error("ProvideNamed with empty name should fail")
	}
}

// TestGenericProvideNamed_NilProvider tests nil provider validation
func TestGenericProvideNamed_NilProvider(t *testing.T) {
	c := services.NewContainer()

	err := services.ProvideNamed[*MockDatabase](c, "test", nil)
	if err == nil {
		t.Error("ProvideNamed with nil provider should fail")
	}
}

// TestGenericProvideNamed_Override tests named service override
func TestGenericProvideNamed_Override(t *testing.T) {
	c := services.NewContainer()

	// First registration
	err := services.ProvideNamed(c, "primary", func() (*MockDatabase, error) {
		return &MockDatabase{Name: "db1"}, nil
	})
	if err != nil {
		t.Fatalf("first ProvideNamed failed: %v", err)
	}

	// Get first instance
	db1, err := services.GetNamed[*MockDatabase](c, "primary")
	if err != nil {
		t.Fatalf("first GetNamed failed: %v", err)
	}

	// Override - should succeed
	err = services.ProvideNamed(c, "primary", func() (*MockDatabase, error) {
		return &MockDatabase{Name: "db2"}, nil
	})
	if err != nil {
		t.Fatalf("override ProvideNamed should succeed: %v", err)
	}

	// Get should return new instance
	db2, err := services.GetNamed[*MockDatabase](c, "primary")
	if err != nil {
		t.Fatalf("second GetNamed failed: %v", err)
	}
	if db2.Name != "db2" {
		t.Errorf("expected Name=%q after override, got %q", "db2", db2.Name)
	}

	// Should be different instance
	if db1 == db2 {
		t.Error("override should discard cached instance")
	}
}

// TestGenericGetNamed_NotFound tests GetNamed for unregistered name
func TestGenericGetNamed_NotFound(t *testing.T) {
	c := services.NewContainer()

	_, err := services.GetNamed[*MockDatabase](c, "nonexistent")
	if err == nil {
		t.Error("GetNamed[T] should fail for unregistered name")
	}
}

// TestGenericRemove tests Remove[T]
func TestGenericRemove(t *testing.T) {
	c := services.NewContainer()

	// Register service
	err := services.Provide(c, func() (*MockDatabase, error) {
		return &MockDatabase{Name: "test-db"}, nil
	})
	if err != nil {
		t.Fatalf("Provide failed: %v", err)
	}

	// Verify it's registered
	if !services.Has[*MockDatabase](c) {
		t.Error("Has[T] should return true after registration")
	}

	// Get instance to cache it
	_, _ = services.Get[*MockDatabase](c)

	// Remove
	services.Remove[*MockDatabase](c)

	// Verify it's gone
	if services.Has[*MockDatabase](c) {
		t.Error("Has[T] should return false after Remove")
	}

	// Get should fail
	_, err = services.Get[*MockDatabase](c)
	if err == nil {
		t.Error("Get[T] should fail after Remove")
	}
}

// TestGenericRemove_WithNamedServices tests Remove[T] also removes named services
func TestGenericRemove_WithNamedServices(t *testing.T) {
	c := services.NewContainer()

	// Register named services
	err := services.ProvideNamed(c, "primary", func() (*MockDatabase, error) {
		return &MockDatabase{Name: "primary"}, nil
	})
	if err != nil {
		t.Fatalf("ProvideNamed failed: %v", err)
	}

	// Get the named service
	_, _ = services.GetNamed[*MockDatabase](c, "primary")

	// Remove should remove all named instances for this type
	services.Remove[*MockDatabase](c)

	// Named service should also be gone
	_, err = services.GetNamed[*MockDatabase](c, "primary")
	if err == nil {
		t.Error("GetNamed[T] should fail after Remove[T]")
	}
}

// TestGenericRemoveNamed tests RemoveNamed[T]
func TestGenericRemoveNamed(t *testing.T) {
	c := services.NewContainer()

	// Register named services
	err := services.ProvideNamed(c, "primary", func() (*MockDatabase, error) {
		return &MockDatabase{Name: "primary"}, nil
	})
	if err != nil {
		t.Fatalf("ProvideNamed failed: %v", err)
	}

	err = services.ProvideNamed(c, "secondary", func() (*MockDatabase, error) {
		return &MockDatabase{Name: "secondary"}, nil
	})
	if err != nil {
		t.Fatalf("ProvideNamed failed: %v", err)
	}

	// Remove only primary
	services.RemoveNamed[*MockDatabase](c, "primary")

	// Primary should be gone
	_, err = services.GetNamed[*MockDatabase](c, "primary")
	if err == nil {
		t.Error("GetNamed[T] should fail for removed name")
	}

	// Secondary should still exist
	_, err = services.GetNamed[*MockDatabase](c, "secondary")
	if err != nil {
		t.Error("GetNamed[T] should succeed for other name")
	}
}

// TestGenericHas tests Has[T]
func TestGenericHas(t *testing.T) {
	c := services.NewContainer()

	// Not registered yet
	if services.Has[*MockDatabase](c) {
		t.Error("Has[T] should return false for unregistered type")
	}

	// Register
	err := services.Provide(c, func() (*MockDatabase, error) {
		return &MockDatabase{Name: "test"}, nil
	})
	if err != nil {
		t.Fatalf("Provide failed: %v", err)
	}

	// Now registered
	if !services.Has[*MockDatabase](c) {
		t.Error("Has[T] should return true after registration")
	}
}

// TestGenericConcurrentAccess tests concurrent access with generic functions
func TestGenericConcurrentAccess(t *testing.T) {
	c := services.NewContainer()

	// Register service
	err := services.Provide(c, func() (*MockDatabase, error) {
		return &MockDatabase{Name: "concurrent-test"}, nil
	})
	if err != nil {
		t.Fatalf("Provide failed: %v", err)
	}

	// Concurrent Get operations
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func() {
			defer func() { done <- true }()
			db, err := services.Get[*MockDatabase](c)
			if err != nil {
				t.Errorf("concurrent Get failed: %v", err)
				return
			}
			if db == nil {
				t.Error("concurrent Get returned nil")
			}
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	// All should have gotten the same instance
	if c.InstanceCount() != 1 {
		t.Errorf("expected 1 cached instance, got %d", c.InstanceCount())
	}
}

// TestGenericMultipleTypes tests multiple service types
func TestGenericMultipleTypes(t *testing.T) {
	c := services.NewContainer()

	// Register different types
	err := services.Provide(c, func() (*MockDatabase, error) {
		return &MockDatabase{Name: "db"}, nil
	})
	if err != nil {
		t.Fatalf("Provide MockDatabase failed: %v", err)
	}

	err = services.Provide(c, func() (*MockLogger, error) {
		return &MockLogger{Level: "info"}, nil
	})
	if err != nil {
		t.Fatalf("Provide MockLogger failed: %v", err)
	}

	// Get both
	db, err := services.Get[*MockDatabase](c)
	if err != nil {
		t.Fatalf("Get MockDatabase failed: %v", err)
	}

	logger, err := services.Get[*MockLogger](c)
	if err != nil {
		t.Fatalf("Get MockLogger failed: %v", err)
	}

	if db.Name != "db" {
		t.Errorf("expected db.Name=%q, got %q", "db", db.Name)
	}
	if logger.Level != "info" {
		t.Errorf("expected logger.Level=%q, got %q", "info", logger.Level)
	}

	// Verify both have instances
	if c.InstanceCount() != 2 {
		t.Errorf("expected 2 instances, got %d", c.InstanceCount())
	}
}

// TestGenericInterfaceType tests registering and retrieving interface types
func TestGenericInterfaceType(t *testing.T) {
	c := services.NewContainer()

	// Register concrete type
	err := services.Provide(c, func() (*mockPostgreSQL, error) {
		return &mockPostgreSQL{ConnectionString: "postgres://localhost"}, nil
	})
	if err != nil {
		t.Fatalf("Provide failed: %v", err)
	}

	// Get by concrete type (interface not implemented in this test)
	pg, err := services.Get[*mockPostgreSQL](c)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if pg.ConnectionString != "postgres://localhost" {
		t.Errorf("expected ConnectionString=%q, got %q", "postgres://localhost", pg.ConnectionString)
	}
}
