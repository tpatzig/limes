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

package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"testing"

	policy "github.com/databus23/goslo.policy"
	"github.com/sapcc/go-bits/assert"
	"github.com/sapcc/limes/pkg/db"
	"github.com/sapcc/limes/pkg/limes"
	"github.com/sapcc/limes/pkg/test"
)

func init() {
	//This is required for limes.GetServiceTypesForArea() to work.
	limes.RegisterQuotaPlugin(test.NewPluginFactory("shared"))
	limes.RegisterQuotaPlugin(test.NewPluginFactory("unshared"))
}

func setupTest(t *testing.T, clusterName, startData string) (*limes.Cluster, http.Handler) {
	//load test database
	test.InitDatabase(t)
	test.ExecSQLFile(t, startData)

	//prepare test configuration
	serviceTypes := []string{"shared", "unshared"}
	isServiceShared := map[string]bool{"shared": true}
	quotaPlugins := map[string]limes.QuotaPlugin{
		"shared":   test.NewPlugin("shared"),
		"unshared": test.NewPlugin("unshared"),
	}
	westConstraintSet := limes.QuotaConstraintSet{
		Domains: map[string]limes.QuotaConstraints{
			"france": {
				"shared": {
					"capacity": {Minimum: p2u64(10), Maximum: p2u64(123)},
					"things":   {Minimum: p2u64(20)},
				},
				"unshared": {
					"capacity": {Maximum: p2u64(20)},
					"things":   {Minimum: p2u64(20), Maximum: p2u64(20)},
				},
			},
		},
		Projects: map[string]map[string]limes.QuotaConstraints{
			"germany": {
				"berlin": {
					//This constraint is used for the happy-path tests, where PUT
					//succeeds because the requested value fits within the constraint.
					"shared": {"capacity": {Minimum: p2u64(1), Maximum: p2u64(6)}},
				},
				"dresden": {
					//These constraints are used for the failure tests, where PUT fails
					//because the requested values conflict with the constraint.
					"shared": {
						"capacity": {Minimum: p2u64(10)},
					},
					"unshared": {
						"capacity": {Minimum: p2u64(10), Maximum: p2u64(10)},
						"things":   {Maximum: p2u64(10)},
					},
				},
			},
		},
	}

	quotaPlugins["shared"].(*test.Plugin).WithExternallyManagedResource = true

	config := limes.Configuration{
		Clusters: map[string]*limes.Cluster{
			"west": {
				ID:               "west",
				ServiceTypes:     serviceTypes,
				IsServiceShared:  isServiceShared,
				DiscoveryPlugin:  test.NewDiscoveryPlugin(),
				QuotaPlugins:     quotaPlugins,
				CapacityPlugins:  map[string]limes.CapacityPlugin{},
				Config:           &limes.ClusterConfiguration{Auth: &limes.AuthParameters{}},
				QuotaConstraints: &westConstraintSet,
			},
			"east": {
				ID:              "east",
				ServiceTypes:    serviceTypes,
				IsServiceShared: isServiceShared,
				QuotaPlugins:    quotaPlugins,
				CapacityPlugins: map[string]limes.CapacityPlugin{},
				Config:          &limes.ClusterConfiguration{Auth: &limes.AuthParameters{}},
			},
			"cloud": {
				ID:              "cloud",
				ServiceTypes:    serviceTypes,
				IsServiceShared: isServiceShared,
				DiscoveryPlugin: test.NewDiscoveryPlugin(),
				QuotaPlugins:    quotaPlugins,
				CapacityPlugins: map[string]limes.CapacityPlugin{},
				Config:          &limes.ClusterConfiguration{Auth: &limes.AuthParameters{}},
			},
		},
	}

	//load test policy (where everything is allowed)
	policyBytes, err := ioutil.ReadFile("../test/policy.json")
	if err != nil {
		t.Fatal(err)
	}
	policyRules := make(map[string]string)
	err = json.Unmarshal(policyBytes, &policyRules)
	if err != nil {
		t.Fatal(err)
	}
	config.API.PolicyEnforcer, err = policy.NewEnforcer(policyRules)
	if err != nil {
		t.Fatal(err)
	}

	cluster := config.Clusters[clusterName]
	router, _ := NewV1Router(cluster, config)
	return cluster, router
}

func Test_InconsistencyOperations(t *testing.T) {
	clusterName, pathtoData := "cloud", "fixtures/start-data-inconsistencies.sql"
	_, router := setupTest(t, clusterName, pathtoData)

	//check ListInconsistencies
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/inconsistencies",
		ExpectStatus: 200,
		ExpectBody:   assert.JSONFixtureFile("./fixtures/inconsistency-list.json"),
	}.Check(t, router)
}

func Test_EmptyInconsistencyReport(t *testing.T) {
	_, router := setupTest(t, "cloud", "/dev/null")

	//check ListInconsistencies
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/inconsistencies",
		ExpectStatus: 200,
		ExpectBody:   assert.JSONFixtureFile("./fixtures/inconsistency-empty.json"),
	}.Check(t, router)
}

func Test_ClusterOperations(t *testing.T) {
	clusterName, pathtoData := "west", "fixtures/start-data.sql"
	_, router := setupTest(t, clusterName, pathtoData)

	//check GetCluster
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/clusters/west",
		ExpectStatus: 200,
		ExpectBody:   assert.JSONFixtureFile("fixtures/cluster-get-west.json"),
	}.Check(t, router)
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/clusters/current",
		ExpectStatus: 200,
		ExpectBody:   assert.JSONFixtureFile("fixtures/cluster-get-west.json"),
	}.Check(t, router)

	//check ListClusters
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/clusters",
		ExpectStatus: 200,
		ExpectBody:   assert.JSONFixtureFile("fixtures/cluster-list.json"),
	}.Check(t, router)
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/clusters?detail",
		ExpectStatus: 200,
		ExpectBody:   assert.JSONFixtureFile("fixtures/cluster-list-detail.json"),
	}.Check(t, router)
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/clusters?local",
		ExpectStatus: 200,
		ExpectBody:   assert.JSONFixtureFile("fixtures/cluster-list-local.json"),
	}.Check(t, router)
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/clusters?service=unknown",
		ExpectStatus: 200,
		ExpectBody:   assert.JSONFixtureFile("./fixtures/cluster-list-no-services.json"),
	}.Check(t, router)
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/clusters?service=shared&resource=unknown",
		ExpectStatus: 200,
		ExpectBody:   assert.JSONFixtureFile("./fixtures/cluster-list-no-resources.json"),
	}.Check(t, router)
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/clusters?service=shared&resource=things",
		ExpectStatus: 200,
		ExpectBody:   assert.JSONFixtureFile("fixtures/cluster-list-filtered.json"),
	}.Check(t, router)

	//check PutCluster error cases
	assert.HTTPRequest{
		Method:       "PUT",
		Path:         "/v1/clusters/east",
		ExpectStatus: 422,
		ExpectBody:   assert.StringData("cannot set shared/things capacity: capacity for this resource is maintained automatically\n"),
		Body: assert.JSONObject{
			"cluster": assert.JSONObject{
				"services": []assert.JSONObject{
					{
						"type": "shared",
						"resources": []assert.JSONObject{
							{"name": "things", "capacity": 100, "comment": "whatever"},
						},
					},
				},
			},
		},
	}.Check(t, router)

	assert.HTTPRequest{
		Method:       "PUT",
		Path:         "/v1/clusters/east",
		ExpectStatus: 422,
		ExpectBody:   assert.StringData("cannot set shared/things capacity: capacity for this resource is maintained automatically\n"),
		Body: assert.JSONObject{
			"cluster": assert.JSONObject{
				"services": []assert.JSONObject{
					{
						"type": "shared",
						"resources": []assert.JSONObject{
							{"name": "things", "capacity": -1},
						},
					},
				},
			},
		},
	}.Check(t, router)

	assert.HTTPRequest{
		Method:       "PUT",
		Path:         "/v1/clusters/east",
		ExpectStatus: 422,
		ExpectBody:   assert.StringData("cannot set shared/capacity capacity: comment is missing\n"),
		Body: assert.JSONObject{
			"cluster": assert.JSONObject{
				"services": []assert.JSONObject{
					{
						"type": "shared",
						"resources": []assert.JSONObject{
							{"name": "capacity", "capacity": 100},
						},
					},
				},
			},
		},
	}.Check(t, router)

	assert.HTTPRequest{
		Method:       "PUT",
		Path:         "/v1/clusters/east",
		ExpectStatus: 422,
		ExpectBody:   assert.StringData("cannot set unknown/things capacity: no such service\n"),
		Body: assert.JSONObject{
			"cluster": assert.JSONObject{
				"services": []assert.JSONObject{
					{
						"type": "unknown",
						"resources": []assert.JSONObject{
							{"name": "things", "capacity": 100, "comment": "foo"},
						},
					},
				},
			},
		},
	}.Check(t, router)

	assert.HTTPRequest{
		Method:       "PUT",
		Path:         "/v1/clusters/east",
		ExpectStatus: 422,
		ExpectBody:   assert.StringData("cannot set shared/unknown capacity: no such resource\n"),
		Body: assert.JSONObject{
			"cluster": assert.JSONObject{
				"services": []assert.JSONObject{
					{
						"type": "shared",
						"resources": []assert.JSONObject{
							{"name": "unknown", "capacity": 100, "comment": "foo"},
						},
					},
				},
			},
		},
	}.Check(t, router)

	assert.HTTPRequest{
		Method:       "PUT",
		Path:         "/v1/clusters/east",
		ExpectStatus: 422,
		ExpectBody:   assert.StringData("cannot set shared/capacity capacity: cannot convert value from <count> to B because units are incompatible\n"),
		Body: assert.JSONObject{
			"cluster": assert.JSONObject{
				"services": []assert.JSONObject{
					{
						"type": "shared",
						"resources": []assert.JSONObject{
							{"name": "capacity", "capacity": 100, "unit": "", "comment": "foo"},
						},
					},
				},
			},
		},
	}.Check(t, router)

	//before checking PutCluster, delete all existing manually-maintained
	//capacity values to be able to check inserts as well as updates
	_, err := db.DB.Exec(`DELETE FROM cluster_resources WHERE comment != ''`)
	if err != nil {
		t.Error(err)
	}

	//check PutCluster insert
	assert.HTTPRequest{
		Method:       "PUT",
		Path:         "/v1/clusters/east",
		ExpectStatus: 200,
		Body: assert.JSONObject{
			"cluster": assert.JSONObject{
				"services": []assert.JSONObject{
					{
						"type": "shared",
						"resources": []assert.JSONObject{
							{"name": "capacity", "capacity": 100, "comment": "hundred"},
						},
					},
					{
						"type": "unshared",
						"resources": []assert.JSONObject{
							{"name": "capacity", "capacity": 200, "comment": "two-hundred"},
						},
					},
				},
			},
		},
	}.Check(t, router)
	expectClusterCapacity(t, "shared", "shared", "capacity", 100, "hundred")
	expectClusterCapacity(t, "east", "unshared", "capacity", 200, "two-hundred")

	//check PutCluster update (and unit conversion)
	assert.HTTPRequest{
		Method:       "PUT",
		Path:         "/v1/clusters/east",
		ExpectStatus: 200,
		Body: assert.JSONObject{
			"cluster": assert.JSONObject{
				"services": []assert.JSONObject{
					{
						"type": "shared",
						"resources": []assert.JSONObject{
							{"name": "capacity", "capacity": 101, "comment": "updated"},
						},
					},
					{
						"type": "unshared",
						"resources": []assert.JSONObject{
							{"name": "capacity", "capacity": 201, "unit": "MiB", "comment": "updated!"},
						},
					},
				},
			},
		},
	}.Check(t, router)
	expectClusterCapacity(t, "shared", "shared", "capacity", 101, "updated")
	expectClusterCapacity(t, "east", "unshared", "capacity", 201<<20, "updated!")

	//check PutCluster delete
	assert.HTTPRequest{
		Method:       "PUT",
		Path:         "/v1/clusters/east",
		ExpectStatus: 200,
		Body: assert.JSONObject{
			"cluster": assert.JSONObject{
				"services": []assert.JSONObject{
					{
						"type": "shared",
						"resources": []assert.JSONObject{
							{"name": "capacity", "capacity": -1},
						},
					},
					{
						"type": "unshared",
						"resources": []assert.JSONObject{
							{"name": "capacity", "capacity": 202, "comment": "updated again"},
						},
					},
				},
			},
		},
	}.Check(t, router)
	expectClusterCapacity(t, "shared", "shared", "capacity", -1, "")
	expectClusterCapacity(t, "east", "unshared", "capacity", 202, "updated again")

	//check PutCluster double-delete (i.e. delete should be idempotent)
	assert.HTTPRequest{
		Method:       "PUT",
		Path:         "/v1/clusters/east",
		ExpectStatus: 200,
		Body: assert.JSONObject{
			"cluster": assert.JSONObject{
				"services": []assert.JSONObject{
					{
						"type": "shared",
						"resources": []assert.JSONObject{
							{"name": "capacity", "capacity": -1},
						},
					},
				},
			},
		},
	}.Check(t, router)
	expectClusterCapacity(t, "shared", "shared", "capacity", -1, "")
}

func expectClusterCapacity(t *testing.T, clusterID, serviceType, resourceName string, capacity int64, comment string) {
	queryStr := `
	SELECT cr.capacity, cr.comment
	  FROM cluster_services cs
	  JOIN cluster_resources cr ON cr.service_id = cs.id
	 WHERE cs.cluster_id = ? AND cs.type = ? AND cr.name = ?
	`
	var (
		actualCapacity int64
		actualComment  string
	)
	err := db.DB.QueryRow(queryStr, clusterID, serviceType, resourceName).Scan(&actualCapacity, &actualComment)
	if err != nil {
		if err == sql.ErrNoRows {
			actualCapacity = -1
			actualComment = ""
		} else {
			t.Error(err)
			return
		}
	}

	if actualCapacity != capacity {
		t.Errorf("expectClusterCapacity failed: expected capacity = %d, but got %d for %s/%s/%s",
			capacity, actualCapacity, clusterID, serviceType, resourceName,
		)
	}
	if actualComment != comment {
		t.Errorf("expectClusterCapacity failed: expected comment = %#v, but got %#v for %s/%s/%s",
			comment, actualComment, clusterID, serviceType, resourceName,
		)
	}
}

func Test_DomainOperations(t *testing.T) {
	clusterName, pathtoData := "west", "fixtures/start-data.sql"
	cluster, router := setupTest(t, clusterName, pathtoData)
	discovery := cluster.DiscoveryPlugin.(*test.DiscoveryPlugin)

	//check GetDomain
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/domains/uuid-for-germany",
		ExpectStatus: 200,
		ExpectBody:   assert.JSONFixtureFile("./fixtures/domain-get-germany.json"),
	}.Check(t, router)
	//domain "france" covers some special cases: an infinite backend quota and
	//missing domain quota entries for one service
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/domains/uuid-for-france",
		ExpectStatus: 200,
		ExpectBody:   assert.JSONFixtureFile("./fixtures/domain-get-france.json"),
	}.Check(t, router)

	//domain "poland" is in a different cluster, so the X-Limes-Cluster-Id header
	//needs to be given
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/domains/uuid-for-poland",
		ExpectStatus: 404,
		ExpectBody:   assert.StringData("no such domain (if it was just created, try to POST /domains/discover)\n"),
	}.Check(t, router)
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/domains/uuid-for-poland",
		Header:       map[string]string{"X-Limes-Cluster-Id": "east"},
		ExpectStatus: 200,
		ExpectBody:   assert.JSONFixtureFile("./fixtures/domain-get-poland.json"),
	}.Check(t, router)

	//check ListDomains
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/domains",
		ExpectStatus: 200,
		ExpectBody:   assert.JSONFixtureFile("./fixtures/domain-list.json"),
	}.Check(t, router)
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/domains?service=unknown",
		ExpectStatus: 200,
		ExpectBody:   assert.JSONFixtureFile("./fixtures/domain-list-no-services.json"),
	}.Check(t, router)
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/domains?service=shared&resource=unknown",
		ExpectStatus: 200,
		ExpectBody:   assert.JSONFixtureFile("./fixtures/domain-list-no-resources.json"),
	}.Check(t, router)
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/domains?service=shared&resource=things",
		ExpectStatus: 200,
		ExpectBody:   assert.JSONFixtureFile("./fixtures/domain-list-filtered.json"),
	}.Check(t, router)

	//check cross-cluster ListDomains
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/domains",
		Header:       map[string]string{"X-Limes-Cluster-Id": "east"},
		ExpectStatus: 200,
		ExpectBody:   assert.JSONFixtureFile("./fixtures/domain-list-east.json"),
	}.Check(t, router)
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/domains",
		Header:       map[string]string{"X-Limes-Cluster-Id": "unknown"},
		ExpectStatus: 404,
		ExpectBody:   assert.StringData("no such cluster\n"),
	}.Check(t, router)

	//check DiscoverDomains
	discovery.StaticDomains = append(discovery.StaticDomains,
		limes.KeystoneDomain{Name: "spain", UUID: "uuid-for-spain"},
	)
	assert.HTTPRequest{
		Method:       "POST",
		Path:         "/v1/domains/discover",
		ExpectStatus: 202,
		ExpectBody:   assert.JSONFixtureFile("./fixtures/domain-discover.json"),
	}.Check(t, router)

	assert.HTTPRequest{
		Method:       "POST",
		Path:         "/v1/domains/discover",
		ExpectStatus: 204, //no content because no new domains discovered
		ExpectBody:   assert.StringData(""),
	}.Check(t, router)

	//check PutDomain error cases
	assert.HTTPRequest{
		Method:       "PUT",
		Path:         "/v1/domains/uuid-for-germany",
		ExpectStatus: 422,
		ExpectBody:   assert.StringData("cannot change shared/capacity quota: domain quota may not be smaller than sum of project quotas in that domain (20 B)\n"),
		Body: assert.JSONObject{
			"domain": assert.JSONObject{
				"services": []assert.JSONObject{
					{
						"type": "shared",
						"resources": []assert.JSONObject{
							//should fail because project quota sum exceeds new quota
							{"name": "capacity", "quota": 1},
						},
					},
				},
			},
		},
	}.Check(t, router)
	assert.HTTPRequest{
		Method:       "PUT",
		Path:         "/v1/domains/uuid-for-germany",
		ExpectStatus: 422,
		ExpectBody:   assert.StringData("cannot change shared/external_things quota: resource is managed externally\n"),
		Body: assert.JSONObject{
			"domain": assert.JSONObject{
				"services": []assert.JSONObject{
					{
						"type": "shared",
						"resources": []assert.JSONObject{
							//should fail because resource is externally managed, so setting quota via API is forbidden
							{"name": "external_things", "quota": 20},
						},
					},
				},
			},
		},
	}.Check(t, router)
	assert.HTTPRequest{
		Method:       "PUT",
		Path:         "/v1/domains/uuid-for-germany",
		ExpectStatus: 422,
		ExpectBody:   assert.StringData("cannot change shared/things quota: cannot convert value from MiB to <count> because units are incompatible\n"),
		Body: assert.JSONObject{
			"domain": assert.JSONObject{
				"services": []assert.JSONObject{
					{
						"type": "shared",
						"resources": []assert.JSONObject{
							//should fail because unit is incompatible with resource
							{"name": "things", "quota": 1, "unit": "MiB"},
						},
					},
				},
			},
		},
	}.Check(t, router)

	//check PutDomain error cases because of constraints
	assert.HTTPRequest{
		Method:       "PUT",
		Path:         "/v1/domains/uuid-for-france",
		ExpectStatus: 422,
		ExpectBody:   assert.StringData("cannot change shared/things quota: requested value \"15\" contradicts constraint \"at least 20\" for this domain and resource\ncannot change unshared/capacity quota: requested value \"30 B\" contradicts constraint \"at most 20 B\" for this domain and resource\ncannot change unshared/things quota: requested value \"19\" contradicts constraint \"exactly 20\" for this domain and resource\n"),
		Body: assert.JSONObject{
			"domain": assert.JSONObject{
				"services": []assert.JSONObject{
					{
						"type": "shared",
						"resources": []assert.JSONObject{
							//should fail because of "at least 20" constraint
							{"name": "things", "quota": 15},
						},
					},
					{
						"type": "unshared",
						"resources": []assert.JSONObject{
							//should fail because of "at most 20" constraint
							{"name": "capacity", "quota": 30},
							//should fail because of "exactly 20" constraint
							{"name": "things", "quota": 19},
						},
					},
				},
			},
		},
	}.Check(t, router)

	//check PutDomain happy path
	assert.HTTPRequest{
		Method:       "PUT",
		Path:         "/v1/domains/uuid-for-germany",
		ExpectStatus: 200,
		Body: assert.JSONObject{
			"domain": assert.JSONObject{
				"services": []assert.JSONObject{
					{
						"type": "shared",
						"resources": []assert.JSONObject{
							{"name": "capacity", "quota": 1234},
						},
					},
				},
			},
		},
	}.Check(t, router)
	expectDomainQuota(t, "germany", "shared", "capacity", 1234)
	assert.HTTPRequest{
		Method:       "PUT",
		Path:         "/v1/domains/uuid-for-germany",
		ExpectStatus: 200,
		Body: assert.JSONObject{
			"domain": assert.JSONObject{
				"services": []assert.JSONObject{
					{
						"type": "shared",
						"resources": []assert.JSONObject{
							{"name": "capacity", "quota": 1, "unit": limes.UnitMebibytes},
						},
					},
				},
			},
		},
	}.Check(t, router)
	expectDomainQuota(t, "germany", "shared", "capacity", 1<<20)

	//check PutDomain on a missing domain quota (see issue #36)
	assert.HTTPRequest{
		Method:       "PUT",
		Path:         "/v1/domains/uuid-for-france",
		ExpectStatus: 200,
		Body: assert.JSONObject{
			"domain": assert.JSONObject{
				"services": []assert.JSONObject{
					{
						"type": "shared",
						"resources": []assert.JSONObject{
							{"name": "capacity", "quota": 123},
						},
					},
				},
			},
		},
	}.Check(t, router)
	expectDomainQuota(t, "france", "shared", "capacity", 123)
}

func expectDomainQuota(t *testing.T, domainName, serviceType, resourceName string, expected uint64) {
	var actualQuota uint64
	err := db.DB.QueryRow(`
		SELECT dr.quota FROM domain_resources dr
		JOIN domain_services ds ON ds.id = dr.service_id
		JOIN domains d ON d.id = ds.domain_id
		WHERE d.name = ? AND ds.type = ? AND dr.name = ?`,
		domainName, serviceType, resourceName).Scan(&actualQuota)
	if err != nil {
		t.Fatal(err)
	}
	if actualQuota != expected {
		t.Errorf(
			"domain quota for %s/%s/%s was not updated in database",
			domainName, serviceType, resourceName,
		)
	}
}

func Test_ProjectOperations(t *testing.T) {
	clusterName, pathtoData := "west", "fixtures/start-data.sql"
	cluster, router := setupTest(t, clusterName, pathtoData)
	discovery := cluster.DiscoveryPlugin.(*test.DiscoveryPlugin)

	//check GetProject
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/domains/uuid-for-germany/projects/uuid-for-berlin",
		ExpectStatus: 200,
		ExpectBody:   assert.JSONFixtureFile("./fixtures/project-get-berlin.json"),
	}.Check(t, router)
	//check rendering of subresources
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/domains/uuid-for-germany/projects/uuid-for-berlin?detail",
		ExpectStatus: 200,
		ExpectBody:   assert.JSONFixtureFile("./fixtures/project-get-details-berlin.json"),
	}.Check(t, router)
	//check rendering of rates
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/domains/uuid-for-germany/projects/uuid-for-berlin?rates=only",
		ExpectStatus: 200,
		ExpectBody:   assert.JSONFixtureFile("./fixtures/project-get-only-rates-berlin.json"),
	}.Check(t, router)
	//dresden has a case of backend quota != quota
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/domains/uuid-for-germany/projects/uuid-for-dresden",
		ExpectStatus: 200,
		ExpectBody:   assert.JSONFixtureFile("./fixtures/project-get-dresden.json"),
	}.Check(t, router)
	//paris has a case of infinite backend quota
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/domains/uuid-for-france/projects/uuid-for-paris",
		ExpectStatus: 200,
		ExpectBody:   assert.JSONFixtureFile("./fixtures/project-get-paris.json"),
	}.Check(t, router)
	//warsaw is in a different cluster
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/domains/uuid-for-poland/projects/uuid-for-warsaw",
		ExpectStatus: 404,
		ExpectBody:   assert.StringData("no such domain (if it was just created, try to POST /domains/discover)\n"),
	}.Check(t, router)
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/domains/uuid-for-poland/projects/uuid-for-warsaw",
		Header:       map[string]string{"X-Limes-Cluster-Id": "east"},
		ExpectStatus: 200,
		ExpectBody:   assert.JSONFixtureFile("./fixtures/project-get-warsaw.json"),
	}.Check(t, router)

	//check ListProjects
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/domains/uuid-for-germany/projects",
		ExpectStatus: 200,
		ExpectBody:   assert.JSONFixtureFile("./fixtures/project-list.json"),
	}.Check(t, router)
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/domains/uuid-for-germany/projects?service=unknown",
		ExpectStatus: 200,
		ExpectBody:   assert.JSONFixtureFile("./fixtures/project-list-no-services.json"),
	}.Check(t, router)
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/domains/uuid-for-germany/projects?service=shared&resource=unknown",
		ExpectStatus: 200,
		ExpectBody:   assert.JSONFixtureFile("./fixtures/project-list-no-resources.json"),
	}.Check(t, router)
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/domains/uuid-for-germany/projects?service=shared&resource=things",
		ExpectStatus: 200,
		ExpectBody:   assert.JSONFixtureFile("./fixtures/project-list-filtered.json"),
	}.Check(t, router)

	//check ?area= filter (esp. interaction with ?service= filter)
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/domains/uuid-for-germany/projects?area=unknown",
		ExpectStatus: 200,
		ExpectBody:   assert.JSONFixtureFile("./fixtures/project-list-no-services.json"),
	}.Check(t, router)
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/domains/uuid-for-germany/projects?area=shared&service=unshared",
		ExpectStatus: 200,
		ExpectBody:   assert.JSONFixtureFile("./fixtures/project-list-no-services.json"),
	}.Check(t, router)
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/domains/uuid-for-germany/projects?area=shared&resource=things",
		ExpectStatus: 200,
		ExpectBody:   assert.JSONFixtureFile("./fixtures/project-list-filtered.json"),
	}.Check(t, router)

	//check cross-cluster ListProjects
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/domains/uuid-for-poland/projects",
		Header:       map[string]string{"X-Limes-Cluster-Id": "east"},
		ExpectStatus: 200,
		ExpectBody:   assert.JSONFixtureFile("./fixtures/project-list-poland.json"),
	}.Check(t, router)
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/domains/uuid-for-poland/projects",
		Header:       map[string]string{"X-Limes-Cluster-Id": "unknown"},
		ExpectStatus: 404,
		ExpectBody:   assert.StringData("no such cluster\n"),
	}.Check(t, router)

	//check DiscoverProjects
	discovery.StaticProjects["uuid-for-germany"] = append(discovery.StaticProjects["uuid-for-germany"],
		limes.KeystoneProject{Name: "frankfurt", UUID: "uuid-for-frankfurt"},
	)
	assert.HTTPRequest{
		Method:       "POST",
		Path:         "/v1/domains/uuid-for-germany/projects/discover",
		ExpectStatus: 202,
		ExpectBody:   assert.JSONFixtureFile("./fixtures/project-discover.json"),
	}.Check(t, router)

	assert.HTTPRequest{
		Method:       "POST",
		Path:         "/v1/domains/uuid-for-germany/projects/discover",
		ExpectStatus: 204, //no content because no new projects discovered
		ExpectBody:   assert.StringData(""),
	}.Check(t, router)

	//check SyncProject
	expectStaleProjectServices(t /*, nothing */)
	assert.HTTPRequest{
		Method:       "POST",
		Path:         "/v1/domains/uuid-for-germany/projects/uuid-for-dresden/sync",
		ExpectStatus: 202,
		ExpectBody:   assert.StringData(""),
	}.Check(t, router)
	expectStaleProjectServices(t, "dresden:shared", "dresden:unshared")

	//SyncProject should discover the given project if not yet done
	discovery.StaticProjects["uuid-for-germany"] = append(discovery.StaticProjects["uuid-for-germany"],
		limes.KeystoneProject{Name: "walldorf", UUID: "uuid-for-walldorf", ParentUUID: "uuid-for-germany"},
	)
	assert.HTTPRequest{
		Method:       "POST",
		Path:         "/v1/domains/uuid-for-germany/projects/uuid-for-walldorf/sync",
		ExpectStatus: 202,
		ExpectBody:   assert.StringData(""),
	}.Check(t, router)
	expectStaleProjectServices(t, "dresden:shared", "dresden:unshared", "walldorf:shared", "walldorf:unshared")

	//check GetProject for a project that has been discovered, but not yet synced
	assert.HTTPRequest{
		Method:       "GET",
		Path:         "/v1/domains/uuid-for-germany/projects/uuid-for-walldorf",
		ExpectStatus: 200,
		ExpectBody:   assert.JSONFixtureFile("./fixtures/project-get-walldorf-not-scraped-yet.json"),
	}.Check(t, router)

	//check PutProject: pre-flight checks
	assert.HTTPRequest{
		Method:       "PUT",
		Path:         "/v1/domains/uuid-for-germany/projects/uuid-for-berlin",
		ExpectStatus: 422,
		ExpectBody:   assert.StringData("cannot change shared/capacity quota: quota may not be lower than current usage\ncannot change shared/things quota: domain quota exceeded (maximum acceptable project quota is 20)\n"),
		Body: assert.JSONObject{
			"project": assert.JSONObject{
				"services": []assert.JSONObject{
					{
						"type": "shared",
						"resources": []assert.JSONObject{
							//should fail because usage exceeds new quota
							{"name": "capacity", "quota": 1},
							//should fail because domain quota exceeded
							{"name": "things", "quota": 30},
						},
					},
				},
			},
		},
	}.Check(t, router)

	assert.HTTPRequest{
		Method:       "PUT",
		Path:         "/v1/domains/uuid-for-germany/projects/uuid-for-dresden",
		ExpectStatus: 422,
		ExpectBody:   assert.StringData("cannot change shared/capacity quota: requested value \"9 B\" contradicts constraint \"at least 10 B\" for this project and resource\ncannot change unshared/capacity quota: requested value \"20 B\" contradicts constraint \"exactly 10 B\" for this project and resource\ncannot change unshared/things quota: requested value \"11\" contradicts constraint \"at most 10\" for this project and resource\n"),
		Body: assert.JSONObject{
			"project": assert.JSONObject{
				"services": []assert.JSONObject{
					{
						"type": "shared",
						"resources": []assert.JSONObject{
							//should fail because of "at least 10" constraint
							{"name": "capacity", "quota": 9},
						},
					},
					{
						"type": "unshared",
						"resources": []assert.JSONObject{
							//should fail because of "exactly 10" constraint
							{"name": "capacity", "quota": 20},
							//should fail because of "at most 10" constraint
							{"name": "things", "quota": 11},
						},
					},
				},
			},
		},
	}.Check(t, router)

	//check PutProject: quota admissible (i.e. will be persisted in DB), but
	//SetQuota fails for some reason (e.g. backend service down)
	plugin := cluster.QuotaPlugins["shared"].(*test.Plugin)
	plugin.SetQuotaFails = true
	assert.HTTPRequest{
		Method:       "PUT",
		Path:         "/v1/domains/uuid-for-germany/projects/uuid-for-berlin",
		ExpectStatus: 202,
		ExpectBody:   assert.StringData("quotas have been accepted, but some error(s) occurred while trying to write the quotas into the backend services:\nSetQuota failed as requested\n"),
		Body: assert.JSONObject{
			"project": assert.JSONObject{
				"services": []assert.JSONObject{
					{
						"type": "shared",
						"resources": []assert.JSONObject{
							{"name": "capacity", "quota": 5},
						},
					},
				},
			},
		},
	}.Check(t, router)
	var (
		actualQuota        uint64
		actualBackendQuota uint64
	)
	err := db.DB.QueryRow(`
		SELECT pr.quota, pr.backend_quota FROM project_resources pr
		JOIN project_services ps ON ps.id = pr.service_id
		JOIN projects p ON p.id = ps.project_id
		WHERE p.name = ? AND ps.type = ? AND pr.name = ?`,
		"berlin", "shared", "capacity").Scan(&actualQuota, &actualBackendQuota)
	if err != nil {
		t.Fatal(err)
	}
	if actualQuota != 5 {
		t.Error("quota was not updated in database")
	}
	if actualBackendQuota == 5 {
		t.Error("backend quota was updated in database even though SetQuota failed")
	}

	//check PutProject happy path
	plugin.SetQuotaFails = false
	assert.HTTPRequest{
		Method:       "PUT",
		Path:         "/v1/domains/uuid-for-germany/projects/uuid-for-berlin",
		ExpectStatus: 200,
		Body: assert.JSONObject{
			"project": assert.JSONObject{
				"services": []assert.JSONObject{
					{
						"type": "shared",
						"resources": []assert.JSONObject{
							{"name": "capacity", "quota": 6},
						},
					},
				},
			},
		},
	}.Check(t, router)
	err = db.DB.QueryRow(`
		SELECT pr.quota, pr.backend_quota FROM project_resources pr
		JOIN project_services ps ON ps.id = pr.service_id
		JOIN projects p ON p.id = ps.project_id
		WHERE p.name = ? AND ps.type = ? AND pr.name = ?`,
		"berlin", "shared", "capacity").Scan(&actualQuota, &actualBackendQuota)
	if err != nil {
		t.Fatal(err)
	}
	if actualQuota != 6 {
		t.Error("quota was not updated in database")
	}
	if actualBackendQuota != 6 {
		t.Error("backend quota was not updated in database")
	}
	//double-check with the plugin that the write was durable, and also that
	//SetQuota sent *all* quotas for that service (even for resources that were
	//not touched) as required by the QuotaPlugin interface documentation
	expectBackendQuota := map[string]uint64{
		"capacity":        6,  //as set above
		"things":          10, //unchanged
		"external_things": 1,  //unchanged
	}
	backendQuota, exists := plugin.OverrideQuota["uuid-for-berlin"]
	if !exists {
		t.Error("quota was not sent to backend")
	}
	if !reflect.DeepEqual(expectBackendQuota, backendQuota) {
		t.Errorf("expected backend quota %#v, but got %#v", expectBackendQuota, backendQuota)
	}
}

func expectStaleProjectServices(t *testing.T, pairs ...string) {
	queryStr := `
		SELECT p.name, ps.type
		  FROM projects p JOIN project_services ps ON ps.project_id = p.id
		 WHERE ps.stale
		 ORDER BY p.name, ps.type
	`
	var actualPairs []string

	err := db.ForeachRow(db.DB, queryStr, nil, func(rows *sql.Rows) error {
		var (
			projectName string
			serviceType string
		)
		err := rows.Scan(&projectName, &serviceType)
		if err != nil {
			return err
		}
		actualPairs = append(actualPairs, fmt.Sprintf("%s:%s", projectName, serviceType))
		return nil
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	if !reflect.DeepEqual(pairs, actualPairs) {
		t.Errorf("expected stale project services %v, but got %v", pairs, actualPairs)
	}
}

//p2s makes a "pointer to string".
func p2s(val string) *string {
	return &val
}

//p2u64 makes a "pointer to uint64".
func p2u64(val uint64) *uint64 {
	return &val
}
