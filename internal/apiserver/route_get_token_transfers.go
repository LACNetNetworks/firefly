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
	"github.com/hyperledger/firefly/pkg/database"
)

var getTokenTransfers = &ffapi.Route{
	Name:       "getTokenTransfers",
	Path:       "tokens/transfers",
	Method:     http.MethodGet,
	PathParams: nil,
	QueryParams: []*ffapi.QueryParam{
		{Name: "fromOrTo", Description: coremsgs.APIParamsTokenTransferFromOrTo},
	},
	Description:     coremsgs.APIEndpointsGetTokenTransfers,
	JSONInputValue:  nil,
	JSONOutputValue: func() interface{} { return []*core.TokenTransfer{} },
	JSONOutputCodes: []int{http.StatusOK},
	Extensions: &coreExtensions{
		FilterFactory: database.TokenTransferQueryFactory,
		CoreJSONHandler: func(r *ffapi.APIRequest, cr *coreRequest) (output interface{}, err error) {
			filter := cr.filter
			if fromOrTo, ok := r.QP["fromOrTo"]; ok {
				fb := database.TokenTransferQueryFactory.NewFilter(cr.ctx)
				filter.Condition(
					fb.Or().
						Condition(fb.Eq("from", fromOrTo)).
						Condition(fb.Eq("to", fromOrTo)))
			}
			return filterResult(cr.or.Assets().GetTokenTransfers(cr.ctx, extractNamespace(r.PP), filter))
		},
	},
}
