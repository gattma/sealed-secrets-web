package main

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	ssClient "github.com/bitnami-labs/sealed-secrets/pkg/client/clientset/versioned/typed/sealedsecrets/v1alpha1"
	authConfig "github.com/gattma/sealed-secrets-web/pkg/auth/config"
	auth "github.com/gattma/sealed-secrets-web/pkg/auth/dex"
	authHandler "github.com/gattma/sealed-secrets-web/pkg/auth/handler"
	"github.com/gattma/sealed-secrets-web/pkg/auth/middleware"
	"github.com/gattma/sealed-secrets-web/pkg/auth/store"
	"github.com/gattma/sealed-secrets-web/pkg/config"
	"github.com/gattma/sealed-secrets-web/pkg/handler"
	"github.com/gattma/sealed-secrets-web/pkg/seal"
	"github.com/gattma/sealed-secrets-web/pkg/version"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/redis/go-redis/v9"
)

var (
	//go:embed templates/index.html
	indexTemplate string
	//go:embed templates/secret.yaml
	initialSecretYAML string

	//go:embed static/*
	static      embed.FS
	staticFS, _ = fs.Sub(static, "static")

	clientConfig clientcmd.ClientConfig
)

func init() {
	gin.SetMode(gin.ReleaseMode)

	// The "usual" clientcmd/kubectl flags
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.DefaultClientConfig = &clientcmd.DefaultClientConfig
	overrides := clientcmd.ConfigOverrides{}
	clientConfig = clientcmd.NewInteractiveDeferredLoadingClientConfig(loadingRules, &overrides, os.Stdin)
}

func main() {
	cfg, err := config.Parse()
	if err != nil {
		log.Fatalf("Could not read the config: %s", err.Error())
	}

	authConf, err := authConfig.LoadFromEnv()
	if err != nil {
		log.Fatalf("Could not read the auth config: %s", err.Error())
		return
	}

	if cfg.PrintVersion {
		fmt.Println(version.Print("sealed secrets web"))
		return
	}

	coreClient, ssc, err := handler.BuildClients(clientConfig, cfg.DisableLoadSecrets)
	if err != nil {
		log.Fatalf("Could build k8s clients:%v", err.Error())
	}
	sealer, err := seal.NewAPISealer(cfg.Ctx, cfg.SealedSecrets)
	if err != nil {
		log.Fatalf("Setup sealer: %s", err.Error())
	}

	// auth
	ctx := context.Background()
	authClient, err := auth.New(ctx, authConf.Auth)
	if err != nil {
		log.Fatalf("failed to initialize auth client : %v", err)
	}
	rdb := redis.NewClient(authConf.RedisClient)
	sessionStore := store.NewSessionRedisManager(rdb)
	authMiddleware := middleware.NewAuthMiddleware(
		ctx,
		authClient,
		sessionStore,
	)

	log.Printf("Running sealed secrets web (%s) on port %d", version.Version, cfg.Web.Port)
	_ = setupRouter(ctx, coreClient, ssc, cfg, sealer, authMiddleware).Run(fmt.Sprintf(":%d", cfg.Web.Port))
}

func setupRouter(
	ctx context.Context,
	coreClient corev1.CoreV1Interface,
	ssClient ssClient.BitnamiV1alpha1Interface,
	cfg *config.Config,
	sealer seal.Sealer,
	authMiddleware *middleware.AuthMiddleware,
) *gin.Engine {
	indexHTML, err := renderIndexHTML(cfg)
	if err != nil {
		log.Fatalf("Could not render the index html template: %s", err.Error())
	}

	sHandler := handler.NewHandler(coreClient, ssClient, cfg)

	r := gin.New()
	r.Use(gin.Recovery())
	if cfg.Web.Logger {
		r.Use(gin.LoggerWithFormatter(ginLogFormatter()))
	}

	// TODO extract?
	aConfig, err := authConfig.LoadFromEnv()
	if err != nil {
		log.Fatalf("failed to load env file config : %v", err)
	}

	authClient, err := auth.New(ctx, aConfig.Auth)
	if err != nil {
		log.Fatalf("failed to initialize auth client : %v", err)
	}

	redisClient := redis.NewClient(aConfig.RedisClient)
	authStore := store.NewAuthRedisManager(redisClient)
	sessionStore := store.NewSessionRedisManager(redisClient)
	ah := authHandler.NewAuthHandler(authClient, authStore, sessionStore)
	// ------------
	h := handler.New(indexHTML, sealer, cfg)

	r.GET("/", h.ShowLoginPage) // TODO logout page
	r.GET("/_health", h.Health)

	protected := r.Group("/")
	protected.Use(authMiddleware.RequireAuth())
	{
		protected.GET("/dashboard", h.Index)
	}

	// TODO --------
	// Auth routes will be added later
	auth := r.Group("/auth")
	{
		auth.GET("/login", ah.LoginHandler)
		auth.GET("/callback", ah.CallbackHandler)
	}
	// ---------------

	r.StaticFS("/static", http.FS(staticFS))
	r.LoadHTMLGlob("./templates/*.*")

	// TODO authentication
	api := r.Group("/api")
	api.Use(authMiddleware.RequireAuth())
	{
		api.GET("/version", h.Version)
		api.POST("/raw", h.Raw)
		api.GET("/certificate", h.Certificate)
		api.POST("/kubeseal", h.KubeSeal)
		api.POST("/dencode", h.Dencode)
		api.POST("/validate", h.Validate)

		api.GET("/secret/:namespace/:name", sHandler.Secret)
		api.GET("/secrets", sHandler.AllSecrets)
	}

	r.NoRoute(h.RedirectToIndex(cfg.Web.Context))
	return r
}

func renderIndexHTML(cfg *config.Config) (string, error) {
	indexTmpl := template.Must(template.New("index.html").Parse(indexTemplate))
	initialSecret := initialSecretYAML
	if cfg.InitialSecret != "" {
		initialSecret = cfg.InitialSecret
	}

	data := map[string]interface{}{
		"DisableLoadSecrets":     cfg.DisableLoadSecrets,
		"DisableValidateSecrets": cfg.SealedSecrets.CertURL != "",
		"WebContext":             cfg.Web.Context,
		"InitialSecret":          initialSecret,
		"Version":                version.Version,
	}

	var tpl bytes.Buffer
	if err := indexTmpl.Execute(&tpl, data); err != nil {
		return "", err
	}
	indexHTML := tpl.String()
	return indexHTML, nil
}

func ginLogFormatter() func(param gin.LogFormatterParams) string {
	return func(param gin.LogFormatterParams) string {
		var statusColor, methodColor, resetColor string
		if param.IsOutputColor() {
			statusColor = param.StatusCodeColor()
			methodColor = param.MethodColor()
			resetColor = param.ResetColor()
		}

		if param.Latency > time.Minute {
			param.Latency = param.Latency.Truncate(time.Second)
		}
		return fmt.Sprintf("%v |%s %3d %s| %13v | %15s |%s %-7s %s %#v\n%s",
			param.TimeStamp.Format("2006/01/02 15:04:05"),
			statusColor, param.StatusCode, resetColor,
			param.Latency,
			param.ClientIP,
			methodColor, param.Method, resetColor,
			handler.Sanitize(param.Path),
			param.ErrorMessage,
		)
	}
}
