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

package audit

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"

	"github.com/gofrs/uuid"
	"github.com/sapcc/go-bits/gopherpolicy"
	"github.com/sapcc/go-bits/logg"
	"github.com/sapcc/limes"
)

var observerUUID string

func init() {
	observerUUID = generateUUID()
}

//CADFEvent contains the CADF event format according to CADF spec (section 6.6.1 Event)
//and includes extensions for better auditing.
type CADFEvent struct {
	TypeURI     string       `json:"typeURI"`
	ID          string       `json:"id"`
	EventTime   string       `json:"eventTime"`
	EventType   string       `json:"eventType"`
	Action      string       `json:"action"`
	Outcome     string       `json:"outcome"`
	Reason      Reason       `json:"reason,omitempty"`
	Initiator   Resource     `json:"initiator"`
	Target      Resource     `json:"target"`
	Observer    Resource     `json:"observer"`
	Attachments []Attachment `json:"attachments,omitempty"`
	RequestPath string       `json:"requestPath,omitempty"`
}

//Resource is a substructure of CADFEvent and contains attributes describing a (OpenStack-) resource.
type Resource struct {
	TypeURI   string `json:"typeURI"`
	Name      string `json:"name,omitempty"`
	Domain    string `json:"domain,omitempty"`
	ID        string `json:"id"`
	Addresses []struct {
		URL  string `json:"url"`
		Name string `json:"name,omitempty"`
	} `json:"addresses,omitempty"`
	Host        *Host        `json:"host,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
	ProjectID   string       `json:"project_id,omitempty"`
	DomainID    string       `json:"domain_id,omitempty"`
}

//Attachment is a substructure of CADFEvent and contains self-describing extensions to the event.
type Attachment struct {
	Name    string      `json:"name,omitempty"`
	TypeURI string      `json:"typeURI"`
	Content interface{} `json:"content"`
}

//Reason is a substructure of CADFevent containing data for the event outcome's reason.
type Reason struct {
	ReasonType string `json:"reasonType"`
	ReasonCode string `json:"reasonCode"`
}

//Host is a substructure of Resource containing data for the event initiator's host.
type Host struct {
	ID       string `json:"id,omitempty"`
	Address  string `json:"address,omitempty"`
	Agent    string `json:"agent,omitempty"`
	Platform string `json:"platform,omitempty"`
}

//ResourceEventParams contains parameters for creating an audit event for changes regarding some service/resource.
type ResourceEventParams struct {
	Token        *gopherpolicy.Token
	Request      *http.Request
	ReasonCode   int
	Time         string
	DomainID     string
	ProjectID    string
	ServiceType  string
	ResourceName string
	OldQuota     uint64
	NewQuota     uint64
	QuotaUnit    limes.Unit
	RejectReason string
}

//newEvent takes the necessary parameters and returns a new audit event.
func (p ResourceEventParams) newEvent() CADFEvent {
	targetID := p.ProjectID
	if p.ProjectID == "" {
		targetID = p.DomainID
	}

	outcome := "failure"
	if p.ReasonCode == http.StatusOK {
		outcome = "success"
	}

	return CADFEvent{
		TypeURI:   "http://schemas.dmtf.org/cloud/audit/1.0/event",
		ID:        generateUUID(),
		EventTime: p.Time,
		EventType: "activity",
		Action:    "update",
		Outcome:   outcome,
		Reason: Reason{
			ReasonType: "HTTP",
			ReasonCode: strconv.Itoa(p.ReasonCode),
		},
		Initiator: Resource{
			TypeURI:   "service/security/account/user",
			Name:      p.Token.Context.Auth["user_name"],
			ID:        p.Token.Context.Auth["user_id"],
			Domain:    p.Token.Context.Auth["domain_name"],
			DomainID:  p.Token.Context.Auth["domain_id"],
			ProjectID: p.Token.Context.Auth["project_id"],
			Host: &Host{
				Address: tryStripPort(p.Request.RemoteAddr),
				Agent:   p.Request.Header.Get("User-Agent"),
			},
		},
		Target: Resource{
			TypeURI:   fmt.Sprintf("service/%s/%s/quota", p.ServiceType, p.ResourceName),
			ID:        targetID,
			DomainID:  p.DomainID,
			ProjectID: p.ProjectID,
			Attachments: []Attachment{{
				Name:    "payload",
				TypeURI: "mime:application/json",
				Content: attachmentContent{
					OldQuota:     p.OldQuota,
					NewQuota:     p.NewQuota,
					Unit:         p.QuotaUnit,
					RejectReason: p.RejectReason},
			}},
		},
		Observer: Resource{
			TypeURI: "service/resources",
			Name:    "limes",
			ID:      observerUUID,
		},
		RequestPath: p.Request.URL.String(),
	}
}

//This type is needed for the custom MarshalJSON behavior.
type attachmentContent struct {
	OldQuota     uint64
	NewQuota     uint64
	Unit         limes.Unit
	NewStatus    bool //for quota bursting
	RejectReason string
}

//MarshalJSON implements the json.Marshaler interface.
func (a attachmentContent) MarshalJSON() ([]byte, error) {
	//copy data into a struct that does not have a custom MarshalJSON
	data := struct {
		OldQuota     uint64     `json:"oldQuota,omitempty"`
		NewQuota     uint64     `json:"newQuota,omitempty"`
		NewStatus    bool       `json:"newStatus,omitempty"`
		Unit         limes.Unit `json:"unit,omitempty"`
		RejectReason string     `json:"rejectReason,omitempty"`
	}{
		OldQuota:     a.OldQuota,
		NewQuota:     a.NewQuota,
		NewStatus:    a.NewStatus,
		Unit:         a.Unit,
		RejectReason: a.RejectReason,
	}
	//Hermes does not accept a JSON object at target.attachments[].content, so we need
	//to wrap the marshaled JSON into a JSON string
	bytes, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return json.Marshal(string(bytes))
}

//BurstEventParams contains parameters for creating an audit event for changes regarding quota bursting.
type BurstEventParams struct {
	Token        *gopherpolicy.Token
	Request      *http.Request
	ReasonCode   int
	Time         string
	DomainID     string
	ProjectID    string
	NewStatus    bool
	RejectReason string
}

func (p BurstEventParams) newEvent() CADFEvent {
	outcome := "failure"
	attachCont := attachmentContent{
		RejectReason: p.RejectReason,
	}
	if p.ReasonCode == http.StatusOK {
		outcome = "success"
		attachCont.NewStatus = p.NewStatus
	}

	return CADFEvent{
		TypeURI:   "http://schemas.dmtf.org/cloud/audit/1.0/event",
		ID:        generateUUID(),
		EventTime: p.Time,
		EventType: "activity",
		Action:    "update",
		Outcome:   outcome,
		Reason: Reason{
			ReasonType: "HTTP",
			ReasonCode: strconv.Itoa(p.ReasonCode),
		},
		Initiator: Resource{
			TypeURI:   "service/security/account/user",
			Name:      p.Token.Context.Auth["user_name"],
			ID:        p.Token.Context.Auth["user_id"],
			Domain:    p.Token.Context.Auth["domain_name"],
			DomainID:  p.Token.Context.Auth["domain_id"],
			ProjectID: p.Token.Context.Auth["project_id"],
			Host: &Host{
				Address: tryStripPort(p.Request.RemoteAddr),
				Agent:   p.Request.Header.Get("User-Agent"),
			},
		},
		Target: Resource{
			TypeURI:   "service/resources/bursting",
			ID:        p.ProjectID,
			DomainID:  p.DomainID,
			ProjectID: p.ProjectID,
			Attachments: []Attachment{{
				Name:    "payload",
				TypeURI: "mime:application/json",
				Content: attachCont,
			}},
		},
		Observer: Resource{
			TypeURI: "service/resources",
			Name:    "limes",
			ID:      observerUUID,
		},
		RequestPath: p.Request.URL.String(),
	}
}

//Generate an UUID based on random numbers (RFC 4122).
func generateUUID() string {
	u, err := uuid.NewV4()
	if err != nil {
		logg.Fatal(err.Error())
	}

	return u.String()
}

func tryStripPort(hostPort string) string {
	host, _, err := net.SplitHostPort(hostPort)
	if err == nil {
		return host
	}
	return hostPort
}
