// Command api is CT-Flow's HTTP backend: every /api endpoint the frontend calls.
//
// Inference and the prompt bank are not here. They live in the Python sidecar
// (backend/inference/service.py) behind VPE_URL, because YOLOE's SAVPE head has no Go
// equivalent and the bank is a torch.save -- see docs/history/REFACTOR_PLAN.md.
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
	"os/signal"
	"syscall"
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

	// Cancelled on SIGINT/SIGTERM, which is what `docker compose down` and a
	// redeploy send. Used only for shutdown -- startup work below wants a
	// context that a stray Ctrl-C during boot does not half-cancel.
	sigCtx, stopSignals := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	ctx := context.Background()
	oidcAuth, err := auth.NewOIDC(ctx,
		os.Getenv("OAUTH_CLIENT_ID"), os.Getenv("OAUTH_CLIENT_SECRET"),
		os.Getenv("OAUTH_ENDPOINT"), os.Getenv("FRONTEND_URL"),
	)
	if err != nil {
		log.Error("cannot configure OIDC", "err", err)
		os.Exit(1)
	}
	// Sign-in is mandatory (T-27). It used to be optional, which meant a
	// deployment that simply forgot to set these variables served every endpoint
	// to anyone who could reach it and reported nothing -- and now that projects
	// carry an owner and every box carries an author, an unauthenticated server
	// would also record every one of them as nobody. Refusing to start is the
	// same contract docker-compose.yml already applies to POSTGRES_PASSWORD.
	if oidcAuth == nil && !auth.Enabled() {
		log.Error("refusing to start without a way to sign in: " +
			"set OAUTH_CLIENT_ID, OAUTH_CLIENT_SECRET, OAUTH_ENDPOINT and FRONTEND_URL " +
			"for company OIDC, or LABEL_TOOL_USERS (see -hash-password) for local " +
			"accounts. See docs/PHASE2_WORKSPACE.md T-27")
		os.Exit(1)
	}
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
		Cfg: cfg, Catalog: catalog, Auth: auth.New(), OIDC: oidcAuth, Log: log,
		Store: db,
		VPE:   vpe.New(env("VPE_URL", "http://127.0.0.1:8001")),
		Jobs:  jobs.NewTracker(),
	}

	addr := ":" + env("PORT", "8000")
	log.Info("starting", "addr", addr, "root", cfg.VMDataRoot, "models", cfg.ModelsDir)

	server := &http.Server{
		Addr:              addr,
		Handler:           srv.RequireLogin(routes(srv)),
		ReadHeaderTimeout: 15 * time.Second,
		// No WriteTimeout: score/evaluate/autolabel are long inference passes
		// and their poll responses are cheap, but a large image download over a
		// slow link is legitimately slow. Idle connections are still reaped.
		IdleTimeout: 120 * time.Second,
	}
	// ListenAndServe off the main goroutine so the signal wait below is what the
	// process blocks on, and a listen failure still exits rather than hanging.
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.ListenAndServe() }()

	select {
	case err := <-serveErr:
		if err != nil && err != http.ErrServerClosed {
			log.Error("server stopped", "err", err)
			os.Exit(1)
		}
	case <-sigCtx.Done():
		// Stop accepting, then let the requests already in flight answer. 8s and
		// not longer: Docker's default stop grace period is 10 seconds, and a
		// drain that outlasts it is not a graceful shutdown, it is a SIGKILL
		// with extra steps. Requests here are short anyway -- the long work is a
		// background job the poller checks on, not a held-open request.
		//
		// Those background jobs do NOT survive this, and deliberately are not
		// waited for -- a pass can legitimately have hours left, and the job
		// tracker they report into is in this process's memory anyway (see
		// internal/platform/jobs). A restart mid-pass has always meant the UI
		// polls an id that no longer exists and gets a 404; making shutdown
		// clean does not change that, and pretending to wait would just turn a
		// redeploy into a SIGKILL after the grace period.
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Error("shutdown did not finish cleanly", "err", err)
		}
	}
	// db.Close is the deferred call above, which os.Exit used to skip entirely.
}

func routes(s *httpapi.Server) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /api/config", s.Handle(s.GetConfig))
	mux.Handle("GET /api/browse", s.Handle(s.Browse))
	mux.Handle("GET /api/image", s.Handle(s.GetImage))

	mux.Handle("GET /api/projects", s.Handle(s.ListProjects))
	mux.Handle("POST /api/projects", s.Handle(s.CreateProject))
	mux.Handle("GET /api/projects/{id}", s.Handle(s.GetProject))
	mux.Handle("PATCH /api/projects/{id}", s.Handle(s.UpdateProject))
	mux.Handle("DELETE /api/projects/{id}", s.Handle(s.DeleteProject))

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
	mux.Handle("GET /api/public/login/redirect", s.Handle(s.OIDCRedirect))
	mux.Handle("POST /api/public/login/callback", s.Handle(s.OIDCCallback))

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
