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

package models

import "github.com/sapcc/limes/pkg/limes"

//ProjectResource represents a single resource within a single Keystone
//project.
type ProjectResource struct {
	ServiceID    uint64 //index into `project_services` table
	Name         string
	Quota        uint64
	Usage        uint64
	BackendQuota uint64
}

//ProjectResourcesTable enables table-level operations on project resources.
var ProjectResourcesTable = &Table{
	Name:       "project_resources",
	AllFields:  []string{"service_id", "name", "quota", "usage", "backend_quota"},
	makeRecord: func() Record { return &ProjectResource{} },
}

//Insert writes this record into the database as a new row.
func (pr *ProjectResource) Insert(db DBInterface) error {
	_, err := db.Exec(
		`INSERT INTO project_resources (service_id, name, quota, usage, backend_quota) VALUES ($1, $2, $3, $4, $5)`,
		pr.ServiceID, pr.Name, pr.Quota, pr.Usage, pr.BackendQuota)
	return err
}

//Table implements the Record interface.
func (pr *ProjectResource) Table() *Table {
	return ProjectResourcesTable
}

//ScanTargets implements the Record interface.
func (pr *ProjectResource) ScanTargets() []interface{} {
	return []interface{}{
		&pr.ServiceID, &pr.Name, &pr.Quota, &pr.Usage, &pr.BackendQuota,
	}
}

//Delete implements the Record interface.
func (pr *ProjectResource) Delete() error {
	_, err := limes.DB.Exec(
		`DELETE FROM project_resources WHERE service_id = $1 AND name = $2`,
		pr.ServiceID, pr.Name)
	return err
}