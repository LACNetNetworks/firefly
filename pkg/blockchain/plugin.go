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

package blockchain

import (
	"context"

	"github.com/hyperledger/firefly-common/pkg/config"
	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly/internal/metrics"
	"github.com/hyperledger/firefly/pkg/core"
)

// Plugin is the interface implemented by each blockchain plugin
type Plugin interface {
	core.Named

	// InitConfig initializes the set of configuration options that are valid, with defaults. Called on all plugins.
	InitConfig(config config.Section)

	// Init initializes the plugin, with configuration
	Init(ctx context.Context, config config.Section, metrics metrics.Manager) error

	// RegisterListener registers a listener to receive callbacks
	RegisterListener(listener Callbacks)

	// ConfigureContract initializes the subscription to the FireFly contract
	// - Checks the provided contract info against the plugin's configuration, and updates it as needed
	// - Initializes the contract info for performing BatchPin transactions, and initializes subscriptions for BatchPin events
	ConfigureContract(ctx context.Context, contracts *core.FireFlyContracts) (err error)

	// TerminateContract marks the given event as the last one to be parsed on the current FireFly contract
	// - Validates that the event came from the currently active FireFly contract
	// - Re-initializes the plugin against the next configured FireFly contract
	// - Updates the provided contract info to record the point of termination and the newly active contract
	TerminateContract(ctx context.Context, contracts *core.FireFlyContracts, termination *Event) (err error)

	// Blockchain interface must not deliver any events until start is called
	Start() error

	// Capabilities returns capabilities - not called until after Init
	Capabilities() *Capabilities

	// VerifierType returns the verifier (key) type that is used by this blockchain
	VerifierType() core.VerifierType

	// NormalizeSigningKey verifies that the supplied identity string is valid syntax according to the protocol.
	// - Can apply transformations to the supplied signing identity (only), such as lower case.
	// - Can perform sophisicated resolution, such as resolving a Fabric shortname to a MSP ID, or using an external REST API plugin to resolve a HD wallet address
	NormalizeSigningKey(ctx context.Context, keyRef string) (string, error)

	// SubmitBatchPin sequences a batch of message globally to all viewers of a given ledger
	SubmitBatchPin(ctx context.Context, nsOpID string, signingKey string, batch *BatchPin) error

	// SubmitNetworkAction writes a special "BatchPin" event which signals the plugin to take an action
	SubmitNetworkAction(ctx context.Context, nsOpID string, signingKey string, action core.NetworkActionType) error

	// InvokeContract submits a new transaction to be executed by custom on-chain logic
	InvokeContract(ctx context.Context, nsOpID string, signingKey string, location *fftypes.JSONAny, method *core.FFIMethod, input map[string]interface{}, options map[string]interface{}) error

	// QueryContract executes a method via custom on-chain logic and returns the result
	QueryContract(ctx context.Context, location *fftypes.JSONAny, method *core.FFIMethod, input map[string]interface{}, options map[string]interface{}) (interface{}, error)

	// AddContractListener adds a new subscription to a user-specified contract and event
	AddContractListener(ctx context.Context, subscription *core.ContractListenerInput) error

	// DeleteContractListener deletes a previously-created subscription
	DeleteContractListener(ctx context.Context, subscription *core.ContractListener) error

	// GetFFIParamValidator returns a blockchain-plugin-specific validator for FFIParams and their JSON Schema
	GetFFIParamValidator(ctx context.Context) (core.FFIParamValidator, error)

	// GenerateFFI returns an FFI from a blockchain specific interface format e.g. an Ethereum ABI
	GenerateFFI(ctx context.Context, generationRequest *core.FFIGenerationRequest) (*core.FFI, error)

	// NormalizeContractLocation validates and normalizes the formatting of the location JSON
	NormalizeContractLocation(ctx context.Context, location *fftypes.JSONAny) (*fftypes.JSONAny, error)

	// GenerateEventSignature generates a strigified signature for the event, incorporating any fields significant to identifying the event as unique
	GenerateEventSignature(ctx context.Context, event *core.FFIEventDefinition) string

	// NetworkVersion returns the version of the network rules being used by this plugin
	NetworkVersion() int
}

const FireFlyActionPrefix = "firefly:"

// Callbacks is the interface provided to the blockchain plugin, to allow it to pass events back to firefly.
//
// Events must be delivered sequentially, such that event 2 is not delivered until the callback invoked for event 1
// has completed. However, it does not matter if these events are workload balance between the firefly core
// cluster instances of the node.
type Callbacks interface {
	// BlockchainOpUpdate notifies firefly of an update to this plugin's operation within a transaction.
	// Only success/failure and errorMessage (for errors) are modeled.
	// opOutput can be used to add opaque protocol specific JSON from the plugin (protocol transaction ID etc.)
	// Note this is an optional hook information, and stored separately to the confirmation of the actual event that was being submitted/sequenced.
	// Only the party submitting the transaction will see this data.
	BlockchainOpUpdate(plugin Plugin, nsOpID string, txState TransactionStatus, blockchainTXID, errorMessage string, opOutput fftypes.JSONObject)

	// BatchPinComplete notifies on the arrival of a sequenced batch of messages, which might have been
	// submitted by us, or by any other authorized party in the network.
	//
	// Error should only be returned in shutdown scenarios
	BatchPinComplete(batch *BatchPin, signingKey *core.VerifierRef) error

	// BlockchainNetworkAction notifies on the arrival of a network operator action
	//
	// Error should only be returned in shutdown scenarios
	BlockchainNetworkAction(action string, event *Event, signingKey *core.VerifierRef) error

	// BlockchainEvent notifies on the arrival of any event from a user-created subscription.
	BlockchainEvent(event *EventWithSubscription) error
}

// Capabilities the supported featureset of the blockchain
// interface implemented by the plugin, with the specified config
type Capabilities struct {
}

// TransactionStatus is the only architecturally significant thing that Firefly tracks on blockchain transactions.
// All other data is consider protocol specific, and hence stored as opaque data.
type TransactionStatus = core.OpStatus

// BatchPin is the set of data pinned to the blockchain for a batch - whether it's private or broadcast.
type BatchPin struct {

	// Namespace goes in the clear on the chain
	Namespace string

	// TransactionID is the firefly transaction ID allocated before transaction submission for correlation with events (it's a UUID so no leakage)
	TransactionID *fftypes.UUID

	// BatchID is the id of the batch - not strictly required, but writing this in plain text to the blockchain makes for easy human correlation on-chain/off-chain (it's a UUID so no leakage)
	BatchID *fftypes.UUID

	// BatchHash is the SHA256 hash of the batch
	BatchHash *fftypes.Bytes32

	// BatchPayloadRef is a string that can be passed to to the storage interface to retrieve the payload. Nil for private messages
	BatchPayloadRef string

	// Contexts is an array of hashes that allow the FireFly runtimes to identify whether one of the messgages in
	// that batch is the next message for a sequence that involves that node. If so that means the FireFly runtime must
	//
	// - The primary subject of each hash is a "context"
	// - The context is a function of:
	//   - A single topic declared in a message - topics are just a string representing a sequence of events that must be processed in order
	//   - A ledger - everone with access to this ledger will see these hashes (Fabric channel, Ethereum chain, EEA privacy group, Corda linear ID)
	//   - A restricted group - if the mesage is private, these are the nodes that are eligible to receive a copy of the private message+data
	// - Each message might choose to include multiple topics (and hence attach to multiple contexts)
	//   - This allows multiple contexts to merge - very important in multi-party data matching scenarios
	// - A batch contains many messages, each with one or more topics
	//   - The array of sequence hashes will cover every unique context within the batch
	// - For private group communications, the hash is augmented as follow:
	//   - The hashes are salted with a UUID that is only passed off chain (the UUID of the Group).
	//   - The hashes are made unique to the sender
	//   - The hashes contain a sender specific nonce that is a monotonically increasing number
	//     for batches sent by that sender, within the context (maintined by the sender FireFly node)
	Contexts []*fftypes.Bytes32

	// Event contains info on the underlying blockchain event for this batch pin
	Event Event
}

type Event struct {
	// Source indicates where the event originated (ie plugin name)
	Source string

	// Name is a short name for the event
	Name string

	// ProtocolID is an alphanumerically sortable string that represents this event uniquely on the blockchain
	ProtocolID string

	// Output is the raw output data from the event
	Output fftypes.JSONObject

	// Info is any additional blockchain info for the event (transaction hash, block number, etc)
	Info fftypes.JSONObject

	// Timestamp is the time the event was emitted from the blockchain
	Timestamp *fftypes.FFTime

	// We capture the blockchain TXID as in the case
	// of a FireFly transaction we want to reflect that blockchain TX back onto the FireFly TX object
	BlockchainTXID string

	// Location is the blockchain location of the contract that emitted the event
	Location string

	// Signature is the event signature, including the event name and output types
	Signature string
}

type EventWithSubscription struct {
	Event

	// Subscription is the ID assigned to a custom contract subscription by the connector
	Subscription string
}
