package main

import (
	"os"

	"github.com/ahmet4dev/gol-lib/logging"
	gol_middlewares "github.com/ahmet4dev/gol-lib/middlewares"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	handler "webscraper/handlers/http"
	"webscraper/internal/configs"
	"webscraper/internal/features/scraper"
)

func main() {
	cfg, err := configs.Load("config.json")
	if err != nil {
		logging.Logger.Fatal("failed to load config", zap.Error(err))
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "10000"
	}

	engine := scraper.New(scraper.Config{
		Concurrency:    cfg.Scraper.Concurrency,
		RequestDelay:   cfg.Scraper.RequestDelay(),
		RandomDelay:    cfg.Scraper.RandomDelay(),
		RetryCount:     cfg.Scraper.RetryCount,
		RequestTimeout: cfg.Scraper.RequestTimeout(),
		MatchThreshold: cfg.Matcher.Threshold,
	})

	app := fiber.New(cfg.Server.Config)
	gol_middlewares.AddDefaultMiddlewares(app, cfg.Server)

	h := handler.New(engine)
	handler.RegisterRoutes(app, h)

	host := ":" + port
	logging.Logger.Info("server starting", zap.String("host", host))
	if err := app.Listen(host); err != nil {
		logging.Logger.Fatal("server failed", zap.Error(err))
	}
}
