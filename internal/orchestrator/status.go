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

package orchestrator

import (
	"context"
	"fmt"

	"github.com/hyperledger/firefly-common/pkg/config"
	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly/internal/coreconfig"
	"github.com/hyperledger/firefly/pkg/core"
	"github.com/hyperledger/firefly/pkg/database"
)

func (or *orchestrator) getPlugins() core.NodeStatusPlugins {
	// Plugins can have more than one name, so they must be iterated over
	tokensArray := make([]*core.NodeStatusPlugin, 0)
	for _, plugin := range or.plugins.Tokens {
		tokensArray = append(tokensArray, &core.NodeStatusPlugin{
			Name:       plugin.Name,
			PluginType: plugin.Plugin.Name(),
		})
	}

	blockchainsArray := make([]*core.NodeStatusPlugin, 0)
	blockchainsArray = append(blockchainsArray, &core.NodeStatusPlugin{
		Name:       or.plugins.Blockchain.Name,
		PluginType: or.plugins.Blockchain.Plugin.Name(),
	})

	databasesArray := make([]*core.NodeStatusPlugin, 0)
	databasesArray = append(databasesArray, &core.NodeStatusPlugin{
		Name:       or.plugins.Database.Name,
		PluginType: or.plugins.Database.Plugin.Name(),
	})

	sharedstorageArray := make([]*core.NodeStatusPlugin, 0)
	sharedstorageArray = append(sharedstorageArray, &core.NodeStatusPlugin{
		Name:       or.plugins.SharedStorage.Name,
		PluginType: or.plugins.SharedStorage.Plugin.Name(),
	})

	dataexchangeArray := make([]*core.NodeStatusPlugin, 0)
	dataexchangeArray = append(dataexchangeArray, &core.NodeStatusPlugin{
		Name:       or.plugins.DataExchange.Name,
		PluginType: or.plugins.DataExchange.Plugin.Name(),
	})

	return core.NodeStatusPlugins{
		Blockchain:    blockchainsArray,
		Database:      databasesArray,
		SharedStorage: sharedstorageArray,
		DataExchange:  dataexchangeArray,
		Events:        or.events.GetPlugins(),
		Tokens:        tokensArray,
		Identity:      []*core.NodeStatusPlugin{},
	}
}

func (or *orchestrator) GetNodeUUID(ctx context.Context, ns string) (node *fftypes.UUID) {
	if or.node != nil {
		return or.node
	}
	status, err := or.GetStatus(ctx, ns)
	if err != nil {
		log.L(or.ctx).Warnf("Failed to query local node UUID: %s", err)
		return nil
	}
	if status.Node.Registered {
		or.node = status.Node.ID
	} else {
		log.L(or.ctx).Infof("Node not yet registered")
	}
	return or.node
}

func (or *orchestrator) GetStatus(ctx context.Context, ns string) (status *core.NodeStatus, err error) {

	org, err := or.identity.GetMultipartyRootOrg(ctx, ns)
	if err != nil {
		log.L(ctx).Warnf("Failed to query local org for status: %s", err)
	}
	status = &core.NodeStatus{
		Namespace: ns,
		Node: core.NodeStatusNode{
			Name: config.GetString(coreconfig.NodeName),
		},
		Org: core.NodeStatusOrg{
			Name: or.config.Multiparty.OrgName,
		},
		Plugins: or.getPlugins(),
	}

	if org != nil {
		status.Org.Registered = true
		status.Org.ID = org.ID
		status.Org.DID = org.DID

		// It's possible namespace will fallback to SystemNamespace (if configured to do so)
		ns = org.Namespace

		verifiers, _, err := or.networkmap.GetIdentityVerifiers(ctx, ns, org.ID.String(), database.VerifierQueryFactory.NewFilter(ctx).And())
		if err != nil {
			return nil, err
		}
		status.Org.Verifiers = make([]*core.VerifierRef, len(verifiers))
		for i, v := range verifiers {
			status.Org.Verifiers[i] = &v.VerifierRef
		}

		node, _, err := or.identity.CachedIdentityLookupNilOK(ctx, ns, fmt.Sprintf("%s%s", core.FireFlyNodeDIDPrefix, status.Node.Name))
		if err != nil {
			return nil, err
		}
		if node != nil && !node.Parent.Equals(org.ID) {
			log.L(ctx).Errorf("Specified node name is in use by another org: %s", err)
			node = nil
		}
		if node != nil {
			status.Node.Registered = true
			status.Node.ID = node.ID
		}
	}

	return status, nil
}
