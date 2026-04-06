// Package services_test provides comprehensive tests for ServiceContainer.
package services_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/wangling-miao/aroute/core/services"
)

// MockDatabase is a test service for testing Provide/Get
type MockDatabase struct {
	Name string
}

// MockLogger is another test service for testing multiple types
type MockLogger struct {
	Level string
}

// MockConfig is a test service that depends on another service
type MockConfig struct {
	DB     *MockDatabase
	Logger *MockLogger
}

func TestNewContainer(t *testing.T) {
	c := services.NewContainer()
	if c == nil {
		t.Fatal("NewContainer returned nil")
	}
	if c.ProviderCount() != 0 {
		t.Errorf("new container should have 0 providers, got %d", c.ProviderCount())
	}
	if c.InstanceCount() != 0 {
		t.Errorf("new container should have 0 instances, got %d", c.InstanceCount())
	}
}

func TestProvide_Get(t *testing.T) {
	c := services.NewContainer()

	// Test Provide
	err := c.Provide(func(c *services.Container) (*MockDatabase, error) {
		return &MockDatabase{Name: "test-db"}, nil
	})
	if err != nil {
		t.Fatalf("Provide failed: %v", err)
	}

	// Test Get (lazy initialization)
	var db *MockDatabase
	err = c.Get(&db)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if db == nil {
		t.Fatal("Get returned nil instance")
	}
	if db.Name != "test-db" {
		t.Errorf("expected Name=%q, got %q", "test-db", db.Name)
	}

	// Test singleton behavior (same instance on second Get)
	var db2 *MockDatabase
	err = c.Get(&db2)
	if err != nil {
		t.Fatalf("second Get failed: %v", err)
	}
	if db != db2 {
		t.Error("Get should return same instance (singleton)")
	}
}

func TestProvide_NilProvider(t *testing.T) {
	c := services.NewContainer()

	err := c.Provide(nil)
	if err == nil {
		t.Error("Provide(nil) should return error")
	}
}

func TestProvide_DuplicateType(t *testing.T) {
	c := services.NewContainer()

	// First registration should succeed
	err := c.Provide(func(c *services.Container) (*MockDatabase, error) {
		return &MockDatabase{Name: "db1"}, nil
	})
	if err != nil {
		t.Fatalf("first Provide failed: %v", err)
	}

	// Second registration of same type should fail
	err = c.Provide(func(c *services.Container) (*MockDatabase, error) {
		return &MockDatabase{Name: "db2"}, nil
	})
	if err == nil {
		t.Error("duplicate Provide should return error")
	}
}

func TestProvide_InvalidSignature(t *testing.T) {
	tests := []struct {
		name     string
		provider interface{}
		wantErr  bool
	}{
		{
			name:     "not a function",
			provider: "not a function",
			wantErr:  true,
		},
		{
			name:     "wrong number of params",
			provider: func() (*MockDatabase, error) { return nil, nil },
			wantErr:  true,
		},
		{
			name:     "wrong number of returns",
			provider: func(c *services.Container) *MockDatabase { return nil },
			wantErr:  true,
		},
		{
			name:     "second return not error",
			provider: func(c *services.Container) (*MockDatabase, string) { return nil, "" },
			wantErr:  true,
		},
		{
			name:     "valid signature",
			provider: func(c *services.Container) (*MockDatabase, error) { return nil, nil },
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := services.NewContainer()
			err := c.Provide(tt.provider)
			if (err != nil) != tt.wantErr {
				t.Errorf("Provide() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestProviderError(t *testing.T) {
	c := services.NewContainer()

	// Provider that returns error
	err := c.Provide(func(c *services.Container) (*MockDatabase, error) {
		return nil, errors.New("provider failed")
	})
	if err != nil {
		t.Fatalf("Provide failed: %v", err)
	}

	var db *MockDatabase
	err = c.Get(&db)
	if err == nil {
		t.Error("Get should fail when provider returns error")
	}
	if db != nil {
		t.Error("Get should return nil on error")
	}
}

func TestGet_NotFound(t *testing.T) {
	c := services.NewContainer()

	var db *MockDatabase
	err := c.Get(&db)
	if err == nil {
		t.Error("Get should fail for unregistered type")
	}
}

func TestGet_NilTarget(t *testing.T) {
	c := services.NewContainer()

	err := c.Get(nil)
	if err == nil {
		t.Error("Get(nil) should return error")
	}
}

func TestGet_NonPointerTarget(t *testing.T) {
	c := services.NewContainer()

	var db MockDatabase
	err := c.Get(db) // pass by value, not pointer
	if err == nil {
		t.Error("Get with non-pointer should return error")
	}
}

func TestMustGet(t *testing.T) {
	c := services.NewContainer()

	err := c.Provide(func(c *services.Container) (*MockDatabase, error) {
		return &MockDatabase{Name: "test-db"}, nil
	})
	if err != nil {
		t.Fatalf("Provide failed: %v", err)
	}

	// Should not panic
	var db *MockDatabase
	c.MustGet(&db)
	if db == nil {
		t.Error("MustGet returned nil")
	}
}

func TestMustGet_Panic(t *testing.T) {
	c := services.NewContainer()

	defer func() {
		if r := recover(); r == nil {
			t.Error("MustGet should panic for unregistered type")
		}
	}()

	var db *MockDatabase
	c.MustGet(&db)
	t.Error("MustGet should have panicked")
}

func TestProvideNamed_GetNamed(t *testing.T) {
	c := services.NewContainer()

	// Register two instances of same type with different names
	err := c.ProvideNamed("primary", func(c *services.Container) (*MockDatabase, error) {
		return &MockDatabase{Name: "primary-db"}, nil
	})
	if err != nil {
		t.Fatalf("ProvideNamed failed: %v", err)
	}

	err = c.ProvideNamed("secondary", func(c *services.Container) (*MockDatabase, error) {
		return &MockDatabase{Name: "secondary-db"}, nil
	})
	if err != nil {
		t.Fatalf("ProvideNamed failed: %v", err)
	}

	// Get primary
	var primary *MockDatabase
	err = c.GetNamed("primary", &primary)
	if err != nil {
		t.Fatalf("GetNamed failed: %v", err)
	}
	if primary.Name != "primary-db" {
		t.Errorf("expected Name=%q, got %q", "primary-db", primary.Name)
	}

	// Get secondary
	var secondary *MockDatabase
	err = c.GetNamed("secondary", &secondary)
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

func TestProvideNamed_Duplicate(t *testing.T) {
	c := services.NewContainer()

	err := c.ProvideNamed("primary", func(c *services.Container) (*MockDatabase, error) {
		return &MockDatabase{Name: "db1"}, nil
	})
	if err != nil {
		t.Fatalf("first ProvideNamed failed: %v", err)
	}

	err = c.ProvideNamed("primary", func(c *services.Container) (*MockDatabase, error) {
		return &MockDatabase{Name: "db2"}, nil
	})
	if err == nil {
		t.Error("duplicate ProvideNamed should fail")
	}
}

func TestProvideNamed_EmptyName(t *testing.T) {
	c := services.NewContainer()

	err := c.ProvideNamed("", func(c *services.Container) (*MockDatabase, error) {
		return nil, nil
	})
	if err == nil {
		t.Error("ProvideNamed with empty name should fail")
	}
}

func TestGetNamed_NotFound(t *testing.T) {
	c := services.NewContainer()

	var db *MockDatabase
	err := c.GetNamed("nonexistent", &db)
	if err == nil {
		t.Error("GetNamed should fail for unregistered name")
	}
}

func TestUnregister(t *testing.T) {
	c := services.NewContainer()

	// Register service
	err := c.Provide(func(c *services.Container) (*MockDatabase, error) {
		return &MockDatabase{Name: "test-db"}, nil
	})
	if err != nil {
		t.Fatalf("Provide failed: %v", err)
	}

	// Verify it's registered
	if !c.Has(&MockDatabase{}) {
		t.Error("Has should return true after registration")
	}

	// Unregister
	err = c.Unregister(&MockDatabase{})
	if err != nil {
		t.Fatalf("Unregister failed: %v", err)
	}

	// Verify it's gone
	if c.Has(&MockDatabase{}) {
		t.Error("Has should return false after Unregister")
	}

	// Get should fail
	var db *MockDatabase
	err = c.Get(&db)
	if err == nil {
		t.Error("Get should fail after Unregister")
	}
}

func TestUnregister_WithNamedServices(t *testing.T) {
	c := services.NewContainer()

	// Register named services
	err := c.ProvideNamed("primary", func(c *services.Container) (*MockDatabase, error) {
		return &MockDatabase{Name: "primary"}, nil
	})
	if err != nil {
		t.Fatalf("ProvideNamed failed: %v", err)
	}

	// Get the named service
	var db *MockDatabase
	err = c.GetNamed("primary", &db)
	if err != nil {
		t.Fatalf("GetNamed failed: %v", err)
	}

	// Unregister should remove all named instances
	err = c.Unregister(&MockDatabase{})
	if err != nil {
		t.Fatalf("Unregister failed: %v", err)
	}

	// Named service should also be gone
	err = c.GetNamed("primary", &db)
	if err == nil {
		t.Error("GetNamed should fail after Unregister")
	}
}

func TestUnregister_NotRegistered(t *testing.T) {
	c := services.NewContainer()

	err := c.Unregister(&MockDatabase{})
	if err != nil {
		t.Error("Unregister on non-existent service should not error")
	}
}

func TestHas(t *testing.T) {
	c := services.NewContainer()

	// Not registered yet
	if c.Has(&MockDatabase{}) {
		t.Error("Has should return false for unregistered type")
	}

	// Register
	err := c.Provide(func(c *services.Container) (*MockDatabase, error) {
		return &MockDatabase{Name: "test"}, nil
	})
	if err != nil {
		t.Fatalf("Provide failed: %v", err)
	}

	// Now registered
	if !c.Has(&MockDatabase{}) {
		t.Error("Has should return true after registration")
	}
}

func TestHas_NilTarget(t *testing.T) {
	c := services.NewContainer()

	if c.Has(nil) {
		t.Error("Has(nil) should return false")
	}
}

func TestHas_NonPointerTarget(t *testing.T) {
	c := services.NewContainer()

	var db MockDatabase
	if c.Has(db) { // pass by value
		t.Error("Has with non-pointer should return false")
	}
}

func TestKeys(t *testing.T) {
	c := services.NewContainer()

	// Empty container
	keys := c.Keys()
	if len(keys) != 0 {
		t.Errorf("empty container should have 0 keys, got %d", len(keys))
	}

	// Register services
	err := c.Provide(func(c *services.Container) (*MockDatabase, error) {
		return nil, nil
	})
	if err != nil {
		t.Fatalf("Provide failed: %v", err)
	}

	err = c.Provide(func(c *services.Container) (*MockLogger, error) {
		return nil, nil
	})
	if err != nil {
		t.Fatalf("Provide failed: %v", err)
	}

	err = c.ProvideNamed("primary", func(c *services.Container) (*MockConfig, error) {
		return nil, nil
	})
	if err != nil {
		t.Fatalf("ProvideNamed failed: %v", err)
	}

	// Should have 3 keys
	keys = c.Keys()
	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(keys))
	}
}

func TestKeys_ThreadSafety(t *testing.T) {
	c := services.NewContainer()

	// Register a service
	err := c.Provide(func(c *services.Container) (*MockDatabase, error) {
		return nil, nil
	})
	if err != nil {
		t.Fatalf("Provide failed: %v", err)
	}

	// Concurrent Keys() calls
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			keys := c.Keys()
			_ = keys
		}()
	}
	wg.Wait()
}

func TestClear(t *testing.T) {
	c := services.NewContainer()

	// Register multiple services
	err := c.Provide(func(c *services.Container) (*MockDatabase, error) {
		return &MockDatabase{Name: "test"}, nil
	})
	if err != nil {
		t.Fatalf("Provide failed: %v", err)
	}

	err = c.ProvideNamed("primary", func(c *services.Container) (*MockLogger, error) {
		return &MockLogger{Level: "debug"}, nil
	})
	if err != nil {
		t.Fatalf("ProvideNamed failed: %v", err)
	}

	// Get instances to cache them
	var db *MockDatabase
	err = c.Get(&db)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	var logger *MockLogger
	err = c.GetNamed("primary", &logger)
	if err != nil {
		t.Fatalf("GetNamed failed: %v", err)
	}

	// Clear
	c.Clear()

	// Everything should be gone
	if c.ProviderCount() != 0 {
		t.Error("Clear should remove all providers")
	}
	if c.InstanceCount() != 0 {
		t.Error("Clear should remove all instances")
	}
}

func TestConcurrentAccess(t *testing.T) {
	c := services.NewContainer()

	// Register service
	err := c.Provide(func(c *services.Container) (*MockDatabase, error) {
		return &MockDatabase{Name: "concurrent-test"}, nil
	})
	if err != nil {
		t.Fatalf("Provide failed: %v", err)
	}

	// Concurrent Get operations
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var db *MockDatabase
			err := c.Get(&db)
			if err != nil {
				t.Errorf("concurrent Get failed: %v", err)
				return
			}
			if db == nil {
				t.Error("concurrent Get returned nil")
			}
		}()
	}
	wg.Wait()

	// All should have gotten the same instance
	if c.InstanceCount() != 1 {
		t.Errorf("expected 1 cached instance, got %d", c.InstanceCount())
	}
}

func TestDependencyInjection(t *testing.T) {
	c := services.NewContainer()

	// Register database
	err := c.Provide(func(c *services.Container) (*MockDatabase, error) {
		return &MockDatabase{Name: "injected-db"}, nil
	})
	if err != nil {
		t.Fatalf("Provide MockDatabase failed: %v", err)
	}

	// Register logger
	err = c.Provide(func(c *services.Container) (*MockLogger, error) {
		return &MockLogger{Level: "info"}, nil
	})
	if err != nil {
		t.Fatalf("Provide MockLogger failed: %v", err)
	}

	// Register config that depends on database and logger
	err = c.Provide(func(c *services.Container) (*MockConfig, error) {
		var db *MockDatabase
		if err := c.Get(&db); err != nil {
			return nil, err
		}

		var logger *MockLogger
		if err := c.Get(&logger); err != nil {
			return nil, err
		}

		return &MockConfig{
			DB:     db,
			Logger: logger,
		}, nil
	})
	if err != nil {
		t.Fatalf("Provide MockConfig failed: %v", err)
	}

	// Get config (should automatically resolve dependencies)
	var config *MockConfig
	err = c.Get(&config)
	if err != nil {
		t.Fatalf("Get MockConfig failed: %v", err)
	}

	if config.DB == nil {
		t.Error("config.DB should not be nil")
	}
	if config.Logger == nil {
		t.Error("config.Logger should not be nil")
	}
	if config.DB.Name != "injected-db" {
		t.Errorf("expected DB.Name=%q, got %q", "injected-db", config.DB.Name)
	}
}

// mockPostgreSQL is a test implementation for interface binding tests
type mockPostgreSQL struct {
	ConnectionString string
}

func (p *mockPostgreSQL) Connect() error {
	return nil
}

func TestInterfaceBinding(t *testing.T) {
	c := services.NewContainer()

	// Register concrete type
	err := c.Provide(func(c *services.Container) (*mockPostgreSQL, error) {
		return &mockPostgreSQL{ConnectionString: "postgres://localhost"}, nil
	})
	if err != nil {
		t.Fatalf("Provide failed: %v", err)
	}

	// Get by concrete type
	var pg *mockPostgreSQL
	err = c.Get(&pg)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if pg.ConnectionString != "postgres://localhost" {
		t.Errorf("expected ConnectionString=%q, got %q", "postgres://localhost", pg.ConnectionString)
	}
}

func TestResetInstance(t *testing.T) {
	c := services.NewContainer()

	// Register service
	err := c.Provide(func(c *services.Container) (*MockDatabase, error) {
		return &MockDatabase{Name: "test-db"}, nil
	})
	if err != nil {
		t.Fatalf("Provide failed: %v", err)
	}

	// Get instance
	var db1 *MockDatabase
	err = c.Get(&db1)
	if err != nil {
		t.Fatalf("first Get failed: %v", err)
	}

	// Unregister and re-register with different factory
	err = c.Unregister(&MockDatabase{})
	if err != nil {
		t.Fatalf("Unregister failed: %v", err)
	}

	err = c.Provide(func(c *services.Container) (*MockDatabase, error) {
		return &MockDatabase{Name: "new-db"}, nil
	})
	if err != nil {
		t.Fatalf("second Provide failed: %v", err)
	}

	// Get new instance
	var db2 *MockDatabase
	err = c.Get(&db2)
	if err != nil {
		t.Fatalf("second Get failed: %v", err)
	}

	// Should be different instance
	if db1 == db2 {
		t.Error("instances should be different after reset")
	}
	if db2.Name != "new-db" {
		t.Errorf("expected Name=%q, got %q", "new-db", db2.Name)
	}
}
