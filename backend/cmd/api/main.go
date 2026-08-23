// Command api is CT-Flow's HTTP backend: every /api endpoint the frontend calls.
//
// Inference and the prompt bank are not here. They live in the Python sidecar
// (backend/inference/service.py) behind VPE_URL, because YOLOE's SAVPE head has no Go
// equivalent and the bank is a torch.save -- see docs/REFACTOR_PLAN.md.
//
// The strangler proxy this started as is gone; it existed only to keep the
// application working while routes moved across one group at a time.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/P-PrPas/CT-Flow/backend/internal/infra/store"
	"github.com/P-PrPas/CT-Flow/backend/internal/infra/vpe"
	"github.com/P-PrPas/CT-Flow/backend/internal/platform/auth"
	"github.com/P-PrPas/CT-Flow/backend/internal/platform/config"
	"github.com/P-PrPas/CT-Flow/backend/internal/platform/jobs"
	"github.com/P-PrPas/CT-Flow/backend/internal/platform/models"
	"github.com/P-PrPas/CT-Flow/backend/internal/transport/httpapi"
)

func main() {
	// The only subcommand. LABEL_TOOL_USERS holds password hashes, and something
	// has to be able to produce one -- this replaces
	// `python -m backend.services.auth <name> <password>`, which went with the
	// FastAPI service.
	//
	//	docker compose run --rm api /app/api -hash-password alice 'their password'
	//
	// The plaintext is never stored or logged; only the hash is printed.
	hashUser := flag.String("hash-password", "",
		"print a LABEL_TOOL_USERS entry for this username, reading the password from the next argument")
	flag.Parse()
	if *hashUser != "" {
		if flag.NArg() != 1 {
			fmt.Fprintln(os.Stderr, "usage: api -hash-password <username> <password>")
			os.Exit(2)
		}
		fmt.Printf("%s:%s\n", *hashUser, auth.HashPassword(flag.Arg(0)))
		return
	}

	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg := config.Load()
	catalogPath := env("MODELS_CATALOG", "models.json")
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
	schemaPath := env("SCHEMA_PATH", "db/schema.sql")
	if err := db.InitSchema(ctx, schemaPath); err != nil {
		log.Error("cannot apply the schema", "path", schemaPath, "err", err)
		os.Exit(1)
	}

	srv := &httpapi.Server{
		Cfg: cfg, Catalog: catalog, Auth: auth.New(), Log: log,
		Store: db,
		VPE:   vpe.New(env("VPE_URL", "http://127.0.0.1:8001")),
		Jobs:  jobs.NewTracker(),
	}

	addr := ":" + env("PORT", "8000")
	log.Info("starting", "addr", addr, "mode", cfg.Mode, "models", cfg.ModelsDir)

	server := &http.Server{
		Addr:              addr,
		Handler:           srv.RequireLogin(routes(srv)),
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

func routes(s *httpapi.Server) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /api/config", s.Handle(s.GetConfig))
	mux.Handle("GET /api/browse", s.Handle(s.Browse))
	mux.Handle("GET /api/image", s.Handle(s.GetImage))

	mux.Handle("POST /api/session", s.Handle(s.OpenSession))
	mux.Handle("GET /api/boxes", s.Handle(s.GetBoxes))

	mux.Handle("POST /api/label", s.Handle(s.SaveLabel))
	mux.Handle("POST /api/relabel", s.Handle(s.Relabel))
	mux.Handle("POST /api/predict", s.Handle(s.Predict))
	mux.Handle("POST /api/upload", s.Handle(s.Upload))

	mux.Handle("POST /api/testset/import", s.Handle(s.TestsetImport))
	mux.Handle("POST /api/testset/remove", s.Handle(s.TestsetRemove))
	mux.Handle("POST /api/testset/label", s.Handle(s.TestsetLabel))

	mux.Handle("GET /api/jobs/{id}", s.Handle(s.GetJob))
	mux.Handle("POST /api/score", s.Handle(s.Score))
	mux.Handle("POST /api/evaluate", s.Handle(s.Evaluate))
	mux.Handle("POST /api/autolabel", s.Handle(s.Autolabel))
	mux.Handle("POST /api/reembed", s.Handle(s.Reembed))

	mux.Handle("GET /api/export", s.Handle(s.Export))

	mux.Handle("GET /api/history", s.Handle(s.GetHistory))
	mux.Handle("POST /api/history", s.Handle(s.AddHistory))
	mux.Handle("DELETE /api/history", s.Handle(s.DeleteHistory))
	mux.Handle("GET /api/events", s.Handle(s.GetEvents))
	mux.Handle("POST /api/events", s.Handle(s.AddEvent))

	mux.Handle("GET /api/auth/me", s.Handle(s.AuthMe))
	mux.Handle("POST /api/auth/login", s.Handle(s.AuthLogin))
	mux.Handle("POST /api/auth/logout", s.Handle(s.AuthLogout))

	// Anything else: JSON, not net/http's text 404, because lib/api.ts reads
	// `detail` off every failed response.
	mux.Handle("/", s.Handle(notFound))
	return mux
}

func notFound(http.ResponseWriter, *http.Request) error {
	return httpapi.ErrNotFound
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
