package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/application"
	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/store"
	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/web"
)

func main() {
	if err := run(); err != nil {
		log.Printf("服务退出：%v", err)
		os.Exit(1)
	}
}

func run() error {
	address := flag.String("addr", addressDefault(), "回环监听地址")
	databasePath := flag.String("db", "oral_history.db", "SQLite 数据库路径")
	selfcheck := flag.Bool("selfcheck", false, "通过真实 HTTP 完成有界业务自检后退出")
	selfcheckTimeout := flag.Duration("selfcheck-timeout", 15*time.Second, "自检超时时间")
	flag.Parse()
	if err := validateAddress(*address); err != nil {
		return err
	}
	ctx := context.Background()
	dbPath := *databasePath
	if *selfcheck {
		dbPath = ":memory:"
	}
	repo, err := store.Open(ctx, dbPath)
	if err != nil {
		return fmt.Errorf("打开数据库: %w", err)
	}
	defer repo.Close()
	app := application.NewService(repo)
	handler := web.NewServer(app)
	listener, err := net.Listen("tcp", *address)
	if err != nil {
		return fmt.Errorf("监听 %s: %w", *address, err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()
	if *selfcheck {
		checkCtx, cancel := context.WithTimeout(ctx, *selfcheckTimeout)
		defer cancel()
		err = runSelfcheck(checkCtx, "http://"+listener.Addr().String())
		shutdownCtx, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		shutdownErr := server.Shutdown(shutdownCtx)
		serveErr := <-serveDone
		if !errors.Is(serveErr, http.ErrServerClosed) {
			return serveErr
		}
		if err != nil {
			return fmt.Errorf("自检失败: %w", err)
		}
		if shutdownErr != nil {
			return shutdownErr
		}
		log.Printf("自检通过：完成建档、修订、标注、协商、封存与分级阅览流程")
		return nil
	}
	log.Printf("口述历史授权编研工作台监听于 http://%s", listener.Addr())
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case <-signalCtx.Done():
	case err = <-serveDone:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err = server.Shutdown(shutdownCtx); err != nil {
		return err
	}
	err = <-serveDone
	if !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
