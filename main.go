// Command task120-spc serves the statistical-process-control HTTP API backed by
// SQLite, and provides a --smoke-test that exercises the full contract (three
// chart types, control-limit recompute, exclude/restore recompute, Westgard
// rules, capability estimable & not-estimable, one-sided spec, subgroup-size
// guard, p-chart bounds, rule toggles, restart recovery, consistency check,
// frontend page) without real-time sleeps.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"task120-spc/internal/chart"
	"task120-spc/internal/httpapi"
	"task120-spc/internal/selfcheck"
	"task120-spc/internal/store"
	"task120-spc/internal/webfs"
)

// webFS is the embedded static frontend (native HTML/CSS/JS, no build step).
// The embed lives in package webfs so the selfcheck can serve the same page.
var webFS = http.FS(webfs.FS())

// DefaultAdminToken protects admin endpoints. Override with SPC_ADMIN_TOKEN.
const DefaultAdminToken = "admin-secret"

func main() {
	smoke := flag.Bool("smoke-test", false, "run self-check and exit")
	dbPath := flag.String("db", "spc.db", "SQLite database file path")
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	if *smoke {
		if err := selfcheck.Run(); err != nil {
			fmt.Println("smoke-test: FAIL:", err)
			osExit(1)
		}
		fmt.Println("smoke-test: ok")
		osExit(0)
	}

	adminToken := os.Getenv("SPC_ADMIN_TOKEN")
	if adminToken == "" {
		adminToken = DefaultAdminToken
	}

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	svc := httpapi.Services{Chart: chart.New(st)}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           httpapi.NewMux(svc, adminToken, webFS),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		log.Printf("spc %s listening on %s (db=%s)", httpapi.Version, *addr, *dbPath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

// osExit is indirected so tests can substitute it; in production it is os.Exit.
var osExit = os.Exit
