// Package services provides the ServiceContainer implementation for dependency injection.
package services

import (
	"fmt"
	"reflect"
)

// Provide registers a service provider using Go generics.
// The provider is called lazily on first Get.
// Returns an error if a service with the same type is already registered.
// Supports override: re-registering replaces the previous provider and discards cached instance.
func Provide[T any](c *Container, provider func() (T, error)) error {
	if provider == nil {
		return fmt.Errorf("services: provider cannot be nil")
	}

	var zero T
	returnType := reflect.TypeOf(zero)
	typeKey := c.typeKey(returnType)

	wrapped := func() (interface{}, error) {
		return provider()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Spec requires service override support - replace existing
	if _, exists := c.providers[typeKey]; exists {
		delete(c.providers, typeKey)
		delete(c.instances, typeKey)
	}
	c.providers[typeKey] = wrapped
	return nil
}

// Get retrieves a service by type using Go generics.
// Returns the service instance or an error if not found/initialization failed.
func Get[T any](c *Container) (T, error) {
	var zero T
	returnType := reflect.TypeOf(zero)
	typeKey := c.typeKey(returnType)

	c.mu.Lock()

	if instance, ok := c.instances[typeKey]; ok {
		c.mu.Unlock()
		return instance.(T), nil
	}

	provider, ok := c.providers[typeKey]
	if !ok {
		c.mu.Unlock()
		return zero, fmt.Errorf("services: type %s not registered", typeKey)
	}

	c.mu.Unlock()

	instance, err := provider()
	if err != nil {
		return zero, fmt.Errorf("services: failed to create instance of %s: %w", typeKey, err)
	}

	c.mu.Lock()
	if existing, ok := c.instances[typeKey]; ok {
		c.mu.Unlock()
		return existing.(T), nil
	}
	c.instances[typeKey] = instance
	c.mu.Unlock()

	return instance.(T), nil
}

// MustGet retrieves a service by type, panicking on error.
// Use for services that MUST be present (programmer error if missing).
func MustGet[T any](c *Container) T {
	t, err := Get[T](c)
	if err != nil {
		panic(err)
	}
	return t
}

// ProvideNamed registers a named service provider using Go generics.
// Use when multiple instances of the same type exist (e.g., multiple databases).
func ProvideNamed[T any](c *Container, name string, provider func() (T, error)) error {
	if name == "" {
		return fmt.Errorf("services: name cannot be empty")
	}
	if provider == nil {
		return fmt.Errorf("services: provider cannot be nil")
	}

	var zero T
	returnType := reflect.TypeOf(zero)
	typeKey := c.typeKey(returnType)
	combinedKey := typeKey + ":" + name

	wrapped := func() (interface{}, error) {
		return provider()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Override support
	if _, exists := c.namedProviders[combinedKey]; exists {
		delete(c.namedProviders, combinedKey)
		delete(c.namedInstances, combinedKey)
	}
	c.namedProviders[combinedKey] = wrapped
	return nil
}

// GetNamed retrieves a named service instance using Go generics.
func GetNamed[T any](c *Container, name string) (T, error) {
	var zero T
	returnType := reflect.TypeOf(zero)
	typeKey := c.typeKey(returnType)
	combinedKey := typeKey + ":" + name

	c.mu.Lock()

	if instance, ok := c.namedInstances[combinedKey]; ok {
		c.mu.Unlock()
		return instance.(T), nil
	}

	provider, ok := c.namedProviders[combinedKey]
	if !ok {
		c.mu.Unlock()
		return zero, fmt.Errorf("services: named service %s:%s not registered", typeKey, name)
	}

	c.mu.Unlock()

	instance, err := provider()
	if err != nil {
		return zero, fmt.Errorf("services: failed to create instance of %s:%s: %w", typeKey, name, err)
	}

	c.mu.Lock()
	if existing, ok := c.namedInstances[combinedKey]; ok {
		c.mu.Unlock()
		return existing.(T), nil
	}
	c.namedInstances[combinedKey] = instance
	c.mu.Unlock()

	return instance.(T), nil
}

// Remove removes a service registration from the container.
// After removal, the service can be re-registered with Provide.
func Remove[T any](c *Container) {
	var zero T
	typeKey := c.typeKey(reflect.TypeOf(zero))

	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.providers, typeKey)
	delete(c.instances, typeKey)

	prefix := typeKey + ":"
	for key := range c.namedProviders {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			delete(c.namedProviders, key)
			delete(c.namedInstances, key)
		}
	}
}

// RemoveNamed removes a named service registration from the container.
func RemoveNamed[T any](c *Container, name string) {
	var zero T
	typeKey := c.typeKey(reflect.TypeOf(zero))
	combinedKey := typeKey + ":" + name

	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.namedProviders, combinedKey)
	delete(c.namedInstances, combinedKey)
}

// Has checks if a service is registered using Go generics.
// Returns true if a provider exists for the type, false otherwise.
func Has[T any](c *Container) bool {
	var zero T
	typeKey := c.typeKey(reflect.TypeOf(zero))

	c.mu.Lock()
	defer c.mu.Unlock()

	_, exists := c.providers[typeKey]
	return exists
}
