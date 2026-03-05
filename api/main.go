package main

import (
	"os"
	"strconv"

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

	if port := os.Getenv("PORT"); port != "" {
		if p, err := strconv.ParseInt(port, 10, 64); err == nil {
			cfg.Server.Port = p
		}
	}

	engine := scraper.New(scraper.Config{
		Concurrency:    cfg.Scraper.Concurrency,
		RequestDelay:   cfg.Scraper.RequestDelay(),
		RandomDelay:    cfg.Scraper.RandomDelay(),
		RetryCount:     cfg.Scraper.RetryCount,
		RequestTimeout: cfg.Scraper.RequestTimeout(),
	})

	app := fiber.New(cfg.Server.Config)
	gol_middlewares.AddDefaultMiddlewares(app, cfg.Server)

	h := handler.New(engine)
	handler.RegisterRoutes(app, h)

	logging.Logger.Info("server starting", zap.String("host", cfg.Server.GetHost()))
	if err := app.Listen(cfg.Server.GetHost()); err != nil {
		logging.Logger.Fatal("server failed", zap.Error(err))
	}
}
