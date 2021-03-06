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
	"errors"

	"github.com/gophercloud/gophercloud"
	"github.com/sapcc/limes/pkg/core"
)

type capacityManualPlugin struct {
	cfg core.CapacitorConfiguration
}

func init() {
	core.RegisterCapacityPlugin(func(c core.CapacitorConfiguration, scrapeSubcapacities map[string]map[string]bool) core.CapacityPlugin {
		return &capacityManualPlugin{c}
	})
}

//ID implements the core.CapacityPlugin interface.
func (p *capacityManualPlugin) ID() string {
	return "manual"
}

var errNoManualData = errors.New(`missing values for capacitor plugin "manual"`)

//Scrape implements the core.CapacityPlugin interface.
func (p *capacityManualPlugin) Scrape(provider *gophercloud.ProviderClient, eo gophercloud.EndpointOpts, clusterID string) (map[string]map[string]core.CapacityData, error) {
	if p.cfg.Manual == nil {
		return nil, errNoManualData
	}

	result := make(map[string]map[string]core.CapacityData)
	for serviceType, serviceData := range p.cfg.Manual {
		serviceResult := make(map[string]core.CapacityData)
		for resourceName, capacity := range serviceData {
			serviceResult[resourceName] = core.CapacityData{Capacity: capacity}
		}
		result[serviceType] = serviceResult
	}

	return result, nil
}
