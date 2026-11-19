//go:build !bindings

package main

// Обычная сборка приложения (не генерация биндингов) — см. bindings_tag_on.go.
const wailsBindingsMode = false

// runBindingsGeneration в обычной сборке не вызывается (wailsBindingsMode=false).
func runBindingsGeneration() {}
