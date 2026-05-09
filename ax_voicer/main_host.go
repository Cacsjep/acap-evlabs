//go:build host

// Voicer host build. Runs the Fiber server without the Axis SDK so the Go
// logic can be exercised on Windows / Linux / macOS dev machines.
//
//	go run -tags=host,mock . [-listen :8889] [-html ./html] [-settings ./settings.json]
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/Cacsjep/voicer/ax_voicer/audio"
	"github.com/Cacsjep/voicer/ax_voicer/controllers"
	"github.com/Cacsjep/voicer/ax_voicer/settings"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/logger"
)

type stdLogger struct{}

func (stdLogger) Infof(f string, a ...interface{})  { log.Printf("INFO  "+f, a...) }
func (stdLogger) Errorf(f string, a ...interface{}) { log.Printf("ERROR "+f, a...) }

func main() {
	listen := flag.String("listen", ":8889", "HTTP listen address")
	htmlDir := flag.String("html", "./html", "static frontend directory")
	settingsPath := flag.String("settings", "./settings.local.json", "settings file path")
	baseUri := flag.String("base", "/local/voicer/voicer", "base URI matching the manifest reverse proxy")
	flag.Parse()

	store, err := settings.NewStore(*settingsPath)
	if err != nil {
		log.Fatalf("settings: %v", err)
	}
	api := &controllers.API{
		Store:  store,
		Player: audio.New(),
		Log:    stdLogger{},
	}

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(cors.New())
	app.Use(logger.New())

	app.Post(*baseUri+"/playvoice", api.PlayVoice)
	app.Get(*baseUri+"/health", api.Health)
	g := app.Group(*baseUri + "/api")
	g.Get("/settings", api.GetSettings)
	g.Post("/settings", api.UpdateSettings)
	g.Get("/audio/info", api.GetAudioInfo)
	g.Post("/test/api_key", api.TestAPIKey)
	g.Get("/test/voices", api.ListVoices)
	g.Get("/test/models", api.ListModels)
	g.Post("/test/play", api.Test)
	g.Post("/test/tone", api.TestTone)
	g.Post("/test/synth_download", api.SynthDownload)

	if _, err := os.Stat(*htmlDir); err == nil {
		app.Use(*baseUri, filesystem.New(filesystem.Config{
			Root:   http.Dir(*htmlDir),
			Browse: false,
			Index:  "index.html",
		}))
	}

	app.Use(func(c *fiber.Ctx) error {
		if strings.HasPrefix(c.Path(), *baseUri+"/api") ||
			c.Path() == *baseUri+"/playvoice" ||
			c.Path() == *baseUri+"/health" {
			return c.SendStatus(fiber.StatusNotFound)
		}
		idx := *htmlDir + "/index.html"
		if data, err := os.ReadFile(idx); err == nil {
			return c.Type("html").Send(data)
		}
		return c.SendStatus(fiber.StatusNotFound)
	})

	log.Printf("voicer host build listening on %s (base=%s, html=%s)", *listen, *baseUri, *htmlDir)
	if err := app.Listen(*listen); err != nil {
		log.Fatal(err)
	}
}
