//go:build bindings

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"

	"zpui/internal/app"
)

// wailsBindingsMode — сборка выполняется генератором биндингов wails
// (wails generate module / wails build без -s). Такой запуск идёт без прав
// администратора и вне окружения приложения.
const wailsBindingsMode = true

// bindingsLogger — интерфейс wails-логгера в stdout (генерация печатает ход).
type bindingsLogger struct{}

func (bindingsLogger) Print(message string) { fmt.Fprintln(os.Stdout, message) }
func (bindingsLogger) Trace(message string)   { fmt.Fprintln(os.Stdout, message) }
func (bindingsLogger) Debug(message string)   { fmt.Fprintln(os.Stdout, message) }
func (bindingsLogger) Info(message string)    { fmt.Fprintln(os.Stdout, message) }
func (bindingsLogger) Warning(message string) { fmt.Fprintln(os.Stdout, message) }
func (bindingsLogger) Error(message string)   { fmt.Fprintln(os.Stderr, message) }
func (bindingsLogger) Fatal(message string)   { fmt.Fprintln(os.Stderr, message) }

// runBindingsGeneration — перечисление методов App для wailsjs. Экземпляр
// нужен только как тип: reflection читает методы, никакие зависимости App
// (config, БД, движок) не инициализируются и не вызываются.
func runBindingsGeneration() {
	a := &app.App{}
	err := wails.Run(&options.App{
		Title:     "ZPUI bindings",
		Width:     960,
		Height:    640,
		Bind:      []interface{}{a},
		Logger:    bindingsLogger{},
		LogLevel:  logger.WARNING,
		OnStartup: func(ctx context.Context) {},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "bindings generation:", err)
		os.Exit(1)
	}
}
