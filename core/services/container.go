// Package services provides the ServiceContainer implementation for dependency injection.
// It supports lazy initialization, singleton caching, named services, and hot-plugging.
package services

import (
	"fmt"
	"reflect"
	"sync"
)

// Container is the concrete implementation of ServiceContainer.
// It provides lazy initialization with caching, thread-safe operations,
// and support for hot-plugging (runtime service registration/unregistration).
//
// Thread safety: All operations are protected by Mutex. We use a regular Mutex
// instead of RWMutex because provider functions may call Get() recursively to
// resolve dependencies, which would deadlock with RWMutex (write lock blocks
// recursive read locks from same goroutine).
type Container struct {
	mu sync.Mutex

	// providers stores lazy factory functions for each service type.
	// Key is the fully qualified type name (e.g., "*github.com/example.UserService").
	// Value is a function that returns the service instance.
	providers map[string]providerFunc

	// instances stores singleton instances after first Get.
	// Key is the type name, value is the cached instance.
	instances map[string]interface{}

	// namedProviders stores lazy factory functions for named services.
	// Key is "type:name", value is the provider function.
	namedProviders map[string]providerFunc

	// namedInstances stores singleton instances for named services.
	// Key is "type:name", value is the cached instance.
	namedInstances map[string]interface{}

	// typeNames maps reflect.Type to its string representation.
	// This allows us to use reflect.Type as map keys efficiently.
	typeNames map[reflect.Type]string
}

// providerFunc is a lazy factory function that creates a service instance.
type providerFunc func() (interface{}, error)

// NewContainer creates a new, empty ServiceContainer.
func NewContainer() *Container {
	return &Container{
		providers:      make(map[string]providerFunc),
		instances:      make(map[string]interface{}),
		namedProviders: make(map[string]providerFunc),
		namedInstances: make(map[string]interface{}),
		typeNames:      make(map[reflect.Type]string),
	}
}

// typeKeyLocked returns a unique string key for a reflect.Type.
// Must be called with c.mu held.
func (c *Container) typeKeyLocked(t reflect.Type) string {
	if key, ok := c.typeNames[t]; ok {
		return key
	}

	// Use pointer type format for consistency
	if t.Kind() == reflect.Ptr {
		key := t.String()
		c.typeNames[t] = key
		return key
	}

	// For interface types, use the interface pointer format
	key := "*" + t.String()
	c.typeNames[t] = key
	return key
}

// Provide registers a service provider.
// The provider is called lazily on first Get.
// Returns an error if a service with the same type is already registered.
//
// The provider function receives the Container itself, allowing providers
// to resolve dependencies through the container.
//
// Thread safety: Uses write lock to prevent concurrent modifications.
func (c *Container) Provide(provider interface{}) error {
	if provider == nil {
		return fmt.Errorf("services: provider cannot be nil")
	}

	// Use reflection to extract the type from provider function signature
	providerType := reflect.TypeOf(provider)
	if providerType.Kind() != reflect.Func {
		return fmt.Errorf("services: provider must be a function, got %T", provider)
	}

	// Provider function signature: func(container) (T, error)
	if providerType.NumIn() != 1 {
		return fmt.Errorf("services: provider function must accept exactly one parameter (container)")
	}

	if providerType.NumOut() != 2 {
		return fmt.Errorf("services: provider function must return (T, error)")
	}

	// The return type (T) is what we'll provide
	returnType := providerType.Out(0)
	errorType := providerType.Out(1)

	// Verify second return value is error
	if !errorType.Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		return fmt.Errorf("services: provider function second return value must be error")
	}

	// Get type key (must be inside lock since typeNames map is shared state)
	c.mu.Lock()
	defer c.mu.Unlock()

	typeKey := c.typeKeyLocked(returnType)

	// Check if already registered
	if _, exists := c.providers[typeKey]; exists {
		return fmt.Errorf("services: type %s already registered", returnType)
	}

	// Store the lazy provider
	c.providers[typeKey] = func() (interface{}, error) {
		// Call the provider function with container reference
		results := reflect.ValueOf(provider).Call([]reflect.Value{reflect.ValueOf(c)})
		instance := results[0].Interface()

		var err error
		if !results[1].IsNil() {
			err = results[1].Interface().(error)
		}

		return instance, err
	}

	return nil
}

// Get retrieves a service by type.
// Returns an error if the service is not found or initialization failed.
// Services are created on first Get call (lazy initialization) and cached.
//
// Thread safety: The mutex is released during provider execution to support
// recursive dependency resolution (providers calling Get() for dependencies).
// This uses a check-then-act pattern with re-checking after provider execution.
func (c *Container) Get(target interface{}) error {
	if target == nil {
		return fmt.Errorf("services: target cannot be nil")
	}

	targetValue := reflect.ValueOf(target)
	if targetValue.Kind() != reflect.Ptr || targetValue.IsNil() {
		return fmt.Errorf("services: target must be a non-nil pointer")
	}

	// Get the type that the pointer points to
	targetType := targetValue.Elem().Type()

	c.mu.Lock()

	typeKey := c.typeKeyLocked(targetType)

	// Check if already cached
	if instance, ok := c.instances[typeKey]; ok {
		c.mu.Unlock()
		targetValue.Elem().Set(reflect.ValueOf(instance))
		return nil
	}

	// Find provider
	provider, ok := c.providers[typeKey]
	if !ok {
		c.mu.Unlock()
		return fmt.Errorf("services: type %s not registered", targetType)
	}

	// Release lock before calling provider (which may call Get recursively)
	c.mu.Unlock()

	// Create instance (provider may call Get() for dependencies)
	instance, err := provider()
	if err != nil {
		return fmt.Errorf("services: failed to create instance of %s: %w", targetType, err)
	}
	if instance == nil {
		return fmt.Errorf("services: provider for %s returned nil instance", targetType)
	}

	// Re-acquire lock to cache instance
	c.mu.Lock()

	// Check if another goroutine created it while we were unlocked
	if existing, ok := c.instances[typeKey]; ok {
		// Another goroutine won - use their instance
		c.mu.Unlock()
		targetValue.Elem().Set(reflect.ValueOf(existing))
		return nil
	}

	// Cache instance
	c.instances[typeKey] = instance
	c.mu.Unlock()

	// Set target
	targetValue.Elem().Set(reflect.ValueOf(instance))
	return nil
}

// MustGet retrieves a service by type, panicking on error.
// Use for services that MUST be present (programmer error if missing).
//
// Thread safety: Same as Get.
func (c *Container) MustGet(target interface{}) {
	if err := c.Get(target); err != nil {
		panic(err)
	}
}

// ProvideNamed registers a named service provider.
// Use when multiple instances of the same type exist (e.g., multiple database connections).
//
// Thread safety: Uses write lock.
func (c *Container) ProvideNamed(name string, provider interface{}) error {
	if name == "" {
		return fmt.Errorf("services: name cannot be empty")
	}
	if provider == nil {
		return fmt.Errorf("services: provider cannot be nil")
	}

	// Validate provider function signature
	providerType := reflect.TypeOf(provider)
	if providerType.Kind() != reflect.Func {
		return fmt.Errorf("services: provider must be a function, got %T", provider)
	}
	if providerType.NumIn() != 1 {
		return fmt.Errorf("services: provider function must accept exactly one parameter (container)")
	}
	if providerType.NumOut() != 2 {
		return fmt.Errorf("services: provider function must return (T, error)")
	}

	returnType := providerType.Out(0)
	errorType := providerType.Out(1)

	if !errorType.Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		return fmt.Errorf("services: provider function second return value must be error")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	typeKey := c.typeKeyLocked(returnType)
	combinedKey := typeKey + ":" + name

	if _, exists := c.namedProviders[combinedKey]; exists {
		return fmt.Errorf("services: named service %s:%s already registered", returnType, name)
	}

	c.namedProviders[combinedKey] = func() (interface{}, error) {
		results := reflect.ValueOf(provider).Call([]reflect.Value{reflect.ValueOf(c)})
		instance := results[0].Interface()

		var err error
		if !results[1].IsNil() {
			err = results[1].Interface().(error)
		}

		return instance, err
	}

	return nil
}

// GetNamed retrieves a named service instance.
// Use when multiple instances of the same type exist.
//
// Thread safety: The mutex is released during provider execution.
func (c *Container) GetNamed(name string, target interface{}) error {
	if name == "" {
		return fmt.Errorf("services: name cannot be empty")
	}
	if target == nil {
		return fmt.Errorf("services: target cannot be nil")
	}

	targetValue := reflect.ValueOf(target)
	if targetValue.Kind() != reflect.Ptr || targetValue.IsNil() {
		return fmt.Errorf("services: target must be a non-nil pointer")
	}

	targetType := targetValue.Elem().Type()

	c.mu.Lock()

	typeKey := c.typeKeyLocked(targetType)
	combinedKey := typeKey + ":" + name

	// Check if already cached
	if instance, ok := c.namedInstances[combinedKey]; ok {
		c.mu.Unlock()
		targetValue.Elem().Set(reflect.ValueOf(instance))
		return nil
	}

	// Find provider
	provider, ok := c.namedProviders[combinedKey]
	if !ok {
		c.mu.Unlock()
		return fmt.Errorf("services: named service %s:%s not registered", targetType, name)
	}

	// Release lock before calling provider
	c.mu.Unlock()

	// Create instance
	instance, err := provider()
	if err != nil {
		return fmt.Errorf("services: failed to create instance of %s:%s: %w", targetType, name, err)
	}
	if instance == nil {
		return fmt.Errorf("services: provider for %s:%s returned nil instance", targetType, name)
	}

	// Re-acquire lock
	c.mu.Lock()

	// Check if another goroutine created it
	if existing, ok := c.namedInstances[combinedKey]; ok {
		c.mu.Unlock()
		targetValue.Elem().Set(reflect.ValueOf(existing))
		return nil
	}

	// Cache instance
	c.namedInstances[combinedKey] = instance
	c.mu.Unlock()

	// Set target
	targetValue.Elem().Set(reflect.ValueOf(instance))
	return nil
}

// Unregister removes a service from the container.
// Used for hot-plug support when disabling plugins.
// Also removes any cached instance.
//
// Thread safety: Uses write lock.
func (c *Container) Unregister(target interface{}) error {
	if target == nil {
		return fmt.Errorf("services: target cannot be nil")
	}

	targetType := reflect.TypeOf(target)
	if targetType.Kind() != reflect.Ptr {
		return fmt.Errorf("services: target must be a pointer to the type to unregister")
	}

	// Get the element type
	elemType := targetType.Elem()

	c.mu.Lock()
	defer c.mu.Unlock()

	typeKey := c.typeKeyLocked(elemType)

	// Remove provider and instance
	delete(c.providers, typeKey)
	delete(c.instances, typeKey)

	// Also remove all named instances of this type
	prefix := typeKey + ":"
	for key := range c.namedProviders {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			delete(c.namedProviders, key)
			delete(c.namedInstances, key)
		}
	}

	return nil
}

// Has checks if a service is registered.
// Returns true if a provider exists for the type, false otherwise.
// Does not check if an instance is cached.
//
// Thread safety: Uses mutex lock.
func (c *Container) Has(target interface{}) bool {
	if target == nil {
		return false
	}

	targetType := reflect.TypeOf(target)
	if targetType.Kind() != reflect.Ptr {
		return false
	}

	elemType := targetType.Elem()

	c.mu.Lock()
	defer c.mu.Unlock()

	typeKey := c.typeKeyLocked(elemType)

	_, exists := c.providers[typeKey]
	return exists
}

// Keys returns all registered service type names.
// Useful for debugging and introspection.
// Returns both unnamed and named services in the format "TypeName" or "TypeName:name".
//
// Thread safety: Uses mutex lock.
func (c *Container) Keys() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	keys := make([]string, 0, len(c.providers)+len(c.namedProviders))

	// Add unnamed services
	for key := range c.providers {
		keys = append(keys, key)
	}

	// Add named services
	for key := range c.namedProviders {
		keys = append(keys, key)
	}

	return keys
}

// Clear removes all registered services and cached instances.
// Useful for testing and cleanup.
//
// Thread safety: Uses write lock.
func (c *Container) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.providers = make(map[string]providerFunc)
	c.instances = make(map[string]interface{})
	c.namedProviders = make(map[string]providerFunc)
	c.namedInstances = make(map[string]interface{})
}

// InstanceCount returns the number of cached instances (both unnamed and named).
// Useful for diagnostics and testing.
//
// Thread safety: Uses mutex lock.
func (c *Container) InstanceCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.instances) + len(c.namedInstances)
}

// ProviderCount returns the number of registered providers (both unnamed and named).
// Useful for diagnostics and testing.
//
// Thread safety: Uses mutex lock.
func (c *Container) ProviderCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.providers) + len(c.namedProviders)
}
