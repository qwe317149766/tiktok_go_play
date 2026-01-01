package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	loadEnv()
	cfg := loadConfig()

	db, err := openDB(cfg.MySQLDSN())
	if err != nil {
		log.Fatalf("db connect failed: %v", err)
	}
	defer db.Close()

	repo := NewRepo(db)
	{
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := repo.EnsureSchema(ctx); err != nil {
			log.Fatalf("ensure schema failed: %v", err)
		}
	}

	// api_key 缓存（全 DB：进程内 TTL cache）
	cache, err := newAPIKeyCache()
	if err != nil {
		log.Fatalf("cache init failed: %v", err)
	}
	defer cache.Close()

	srv := NewServer(cfg, repo, cache)
	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("api listening on %s", cfg.Addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen error: %v", err)
		}
	}()

	// graceful shutdown
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	log.Printf("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
}


