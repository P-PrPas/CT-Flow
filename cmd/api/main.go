// Command api is CT-Flow's HTTP backend.
//
// During the port (docs/REFACTOR_PLAN.md phase 2) it is a strangler: routes
// implemented here are served here, and everything else is proxied to the
// FastAPI service still running behind it. That is what keeps the application
// working at every commit -- and what makes a rollback one deleted line in
// routes() rather than a revert and a redeploy.
//
// Set LEGACY_URL to the FastAPI service. Once nothing needs proxying, drop it
// and the fallback goes with it.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"

	"github.com/P-PrPas/CT-Flow/internal/api"
	"github.com/P-PrPas/CT-Flow/internal/auth"
	"github.com/P-PrPas/CT-Flow/internal/config"
	"github.com/P-PrPas/CT-Flow/internal/models"
	"github.com/P-PrPas/CT-Flow/internal/store"
	"github.com/P-PrPas/CT-Flow/internal/vpe"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg := config.Load()
	catalogPath := env("MODELS_CATALOG", "backend/models.json")
	catalog, err := models.Load(catalogPath, cfg.ModelsDir)
	if err != nil {
		// Fail at startup, not at the first request: a missing or malformed
		// catalog means GET /api/config cannot answer, and that is the one call
		// the UI needs before it can render anything at all.
		log.Error("cannot load the model catalog", "path", catalogPath, "err", err)
		os.Exit(1)
	}

	ctx := context.Background()
	db, err := store.Open(ctx, env("DATABASE_URL",
		"postgresql://labeltool:labeltool@localhost:5432/labeltool"))
	if err != nil {
		log.Error("cannot reach postgres", "err", err)
		os.Exit(1)
	}
	defer db.Close()
	// Idempotent, so it runs on every boot rather than needing a migration step
	// -- the same thing the FastAPI startup hook did.
	schemaPath := env("SCHEMA_PATH", "backend/schema.sql")
	if err := db.InitSchema(ctx, schemaPath); err != nil {
		log.Error("cannot apply the schema", "path", schemaPath, "err", err)
		os.Exit(1)
	}

	srv := &api.Server{
		Cfg: cfg, Catalog: catalog, Auth: auth.New(), Log: log,
		Store: db,
		VPE:   vpe.New(env("VPE_URL", "http://127.0.0.1:8001")),
	}

	addr := ":" + env("PORT", "8000")
	log.Info("starting", "addr", addr, "mode", cfg.Mode, "models", cfg.ModelsDir,
		"legacy", os.Getenv("LEGACY_URL"))

	server := &http.Server{
		Addr:              addr,
		Handler:           srv.RequireLogin(routes(srv, log)),
		ReadHeaderTimeout: 15 * time.Second,
		// No WriteTimeout: score/evaluate/autolabel are long inference passes
		// and their poll responses are cheap, but a large image download over a
		// slow link is legitimately slow. Idle connections are still reaped.
		IdleTimeout: 120 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

// routes registers what Go serves. Everything unregistered falls through to "/"
// and is proxied, so removing a line here rolls that endpoint back to Python
// without touching anything else.
func routes(s *api.Server, log *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /api/config", s.Handle(s.GetConfig))
	mux.Handle("GET /api/browse", s.Handle(s.Browse))
	mux.Handle("GET /api/image", s.Handle(s.GetImage))

	mux.Handle("POST /api/session", s.Handle(s.OpenSession))
	mux.Handle("GET /api/boxes", s.Handle(s.GetBoxes))

	mux.Handle("POST /api/testset/import", s.Handle(s.TestsetImport))
	mux.Handle("POST /api/testset/remove", s.Handle(s.TestsetRemove))
	mux.Handle("POST /api/testset/label", s.Handle(s.TestsetLabel))

	mux.Handle("GET /api/history", s.Handle(s.GetHistory))
	mux.Handle("POST /api/history", s.Handle(s.AddHistory))
	mux.Handle("DELETE /api/history", s.Handle(s.DeleteHistory))
	mux.Handle("GET /api/events", s.Handle(s.GetEvents))
	mux.Handle("POST /api/events", s.Handle(s.AddEvent))

	mux.Handle("GET /api/auth/me", s.Handle(s.AuthMe))
	mux.Handle("POST /api/auth/login", s.Handle(s.AuthLogin))
	mux.Handle("POST /api/auth/logout", s.Handle(s.AuthLogout))

	mux.Handle("/", legacy(log))
	return mux
}

func legacy(log *slog.Logger) http.Handler {
	target := os.Getenv("LEGACY_URL")
	if target == "" {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"detail":"not found"}`, http.StatusNotFound)
		})
	}
	u, err := url.Parse(target)
	if err != nil {
		log.Error("LEGACY_URL is not a URL", "value", target, "err", err)
		os.Exit(1)
	}
	proxy := httputil.NewSingleHostReverseProxy(u)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Error("proxy to the legacy service failed", "path", r.URL.Path, "err", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"detail":"backend unavailable"}`))
	}
	// FlushInterval -1 streams responses through as they arrive rather than
	// buffering: the poll loop wants each /api/jobs response immediately, and
	// GET /api/image can be tens of megabytes.
	proxy.FlushInterval = -1
	return proxy
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
