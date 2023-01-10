# teamcity-exporter
A simple Golang project to export TeamCity metrics as Prometheus metrics.

## Configuration

This project uses Viper for its simple configuration. At present, it only supports token authentication to TeamCity.
The various configuration elements are listed in the table below along with a description and their corresponding
environment variable.

| Element               | Description                                   | Variable                   | Default    |
|-----------------------|-----------------------------------------------|----------------------------|------------|
| TeamCity Address      | The address of the TeamCity server.           | `TEAMCITY_ADDR`            | N/A        |
| TeamCity Token        | The token used to access the TeamCity API.    | `TEAMCITY_TOKEN`           | N/A        |
| TeamCity Root Project | The ID of the project to collect metrics for. | `TEAMCITY_ROOT_PROJECT_ID` | `_Root`    |
| Metrics Path          | The path to expose the metrics endpoint on.   | `TEAMCITY_METRICS_PATH`    | `/metrics` |
| Metrics Port          | The port to expose the metrics endpoint on.   | `TEAMCITY_METRICS_PORT`    | `2112`     |

## Metrics

The metrics exported by this exporter are described in the table below.

| Name                           | Description                                             | Labels                                    |
|--------------------------------|---------------------------------------------------------|-------------------------------------------|
| `teamcity_projects`            | The total number of subprojects for a TeamCity project. | `project_id`                              |
| `teamcity_project_build_types` | The total number of build types for a TeamCity project. | `project_id`                              |
| `teamcity_build_start_time`    | The start time of a TeamCity build job.                 | `project_id`, `build_type_id`, `build_id` |
| `teamcity_build_finish_time`   | The finish time of a TeamCity build job.                | `project_id`, `build_type_id`, `build_id` |
| `teamcity_build_state`         | The state of a TeamCity build job.                      | `project_id`, `build_type_id`, `build_id` |
| `teamcity_build_status`        | The status of a TeamCity build job.                     | `project_id`, `build_type_id`, `build_id` |

### Build State

The mapping of TeamCity build state values is described in the table below.

| Name       | Value |
|------------|-------|
| `unknown`  | `0`   |
| `queued`   | `1`   |
| `finished` | `2`   |
| `running`  | `3`   |
| `deleted`  | `4`   |

### Build Status

The mapping of TeamCity build status values is described in the table below.

| Name      | Value |
|-----------|-------|
| `UNKNOWN` | `0`   |
| `SUCCESS` | `1`   |
| `FAILURE` | `2`   |