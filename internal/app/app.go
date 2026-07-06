package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"zpui/internal/config"
	"zpui/internal/database"
	"zpui/internal/logger"
	"zpui/internal/modules"
	"zpui/internal/updater"
)

type App struct {
	ctx        context.Context
	cfg        *config.Config
	log        *logger.Logger
	db         *database.DB
	mgr        *modules.Manager
	updater    *updater.Updater
	version    string
	exeDir     string
	hidden     bool
	skipChecks bool
	startup    *startupState
	pidPath    string
}

func New(cfg *config.Config, log *logger.Logger, db *database.DB, mgr *modules.Manager, upd *updater.Updater, version, exeDir string, skipChecks bool) *App {
	return &App{
		cfg:        cfg,
		log:        log,
		db:         db,
		mgr:        mgr,
		updater:    upd,
		version:    version,
		exeDir:     exeDir,
		skipChecks: skipChecks,
		startup:    &startupState{info: StartupInfo{Stage: StageWelcome, Progress: 0}},
		pidPath:    filepath.Join(exeDir, "zpui.pid"),
	}
}

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	a.log.Info("app", fmt.Sprintf("ZPUI v%s started (Go %s %s/%s)", a.version, runtime.Version(), runtime.GOOS, runtime.GOARCH))
	a.log.Info("app", fmt.Sprintf("Modules dir: %s", a.mgr.RootDir()))

	_ = os.WriteFile(a.pidPath, []byte(strconv.Itoa(os.Getpid())), 0644)

	if !a.skipChecks {
		go a.runStartupSequence()
	} else {
		a.startup.update(func(s *StartupInfo) {
			s.Stage = StageDone
			s.Progress = 1.0
		})
		a.startup.completed = true
	}
}

func (a *App) Shutdown(ctx context.Context) {
	a.log.Info("app", "Stopping all modules...")
	a.mgr.StopAll()
	os.Remove(a.pidPath)
	a.log.Info("app", "Shutdown complete")
}

func (a *App) ExeDir() string  { return a.exeDir }
func (a *App) Version() string { return a.version }
