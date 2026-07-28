//go:build desktop

package cli

import (
	"context"
	_ "embed"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/SEObserver/crawlobserver/internal/apikeys"
	"github.com/SEObserver/crawlobserver/internal/applog"
	"github.com/SEObserver/crawlobserver/internal/backup"
	chmanaged "github.com/SEObserver/crawlobserver/internal/clickhouse"
	"github.com/SEObserver/crawlobserver/internal/config"
	"github.com/SEObserver/crawlobserver/internal/server"
	"github.com/SEObserver/crawlobserver/internal/storage"
	"github.com/SEObserver/crawlobserver/internal/telemetry"
	"github.com/SEObserver/crawlobserver/internal/updater"
	"github.com/posthog/posthog-go"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	webview "github.com/webview/webview_go"
)

//go:embed appicon.png
var appIcon []byte

var guiCmd = &cobra.Command{
	Use:   "gui",
	Short: "Start the desktop GUI",
	Long:  `Start the native desktop application with embedded web UI.`,
	RunE:  runGUI,
}

func init() {
	rootCmd.AddCommand(guiCmd)

	// Make "gui" the default command when no subcommand is given (double-click .app)
	defaultCmd := guiCmd
	originalRun := rootCmd.RunE
	rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if originalRun != nil {
			return originalRun(cmd, args)
		}
		return defaultCmd.RunE(cmd, args)
	}
}

func runGUI(cmd *cobra.Command, args []string) error {
	// Ensure data directory exists for GUI mode (macOS launches .app with cwd=/)
	dataDir, err := appDataDir()
	if err != nil {
		return fmt.Errorf("creating data directory: %w", err)
	}

	// Point viper to writable config in app data dir (cwd is / in .app bundles)
	viper.SetConfigFile(filepath.Join(dataDir, "config.yaml"))
	_ = viper.ReadInConfig()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	telemetry.Init(cfg.Telemetry.Enabled, cfg.Telemetry.InstanceID, updater.Version)
	defer telemetry.Close()
	telemetry.Track("app_started", posthog.NewProperties().Set("mode", "desktop"))

	// In desktop mode, auth is unnecessary — server listens on 127.0.0.1 only
	cfg.Server.Username = ""
	cfg.Server.Password = ""

	// Find port BEFORE creating the webview
	guiPort, err := findFreePort()
	if err != nil {
		return fmt.Errorf("finding free port: %w", err)
	}
	cfg.Server.Port = guiPort
	applog.Infof("cli", "GUI mode: using internal HTTP port %d", guiPort)

	serverURL := fmt.Sprintf("http://127.0.0.1:%d", guiPort)

	appName := "CrawlObserver"
	if cfg.Theme.AppName != "" {
		appName = cfg.Theme.AppName
	}

	// 1. Create the HTTP server in setup mode IMMEDIATELY (serves frontend + setup endpoints)
	httpSrv := server.NewSetupServer(cfg)
	httpSrv.IsDesktop = true
	httpSrv.UpdateStatus = updater.NewUpdateStatus()

	// Start HTTP server so the frontend is available right away
	go func() {
		if err := httpSrv.Start(); err != nil && err != http.ErrServerClosed {
			applog.Errorf("cli", "HTTP server error: %v", err)
		}
	}()
	waitForServer(serverURL, 10*time.Second)

	// 2. Create native webview window and navigate to Svelte app (not splash)
	w := webview.New(false)
	defer w.Destroy()
	setupNativeMenu()
	installClipboardMonitor(w.Window())

	w.Bind("__saveFile", func(filename, content string) error {
		return nativeSaveFile(filename, content)
	})
	w.Init(`window.__isDesktopApp = true;`)

	w.SetTitle(appName)
	w.SetSize(1440, 900, webview.HintNone)
	w.SetSize(800, 600, webview.HintMin)
	w.Navigate(serverURL) // Svelte app loads immediately (shows onboarding or waiting screen)

	// Resources created by the background goroutine, protected by mutex
	var mu sync.Mutex
	var store interface{ Close() error }
	var keyStore *apikeys.Store
	var chCleanup func()

	// 3. Background goroutine: download ClickHouse, start it, transition server to ready
	go func() {
		setupErr := func() error {
			s, cleanup, managedCH, err := setupClickHouseWithProgress(cfg, cfg.ClickHouse.Database, httpSrv)
			if err != nil {
				return fmt.Errorf("ClickHouse setup: %w", err)
			}

			httpSrv.SetDownloadProgress(server.SetupProgress{Stage: "local_storage"})
			ks, err := apikeys.NewStore(cfg.Server.SQLitePath)
			if err != nil {
				s.Close()
				cleanup()
				return fmt.Errorf("opening SQLite store: %w", err)
			}

			// Transition: wire store, keyStore, manager — server leaves setup mode
			httpSrv.SetDownloadProgress(server.SetupProgress{Stage: "finalizing"})
			httpSrv.TransitionToReady(s, ks)
			applog.Init(s)

			// Wire backup options
			chDataDir := cfg.ClickHouse.DataDir
			if chDataDir == "" {
				chDataDir = chmanaged.DefaultDataDir()
			}
			backupDir := filepath.Join(dataDir, "backups")
			configPath := viper.ConfigFileUsed()

			httpSrv.BackupOpts = &backup.BackupOptions{
				DataDir:    chDataDir,
				SQLitePath: cfg.Server.SQLitePath,
				ConfigPath: configPath,
				BackupDir:  backupDir,
			}

			// Wire ClickHouse stop/start for backup/restore
			if managedCH != nil {
				httpSrv.StopClickHouse = func() {
					managedCH.Stop()
				}
				httpSrv.StartClickHouse = func() error {
					ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
					defer cancel()
					return managedCH.Restart(ctx)
				}
			}

			// Background update check (5s after startup)
			go func() {
				time.Sleep(5 * time.Second)
				applog.Info("cli", "Checking for updates...")
				httpSrv.UpdateStatus.Check()
				snap := httpSrv.UpdateStatus.Snapshot()
				if snap.Available {
					applog.Infof("cli", "Update available: %s -> %s", snap.CurrentVersion, snap.LatestVersion)
				} else if snap.Error != "" {
					applog.Warnf("cli", "Update check error: %s", snap.Error)
				} else {
					applog.Info("cli", "Application is up to date.")
				}
			}()

			// Store references for shutdown
			mu.Lock()
			store = s
			keyStore = ks
			chCleanup = cleanup
			mu.Unlock()

			return nil
		}()

		if setupErr != nil {
			applog.Errorf("cli", "Setup failed: %v", setupErr)
			httpSrv.SetSetupError(setupErr)
		}
	}()

	// Run the webview event loop (blocks until window is closed)
	w.Run()

	// Clean shutdown after window is closed
	mu.Lock()
	localStore := store
	localKeyStore := keyStore
	localCleanup := chCleanup
	mu.Unlock()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	httpSrv.Stop(shutdownCtx)

	if localKeyStore != nil {
		localKeyStore.Close()
	}
	if localStore != nil {
		localStore.Close()
	}
	if localCleanup != nil {
		localCleanup()
	}

	return nil
}

// setupClickHouseWithProgress wraps setupClickHouse and reports download progress to the server.
func setupClickHouseWithProgress(cfg *config.Config, connectDB string, srv *server.Server) (*storage.Store, func(), *chmanaged.ManagedServer, error) {
	return setupClickHouseWithCb(cfg, connectDB, func(p chmanaged.DownloadProgress) {
		srv.SetDownloadProgress(server.SetupProgress{
			Stage:           "downloading",
			Percent:         p.Percent,
			BytesDownloaded: p.BytesDownloaded,
			TotalBytes:      p.TotalBytes,
		})
	}, func(stage string) {
		srv.SetDownloadProgress(server.SetupProgress{Stage: stage})
	})
}

// setupClickHouseWithCb is like setupClickHouse but passes a download progress callback.
func setupClickHouseWithCb(
	cfg *config.Config,
	connectDB string,
	onProgress func(chmanaged.DownloadProgress),
	onStage func(string),
) (*storage.Store, func(), *chmanaged.ManagedServer, error) {
	noop := func() {}

	onStage("detecting")
	mode := cfg.ClickHouse.Mode
	if mode == "" {
		mode = detectMode(cfg)
	}

	var host, username, password string
	var port int
	cleanup := noop
	var managed *chmanaged.ManagedServer

	switch mode {
	case "external":
		onStage("connecting_clickhouse")
		applog.Infof("cli", "Using external ClickHouse at %s:%d", cfg.ClickHouse.Host, cfg.ClickHouse.Port)
		host = cfg.ClickHouse.Host
		port = cfg.ClickHouse.Port
		username = cfg.ClickHouse.Username
		password = cfg.ClickHouse.Password

	case "managed":
		dataDir := cfg.ClickHouse.DataDir
		if dataDir == "" {
			dataDir = chmanaged.DefaultDataDir()
		}

		binaryPath := chmanaged.FindBinary(cfg.ClickHouse.BinaryPath, dataDir)
		if binaryPath == "" {
			onStage("downloading")
			applog.Info("cli", "No ClickHouse binary found, downloading...")
			var err error
			binaryPath, err = chmanaged.DownloadBinary(dataDir, onProgress)
			if err != nil {
				return nil, noop, nil, fmt.Errorf("downloading ClickHouse: %w", err)
			}
		}

		srv := chmanaged.NewManagedServer(dataDir)
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		onStage("starting_clickhouse")
		if err := srv.Start(ctx, binaryPath); err != nil {
			return nil, noop, nil, fmt.Errorf("starting managed ClickHouse: %w", err)
		}

		host = "127.0.0.1"
		port = srv.TCPPort()
		username = "default"
		password = ""
		cleanup = func() { srv.Stop() }
		managed = srv

	default:
		return nil, noop, nil, fmt.Errorf("unknown clickhouse.mode: %q", mode)
	}

	// Auto-migrate
	if connectDB != "default" {
		onStage("migrating")
		initStore, err := storage.NewStore(host, port, "default", username, password)
		if err != nil {
			cleanup()
			return nil, noop, nil, fmt.Errorf("connecting for migrations: %w", err)
		}
		applog.Info("cli", "Running auto-migrations...")
		if err := initStore.Migrate(context.Background()); err != nil {
			initStore.Close()
			cleanup()
			return nil, noop, nil, fmt.Errorf("auto-migration: %w", err)
		}
		initStore.Close()
	}

	onStage("connecting_database")
	store, err := storage.NewStore(host, port, connectDB, username, password)
	if err != nil {
		cleanup()
		return nil, noop, nil, fmt.Errorf("connecting to ClickHouse: %w", err)
	}

	if connectDB == "default" {
		onStage("migrating")
		applog.Info("cli", "Running migrations...")
		if err := store.Migrate(context.Background()); err != nil {
			store.Close()
			cleanup()
			return nil, noop, nil, fmt.Errorf("migration: %w", err)
		}
		applog.Info("cli", "Migrations complete.")
	}

	return store, cleanup, managed, nil
}

func waitForServer(url string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url + "/api/health")
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	applog.Warn("cli", "Server may not be ready")
}

// findFreePort asks the OS for an available port.
func findFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port, nil
}

// appDataDir returns ~/Library/Application Support/CrawlObserver (macOS) or equivalent,
// creating it if needed.
func appDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Library", "Application Support", "CrawlObserver")
	return dir, os.MkdirAll(dir, 0755)
}
