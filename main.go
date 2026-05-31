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

	"transbridge/internal/app"
)

func main() {
	configFile := flag.String("config", "config.yml", "配置文件路径")
	flag.Parse()

	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	runningOnVercel := os.Getenv("VERCEL") != ""
	application, err := app.New(*configFile, app.Options{
		Serverless: runningOnVercel,
	})
	if err != nil {
		log.Fatalf("Failed to initialize application: %v", err)
	}

	server := application.HTTPServer()
	if port := os.Getenv("PORT"); port != "" {
		server.Addr = ":" + port
	}

	runCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 2)

	go func() {
		log.Printf("Starting server on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("http server error: %w", err)
		}
	}()

	if application.TelegramBot != nil && !runningOnVercel {
		go func() {
			if err := application.TelegramBot.Run(runCtx); err != nil && !errors.Is(err, context.Canceled) {
				errCh <- fmt.Errorf("telegram bot error: %w", err)
			}
		}()
	} else if application.TelegramBot != nil {
		log.Println("Telegram polling disabled on Vercel; use /telegram/webhook")
	}

	select {
	case <-runCtx.Done():
		log.Println("Received shutdown signal")
	case err := <-errCh:
		log.Printf("Application stopped by internal error: %v", err)
		stop()
	}

	log.Println("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	if err := application.Close(ctx); err != nil {
		log.Printf("Error closing application: %v", err)
	}

	log.Println("Server exited")
}
