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

package plugins

import (
	"sort"
	"strings"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/limits"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/quotasets"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/pagination"
	"github.com/sapcc/limes/pkg/limes"
)

type novaPlugin struct {
	cfg             limes.ServiceConfiguration
	scrapeInstances bool
	resources       []limes.ResourceInfo
	flavors         map[string]*flavors.Flavor
}

var novaDefaultResources = []limes.ResourceInfo{
	{
		Name: "cores",
		Unit: limes.UnitNone,
	},
	{
		Name: "instances",
		Unit: limes.UnitNone,
	},
	{
		Name: "ram",
		Unit: limes.UnitMebibytes,
	},
}

func init() {
	limes.RegisterQuotaPlugin(func(c limes.ServiceConfiguration, scrapeSubresources map[string]bool) limes.QuotaPlugin {
		return &novaPlugin{
			cfg:             c,
			scrapeInstances: scrapeSubresources["instances"],
			resources:       novaDefaultResources,
		}
	})
}

//Init implements the limes.QuotaPlugin interface.
func (p *novaPlugin) Init(provider *gophercloud.ProviderClient) error {
	client, err := p.Client(provider)
	if err != nil {
		return err
	}

	//look at quota class "default" to determine which quotas exist
	url := client.ServiceURL("os-quota-class-sets", "default")
	var result gophercloud.Result
	_, err = client.Get(url, &result.Body, nil)
	if err != nil {
		return err
	}

	//At SAP Converged Cloud, we use per-flavor instance quotas for baremetal
	//(Ironic) flavors, to control precisely how many baremetal machines can be
	//used by each domain/project. Each such quota has the resource name
	//"instances_${FLAVOR_NAME}".
	var body struct {
		QuotaClassSet map[string]int64 `yaml:"quota_class_set"`
	}
	err = result.ExtractInto(&body)
	if err != nil {
		return err
	}

	for key := range body.QuotaClassSet {
		if strings.HasPrefix(key, "instances_") {
			p.resources = append(p.resources, limes.ResourceInfo{
				Name:     key,
				Category: "per_flavor",
				Unit:     limes.UnitNone,
			})
		}
	}

	sort.Slice(p.resources, func(i, j int) bool {
		return p.resources[i].Name < p.resources[j].Name
	})
	return nil
}

//ServiceInfo implements the limes.QuotaPlugin interface.
func (p *novaPlugin) ServiceInfo() limes.ServiceInfo {
	return limes.ServiceInfo{
		Type:        "compute",
		ProductName: "nova",
		Area:        "compute",
	}
}

//Resources implements the limes.QuotaPlugin interface.
func (p *novaPlugin) Resources() []limes.ResourceInfo {
	return p.resources
}

func (p *novaPlugin) Client(provider *gophercloud.ProviderClient) (*gophercloud.ServiceClient, error) {
	return openstack.NewComputeV2(provider,
		gophercloud.EndpointOpts{Availability: gophercloud.AvailabilityPublic},
	)
}

//Scrape implements the limes.QuotaPlugin interface.
func (p *novaPlugin) Scrape(provider *gophercloud.ProviderClient, domainUUID, projectUUID string) (map[string]limes.ResourceData, error) {
	client, err := p.Client(provider)
	if err != nil {
		return nil, err
	}

	var limitsData struct {
		Limits struct {
			Absolute struct {
				MaxTotalCores      int64  `json:"maxTotalCores"`
				MaxTotalInstances  int64  `json:"maxTotalInstances"`
				MaxTotalRAMSize    int64  `json:"maxTotalRAMSize"`
				TotalCoresUsed     uint64 `json:"totalCoresUsed"`
				TotalInstancesUsed uint64 `json:"totalInstancesUsed"`
				TotalRAMUsed       uint64 `json:"totalRAMUsed"`
			} `json:"absolute"`
			AbsolutePerFlavor map[string]struct {
				MaxTotalInstances  int64  `json:"maxTotalInstances"`
				TotalInstancesUsed uint64 `json:"totalInstancesUsed"`
			} `json:"absolutePerFlavor"`
		} `json:"limits"`
	}
	err = limits.Get(client, limits.GetOpts{TenantID: projectUUID}).ExtractInto(&limitsData)
	if err != nil {
		return nil, err
	}

	result := map[string]limes.ResourceData{
		"cores": {
			Quota: limitsData.Limits.Absolute.MaxTotalCores,
			Usage: limitsData.Limits.Absolute.TotalCoresUsed,
		},
		"instances": {
			Quota: limitsData.Limits.Absolute.MaxTotalInstances,
			Usage: limitsData.Limits.Absolute.TotalInstancesUsed,
		},
		"ram": {
			Quota: limitsData.Limits.Absolute.MaxTotalRAMSize,
			Usage: limitsData.Limits.Absolute.TotalRAMUsed,
		},
	}
	for flavorName, flavorLimits := range limitsData.Limits.AbsolutePerFlavor {
		result["instances_"+flavorName] = limes.ResourceData{
			Quota: flavorLimits.MaxTotalInstances,
			Usage: flavorLimits.TotalInstancesUsed,
		}
	}

	if p.scrapeInstances {
		listOpts := novaServerListOpts{
			AllTenants: true,
			TenantID:   projectUUID,
		}

		err := servers.List(client, listOpts).EachPage(func(page pagination.Page) (bool, error) {
			instances, err := servers.ExtractServers(page)
			if err != nil {
				return false, err
			}

			for _, instance := range instances {
				subResource := map[string]interface{}{
					"id":     instance.ID,
					"name":   instance.Name,
					"status": instance.Status,
				}
				flavor, err := p.getFlavor(client, instance.Flavor["id"].(string))
				if err == nil {
					subResource["flavor"] = flavor.Name
					subResource["vcpu"] = flavor.VCPUs
					subResource["ram"] = limes.ValueWithUnit{
						Value: uint64(flavor.RAM),
						Unit:  limes.UnitMebibytes,
					}
					subResource["disk"] = limes.ValueWithUnit{
						Value: uint64(flavor.Disk),
						Unit:  limes.UnitGibibytes,
					}
				}

				resource, exists := result["instances_"+flavor.Name]
				if !exists {
					resource = result["instances"]
				}
				resource.Subresources = append(resource.Subresources, subResource)
			}
			return true, nil
		})
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

//SetQuota implements the limes.QuotaPlugin interface.
func (p *novaPlugin) SetQuota(provider *gophercloud.ProviderClient, domainUUID, projectUUID string, quotas map[string]uint64) error {
	client, err := p.Client(provider)
	if err != nil {
		return err
	}

	return quotasets.Update(client, projectUUID, novaQuotaUpdateOpts(quotas)).Err
}

//Getting and caching flavor details
//Changing a flavor is not supported from OpenStack, so no invalidating of the cache needed
//Acces to the map is not thread safe
func (p *novaPlugin) getFlavor(client *gophercloud.ServiceClient, flavorID string) (*flavors.Flavor, error) {
	if p.flavors == nil {
		p.flavors = make(map[string]*flavors.Flavor)
	}

	if flavor, ok := p.flavors[flavorID]; ok {
		return flavor, nil
	}

	flavor, err := flavors.Get(client, flavorID).Extract()
	if err == nil {
		p.flavors[flavorID] = flavor
	}
	return flavor, err
}

func makeIntPointer(value int) *int {
	return &value
}

type novaServerListOpts struct {
	AllTenants bool   `q:"all_tenants"`
	TenantID   string `q:"tenant_id"`
}

func (opts novaServerListOpts) ToServerListQuery() (string, error) {
	q, err := gophercloud.BuildQueryString(opts)
	return q.String(), err
}

type novaQuotaUpdateOpts map[string]uint64

func (opts novaQuotaUpdateOpts) ToComputeQuotaUpdateMap() (map[string]interface{}, error) {
	result := make(map[string]interface{}, len(opts))
	for key, val := range opts {
		result[key] = val
	}
	return map[string]interface{}{"quota_set": result}, nil
}
