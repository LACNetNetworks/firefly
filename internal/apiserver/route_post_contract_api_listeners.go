// Copyright © 2022 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package apiserver

import (
	"net/http"

	"github.com/hyperledger/firefly-common/pkg/ffapi"
	"github.com/hyperledger/firefly/internal/coremsgs"
	"github.com/hyperledger/firefly/pkg/core"
)

var postContractAPIListeners = &ffapi.Route{
	Name:   "postContractAPIListeners",
	Path:   "apis/{apiName}/listeners/{eventPath}",
	Method: http.MethodPost,
	PathParams: []*ffapi.PathParam{
		{Name: "apiName", Description: coremsgs.APIParamsContractAPIName},
		{Name: "eventPath", Description: coremsgs.APIParamsEventPath},
	},
	QueryParams:     []*ffapi.QueryParam{},
	Description:     coremsgs.APIEndpointsPostNewContractListener,
	JSONInputValue:  func() interface{} { return &core.ContractListener{} },
	JSONOutputValue: func() interface{} { return &core.ContractListener{} },
	JSONOutputCodes: []int{http.StatusOK},
	Extensions: &coreExtensions{
		CoreJSONHandler: func(r *ffapi.APIRequest, cr *coreRequest) (output interface{}, err error) {
			return cr.or.Contracts().AddContractAPIListener(cr.ctx, extractNamespace(r.PP), r.PP["apiName"], r.PP["eventPath"], r.Input.(*core.ContractListener))
		},
	},
}
