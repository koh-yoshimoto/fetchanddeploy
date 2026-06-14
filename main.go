package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	configPath := flag.String("config", "config.yaml", "設定ファイルのパス")
	showVersion := flag.Bool("version", false, "バージョンを表示して終了")
	flag.Parse()

	if *showVersion {
		log.SetFlags(0)
		log.Printf("fetchanddeploy %s", version)
		return
	}

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("設定エラー: %v", err)
	}

	if err := setupLogging(cfg.LogFile); err != nil {
		log.Fatalf("ログ設定エラー: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle(cfg.Path, NewHandler(cfg, NewDeployer(), NewNotifier()))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	})

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// グレースフルシャットダウン。
	idleClosed := make(chan struct{})
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		log.Print("シャットダウンします…")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("シャットダウンエラー: %v", err)
		}
		close(idleClosed)
	}()

	log.Printf("fetchanddeploy %s 起動 listen=%s path=%s repos=%d", version, cfg.Listen, cfg.Path, len(cfg.Repositories))
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("サーバ起動エラー: %v", err)
	}
	<-idleClosed
}

// setupLogging はファイル指定があればstdoutと併せて出力する。
func setupLogging(logFile string) error {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	if logFile == "" {
		return nil
	}
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	log.SetOutput(io.MultiWriter(os.Stdout, f))
	return nil
}
