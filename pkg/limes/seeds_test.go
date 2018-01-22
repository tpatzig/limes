/*******************************************************************************
*
* Copyright 2018 SAP SE
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

package limes

import (
	"reflect"
	"testing"

	"github.com/gophercloud/gophercloud"
)

func TestQuotaSeedParsingSuccess(t *testing.T) {
	seeds, errs := NewQuotaSeeds(clusterForQuotaSeedTest(), "fixtures/quota-seed-valid.yaml")

	if len(errs) > 0 {
		t.Errorf("expected no parsing errors, got %d errors:\n", len(errs))
		for idx, err := range errs {
			t.Logf("[%d] %s\n", idx+1, err.Error())
		}
	}

	expected := QuotaSeeds{
		Domains: map[string]QuotaSeedValues{
			"germany": {
				"service-one": {
					"things":       20,
					"capacity_MiB": 10240,
				},
				"service-two": {
					"capacity_MiB": 1,
				},
			},
			"poland": {
				"service-two": {
					"things": 5,
				},
			},
		},
		Projects: map[string]map[string]QuotaSeedValues{
			"germany": {
				"berlin": {
					"service-one": {
						"things":       10,
						"capacity_MiB": 5120,
					},
				},
				"dresden": {
					"service-one": {
						"things": 5,
					},
					"service-two": {
						"capacity_MiB": 1,
					},
				},
			},
			"poland": {
				"warsaw": {
					"service-two": {
						"things": 5,
					},
				},
			},
		},
	}
	if !reflect.DeepEqual(seeds, &expected) {
		t.Errorf("actual = %#v\n", seeds)
		t.Errorf("expected = %#v\n", expected)
	}
}

func clusterForQuotaSeedTest() *Cluster {
	return &Cluster{
		QuotaPlugins: map[string]QuotaPlugin{
			"service-one": quotaSeedTestPlugin{"service-one"},
			"service-two": quotaSeedTestPlugin{"service-two"},
		},
	}
}

func TestQuotaSeedParsingFailure(t *testing.T) {
	_, errs := NewQuotaSeeds(clusterForQuotaSeedTest(), "fixtures/quota-seed-invalid.yaml")

	expectedErrors := []string{
		"missing domain name for project atlantis",
		`invalid seed values for domain germany: value "10 GiB or something" for service-one/capacity_MiB does not match expected format "<number> <unit>"`,
		"invalid seed values for domain germany: cannot convert value from ounce to MiB because units are incompatible",
		"invalid seed values for project germany/berlin: no such service: unknown",
		`invalid seed values for project germany/dresden: invalid value "NaN" for service-one/things: strconv.ParseUint: parsing "NaN": invalid syntax`,
	}
	expectedErrs := make(map[string]bool)
	for _, err := range expectedErrors {
		expectedErrs[err] = true
	}

	for _, err := range errs {
		err := err.Error()
		if expectedErrs[err] {
			delete(expectedErrs, err) //check that one off the list
		} else {
			t.Errorf("got unexpected error: %s", err)
		}
	}
	for err := range expectedErrs {
		t.Errorf("did not get expected error: %s", err)
	}
}

type quotaSeedTestPlugin struct {
	ServiceType string
}

func (p quotaSeedTestPlugin) Init(client *gophercloud.ProviderClient) error {
	return nil
}
func (p quotaSeedTestPlugin) ServiceInfo() ServiceInfo {
	return ServiceInfo{Type: p.ServiceType}
}
func (p quotaSeedTestPlugin) Scrape(client *gophercloud.ProviderClient, domainUUID, projectUUID string) (map[string]ResourceData, error) {
	return nil, nil
}
func (p quotaSeedTestPlugin) SetQuota(client *gophercloud.ProviderClient, domainUUID, projectUUID string, quotas map[string]uint64) error {
	return nil
}

func (p quotaSeedTestPlugin) Resources() []ResourceInfo {
	return []ResourceInfo{
		{Name: "things", Unit: UnitNone},
		{Name: "capacity_MiB", Unit: UnitMebibytes},
	}
}
