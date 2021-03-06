/*******************************************************************************
*
* Copyright 2017-2018 SAP SE
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
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sapcc/go-bits/gopherpolicy"
	"github.com/sapcc/go-bits/respondwith"
	"github.com/sapcc/limes"
	"github.com/sapcc/limes/pkg/audit"
	"github.com/sapcc/limes/pkg/core"
	"github.com/sapcc/limes/pkg/db"
	"github.com/sapcc/limes/pkg/reports"
)

//QuotaUpdater contains the shared code for domain and project PUT requests.
//See func PutDomain and func PutProject for how it's used.
type QuotaUpdater struct {
	//scope
	Cluster *core.Cluster
	Domain  *db.Domain  //always set (for project quota updates, contains the project's domain)
	Project *db.Project //nil for domain quota updates
	//AuthZ info
	CanRaise   bool
	CanRaiseLP bool //low-privilege raise
	CanLower   bool

	//filled by ValidateInput(), key = service type + resource name
	Requests map[string]map[string]QuotaRequest
}

//QuotaRequest describes a single quota value that a PUT request wants to
//change. It appears in type QuotaUpdater.
type QuotaRequest struct {
	OldValue        uint64
	NewValue        uint64
	Unit            limes.Unit
	ValidationError *core.QuotaValidationError
}

//ScopeType is used for constructing error messages.
func (u QuotaUpdater) ScopeType() string {
	if u.Project == nil {
		return "domain"
	}
	return "project"
}

//QuotaConstraints returns the quota constraints that apply to this updater's scope.
func (u QuotaUpdater) QuotaConstraints() core.QuotaConstraints {
	if u.Cluster.QuotaConstraints == nil {
		return nil
	}
	if u.Project == nil {
		return u.Cluster.QuotaConstraints.Domains[u.Domain.Name]
	}
	return u.Cluster.QuotaConstraints.Projects[u.Domain.Name][u.Project.Name]
}

////////////////////////////////////////////////////////////////////////////////
// validation phase

//ValidateInput reads the given input and validates the quotas contained therein.
//Results are collected into u.Requests. The return value is only set for unexpected
//errors, not for validation errors.
func (u *QuotaUpdater) ValidateInput(input limes.QuotaRequest, dbi db.Interface) error {
	//gather a report on the domain's quotas to decide whether a quota update is legal
	domainReport, err := GetDomainReport(u.Cluster, *u.Domain, dbi, reports.Filter{})
	if err != nil {
		return err
	}
	//for project scope, we also need a project report for validation
	var projectReport *limes.ProjectReport
	if u.Project != nil {
		projectReport, err = GetProjectReport(u.Cluster, *u.Domain, *u.Project, dbi, reports.Filter{}, false)
		if err != nil {
			return err
		}
	}

	//go through all services and resources and validate the requested quotas
	u.Requests = make(map[string]map[string]QuotaRequest)
	for _, quotaPlugin := range u.Cluster.QuotaPlugins {
		srv := quotaPlugin.ServiceInfo()
		u.Requests[srv.Type] = make(map[string]QuotaRequest)

		for _, res := range quotaPlugin.Resources() {
			//find the report data for this resource
			var (
				domRes  *limes.DomainResourceReport
				projRes *limes.ProjectResourceReport
			)
			if domainService, exists := domainReport.Services[srv.Type]; exists {
				domRes = domainService.Resources[res.Name]
			}
			if domRes == nil {
				return fmt.Errorf("no domain report for resource %s/%s", srv.Type, res.Name)
			}
			if u.Project != nil {
				if projectService, exists := projectReport.Services[srv.Type]; exists {
					projRes = projectService.Resources[res.Name]
				}
				if projRes == nil {
					return fmt.Errorf("no project report for resource %s/%s", srv.Type, res.Name)
				}
			}

			req := QuotaRequest{
				OldValue: domRes.DomainQuota,
				Unit:     domRes.Unit,
			}
			if u.Project != nil {
				req.OldValue = projRes.Quota
			}

			//convert given value to correct unit
			newQuota, exists := input[srv.Type][res.Name]
			if !exists {
				continue
			}
			req.NewValue, err = core.ConvertUnitFor(u.Cluster, srv.Type, res.Name, newQuota)
			if err != nil {
				req.ValidationError = &core.QuotaValidationError{
					Status:  http.StatusUnprocessableEntity,
					Message: err.Error(),
				}
			} else {
				//skip this resource entirely if no change is requested
				if req.OldValue == req.NewValue {
					continue //with next resource
				}
				//value is valid and novel -> perform further validation
				req.ValidationError = u.validateQuota(srv, res, *domRes, projRes, req.NewValue)
			}

			u.Requests[srv.Type][res.Name] = req
		}
	}

	return nil
}

func (u QuotaUpdater) validateQuota(srv limes.ServiceInfo, res limes.ResourceInfo, domRes limes.DomainResourceReport, projRes *limes.ProjectResourceReport, newQuota uint64) *core.QuotaValidationError {
	//can we change this quota at all?
	if res.ExternallyManaged {
		return &core.QuotaValidationError{
			Status:  http.StatusUnprocessableEntity,
			Message: "resource is managed externally",
		}
	}

	//check quota constraints
	constraint := u.QuotaConstraints()[srv.Type][res.Name]
	err := constraint.Validate(newQuota)
	if err != nil {
		err.Message += fmt.Sprintf(" for this %s and resource", u.ScopeType())
		return err
	}

	//check authorization for quota change
	var (
		oldQuota uint64
		lprLimit uint64
	)
	if u.Project == nil {
		oldQuota = domRes.DomainQuota
		lprLimit = u.Cluster.LowPrivilegeRaise.LimitsForDomains[srv.Type][res.Name]
	} else {
		oldQuota = projRes.Quota
		lprLimit = u.Cluster.LowPrivilegeRaise.LimitsForProjects[srv.Type][res.Name]
		if !u.Cluster.Config.LowPrivilegeRaise.IsAllowedForProjectsIn(u.Domain.Name) {
			lprLimit = 0
		}
	}
	err = u.validateAuthorization(oldQuota, newQuota, lprLimit, res.Unit)
	if err != nil {
		err.Message += fmt.Sprintf(" in this %s", u.ScopeType())
		return err
	}

	//specific rules for domain quotas vs. project quotas
	if u.Project == nil {
		return u.validateDomainQuota(domRes, newQuota)
	}
	return u.validateProjectQuota(domRes, *projRes, newQuota)
}

func (u QuotaUpdater) validateAuthorization(oldQuota, newQuota, lprLimit uint64, unit limes.Unit) *core.QuotaValidationError {
	if oldQuota >= newQuota {
		if u.CanLower {
			return nil
		}
		return &core.QuotaValidationError{
			Status:  http.StatusForbidden,
			Message: "user is not allowed to lower quotas",
		}
	}

	if u.CanRaise {
		return nil
	}
	if u.CanRaiseLP && lprLimit > 0 {
		if newQuota <= lprLimit {
			return nil
		}
		return &core.QuotaValidationError{
			Status:       http.StatusForbidden,
			Message:      "user is not allowed to raise quotas that high",
			MaximumValue: &lprLimit,
			Unit:         unit,
		}
	}
	return &core.QuotaValidationError{
		Status:  http.StatusForbidden,
		Message: "user is not allowed to raise quotas",
	}
}

func (u QuotaUpdater) validateDomainQuota(report limes.DomainResourceReport, newQuota uint64) *core.QuotaValidationError {
	//check that existing project quotas fit into new domain quota
	if newQuota < report.ProjectsQuota {
		min := report.ProjectsQuota
		return &core.QuotaValidationError{
			Status:       http.StatusConflict,
			Message:      "domain quota may not be smaller than sum of project quotas in that domain",
			MinimumValue: &min,
			Unit:         report.Unit,
		}
	}

	return nil
}

func (u QuotaUpdater) validateProjectQuota(domRes limes.DomainResourceReport, projRes limes.ProjectResourceReport, newQuota uint64) *core.QuotaValidationError {
	//check that usage fits into quota
	if projRes.Usage > newQuota {
		min := projRes.Usage
		return &core.QuotaValidationError{
			Status:       http.StatusConflict,
			Message:      "quota may not be lower than current usage",
			MinimumValue: &min,
			Unit:         projRes.Unit,
		}
	}

	//check that domain quota is not exceeded
	//
	//NOTE: It looks like an arithmetic overflow (or rather, underflow) is
	//possible here, but it isn't. projectsQuota is the sum over all current
	//project quotas, including res.Quota, and thus is always bigger (since these
	//quotas are all unsigned). Also, we're doing everything in a transaction, so
	//an overflow because of concurrent quota changes is also out of the
	//question.
	newProjectsQuota := domRes.ProjectsQuota - projRes.Quota + newQuota
	if newProjectsQuota > domRes.DomainQuota {
		maxQuota := domRes.DomainQuota - (domRes.ProjectsQuota - projRes.Quota)
		if domRes.DomainQuota < domRes.ProjectsQuota-projRes.Quota {
			maxQuota = 0
		}
		return &core.QuotaValidationError{
			Status:       http.StatusConflict,
			Message:      "domain quota exceeded",
			MaximumValue: &maxQuota,
			Unit:         domRes.Unit,
		}
	}

	return nil
}

//IsValid returns true if all u.Requests are valid (i.e. ValidationError == nil).
func (u QuotaUpdater) IsValid() bool {
	for _, reqs := range u.Requests {
		for _, req := range reqs {
			if req.ValidationError != nil {
				return false
			}
		}
	}
	return true
}

//WriteSimulationReport produces the HTTP response for the POST /simulate-put
//endpoints.
func (u QuotaUpdater) WriteSimulationReport(w http.ResponseWriter) {
	type unacceptableResource struct {
		ServiceType  string `json:"service_type"`
		ResourceName string `json:"resource_name"`
		core.QuotaValidationError
	}
	var result struct {
		IsValid               bool                   `json:"success,keepempty"`
		UnacceptableResources []unacceptableResource `json:"unacceptable_resources,omitempty"`
	}
	result.IsValid = true //until proven otherwise

	for srvType, reqs := range u.Requests {
		for resName, req := range reqs {
			if req.ValidationError != nil {
				result.IsValid = false
				result.UnacceptableResources = append(result.UnacceptableResources,
					unacceptableResource{
						ServiceType:          srvType,
						ResourceName:         resName,
						QuotaValidationError: *req.ValidationError,
					},
				)
			}
		}
	}

	//deterministic ordering for unit tests
	sort.Slice(result.UnacceptableResources, func(i, j int) bool {
		srvType1 := result.UnacceptableResources[i].ServiceType
		srvType2 := result.UnacceptableResources[j].ServiceType
		if srvType1 != srvType2 {
			return srvType1 < srvType2
		}
		resName1 := result.UnacceptableResources[i].ResourceName
		resName2 := result.UnacceptableResources[j].ResourceName
		return resName1 < resName2
	})

	respondwith.JSON(w, http.StatusOK, result)
}

//WritePutErrorResponse produces a negative HTTP response for this PUT request.
//It may only be used when `u.IsValid()` is false.
func (u QuotaUpdater) WritePutErrorResponse(w http.ResponseWriter) {
	var lines []string
	hasSubstatus := make(map[int]bool)

	//collect error messages
	for srvType, reqs := range u.Requests {
		for resName, req := range reqs {
			err := req.ValidationError
			if err != nil {
				hasSubstatus[err.Status] = true
				line := fmt.Sprintf("cannot change %s/%s quota: %s",
					srvType, resName, err.Message)
				var notes []string
				if err.MinimumValue != nil {
					notes = append(notes, fmt.Sprintf("minimum acceptable %s quota is %v",
						u.ScopeType(), limes.ValueWithUnit{Value: *err.MinimumValue, Unit: err.Unit}))
				}
				if err.MaximumValue != nil {
					notes = append(notes, fmt.Sprintf("maximum acceptable %s quota is %v",
						u.ScopeType(), limes.ValueWithUnit{Value: *err.MaximumValue, Unit: err.Unit}))
				}
				if len(notes) > 0 {
					line += fmt.Sprintf(" (%s)", strings.Join(notes, ", "))
				}
				lines = append(lines, line)
			}
		}
	}
	sort.Strings(lines) //for determinism in unit test
	msg := strings.Join(lines, "\n")

	//when all errors have the same status, report that; otherwise use 422
	//(Unprocessable Entity) as a reasonable overall default
	status := http.StatusUnprocessableEntity
	if len(hasSubstatus) == 1 {
		for s := range hasSubstatus {
			status = s
		}
	}
	http.Error(w, msg, status)
}

////////////////////////////////////////////////////////////////////////////////
// integration with package audit

//CommitAuditTrail prepares an audit.Trail instance for this updater and
//commits it.
func (u QuotaUpdater) CommitAuditTrail(token *gopherpolicy.Token, r *http.Request, requestTime time.Time) {
	var trail audit.Trail

	projectUUID := ""
	if u.Project != nil {
		projectUUID = u.Project.UUID
	}

	invalid := !u.IsValid()
	statusCode := http.StatusOK
	if invalid {
		statusCode = http.StatusUnprocessableEntity
	}

	for srvType, reqs := range u.Requests {
		for resName, req := range reqs {
			// low-privilege-raise metrics
			if u.CanRaiseLP && !u.CanRaise {
				labels := prometheus.Labels{
					"os_cluster": u.Cluster.ID,
					"service":    srvType,
					"resource":   resName,
				}
				if u.ScopeType() == "domain" {
					if invalid {
						lowPrivilegeRaiseDomainFailureCounter.With(labels).Inc()
					} else {
						lowPrivilegeRaiseDomainSuccessCounter.With(labels).Inc()
					}
				} else {
					if invalid {
						lowPrivilegeRaiseProjectFailureCounter.With(labels).Inc()
					} else {
						lowPrivilegeRaiseProjectSuccessCounter.With(labels).Inc()
					}
				}
			}

			//if !u.IsValid(), then all requested quotas in this PUT are considered
			//invalid (and none are committed), so set the rejectReason to explain this
			rejectReason := ""
			if invalid {
				if req.ValidationError == nil {
					rejectReason = "cannot commit this because other values in this request are unacceptable"
				} else {
					rejectReason = req.ValidationError.Message
				}
			}

			trail.Add(audit.EventParams{
				Token:      token,
				Request:    r,
				ReasonCode: statusCode,
				Time:       requestTime,
				Target: audit.QuotaEventTarget{
					DomainID:     u.Domain.UUID,
					ProjectID:    projectUUID, //is empty for domain quota updates, see above
					ServiceType:  srvType,
					ResourceName: resName,
					OldQuota:     req.OldValue,
					NewQuota:     req.NewValue,
					QuotaUnit:    req.Unit,
					RejectReason: rejectReason,
				},
			})
		}
	}

	trail.Commit(u.Cluster.ID, u.Cluster.Config.CADF)
}
