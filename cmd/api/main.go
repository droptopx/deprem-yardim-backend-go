package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Shopify/sarama"
	"github.com/acikkaynak/backend-api-go/broker"
	"github.com/acikkaynak/backend-api-go/handler"
	"github.com/acikkaynak/backend-api-go/middleware/auth"
	"github.com/acikkaynak/backend-api-go/middleware/cache"
	"github.com/acikkaynak/backend-api-go/repository"
	_ "github.com/acikkaynak/backend-api-go/swagger"
	swagger "github.com/arsmn/fiber-swagger/v2"
	"github.com/gofiber/adaptor/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/monitor"
	"github.com/gofiber/fiber/v2/middleware/pprof"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Application struct {
	app           *fiber.App
	repo          *repository.Repository
	kafkaProducer sarama.SyncProducer
}

func (a *Application) Register() {
	a.app.Get("/", handler.RedirectSwagger)
	a.app.Get("/healthcheck", handler.Healtcheck)
	a.app.Get("/metrics", adaptor.HTTPHandler(promhttp.Handler()))
	a.app.Get("/monitor", monitor.New())
	a.app.Get("/feeds/areas", handler.GetFeedAreas(a.repo))
	a.app.Patch("/feeds/areas", handler.UpdateFeedLocationsHandler(a.repo))
	a.app.Get("/feeds/:id/", handler.GetFeedById(a.repo))
	// We need to set up authentication for POST /events endpoint.
	a.app.Post("/events", handler.CreateEventHandler(a.kafkaProducer))
	a.app.Get("/caches/prune", handler.InvalidateCache())
	route := a.app.Group("/swagger")
	route.Get("*", swagger.HandlerDefault)
}

// @title               IT Afet Yardım
// @version             1.0
// @description         Afet Harita API
// @BasePath            /
// @schemes             http https
func main() {
	repo := repository.New()
	defer repo.Close()

	needsHandler := handler.NewNeedsHandler(repo)

	kafkaProducer, err := broker.NewProducer()
	if err != nil {
		log.Println("failed to init kafka produder. err:", err)
	}

	app := fiber.New()
	app.Use(cors.New())
	app.Use(recover.New())
	app.Use(auth.New())
	app.Use(pprof.New())
	app.Use(cache.New())

	app.Get("/needs", needsHandler.HandleList)
	app.Post("/needs", needsHandler.HandleCreate)

	application := &Application{app: app, repo: repo, kafkaProducer: kafkaProducer}
	application.Register()

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT)
	signal.Notify(c, syscall.SIGTERM)

	go func() {
		_ = <-c
		fmt.Println("application gracefully shutting down..")
		_ = app.Shutdown()
	}()

	if err := app.Listen(":80"); err != nil {
		panic(fmt.Sprintf("app error: %s", err.Error()))
	}
}
