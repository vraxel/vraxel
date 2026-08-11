package main

import (
	"context"
	"flag"
	"io/fs"
	"os"
	"time"

	"vraxel.io/vraxel/app/vraxel-server/handler"
	"vraxel.io/vraxel/lib/apiserver"
	libaudit "vraxel.io/vraxel/lib/audit"
	"vraxel.io/vraxel/lib/buildinfo"
	"vraxel.io/vraxel/lib/config"
	"vraxel.io/vraxel/lib/httpserver"
	"vraxel.io/vraxel/lib/lflag"
	"vraxel.io/vraxel/lib/logger"
	"vraxel.io/vraxel/lib/oidc"
	"vraxel.io/vraxel/lib/profile"
	"vraxel.io/vraxel/lib/rest/filters"
	"vraxel.io/vraxel/lib/utils/procutil"

	localapis "vraxel.io/vraxel/app/vraxel-server/apis"
	"vraxel.io/vraxel/pkg/apis"
	"vraxel.io/vraxel/pkg/db"
	"vraxel.io/vraxel/pkg/db/migrations"
	"vraxel.io/vraxel/ui"
)

var (
	httpListenAddrs  = lflag.NewArrayString("httpListenerAddr", "The address to listen on for HTTP requests")
	useProxyProtocol = lflag.NewArrayBool("httpListenerAddr.useProxyProtocol", "Whether to use proxy protocol for connections accepted at the corresponding -httpListenAddr")
	configPath       = flag.String("config", "/etc/vraxel/config.yaml", "Path to the YAML configuration file")
)

const (
	VraxelAPIServer = "vraxel-server"
)

func main() {
	defer profile.Profile().Stop()

	// 1. Initialize
	flag.CommandLine.SetOutput(os.Stdout)
	flag.Usage = usage
	lflag.Parse()
	initCLIFlags()
	buildinfo.Init()
	logger.Init()

	ctx := procutil.SetupSignalContext()
	cfg := loadConfig()

	// Database
	database, err := db.NewDB(ctx, dbConfigFrom(cfg))
	if err != nil {
		logger.Fatalf("cannot create database: %v", err)
	}

	// Run database migrations
	if err := db.Migrate(ctx, database.GetPool(), migrations.FS); err != nil {
		logger.Fatalf("cannot run database migrations: %v", err)
	}
	logger.Infof("database migrations applied")

	// Hot-reload
	config.RegisterReloadCallback(func(c *config.Config) {
		logger.Reload(c.Logger.Level, c.Logger.Format)
		if err := database.Reload(ctx, dbConfigFrom(c)); err != nil {
			logger.Errorf("failed to reload database config: %v", err)
		}
	})
	go watchSIGHUP()

	// Audit writer (async, must start before HTTP server).
	auditWriter := apis.NewAuditWriter(database)
	auditWriter.Start(ctx)

	// API modules (admin + role seeding)
	apisResult := apis.NewModules(ctx, database)

	// OIDC provider (depends on iam module stores)
	oidcProvider := apis.NewOIDCProvider(database, apisResult, &cfg.OIDC)

	// RBAC authorizer (also Subscribes the last channel on the
	// pgnotify multiplexer, so Mux.Start must come after this).
	authorizer := apis.NewAuthorizer(apisResult)

	// Start the cross-instance LISTEN multiplexer now that every
	// module has Subscribed. One dedicated PG connection serves all
	// channels for the lifetime of ctx; cancellation closes it.
	apisResult.Mux.Start(ctx)

	// 2. Start HTTP server
	listenAddrs := *httpListenAddrs
	if len(listenAddrs) == 0 {
		listenAddrs = []string{":9099"}
	}

	startTime := time.Now()

	rootHandler := buildRootHandler(ctx, cfg, database, apisResult, oidcProvider, authorizer, auditWriter)

	go httpserver.Serve(listenAddrs, rootHandler, httpserver.ServerOptions{
		UseProxyProtocol: useProxyProtocol,
	})
	logger.Infof("vraxel-server started at %q in %.3f seconds", listenAddrs, time.Since(startTime).Seconds())

	// 3. Wait for shutdown signal
	<-ctx.Done()
	gracefulShutdown(listenAddrs, auditWriter, database)
}

// buildRootHandler builds the API server handler and the
// embedded-frontend-backed root handler. Fatals on any construction
// error (handler wiring or embedded asset load).
func buildRootHandler(
	ctx context.Context,
	cfg *config.Config,
	database *db.DB,
	apisResult apis.Result,
	oidcProvider *oidc.Provider,
	authorizer *filters.Authorizer,
	auditWriter *libaudit.Writer,
) httpserver.RequestHandler {
	// The apiserver hosts every API module; route-level authorization
	// needs the RBAC checker, which NewAuthorizer produced just above.
	srv := apiserver.New(apiserver.Config{
		Checker:       authorizer.Checker,
		AuditLogger:   auditWriter,
		SkipPermCodes: []string{filters.PermListCode},
	})
	for _, register := range apisResult.Registrars {
		register(srv)
	}
	if err := apisResult.SyncPermissions(ctx, srv); err != nil {
		logger.Fatalf("cannot sync permissions: %v", err)
	}
	logger.Infof("apiserver serving modules: %v", srv.Modules())

	apiHandler := handler.NewAPIServerHandler(handler.APIServerConfig{
		Name:         VraxelAPIServer,
		OIDCProvider: oidcProvider,
		AuditLogger:  auditWriter,
		Server:       srv,
	})

	distFS, err := fs.Sub(ui.DistFS, "dist")
	if err != nil {
		logger.Fatalf("cannot load embedded frontend: %v", err)
	}

	return handler.NewRootHandler(handler.RootHandlerConfig{
		APIHandler:  apiHandler,
		OIDCMux:     apis.NewOIDCMux(oidcProvider, auditWriter, apisResult, &cfg.OIDC, cfg.Server.ExternalURL),
		OpenAPISpec: localapis.OpenAPISpec,
		FrontendFS:  distFS,
		ReadinessChecks: []handler.ReadinessCheck{
			{Name: "database", Fn: func(ctx context.Context) error { return database.GetPool().Ping(ctx) }},
		},
	})
}

// gracefulShutdown stops HTTP, then flushes the audit writer and closes
// the database pool.
func gracefulShutdown(listenAddrs []string, auditWriter *libaudit.Writer, database *db.DB) {
	startTime := time.Now()

	if err := httpserver.Stop(listenAddrs); err != nil {
		logger.Fatalf("cannot stop the vraxel-server: %s", err)
	}

	auditWriter.Stop()
	database.Close()
	logger.Infof("successfully shut down vraxel-server in %.3f seconds", time.Since(startTime).Seconds())
}

// loadConfig loads configuration: file -> defaults -> env overrides -> CLI overrides.
func loadConfig() *config.Config {
	cfg, err := config.LoadFromFile(*configPath)
	if err != nil {
		logger.Fatalf("cannot load config from %q: %v", *configPath, err)
	}
	config.ApplyEnvOverrides(cfg)
	applyCLIOverrides(cfg)
	config.SetDefaults(cfg)
	if err := cfg.Server.Validate(); err != nil {
		logger.Fatalf("invalid server config: %v", err)
	}
	config.Set(cfg)
	applyTrustedProxies(cfg)
	logger.Infof("configuration loaded from %q", *configPath)
	return cfg
}

// applyTrustedProxies pushes server.trustedProxies into the client-IP
// resolver. Validate() already rejected unparseable entries, so an error
// here is impossible; log rather than fatal on reload paths.
func applyTrustedProxies(cfg *config.Config) {
	prefixes, err := cfg.Server.ParseTrustedProxies()
	if err != nil {
		logger.Errorf("invalid server.trustedProxies: %v", err)
		return
	}
	libaudit.SetTrustedProxies(prefixes)
}

var cliFlags map[string]string

func initCLIFlags() {
	cliFlags = make(map[string]string)
	flag.Visit(func(f *flag.Flag) {
		cliFlags[f.Name] = f.Value.String()
	})
}

func applyCLIOverrides(cfg *config.Config) {
	for name, val := range cliFlags {
		switch name {
		case "loggerLevel":
			cfg.Logger.Level = val
		case "loggerFormat":
			cfg.Logger.Format = val
		}
	}
}

func dbConfigFrom(cfg *config.Config) db.Config {
	return db.Config{
		Host:     cfg.Database.Host,
		Port:     cfg.Database.Port,
		User:     cfg.Database.User,
		Password: cfg.Database.Password,
		DBName:   cfg.Database.DBName,
		SSLMode:  cfg.Database.SSLMode,
		MaxConns: cfg.Database.MaxConns,
	}
}

// watchSIGHUP listens for SIGHUP and reloads configuration.
func watchSIGHUP() {
	for range procutil.NewSighupChan() {
		logger.Infof("received SIGHUP, reloading configuration from %q", *configPath)
		newCfg, err := config.LoadFromFile(*configPath)
		if err != nil {
			logger.Errorf("failed to reload config: %v", err)
			continue
		}
		config.ApplyEnvOverrides(newCfg)
		applyCLIOverrides(newCfg)
		config.SetDefaults(newCfg)
		if err := newCfg.Server.Validate(); err != nil {
			logger.Errorf("rejected reload: invalid server config: %v", err)
			continue
		}
		config.Set(newCfg)
		applyTrustedProxies(newCfg)
		logger.Infof("configuration reloaded successfully")
	}
}

func usage() {
	const s = `
vraxel-server is an API platform skeleton.
`
	lflag.Usage(s)
}
