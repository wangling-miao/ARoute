package registry

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/wangling-miao/aroute/core"
	bolt "go.etcd.io/bbolt"
)

const (
	// BucketName is the bbolt bucket name for storing plugin entries.
	BucketName = "plugins"
)

// BoltRegistry implements Registry using bbolt as the persistent storage backend.
// All operations are thread-safe. bbolt supports concurrent readers but only one writer.
//
// Thread safety:
// - Read operations (Get, List) use read transactions
// - Write operations (Register, Update, Remove, Enable, Disable) use write transactions
// - bbolt serializes write transactions automatically
type BoltRegistry struct {
	mu     sync.RWMutex
	db     *bolt.DB
	closed bool
}

// NewBoltRegistry creates a new registry backed by bbolt.
// The database file will be created if it doesn't exist.
// The bucket will be created on first use.
//
// Thread safety: Safe to call from multiple goroutines.
func NewBoltRegistry(dbPath string) (*BoltRegistry, error) {
	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("open bbolt database: %w", err)
	}

	// Create bucket if it doesn't exist
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(BucketName))
		if err != nil {
			return fmt.Errorf("create bucket: %w", err)
		}
		return nil
	})

	if err != nil {
		db.Close()
		return nil, err
	}

	return &BoltRegistry{
		db: db,
	}, nil
}

// Register adds a new plugin to the registry.
// Returns ErrPluginExists if a plugin with the same name already exists.
// The plugin is registered with enabled=true by default (per spec requirement).
func (r *BoltRegistry) Register(entry *PluginEntry) error {
	if entry == nil {
		return &PluginError{Op: "register", Msg: "entry cannot be nil"}
	}

	if err := entry.Manifest.Validate(); err != nil {
		return &PluginError{
			PluginName: entry.Manifest.Name,
			Op:         "register",
			Msg:        fmt.Sprintf("invalid manifest: %v", err),
		}
	}

	// Enforce initial state as enabled=true per spec requirement
	entry.Enabled = true

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return ErrRegistryClosed
	}

	err := r.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(BucketName))
		if bucket == nil {
			return fmt.Errorf("bucket not found")
		}

		// Check if plugin already exists
		existing := bucket.Get([]byte(entry.Manifest.Name))
		if existing != nil {
			return &PluginError{
				PluginName: entry.Manifest.Name,
				Op:         "register",
				Msg:        "plugin already exists",
			}
		}

		// Serialize entry
		data, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("serialize entry: %w", err)
		}

		// Store entry
		return bucket.Put([]byte(entry.Manifest.Name), data)
	})

	return err
}

// Get retrieves a plugin entry by name.
// Returns ErrPluginNotFound if no plugin with that name exists.
func (r *BoltRegistry) Get(name string) (*PluginEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.closed {
		return nil, ErrRegistryClosed
	}

	var entry PluginEntry

	err := r.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(BucketName))
		if bucket == nil {
			return ErrPluginNotFound
		}

		data := bucket.Get([]byte(name))
		if data == nil {
			return &PluginError{
				PluginName: name,
				Op:         "get",
				Msg:        "plugin not found",
			}
		}

		if err := json.Unmarshal(data, &entry); err != nil {
			return fmt.Errorf("deserialize entry: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return &entry, nil
}

// List returns all registered plugins.
// Returns an empty slice if no plugins are registered.
func (r *BoltRegistry) List() ([]*PluginEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.closed {
		return nil, ErrRegistryClosed
	}

	var entries []*PluginEntry

	err := r.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(BucketName))
		if bucket == nil {
			return nil // Empty bucket
		}

		return bucket.ForEach(func(key, value []byte) error {
			var entry PluginEntry
			if err := json.Unmarshal(value, &entry); err != nil {
				return fmt.Errorf("deserialize entry %s: %w", string(key), err)
			}
			entries = append(entries, &entry)
			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	// Return empty slice, not nil
	if entries == nil {
		entries = []*PluginEntry{}
	}

	return entries, nil
}

// Update modifies a plugin's manifest while preserving its enabled state.
// Returns ErrPluginNotFound if the plugin doesn't exist.
func (r *BoltRegistry) Update(name string, manifest core.Manifest) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return ErrRegistryClosed
	}

	return r.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(BucketName))
		if bucket == nil {
			return &PluginError{
				PluginName: name,
				Op:         "update",
				Msg:        "plugin not found",
			}
		}

		// Get existing entry
		data := bucket.Get([]byte(name))
		if data == nil {
			return &PluginError{
				PluginName: name,
				Op:         "update",
				Msg:        "plugin not found",
			}
		}

		var entry PluginEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			return fmt.Errorf("deserialize entry: %w", err)
		}

		// Validate new manifest
		if err := manifest.Validate(); err != nil {
			return &PluginError{
				PluginName: name,
				Op:         "update",
				Msg:        fmt.Sprintf("invalid manifest: %v", err),
			}
		}

		// Update manifest, preserve enabled state
		entry.Manifest = manifest

		// Serialize and store
		newData, err := json.Marshal(&entry)
		if err != nil {
			return fmt.Errorf("serialize entry: %w", err)
		}

		return bucket.Put([]byte(name), newData)
	})
}

// Remove deletes a plugin entry from the registry.
// Returns ErrPluginNotFound if the plugin doesn't exist.
func (r *BoltRegistry) Remove(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return ErrRegistryClosed
	}

	return r.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(BucketName))
		if bucket == nil {
			return &PluginError{
				PluginName: name,
				Op:         "remove",
				Msg:        "plugin not found",
			}
		}

		// Check if exists
		data := bucket.Get([]byte(name))
		if data == nil {
			return &PluginError{
				PluginName: name,
				Op:         "remove",
				Msg:        "plugin not found",
			}
		}

		return bucket.Delete([]byte(name))
	})
}

// Enable sets a plugin's enabled state to true.
// Returns ErrPluginNotFound if the plugin doesn't exist.
// If the plugin is already enabled, this is a no-op.
func (r *BoltRegistry) Enable(name string) error {
	return r.setEnabled(name, true)
}

// Disable sets a plugin's enabled state to false.
// Returns ErrPluginNotFound if the plugin doesn't exist.
// If the plugin is already disabled, this is a no-op.
func (r *BoltRegistry) Disable(name string) error {
	return r.setEnabled(name, false)
}

// setEnabled updates the plugin's enabled state.
func (r *BoltRegistry) setEnabled(name string, enabled bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return ErrRegistryClosed
	}

	return r.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(BucketName))
		if bucket == nil {
			return &PluginError{
				PluginName: name,
				Op:         "enable",
				Msg:        "plugin not found",
			}
		}

		// Get existing entry
		data := bucket.Get([]byte(name))
		if data == nil {
			op := "enable"
			if !enabled {
				op = "disable"
			}
			return &PluginError{
				PluginName: name,
				Op:         op,
				Msg:        "plugin not found",
			}
		}

		var entry PluginEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			return fmt.Errorf("deserialize entry: %w", err)
		}

		// No-op if already in desired state
		if entry.Enabled == enabled {
			return nil
		}

		// Update state
		entry.Enabled = enabled

		// Serialize and store
		newData, err := json.Marshal(&entry)
		if err != nil {
			return fmt.Errorf("serialize entry: %w", err)
		}

		return bucket.Put([]byte(name), newData)
	})
}

// Close releases the database connection.
// After Close, all other methods will return ErrRegistryClosed.
func (r *BoltRegistry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}

	r.closed = true
	return r.db.Close()
}
