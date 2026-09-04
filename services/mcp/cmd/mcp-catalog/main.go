// Command mcp-catalog serves the mark8ly storefront catalog as an MCP
// server: five read-only tools (list/get products, list categories, list
// products by category, get branding), gated on a shared key, over
// streamable HTTP.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/mark8ly/mcp/internal/catalog"
	"github.com/mark8ly/mcp/internal/config"
	"github.com/mark8ly/mcp/internal/server"

	gsmcp "github.com/tesserix/go-shared/mcp"
	"github.com/tesserix/go-shared/mcp/observe"
)

// metricsServiceName labels every metric this process emits. It matches the
// name observe's own tests use and the estate registry's record for this
// connector.
const metricsServiceName = "mcp-catalog"

// shutdownTimeout bounds how long graceful shutdown waits for in-flight
// requests to drain once SIGTERM arrives.
const shutdownTimeout = 10 * time.Second

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		log.Error("config: load", "err", err)
		os.Exit(1)
	}

	client, err := catalog.NewClient(cfg.StorefrontBaseURL, cfg.StorefrontKey, cfg.UpstreamTimeout)
	if err != nil {
		log.Error("catalog: new client", "err", err)
		os.Exit(1)
	}

	registry := gsmcp.NewRegistry()
	if err := catalog.RegisterTools(registry, client); err != nil {
		log.Error("catalog: register tools", "err", err)
		os.Exit(1)
	}

	metricsRegistry := prometheus.NewRegistry()
	toolMetrics, err := observe.NewToolMetrics(metricsRegistry, metricsServiceName)
	if err != nil {
		log.Error("observe: new tool metrics", "err", err)
		os.Exit(1)
	}

	mcpHandler, err := server.New(registry, cfg.MCPKey, toolMetrics)
	if err != nil {
		log.Error("server: new", "err", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.Handle("/mcp", mcpHandler)
	mux.Handle("/metrics", promhttp.HandlerFor(metricsRegistry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Config values are logged by NAME only, never by value — the storefront
	// key and the MCP shared key are secrets, and a truncated secret is still
	// a secret.
	log.Info("starting mcp-catalog",
		"port", cfg.Port,
		"upstream_timeout", cfg.UpstreamTimeout.String(),
		"storefront_base_url", cfg.StorefrontBaseURL,
	)

	serveErr := make(chan error, 1)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case <-ctx.Done():
		log.Info("shutting down: signal received")
	case err := <-serveErr:
		if err != nil {
			log.Error("http: listen and serve", "err", err)
			os.Exit(1)
		}
		return
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Error("http: shutdown", "err", err)
		os.Exit(1)
	}
	log.Info("shutdown complete")
}
