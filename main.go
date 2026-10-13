package main

import (
	"API_Gateway/backend"
	"API_Gateway/builder"
	"API_Gateway/connector"
	"API_Gateway/handler"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	if err := run(); err != nil {
		log.Printf("[main]%s", err.Error())
		os.Exit(1)
	}
	log.Println("[main]gateway stopped")
}

func run() error {
	table, err := buildConfig()
	if err != nil {
		return err
	}

	bk, lst, err := initComponents(table)
	if err != nil {
		return err
	}
	defer func() {
		if err := bk.Close(); err != nil {
			log.Printf("[main]failed to close backend, errmsg: %s", err.Error())
		}
	}()

	return serve(lst)
}

// buildConfig 构建配置
func buildConfig() (*builder.Config, error) {
	table, err := builder.Build("configs")
	if err != nil {
		return nil, fmt.Errorf("failed to build config: %w", err)
	}
	return table, nil
}

// initComponents 组件初始化，按数据流逆序 backend → handler → connector 接线
func initComponents(table *builder.Config) (*backend.BManager, *connector.Listener, error) {
	bk := backend.NewBManager(table.Backend)

	hdl, err := handler.NewHandler(table.Handler, bk.Call)
	if err != nil {
		// 连接池已起 reaper 协程，失败路径也要回收
		_ = bk.Close()
		return nil, nil, fmt.Errorf("failed to new handler: %w", err)
	}

	lst, err := connector.NewListener(table.Connector, hdl.HandleHTTPConn)
	if err != nil {
		_ = bk.Close()
		return nil, nil, fmt.Errorf("failed to new listener: %w", err)
	}
	return bk, lst, nil
}

// serve 服务启动，阻塞到启动失败或收到退出信号
func serve(lst *connector.Listener) error {
	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() { serveErr <- lst.Connect() }()

	select {
	case err := <-serveErr:
		return fmt.Errorf("failed to start service: %w", err)
	case <-sigCtx.Done():
	}

	stop()
	log.Println("[main]shutting down")

	lst.Close()

	<-serveErr
	return nil
}
