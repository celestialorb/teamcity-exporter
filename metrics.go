package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/cvbarros/go-teamcity/teamcity"
	"github.com/prometheus/client_golang/prometheus"
	logrus "github.com/sirupsen/logrus"
	viper "github.com/spf13/viper"
)

type TeamCityCollector struct {
	client *teamcity.Client

	// TeamCity project metrics.
	projects   *prometheus.Desc
	buildTypes *prometheus.Desc

	// TeamCity build metrics.
	buildStartTime  *prometheus.Desc
	buildFinishTime *prometheus.Desc
	buildState      *prometheus.Desc
	buildStatus     *prometheus.Desc
}

func NewTeamCityCollector() *TeamCityCollector {
	return &TeamCityCollector{
		projects: prometheus.NewDesc(
			"teamcity_projects",
			"The total number of subprojects for a TeamCity project.",
			[]string{"project_id"},
			prometheus.Labels{},
		),
		buildTypes: prometheus.NewDesc(
			"teamcity_project_build_types",
			"The total number of build types for a TeamCity project.",
			[]string{"project_id"},
			prometheus.Labels{},
		),

		buildStartTime: prometheus.NewDesc(
			"teamcity_build_start_time",
			"The start time of a TeamCity build job.",
			[]string{"project_id", "build_type_id", "build_id"},
			prometheus.Labels{},
		),

		buildFinishTime: prometheus.NewDesc(
			"teamcity_build_finish_time",
			"The finish time of a TeamCity build job.",
			[]string{"project_id", "build_type_id", "build_id"},
			prometheus.Labels{},
		),

		buildState: prometheus.NewDesc(
			"teamcity_build_state",
			"The state of a TeamCity build job.",
			[]string{"project_id", "build_type_id", "build_id"},
			prometheus.Labels{},
		),

		buildStatus: prometheus.NewDesc(
			"teamcity_build_status",
			"The status of a TeamCity build job.",
			[]string{"project_id", "build_type_id", "build_id"},
			prometheus.Labels{},
		),
	}
}

type TeamCityTime struct {
	time.Time
}

func (t *TeamCityTime) UnmarshalJSON(b []byte) error {
	text := strings.Trim(string(b), "\"")
	tm, err := time.Parse("20060102T150405-0700", text)
	t.Time = tm
	return err
}

func (collector TeamCityCollector) Describe(ch chan<- *prometheus.Desc) {}
func (collector TeamCityCollector) Collect(ch chan<- prometheus.Metric) {
	logrus.Info("collecting TeamCity metrics")

	err := collector.collectProjectMetrics(viper.GetString("root.project.id"), ch)
	if err != nil {
		logrus.Error(err)
	}
}

func (collector *TeamCityCollector) collectProjectMetrics(identifier string, ch chan<- prometheus.Metric) error {
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

	// Collect metrics on builds for the project.
	err = collector.collectProjectBuildMetrics(p.ID, ch)
	if err != nil {
		return err
	}

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

func (collector *TeamCityCollector) collectProjectBuildMetrics(identifier string, ch chan<- prometheus.Metric) error {
	logger := logrus.WithFields(logrus.Fields{"project": identifier})

	url := fmt.Sprintf(
		"%s/app/rest/builds?locator=count:%d,project:id:%s&fields=count,nextHref,build(id,buildTypeId,status,state,startDate,finishDate)",
		viper.GetString("addr"),
		viper.GetUint("page.count"),
		identifier,
	)

	client := collector.client.HTTPClient
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", viper.GetString("token")))
	response, err := client.Do(request)
	if err != nil {
		return err
	}

	if response.StatusCode == http.StatusNotFound {
		return nil
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}

	builds := BuildResponse{}
	err = json.Unmarshal(body, &builds)
	if err != nil {
		return err
	}

	logger.WithFields(logrus.Fields{"count": builds.Count}).Info("found builds")
	for _, build := range builds.Builds {
		labels := []string{identifier, build.BuildTypeID, fmt.Sprintf("%d", build.ID)}

		// Set the build start time metric.
		ch <- prometheus.MustNewConstMetric(
			collector.buildStartTime,
			prometheus.GaugeValue,
			float64(build.StartDate.Unix()),
			labels...,
		)

		// Set the build finish time metric.
		ch <- prometheus.MustNewConstMetric(
			collector.buildFinishTime,
			prometheus.GaugeValue,
			float64(build.FinishDate.Unix()),
			labels...,
		)

		// Set the build "status" metric.
		ch <- prometheus.MustNewConstMetric(
			collector.buildStatus,
			prometheus.GaugeValue,
			float64(ParseBuildStatus(build.Status)),
			labels...,
		)

		// Set the build "state" metric.
		ch <- prometheus.MustNewConstMetric(
			collector.buildState,
			prometheus.GaugeValue,
			float64(ParseBuildState(build.State)),
			labels...,
		)
	}

	// Check for another request that we need to make to continue to get builds.
	if builds.NextHRef != "" {
		logger.Fatal("multipage requests are not yet supported")
	}

	return nil
}

type BuildStatus int
type BuildState int

const (
	BuildStateUnknown BuildState = iota
	BuildQueued
	BuildFinished
	BuildRunning
	BuildDeleted
)

const (
	BuildStatusUnknown BuildStatus = iota
	BuildSuccess
	BuildFailure
)

func ParseBuildState(s string) BuildState {
	switch s {
	case "queued":
		return BuildQueued
	case "finished":
		return BuildFinished
	case "running":
		return BuildRunning
	case "deleted":
		return BuildDeleted
	}
	return BuildStateUnknown
}

func ParseBuildStatus(s string) BuildStatus {
	switch s {
	case "SUCCESS":
		return BuildSuccess
	case "FAILURE":
		return BuildFailure
	}
	return BuildStatusUnknown
}

type Build struct {
	ID          uint64       `json:"id"`
	BuildTypeID string       `json:"buildTypeId"`
	Status      string       `json:"status"`
	State       string       `json:"state"`
	StartDate   TeamCityTime `json:"startDate,omitempty"`
	FinishDate  TeamCityTime `json:"finishDate,omitempty"`
}

type BuildResponse struct {
	Count    uint64  `json:"count"`
	HRef     string  `json:"href,omitempty"`
	NextHRef string  `json:"nextHref,omitempty"`
	PrevHRef string  `json:"prevHref,omitempty"`
	Builds   []Build `json:"build"`
}
