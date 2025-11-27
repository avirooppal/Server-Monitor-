package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"server-monitor/backend/internal/api"
	"server-monitor/backend/internal/db"
	"server-monitor/backend/internal/service"
	ws "server-monitor/backend/internal/websocket"
)

func main() {
	// Get DB connection string from env
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://monitor:secret@localhost:5432/monitoring?sslmode=disable"
	}

	// Initialize database
	database, err := db.NewDatabase(dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer database.GetPool().Close()

	// Initialize DB schema
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := database.Init(ctx); err != nil {
		log.Fatalf("Failed to initialize database schema: %v", err)
	}

	// Initialize WebSocket Hub
	hub := ws.NewWSHub()
	go hub.Run()

	// Initialize API
	apiHandler := api.NewAPI(database, hub)

	// Start Retention Worker
	retentionWorker := service.NewRetentionWorker(database)
	retentionWorker.Start()

	// Setup Router
	r := gin.Default()

	// CORS Configuration
	config := cors.DefaultConfig()
	config.AllowAllOrigins = true
	config.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "Authorization"}
	r.Use(cors.New(config))

	// API Routes
	v1 := r.Group("/api/v1")
	{
		v1.POST("/servers", apiHandler.RegisterServer)
		v1.GET("/servers", apiHandler.ListServers)
		v1.POST("/ingest", apiHandler.IngestMetrics)
		v1.POST("/ingest/processes", apiHandler.IngestProcessSnapshot)
		v1.POST("/ingest/containers", apiHandler.IngestContainerSnapshot)
		v1.GET("/metrics", apiHandler.GetMetrics)
		v1.GET("/containers", apiHandler.GetContainerSnapshot)
		v1.GET("/stream", apiHandler.HandleWebSocket)

		// Cron
		v1.POST("/cron/jobs", apiHandler.CreateCronJob)
		v1.GET("/cron/jobs", apiHandler.ListCronJobs)
		v1.POST("/cron/results", apiHandler.IngestCronResult)

		// Ports
		v1.POST("/ports/checks", apiHandler.CreatePortCheck)
		v1.GET("/ports/checks", apiHandler.ListPortChecks)
		v1.POST("/ports/results", apiHandler.IngestPortResult)
	}

	// Health Check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Serve static files from web directory
	r.Static("/web", "./web")
	
	// Serve index.html for root and any unmatched routes (SPA fallback)
	r.NoRoute(func(c *gin.Context) {
		c.File("./web/index.html")
	})

	// Start Server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	log.Printf("Server starting on :%s", port)

	// Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exiting")
}
