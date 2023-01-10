package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/cvbarros/go-teamcity/teamcity"
	logrus "github.com/sirupsen/logrus"
	viper "github.com/spf13/viper"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	// Setup mapping of environment variables to configuration elements.
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.SetEnvPrefix("TEAMCITY")
	viper.AutomaticEnv()

	// Set defaults for logging configuration.
	viper.SetDefault("logging.format", "json")
	viper.SetDefault("logging.level", "info")

	// Set defaults for TeamCity API configuration.
	viper.SetDefault("page.count", 10000)
	viper.SetDefault("root.project.id", "_Root")

	// Set defaults for exporting metrics.
	viper.SetDefault("metrics.listen", "0.0.0.0")
	viper.SetDefault("metrics.path", "/metrics")
	viper.SetDefault("metrics.port", 2112)

	// Setup our logging system, first parse and set the level defaulting to INFO if we can't determine it.
	level, err := logrus.ParseLevel(viper.GetString("logging.level"))
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"level": viper.GetString("logging.level"),
		}).Warn("invalid logrus logging level, using INFO")
		level = logrus.InfoLevel
	}
	logrus.SetLevel(level)

	// Setup the formatter for our logger.
	switch viper.GetString("logging.format") {
	case "text":
		fallthrough
	case "logfmt":
		logrus.SetFormatter(&logrus.TextFormatter{})
	case "json":
		fallthrough
	default:
		logrus.SetFormatter(&logrus.JSONFormatter{})
	}
	logrus.WithFields(logrus.Fields{
		"logging.format": viper.GetString("logging.format"),
		"logging.level":  logrus.GetLevel(),
	}).Debug("logger configuration finished")

	logrus.Info("initialize TeamCity exporter configuration")
	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 10
	retryClient.Logger = nil

	httpClient := retryClient.StandardClient()

	client, err := teamcity.NewClientWithAddress(
		teamcity.TokenAuth(viper.GetString("token")),
		viper.GetString("addr"),
		httpClient,
	)
	if err != nil {
		logrus.Error(err)
	}

	collector := NewTeamCityCollector()
	collector.client = client

	logrus.Info("registering TeamCity metrics collector")
	prometheus.MustRegister(collector)

	http.Handle(viper.GetString("metrics.path"), promhttp.Handler())
	err = http.ListenAndServe(fmt.Sprintf("%s:%d", viper.GetString("metrics.listen"), viper.GetInt("metrics.port")), nil)
	if err != nil {
		logrus.Fatal(err)
	}
}
