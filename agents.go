package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/cvbarros/go-teamcity/teamcity"
	"github.com/prometheus/client_golang/prometheus"
	logrus "github.com/sirupsen/logrus"
	viper "github.com/spf13/viper"
)

type Agent struct {
	ID           uint64 `json:"id"`
	Name         string `json:"name"`
	Authorized   bool   `json:"authorized"`
	Connected    bool   `json:"connected"`
	Enabled      bool   `json:"enabled"`
	CurrentBuild Build  `json:"build"`
}

type AgentsResponse struct {
	Count    uint64  `json:"count"`
	HRef     string  `json:"href,omitempty"`
	NextHRef string  `json:"nextHref,omitempty"`
	PrevHRef string  `json:"prevHref,omitempty"`
	Agents   []Agent `json:"agent"`
}

type TeamCityAgentCollector struct {
	client *teamcity.Client

	agentAuthorized     *prometheus.Desc
	agentConnected      *prometheus.Desc
	agentEnabled        *prometheus.Desc
	agentCurrentBuildId *prometheus.Desc
}

func NewTeamCityAgentCollector(client *teamcity.Client) *TeamCityAgentCollector {
	constLabels := prometheus.Labels{}

	return &TeamCityAgentCollector{
		client: client,

		// Agent metrics descriptions.
		agentAuthorized: prometheus.NewDesc(
			"teamcity_agent_authorized",
			"The authorized status of a TeamCity agent.",
			[]string{"agent_id", "agent_name"},
			constLabels,
		),

		agentConnected: prometheus.NewDesc(
			"teamcity_agent_connected",
			"The connected status of a TeamCity agent.",
			[]string{"agent_id", "agent_name"},
			constLabels,
		),

		agentEnabled: prometheus.NewDesc(
			"teamcity_agent_enabled",
			"The enabled status of a TeamCity agent.",
			[]string{"agent_id", "agent_name"},
			constLabels,
		),

		agentCurrentBuildId: prometheus.NewDesc(
			"teamcity_agent_current_build_id",
			"The build ID of the current build of a TeamCity agent.",
			[]string{"agent_id", "agent_name"},
			constLabels,
		),
	}
}

func (collector TeamCityAgentCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- collector.agentAuthorized
	ch <- collector.agentConnected
	ch <- collector.agentEnabled
	ch <- collector.agentCurrentBuildId
}

func (collector TeamCityAgentCollector) Collect(ch chan<- prometheus.Metric) {
	logrus.Info("collecting TeamCity agent metrics")

	url := fmt.Sprintf(
		"%s/app/rest/agents?locator=count:%d&fields=count,nextHref,agent(id,name,authorized,connected,enabled,build(id))",
		viper.GetString("addr"),
		viper.GetUint("page.count"),
	)

	client := collector.client.HTTPClient
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		logrus.Error(err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", viper.GetString("token")))
	response, err := client.Do(request)
	if err != nil {
		logrus.Error(err)
	}

	if response.StatusCode == http.StatusNotFound {
		return
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		logrus.Error(err)
	}

	agents := AgentsResponse{}
	err = json.Unmarshal(body, &agents)
	if err != nil {
		logrus.Error(err)
	}

	// Check for another request that we need to make to continue to get builds.
	if agents.NextHRef != "" {
		logrus.Fatal("multipage requests are not yet supported")
	}

	for _, agent := range agents.Agents {
		labels := []string{fmt.Sprintf("%d", agent.ID), agent.Name}

		// Set the agent authorized metric.
		ch <- prometheus.MustNewConstMetric(
			collector.agentAuthorized,
			prometheus.GaugeValue,
			float64(map[bool]int{true: 1, false: 0}[agent.Authorized]),
			labels...,
		)

		// Set the agent connected metric.
		ch <- prometheus.MustNewConstMetric(
			collector.agentConnected,
			prometheus.GaugeValue,
			float64(map[bool]int{true: 1, false: 0}[agent.Connected]),
			labels...,
		)

		// Set the agent enabled metric.
		ch <- prometheus.MustNewConstMetric(
			collector.agentEnabled,
			prometheus.GaugeValue,
			float64(map[bool]int{true: 1, false: 0}[agent.Enabled]),
			labels...,
		)

		// Set the current build ID for the agent metric.
		ch <- prometheus.MustNewConstMetric(
			collector.agentCurrentBuildId,
			prometheus.GaugeValue,
			float64(agent.CurrentBuild.ID),
			labels...,
		)
	}
}
