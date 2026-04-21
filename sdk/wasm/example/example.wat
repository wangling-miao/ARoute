(module
  ;; Host function imports from the "cms" module
  (import "cms" "service_has" (func $cms_service_has (param i32) (result i32)))
  (import "cms" "service_get" (func $cms_service_get (param i32) (result i32)))
  (import "cms" "event_subscribe" (func $cms_event_subscribe (param i32 i32) (result i32)))
  (import "cms" "event_publish" (func $cms_event_publish (param i32 i32 i32 i32)))
  (import "cms" "memory_alloc" (func $cms_memory_alloc (param i32) (result i32)))
  (import "cms" "memory_free" (func $cms_memory_free (param i32)))

  ;; Memory: 1 page (64KB) minimum, 512 max (32MB)
  (memory (export "memory") 1 512)

  ;; Data segments
  ;; offset 0:   "wasm.example.started" (20 bytes)
  ;; offset 32:  started payload (62 bytes)
  ;; offset 128: "wasm.example.stopped" (20 bytes)
  ;; offset 160: stopped payload (60 bytes)
  (data (i32.const 0) "wasm.example.started")
  (data (i32.const 32) "{\"plugin\":\"example-wasm\",\"version\":\"1.0.0\",\"status\":\"running\"}")
  (data (i32.const 128) "wasm.example.stopped")
  (data (i32.const 160) "{\"plugin\":\"example-wasm\",\"status\":\"stopped\",\"graceful\":true}")

  ;; init — called during plugin initialization
  (func (export "init") (result i32)
    ;; Check if service index 0 is available
    (call $cms_service_has (i32.const 0))
    drop
    (i32.const 0)
  )

  ;; start — called after all plugins are initialized
  (func (export "start") (result i32)
    ;; Publish "wasm.example.started" event with plugin metadata
    (call $cms_event_publish
      (i32.const 0)    ;; topic offset
      (i32.const 20)   ;; topic length
      (i32.const 32)   ;; data offset
      (i32.const 62)   ;; data length
    )
    (i32.const 0)
  )

  ;; stop — called during graceful shutdown
  (func (export "stop") (result i32)
    ;; Publish "wasm.example.stopped" event
    (call $cms_event_publish
      (i32.const 128)  ;; topic offset
      (i32.const 20)   ;; topic length
      (i32.const 160)  ;; data offset
      (i32.const 60)   ;; data length
    )
    (i32.const 0)
  )

  ;; allocate_memory — simple bump allocator for host memory operations
  (func (export "allocate_memory") (param $size i32) (result i32)
    (i32.const 256)
  )
)
