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

package fabric

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/go-resty/resty/v2"
	"github.com/hyperledger/firefly-common/pkg/config"
	"github.com/hyperledger/firefly-common/pkg/ffresty"
	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly-common/pkg/wsclient"
	"github.com/hyperledger/firefly/internal/coremsgs"
	"github.com/hyperledger/firefly/internal/metrics"
	"github.com/hyperledger/firefly/pkg/blockchain"
	"github.com/hyperledger/firefly/pkg/core"
)

const (
	broadcastBatchEventName = "BatchPin"
)

type Fabric struct {
	ctx             context.Context
	topic           string
	defaultChannel  string
	signer          string
	prefixShort     string
	prefixLong      string
	capabilities    *blockchain.Capabilities
	callbacks       callbacks
	client          *resty.Client
	streams         *streamManager
	streamID        string
	fireflyContract struct {
		mux            sync.Mutex
		chaincode      string
		fromBlock      string
		networkVersion int
		subscription   string
	}
	idCache          map[string]*fabIdentity
	wsconn           wsclient.WSClient
	closed           chan struct{}
	metrics          metrics.Manager
	fabconnectConf   config.Section
	contractConf     config.ArraySection
	contractConfSize int
}

type callbacks struct {
	listeners []blockchain.Callbacks
}

func (cb *callbacks) BlockchainOpUpdate(plugin blockchain.Plugin, nsOpID string, txState blockchain.TransactionStatus, blockchainTXID, errorMessage string, opOutput fftypes.JSONObject) {
	for _, cb := range cb.listeners {
		cb.BlockchainOpUpdate(plugin, nsOpID, txState, blockchainTXID, errorMessage, opOutput)
	}
}

func (cb *callbacks) BatchPinComplete(batch *blockchain.BatchPin, signingKey *core.VerifierRef) error {
	for _, cb := range cb.listeners {
		if err := cb.BatchPinComplete(batch, signingKey); err != nil {
			return err
		}
	}
	return nil
}

func (cb *callbacks) BlockchainNetworkAction(action string, event *blockchain.Event, signingKey *core.VerifierRef) error {
	for _, cb := range cb.listeners {
		if err := cb.BlockchainNetworkAction(action, event, signingKey); err != nil {
			return err
		}
	}
	return nil
}

func (cb *callbacks) BlockchainEvent(event *blockchain.EventWithSubscription) error {
	for _, cb := range cb.listeners {
		if err := cb.BlockchainEvent(event); err != nil {
			return err
		}
	}
	return nil
}

type eventStreamWebsocket struct {
	Topic string `json:"topic"`
}

type fabTxInputHeaders struct {
	ID            string         `json:"id,omitempty"`
	Type          string         `json:"type"`
	PayloadSchema *PayloadSchema `json:"payloadSchema,omitempty"`
	Signer        string         `json:"signer,omitempty"`
	Channel       string         `json:"channel,omitempty"`
	Chaincode     string         `json:"chaincode,omitempty"`
}

type fabError struct {
	Error string `json:"error,omitempty"`
}

type PayloadSchema struct {
	Type        string        `json:"type"`
	PrefixItems []*PrefixItem `json:"prefixItems"`
}

type PrefixItem struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type fabQueryNamedOutput struct {
	Headers *fabTxInputHeaders `json:"headers"`
	Result  interface{}        `json:"result"`
}

type ffiParamSchema struct {
	Type string `json:"type,omitempty"`
}

type fabWSCommandPayload struct {
	Type  string `json:"type"`
	Topic string `json:"topic,omitempty"`
}

type fabIdentity struct {
	MSPID  string `json:"mspId"`
	ECert  string `json:"enrollmentCert"`
	CACert string `json:"caCert"`
}

type Location struct {
	Channel   string `json:"channel"`
	Chaincode string `json:"chaincode"`
}

var batchPinEvent = "BatchPin"
var batchPinMethodName = "PinBatch"
var batchPinPrefixItems = []*PrefixItem{
	{
		Name: "namespace",
		Type: "string",
	},
	{
		Name: "uuids",
		Type: "string",
	},
	{
		Name: "batchHash",
		Type: "string",
	},
	{
		Name: "payloadRef",
		Type: "string",
	},
	{
		Name: "contexts",
		Type: "string",
	},
}
var networkVersionMethodName = "NetworkVersion"

var fullIdentityPattern = regexp.MustCompile(".+::x509::(.+)::.+")

var cnPattern = regexp.MustCompile("CN=([^,]+)")

func (f *Fabric) Name() string {
	return "fabric"
}

func (f *Fabric) VerifierType() core.VerifierType {
	return core.VerifierTypeMSPIdentity
}

func (f *Fabric) Init(ctx context.Context, config config.Section, metrics metrics.Manager) (err error) {
	f.InitConfig(config)
	fabconnectConf := f.fabconnectConf

	f.ctx = log.WithLogField(ctx, "proto", "fabric")
	f.idCache = make(map[string]*fabIdentity)
	f.metrics = metrics
	f.capabilities = &blockchain.Capabilities{}

	if fabconnectConf.GetString(ffresty.HTTPConfigURL) == "" {
		return i18n.NewError(ctx, coremsgs.MsgMissingPluginConfig, "url", "blockchain.fabric.fabconnect")
	}
	f.client = ffresty.New(f.ctx, fabconnectConf)

	f.defaultChannel = fabconnectConf.GetString(FabconnectConfigDefaultChannel)
	// the org identity is guaranteed to be configured by the core
	f.signer = fabconnectConf.GetString(FabconnectConfigSigner)
	f.topic = fabconnectConf.GetString(FabconnectConfigTopic)
	if f.topic == "" {
		return i18n.NewError(ctx, coremsgs.MsgMissingPluginConfig, "topic", "blockchain.fabric.fabconnect")
	}
	f.prefixShort = fabconnectConf.GetString(FabconnectPrefixShort)
	f.prefixLong = fabconnectConf.GetString(FabconnectPrefixLong)

	wsConfig := wsclient.GenerateConfig(fabconnectConf)
	if wsConfig.WSKeyPath == "" {
		wsConfig.WSKeyPath = "/ws"
	}
	f.wsconn, err = wsclient.New(f.ctx, wsConfig, nil, f.afterConnect)
	if err != nil {
		return err
	}

	f.streams = &streamManager{client: f.client, signer: f.signer}
	batchSize := f.fabconnectConf.GetUint(FabconnectConfigBatchSize)
	batchTimeout := uint(f.fabconnectConf.GetDuration(FabconnectConfigBatchTimeout).Milliseconds())
	stream, err := f.streams.ensureEventStream(f.ctx, f.topic, batchSize, batchTimeout)
	if err != nil {
		return err
	}
	f.streamID = stream.ID
	log.L(f.ctx).Infof("Event stream: %s", f.streamID)

	f.closed = make(chan struct{})
	go f.eventLoop()

	return nil
}

func (f *Fabric) resolveFireFlyContract(ctx context.Context, contractIndex int) (chaincode, fromBlock string, err error) {

	if f.contractConfSize > 0 || contractIndex > 0 {
		// New config (array of objects under "fireflyContract")
		if contractIndex >= f.contractConfSize {
			return "", "", i18n.NewError(ctx, coremsgs.MsgInvalidFireFlyContractIndex, fmt.Sprintf("blockchain.fabric.fireflyContract[%d]", contractIndex))
		}
		chaincode = f.contractConf.ArrayEntry(contractIndex).GetString(FireFlyContractChaincode)
		if chaincode == "" {
			return "", "", i18n.NewError(ctx, coremsgs.MsgMissingPluginConfig, "chaincode", "blockchain.fabric.fireflyContract")
		}
		fromBlock = f.contractConf.ArrayEntry(contractIndex).GetString(FireFlyContractFromBlock)
	} else {
		// Old config (attributes under "ethconnect")
		chaincode = f.fabconnectConf.GetString(FabconnectConfigChaincodeDeprecated)
		if chaincode != "" {
			log.L(ctx).Warnf("The %s.%s config key has been deprecated. Please use %s.%s instead",
				FabconnectConfigKey, FabconnectConfigChaincodeDeprecated,
				FireFlyContractConfigKey, FireFlyContractChaincode)
			fromBlock = string(core.SubOptsFirstEventOldest)
		} else {
			return "", "", i18n.NewError(ctx, coremsgs.MsgMissingPluginConfig, "chaincode", "blockchain.fabric.fabconnect")
		}
	}

	return chaincode, fromBlock, nil
}

func (f *Fabric) ConfigureContract(ctx context.Context, contracts *core.FireFlyContracts) (err error) {

	chaincode, fromBlock, err := f.resolveFireFlyContract(ctx, contracts.Active.Index)
	if err != nil {
		return err
	}

	location := &Location{Channel: f.defaultChannel, Chaincode: chaincode}
	sub, err := f.streams.ensureFireFlySubscription(ctx, location, fromBlock, f.streamID, batchPinEvent)
	if err == nil {
		var version int
		version, err = f.getNetworkVersion(ctx, chaincode)
		if err == nil {
			f.fireflyContract.mux.Lock()
			f.fireflyContract.chaincode = chaincode
			f.fireflyContract.fromBlock = fromBlock
			f.fireflyContract.networkVersion = version
			f.fireflyContract.subscription = sub.ID
			f.fireflyContract.mux.Unlock()
			contracts.Active.Info = fftypes.JSONObject{
				"chaincode":    chaincode,
				"fromBlock":    fromBlock,
				"subscription": sub.ID,
			}
		}
	}
	return err
}

func (f *Fabric) TerminateContract(ctx context.Context, contracts *core.FireFlyContracts, termination *blockchain.Event) (err error) {

	chaincode := termination.Info.GetString("chaincodeId")
	f.fireflyContract.mux.Lock()
	fireflyChaincode := f.fireflyContract.chaincode
	f.fireflyContract.mux.Unlock()
	if chaincode != fireflyChaincode {
		log.L(ctx).Warnf("Ignoring termination request from chaincode %s, which differs from active chaincode %s", chaincode, fireflyChaincode)
		return nil
	}

	log.L(ctx).Infof("Processing termination request from chaincode %s", chaincode)
	contracts.Active.FinalEvent = termination.ProtocolID
	contracts.Terminated = append(contracts.Terminated, contracts.Active)
	contracts.Active = core.FireFlyContractInfo{Index: contracts.Active.Index + 1}
	return f.ConfigureContract(ctx, contracts)
}

func (f *Fabric) RegisterListener(listener blockchain.Callbacks) {
	f.callbacks.listeners = append(f.callbacks.listeners, listener)
}

func (f *Fabric) Start() (err error) {
	return f.wsconn.Connect()
}

func (f *Fabric) Capabilities() *blockchain.Capabilities {
	return f.capabilities
}

func (f *Fabric) afterConnect(ctx context.Context, w wsclient.WSClient) error {
	// Send a subscribe to our topic after each connect/reconnect
	b, _ := json.Marshal(&fabWSCommandPayload{
		Type:  "listen",
		Topic: f.topic,
	})
	err := w.Send(ctx, b)
	if err == nil {
		b, _ = json.Marshal(&fabWSCommandPayload{
			Type: "listenreplies",
		})
		err = w.Send(ctx, b)
	}
	return err
}

func decodeJSONPayload(ctx context.Context, payloadString string) *fftypes.JSONObject {
	bytes, err := base64.StdEncoding.DecodeString(payloadString)
	if err != nil {
		log.L(ctx).Errorf("BatchPin event is not valid - bad payload content: %s", payloadString)
		return nil
	}
	dataBytes := fftypes.JSONAnyPtrBytes(bytes)
	payload, ok := dataBytes.JSONObjectOk()
	if !ok {
		log.L(ctx).Errorf("BatchPin event is not valid - bad JSON payload: %s", bytes)
		return nil
	}
	return &payload
}

func (f *Fabric) parseBlockchainEvent(ctx context.Context, msgJSON fftypes.JSONObject) *blockchain.Event {
	payloadString := msgJSON.GetString("payload")
	payload := decodeJSONPayload(ctx, payloadString)
	if payload == nil {
		return nil // move on
	}

	sTransactionHash := msgJSON.GetString("transactionId")
	blockNumber := msgJSON.GetInt64("blockNumber")
	transactionIndex := msgJSON.GetInt64("transactionIndex")
	eventIndex := msgJSON.GetInt64("eventIndex")
	name := msgJSON.GetString("eventName")
	timestamp := msgJSON.GetInt64("timestamp")
	chaincode := msgJSON.GetString("chaincodeId")

	delete(msgJSON, "payload")
	return &blockchain.Event{
		BlockchainTXID: sTransactionHash,
		Source:         f.Name(),
		Name:           name,
		ProtocolID:     fmt.Sprintf("%.12d/%.6d/%.6d", blockNumber, transactionIndex, eventIndex),
		Output:         *payload,
		Info:           msgJSON,
		Timestamp:      fftypes.UnixTime(timestamp),
		Location:       f.buildEventLocationString(chaincode),
		Signature:      name,
	}
}

func (f *Fabric) handleBatchPinEvent(ctx context.Context, msgJSON fftypes.JSONObject) (err error) {
	event := f.parseBlockchainEvent(ctx, msgJSON)
	if event == nil {
		return nil // move on
	}

	signer := event.Output.GetString("signer")
	nsOrAction := event.Output.GetString("namespace")
	sUUIDs := event.Output.GetString("uuids")
	sBatchHash := event.Output.GetString("batchHash")
	sPayloadRef := event.Output.GetString("payloadRef")
	sContexts := event.Output.GetStringArray("contexts")

	verifier := &core.VerifierRef{
		Type:  core.VerifierTypeMSPIdentity,
		Value: signer,
	}

	// Check if this is actually an operator action
	if strings.HasPrefix(nsOrAction, blockchain.FireFlyActionPrefix) {
		action := nsOrAction[len(blockchain.FireFlyActionPrefix):]
		return f.callbacks.BlockchainNetworkAction(action, event, verifier)
	}

	hexUUIDs, err := hex.DecodeString(strings.TrimPrefix(sUUIDs, "0x"))
	if err != nil || len(hexUUIDs) != 32 {
		log.L(ctx).Errorf("BatchPin event is not valid - bad uuids (%s): %s", sUUIDs, err)
		return nil // move on
	}
	var txnID fftypes.UUID
	copy(txnID[:], hexUUIDs[0:16])
	var batchID fftypes.UUID
	copy(batchID[:], hexUUIDs[16:32])

	var batchHash fftypes.Bytes32
	err = batchHash.UnmarshalText([]byte(sBatchHash))
	if err != nil {
		log.L(ctx).Errorf("BatchPin event is not valid - bad batchHash (%s): %s", sBatchHash, err)
		return nil // move on
	}

	contexts := make([]*fftypes.Bytes32, len(sContexts))
	for i, sHash := range sContexts {
		var hash fftypes.Bytes32
		err = hash.UnmarshalText([]byte(sHash))
		if err != nil {
			log.L(ctx).Errorf("BatchPin event is not valid - bad pin %d (%s): %s", i, sHash, err)
			return nil // move on
		}
		contexts[i] = &hash
	}

	batch := &blockchain.BatchPin{
		Namespace:       nsOrAction,
		TransactionID:   &txnID,
		BatchID:         &batchID,
		BatchHash:       &batchHash,
		BatchPayloadRef: sPayloadRef,
		Contexts:        contexts,
		Event:           *event,
	}

	// If there's an error dispatching the event, we must return the error and shutdown
	return f.callbacks.BatchPinComplete(batch, verifier)
}

func (f *Fabric) buildEventLocationString(chaincode string) string {
	return fmt.Sprintf("chaincode=%s", chaincode)
}

func (f *Fabric) handleContractEvent(ctx context.Context, msgJSON fftypes.JSONObject) (err error) {
	event := f.parseBlockchainEvent(ctx, msgJSON)
	if event == nil {
		return nil // move on
	}
	return f.callbacks.BlockchainEvent(&blockchain.EventWithSubscription{
		Event:        *event,
		Subscription: msgJSON.GetString("subId"),
	})
}

func (f *Fabric) handleReceipt(ctx context.Context, reply fftypes.JSONObject) {
	l := log.L(ctx)

	headers := reply.GetObject("headers")
	requestID := headers.GetString("requestId")
	replyType := headers.GetString("type")
	txHash := reply.GetString("transactionId")
	message := reply.GetString("errorMessage")
	if requestID == "" || replyType == "" {
		l.Errorf("Reply cannot be processed: %+v", reply)
		return
	}
	updateType := core.OpStatusSucceeded
	if replyType != "TransactionSuccess" {
		updateType = core.OpStatusFailed
	}
	l.Infof("Fabconnect '%s' reply tx=%s (request=%s) %s", replyType, txHash, requestID, message)
	f.callbacks.BlockchainOpUpdate(f, requestID, updateType, txHash, message, reply)
}

func (f *Fabric) handleMessageBatch(ctx context.Context, messages []interface{}) error {
	l := log.L(ctx)

	for i, msgI := range messages {
		msgMap, ok := msgI.(map[string]interface{})
		if !ok {
			l.Errorf("Message cannot be parsed as JSON: %+v", msgI)
			return nil // Swallow this and move on
		}
		msgJSON := fftypes.JSONObject(msgMap)

		l1 := l.WithField("fabmsgidx", i)
		ctx1 := log.WithLogger(ctx, l1)
		eventName := msgJSON.GetString("eventName")
		sub := msgJSON.GetString("subId")
		l1.Infof("Received '%s' message", eventName)
		l1.Tracef("Message: %+v", msgJSON)

		f.fireflyContract.mux.Lock()
		fireflySub := f.fireflyContract.subscription
		f.fireflyContract.mux.Unlock()

		if sub == fireflySub {
			// Matches the active FireFly BatchPin subscription
			switch eventName {
			case broadcastBatchEventName:
				if err := f.handleBatchPinEvent(ctx1, msgJSON); err != nil {
					return err
				}
			default:
				l.Infof("Ignoring event with unknown name: %s", eventName)
			}
		} else {
			// Subscription not recognized - assume it's from a custom contract listener
			// (event manager will reject it if it's not)
			if err := f.handleContractEvent(ctx, msgJSON); err != nil {
				return err
			}
		}
	}

	return nil
}

func (f *Fabric) eventLoop() {
	defer f.wsconn.Close()
	defer close(f.closed)
	l := log.L(f.ctx).WithField("role", "event-loop")
	ctx := log.WithLogger(f.ctx, l)
	ack, _ := json.Marshal(map[string]string{"type": "ack", "topic": f.topic})
	for {
		select {
		case <-ctx.Done():
			l.Debugf("Event loop exiting (context cancelled)")
			return
		case msgBytes, ok := <-f.wsconn.Receive():
			if !ok {
				l.Debugf("Event loop exiting (receive channel closed)")
				return
			}

			var msgParsed interface{}
			err := json.Unmarshal(msgBytes, &msgParsed)
			if err != nil {
				l.Errorf("Message cannot be parsed as JSON: %s\n%s", err, string(msgBytes))
				continue // Swallow this and move on
			}
			switch msgTyped := msgParsed.(type) {
			case []interface{}:
				err = f.handleMessageBatch(ctx, msgTyped)
				if err == nil {
					err = f.wsconn.Send(ctx, ack)
				}
			case map[string]interface{}:
				f.handleReceipt(ctx, fftypes.JSONObject(msgTyped))
			default:
				l.Errorf("Message unexpected: %+v", msgTyped)
				continue
			}

			// Send the ack - only fails if shutting down
			if err != nil {
				l.Errorf("Event loop exiting: %s", err)
				return
			}
		}
	}
}

func (f *Fabric) NormalizeSigningKey(ctx context.Context, signingKeyInput string) (string, error) {
	// we expand the short user name into the fully qualified onchain identity:
	// mspid::x509::{ecert DN}::{CA DN}	return signingKeyInput, nil
	if !fullIdentityPattern.MatchString(signingKeyInput) {
		existingID := f.idCache[signingKeyInput]
		if existingID == nil {
			var idRes fabIdentity
			res, err := f.client.R().SetContext(f.ctx).SetResult(&idRes).Get(fmt.Sprintf("/identities/%s", signingKeyInput))
			if err != nil || !res.IsSuccess() {
				return "", i18n.NewError(f.ctx, coremsgs.MsgFabconnectRESTErr, err)
			}
			f.idCache[signingKeyInput] = &idRes
			existingID = &idRes
		}

		ecertDN, err := getDNFromCertString(existingID.ECert)
		if err != nil {
			return "", i18n.NewError(f.ctx, coremsgs.MsgFailedToDecodeCertificate, err)
		}
		cacertDN, err := getDNFromCertString(existingID.CACert)
		if err != nil {
			return "", i18n.NewError(f.ctx, coremsgs.MsgFailedToDecodeCertificate, err)
		}
		resolvedSigningKey := fmt.Sprintf("%s::x509::%s::%s", existingID.MSPID, ecertDN, cacertDN)
		log.L(f.ctx).Debugf("Resolved signing key: %s", resolvedSigningKey)
		return resolvedSigningKey, nil
	}
	return signingKeyInput, nil
}

func wrapError(ctx context.Context, errRes *fabError, res *resty.Response, err error) error {
	if errRes != nil && errRes.Error != "" {
		return i18n.WrapError(ctx, err, coremsgs.MsgFabconnectRESTErr, errRes.Error)
	}
	return ffresty.WrapRestErr(ctx, res, err, coremsgs.MsgFabconnectRESTErr)
}

func (f *Fabric) invokeContractMethod(ctx context.Context, channel, chaincode, methodName, signingKey, requestID string, prefixItems []*PrefixItem, input map[string]interface{}, options map[string]interface{}) error {
	body, err := f.buildFabconnectRequestBody(ctx, channel, chaincode, methodName, signingKey, requestID, prefixItems, input, options)
	if err != nil {
		return err
	}
	var resErr fabError
	res, err := f.client.R().
		SetContext(ctx).
		SetHeader("x-firefly-sync", "false").
		SetBody(body).
		SetError(&resErr).
		Post("/transactions")
	if err != nil || !res.IsSuccess() {
		return wrapError(ctx, &resErr, res, err)
	}
	return nil
}

func (f *Fabric) queryContractMethod(ctx context.Context, channel, chaincode, methodName, signingKey, requestID string, prefixItems []*PrefixItem, input map[string]interface{}, options map[string]interface{}) (*resty.Response, error) {
	body, err := f.buildFabconnectRequestBody(ctx, channel, chaincode, methodName, signingKey, requestID, prefixItems, input, options)
	if err != nil {
		return nil, err
	}
	var resErr fabError
	res, err := f.client.R().
		SetContext(ctx).
		SetBody(body).
		SetError(&resErr).
		Post("/query")
	if err != nil || !res.IsSuccess() {
		return res, wrapError(ctx, &resErr, res, err)
	}
	return res, nil
}

func getUserName(fullIDString string) string {
	matches := fullIdentityPattern.FindStringSubmatch(fullIDString)
	if len(matches) == 0 {
		return fullIDString
	}
	matches = cnPattern.FindStringSubmatch(matches[1])
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func hexFormatB32(b *fftypes.Bytes32) string {
	if b == nil {
		return "0x0000000000000000000000000000000000000000000000000000000000000000"
	}
	return "0x" + hex.EncodeToString(b[0:32])
}

func (f *Fabric) SubmitBatchPin(ctx context.Context, nsOpID string, signingKey string, batch *blockchain.BatchPin) error {
	hashes := make([]string, len(batch.Contexts))
	for i, v := range batch.Contexts {
		hashes[i] = hexFormatB32(v)
	}
	var uuids fftypes.Bytes32
	copy(uuids[0:16], (*batch.TransactionID)[:])
	copy(uuids[16:32], (*batch.BatchID)[:])
	pinInput := map[string]interface{}{
		"namespace":  batch.Namespace,
		"uuids":      hexFormatB32(&uuids),
		"batchHash":  hexFormatB32(batch.BatchHash),
		"payloadRef": batch.BatchPayloadRef,
		"contexts":   hashes,
	}
	input, _ := jsonEncodeInput(pinInput)
	f.fireflyContract.mux.Lock()
	chaincode := f.fireflyContract.chaincode
	f.fireflyContract.mux.Unlock()
	return f.invokeContractMethod(ctx, f.defaultChannel, chaincode, batchPinMethodName, signingKey, nsOpID, batchPinPrefixItems, input, nil)
}

func (f *Fabric) SubmitNetworkAction(ctx context.Context, nsOpID string, signingKey string, action core.NetworkActionType) error {
	pinInput := map[string]interface{}{
		"namespace":  "firefly:" + action,
		"uuids":      hexFormatB32(nil),
		"batchHash":  hexFormatB32(nil),
		"payloadRef": "",
		"contexts":   []string{},
	}
	input, _ := jsonEncodeInput(pinInput)
	f.fireflyContract.mux.Lock()
	chaincode := f.fireflyContract.chaincode
	f.fireflyContract.mux.Unlock()
	return f.invokeContractMethod(ctx, f.defaultChannel, chaincode, batchPinMethodName, signingKey, nsOpID, batchPinPrefixItems, input, nil)
}

func (f *Fabric) buildFabconnectRequestBody(ctx context.Context, channel, chaincode, methodName, signingKey, requestID string, prefixItems []*PrefixItem, input map[string]interface{}, options map[string]interface{}) (map[string]interface{}, error) {
	// All arguments must be JSON serialized
	args, err := jsonEncodeInput(input)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, i18n.MsgJSONObjectParseFailed, "params")
	}
	body := map[string]interface{}{
		"headers": &fabTxInputHeaders{
			ID: requestID,
			PayloadSchema: &PayloadSchema{
				Type:        "array",
				PrefixItems: prefixItems,
			},
			Channel:   channel,
			Chaincode: chaincode,
			Signer:    getUserName(signingKey),
		},
		"func": methodName,
		"args": args,
	}
	for k, v := range options {
		// Set the new field if it's not already set. Do not allow overriding of existing fields
		if _, ok := body[k]; !ok {
			body[k] = v
		} else {
			return nil, i18n.NewError(ctx, coremsgs.MsgOverrideExistingFieldCustomOption, k)
		}
	}
	return body, nil
}

func (f *Fabric) InvokeContract(ctx context.Context, nsOpID string, signingKey string, location *fftypes.JSONAny, method *core.FFIMethod, input map[string]interface{}, options map[string]interface{}) error {
	fabricOnChainLocation, err := parseContractLocation(ctx, location)
	if err != nil {
		return err
	}

	// Build the payload schema for the method parameters
	prefixItems := make([]*PrefixItem, len(method.Params))
	for i, param := range method.Params {
		var paramSchema ffiParamSchema
		if err := json.Unmarshal(param.Schema.Bytes(), &paramSchema); err != nil {
			return i18n.WrapError(ctx, err, i18n.MsgJSONObjectParseFailed, fmt.Sprintf("%s.schema", param.Name))
		}

		prefixItems[i] = &PrefixItem{
			Name: param.Name,
			Type: paramSchema.Type,
		}
	}

	return f.invokeContractMethod(ctx, fabricOnChainLocation.Channel, fabricOnChainLocation.Chaincode, method.Name, signingKey, nsOpID, prefixItems, input, options)
}

func (f *Fabric) QueryContract(ctx context.Context, location *fftypes.JSONAny, method *core.FFIMethod, input map[string]interface{}, options map[string]interface{}) (interface{}, error) {
	fabricOnChainLocation, err := parseContractLocation(ctx, location)
	if err != nil {
		return nil, err
	}

	// Build the payload schema for the method parameters
	prefixItems := make([]*PrefixItem, len(method.Params))
	for i, param := range method.Params {
		prefixItems[i] = &PrefixItem{
			Name: param.Name,
			Type: "string",
		}
	}

	res, err := f.queryContractMethod(ctx, fabricOnChainLocation.Channel, fabricOnChainLocation.Chaincode, method.Name, f.signer, "", prefixItems, input, options)
	if err != nil {
		return nil, err
	}
	output := &fabQueryNamedOutput{}
	if err = json.Unmarshal(res.Body(), output); err != nil {
		return nil, err
	}
	return output.Result, nil
}

func jsonEncodeInput(params map[string]interface{}) (output map[string]interface{}, err error) {
	output = make(map[string]interface{}, len(params))
	for field, value := range params {
		switch v := value.(type) {
		case string:
			output[field] = v
		default:
			encodedValue, err := json.Marshal(v)
			if err != nil {
				return nil, err
			}
			output[field] = string(encodedValue)
		}

	}
	return
}

func (f *Fabric) NormalizeContractLocation(ctx context.Context, location *fftypes.JSONAny) (result *fftypes.JSONAny, err error) {
	parsed, err := parseContractLocation(ctx, location)
	if err != nil {
		return nil, err
	}
	normalized, err := json.Marshal(parsed)
	if err == nil {
		result = fftypes.JSONAnyPtrBytes(normalized)
	}
	return result, err
}

func parseContractLocation(ctx context.Context, location *fftypes.JSONAny) (*Location, error) {
	fabricLocation := Location{}
	if err := json.Unmarshal(location.Bytes(), &fabricLocation); err != nil {
		return nil, i18n.NewError(ctx, coremsgs.MsgContractLocationInvalid, err)
	}
	if fabricLocation.Channel == "" {
		return nil, i18n.NewError(ctx, coremsgs.MsgContractLocationInvalid, "'channel' not set")
	}
	if fabricLocation.Chaincode == "" {
		return nil, i18n.NewError(ctx, coremsgs.MsgContractLocationInvalid, "'chaincode' not set")
	}
	return &fabricLocation, nil
}

func (f *Fabric) AddContractListener(ctx context.Context, listener *core.ContractListenerInput) error {
	location, err := parseContractLocation(ctx, listener.Location)
	if err != nil {
		return err
	}
	result, err := f.streams.createSubscription(ctx, location, f.streamID, "", listener.Event.Name, listener.Options.FirstEvent)
	if err != nil {
		return err
	}
	listener.BackendID = result.ID
	return nil
}

func (f *Fabric) DeleteContractListener(ctx context.Context, subscription *core.ContractListener) error {
	return f.streams.deleteSubscription(ctx, subscription.BackendID)
}

func (f *Fabric) GetFFIParamValidator(ctx context.Context) (core.FFIParamValidator, error) {
	// Fabconnect does not require any additional validation beyond "JSON Schema correctness" at this time
	return nil, nil
}

func (f *Fabric) GenerateFFI(ctx context.Context, generationRequest *core.FFIGenerationRequest) (*core.FFI, error) {
	return nil, i18n.NewError(ctx, coremsgs.MsgFFIGenerationUnsupported)
}

func (f *Fabric) GenerateEventSignature(ctx context.Context, event *core.FFIEventDefinition) string {
	return event.Name
}

func (f *Fabric) getNetworkVersion(ctx context.Context, chaincode string) (int, error) {
	res, err := f.queryContractMethod(ctx, f.defaultChannel, chaincode, networkVersionMethodName, f.signer, "", []*PrefixItem{}, map[string]interface{}{}, nil)
	if err != nil || !res.IsSuccess() {
		// "Function not found" is interpreted as "default to version 1"
		notFoundError := fmt.Sprintf("Function %s not found", networkVersionMethodName)
		if strings.Contains(err.Error(), notFoundError) {
			return 1, nil
		}
		return 0, err
	}
	output := &fabQueryNamedOutput{}
	if err = json.Unmarshal(res.Body(), output); err != nil {
		return 0, err
	}
	return int(output.Result.(float64)), nil
}

func (f *Fabric) NetworkVersion() int {
	f.fireflyContract.mux.Lock()
	defer f.fireflyContract.mux.Unlock()
	return f.fireflyContract.networkVersion
}
