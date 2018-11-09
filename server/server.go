package main

import (
	"context"
	"errors"
	"html/template"
	"net/http"
	"os"

	"contrib.go.opencensus.io/exporter/stackdriver"
	"contrib.go.opencensus.io/exporter/stackdriver/monitoredresource"
	"contrib.go.opencensus.io/exporter/stackdriver/propagation"
	"github.com/99designs/gqlgen-contrib/gqlopencensus"
	gql "github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/handler"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/cors"
	"github.com/icco/graphql"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/trace"
	"gopkg.in/unrolled/render.v1"
	"gopkg.in/unrolled/secure.v1"
)

var (
	// Renderer is a renderer for all occasions. These are our preferred default options.
	// See:
	//  - https://github.com/unrolled/render/blob/v1/README.md
	//  - https://godoc.org/gopkg.in/unrolled/render.v1
	Renderer = render.New(render.Options{
		Charset:                   "UTF-8",
		Directory:                 "./server/views",
		DisableHTTPErrorRendering: false,
		Extensions:                []string{".tmpl", ".html"},
		IndentJSON:                false,
		IndentXML:                 true,
		Layout:                    "layout",
		RequirePartials:           true,
		Funcs:                     []template.FuncMap{{}},
	})

	dbURL = os.Getenv("DATABASE_URL")

	log = &logrus.Logger{
		Out:       os.Stderr,
		Formatter: new(logrus.JSONFormatter),
		Hooks:     make(logrus.LevelHooks),
		Level:     logrus.DebugLevel,
	}
)

func main() {
	if dbURL == "" {
		log.Fatalf("DATABASE_URL is empty!")
	}

	_, err := graphql.InitDB(dbURL)
	if err != nil {
		log.Fatalf("Init DB: %+v", err)
	}

	OAuthConfig = configureOAuthClient(
		os.Getenv("OAUTH2_CLIENTID"),
		os.Getenv("OAUTH2_SECRET"),
		os.Getenv("OAUTH2_REDIRECT"))

	port := "8080"
	if fromEnv := os.Getenv("PORT"); fromEnv != "" {
		port = fromEnv
	}
	log.Printf("Starting up on http://localhost:%s", port)

	if os.Getenv("ENABLE_STACKDRIVER") != "" {
		sd, err := stackdriver.NewExporter(stackdriver.Options{
			ProjectID:               "icco-cloud",
			MetricPrefix:            "graphql",
			MonitoredResource:       monitoredresource.Autodetect(),
			DefaultMonitoringLabels: &stackdriver.Labels{},
			DefaultTraceAttributes:  map[string]interface{}{"/http/host": "graphql.natwelch.com"},
		})

		if err != nil {
			log.Fatalf("Failed to create the Stackdriver exporter: %v", err)
		}
		defer sd.Flush()

		view.RegisterExporter(sd)
		trace.RegisterExporter(sd)
		trace.ApplyConfig(trace.Config{
			DefaultSampler: trace.AlwaysSample(),
		})
	}

	isDev := os.Getenv("NAT_ENV") != "production"

	r := chi.NewRouter()

	// TODO: Add status code info
	r.Use(func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {

			scheme := "http"
			if r.TLS != nil {
				scheme = "https"
			}

			reqData := map[string]interface{}{
				"host":   r.Host,
				"path":   r.RequestURI,
				"proto":  r.Proto,
				"ip":     r.RemoteAddr,
				"scheme": scheme,
			}
			log.WithField("req", reqData).Info()

			next.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	})
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(ContextMiddleware)

	r.Use(cors.New(cors.Options{
		AllowCredentials:   true,
		OptionsPassthrough: true,
		AllowedOrigins:     []string{"*"},
		AllowedMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:     []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:     []string{"Link"},
		MaxAge:             300, // Maximum value not ignored by any of major browsers
	}).Handler)

	r.NotFound(notFoundHandler)

	// Stuff that does not ssl redirect
	r.Group(func(r chi.Router) {
		r.Use(secure.New(secure.Options{
			BrowserXssFilter:   true,
			ContentTypeNosniff: true,
			FrameDeny:          true,
			HostsProxyHeaders:  []string{"X-Forwarded-Host"},
			IsDevelopment:      isDev,
			SSLProxyHeaders:    map[string]string{"X-Forwarded-Proto": "https"},
		}).Handler)

		r.Get("/healthz", healthCheckHandler)
	})

	// Everything that does SSL only
	r.Group(func(r chi.Router) {
		r.Use(secure.New(secure.Options{
			BrowserXssFilter:     true,
			ContentTypeNosniff:   true,
			FrameDeny:            true,
			HostsProxyHeaders:    []string{"X-Forwarded-Host"},
			IsDevelopment:        isDev,
			SSLProxyHeaders:      map[string]string{"X-Forwarded-Proto": "https"},
			SSLRedirect:          !isDev,
			STSIncludeSubdomains: true,
			STSPreload:           true,
			STSSeconds:           315360000,
		}).Handler)

		r.Get("/cron", cronHandler)

		r.Mount("/debug", middleware.Profiler())

		r.Mount("/admin", adminRouter())

		r.Handle("/", handler.Playground("graphql", "/graphql"))
		r.Handle("/graphql", handler.GraphQL(
			graphql.NewExecutableSchema(graphql.New()),
			handler.RecoverFunc(func(ctx context.Context, intErr interface{}) error {
				err, ok := intErr.(error)
				if ok {
					log.WithError(err).Error("Error seen during graphql")
				}
				return errors.New("Fatal message seen when processing request")
			}),
			handler.CacheSize(512),
			handler.RequestMiddleware(func(ctx context.Context, next func(ctx context.Context) []byte) []byte {
				rctx := gql.GetRequestContext(ctx)

				// We do this because RequestContext has fields that can't be easily
				// serialized in json, and we don't care about them.
				subsetContext := map[string]interface{}{
					"query":      rctx.RawQuery,
					"variables":  rctx.Variables,
					"extensions": rctx.Extensions,
				}

				log.WithField("gql", subsetContext).Printf("request gql")

				return next(ctx)
			}),
			handler.Tracer(gqlopencensus.New()),
		))

		// Auth stuff
		r.HandleFunc("/login", loginHandler)
		r.HandleFunc("/logout", logoutHandler)
		r.HandleFunc("/callback", callbackHandler)
	})

	h := &ochttp.Handler{
		Handler:     r,
		Propagation: &propagation.HTTPFormat{},
	}
	if err := view.Register(ochttp.DefaultServerViews...); err != nil {
		log.Fatal("Failed to register ochttp.DefaultServerViews")
	}

	log.Fatal(http.ListenAndServe(":"+port, h))
}

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	Renderer.JSON(w, http.StatusOK, map[string]string{
		"healthy": "true",
	})
}

func cronHandler(w http.ResponseWriter, r *http.Request) {
	go func(ctx context.Context) {
		var posts []*graphql.Post
		var err error
		perPage := 10

		for i := 0; err == nil || len(posts) > 0; i += perPage {
			posts, err = graphql.Posts(ctx, &perPage, &i)
			if err == nil {
				for _, p := range posts {
					err = p.Save(ctx)
					if err != nil {
						log.WithError(err).Printf("Error saving post")
					}
				}
			}
		}
	}(context.Background())

	Renderer.JSON(w, http.StatusOK, map[string]string{
		"cron": "ok",
	})
}

func notFoundHandler(w http.ResponseWriter, r *http.Request) {
	Renderer.HTML(w, http.StatusNotFound, "404", struct{ Title string }{Title: "404: This page could not be found"})
}
