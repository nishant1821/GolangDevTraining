// Package main is the entry point of the Pulse binary.
//
// cmd/server/main.go is the "wiring" file — it creates all the pieces
// (config, DB, repos, services, scheduler, pool) and connects them together.
// It should contain as LITTLE logic as possible; it just assembles and starts.
//
// DEPENDENCY INJECTION BY HAND — what that means:
//   Go has no DI framework. We create every dependency explicitly and pass it
//   into the thing that needs it. The order is always:
//     outermost layer needs → innermost layer first.
//
//   config  (needs nothing)
//     ↓
//   db      (needs config.DatabaseURL)
//     ↓
//   repos   (need db)
//     ↓
//   service (needs repos)
//     ↓
//   checker (needs config.CheckTimeout)
//     ↓
//   scheduler+pool (need ctx, service, checker, config)
//     ↓
//   http server (needs service — handlers added in Stage 7)
//
// Python analogy: if __name__ == "__main__": app = create_app(config)
// Node.js analogy: const app = buildApp({ config, db, repos, services })
package main

import (
	"context"           // context.WithCancel, context.WithTimeout
	"encoding/json"     // json.NewEncoder — health handler response
	"net/http"          // http.Server, http.ErrServerClosed
	"os"                // os.Exit, os.Stdout
	"os/signal"         // signal.Notify
	"syscall"           // syscall.SIGTERM, syscall.SIGINT
	"time"              // time.Now, time.Second

	"github.com/go-chi/chi/v5"
	chiMW "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"        // zerolog.New — structured logger
	"github.com/rs/zerolog/log"    // log.Logger global — used by zerolog internally
	"gorm.io/gorm"                 // *gorm.DB — health check pings the underlying sql.DB

	"github.com/nishantks908/pulse/config"
	"github.com/nishantks908/pulse/internal/checker"          // checker.NewHTTPChecker
	"github.com/nishantks908/pulse/internal/domain"            // domain.Monitor — for due() closure
	"github.com/nishantks908/pulse/internal/handler"           // handler.AuthHandler, handler.MonitorHandler
	"github.com/nishantks908/pulse/internal/middleware"        // middleware.RequestID, middleware.Auth, middleware.RateLimit
	"github.com/nishantks908/pulse/internal/monitor"           // monitor.Scheduler, monitor.RunPool
	"github.com/nishantks908/pulse/internal/notifier"          // notifier.NewLogNotifier — alert on DOWN
	"github.com/nishantks908/pulse/internal/platform/cache"   // cache.New — Redis wrapper
	"github.com/nishantks908/pulse/internal/platform/database"
	"github.com/nishantks908/pulse/internal/repository"       // repository.New*, repository.NewTransactor
	"github.com/nishantks908/pulse/internal/service"          // service.NewMonitorService, service.NewUserService
)

func main() {

	// ── 1. LOGGER ────────────────────────────────────────────────────────────
	// zerolog.ConsoleWriter formats logs as pretty, colour-coded text in dev.
	// In production you'd switch to JSON: zerolog.New(os.Stdout).
	//
	// zerolog.New returns a zerolog.Logger (value type, copy-safe).
	// .With().Timestamp().Logger() adds the "time" field to every log line.
	//
	// Python: logging.basicConfig(format="%(asctime)s %(levelname)s %(message)s")
	// Node.js: const log = pino({ transport: { target: 'pino-pretty' } })
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).
		With().Timestamp().Logger()

	// Set the global zerolog logger so zerolog/log.* calls work anywhere.
	// We still pass `logger` explicitly to middleware — globals are fine for
	// process-level init but explicit injection is better for libraries.
	log.Logger = logger

	logger.Info().Msg("Pulse starting…")

	// ── 2. CONFIG ────────────────────────────────────────────────────────────
	// Read ALL configuration from environment variables at startup.
	// We call Load() ONCE here and pass cfg down to everything that needs it.
	// "One call, many uses" — we never call config.Load() inside a handler.
	//
	// Python: settings = Settings()   # pydantic BaseSettings
	// Node.js: const config = require('./config')
	cfg := config.Load()

	// ── 3. DATABASE ──────────────────────────────────────────────────────────
	// Connect to Postgres and run AutoMigrate to sync the schema.
	// We crash fast on failure — no DB means the whole service is broken.
	//
	// Python: engine = create_engine(url); Base.metadata.create_all(engine)
	// Node.js: await sequelize.authenticate(); await sequelize.sync({ alter: true })
	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		logger.Fatal().Err(err).Msg("db connect failed")
	}
	if err := database.AutoMigrate(db); err != nil {
		logger.Fatal().Err(err).Msg("db migrate failed")
	}
	logger.Info().Msg("database connected and migrated")

	// ── 3. REDIS CACHE ───────────────────────────────────────────────────────
	// Redis is optional — if it's unreachable, we log a warning and run
	// without caching (DB-only). Service and rate-limiter handle nil gracefully.
	//
	// Graceful degradation rule:
	//   DB down   → Fatal (can't serve any data without DB)
	//   Redis down → Warn  (cache miss every time, but service still works)
	//
	// Python: cache = redis.from_url(url) if REDIS_URL else None
	// Node.js: const cache = REDIS_URL ? new Redis(REDIS_URL) : null
	var cacheClient *cache.Cache
	if c, err := cache.New(cfg.RedisURL); err != nil {
		logger.Warn().Err(err).Msg("redis unavailable — running without cache")
	} else {
		cacheClient = c
		logger.Info().Str("url", cfg.RedisURL).Msg("redis connected")
	}

	// ── 4. REPOSITORIES ──────────────────────────────────────────────────────
	monitorRepo  := repository.NewMonitorRepository(db)
	checkRepo    := repository.NewCheckRepository(db)
	incidentRepo := repository.NewIncidentRepository(db)

	// ── 4b. TRANSACTOR ───────────────────────────────────────────────────────
	// Transactor wraps db.Transaction so the service can run multiple repo
	// writes atomically WITHOUT importing *gorm.DB.
	//
	// Stage 9: used by RecordOutcome to wrap (incident CREATE + status UPDATE)
	// in one atomic boundary — either both commit or both roll back.
	//
	// Python: session.begin() context manager
	// Node.js: knex.transaction(async trx => { ... })
	txr := repository.NewTransactor(db)

	// ── 4c. NOTIFIER ─────────────────────────────────────────────────────────
	// Notifier is called AFTER the incident transaction commits (Stage 9).
	// LogNotifier writes a structured zerolog error line — swap for
	// EmailNotifier/SlackNotifier in production without touching service code.
	//
	// Python: alerter = LogNotifier(logger)
	// Node.js: const alerter = new LogNotifier(logger)
	alerter := notifier.NewLogNotifier(logger)

	// ── 5. MONITOR SERVICE ───────────────────────────────────────────────────
	// cacheClient is passed in — nil means "no caching".
	// service.NewMonitorService guards every cache call with `if s.cache != nil`.
	//
	// Stage 9 additions: txr (Transactor) and alerter (Notifier) are injected.
	//
	// Python: svc = MonitorService(repos..., cache=cache_client, tx=txr, notifier=alerter)
	// Node.js: const svc = new MonitorService(repos, cacheClient, txr, alerter)
	svc := service.NewMonitorService(monitorRepo, checkRepo, incidentRepo, cacheClient, txr, alerter, logger)

	// ── 4b. USER SERVICE ─────────────────────────────────────────────────────
	// UserService handles registration + login.
	// It needs:
	//   - UserRepository  (interface — DB access)
	//   - jwtSecret       ([]byte — HMAC signing key from config)
	//   - jwtTTL          (how long tokens are valid, from config)
	//
	// Python:
	//   user_svc = UserService(user_repo, jwt_secret, jwt_expiry)
	// Node.js:
	//   const userSvc = new UserService(userRepo, jwtSecret, jwtExpiry)
	userRepo := repository.NewUserRepository(db)
	userSvc := service.NewUserService(userRepo, []byte(cfg.JWTSecret), cfg.JWTExpiry)

	// ── 5. HTTP CHECKER ──────────────────────────────────────────────────────
	// The checker performs actual HTTP probes.
	// cfg.CheckTimeout is the per-request deadline (e.g. 10 seconds).
	// We pass it here so the service never has to deal with timeouts directly.
	//
	// Python: checker = HttpChecker(timeout=cfg.check_timeout)
	// Node.js: const checker = new HttpChecker({ timeout: cfg.checkTimeout })
	httpChecker := checker.NewHTTPChecker(cfg.CheckTimeout)

	// ── 6. PIPELINE CONTEXT ───────────────────────────────────────────────────
	// The pipeline (Scheduler + Pool) needs its OWN context, separate from the
	// HTTP server shutdown context.
	//
	// WHY separate?
	//   - HTTP server shutdown context has a 10-second deadline.
	//   - We cancel the pipeline on SIGTERM (before the deadline) so in-flight
	//     checks can finish cleanly BEFORE we stop the HTTP server.
	//   - If we shared one context, pipeline and HTTP server would race.
	//
	// Python: pipeline_task = asyncio.create_task(run_pipeline(...))
	// Node.js: const pipelineAbort = new AbortController()
	pipelineCtx, pipelineCancel := context.WithCancel(context.Background())
	// defer ensures pipeline stops even if main() returns unexpectedly early.
	defer pipelineCancel()

	// ── 7. SCHEDULER + POOL ───────────────────────────────────────────────────
	// Wire the full Scheduler → Pool pipeline.
	//
	// `due` is a closure that the Scheduler calls on every tick.
	// In production it hits the DB. This closure is the ONLY place that
	// connects the Scheduler (monitor package) to the DB (repository package).
	//
	// Note: we pass context.Background() to ListDue, not pipelineCtx —
	// the DB query should complete even when we're shutting down, so we get
	// results for any tick that already fired.
	due := func() []domain.Monitor {
		monitors, err := monitorRepo.ListDue(context.Background(), time.Now())
		if err != nil {
			// Structured log replaces fmt.Fprintf(os.Stderr, ...).
			// zerolog writes JSON in production; ConsoleWriter formats it for dev.
			// The "error" field is queryable in log aggregators (Datadog, Loki).
			logger.Error().Err(err).Msg("scheduler: list due monitors failed")
			return nil
		}
		return monitors
	}

	// Scheduler ticks every cfg.ScheduleInterval, calls due(), pushes Jobs.
	// Returns a jobs channel that is closed when pipelineCtx is cancelled.
	jobs := monitor.Scheduler(pipelineCtx, cfg.ScheduleInterval, due)

	// RunPool starts cfg.WorkerCount goroutines that consume jobs, call
	// httpChecker.Check(), and emit Outcomes.
	// Returns an outcomes channel closed when all workers finish.
	outcomes := monitor.RunPool(pipelineCtx, httpChecker, cfg.WorkerCount, jobs)

	// ── 8. OUTCOME RECORDING GOROUTINE ───────────────────────────────────────
	// A single goroutine drains the outcomes channel.
	// For each Outcome it calls svc.RecordOutcome which:
	//   1. Saves the Check row to DB.
	//   2. Updates monitor.NextCheckAt.
	//   3. Opens or resolves an Incident.
	//
	// This goroutine exits automatically when outcomes is closed
	// (which happens after all workers finish, triggered by pipelineCancel).
	//
	// Python:
	//   async for outcome in outcomes_queue:
	//       await svc.record_outcome(outcome)
	// Node.js:
	//   for await (const outcome of outcomesStream) {
	//       await svc.recordOutcome(outcome)
	//   }
	go func() {
		for o := range outcomes {
			if err := svc.RecordOutcome(context.Background(), o); err != nil {
				logger.Error().Err(err).Msg("record outcome failed")
			}
		}
		logger.Info().Msg("outcome recorder stopped")
	}()

	logger.Info().
		Str("interval", cfg.ScheduleInterval.String()).
		Int("workers", cfg.WorkerCount).
		Msg("pipeline started")

	// ── 9. HANDLERS ──────────────────────────────────────────────────────────
	authH    := handler.NewAuthHandler(userSvc)
	monitorH := handler.NewMonitorHandler(svc)

	// ── 10. ROUTER + MIDDLEWARE ───────────────────────────────────────────────
	// Middleware ordering (outermost → innermost on the way IN):
	//
	//   RequestID  → generates UUID, stores in ctx, sets X-Request-ID header
	//   Logger     → reads UUID from ctx, logs after handler returns (LIFO)
	//   Recoverer  → catches panics from handlers, returns 500 instead of crash
	//
	// On the way OUT the order reverses (LIFO stack):
	//   Recoverer → Logger (captures status + latency) → RequestID
	//
	// WHY RequestID must be BEFORE Logger?
	//   Logger.NewLogger calls RequestIDFromCtx() to include the ID in the log line.
	//   If RequestID runs after Logger, the context doesn't have the UUID yet —
	//   the log line would show an empty request_id.
	//
	// Python/FastAPI:
	//   app.add_middleware(RequestIDMiddleware)   ← outermost (added last, runs first)
	//   app.add_middleware(LoggingMiddleware)
	// Node.js/Express:
	//   app.use(requestIdMiddleware)  ← first app.use = first to run
	//   app.use(loggerMiddleware)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)               // 1st: generate UUID → context + header
	r.Use(middleware.NewLogger(logger))        // 2nd: structured request log (reads UUID)
	r.Use(chiMW.Recoverer)                     // 3rd: panic → 500, never crash

	// ── 11. ROUTES ────────────────────────────────────────────────────────────
	//
	// All routes live under /api so versioning is easy later (/api/v2/...).
	//
	// Public (no JWT):
	//   GET  /health
	//   POST /api/auth/register
	//   POST /api/auth/login
	//
	// Protected (JWT in Authorization: Bearer header):
	//   POST   /api/monitors
	//   GET    /api/monitors
	//   GET    /api/monitors/{id}
	//   PATCH  /api/monitors/{id}/pause
	//   PATCH  /api/monitors/{id}/resume
	//   DELETE /api/monitors/{id}
	//   GET    /api/monitors/{id}/checks   ← paginated probe history

	r.Get("/health", newHealthHandler(db, cacheClient))

	r.Route("/api", func(r chi.Router) {
		// Rate limiting: 200 req/min per IP across all /api routes.
		// Applies BEFORE auth — even unauthenticated bursts (e.g., brute-force
		// on /auth/login) are throttled.
		//
		// cacheClient nil → RateLimit fails-open (no limiting, but no crash).
		r.Use(middleware.RateLimit(cacheClient, 200, time.Minute))

		r.Post("/auth/register", authH.Register)
		r.Post("/auth/login", authH.Login)

		r.Route("/monitors", func(r chi.Router) {
			r.Use(middleware.Auth([]byte(cfg.JWTSecret)))
			r.Post("/", monitorH.Create)
			r.Get("/", monitorH.List)
			r.Get("/{id}", monitorH.Get)
			r.Patch("/{id}/pause", monitorH.Pause)
			r.Patch("/{id}/resume", monitorH.Resume)
			r.Delete("/{id}", monitorH.Delete)
			r.Get("/{id}/checks", monitorH.Checks)
		})
	})

	// ── 11. HTTP SERVER ───────────────────────────────────────────────────────
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info().Str("addr", ":"+cfg.Port).Msg("HTTP server listening")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("server failed to start")
		}
	}()

	// ── 12. WAIT FOR SHUTDOWN SIGNAL ─────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	<-quit // BLOCKS here until SIGTERM or Ctrl+C

	logger.Info().Msg("shutdown signal received")

	// ── 13. GRACEFUL SHUTDOWN ─────────────────────────────────────────────────
	pipelineCancel()
	logger.Info().Msg("pipeline cancelled")

	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutCtx); err != nil {
		logger.Error().Err(err).Msg("shutdown timed out")
	} else {
		logger.Info().Msg("Pulse stopped cleanly")
	}
}

// newHealthHandler returns an http.HandlerFunc that pings Postgres and Redis
// and reports their status. Used by load balancers and k8s liveness probes.
//
// Response 200 — both dependencies reachable:
//   {"status":"ok","postgres":"ok","redis":"ok"}
//
// Response 503 — one or more dependencies down:
//   {"status":"degraded","postgres":"ok","redis":"down"}
//
// WHY 503 (not 200 with an error field)?
//   Load balancers and orchestrators (k8s, ECS) look at the HTTP status code
//   to decide whether to route traffic to this instance.
//   A 503 tells them "take this pod out of rotation — it can't serve requests."
//   A 200 with {"status":"degraded"} would be silently ignored.
//
// Redis nil (not configured) → reported as "disabled", does NOT trigger 503.
//   Service degrades gracefully when Redis is unavailable (Stage 8 policy).
//
// Python/Flask: @app.route("/health") def health(): return jsonify(checks), 503 if bad
// Node.js/Express: app.get("/health", (req, res) => res.status(ok ? 200 : 503).json(checks))
func newHealthHandler(db *gorm.DB, c *cache.Cache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 3-second deadline — health checks must be fast.
		// A slow health check causes false negatives under load.
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		type response struct {
			Status   string `json:"status"`
			Postgres string `json:"postgres"`
			Redis    string `json:"redis"`
		}

		res := response{}
		code := http.StatusOK

		// ── Check Postgres ────────────────────────────────────────────────────
		// db.DB() returns the underlying *sql.DB from the standard library.
		// PingContext sends a cheap round-trip to verify the connection is alive.
		//
		// Python: engine.connect().execute("SELECT 1")
		// Node.js: await pool.query("SELECT 1")
		sqlDB, err := db.DB()
		if err != nil || sqlDB.PingContext(ctx) != nil {
			res.Postgres = "down"
			code = http.StatusServiceUnavailable // 503
		} else {
			res.Postgres = "ok"
		}

		// ── Check Redis ───────────────────────────────────────────────────────
		// c == nil means Redis was not configured at startup (graceful degrade).
		// "disabled" ≠ "down" — don't penalise a deliberately optional dependency.
		if c == nil {
			res.Redis = "disabled"
		} else if err := c.Ping(ctx); err != nil {
			res.Redis = "down"
			code = http.StatusServiceUnavailable
		} else {
			res.Redis = "ok"
		}

		if code == http.StatusOK {
			res.Status = "ok"
		} else {
			res.Status = "degraded"
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(res) //nolint:errcheck
	}
}
