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
	"fmt"
	"strings"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/sapcc/go-bits/logg"
	"github.com/sapcc/limes"
	"github.com/sapcc/limes/pkg/core"
)

type neutronPlugin struct {
	cfg                     core.ServiceConfiguration
	hasQuotaDetailsEndpoint bool
	resources               []limes.ResourceInfo
	resourcesMeta           []neutronResourceMetadata
}

var neutronResources = []limes.ResourceInfo{
	////////// SDN resources
	{
		Name:     "floating_ips",
		Unit:     limes.UnitNone,
		Category: "networking",
	},
	{
		Name:     "networks",
		Unit:     limes.UnitNone,
		Category: "networking",
	},
	{
		Name:     "ports",
		Unit:     limes.UnitNone,
		Category: "networking",
	},
	{
		Name:     "rbac_policies",
		Unit:     limes.UnitNone,
		Category: "networking",
	},
	{
		Name:     "routers",
		Unit:     limes.UnitNone,
		Category: "networking",
	},
	{
		Name:     "security_group_rules",
		Unit:     limes.UnitNone,
		Category: "networking",
		//for "default" security group
		AutoApproveInitialQuota: 4,
	},
	{
		Name:     "security_groups",
		Unit:     limes.UnitNone,
		Category: "networking",
		//for "default" security group
		AutoApproveInitialQuota: 1,
	},
	{
		Name:     "subnet_pools",
		Unit:     limes.UnitNone,
		Category: "networking",
	},
	{
		Name:     "subnets",
		Unit:     limes.UnitNone,
		Category: "networking",
	},
	////////// LBaaS resources
	{
		Name:     "healthmonitors",
		Unit:     limes.UnitNone,
		Category: "loadbalancing",
	},
	{
		Name:     "l7policies",
		Unit:     limes.UnitNone,
		Category: "loadbalancing",
	},
	{
		Name:     "listeners",
		Unit:     limes.UnitNone,
		Category: "loadbalancing",
	},
	{
		Name:     "loadbalancers",
		Unit:     limes.UnitNone,
		Category: "loadbalancing",
	},
	{
		Name:     "pools",
		Unit:     limes.UnitNone,
		Category: "loadbalancing",
	},
}

func init() {
	core.RegisterQuotaPlugin(func(c core.ServiceConfiguration, scrapeSubresources map[string]bool) core.QuotaPlugin {
		return &neutronPlugin{
			cfg:                     c,
			hasQuotaDetailsEndpoint: false, //until proven otherwise in Init()
			resources:               neutronResources,
			resourcesMeta:           neutronResourceMeta,
		}
	})
}

//Init implements the core.QuotaPlugin interface.
func (p *neutronPlugin) Init(provider *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) error {
	client, err := openstack.NewNetworkV2(provider, eo)
	if err != nil {
		return err
	}

	//probe support for the quotas/:project_id/details endpoint (available in
	//Pike and above)
	var result gophercloud.Result
	url := client.ServiceURL("quotas", "unknown", "details") //FIXME should use a valid project ID, such as AuthResult().GetProject().ID
	_, err = client.Get(url, &result.Body, nil)
	if err != nil {
		if _, ok := err.(gophercloud.ErrDefault404); ok {
			logg.Debug("Neutron does NOT have the /quotas/:project_id/details endpoint - falling back to counting usage manually")
			p.hasQuotaDetailsEndpoint = false
			return nil
		}
		return err
	}
	logg.Debug("Neutron HAS the /quotas/:project_id/details endpoint - usage can be scraped efficiently")
	p.hasQuotaDetailsEndpoint = true

	//probe support for specific resources
	var data struct {
		Quotas map[string]interface{} `json:"quota"`
	}
	data.Quotas = make(map[string]interface{})
	err = result.ExtractInto(&data)
	if err != nil {
		return err
	}

	if _, exists := data.Quotas["member"]; exists {
		logg.Debug("Neutron HAS the pool members resource")
		p.resources = append(p.resources, limes.ResourceInfo{
			Name:     "pool_members",
			Unit:     limes.UnitNone,
			Category: "loadbalancing",
		})
		p.resourcesMeta = append(p.resourcesMeta, neutronResourceMetadata{
			LimesName:   "pool_members",
			NeutronName: "member",
		})
	} else {
		logg.Debug("Neutron does NOT have the pool members resource")
	}
	return nil
}

//ServiceInfo implements the core.QuotaPlugin interface.
func (p *neutronPlugin) ServiceInfo() limes.ServiceInfo {
	return limes.ServiceInfo{
		Type:        "network",
		ProductName: "neutron",
		Area:        "network",
	}
}

//Resources implements the core.QuotaPlugin interface.
func (p *neutronPlugin) Resources() []limes.ResourceInfo {
	return p.resources
}

type neutronResourceMetadata struct {
	LimesName       string
	NeutronName     string
	EndpointPath    []string
	JSONToplevelKey string
}

var neutronResourceMeta = []neutronResourceMetadata{
	{
		LimesName:       "networks",
		NeutronName:     "network",
		EndpointPath:    []string{"networks"},
		JSONToplevelKey: "networks",
	},
	{
		LimesName:       "subnets",
		NeutronName:     "subnet",
		EndpointPath:    []string{"subnets"},
		JSONToplevelKey: "subnets",
	},
	{
		LimesName:       "subnet_pools",
		NeutronName:     "subnetpool",
		EndpointPath:    []string{"subnetpools"},
		JSONToplevelKey: "subnetpools",
	},
	{
		LimesName:       "floating_ips",
		NeutronName:     "floatingip",
		EndpointPath:    []string{"floatingips"},
		JSONToplevelKey: "floatingips",
	},
	{
		LimesName:       "routers",
		NeutronName:     "router",
		EndpointPath:    []string{"routers"},
		JSONToplevelKey: "routers",
	},
	{
		LimesName:       "ports",
		NeutronName:     "port",
		EndpointPath:    []string{"ports"},
		JSONToplevelKey: "ports",
	},
	{
		LimesName:       "security_groups",
		NeutronName:     "security_group",
		EndpointPath:    []string{"security-groups"},
		JSONToplevelKey: "security_groups",
	},
	{
		LimesName:       "security_group_rules",
		NeutronName:     "security_group_rule",
		EndpointPath:    []string{"security-group-rules"},
		JSONToplevelKey: "security_group_rules",
	},
	{
		LimesName:       "rbac_policies",
		NeutronName:     "rbac_policy",
		EndpointPath:    []string{"rbac-policies"},
		JSONToplevelKey: "rbac_policies",
	},
	{
		LimesName:       "loadbalancers",
		NeutronName:     "loadbalancer",
		EndpointPath:    []string{"lbaas", "loadbalancers"},
		JSONToplevelKey: "loadbalancers",
	},
	{
		LimesName:       "listeners",
		NeutronName:     "listener",
		EndpointPath:    []string{"lbaas", "listeners"},
		JSONToplevelKey: "listeners",
	},
	{
		LimesName:       "pools",
		NeutronName:     "pool",
		EndpointPath:    []string{"lbaas", "pools"},
		JSONToplevelKey: "pools",
	},
	{
		LimesName:       "healthmonitors",
		NeutronName:     "healthmonitor",
		EndpointPath:    []string{"lbaas", "healthmonitors"},
		JSONToplevelKey: "healthmonitors",
	},
	{
		LimesName:       "l7policies",
		NeutronName:     "l7policy",
		EndpointPath:    []string{"lbaas", "l7policies"},
		JSONToplevelKey: "l7policies",
	},
}

type neutronQueryOpts struct {
	Fields      string `q:"fields"`
	ProjectUUID string `q:"tenant_id"`
}

//Scrape implements the core.QuotaPlugin interface.
func (p *neutronPlugin) Scrape(provider *gophercloud.ProviderClient, eo gophercloud.EndpointOpts, clusterID, domainUUID, projectUUID string) (map[string]core.ResourceData, error) {
	client, err := openstack.NewNetworkV2(provider, eo)
	if err != nil {
		return nil, err
	}

	if p.hasQuotaDetailsEndpoint {
		return p.scrapeNewStyle(client, projectUUID)
	}
	return p.scrapeOldStyle(client, projectUUID)
}

func (p *neutronPlugin) scrapeNewStyle(client *gophercloud.ServiceClient, projectUUID string) (map[string]core.ResourceData, error) {
	var result gophercloud.Result
	url := client.ServiceURL("quotas", projectUUID, "details")
	_, err := client.Get(url, &result.Body, nil)
	if err != nil {
		return nil, err
	}

	type neutronQuotaStruct struct {
		Quota int64  `json:"limit"`
		Usage uint64 `json:"used"`
	}
	var quotas struct {
		Values map[string]neutronQuotaStruct `json:"quota"`
	}
	quotas.Values = make(map[string]neutronQuotaStruct)
	err = result.ExtractInto(&quotas)
	if err != nil {
		return nil, err
	}

	//convert data returned by Neutron into Limes' internal format
	data := make(map[string]core.ResourceData)
	for _, res := range p.resourcesMeta {
		values := quotas.Values[res.NeutronName]
		data[res.LimesName] = core.ResourceData{
			Quota: values.Quota,
			Usage: values.Usage,
		}
	}
	return data, nil
}

func (p *neutronPlugin) scrapeOldStyle(client *gophercloud.ServiceClient, projectUUID string) (map[string]core.ResourceData, error) {
	data := make(map[string]core.ResourceData)

	//query quotas
	var result gophercloud.Result
	url := client.ServiceURL("quotas", projectUUID)
	_, err := client.Get(url, &result.Body, nil)
	if err != nil {
		return nil, err
	}

	var quotas struct {
		Values map[string]int64 `json:"quota"`
	}
	quotas.Values = make(map[string]int64)
	err = result.ExtractInto(&quotas)
	if err != nil {
		return nil, err
	}

	//calculate usage by counting resources by hand
	query, err := gophercloud.BuildQueryString(neutronQueryOpts{Fields: "id", ProjectUUID: projectUUID})
	if err != nil {
		return nil, err
	}
	for _, res := range p.resourcesMeta {
		url := client.ServiceURL(res.EndpointPath...) + query.String()
		count, err := countNeutronThings(client, url)
		if err != nil {
			return nil, err
		}

		data[res.LimesName] = core.ResourceData{
			Quota: quotas.Values[res.NeutronName],
			Usage: uint64(count),
		}
	}

	return data, nil
}

//SetQuota implements the core.QuotaPlugin interface.
func (p *neutronPlugin) SetQuota(provider *gophercloud.ProviderClient, eo gophercloud.EndpointOpts, clusterID, domainUUID, projectUUID string, quotas map[string]uint64) error {
	//map resource names from Limes to Neutron
	var requestData struct {
		Quotas map[string]uint64 `json:"quota"`
	}
	requestData.Quotas = make(map[string]uint64)
	for _, res := range p.resourcesMeta {
		quota, exists := quotas[res.LimesName]
		if exists {
			requestData.Quotas[res.NeutronName] = quota
		}
	}

	client, err := openstack.NewNetworkV2(provider, eo)
	if err != nil {
		return err
	}

	url := client.ServiceURL("quotas", projectUUID)
	_, err = client.Put(url, requestData, nil, &gophercloud.RequestOpts{OkCodes: []int{200}})
	return err
}

//I know that gophercloud has a pagination implementation, but it would lead to
//a ton of code duplication because Gophercloud insists on using different
//types for each resource.
func countNeutronThings(client *gophercloud.ServiceClient, firstPageURL string) (int, error) {
	url := firstPageURL
	count := 0

	type entry struct {
		//if this entry is in the list of things, then this field is set
		ID string `json:"id"`
		//if this entry is in the list of links, then these fields are set
		URL string `json:"href"`
		Rel string `json:"rel"`
	}

	for {
		jsonBody := make(map[string][]entry)
		_, err := client.Get(url, &jsonBody, nil)
		if err != nil {
			return 0, err
		}
		keySetError := func() (int, error) {
			allKeys := make([]string, 0, len(jsonBody))
			for key := range jsonBody {
				allKeys = append(allKeys, key)
			}
			return 0, fmt.Errorf("GET %s returned JSON with unexpected set of keys: %s", url, strings.Join(allKeys, ", "))
		}

		//we should have two keys, one for the list of things (e.g. "ports") and
		//one for the list of links (e.g. "ports_links")
		if len(jsonBody) > 2 {
			return keySetError()
		}

		var (
			links     []entry
			hasLinks  bool
			things    []entry
			hasThings bool
		)
		for key, entries := range jsonBody {
			if strings.HasSuffix(key, "_links") {
				if hasLinks {
					return keySetError()
				}
				links = entries
				hasLinks = true
			} else {
				if hasThings {
					return keySetError()
				}
				things = entries
				hasThings = true
			}
		}

		if !hasThings {
			return keySetError()
		}

		//page is valid - count the things and find the next page (if any)
		count += len(things)
		url = ""
		for _, link := range links {
			if link.Rel == "next" {
				url = link.URL
			}
		}
		if url == "" {
			return count, nil
		}
	}
}
