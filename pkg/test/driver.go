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

package test

import (
	policy "github.com/databus23/goslo.policy"
	"github.com/sapcc/limes/pkg/limes"
)

//Driver is a limes.Driver implementation for unit tests that does not talk to
//an actual OpenStack. It returns a static set of domains and projects, and a
//static set of quota/usage values for any project.
type Driver struct {
	cluster *limes.Cluster
}

//NewDriver creates a Driver instance. The Cluster does not need to have the
//Keystone auth params fields filled. Only the cluster ID and service list are
//required.
func NewDriver(cluster *limes.Cluster) *Driver {
	return &Driver{
		cluster: cluster,
	}
}

//Cluster implements the limes.Driver interface.
func (d *Driver) Cluster() *limes.Cluster {
	return d.cluster
}

//ValidateToken implements the limes.Driver interface.
func (d *Driver) ValidateToken(token string) (policy.Context, error) {
	return policy.Context{}, nil
}
