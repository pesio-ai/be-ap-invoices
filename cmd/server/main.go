package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	pb "github.com/pesio-ai/be-lib-proto/gen/go/ap"
	"github.com/pesio-ai/be-lib-common/config"
	"github.com/pesio-ai/be-lib-common/database"
	"github.com/pesio-ai/be-lib-common/logger"
	"github.com/pesio-ai/be-lib-common/middleware"
	"github.com/pesio-ai/be-ap-invoices/internal/client"
	"github.com/pesio-ai/be-ap-invoices/internal/handler"
	"github.com/pesio-ai/be-ap-invoices/internal/repository"
	"github.com/pesio-ai/be-ap-invoices/internal/service"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log := logger.New(logger.Config{
		Level:       os.Getenv("LOG_LEVEL"),
		Environment: cfg.Service.Environment,
		ServiceName: cfg.Service.Name,
		Version:     cfg.Service.Version,
	})

	log.Info().
		Str("service", cfg.Service.Name).
		Str("version", cfg.Service.Version).
		Str("environment", cfg.Service.Environment).
		Msg("Starting Invoices Service (AP-2)")

	// Create context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize database
	db, err := database.New(ctx, database.Config{
		Host:        cfg.Database.Host,
		Port:        cfg.Database.Port,
		User:        cfg.Database.User,
		Password:    cfg.Database.Password,
		Database:    cfg.Database.Database,
		SSLMode:     cfg.Database.SSLMode,
		MaxConns:    cfg.Database.MaxConns,
		MinConns:    cfg.Database.MinConns,
		MaxConnTime: cfg.Database.MaxConnTime,
		MaxIdleTime: cfg.Database.MaxIdleTime,
		HealthCheck: cfg.Database.HealthCheck,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}
	defer db.Close()
	log.Info().Msg("Database connection established")

	// Initialize repositories
	invoiceRepo := repository.NewInvoiceRepository(db)

	// Initialize gRPC service clients
	vendorsGrpcAddr := getEnv("VENDORS_GRPC_URL", "localhost:9084")
	vendorsClient, err := client.NewVendorsGRPCClient(vendorsGrpcAddr)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create vendors gRPC client")
	}
	defer vendorsClient.Close()

	accountsGrpcAddr := getEnv("ACCOUNTS_GRPC_URL", "localhost:9082")
	accountsClient, err := client.NewAccountsGRPCClient(accountsGrpcAddr)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create accounts gRPC client")
	}
	defer accountsClient.Close()

	journalsGrpcAddr := getEnv("JOURNALS_GRPC_URL", "localhost:9083")
	journalsClient, err := client.NewJournalsGRPCClient(journalsGrpcAddr)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create journals gRPC client")
	}
	defer journalsClient.Close()

	log.Info().
		Str("vendors_grpc", vendorsGrpcAddr).
		Str("accounts_grpc", accountsGrpcAddr).
		Str("journals_grpc", journalsGrpcAddr).
		Msg("gRPC service clients initialized")

	// Initialize services
	invoiceService := service.NewInvoiceService(invoiceRepo, vendorsClient, accountsClient, journalsClient, log)

	// Setup HTTP routes
	httpHandler := handler.NewHTTPHandler(invoiceService, log)
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy"}`))
	})

	// Invoice routes
	mux.HandleFunc("/api/v1/invoices", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			httpHandler.ListInvoices(w, r)
		case http.MethodPost:
			httpHandler.CreateInvoice(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/v1/invoices/get", httpHandler.GetInvoice)
	mux.HandleFunc("/api/v1/invoices/submit", httpHandler.SubmitForApproval)
	mux.HandleFunc("/api/v1/invoices/approve", httpHandler.ApproveInvoice)
	mux.HandleFunc("/api/v1/invoices/post", httpHandler.PostInvoice)
	mux.HandleFunc("/api/v1/invoices/payment", httpHandler.RecordPayment)
	mux.HandleFunc("/api/v1/invoices/delete", httpHandler.DeleteInvoice)

	// Apply middleware
	var h http.Handler = mux
	h = middleware.RequestID(h)
	h = middleware.Logger(&log.Logger)(h)
	h = middleware.Recovery(&log.Logger)(h)
	h = middleware.CORS([]string{"*"})(h)
	h = middleware.Timeout(30 * time.Second)(h)

	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      h,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	go func() {
		log.Info().Int("port", cfg.Server.Port).Msg("Starting HTTP server")
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("HTTP server failed")
		}
	}()

	// Start gRPC server
	grpcPort := getEnvInt("GRPC_PORT", 9085)
	grpcHandler := handler.NewGRPCHandler(invoiceService, log.Logger)

	grpcServer := grpc.NewServer()
	pb.RegisterInvoicesServiceServer(grpcServer, grpcHandler)
	reflection.Register(grpcServer) // Enable reflection for debugging

	grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%d", grpcPort))
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create gRPC listener")
	}

	go func() {
		log.Info().Int("port", grpcPort).Msg("Starting gRPC server")
		if err := grpcServer.Serve(grpcListener); err != nil {
			log.Error().Err(err).Msg("gRPC server failed")
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutting down server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("HTTP server shutdown failed")
	}

	// Stop gRPC server gracefully
	grpcServer.GracefulStop()

	log.Info().Msg("Server stopped")
}

// getEnv gets an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt gets an environment variable as int or returns a default value
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var result int
		_, err := fmt.Sscanf(value, "%d", &result)
		if err == nil {
			return result
		}
	}
	return defaultValue
}
