//go:build !host

// Voicer ACAP entry point (Axis camera build).
//
// This file imports goxis, which depends on the Axis SDK C libraries and only
// compiles inside the goxisbuilder Docker image. For host-side dev / tests use
// `-tags=host` (see main_host.go).
package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Cacsjep/goxis/pkg/acapapp"
	"github.com/Cacsjep/voicer/ax_voicer/audio"
	"github.com/Cacsjep/voicer/ax_voicer/controllers"
	"github.com/Cacsjep/voicer/ax_voicer/settings"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/logger"
)

const listenAddr = ":8889"

func main() {
	app := acapapp.NewAcapApplication()

	baseUri, err := app.AcapWebBaseUri()
	if err != nil {
		app.Syslog.Crit("AcapWebBaseUri: " + err.Error())
		return
	}
	app.Syslog.Infof("Voicer base path: %s", baseUri)

	settingsPath := resolveSettingsPath()
	store, err := settings.NewStore(settingsPath)
	if err != nil {
		app.Syslog.Crit("settings: " + err.Error())
		return
	}
	app.Syslog.Infof("Voicer settings file: %s", settingsPath)

	api := &controllers.API{
		Store:  store,
		Player: audio.New(),
		Log:    app.Syslog,
	}

	fapp := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})
	fapp.Use(cors.New())
	fapp.Use(logger.New(logger.Config{
		Format: "voicer ${status} ${method} ${path} ${latency}\n",
	}))

	registerRoutes(fapp, baseUri, api)

	app.AddCloseCleanFunc(func() {
		app.Syslog.Info("Shutting down voicer")
		_ = fapp.Shutdown()
	})

	go app.RunInBackground()
	app.Syslog.Infof("Voicer listening on %s", listenAddr)
	if err := fapp.Listen(listenAddr); err != nil {
		app.Syslog.Crit("listen: " + err.Error())
	}
}

func registerRoutes(app *fiber.App, baseUri string, api *controllers.API) {
	// Public 3rd-party endpoint at <cam>/local/voicer/playvoice. Mounted
	// before the API group so it isn't routed under /api.
	app.Post(baseUri+"/playvoice", api.PlayVoice)
	app.Get(baseUri+"/health", api.Health)

	apiGroup := app.Group(baseUri + "/api")
	apiGroup.Get("/settings", api.GetSettings)
	apiGroup.Post("/settings", api.UpdateSettings)
	apiGroup.Get("/audio/info", api.GetAudioInfo)
	apiGroup.Post("/test/api_key", api.TestAPIKey)
	apiGroup.Get("/test/voices", api.ListVoices)
	apiGroup.Get("/test/models", api.ListModels)
	apiGroup.Post("/test/play", api.Test)
	apiGroup.Post("/test/tone", api.TestTone)
	apiGroup.Post("/test/synth_download", api.SynthDownload)

	// Static files (built Vue app) live in ./html relative to the binary.
	app.Use(baseUri, filesystem.New(filesystem.Config{
		Root:   http.Dir("./html"),
		Browse: false,
		Index:  "index.html",
	}))

	indexBytes, _ := os.ReadFile("./html/index.html")
	app.Use(func(c *fiber.Ctx) error {
		if c.Method() != fiber.MethodGet {
			return c.SendStatus(fiber.StatusNotFound)
		}
		path := c.Path()
		if strings.HasPrefix(path, baseUri+"/api") ||
			strings.HasPrefix(path, baseUri+"/playvoice") ||
			strings.HasPrefix(path, baseUri+"/health") {
			return c.SendStatus(fiber.StatusNotFound)
		}
		if strings.HasPrefix(path, baseUri+"/assets") {
			return c.SendStatus(fiber.StatusNotFound)
		}
		if len(indexBytes) == 0 {
			return c.SendStatus(fiber.StatusNotFound)
		}
		return c.Type("html").Send(indexBytes)
	})
}

// resolveSettingsPath uses /usr/local/packages/voicer/localdata when running on
// a real device (the Axis-blessed writeable folder for the package), and a
// folder next to the executable otherwise.
func resolveSettingsPath() string {
	if env := os.Getenv("VOICER_SETTINGS_PATH"); env != "" {
		return env
	}
	const localData = "/usr/local/packages/voicer/localdata"
	if st, err := os.Stat(localData); err == nil && st.IsDir() {
		return filepath.Join(localData, "settings.json")
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "settings.json")
	}
	return "settings.json"
}
