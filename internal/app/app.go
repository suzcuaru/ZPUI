package app

import (
	"context"
	"fmt"
	"runtime"

	"zpui/internal/config"
	"zpui/internal/logger"
	"zpui/internal/modules"
)

type App struct {
	ctx     context.Context
	cfg     *config.Config
	log     *logger.Logger
	mgr     *modules.Manager
	version string
	exeDir  string
	hidden  bool
}

func New(cfg *config.Config, log *logger.Logger, mgr *modules.Manager, version, exeDir string) *App {
	return &App{
		cfg:     cfg,
		log:     log,
		mgr:     mgr,
		version: version,
		exeDir:  exeDir,
	}
}

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	a.log.Info("app", fmt.Sprintf("ZPUI v%s started (Go %s %s/%s)", a.version, runtime.Version(), runtime.GOOS, runtime.GOARCH))
	a.log.Info("app", fmt.Sprintf("Modules dir: %s", a.mgr.RootDir()))
}

func (a *App) Shutdown(ctx context.Context) {
	a.log.Info("app", "Stopping all modules...")
	a.mgr.StopAll()
	a.log.Info("app", "Shutdown complete")
}
