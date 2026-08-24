package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/auth"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/clock"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/config"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/httpapi"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/service"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/storage/sqlite"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/worker"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	store, err := sqlite.Open(ctx, cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer store.Close()
	if err = store.Migrate(ctx); err != nil {
		return err
	}
	if err = bootstrap(ctx, store); err != nil {
		return err
	}
	realClock := clock.System{}
	authService := service.NewAuth(store, realClock, cfg.SessionTTL)
	batchService := service.NewBatches(store, store, realClock)
	orderService := service.NewOrders(store, store, realClock)
	settlementService := service.NewSettlements(store, store, realClock)
	handler := httpapi.New(authService, batchService, orderService, settlementService, store, logger)
	server := &http.Server{Addr: cfg.Address, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second}
	publisher := worker.LogPublisher{Logger: logger}
	outbox := worker.NewOutbox(store, realClock, publisher, logger, "server-worker", cfg.WorkerInterval, cfg.LeaseDuration)
	go outbox.Run(ctx)
	serveErr := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "address", cfg.Address)
		serveErr <- server.ListenAndServe()
	}()
	select {
	case err = <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			cancel()
			outbox.Wait()
			return err
		}
	case <-ctx.Done():
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()
	shutdownErr := server.Shutdown(shutdownCtx)
	cancel()
	outbox.Wait()
	return shutdownErr
}

func bootstrap(ctx context.Context, store *sqlite.Store) error {
	password := strings.TrimSpace(os.Getenv("BOOTSTRAP_PASSWORD"))
	if password == "" {
		return nil
	}
	exists, err := store.TenantExists(ctx, "xiji-coop")
	if err != nil || exists {
		return err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	roles := []domain.Role{domain.RoleOperator, domain.RoleInspector, domain.RoleDispatcher, domain.RoleFinance, domain.RoleFarmer}
	users := make([]domain.User, 0, len(roles))
	for _, role := range roles {
		id, idErr := auth.NewID("usr")
		if idErr != nil {
			return idErr
		}
		users = append(users, domain.User{ID: id, TenantID: "xiji-coop", Email: string(role) + "@mushroomchain.local", DisplayName: string(role), Role: role, PasswordHash: hash, Active: true, CreatedAt: now, UpdatedAt: now})
	}
	return store.SeedTenant(ctx, "xiji-coop", "Xiji Mushroom Cooperative", users)
}
