package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/cvbarros/go-teamcity/teamcity"
	"github.com/prometheus/client_golang/prometheus"
	logrus "github.com/sirupsen/logrus"
	viper "github.com/spf13/viper"
)

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

type TeamCityBuildsCollector struct {
	client *teamcity.Client

	buildStartTime  *prometheus.Desc
	buildFinishTime *prometheus.Desc
	buildState      *prometheus.Desc
	buildStatus     *prometheus.Desc
}

func NewTeamCityBuildsCollector(client *teamcity.Client) *TeamCityBuildsCollector {
	constLabels := prometheus.Labels{}

	return &TeamCityBuildsCollector{
		// Set the TeamCity client.
		client: client,

		// Build metric descriptions.
		buildStartTime: prometheus.NewDesc(
			"teamcity_build_start_time",
			"The start time of a TeamCity build job.",
			[]string{"project_id", "build_type_id", "build_id"},
			constLabels,
		),

		buildFinishTime: prometheus.NewDesc(
			"teamcity_build_finish_time",
			"The finish time of a TeamCity build job.",
			[]string{"project_id", "build_type_id", "build_id"},
			constLabels,
		),

		buildState: prometheus.NewDesc(
			"teamcity_build_state",
			"The state of a TeamCity build job.",
			[]string{"project_id", "build_type_id", "build_id"},
			constLabels,
		),

		buildStatus: prometheus.NewDesc(
			"teamcity_build_status",
			"The status of a TeamCity build job.",
			[]string{"project_id", "build_type_id", "build_id"},
			constLabels,
		),
	}
}

func (collector TeamCityBuildsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- collector.buildFinishTime
	ch <- collector.buildStartTime
	ch <- collector.buildState
	ch <- collector.buildStatus
}

func (collector TeamCityBuildsCollector) Collect(ch chan<- prometheus.Metric) {
	logrus.Info("collecting TeamCity builds metrics")

	err := collector.collectBuildMetrics(viper.GetString("root.project.id"), ch)
	if err != nil {
		logrus.Error(err)
	}
}

func (collector *TeamCityBuildsCollector) collectBuildMetrics(identifier string, ch chan<- prometheus.Metric) error {
	logger := logrus.WithFields(logrus.Fields{"project": identifier})

	logger.Info("collecting project")
	p, err := collector.client.Projects.GetByID(identifier)
	if err != nil {
		return err
	}

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
			err := collector.collectBuildMetrics(identifier, ch)
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

func (collector *TeamCityBuildsCollector) collectProjectBuildMetrics(identifier string, ch chan<- prometheus.Metric) error {
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
