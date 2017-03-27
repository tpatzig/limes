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
	"time"

	"github.com/sapcc/limes/pkg/db"
	"github.com/sapcc/limes/pkg/limes"
	"github.com/sapcc/limes/pkg/util"
)

//Cluster contains aggregated data about resource usage in a cluster.
type Cluster struct {
	ID           string          `json:"id"`
	Services     ClusterServices `json:"services,keepempty"`
	MaxScrapedAt *int64          `json:"max_scraped_at,omitempty"`
	MinScrapedAt *int64          `json:"min_scraped_at,omitempty"`
}

//ClusterService is a substructure of Cluster containing data for
//a single backend service.
type ClusterService struct {
	Type         string           `json:"type"`
	Resources    ClusterResources `json:"resources,keepempty"`
	MaxScrapedAt int64            `json:"max_scraped_at,keepempty"`
	MinScrapedAt int64            `json:"min_scraped_at,keepempty"`
}

//ClusterResource is a substructure of Cluster containing data for
//a single resource.
type ClusterResource struct {
	Name         string     `json:"name"`
	Unit         limes.Unit `json:"unit,omitempty"`
	Capacity     *uint64    `json:"capacity,omitempty"`
	DomainsQuota uint64     `json:"domains_quota,keepempty"`
	Usage        uint64     `json:"usage,keepempty"`
}

//ClusterServices provides fast lookup of services using a map, but serializes
//to JSON as a list.
type ClusterServices map[string]*ClusterService

//MarshalJSON implements the json.Marshaler interface.
func (s ClusterServices) MarshalJSON() ([]byte, error) {
	//serialize with ordered keys to ensure testcase stability
	types := make([]string, 0, len(s))
	for typeStr := range s {
		types = append(types, typeStr)
	}
	sort.Strings(types)
	list := make([]*ClusterService, len(s))
	for idx, typeStr := range types {
		list[idx] = s[typeStr]
	}
	return json.Marshal(list)
}

//ClusterResources provides fast lookup of resources using a map, but serializes
//to JSON as a list.
type ClusterResources map[string]*ClusterResource

//MarshalJSON implements the json.Marshaler interface.
func (r ClusterResources) MarshalJSON() ([]byte, error) {
	//serialize with ordered keys to ensure testcase stability
	names := make([]string, 0, len(r))
	for name := range r {
		names = append(names, name)
	}
	sort.Strings(names)
	list := make([]*ClusterResource, len(r))
	for idx, name := range names {
		list[idx] = r[name]
	}
	return json.Marshal(list)
}

var clusterReportQuery1 = `
	SELECT d.cluster_id, ps.type, pr.name, SUM(pr.usage), MIN(ps.scraped_at), MAX(ps.scraped_at)
	  FROM domains d
	  JOIN projects p ON p.domain_id = d.id
	  JOIN project_services ps ON ps.project_id = p.id
	  JOIN project_resources pr ON pr.service_id = ps.id
	 WHERE %s GROUP BY d.cluster_id, ps.type, pr.name
`

var clusterReportQuery2 = `
	SELECT d.cluster_id, ds.type, dr.name, SUM(dr.quota)
	  FROM domains d
	  JOIN domain_services ds ON ds.domain_id = d.id
	  JOIN domain_resources dr ON dr.service_id = ds.id
	 WHERE %s GROUP BY d.cluster_id, ds.type, dr.name
`

var clusterReportQuery3 = `
	SELECT cs.cluster_id, cs.type, cr.name, cr.capacity, cs.scraped_at
	  FROM cluster_services cs
	  JOIN cluster_resources cr ON cr.service_id = cs.id
	 WHERE %s
`

var clusterReportQuery4 = `
	SELECT ds.type, dr.name, SUM(dr.quota)
	  FROM domain_services ds
	  JOIN domain_resources dr ON dr.service_id = ds.id
	 WHERE %s GROUP BY ds.type, dr.name
`

var clusterReportQuery5 = `
	SELECT ps.type, pr.name, SUM(pr.usage)
	  FROM project_services ps
	  JOIN project_resources pr ON pr.service_id = ps.id
	 WHERE %s GROUP BY ps.type, pr.name
`

//GetClusters returns Cluster reports for al clusters or, if clusterID is
//non-nil, for that cluster only.
//
//In contrast to nearly everything else in Limes, this needs the full
//limes.Configuration (instead of just the current limes.ClusterConfiguration)
//to look at the services enabled in other clusters.
func GetClusters(config limes.Configuration, clusterID *string, dbi db.Interface, filter Filter) ([]*Cluster, error) {
	//first query: collect project usage data in these clusters
	whereStr, queryArgs := db.BuildSimpleWhereClause(makeClusterFilter("d", "ps", "pr", clusterID, filter))
	rows, err := dbi.Query(fmt.Sprintf(clusterReportQuery1, whereStr), queryArgs...)
	if err != nil {
		return nil, err
	}

	clusters := make(clusters)
	err = db.ForeachRow(rows, func() error {
		var (
			clusterID    string
			serviceType  string
			resourceName string
			usage        uint64
			minScrapedAt util.Time
			maxScrapedAt util.Time
		)
		err := rows.Scan(&clusterID, &serviceType, &resourceName, &usage, &minScrapedAt, &maxScrapedAt)
		if err != nil {
			return err
		}

		_, service, resource := clusters.Find(clusterID, serviceType, resourceName)
		service.MaxScrapedAt = time.Time(maxScrapedAt).Unix()
		service.MinScrapedAt = time.Time(minScrapedAt).Unix()
		resource.Usage = usage
		return nil
	})
	if err != nil {
		return nil, err
	}

	//second query: collect domain quota data in these clusters
	whereStr, queryArgs = db.BuildSimpleWhereClause(makeClusterFilter("d", "ds", "dr", clusterID, filter))
	rows, err = dbi.Query(fmt.Sprintf(clusterReportQuery2, whereStr), queryArgs...)
	if err != nil {
		return nil, err
	}

	err = db.ForeachRow(rows, func() error {
		var (
			clusterID    string
			serviceType  string
			resourceName string
			quota        uint64
		)
		err := rows.Scan(&clusterID, &serviceType, &resourceName, &quota)
		if err != nil {
			return err
		}

		_, _, resource := clusters.Find(clusterID, serviceType, resourceName)
		resource.DomainsQuota = quota
		return nil
	})
	if err != nil {
		return nil, err
	}

	//third query: collect capacity data for these clusters
	whereStr, queryArgs = db.BuildSimpleWhereClause(makeClusterFilter("cs", "cs", "cr", clusterID, filter))
	rows, err = dbi.Query(fmt.Sprintf(clusterReportQuery3, whereStr), queryArgs...)
	if err != nil {
		return nil, err
	}

	err = db.ForeachRow(rows, func() error {
		var (
			clusterID    string
			serviceType  string
			resourceName string
			capacity     uint64
			scrapedAt    util.Time
		)
		err := rows.Scan(&clusterID, &serviceType, &resourceName, &capacity, &scrapedAt)
		if err != nil {
			return err
		}

		cluster, _, resource := clusters.Find(clusterID, serviceType, resourceName)
		resource.Capacity = &capacity

		scrapedAtUnix := time.Time(scrapedAt).Unix()
		if cluster.MaxScrapedAt == nil || *cluster.MaxScrapedAt < scrapedAtUnix {
			cluster.MaxScrapedAt = &scrapedAtUnix
		}
		if cluster.MinScrapedAt == nil || *cluster.MinScrapedAt > scrapedAtUnix {
			cluster.MinScrapedAt = &scrapedAtUnix
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	//validate against known services/resources
	isSharedService := make(map[string]bool)
	for clusterID, cluster := range clusters {
		clusterConfig, exists := config.Clusters[clusterID]
		if !exists {
			delete(clusters, clusterID)
			continue
		}

		isValidService := make(map[string]bool)
		for _, serviceConfig := range clusterConfig.Services {
			isValidService[serviceConfig.Type] = true
			if serviceConfig.Shared {
				isSharedService[serviceConfig.Type] = true
			}
		}

		for serviceType, service := range cluster.Services {
			if !isValidService[serviceType] {
				delete(cluster.Services, serviceType)
				continue
			}

			isValidResource := make(map[string]bool)
			if plugin := limes.GetQuotaPlugin(serviceType); plugin != nil {
				for _, res := range plugin.Resources() {
					isValidResource[res.Name] = true
				}
			}

			for resourceName := range service.Resources {
				if !isValidResource[resourceName] {
					delete(service.Resources, resourceName)
				}
			}
		}
	}

	if len(isSharedService) > 0 {

		//fourth query: aggregate domain quota for shared services
		sharedServiceTypes := make([]string, 0, len(isSharedService))
		for serviceType := range isSharedService {
			sharedServiceTypes = append(sharedServiceTypes, serviceType)
		}
		whereStr, queryArgs = db.BuildSimpleWhereClause(map[string]interface{}{"ds.type": sharedServiceTypes})
		rows, err = dbi.Query(fmt.Sprintf(clusterReportQuery4, whereStr), queryArgs...)
		if err != nil {
			return nil, err
		}

		sharedQuotaSums := make(map[string]map[string]uint64)
		err = db.ForeachRow(rows, func() error {
			var (
				serviceType  string
				resourceName string
				quota        uint64
			)
			err := rows.Scan(&serviceType, &resourceName, &quota)
			if err != nil {
				return err
			}

			if sharedQuotaSums[serviceType] == nil {
				sharedQuotaSums[serviceType] = make(map[string]uint64)
			}
			sharedQuotaSums[serviceType][resourceName] = quota
			return nil
		})
		if err != nil {
			return nil, err
		}

		//fifth query: aggregate project quota for shared services
		whereStr, queryArgs = db.BuildSimpleWhereClause(map[string]interface{}{"ps.type": sharedServiceTypes})
		rows, err = dbi.Query(fmt.Sprintf(clusterReportQuery5, whereStr), queryArgs...)
		if err != nil {
			return nil, err
		}

		sharedUsageSums := make(map[string]map[string]uint64)
		err = db.ForeachRow(rows, func() error {
			var (
				serviceType  string
				resourceName string
				usage        uint64
			)
			err := rows.Scan(&serviceType, &resourceName, &usage)
			if err != nil {
				return err
			}

			if sharedUsageSums[serviceType] == nil {
				sharedUsageSums[serviceType] = make(map[string]uint64)
			}
			sharedUsageSums[serviceType][resourceName] = usage
			return nil
		})
		if err != nil {
			return nil, err
		}

		for _, cluster := range clusters {
			isSharedService := make(map[string]bool)
			for _, serviceConfig := range config.Clusters[cluster.ID].Services {
				//NOTE: cluster config is guaranteed to exist due to earlier validation
				if serviceConfig.Shared {
					isSharedService[serviceConfig.Type] = true
				}
			}

			for _, service := range cluster.Services {
				if isSharedService[service.Type] && sharedQuotaSums[service.Type] != nil {
					for _, resource := range service.Resources {
						quota, exists := sharedQuotaSums[service.Type][resource.Name]
						if exists {
							resource.DomainsQuota = quota
						}
						usage, exists := sharedUsageSums[service.Type][resource.Name]
						if exists {
							resource.Usage = usage
						}
					}
				}
			}
		}

	}

	//flatten result (with stable order to keep the tests happy)
	ids := make([]string, 0, len(clusters))
	for id := range clusters {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]*Cluster, len(clusters))
	for idx, id := range ids {
		result[idx] = clusters[id]
	}

	return result, nil
}

func makeClusterFilter(tableWithClusterID, tableWithServiceType, tableWithResourceName string, clusterID *string, filter Filter) map[string]interface{} {
	fields := make(map[string]interface{})
	filter.ApplyTo(fields, tableWithServiceType, tableWithResourceName)
	if clusterID != nil {
		fields[tableWithClusterID+".cluster_id"] = *clusterID
	}
	return fields
}

type clusters map[string]*Cluster

func (c clusters) Find(clusterID, serviceType, resourceName string) (*Cluster, *ClusterService, *ClusterResource) {
	cluster, exists := c[clusterID]
	if !exists {
		cluster = &Cluster{
			ID:       clusterID,
			Services: make(ClusterServices),
		}
		c[clusterID] = cluster
	}

	service, exists := cluster.Services[serviceType]
	if !exists {
		service = &ClusterService{
			Type:      serviceType,
			Resources: make(ClusterResources),
		}
		cluster.Services[serviceType] = service
	}

	resource, exists := service.Resources[resourceName]
	if !exists {
		resource = &ClusterResource{
			Name: resourceName,
			Unit: limes.UnitFor(serviceType, resourceName),
		}
		service.Resources[resourceName] = resource
	}

	return cluster, service, resource
}
