package main

import (
	"sync"

	"github.com/cvbarros/go-teamcity/teamcity"
	"github.com/prometheus/client_golang/prometheus"
	logrus "github.com/sirupsen/logrus"
	viper "github.com/spf13/viper"
)

type TeamCityProjectsCollector struct {
	client *teamcity.Client

	buildTypes *prometheus.Desc
	projects   *prometheus.Desc
}

func NewTeamCityProjectsCollector(client *teamcity.Client) *TeamCityProjectsCollector {
	constLabels := prometheus.Labels{}

	return &TeamCityProjectsCollector{
		// Set the TeamCity client.
		client: client,

		buildTypes: prometheus.NewDesc(
			"teamcity_project_build_types_total",
			"The total number of build types for a TeamCity project.",
			[]string{"project_id"},
			constLabels,
		),
		projects: prometheus.NewDesc(
			"teamcity_projects_total",
			"The total number of subprojects for a TeamCity project.",
			[]string{"project_id"},
			constLabels,
		),
	}
}

func (collector TeamCityProjectsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- collector.buildTypes
	ch <- collector.projects
}

func (collector TeamCityProjectsCollector) Collect(ch chan<- prometheus.Metric) {
	logrus.Info("collecting TeamCity project metrics")

	err := collector.collectProjectMetrics(viper.GetString("root.project.id"), ch)
	if err != nil {
		logrus.Error(err)
	}
}

func (collector *TeamCityProjectsCollector) collectProjectMetrics(identifier string, ch chan<- prometheus.Metric) error {
	logger := logrus.WithFields(logrus.Fields{"project": identifier})

	logger.Info("collecting project")
	p, err := collector.client.Projects.GetByID(identifier)
	if err != nil {
		return err
	}

	// Set the subproject count metric.
	logger.WithFields(logrus.Fields{"value": p.ChildProjects.Count}).Debug("setting project count metric")
	ch <- prometheus.MustNewConstMetric(
		collector.projects,
		prometheus.GaugeValue,
		float64(p.ChildProjects.Count),
		p.ID,
	)

	// Set the build type count metric.
	logger.WithFields(logrus.Fields{"value": p.BuildTypes.Count}).Debug("setting build type count metric")
	ch <- prometheus.MustNewConstMetric(
		collector.buildTypes,
		prometheus.GaugeValue,
		float64(p.BuildTypes.Count),
		p.ID,
	)

	// Collect metrics on the subprojects.
	wg := sync.WaitGroup{}
	wg.Add(p.ChildProjects.Count)
	for _, subproject := range p.ChildProjects.Items {
		go func(identifier string) {
			err := collector.collectProjectMetrics(identifier, ch)
			if err != nil {
				logger.Error(err)
			}
			wg.Done()
		}(subproject.ID)
	}

	logger.Debug("waiting for project collection")
	wg.Wait()
	logger.Debug("project collection finished")

	return nil
}
