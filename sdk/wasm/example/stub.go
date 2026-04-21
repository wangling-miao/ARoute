//go:build !tinygo

// Package main provides an L3 Wasm example plugin for Aroute CMS.
// This file is a stub for standard Go builds. Build with TinyGo to get
// the actual Wasm output:
//
//	tinygo build -o build/example.wasm -target=wasi -no-debug -scheduler=none -gc=leaking .
package main

func main() {}
