package limes

import (
	"testing"

	th "github.com/gophercloud/gophercloud/testhelper"
)

var domainServicesMockJSON = `
	[
		{
			"type": "compute",
			"area": "compute",
			"resources": [
				{
					"name": "cores",
					"quota": 50,
					"projects_quota": 20,
					"usage": 10
				},
				{
					"name": "ram",
					"unit": "MiB",
					"quota": 20480,
					"projects_quota": 10240,
					"usage": 4080
				}
			],
			"max_scraped_at": 1538955269,
			"min_scraped_at": 1538955116
		}
	]
`

var domainResourcesMockJSON = `
	[
		{
			"name": "cores",
			"quota": 50,
			"projects_quota": 20,
			"usage": 10
		},
		{
			"name": "ram",
			"unit": "MiB",
			"quota": 20480,
			"projects_quota": 10240,
			"usage": 4080
		}
	]
`

var domainMockResources = &DomainResourceReports{
	"cores": &DomainResourceReport{
		ResourceInfo: ResourceInfo{
			Name: "cores",
		},
		DomainQuota:   50,
		ProjectsQuota: 20,
		Usage:         10,
	},
	"ram": &DomainResourceReport{
		ResourceInfo: ResourceInfo{
			Name: "ram",
			Unit: UnitMebibytes,
		},
		DomainQuota:   20480,
		ProjectsQuota: 10240,
		Usage:         4080,
	},
}

var domainMockServices = &DomainServiceReports{
	"compute": &DomainServiceReport{
		ServiceInfo: ServiceInfo{
			Type: "compute",
			Area: "compute",
		},
		Resources:    *domainMockResources,
		MaxScrapedAt: p2i64(1538955269),
		MinScrapedAt: p2i64(1538955116),
	},
}

func TestDomainServicesMarshal(t *testing.T) {
	th.CheckJSONEquals(t, domainServicesMockJSON, domainMockServices)
}

func TestDomainServicesUnmarshal(t *testing.T) {
	actual := &DomainServiceReports{}
	err := actual.UnmarshalJSON([]byte(domainServicesMockJSON))
	th.AssertNoErr(t, err)
	th.CheckDeepEquals(t, domainMockServices, actual)
}

func TestDomainResourcesMarshal(t *testing.T) {
	th.CheckJSONEquals(t, domainResourcesMockJSON, domainMockResources)
}

func TestDomainResourcesUnmarshal(t *testing.T) {
	actual := &DomainResourceReports{}
	err := actual.UnmarshalJSON([]byte(domainResourcesMockJSON))
	th.AssertNoErr(t, err)
	th.CheckDeepEquals(t, domainMockResources, actual)
}
