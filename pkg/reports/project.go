/*******************************************************************************
*
* Copyright 2017 SAP SE
*
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You should have received a copy of the License along with this
* program. If not, you may obtain a copy of the License at
*
*     http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
*
*******************************************************************************/

package reports

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"database/sql"
	"github.com/sapcc/limes/pkg/db"
	"github.com/sapcc/limes/pkg/limes"
	"github.com/sapcc/limes/pkg/util"
	"time"
)

//Project contains all data about resource usage in a project.
type Project struct {
	UUID       string          `json:"id"`
	Name       string          `json:"name"`
	ParentUUID string          `json:"parent_id"`
	Services   ProjectServices `json:"services,keepempty"`
}

//ProjectService is a substructure of Project containing data for
//a single backend service.
type ProjectService struct {
	limes.ServiceInfo
	Resources ProjectResources  `json:"resources,keepempty"`
	Rates     ProjectRateLimits `json:"rates,omitempty"`
	ScrapedAt int64             `json:"scraped_at,omitempty"`
}

//ProjectResource is a substructure of Project containing data for
//a single resource.
type ProjectResource struct {
	limes.ResourceInfo
	Quota uint64 `json:"quota,keepempty"`
	Usage uint64 `json:"usage,keepempty"`
	//This is a pointer to a value to enable precise control over whether this field is rendered in output.
	BackendQuota *int64          `json:"backend_quota,omitempty"`
	Subresources util.JSONString `json:"subresources,omitempty"`
}

//ProjectRateLimit is the structure for rate limits per target type URI and their limited actions
type ProjectRateLimit struct {
	TargetTypeURI string           `json:"target_type_uri,keepempty"`
	Actions       RateLimitActions `json:"actions,keepempty"`
}

//RateLimitAction is defines an action and its rate limit
type RateLimitAction struct {
	Name  string `json:"name,keepempty"`
	Limit string `json:"limit,keepempty"`
}

//ProjectServices provides fast lookup of services using a map, but serializes
//to JSON as a list.
type ProjectServices map[string]*ProjectService

//MarshalJSON implements the json.Marshaler interface.
func (s ProjectServices) MarshalJSON() ([]byte, error) {
	//serialize with ordered keys to ensure testcase stability
	types := make([]string, 0, len(s))
	for typeStr := range s {
		types = append(types, typeStr)
	}
	sort.Strings(types)
	list := make([]*ProjectService, len(s))
	for idx, typeStr := range types {
		list[idx] = s[typeStr]
	}
	return json.Marshal(list)
}

//UnmarshalJSON implements the json.Unmarshaler interface
func (s *ProjectServices) UnmarshalJSON(b []byte) error {
	tmp := make([]*ProjectService, 0)
	err := json.Unmarshal(b, &tmp)
	if err != nil {
		return err
	}
	t := make(ProjectServices)
	for _, ps := range tmp {
		t[ps.Type] = ps
	}
	*s = ProjectServices(t)
	return nil
}

//ProjectResources provides fast lookup of resources using a map, but serializes
//to JSON as a list.
type ProjectResources map[string]*ProjectResource

//MarshalJSON implements the json.Marshaler interface.
func (r ProjectResources) MarshalJSON() ([]byte, error) {
	//serialize with ordered keys to ensure testcase stability
	names := make([]string, 0, len(r))
	for name := range r {
		names = append(names, name)
	}
	sort.Strings(names)
	list := make([]*ProjectResource, len(r))
	for idx, name := range names {
		list[idx] = r[name]
	}
	return json.Marshal(list)
}

//UnmarshalJSON implements the json.Unmarshaler interface
func (r *ProjectResources) UnmarshalJSON(b []byte) error {
	tmp := make([]*ProjectResource, 0)
	err := json.Unmarshal(b, &tmp)
	if err != nil {
		return err
	}
	t := make(ProjectResources)
	for _, pr := range tmp {
		t[pr.Name] = pr
	}
	*r = ProjectResources(t)
	return nil
}

//ProjectRateLimits provides a fast lookup of rate limits using a map, but serializes
//to JSON as a list.
type ProjectRateLimits map[string]*ProjectRateLimit

//MarshalJSON implements the json.Marshaler interface.
func (r ProjectRateLimits) MarshalJSON() ([]byte, error) {
	//serialize with ordered keys to ensure testcase stability
	names := make([]string, 0, len(r))
	for name := range r {
		names = append(names, name)
	}
	sort.Strings(names)
	list := make([]*ProjectRateLimit, len(r))
	for idx, name := range names {
		list[idx] = r[name]
	}
	return json.Marshal(list)
}

//UnmarshalJSON implements the json.Unmarshaler interface
func (r *ProjectRateLimits) UnmarshalJSON(b []byte) error {
	tmp := make([]*ProjectRateLimit, 0)
	err := json.Unmarshal(b, &tmp)
	if err != nil {
		return err
	}
	t := make(ProjectRateLimits)
	for _, pr := range tmp {
		t[pr.TargetTypeURI] = pr
	}
	*r = ProjectRateLimits(t)
	return nil
}


//RateLimitActions provides a fast lookup of rate limit actions using a map, but serializes
//to JSON as a list.
type RateLimitActions map[string]*RateLimitAction

//MarshalJSON implements the json.Marshaler interface.
func (r RateLimitActions) MarshalJSON() ([]byte, error) {
	//serialize with ordered keys to ensure testcase stability
	actionNames := make([]string, 0, len(r))
	for actionName := range r {
		actionNames = append(actionNames, actionName)
	}
	sort.Strings(actionNames)
	list := make([]*RateLimitAction, len(r))
	for idx, name := range actionNames {
		list[idx] = r[name]
	}
	return json.Marshal(list)
}

//UnmarshalJSON implements the json.Unmarshaler interface
func (r *RateLimitActions) UnmarshalJSON(b []byte) error {
	tmp := make([]*RateLimitAction, 0)
	err := json.Unmarshal(b, &tmp)
	if err != nil {
		return err
	}
	t := make(RateLimitActions)
	for _, pr := range tmp {
		t[pr.Name] = pr
	}
	*r = RateLimitActions(t)
	return nil
}

var projectReportQuery = `
	SELECT p.uuid, p.name, COALESCE(p.parent_uuid, ''), ps.type, ps.scraped_at, pr.name, pr.quota, pr.usage, pr.backend_quota, pr.subresources
	  FROM projects p
	  LEFT OUTER JOIN project_services ps ON ps.project_id = p.id {{AND ps.type = $service_type}}
	  LEFT OUTER JOIN project_resources pr ON pr.service_id = ps.id {{AND pr.name = $resource_name}}
	 WHERE %s
`

var projectRatesReportQuery = `
	SELECT p.uuid, ps.type, prl.target_type_uri, prl.action, prl.rate_limit
	  FROM projects p
	  LEFT OUTER JOIN project_services ps ON ps.project_id = p.id {{AND ps.type = $service_type}}
	  LEFT OUTER JOIN project_rate_limits prl ON prl.service_id = ps.id
	 WHERE %s
`

//GetProjects returns Project reports for all projects in the given domain or,
//if projectID is non-nil, for that project only.
func GetProjects(cluster *limes.Cluster, domainID int64, projectID *int64, dbi db.Interface, filter ProjectFilter) ([]*Project, error) {
	fields := map[string]interface{}{"p.domain_id": domainID}
	if projectID != nil {
		fields["p.id"] = *projectID
	}

	//avoid collecting the potentially large subresources strings when possible
	queryStr := projectReportQuery
	if !filter.withSubResources {
		queryStr = strings.Replace(queryStr, "pr.subresources", "''", 1)
	}

	projects := make(projects)

	//query project report
	queryStr, joinArgs := filter.PrepareQuery(queryStr)
	whereStr, whereArgs := db.BuildSimpleWhereClause(fields, len(joinArgs))
	err := db.ForeachRow(db.DB, fmt.Sprintf(queryStr, whereStr), append(joinArgs, whereArgs...), func(rows *sql.Rows) error {
		var (
			projectUUID       string
			projectName       string
			projectParentUUID string
			serviceType       *string
			scrapedAt         *util.Time
			resourceName      *string
			quota             *uint64
			usage             *uint64
			backendQuota      *int64
			subresources      *string
		)
		err := rows.Scan(
			&projectUUID, &projectName, &projectParentUUID,
			&serviceType, &scrapedAt, &resourceName,
			&quota, &usage, &backendQuota, &subresources,
		)
		if err != nil {
			return err
		}

		project, service, resource := projects.Find(cluster, projectUUID, serviceType, resourceName, nil, nil)

		project.Name = projectName
		project.ParentUUID = projectParentUUID

		if service != nil {
			if scrapedAt != nil {
				service.ScrapedAt = time.Time(*scrapedAt).Unix()
			}
		}

		if resource != nil {
			if subresources != nil {
				resource.Subresources = util.JSONString(*subresources)
			}

			if usage != nil {
				resource.Usage = *usage
			}
			if quota != nil {
				resource.Quota = *quota
				if backendQuota != nil && (*backendQuota < 0 || uint64(*backendQuota) != *quota) {
					resource.BackendQuota = backendQuota
				}
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	//query project rate limits
	if filter.withRates {
		queryStr, joinArgs = filter.PrepareQuery(projectRatesReportQuery)
		whereStr, whereArgs = db.BuildSimpleWhereClause(fields, len(joinArgs))
		err = db.ForeachRow(db.DB, fmt.Sprintf(queryStr, whereStr), append(joinArgs, whereArgs...), func(rows *sql.Rows) error {
			var (
				projectUUID string
				serviceID,
				targetTypeURI,
				action,
				limit *string
			)
			err := rows.Scan(&projectUUID, &serviceID, &targetTypeURI, &action, &limit)
			if err != nil {
				return err
			}

			_, service, _ := projects.Find(cluster, projectUUID, serviceID, nil, targetTypeURI, action)

			if service != nil {
				rate, exists := service.Rates[*targetTypeURI]
				if !exists {
					rate.Actions[*action] = &RateLimitAction{
						Name:  *action,
						Limit: *limit,
					}
				}
				rate.Actions[*action].Name = *action
				rate.Actions[*action].Limit = *limit

				// show only rates
				if filter.onlyRates {
					service.Resources = nil
				}

			}

			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	//flatten result (with stable order to keep the tests happy)
	uuids := make([]string, 0, len(projects))
	for uuid := range projects {
		uuids = append(uuids, uuid)
	}
	sort.Strings(uuids)
	result := make([]*Project, len(projects))
	for idx, uuid := range uuids {
		project := projects[uuid]
		for _, service := range project.Services {
			service.Rates = sortRateLimits(service.Rates)
		}

		result[idx] = projects[uuid]
	}

	return result, nil
}

func sortRateLimits(limits ProjectRateLimits) ProjectRateLimits {
	if limits == nil {
		return nil
	}

	targetTypeURIs := make([]string, 0, len(limits))

	for ttu := range limits {
		targetTypeURIs = append(targetTypeURIs, ttu)
	}
	sort.Strings(targetTypeURIs)

	result := make(ProjectRateLimits, len(limits))
	for _, ttu := range targetTypeURIs {
		limits[ttu].Actions = sortRateLimitActions(limits[ttu].Actions)
		result[ttu] = limits[ttu]
	}
	return result
}

func sortRateLimitActions(actions RateLimitActions) RateLimitActions {
	if actions == nil {
		return nil
	}

	actionNames := make([]string, 0, len(actions))

	for name := range actions {
		actionNames = append(actionNames, name)
	}
	sort.Strings(actionNames)

	result := make(RateLimitActions, len(actions))
	for _, name := range actionNames {
		result[name] = actions[name]
	}

	return result
}

type projects map[string]*Project

func (p projects) Find(cluster *limes.Cluster, projectUUID string, serviceType, resourceName, targetTypeURI, action *string) (*Project, *ProjectService, *ProjectResource) {
	project, exists := p[projectUUID]
	if !exists {
		project = &Project{
			UUID:     projectUUID,
			Services: make(ProjectServices),
		}
		p[projectUUID] = project
	}

	if serviceType == nil {
		return project, nil, nil
	}

	service, exists := project.Services[*serviceType]
	if !exists {
		if !cluster.HasService(*serviceType) {
			return project, nil, nil
		}

		service = &ProjectService{
			ServiceInfo: cluster.InfoForService(*serviceType),
			Resources:   make(ProjectResources),
			Rates:       make(ProjectRateLimits),
		}

		project.Services[*serviceType] = service
	}

	var resource *ProjectResource
	if resourceName != nil && cluster.HasResource(*serviceType, *resourceName) {
		resource, exists = service.Resources[*resourceName]
		if !exists {
			resource = &ProjectResource{
				ResourceInfo: cluster.InfoForResource(*serviceType, *resourceName),
			}
			service.Resources[*resourceName] = resource
		}
	}

	if targetTypeURI == nil {
		return project, service, resource
	}

	rate, exists := service.Rates[*targetTypeURI]
	if !exists {
		rate = &ProjectRateLimit{
			TargetTypeURI: *targetTypeURI,
			Actions:       make(RateLimitActions),
		}
		service.Rates[*targetTypeURI] = rate
	}

	if action == nil {
		return project, service, resource
	}

	_, exists = rate.Actions[*action]
	if !exists {
		rate.Actions[*action] = &RateLimitAction{}
	}

	return project, service, resource
}
