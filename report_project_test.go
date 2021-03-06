package limes

import (
	"testing"

	th "github.com/gophercloud/gophercloud/testhelper"
)

var projectServicesMockJSON = `
	[
		{
			"type": "shared",
			"area": "shared",
			"resources": [
				{
					"name": "capacity",
					"unit": "B",
					"quota": 10,
					"usable_quota": 11,
					"usage": 2
				},
				{
					"name": "things",
					"quota": 10,
					"usable_quota": 10,
					"usage": 2
				}
			],
			"scraped_at": 22
		}
	]
`

var projectResourcesMockJSON = `
 [
		{
			"name": "capacity",
			"unit": "B",
			"quota": 10,
			"usable_quota": 11,
			"usage": 2
		},
		{
			"name": "things",
			"quota": 10,
			"usable_quota": 10,
			"usage": 2
		}
	]
`

var projectMockResources = &ProjectResourceReports{
	"capacity": &ProjectResourceReport{
		ResourceInfo: ResourceInfo{
			Name: "capacity",
			Unit: UnitBytes,
		},
		Quota:       10,
		UsableQuota: 11,
		Usage:       2,
	},
	"things": &ProjectResourceReport{
		ResourceInfo: ResourceInfo{
			Name: "things",
		},
		Quota:       10,
		UsableQuota: 10,
		Usage:       2,
	},
}

var projectMockServices = &ProjectServiceReports{
	"shared": &ProjectServiceReport{
		ServiceInfo: ServiceInfo{
			Type: "shared",
			Area: "shared",
		},
		Resources: *projectMockResources,
		ScrapedAt: p2i64(22),
	},
}

func TestProjectServicesMarshall(t *testing.T) {
	th.CheckJSONEquals(t, projectServicesMockJSON, projectMockServices)
}

func TestProjectServicesUnmarshall(t *testing.T) {
	actual := &ProjectServiceReports{}
	err := actual.UnmarshalJSON([]byte(projectServicesMockJSON))
	th.AssertNoErr(t, err)
	th.CheckDeepEquals(t, projectMockServices, actual)
}

func TestProjectResourcesMarshall(t *testing.T) {
	th.CheckJSONEquals(t, projectResourcesMockJSON, projectMockResources)
}

func TestProjectResourcesUnmarshall(t *testing.T) {
	actual := &ProjectResourceReports{}
	err := actual.UnmarshalJSON([]byte(projectResourcesMockJSON))
	th.AssertNoErr(t, err)
	th.CheckDeepEquals(t, projectMockResources, actual)
}
